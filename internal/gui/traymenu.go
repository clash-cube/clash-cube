package gui

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/mihomobar/internal/backend"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// trayMenu is the tray icon's right-click menu, laid out as Surge's is
// (docs/design.md §10.2): the main window, the outbound mode, a submenu per
// proxy group, the connectivity quality, the apps moving the most traffic,
// the two switches, profiles, then the core and the app. It is built afresh
// from the backend each time something it shows changes, but not while it
// shows: then only the quality and the apps change, in place.
type trayMenu struct {
	h  *host
	mu sync.Mutex

	last          backend.State
	groups        []Group
	pending       bool   // a rebuild is scheduled
	refreshAgain  bool   // changes arrived while a group snapshot was loading
	groupRevision uint64 // streamed results must not be overwritten by older snapshots

	open  bool          // the menu shows
	stale bool          // it should be rebuilt once it closes
	stop  chan struct{} // ends the sampling while it shows

	meter     backend.ClientMeter
	clients   []backend.ClientRate
	quality   int // ms through the proxy (direct in direct mode); 0 unmeasured, -1 failed
	measured  time.Time
	measuring bool
	testing   map[string]backend.LatencyEvent // active tests from any surface

	// the rows that change while the menu shows; main thread only
	qualityItem *application.MenuItem
	clientItems []*application.MenuItem
	nodes       map[string][]nodeRow             // group → its nodes' rows
	tests       map[string]*application.MenuItem // group → its "Test Latency" row
}

// topClients is how many app rows the menu has, filled or not, as Surge's.
const topClients = 5

// theTrayMenu is the menu AppKit's open and close callbacks reach.
var theTrayMenu *trayMenu

func newTrayMenu(h *host) *trayMenu {
	m := &trayMenu{h: h, last: h.b.State(), testing: map[string]backend.LatencyEvent{}}
	theTrayMenu = m
	return m
}

// zh says whether the menu is in Chinese.
func zh() bool {
	switch settings.Load().Lang {
	case "zh":
		return true
	case "en":
		return false
	}
	for _, k := range []string{"LC_ALL", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return strings.HasPrefix(v, "zh")
		}
	}
	return systemPrefersChinese()
}

func tr(en, cn string) string {
	if zh() {
		return cn
	}
	return en
}

// The menu's labels carry a little markup that styleTrayMenu draws
// (traymenu_darwin.go). detail puts right at the menu's right edge, in the
// secondary colour; a right made by badge is drawn as a coloured badge;
// withIcon puts a file's icon before a name; subtitled adds a second line.
func detail(left, right string) string {
	if right == "" {
		return left
	}
	return left + "\t" + right
}

// badge's kind is accent, good, ok, bad or none.
func badge(kind, text string) string { return "\x01" + kind + "\x02" + text }

func withIcon(path, name string) string { return "\x03" + path + "\x04" + name }

// keyed gives a row its shortcut as Surge shows it: the key drawn as the
// row's detail, and held by a hidden twin. A real key equivalent would get
// a column of its own, which pushes every row's detail away from the
// submenu arrows.
func keyed(menu *application.Menu, it *application.MenuItem, key string, click func(*application.Context)) {
	it.SetLabel(detail(it.Label(), "⌘ "+key)).OnClick(click)
	menu.Add(it.Label()).SetAccelerator("CmdOrCtrl+" + key).SetHidden(true).OnClick(click)
}

// subtitled gives a row a second line under its title, in a smaller
// secondary font, as Surge's outbound modes have.
func subtitled(title, sub string) string { return title + "\x06" + sub }

// stay marks a row whose click leaves the menu up, to watch what it does.
func stay(label string) string { return "\x05" + label }

// delayBadge is a node's latency as Surge's benchmark shows it. An untested
// node's is blank but holds the room, so delays coming in while the menu
// shows don't outgrow it.
func delayBadge(d int) string {
	if d == 0 {
		return badge("space", "")
	}
	return badge(delayKind(d), delayText(d))
}

func testLabel(testing bool) string {
	if testing {
		return stay(tr("Testing…", "测速中…"))
	}
	return stay(tr("Test Latency", "测速"))
}

// nodeRow is a node's row in a group's submenu, to update in place.
type nodeRow struct {
	name  string
	delay int // as last shown
	item  *application.MenuItem
}

func (m *trayMenu) relabel() { m.rebuild() }

// latencyChanged keeps an open menu's rows in place while showing shared
// progress. Cached groups and all occurrences of a node get the same result.
func (m *trayMenu) latencyChanged(e backend.LatencyEvent) {
	application.InvokeAsync(func() {
		m.mu.Lock()
		m.groupRevision++
		if e.Running {
			m.testing[e.Key] = e
		} else {
			delete(m.testing, e.Key)
		}
		for _, g := range m.groups {
			for i := range g.Members {
				if d, ok := e.Delays[g.Members[i].Name]; ok {
					g.Members[i].Delay = d
				}
			}
		}
		m.mu.Unlock()
		for _, rows := range m.nodes {
			for i := range rows {
				r := &rows[i]
				if d, ok := e.Delays[r.name]; ok {
					r.delay = d
				}
			}
		}
		m.paintLatency()
		if !e.Running {
			m.refresh()
		}
	})
}

// paintLatency runs on the main thread; overlapping tests cannot clear each
// other's pending indicators.
func (m *trayMenu) paintLatency() {
	m.mu.Lock()
	pending := map[string]bool{}
	for _, e := range m.testing {
		for _, name := range e.Pending {
			pending[name] = true
		}
	}
	for group, it := range m.tests {
		_, testing := m.testing["group/"+group]
		it.SetLabel(testLabel(testing))
	}
	m.mu.Unlock()
	for _, rows := range m.nodes {
		for _, r := range rows {
			right := delayBadge(r.delay)
			if pending[r.name] {
				right = badge("none", "···")
			}
			r.item.SetLabel(detail(r.name, right))
		}
	}
	styleTrayMenu()
}

// update takes a new state; groups are reloaded when the core is up.
func (m *trayMenu) update(st backend.State) {
	m.mu.Lock()
	prev := m.last
	m.last = st
	m.groupRevision++
	if prev.Core != st.Core || prev.Profile != st.Profile {
		clear(m.testing)
	}
	// what the quality says depends on the path traffic takes
	retest := st.Core == "running" && (prev.Core != "running" || prev.Mode != st.Mode || prev.Profile != st.Profile)
	if retest {
		m.measured = time.Time{}
	}
	m.mu.Unlock()
	if retest {
		time.AfterFunc(2*time.Second, m.measure)
	}
	m.refresh()
}

// refresh reloads the groups and rebuilds, at most every 300ms.
func (m *trayMenu) refresh() {
	m.mu.Lock()
	if m.pending {
		m.refreshAgain = true
		m.mu.Unlock()
		return
	}
	m.pending = true
	m.mu.Unlock()
	time.AfterFunc(300*time.Millisecond, func() {
		var groups []Group
		m.mu.Lock()
		running := m.last.Core == "running"
		revision := m.groupRevision
		m.mu.Unlock()
		if running {
			groups, _ = (&ProxyService{h: m.h}).Groups()
		}
		application.InvokeAsync(func() {
			m.mu.Lock()
			stale := revision != m.groupRevision
			again := m.refreshAgain || stale
			m.pending, m.refreshAgain = false, false
			if !stale {
				m.groups = groups
			}
			m.mu.Unlock()
			if again {
				m.refresh()
			}
			if stale {
				return
			}
			// Reconcile the open menu too: core group tests can change the
			// selected leaf and its history without rebuilding the menu.
			for _, g := range groups {
				for _, mem := range g.Members {
					for i := range m.nodes[g.Name] {
						r := &m.nodes[g.Name][i]
						if r.name == mem.Name {
							r.delay = mem.Delay
							r.item.SetChecked(r.name == g.Now)
						}
					}
				}
			}
			m.rebuild()
			m.paintLatency()
		})
	})
}

// run does fn off the main thread, logging its error and refreshing after.
func (m *trayMenu) run(name string, fn func() error) func(*application.Context) {
	return func(*application.Context) {
		go func() {
			if err := fn(); err != nil {
				log.Printf("%s: %v", name, err)
			}
			m.refresh()
		}()
	}
}

func delayText(d int) string {
	switch {
	case d > 0:
		return fmt.Sprintf("%d ms", d)
	case d < 0:
		return tr("Failed", "失败")
	}
	return ""
}

// delayKind colours a latency as the window's delay chips do.
func delayKind(d int) string {
	switch {
	case d == 0:
		return "none"
	case d > 0 && d < 200:
		return "good"
	case d > 0 && d < 500:
		return "ok"
	}
	return "bad"
}

// tracking is the menu opening or closing; on the main thread. While it
// shows, the apps are sampled every second and a stale quality measured.
func (m *trayMenu) tracking(open bool) {
	m.mu.Lock()
	m.open = open
	stale := m.stale && !open
	if !open {
		m.stale = false
		if m.stop != nil {
			close(m.stop)
			m.stop = nil
		}
	} else if m.last.Core == "running" && m.stop == nil {
		m.stop = make(chan struct{})
		m.meter = backend.ClientMeter{}
		go m.sample(m.stop)
	}
	retest := open && m.last.Core == "running" && time.Since(m.measured) > 30*time.Second
	m.mu.Unlock()
	if retest {
		go m.measure()
	}
	if stale {
		m.refresh()
	}
}

// sample measures each app's speed until stop: at once, soon after (the
// first sample has no speeds), then every second.
func (m *trayMenu) sample(stop chan struct{}) {
	c, err := m.h.b.Client()
	if err != nil {
		return
	}
	wait := 400 * time.Millisecond
	for first := true; ; first = false {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conns, err := c.Connections(ctx)
		cancel()
		if err == nil {
			m.mu.Lock()
			rates := m.meter.Sample(time.Now(), conns.Connections)
			if !first {
				m.clients = rates
			}
			m.mu.Unlock()
			if !first {
				application.InvokeAsync(m.live)
			}
		}
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}
		wait = time.Second
	}
}

// remeasure takes the quality again, the network having changed.
func (m *trayMenu) remeasure() {
	m.mu.Lock()
	m.measured = time.Time{}
	m.mu.Unlock()
	go m.measure()
}

// measure takes the connectivity quality, once at a time.
func (m *trayMenu) measure() {
	m.mu.Lock()
	if m.measuring || m.last.Core != "running" {
		m.mu.Unlock()
		return
	}
	m.measuring = true
	m.mu.Unlock()
	application.InvokeAsync(m.live)

	c, err := m.h.b.Connectivity()
	d := c.Proxy
	if settings.Load().Mode == "direct" {
		d = c.Internet
	}
	if err != nil {
		d = -1
	}
	m.mu.Lock()
	m.quality, m.measured, m.measuring = d, time.Now(), false
	m.mu.Unlock()
	application.InvokeAsync(m.live)
}

func (m *trayMenu) qualityLabel() string {
	m.mu.Lock()
	d, measuring := m.quality, m.measuring
	m.mu.Unlock()
	label := tr("Connectivity Quality", "连通质量")
	switch {
	case measuring && d == 0:
		return detail(label, badge("none", "···"))
	case d == 0:
		return label
	}
	return detail(label, delayBadge(d))
}

func clientLabel(clients []backend.ClientRate, i int) string {
	if i >= len(clients) {
		return "—"
	}
	c := clients[i]
	return detail(withIcon(c.Path, c.Name), speed(c.Up+c.Down))
}

// live updates the rows that change while the menu shows; main thread.
func (m *trayMenu) live() {
	if m.qualityItem == nil {
		return
	}
	m.mu.Lock()
	clients := m.clients
	m.mu.Unlock()
	m.qualityItem.SetLabel(m.qualityLabel())
	for i, it := range m.clientItems {
		it.SetLabel(clientLabel(clients, i))
		it.SetEnabled(i < len(clients))
	}
	styleTrayMenu()
}

// rebuild makes the menu from what is known; on the main thread.
func (m *trayMenu) rebuild() {
	if m.h.tray == nil {
		return
	}
	watchTrayMenu()
	m.mu.Lock()
	if m.open {
		m.stale = true
		m.mu.Unlock()
		return
	}
	st, groups, clients := m.last, m.groups, m.clients
	m.mu.Unlock()
	b := m.h.b
	running := st.Core == "running"
	menu := m.h.app.NewMenu()

	// the core's state, while it isn't running: the icon says the rest
	if !running {
		status := tr("Stopped", "已停止")
		switch st.Core {
		case "starting":
			status = tr("Starting…", "正在启动…")
		case "crashed":
			status = tr("Core stopped with an error", "内核异常退出")
		}
		menu.Add(status).SetEnabled(false)
		menu.AddSeparator()
	}
	keyed(menu, menu.Add(tr("Show Main Window", "显示主窗口")), "M", func(*application.Context) { m.h.showMain("") })
	menu.AddSeparator()

	// outbound mode, its letter as on the tray icon
	om := menu.AddSubmenu(detail(tr("Outbound Mode", "出站模式"), badge("accent", modeLetter(st.Mode))))
	for i, md := range []struct{ id, en, cn, subEN, subCN string }{
		{"direct", "Direct Outbound", "直接连接",
			"All requests will be sent to the target server directly", "所有请求都将直接发送至目标服务器"},
		{"global", "Global Proxy", "全局代理",
			"All requests will be forwarded to a proxy server", "所有请求都将转发至代理服务器"},
		{"rule", "Rule-Based Proxy", "规则判定",
			"Using rule system to determine how to process requests", "使用规则系统决定如何处理请求"},
	} {
		if i > 0 {
			om.AddSeparator()
		}
		id := md.id
		om.AddRadio(subtitled(tr(md.en, md.cn), tr(md.subEN, md.subCN)), st.Mode == id).OnClick(m.run("mode", func() error { return b.SetMode(id) }))
	}

	// proxy groups
	m.nodes, m.tests = map[string][]nodeRow{}, map[string]*application.MenuItem{}
	if running && len(groups) > 0 {
		menu.AddSeparator()
		ps := &ProxyService{h: m.h}
		for _, g := range groups {
			if g.Hidden || (g.Name == "GLOBAL" && st.Mode != "global") {
				continue
			}
			sub := menu.AddSubmenu(detail(g.Name, g.Now))
			// first, and the menu stays up to show the delays coming in
			m.tests[g.Name] = sub.Add(testLabel(false)).SetTooltip(tr("⌥-click a node to test it alone", "按住 ⌥ 点击节点单独测速")).
				OnClick(m.run("test group", func() error { _, err := ps.TestLatency("group", g.Name); return err }))
			sub.AddSeparator()
			selectable := g.Type == "Selector"
			for _, mem := range g.Members {
				name := mem.Name
				group := g.Name
				it := sub.AddCheckbox(detail(mem.Name, delayBadge(mem.Delay)), name == g.Now)
				m.nodes[g.Name] = append(m.nodes[g.Name], nodeRow{name, mem.Delay, it})
				// ⌥-click tests just this node, without switching to it
				it.OnClick(m.run("select", func() error {
					if optionHeld() {
						_, err := ps.TestLatency("node", name)
						return err
					}
					if !selectable {
						return nil
					}
					return ps.Select(group, name)
				}))
			}
		}
		menu.Add(stay(tr("Test All Latency", "全部测速"))).OnClick(m.run("test all", func() error {
			_, err := ps.TestLatency("all", "")
			return err
		}))
	}

	// how the network does, and who uses it
	m.qualityItem, m.clientItems = nil, nil
	if running {
		connections := func(*application.Context) { m.h.showMain("connections") }
		menu.AddSeparator()
		m.qualityItem = menu.Add(m.qualityLabel()).SetEnabled(false)
		menu.AddSeparator()
		menu.Add(tr("Top Clients", "活跃应用")).SetEnabled(false)
		for i := range topClients {
			it := menu.Add(clientLabel(clients, i)).SetEnabled(i < len(clients)).OnClick(connections)
			m.clientItems = append(m.clientItems, it)
		}
		menu.AddSeparator()
		keyed(menu, menu.Add(tr("Dashboard…", "仪表盘…")), "D", connections)
	}

	// switches
	menu.AddSeparator()
	keyed(menu, menu.AddCheckbox(tr("Set as System Proxy", "设置为系统代理"), st.SystemProxy), "S",
		m.run("system proxy", func() error { return b.SetSystemProxy(!st.SystemProxy) }))
	keyed(menu, menu.AddCheckbox(tr("Enhanced Mode (TUN)", "增强模式 (TUN)"), st.Tun), "E", func(*application.Context) {
		go func() {
			if err := b.SetTun(!st.Tun, helperPrompt()); err != nil {
				log.Println("tun:", err)
				application.InvokeAsync(func() { m.h.showMain("settings") })
			}
			m.refresh()
		}()
	})

	// profiles
	menu.AddSeparator()
	pm := menu.AddSubmenu(detail(tr("Profiles", "配置"), st.ProfileName))
	for _, p := range profiles.List() {
		id := p.ID
		pm.AddCheckbox(p.Name, p.ID == st.Profile).OnClick(m.run("use profile", func() error { return b.UseProfile(id) }))
	}
	pm.AddSeparator()
	pm.Add(tr("Update All Subscriptions", "更新全部订阅")).OnClick(m.run("update", func() error {
		if failed := (&ProfileService{m.h}).UpdateAll(); len(failed) > 0 {
			return fmt.Errorf("%s", strings.Join(failed, "; "))
		}
		return nil
	}))
	pm.Add(tr("Manage Profiles…", "管理配置…")).OnClick(func(*application.Context) { m.h.showMain("profiles") })
	// ⌥-click copies it for the LAN address, for another machine to use
	menu.Add(tr("Copy Shell Export Command", "复制终端代理命令")).OnClick(func(*application.Context) {
		host := "127.0.0.1"
		if optionHeld() {
			if ip := lanIP(); ip != "" {
				host = ip
			}
		}
		m.h.app.Clipboard.SetText(proxyCommand(host))
	})

	// core
	menu.AddSeparator()
	if running {
		menu.Add(tr("Restart Core", "重启内核")).OnClick(m.run("restart", b.Restart))
		menu.Add(tr("Stop Core", "停止内核")).OnClick(m.run("stop", b.Stop))
	} else {
		menu.Add(tr("Start Core", "启动内核")).OnClick(m.run("start", b.Start))
	}

	menu.AddSeparator()
	keyed(menu, menu.Add(tr("Quit MihomoBar", "退出 MihomoBar")), "Q", func(*application.Context) { m.h.app.Quit() })

	m.h.tray.SetMenu(menu)
	styleTrayMenu()
}
