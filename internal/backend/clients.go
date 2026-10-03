package backend

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
)

// ClientRate is one app's traffic now, as Surge's "Top Clients" lists it:
// its name, the path its icon comes from, and its speed in bytes a second.
type ClientRate struct {
	Name     string
	Path     string // the app bundle, or the executable; "" for a LAN client
	Up, Down int64
	total    int64
}

// ClientMeter turns successive snapshots of /connections into each app's
// speed. An app's helper processes count as the app: Chrome's renderers are
// Chrome.
type ClientMeter struct {
	at   time.Time
	seen map[string][2]int64 // connection → upload, download at the last sample
}

// Sample takes the connections at now and returns the apps by speed, then
// by bytes moved. The first sample has no speeds yet.
func (m *ClientMeter) Sample(now time.Time, conns []mihomoapi.Connection) []ClientRate {
	secs := now.Sub(m.at).Seconds()
	first := m.at.IsZero() || secs <= 0
	seen := make(map[string][2]int64, len(conns))
	byApp := map[string]*ClientRate{}
	var out []*ClientRate
	for _, c := range conns {
		name, path := appOf(c.Metadata)
		r := byApp[path+"\x00"+name]
		if r == nil {
			r = &ClientRate{Name: name, Path: path}
			byApp[path+"\x00"+name] = r
			out = append(out, r)
		}
		up, down := c.Upload, c.Download
		seen[c.ID] = [2]int64{up, down}
		r.total += up + down
		if first {
			continue
		}
		// a connection opened since the last sample moved all its bytes since
		if before, ok := m.seen[c.ID]; ok {
			up, down = max(up-before[0], 0), max(down-before[1], 0)
		} else if c.Start.Before(m.at) {
			continue
		}
		r.Up += int64(float64(up) / secs)
		r.Down += int64(float64(down) / secs)
	}
	m.at, m.seen = now, seen
	slices.SortStableFunc(out, func(a, b *ClientRate) int {
		return cmp.Or(cmp.Compare(b.Up+b.Down, a.Up+a.Down), cmp.Compare(b.total, a.total), strings.Compare(a.Name, b.Name))
	})
	rates := make([]ClientRate, len(out))
	for i, r := range out {
		rates[i] = *r
	}
	return rates
}

// appOf names the app a connection belongs to: the outermost .app bundle
// on its process's path, else the process, else the client's address.
func appOf(md mihomoapi.Metadata) (name, path string) {
	if p := md.ProcessPath; p != "" {
		if i := strings.Index(p, ".app/"); i >= 0 {
			bundle := p[:i+len(".app")]
			return strings.TrimSuffix(filepath.Base(bundle), ".app"), bundle
		}
		return cmp.Or(md.Process, filepath.Base(p)), p
	}
	return cmp.Or(md.Process, md.SourceIP), ""
}
