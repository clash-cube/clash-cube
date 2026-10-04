package backend

import (
	"context"
	"sort"

	"github.com/localhost-copilot/mihomobar/internal/modules"
)

// Node is one of the running profile's nodes, for a route to pick from.
type Node struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Delay  int    `json:"delay"`            // last test, ms; 0 untested, -1 failed
	Region string `json:"region,omitempty"` // the first modules.Regions key that takes it
}

// Nodes is the running profile's nodes and its providers', by name; nil
// with the core stopped.
func (b *Backend) Nodes() []Node {
	c, err := b.Client()
	if err != nil {
		return nil
	}
	ctx := context.Background()
	all, err := c.Proxies(ctx)
	if err != nil {
		return nil
	}
	if pvs, err := c.ProxyProviders(ctx); err == nil {
		for _, pv := range pvs {
			if pv.VehicleType == "Compatible" {
				continue
			}
			for _, p := range pv.Proxies {
				if _, ok := all[p.Name]; !ok {
					all[p.Name] = p
				}
			}
		}
	}
	out := []Node{}
	for _, p := range all {
		if len(p.All) > 0 || builtinPolicy[p.Type] {
			continue
		}
		d := 0
		if n := len(p.History); n > 0 {
			if d = p.History[n-1].Delay; d == 0 {
				d = -1
			}
		}
		n := Node{Name: p.Name, Type: p.Type, Delay: d}
		for _, r := range modules.Regions {
			if r.Matches(p.Name) {
				n.Region = r.Key
				break
			}
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RegionNodes is a region and how many of the running profile's nodes it
// takes; Count is -1 with the core stopped.
type RegionNodes struct {
	modules.Region
	Count int `json:"count"`
}

// RouteRegions is the regions a route can pick nodes by, counted over the
// nodes the core has now.
func (b *Backend) RouteRegions() []RegionNodes {
	nodes := b.Nodes()
	out := make([]RegionNodes, 0, len(modules.Regions))
	for _, r := range modules.Regions {
		n := -1
		if nodes != nil {
			n = 0
			for _, node := range nodes {
				if node.Region == r.Key {
					n++
				}
			}
		}
		out = append(out, RegionNodes{r, n})
	}
	return out
}

// the types /proxies gives the core's own policies
var builtinPolicy = map[string]bool{"Direct": true, "Reject": true, "RejectDrop": true, "Compatible": true, "Pass": true, "PassRule": true}
