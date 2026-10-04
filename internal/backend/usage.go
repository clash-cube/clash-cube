package backend

import (
	"cmp"
	"context"
	"time"

	"github.com/localhost-copilot/clashferry/internal/appdir"
	"github.com/localhost-copilot/clashferry/internal/mihomoapi"
	"github.com/localhost-copilot/clashferry/internal/usage"
)

const (
	usageEvery = 2 * time.Second
	usageFlush = time.Minute
)

// usageMeter turns successive /connections snapshots into what each app,
// host, policy and network moved since the last one.
type usageMeter struct {
	upTotal, downTotal int64
	seen               map[string][2]int64 // connection → upload, download
}

// sample is the core's bytes since the last snapshot and their split.
// closed is the connections that ended since, with their final counts, so
// one that opened and closed between two snapshots is counted too.
func (m *usageMeter) sample(cs mihomoapi.Connections, closed []mihomoapi.Connection, network usage.Item) (up, down int64, items []usage.Item) {
	if m.seen != nil {
		// the totals go back to 0 when the core starts again
		up, down = cs.UploadTotal-m.upTotal, cs.DownloadTotal-m.downTotal
		if up < 0 || down < 0 {
			up, down = cs.UploadTotal, cs.DownloadTotal
		}
	}
	seen := make(map[string][2]int64, len(cs.Connections))
	newConns := 0
	add := func(c mihomoapi.Connection) {
		cu, cd := c.Upload, c.Download
		before, known := m.seen[c.ID]
		if known {
			cu, cd = max(cu-before[0], 0), max(cd-before[1], 0)
		} else {
			newConns++
		}
		if cu == 0 && cd == 0 && known {
			return
		}
		app, path := appOf(c.Metadata)
		host := cmp.Or(c.Metadata.SniffHost, c.Metadata.Host, c.Metadata.DestIP)
		policy := ""
		if len(c.Chains) > 0 {
			policy = c.Chains[0]
		}
		items = append(items,
			usage.Item{Dim: usage.App, Key: path + "\x00" + app, Name: app, Path: path, Up: cu, Down: cd, New: !known},
			usage.Item{Dim: usage.Host, Key: host, Name: host, Up: cu, Down: cd, New: !known},
			usage.Item{Dim: usage.Policy, Key: policy, Name: policy, Up: cu, Down: cd, New: !known},
		)
	}
	ended := make(map[string]bool, len(closed))
	for _, c := range closed {
		ended[c.ID] = true
	}
	for _, c := range cs.Connections {
		// closed after the snapshot: its final counts follow
		if ended[c.ID] {
			continue
		}
		seen[c.ID] = [2]int64{c.Upload, c.Download}
		if m.seen != nil {
			add(c)
		}
	}
	if m.seen != nil {
		for _, c := range closed {
			add(c)
		}
	}
	m.upTotal, m.downTotal, m.seen = cs.UploadTotal, cs.DownloadTotal, seen
	if network.Key != "" {
		network.Up, network.Down = up, down
		items = append(items, network)
		for range newConns {
			items = append(items, usage.Item{Dim: network.Dim, Key: network.Key, Name: network.Name, New: true})
		}
	}
	return up, down, items
}

// usageNetwork names the network traffic counts under now: a Wi-Fi by its
// name, any wired network as one; "" offline.
func (b *Backend) usageNetwork() usage.Item {
	b.mu.Lock()
	n := b.net.seen
	b.mu.Unlock()
	it := usage.Item{Dim: usage.Network}
	switch {
	case n.Kind == "wired":
		it.Key = "wired"
	case n.Kind == "wifi" && n.WiFi.SSID != "":
		it.Key, it.Name = "wifi:"+n.WiFi.SSID, n.WiFi.SSID
	case n.Kind == "wifi":
		it.Key = "wifi"
	case n.Kind == "":
		it.Key = "unknown"
	}
	return it
}

// Usage is the traffic statistics, kept across runs.
func (b *Backend) Usage() *usage.Store {
	b.usageOnce.Do(func() { b.usage = usage.Open(appdir.Usage()) })
	return b.usage
}

// recordUsage samples the core's connections into the statistics while it
// runs, writing them out every minute and when it stops.
func (b *Backend) recordUsage(ctx context.Context, c *mihomoapi.Client) {
	store := b.Usage()
	defer store.Flush()
	var m usageMeter
	t := time.NewTicker(usageEvery)
	defer t.Stop()
	flushed := time.Now()
	for {
		rctx, cancel := context.WithTimeout(ctx, usageEvery)
		cs, err := c.Connections(rctx)
		// asked after the snapshot, so a connection that ends between the
		// two is in this list rather than lost
		closed, _ := c.ClosedConnections(rctx)
		cancel()
		if err == nil {
			now := time.Now()
			up, down, items := m.sample(cs, closed, b.usageNetwork())
			if up != 0 || down != 0 || len(items) > 0 {
				store.Record(now, up, down, items)
			}
			if now.Sub(flushed) >= usageFlush {
				_ = store.Flush()
				flushed = now
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
