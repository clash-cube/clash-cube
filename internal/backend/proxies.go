package backend

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

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

// providerProxies is every provider's proxies by name, which /proxies
// leaves out.
func providerProxies(ctx context.Context, c *mihomoapi.Client) (map[string]mihomoapi.ProxyProvider, map[string]mihomoapi.Proxy, error) {
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
func (b *Backend) Groups() ([]Group, error) {
	c, err := b.Client()
	if err != nil {
		return nil, err
	}
	all, err := c.Proxies(context.Background())
	if err != nil {
		return nil, err
	}
	// a group that uses a provider lists its proxies by name only
	_, fromProviders, _ := providerProxies(context.Background(), c)
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
	for _, m := range modules.For(settings.Load().Profile, modules.List()) {
		if k, ok := m.Kind().(modules.Grouper); ok {
			if g := k.GroupIn(func(n string) bool { _, ok := all[n]; return ok }); g != "" {
				made[g] = m.ID
			}
		}
	}
	var out []Group
	for _, name := range order {
		p, ok := all[name]
		if !ok || len(p.All) == 0 || strings.HasPrefix(name, runtimecfg.ChainPrefix) {
			continue
		}
		g := Group{Name: p.Name, Type: p.Type, Now: p.Now, Hidden: p.Hidden, Icon: p.Icon, TestURL: p.TestURL, Module: made[p.Name]}
		for _, m := range p.All {
			if name == "GLOBAL" && strings.HasPrefix(m, runtimecfg.ChainPrefix) {
				continue
			}
			mp := all[m]
			selected := all[SelectedProxy(all, m)]
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
func (b *Backend) Providers() ([]Provider, error) {
	c, err := b.Client()
	if err != nil {
		return nil, err
	}
	all, _, err := providerProxies(context.Background(), c)
	if err != nil {
		return nil, err
	}
	out := []Provider{}
	for _, p := range all {
		if p.VehicleType == "Compatible" || p.Name == "default" || strings.HasPrefix(p.Name, runtimecfg.ChainPrefix) {
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
