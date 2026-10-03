package gui

import (
	"errors"
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
// (docs/design.md §10.2): status, outbound mode, a submenu per proxy
// group, the two switches, profiles, then the core and the app. It is built
// afresh from the backend each time something it shows changes.
type trayMenu struct {
	h  *host
	mu sync.Mutex

	last    backend.State
	groups  []Group
	pending bool // a rebuild is scheduled
}

func newTrayMenu(h *host) *trayMenu {
	m := &trayMenu{h: h, last: h.b.State()}
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

func (m *trayMenu) relabel() { m.rebuild() }

// update takes a new state; groups are reloaded when the core is up.
func (m *trayMenu) update(st backend.State) {
	m.mu.Lock()
	m.last = st
	m.mu.Unlock()
	m.refresh()
}

// refresh reloads the groups and rebuilds, at most every 300ms.
func (m *trayMenu) refresh() {
	m.mu.Lock()
	if m.pending {
		m.mu.Unlock()
		return
	}
	m.pending = true
	m.mu.Unlock()
	time.AfterFunc(300*time.Millisecond, func() {
		var groups []Group
		if m.last.Core == "running" {
			groups, _ = (&ProxyService{h: m.h}).Groups()
		}
		m.mu.Lock()
		m.groups = groups
		m.pending = false
		m.mu.Unlock()
		application.InvokeAsync(m.rebuild)
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
		return tr("timeout", "超时")
	}
	return ""
}

// rebuild makes the menu from what is known; on the main thread.
func (m *trayMenu) rebuild() {
	if m.h.tray == nil {
		return
	}
	m.mu.Lock()
	st, groups := m.last, m.groups
	m.mu.Unlock()
	b := m.h.b
	running := st.Core == "running"
	menu := m.h.app.NewMenu()

	// status
	var status string
	switch st.Core {
	case "running":
		status = tr("Running", "运行中") + " · " + st.ProfileName
	case "starting":
		status = tr("Starting…", "正在启动…")
	case "crashed":
		status = tr("Core stopped with an error", "内核异常退出")
	default:
		status = tr("Stopped", "已停止")
	}
	menu.Add(status).SetEnabled(false)
	if running && !st.SystemProxy && !st.Tun {
		menu.Add(tr("Not taking over traffic: turn on System Proxy or TUN", "未接管流量：请开启系统代理或 TUN")).SetEnabled(false)
	}
	menu.AddSeparator()

	// outbound mode
	for _, md := range []struct{ id, en, cn string }{
		{"direct", "Direct Outbound", "直接连接"},
		{"global", "Global Proxy", "全局代理"},
		{"rule", "Rule-Based Proxy", "规则判定"},
	} {
		id := md.id
		menu.AddRadio(tr(md.en, md.cn), st.Mode == id).OnClick(m.run("mode", func() error { return b.SetMode(id) }))
	}

	// proxy groups
	if running && len(groups) > 0 {
		menu.AddSeparator()
		ps := &ProxyService{h: m.h}
		var shown []Group
		for _, g := range groups {
			if g.Hidden || (g.Name == "GLOBAL" && st.Mode != "global") {
				continue
			}
			shown = append(shown, g)
			label := g.Name
			if g.Now != "" {
				label += "    " + g.Now
			}
			sub := menu.AddSubmenu(label)
			selectable := g.Type == "Selector"
			for _, mem := range g.Members {
				name := mem.Name
				item := mem.Name
				if d := delayText(mem.Delay); d != "" {
					item += "    " + d
				}
				group := g.Name
				it := sub.AddCheckbox(item, name == g.Now)
				// ⌥-click tests just this node, without switching to it
				it.OnClick(m.run("select", func() error {
					if optionHeld() {
						_, err := ps.Delay(name, g.TestURL)
						return err
					}
					if !selectable {
						return nil
					}
					return ps.Select(group, name)
				}))
			}
			sub.AddSeparator()
			group := g.Name
			sub.Add(tr("Test Latency    (⌥-click a node to test it alone)", "测速    （⌥ 点击节点单独测速）")).OnClick(m.run("test", func() error {
				_, err := ps.GroupDelay(group, g.TestURL)
				return err
			}))
		}
		// every group at once, so one needn't open each to test it
		menu.Add(tr("Test All Latency", "全部测速")).OnClick(m.run("test all", func() error {
			errs := make([]error, len(shown))
			var wg sync.WaitGroup
			for i, g := range shown {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, errs[i] = ps.GroupDelay(g.Name, g.TestURL)
				}()
			}
			wg.Wait()
			return errors.Join(errs...)
		}))
	}

	// switches
	menu.AddSeparator()
	menu.AddCheckbox(tr("Set as System Proxy", "设置为系统代理"), st.SystemProxy).
		OnClick(m.run("system proxy", func() error { return b.SetSystemProxy(!st.SystemProxy) }))
	tun := menu.AddCheckbox(tr("Enhanced Mode (TUN)", "增强模式 (TUN)"), st.Tun)
	tun.OnClick(func(*application.Context) {
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
	pm := menu.AddSubmenu(tr("Profiles", "配置") + "    " + st.ProfileName)
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
	menu.Add(tr("Open Dashboard…", "打开主界面…")).OnClick(func(*application.Context) { m.h.showMain("") })
	menu.Add(tr("Quit MihomoBar", "退出 MihomoBar")).OnClick(func(*application.Context) { m.h.app.Quit() })

	m.h.tray.SetMenu(menu)
}
