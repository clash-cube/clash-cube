package gui

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/backend"
	"github.com/localhost-copilot/mihomobar/internal/hotkeys"
	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/modules"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/usage"
	"github.com/localhost-copilot/mihomobar/internal/userrules"
	"github.com/localhost-copilot/mihomobar/internal/wifi"
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

// DNSEgress is the resolver in use and the address its queries leave from.
func (s *AppService) DNSEgress() (backend.DNSEgress, error) { return s.h.b.DNSEgress() }

// ProxyEgress is where proxied traffic leaves, and the chain it took.
func (s *AppService) ProxyEgress() (backend.ProxyEgress, error) { return s.h.b.ProxyEgress() }

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
// machine to use (Allow LAN must be on); "" when the Mac has none.
func (s *AppService) LANProxyCommand() string {
	if ip := lanIP(); ip != "" {
		return proxyCommand(ip)
	}
	return ""
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
// RunningApps is the apps running now, for a process rule to name.
func (s *AppService) RunningApps() []App { return runningApps() }

// ChooseApp asks for an app or an executable; a cancelled dialog is the
// zero App.
func (s *AppService) ChooseApp() (App, error) {
	path, err := s.h.app.Dialog.OpenFile().
		SetTitle("Choose an app").
		SetDirectory("/Applications").
		CanChooseFiles(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return App{}, err
	}
	a, ok := appAt(path)
	if !ok {
		return App{}, errors.New("not an app or an executable: " + path)
	}
	return a, nil
}

// Events is the recent events, oldest first.
func (s *AppService) Events() []backend.Event { return s.h.b.Events() }
func (s *AppService) ClearEvents()            { s.h.b.ClearEvents() }

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
	Module  string   `json:"module,omitempty"` // the ID of the module that made it
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
	h *host

	meterMu sync.Mutex
	meter   backend.ClientMeter // for TopClients
	meterAt time.Time
}

// providerProxies is every provider's proxies by name, which /proxies
// leaves out.
func (s *ProxyService) providerProxies(ctx context.Context, c *mihomoapi.Client) (map[string]mihomoapi.ProxyProvider, map[string]mihomoapi.Proxy, error) {
	pvs, err := c.ProxyProviders(ctx)
	if err != nil {
		return nil, nil, err
	}
	byName := map[string]mihomoapi.Proxy{}
	for _, p := range pvs {
		// the core's own, for groups' proxies: those /proxies has
		if p.VehicleType == "Compatible" {
			continue
		}
		for _, m := range p.Proxies {
			byName[m.Name] = m
		}
	}
	return pvs, byName, nil
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
	// a group that uses a provider lists its proxies by name only
	_, fromProviders, _ := s.providerProxies(context.Background(), c)
	for name, p := range fromProviders {
		if _, ok := all[name]; !ok {
			all[name] = p
		}
	}
	var order []string
	if g, ok := all["GLOBAL"]; ok {
		order = append(order, g.All...)
	}
	order = append(order, "GLOBAL")
	made := map[string]string{}
	for _, m := range modules.List() {
		if m.Enabled && m.Route != nil {
			if g := m.Route.GroupIn(func(n string) bool { _, ok := all[n]; return ok }); g != "" {
				made[g] = m.ID
			}
		}
	}
	var out []Group
	for _, name := range order {
		p, ok := all[name]
		if !ok || len(p.All) == 0 {
			continue
		}
		g := Group{Name: p.Name, Type: p.Type, Now: p.Now, Hidden: p.Hidden, Icon: p.Icon, TestURL: p.TestURL, Module: made[p.Name]}
		for _, m := range p.All {
			mp := all[m]
			selected := all[backend.SelectedProxy(all, m)]
			g.Members = append(g.Members, Member{Name: m, Type: mp.Type, UDP: mp.UDP, Delay: lastDelay(selected), Group: len(mp.All) > 0})
		}
		out = append(out, g)
	}
	return out, nil
}

// Provider is a proxy provider with its nodes, as the profile lists them.
type Provider struct {
	Name        string    `json:"name"`
	VehicleType string    `json:"vehicleType"` // HTTP | File | Inline
	TestURL     string    `json:"testUrl"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Total       int64     `json:"total"`
	Expire      int64     `json:"expire"`
	Members     []Member  `json:"members"`
}

// Providers is the profile's proxy providers, by name; the ones the core
// makes for groups' own proxies are left out.
func (s *ProxyService) Providers() ([]Provider, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	all, _, err := s.providerProxies(context.Background(), c)
	if err != nil {
		return nil, err
	}
	out := []Provider{}
	for _, p := range all {
		if p.VehicleType == "Compatible" || p.Name == "default" {
			continue
		}
		pv := Provider{Name: p.Name, VehicleType: p.VehicleType, TestURL: p.TestURL, UpdatedAt: p.UpdatedAt, Members: []Member{}}
		if si := p.SubscriptionInfo; si != nil {
			pv.Upload, pv.Download, pv.Total, pv.Expire = si.Upload, si.Download, si.Total, si.Expire
		}
		for _, m := range p.Proxies {
			pv.Members = append(pv.Members, Member{Name: m.Name, Type: m.Type, UDP: m.UDP, Delay: lastDelay(m)})
		}
		out = append(out, pv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UpdateProvider fetches a proxy provider again.
func (s *ProxyService) UpdateProvider(name string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.UpdateProxyProvider(context.Background(), name)
}

// Select picks a group's member; the old one's connections are closed so
// the switch shows at once.
func (s *ProxyService) Select(group, name string) error { return s.h.b.SelectProxy(group, name) }

// TestLatency shares scheduling, progress and results across all surfaces.
func (s *ProxyService) TestLatency(kind, name string) (backend.LatencyResult, error) {
	return s.h.b.TestLatency(kind, name, func(e backend.LatencyEvent) {
		(sink{h: s.h}).emit("proxy-latency", e)
		if s.h.menu != nil {
			s.h.menu.latencyChanged(e)
		}
	})
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

func (s *ProxyService) FlushDNS() error { return s.h.b.FlushDNS() }

// UpdateGeo waits for the core to download its GEO databases; Updating in
// the result means one was already under way.
func (s *ProxyService) UpdateGeo() (backend.GeoInfo, error) { return s.h.b.UpdateGeo() }

// GeoInfo is when the GEO databases last changed, and whether an update runs.
func (s *ProxyService) GeoInfo() backend.GeoInfo { return s.h.b.GeoInfo() }

// LookupHost tells how the core resolves and routes host (a name, an
// address, host:port or a URL).
func (s *ProxyService) LookupHost(host string) (backend.Lookup, error) { return s.h.b.LookupHost(host) }

// RuleProviders is the profile's rule providers, by name.
func (s *ProxyService) RuleProviders() ([]mihomoapi.RuleProvider, error) {
	c, err := s.client()
	if err != nil {
		return []mihomoapi.RuleProvider{}, err
	}
	all, err := c.RuleProviders(context.Background())
	if err != nil {
		return []mihomoapi.RuleProvider{}, err
	}
	out := make([]mihomoapi.RuleProvider, 0, len(all))
	for _, p := range all {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UpdateRuleProvider fetches a rule provider again; the core keeps the
// rules it had when the fetch fails.
func (s *ProxyService) UpdateRuleProvider(name string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return c.UpdateRuleProvider(ctx, name)
}

// UserRules is the rules added in the app, ahead of the profile's.
func (s *ProxyService) UserRules() []userrules.Rule { return userrules.List() }

// RuleTypes is the rule types a user rule can have.
func (s *ProxyService) RuleTypes() []string { return userrules.Types }

// SetUserRules replaces them; the core takes them at once.
func (s *ProxyService) SetUserRules(rs []userrules.Rule) error { return s.h.b.SetRules(rs) }

// AddUserRule puts a rule first.
func (s *ProxyService) AddUserRule(r userrules.Rule) error { return s.h.b.AddRule(r) }

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

// Duplicate copies a profile as a local one the user can edit; name is
// the copy's.
func (s *ProfileService) Duplicate(id, name string) (profiles.Profile, error) {
	return s.h.b.DuplicateProfile(id, name)
}

// ModuleTemplates is the ready-made modules.
func (s *ProfileService) ModuleTemplates() []modules.Template { return modules.Templates }

// RouteServices is the services a module can send through a policy.
func (s *ProfileService) RouteServices() []modules.Service { return modules.Services }

// RouteRegions is the regions a route's group can take nodes by, each with
// how many of the running profile's nodes it takes.
func (s *ProfileService) RouteRegions() []backend.RegionNodes { return s.h.b.RouteRegions() }

// RouteNodes is the running profile's nodes, for a route to pick from.
func (s *ProfileService) RouteNodes() []backend.Node { return s.h.b.Nodes() }

// RouteBody is the YAML a route makes, to open as a module of its own.
func (s *ProfileService) RouteBody(r modules.Route) (string, error) {
	return r.Body(settings.Load().Profile, nil)
}

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

// SetNetworkAuto turns the network rules on or off.
func (s *SettingsService) SetNetworkAuto(on bool) (settings.Settings, error) {
	return s.h.b.SetNetworkAuto(on)
}

// SetNetworkRules replaces the network rules; the one in effect is applied.
func (s *SettingsService) SetNetworkRules(rules []settings.NetworkRule) (settings.Settings, error) {
	return s.h.b.SetNetworkRules(rules)
}

// ResumeNetworkAuto drops the changes made by hand and applies the rule again.
func (s *SettingsService) ResumeNetworkAuto() { s.h.b.ResumeNetworkAuto() }

func (s *SettingsService) RequestWiFiPermission() { wifi.RequestPermission() }

func (s *SettingsService) SavedWiFiNetworks() ([]string, error) { return wifi.SavedNetworks() }

// Patch is a partial update: the keys of p (as settings.json names them)
// that are present are changed.
type Patch struct {
	MixedPort       *int      `json:"mixedPort,omitempty"`
	AllowLan        *bool     `json:"allowLan,omitempty"`
	IPv6            *bool     `json:"ipv6,omitempty"`
	LogLevel        *string   `json:"logLevel,omitempty"`
	TunStack        *string   `json:"tunStack,omitempty"`
	ICMPForwarding  *bool     `json:"icmpForwarding,omitempty"`
	GuardIPv6       *bool     `json:"guardIPv6,omitempty"`
	GuardDNS        *bool     `json:"guardDNS,omitempty"`
	BlockSTUN       *bool     `json:"blockSTUN,omitempty"`
	DNSRespectRules *bool     `json:"dnsRespectRules,omitempty"`
	Bypass          *[]string `json:"bypass,omitempty"`
	AutoStart       *bool     `json:"autoStart,omitempty"`
	LaunchAtLogin   *bool     `json:"launchAtLogin,omitempty"`
	Theme           *string   `json:"theme,omitempty"`
	Lang            *string   `json:"lang,omitempty"`
	Dock            *string   `json:"dock,omitempty"`
	TestURL         *string   `json:"testUrl,omitempty"`
	TraySpeed       *bool     `json:"traySpeed,omitempty"`
	FindProcess     *bool     `json:"findProcess,omitempty"`
	Notify          *bool     `json:"notify,omitempty"`
	SaveData        *bool     `json:"saveData,omitempty"`
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
		set(&st.ICMPForwarding, p.ICMPForwarding)
		set(&st.GuardIPv6, p.GuardIPv6)
		set(&st.GuardDNS, p.GuardDNS)
		set(&st.BlockSTUN, p.BlockSTUN)
		set(&st.DNSRespectRules, p.DNSRespectRules)
		set(&st.Bypass, p.Bypass)
		set(&st.AutoStart, p.AutoStart)
		set(&st.LaunchAtLogin, p.LaunchAtLogin)
		set(&st.Theme, p.Theme)
		set(&st.Lang, p.Lang)
		set(&st.Dock, p.Dock)
		set(&st.TestURL, p.TestURL)
		set(&st.TraySpeed, p.TraySpeed)
		set(&st.FindProcess, p.FindProcess)
		set(&st.Notify, p.Notify)
		set(&st.SaveData, p.SaveData)
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

// Usage is the traffic statistics of the days from..to ("2006-01-02",
// inclusive); a single day comes in hours, and hour (0–23, else -1)
// narrows it to one.
func (s *AppService) Usage(from, to string, hour int) (usage.Report, error) {
	return s.h.b.Usage().Query(from, to, hour)
}

// ClearUsage forgets the traffic statistics.
func (s *AppService) ClearUsage() error { return s.h.b.Usage().Clear() }

// Modules is the user's modules, merged over every profile in order.
func (s *ProfileService) Modules() []modules.Module {
	ms := modules.List()
	for i := range ms {
		ms[i].Body = modules.ReadableYAML(ms[i].Body)
	}
	return ms
}

// SetModules replaces them; a configuration the core refuses keeps the
// previous ones.
func (s *ProfileService) SetModules(ms []modules.Module) error { return s.h.b.SetModules(ms) }

// ModuleKeys is the top-level keys a module's body sets, for its summary;
// an error says why the body isn't a module.
func (s *ProfileService) ModuleKeys(body string) ([]string, error) {
	if _, err := modules.Parse(body); err != nil {
		return []string{}, err
	}
	return append([]string{}, modules.Module{Body: body}.Keys()...), nil
}

// Hotkey is an action's global shortcut, as Settings shows it.
type Hotkey struct {
	Action string `json:"action"`
	Keys   string `json:"keys"`            // as "Ctrl+Option+Cmd+P"; "" for none
	Error  string `json:"error,omitempty"` // why it isn't working
}

// Hotkeys is every action and its shortcut.
func (s *SettingsService) Hotkeys() []Hotkey {
	return s.h.hotkeys(s.h.keys.apply())
}

func (h *host) hotkeys(failed map[string]string) []Hotkey {
	set := settings.Load().Hotkeys
	out := make([]Hotkey, 0, len(hotkeys.Actions))
	for _, a := range hotkeys.Actions {
		hk := Hotkey{Action: a, Keys: set[a]}
		if failed[a] != "" {
			hk.Error = "Another app is using this shortcut"
		}
		out = append(out, hk)
	}
	return out
}

// SetHotkey gives an action a shortcut ("" clears it). One refused (taken
// by macOS or another action, or not a usable combination) leaves the
// settings as they were; the error is a hotkeys.Problem.
func (s *SettingsService) SetHotkey(action, keys string) ([]Hotkey, error) {
	norm := ""
	if keys != "" {
		var err error
		if norm, err = hotkeys.Check(action, keys, settings.Load().Hotkeys); err != nil {
			return s.h.hotkeys(nil), err
		}
	} else if !slices.Contains(hotkeys.Actions, action) {
		return s.h.hotkeys(nil), errors.New("unknown action " + action)
	}
	if _, err := settings.Update(func(st *settings.Settings) {
		if st.Hotkeys == nil {
			st.Hotkeys = map[string]string{}
		}
		if norm == "" {
			delete(st.Hotkeys, action)
		} else {
			st.Hotkeys[action] = norm
		}
	}); err != nil {
		return s.h.hotkeys(nil), err
	}
	failed := s.h.keys.apply()
	if failed[action] != "" {
		// another app holds it: don't keep a shortcut that does nothing
		_, _ = settings.Update(func(st *settings.Settings) { delete(st.Hotkeys, action) })
		return s.h.hotkeys(nil), hotkeys.Problem{Text: "Another app is using this shortcut"}
	}
	return s.h.hotkeys(failed), nil
}

// RecordHotkey lifts every shortcut while the page records one, so that
// pressing a set one is recorded rather than run; false puts them back.
func (s *SettingsService) RecordHotkey(on bool) { s.h.keys.pause(on) }

// Recommended is what UseRecommendedHotkeys did: the shortcuts now, and
// the actions it left without one, the shortcut being taken.
type Recommended struct {
	Hotkeys []Hotkey `json:"hotkeys"`
	Skipped []string `json:"skipped"`
}

// UseRecommendedHotkeys gives every action without a shortcut the
// recommended one; those already set are kept, and one macOS, another
// action or another app holds is skipped.
func (s *SettingsService) UseRecommendedHotkeys() (Recommended, error) {
	out := Recommended{Skipped: []string{}}
	for _, a := range hotkeys.Actions {
		if settings.Load().Hotkeys[a] != "" {
			continue
		}
		if _, err := s.SetHotkey(a, hotkeys.Recommended[a]); err != nil {
			out.Skipped = append(out.Skipped, a)
		}
	}
	out.Hotkeys = s.h.hotkeys(s.h.keys.apply())
	return out, nil
}
