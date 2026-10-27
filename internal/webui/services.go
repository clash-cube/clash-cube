package webui

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/router"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/usage"
	"github.com/localhost-copilot/clashcube/internal/userrules"
)

type AppService struct{ b *backend.Backend }
type ProfileService struct{ b *backend.Backend }
type ProxyService struct {
	b       *backend.Backend
	meterMu sync.Mutex
	meter   backend.ClientMeter
	meterAt time.Time
}

func (s *ProxyService) client() (*mihomoapi.Client, error) { return s.b.Client() }

func (s *AppService) State() backend.State { return s.b.State() }

func (s *AppService) Start() error { return s.b.Start() }

func (s *AppService) Stop() error { return s.b.Stop() }

func (s *AppService) Restart() error { return s.b.Restart() }

func (s *AppService) Reload() error { return s.b.Reload() }

func (s *AppService) SetMode(mode string) error {
	return s.b.SetMode(mode)
}

func (s *AppService) TailscaleLogin(name string) error { return s.b.TailscaleLogin(name) }

func (s *AppService) TailscaleLogout(name string) error { return s.b.TailscaleLogout(name) }

func (s *AppService) TailscalePing(name, ip string) (mihomoapi.TailscalePing, error) {
	return s.b.TailscalePing(name, ip)
}

func (s *AppService) TailscaleLocalNetworks() []string { return s.b.TailscaleLocalNetworks() }

func (s *AppService) TailscaleHostname() string { return s.b.TailscaleHostname() }

func (s *AppService) Connectivity() (backend.Connectivity, error) { return s.b.Connectivity() }

func (s *AppService) ConnectivityItem(key string) (backend.Connectivity, error) {
	return s.b.ConnectivityItem(key)
}

func (s *AppService) ConnectivityHistory() map[string][]backend.LatencySample {
	return s.b.ConnectivityHistory()
}

func (s *AppService) DirectEgress() (backend.Egress, error) { return s.b.DirectEgress() }

func (s *AppService) DNSEgress() (backend.DNSEgress, error) { return s.b.DNSEgress() }

func (s *AppService) ProxyEgress() (backend.ProxyEgress, error) { return s.b.ProxyEgress() }

func (s *AppService) AICheck(service string, force bool) (backend.AICheck, error) {
	return s.b.AICheck(service, force)
}

func (s *AppService) Events() []backend.Event { return s.b.Events() }

func (s *AppService) ClearEvents() { s.b.ClearEvents() }

func (s *AppService) Usage(from, to string, hour int) (usage.Report, error) {
	return s.b.Usage().Query(from, to, hour)
}

func (s *AppService) ClearUsage() error { return s.b.Usage().Clear() }

func (s *ProxyService) Connections() (mihomoapi.Connections, error) {
	c, err := s.client()
	if err != nil {
		return mihomoapi.Connections{Connections: []mihomoapi.Connection{}}, err
	}
	conns, err := c.Connections(context.Background())
	sort.Slice(conns.Connections, func(i, j int) bool { return conns.Connections[i].Start.After(conns.Connections[j].Start) })
	return conns, err
}

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
	return rates[:min(max(0, n), len(rates))], nil
}

func (s *ProxyService) Globe() (backend.Globe, error) { return s.b.Globe() }

func (s *ProxyService) UpdateProvider(name string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.UpdateProxyProvider(context.Background(), name)
}

func (s *ProxyService) Select(group, name string) error { return s.b.SelectProxy(group, name) }

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

func (s *ProxyService) FlushDNS() error { return s.b.FlushDNS() }

func (s *ProxyService) UpdateGeo() (backend.GeoInfo, error) { return s.b.UpdateGeo() }

func (s *ProxyService) GeoInfo() backend.GeoInfo { return s.b.GeoInfo() }

func (s *ProxyService) CityGeoInfo() backend.CityGeoInfo { return s.b.CityGeoInfo() }

func (s *ProxyService) SetCityGeo(on bool) (backend.CityGeoInfo, error) { return s.b.SetCityGeo(on) }

func (s *ProxyService) UpdateCityGeo() backend.CityGeoInfo { return s.b.UpdateCityGeo() }

func (s *ProxyService) LookupHost(host string) (backend.Lookup, error) { return s.b.LookupHost(host) }

func (s *ProxyService) RuleEntries(kind, name, query string, limit int) (backend.RuleEntries, error) {
	return s.b.RuleEntries(kind, name, query, limit)
}

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

func (s *ProxyService) UpdateRuleProvider(name string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return c.UpdateRuleProvider(ctx, name)
}

func (s *ProxyService) UserRules() []userrules.Rule { return userrules.List() }

func (s *ProxyService) RuleTypes() []string { return userrules.Types }

func (s *ProxyService) SetUserRules(rs []userrules.Rule) error { return s.b.SetRules(rs) }

func (s *ProxyService) AddUserRule(r userrules.Rule) error { return s.b.AddRule(r) }

func (s *ProxyService) TempRules() []userrules.Rule { return s.b.TempRules() }

func (s *ProxyService) SetTempRules(rs []userrules.Rule) error { return s.b.SetTempRules(rs) }

func (s *ProxyService) AddTempRule(r userrules.Rule) error { return s.b.AddTempRule(r) }

func (s *ProfileService) List() []profiles.Profile { return profiles.List() }

func (s *ProfileService) ImportURL(url, name string, interval int) (profiles.Profile, error) {
	p, err := profiles.ImportURL(url, name, interval)
	if err == nil {
		s.b.ProfileChanged("")
	}
	return p, err
}

func (s *ProfileService) Update(id string) (profiles.Profile, error) {
	p, err := profiles.Update(id)
	if err == nil {
		s.b.ProfileChanged(id)
	}
	return p, err
}

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
		s.b.ProfileChanged(p.ID)
	}
	return failed
}

func (s *ProfileService) Edit(id, name string, interval int) (profiles.Profile, error) {
	p, err := profiles.Edit(id, name, interval)
	if err == nil {
		s.b.ProfileChanged("")
	}
	return p, err
}

func (s *ProfileService) Remove(id string) error { return s.b.RemoveProfile(id) }

func (s *ProfileService) Use(id string) error { return s.b.UseProfile(id) }

func (s *ProfileService) Duplicate(id, name string) (profiles.Profile, error) {
	return s.b.DuplicateProfile(id, name)
}

func (s *ProfileService) ModuleTemplates() []modules.Template { return modules.Templates }

func (s *ProfileService) RouteServices() []modules.Service { return modules.Services }

func (s *ProfileService) RouteRegions() []backend.RegionNodes { return s.b.RouteRegions() }

func (s *ProfileService) RouteNodes() []backend.Node { return s.b.Nodes() }

func (s *ProfileService) ModuleBody(m modules.Module) (string, error) {
	return s.b.ModuleBody(m)
}

func (s *ProfileService) FreePort(from int) int { return s.b.FreePort(from) }

func (s *ProfileService) RuntimeConfig() (backend.RuntimeView, error) {
	v, err := s.b.RuntimeConfig()
	v.Body = redactController(v.Body)
	v.Refused = redactController(v.Refused)
	for i := range v.Lines {
		v.Lines[i].Text = redactController(v.Lines[i].Text)
	}
	return v, err
}

func (s *ProfileService) DismissRefusal() { s.b.DismissRefusal() }

func (s *ProfileService) Modules() []modules.Module {
	ms := modules.List()
	for i := range ms {
		ms[i].Body = modules.ReadableYAML(ms[i].Body)
	}
	return ms
}

func (s *ProfileService) SetModules(ms []modules.Module) error { return s.b.SetModules(ms) }

func (s *ProfileService) ModuleKeys(body string) ([]string, error) {
	if _, err := modules.Parse(body); err != nil {
		return []string{}, err
	}
	return append([]string{}, modules.Module{Body: body}.Keys()...), nil
}

// NewHandler publishes an explicit set of business operations. Native desktop
// privileges, arbitrary filesystem access and the core's credentials stay private.
func NewHandler(b *backend.Backend, events *Broker, routers ...*router.Manager) http.Handler {
	app := &AppService{b: b}
	proxy := &ProxyService{b: b}
	profile := &ProfileService{b: b}
	methods := map[string]any{
		"App/State":                  app.State,
		"App/Start":                  app.Start,
		"App/Stop":                   app.Stop,
		"App/Restart":                app.Restart,
		"App/Reload":                 app.Reload,
		"App/SetMode":                app.SetMode,
		"App/TailscaleLogin":         app.TailscaleLogin,
		"App/TailscaleLogout":        app.TailscaleLogout,
		"App/TailscalePing":          app.TailscalePing,
		"App/TailscaleLocalNetworks": app.TailscaleLocalNetworks,
		"App/TailscaleHostname":      app.TailscaleHostname,
		"App/Connectivity":           app.Connectivity,
		"App/ConnectivityItem":       app.ConnectivityItem,
		"App/ConnectivityHistory":    app.ConnectivityHistory,
		"App/DirectEgress":           app.DirectEgress,
		"App/DNSEgress":              app.DNSEgress,
		"App/ProxyEgress":            app.ProxyEgress,
		"App/AICheck":                app.AICheck,
		"App/Events":                 app.Events,
		"App/ClearEvents":            app.ClearEvents,
		"App/Usage":                  app.Usage,
		"App/ClearUsage":             app.ClearUsage,
		"Proxy/Connections":          proxy.Connections,
		"Proxy/TopClients":           proxy.TopClients,
		"Proxy/Globe":                proxy.Globe,
		"Proxy/UpdateProvider":       proxy.UpdateProvider,
		"Proxy/Select":               proxy.Select,
		"Proxy/CloseConnection":      proxy.CloseConnection,
		"Proxy/CloseAllConnections":  proxy.CloseAllConnections,
		"Proxy/Rules":                proxy.Rules,
		"Proxy/FlushDNS":             proxy.FlushDNS,
		"Proxy/UpdateGeo":            proxy.UpdateGeo,
		"Proxy/GeoInfo":              proxy.GeoInfo,
		"Proxy/CityGeoInfo":          proxy.CityGeoInfo,
		"Proxy/SetCityGeo":           proxy.SetCityGeo,
		"Proxy/UpdateCityGeo":        proxy.UpdateCityGeo,
		"Proxy/LookupHost":           proxy.LookupHost,
		"Proxy/RuleEntries":          proxy.RuleEntries,
		"Proxy/RuleProviders":        proxy.RuleProviders,
		"Proxy/UpdateRuleProvider":   proxy.UpdateRuleProvider,
		"Proxy/UserRules":            proxy.UserRules,
		"Proxy/RuleTypes":            proxy.RuleTypes,
		"Proxy/SetUserRules":         proxy.SetUserRules,
		"Proxy/AddUserRule":          proxy.AddUserRule,
		"Proxy/TempRules":            proxy.TempRules,
		"Proxy/SetTempRules":         proxy.SetTempRules,
		"Proxy/AddTempRule":          proxy.AddTempRule,
		"Profiles/List":              profile.List,
		"Profiles/ImportURL":         profile.ImportURL,
		"Profiles/Update":            profile.Update,
		"Profiles/UpdateAll":         profile.UpdateAll,
		"Profiles/Edit":              profile.Edit,
		"Profiles/Remove":            profile.Remove,
		"Profiles/Use":               profile.Use,
		"Profiles/Duplicate":         profile.Duplicate,
		"Profiles/ModuleTemplates":   profile.ModuleTemplates,
		"Profiles/RouteServices":     profile.RouteServices,
		"Profiles/RouteRegions":      profile.RouteRegions,
		"Profiles/RouteNodes":        profile.RouteNodes,
		"Profiles/ModuleBody":        profile.ModuleBody,
		"Profiles/FreePort":          profile.FreePort,
		"Profiles/RuntimeConfig":     profile.RuntimeConfig,
		"Profiles/DismissRefusal":    profile.DismissRefusal,
		"Profiles/Modules":           profile.Modules,
		"Profiles/SetModules":        profile.SetModules,
		"Profiles/ModuleKeys":        profile.ModuleKeys,
		"Proxy/Groups":               b.Groups,
		"Proxy/Providers":            b.Providers,
		"Proxy/TestLatency": func(kind, name string) (backend.LatencyResult, error) {
			return b.TestLatency(kind, name, func(e backend.LatencyEvent) { events.Publish("proxy-latency", e) })
		},
		"Profiles/ImportContent": func(name, body string) (profiles.Profile, error) {
			p, err := profiles.ImportContent(name, []byte(body))
			if err == nil {
				b.ProfileChanged("")
			}
			return p, err
		},
		"Profiles/Read":               func(id string) (string, error) { return readProfile(id) },
		"Profiles/TakeImportRequests": func() []profiles.ImportRequest { return []profiles.ImportRequest{} },
		"Settings/Get":                settings.Load,
		"Settings/DefaultUserAgent":   b.DefaultUserAgent,
		"Settings/Patch": func(p backend.Patch) (settings.Settings, error) {
			if p.LaunchAtLogin != nil || p.Dock != nil || p.TraySpeed != nil || p.Notify != nil || p.AutoUpdateApp != nil || p.Bypass != nil || p.SaveData != nil || p.TunStack != nil || p.ICMPForwarding != nil || p.GuardIPv6 != nil || p.GuardDNS != nil {
				return settings.Load(), errors.New("setting unavailable on OpenWrt")
			}
			return b.PatchFields(p)
		},
	}
	if len(routers) > 0 && routers[0] != nil {
		r := routers[0]
		methods["Router/Get"] = r.Get
		methods["Router/SetEnabled"] = r.SetEnabled
		methods["Router/SetAccess"] = r.SetAccess
		methods["Router/Diagnostics"] = r.Diagnostics
		methods["Router/Maintenance"] = r.Maintenance
		methods["Router/SetMaintenance"] = r.SetMaintenance
		methods["Router/Updates"] = r.Updates
		methods["Router/CheckUpdates"] = r.CheckUpdates
		methods["Router/UpdateCore"] = r.UpdateCore
		methods["Router/Reconcile"] = r.Reconcile
	}
	return handler{methods: methods, events: events}
}
