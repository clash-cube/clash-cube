package gui

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/backend"
	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// AppService is the app and its windows.
type AppService struct{ h *host }

func (s *AppService) State() backend.State { return s.h.b.State() }
func (s *AppService) Start() error         { return s.h.b.Start() }
func (s *AppService) Stop() error          { return s.h.b.Stop() }
func (s *AppService) Restart() error       { return s.h.b.Restart() }
func (s *AppService) SetMode(mode string) error {
	return s.h.b.SetMode(mode)
}
func (s *AppService) SetSystemProxy(on bool) error { return s.h.b.SetSystemProxy(on) }
func (s *AppService) SetTun(on bool) error         { return s.h.b.SetTun(on, helperPrompt()) }

// Connectivity measures router, DNS, internet and proxy latency.
func (s *AppService) Connectivity() (backend.Connectivity, error) { return s.h.b.Connectivity() }

// ConnectivityItem measures one of "router", "dns", "internet", "proxy";
// only that item's fields are set.
func (s *AppService) ConnectivityItem(key string) (backend.Connectivity, error) {
	return s.h.b.ConnectivityItem(key)
}

// DirectEgress is the direct route's interface and public address.
func (s *AppService) DirectEgress() (backend.Egress, error) { return s.h.b.DirectEgress() }

// HelperStatus is service mode's state, for Settings.
func (s *AppService) HelperStatus() backend.HelperStatus { return s.h.b.HelperStatus() }

// EnableServiceMode installs the privileged helper (asking for a password).
func (s *AppService) EnableServiceMode() error { return s.h.b.EnableServiceMode(helperPrompt()) }

// DisableServiceMode runs the core as the user again; uninstall removes the helper.
func (s *AppService) DisableServiceMode(uninstall bool) error {
	return s.h.b.DisableServiceMode(uninstall, tr("MihomoBar wants to remove its privileged helper.", "MihomoBar 需要移除特权助手。"))
}

func helperPrompt() string {
	return tr("MihomoBar needs to install a privileged helper to run Enhanced Mode (TUN).", "MihomoBar 需要安装特权助手以启用增强模式 (TUN)。")
}

// ShowMain opens the main window, on view if one is given.
func (s *AppService) ShowMain(view string) { application.InvokeAsync(func() { s.h.showMain(view) }) }
func (s *AppService) HidePanel()           { application.InvokeAsync(func() { s.h.panel.Hide() }) }
func (s *AppService) Quit()                { application.InvokeAsync(s.h.app.Quit) }

// FitPanel sizes the tray panel to its page's height (CSS pixels), gliding
// over ms when it is shown.
func (s *AppService) FitPanel(height, ms int) {
	application.InvokeAsync(func() { s.h.fitPanel(height, ms) })
}

func (s *AppService) CopyText(text string) bool { return s.h.app.Clipboard.SetText(text) }

func (s *AppService) OpenURL(url string) error { return s.h.app.Browser.OpenURL(url) }

// ProxyCommand is the shell export line for the mixed port.
func (s *AppService) ProxyCommand() string { return proxyCommand("127.0.0.1") }

// LANProxyCommand is the same for this Mac's LAN address, for another
// machine to use (Allow LAN must be on).
func (s *AppService) LANProxyCommand() string {
	if ip := lanIP(); ip != "" {
		return proxyCommand(ip)
	}
	return proxyCommand("127.0.0.1")
}

func proxyCommand(host string) string {
	p := itoa(settings.Load().MixedPort)
	addr := "http://" + host + ":" + p
	return "export https_proxy=" + addr + " http_proxy=" + addr + " all_proxy=socks5://" + host + ":" + p
}

// lanIP is the address of the interface the default route leaves by.
func lanIP() string {
	c, err := net.Dial("udp", "192.0.2.1:9") // no packet is sent
	if err != nil {
		return ""
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	if ip.IsLoopback() || strings.HasPrefix(ip.String(), "198.18.") {
		return ""
	}
	return ip.String()
}

// AppIcon is an app's icon (a bundle or executable path; "" for a LAN
// client) as a PNG data URL, for the page to show.
func (s *AppService) AppIcon(path string) string { return appIcon(path) }

// RevealData opens the app's data folder in Finder.
func (s *AppService) RevealData() error { return exec.Command("open", appdir.Root()).Run() }

// Group is a proxy group with its members, in the profile's order.
type Group struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	Hidden  bool     `json:"hidden"`
	Icon    string   `json:"icon"`
	TestURL string   `json:"testUrl"`
	Members []Member `json:"members"`
}

type Member struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	UDP   bool   `json:"udp"`
	Delay int    `json:"delay"` // last test, ms; 0 untested, -1 failed
	Group bool   `json:"group"` // a group itself
}

// ProxyService is the core's proxies, groups, connections and rules.
type ProxyService struct {
	h       *host
	testing sync.Map // group → struct{}

	meterMu sync.Mutex
	meter   backend.ClientMeter // for TopClients
	meterAt time.Time
}

func (s *ProxyService) client() (*mihomoapi.Client, error) { return s.h.b.Client() }

func lastDelay(p mihomoapi.Proxy) int {
	if len(p.History) == 0 {
		return 0
	}
	d := p.History[len(p.History)-1].Delay
	if d == 0 {
		return -1
	}
	return d
}

// Groups is every proxy group, GLOBAL last.
func (s *ProxyService) Groups() ([]Group, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	all, err := c.Proxies(context.Background())
	if err != nil {
		return nil, err
	}
	var order []string
	if g, ok := all["GLOBAL"]; ok {
		order = append(order, g.All...)
	}
	order = append(order, "GLOBAL")
	var out []Group
	for _, name := range order {
		p, ok := all[name]
		if !ok || len(p.All) == 0 {
			continue
		}
		g := Group{Name: p.Name, Type: p.Type, Now: p.Now, Hidden: p.Hidden, Icon: p.Icon, TestURL: p.TestURL}
		for _, m := range p.All {
			mp := all[m]
			g.Members = append(g.Members, Member{Name: m, Type: mp.Type, UDP: mp.UDP, Delay: lastDelay(mp), Group: len(mp.All) > 0})
		}
		out = append(out, g)
	}
	return out, nil
}

func (s *ProxyService) Select(group, name string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	if err := c.Select(context.Background(), group, name); err != nil {
		return err
	}
	// the old proxy's connections are closed so the switch shows at once
	conns, err := c.Connections(context.Background())
	if err == nil {
		for _, cn := range conns.Connections {
			for _, ch := range cn.Chains {
				if ch == group {
					_ = c.CloseConnection(context.Background(), cn.ID)
					break
				}
			}
		}
	}
	return nil
}

func (s *ProxyService) testURL(u string) string {
	if u != "" {
		return u
	}
	return settings.Load().TestURL
}

// Delay tests one proxy: ms, or -1 when it failed.
func (s *ProxyService) Delay(name, testURL string) (int, error) {
	c, err := s.client()
	if err != nil {
		return 0, err
	}
	d, err := c.Delay(context.Background(), name, s.testURL(testURL), 5*time.Second)
	var ae *mihomoapi.APIError
	if errors.As(err, &ae) {
		return -1, nil
	}
	return d, err
}

// GroupDelay tests every member of a group: name → ms, -1 failed.
func (s *ProxyService) GroupDelay(group, testURL string) (map[string]int, error) {
	if _, busy := s.testing.LoadOrStore(group, struct{}{}); busy {
		return nil, errors.New("already testing")
	}
	defer s.testing.Delete(group)
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	res, err := c.GroupDelay(context.Background(), group, s.testURL(testURL), 5*time.Second)
	var ae *mihomoapi.APIError
	if err != nil && !errors.As(err, &ae) {
		return nil, err
	}
	// members that failed are absent from the answer
	all, _ := c.Proxies(context.Background())
	out := map[string]int{}
	for _, m := range all[group].All {
		if d, ok := res[m]; ok && d > 0 {
			out[m] = d
		} else {
			out[m] = -1
		}
	}
	return out, nil
}

func (s *ProxyService) Connections() (mihomoapi.Connections, error) {
	c, err := s.client()
	if err != nil {
		return mihomoapi.Connections{Connections: []mihomoapi.Connection{}}, err
	}
	conns, err := c.Connections(context.Background())
	sort.Slice(conns.Connections, func(i, j int) bool { return conns.Connections[i].Start.After(conns.Connections[j].Start) })
	return conns, err
}

// TopClients is the n apps moving the most traffic, their speeds measured
// since the last call. After a pause (the panel was hidden) it starts over,
// and the first call has no speeds yet.
func (s *ProxyService) TopClients(n int) ([]backend.ClientRate, error) {
	c, err := s.client()
	if err != nil {
		return []backend.ClientRate{}, err
	}
	conns, err := c.Connections(context.Background())
	if err != nil {
		return []backend.ClientRate{}, err
	}
	s.meterMu.Lock()
	now := time.Now()
	if now.Sub(s.meterAt) > 5*time.Second {
		s.meter = backend.ClientMeter{}
	}
	s.meterAt = now
	rates := s.meter.Sample(now, conns.Connections)
	s.meterMu.Unlock()
	return rates[:min(n, len(rates))], nil
}

func (s *ProxyService) CloseConnection(id string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.CloseConnection(context.Background(), id)
}

func (s *ProxyService) CloseAllConnections() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.CloseAllConnections(context.Background())
}

func (s *ProxyService) Rules() ([]mihomoapi.Rule, error) {
	c, err := s.client()
	if err != nil {
		return []mihomoapi.Rule{}, err
	}
	return c.Rules(context.Background())
}

func (s *ProxyService) FlushDNS() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	_ = c.FlushFakeIP(context.Background())
	return c.FlushDNS(context.Background())
}

func (s *ProxyService) UpdateGeo() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.UpdateGeo(context.Background())
}

// ProfileService is the profiles.
type ProfileService struct{ h *host }

func (s *ProfileService) List() []profiles.Profile { return profiles.List() }

func (s *ProfileService) ImportURL(url, name string, interval int) (profiles.Profile, error) {
	p, err := profiles.ImportURL(url, name, interval)
	if err == nil {
		s.h.b.ProfileChanged("")
	}
	return p, err
}

// ImportFile asks for a YAML file and imports it; an empty profile when the
// user cancelled.
func (s *ProfileService) ImportFile() (profiles.Profile, error) {
	path, err := s.h.app.Dialog.OpenFile().
		SetTitle("Import configuration").
		AddFilter("YAML", "*.yaml;*.yml").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return profiles.Profile{}, err
	}
	p, err := profiles.ImportFile(path, "")
	if err == nil {
		s.h.b.ProfileChanged("")
	}
	return p, err
}

func (s *ProfileService) Update(id string) (profiles.Profile, error) {
	p, err := profiles.Update(id)
	if err == nil {
		s.h.b.ProfileChanged(id)
	}
	return p, err
}

// UpdateAll refreshes every subscription; the names of those that failed.
func (s *ProfileService) UpdateAll() []string {
	var failed []string
	for _, p := range profiles.List() {
		if p.URL == "" {
			continue
		}
		if _, err := profiles.Update(p.ID); err != nil {
			failed = append(failed, p.Name+": "+err.Error())
			continue
		}
		s.h.b.ProfileChanged(p.ID)
	}
	return failed
}

func (s *ProfileService) Edit(id, name string, interval int) (profiles.Profile, error) {
	p, err := profiles.Edit(id, name, interval)
	if err == nil {
		s.h.b.ProfileChanged("")
	}
	return p, err
}

func (s *ProfileService) Remove(id string) error { return s.h.b.RemoveProfile(id) }
func (s *ProfileService) Use(id string) error    { return s.h.b.UseProfile(id) }

// Reveal shows a profile's file in Finder.
func (s *ProfileService) Reveal(id string) error {
	p, ok := profiles.Get(id)
	if !ok {
		return errors.New("no such profile")
	}
	return exec.Command("open", "-R", p.Path()).Run()
}

// Edit opens a profile's file in the default editor.
func (s *ProfileService) OpenInEditor(id string) error {
	p, ok := profiles.Get(id)
	if !ok {
		return errors.New("no such profile")
	}
	if _, err := os.Stat(p.Path()); err != nil {
		return err
	}
	return exec.Command("open", "-t", p.Path()).Run()
}

// SettingsService is the app's settings.
type SettingsService struct{ h *host }

func (s *SettingsService) Get() settings.Settings { return settings.Load() }

// Patch is a partial update: the keys of p (as settings.json names them)
// that are present are changed.
type Patch struct {
	MixedPort     *int      `json:"mixedPort,omitempty"`
	AllowLan      *bool     `json:"allowLan,omitempty"`
	IPv6          *bool     `json:"ipv6,omitempty"`
	LogLevel      *string   `json:"logLevel,omitempty"`
	TunStack      *string   `json:"tunStack,omitempty"`
	Bypass        *[]string `json:"bypass,omitempty"`
	AutoStart     *bool     `json:"autoStart,omitempty"`
	LaunchAtLogin *bool     `json:"launchAtLogin,omitempty"`
	Theme         *string   `json:"theme,omitempty"`
	Lang          *string   `json:"lang,omitempty"`
	Dock          *string   `json:"dock,omitempty"`
	TestURL       *string   `json:"testUrl,omitempty"`
	TraySpeed     *bool     `json:"traySpeed,omitempty"`
	FindProcess   *bool     `json:"findProcess,omitempty"`
}

func (s *SettingsService) Patch(p Patch) (settings.Settings, error) {
	if p.MixedPort != nil && (*p.MixedPort < 1 || *p.MixedPort > 65535) {
		return settings.Load(), errors.New("port must be 1–65535")
	}
	if p.Bypass != nil {
		var clean []string
		for _, b := range *p.Bypass {
			if b = strings.TrimSpace(b); b != "" {
				clean = append(clean, b)
			}
		}
		p.Bypass = &clean
	}
	out, err := s.h.b.PatchSettings(func(st *settings.Settings) {
		set(&st.MixedPort, p.MixedPort)
		set(&st.AllowLan, p.AllowLan)
		set(&st.IPv6, p.IPv6)
		set(&st.LogLevel, p.LogLevel)
		set(&st.TunStack, p.TunStack)
		set(&st.Bypass, p.Bypass)
		set(&st.AutoStart, p.AutoStart)
		set(&st.LaunchAtLogin, p.LaunchAtLogin)
		set(&st.Theme, p.Theme)
		set(&st.Lang, p.Lang)
		set(&st.Dock, p.Dock)
		set(&st.TestURL, p.TestURL)
		set(&st.TraySpeed, p.TraySpeed)
		set(&st.FindProcess, p.FindProcess)
	})
	if p.Dock != nil {
		application.InvokeAsync(func() { s.h.dock(s.h.main.IsVisible()) })
	}
	if p.Lang != nil {
		application.InvokeAsync(s.h.relabelTray)
	}
	if p.LaunchAtLogin != nil {
		if lerr := setLaunchAtLogin(*p.LaunchAtLogin); lerr != nil && err == nil {
			err = lerr
		}
	}
	return out, err
}

func set[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
