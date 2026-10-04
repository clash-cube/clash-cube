package backend

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

// FlushDNS clears the core's DNS cache and its fake-ip pool (a no-op
// without one).
func (b *Backend) FlushDNS() error {
	c, err := b.Client()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := c.FlushDNS(ctx); err != nil {
		return err
	}
	return c.FlushFakeIP(ctx)
}

// GeoInfo is the core's GEO databases: when they last changed (unix ms, 0
// when there are none yet) and whether an update is under way.
type GeoInfo struct {
	Updated  int64 `json:"updated"`
	Updating bool  `json:"updating"`
}

// geoFiles are the databases mihomo keeps in its home (constant/path.go).
var geoFiles = []string{"Country.mmdb", "geoip.db", "geoip.metadb", "ASN.mmdb", "GeoIP.dat", "GeoSite.dat"}

func (b *Backend) GeoInfo() GeoInfo {
	b.mu.Lock()
	updating := b.geoUpdating
	b.mu.Unlock()
	return GeoInfo{Updated: geoUpdated(), Updating: updating}
}

func geoUpdated() int64 {
	es, _ := os.ReadDir(appdir.CoreHome())
	var last time.Time
	for _, e := range es {
		for _, n := range geoFiles {
			if !e.IsDir() && strings.EqualFold(e.Name(), n) {
				if fi, err := e.Info(); err == nil && fi.ModTime().After(last) {
					last = fi.ModTime()
				}
			}
		}
	}
	if last.IsZero() {
		return 0
	}
	return last.UnixMilli()
}

// UpdateGeo has the core download its GEO databases again, waiting until it
// has. An update already under way, ours or the core's own, is not an
// error: the result has Updating set instead.
func (b *Backend) UpdateGeo() (GeoInfo, error) {
	c, err := b.Client()
	if err != nil {
		return b.GeoInfo(), err
	}
	b.mu.Lock()
	if b.geoUpdating {
		b.mu.Unlock()
		return b.GeoInfo(), nil
	}
	b.geoUpdating = true
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	err = c.UpdateGeo(ctx)
	cancel()
	b.mu.Lock()
	b.geoUpdating = false
	b.mu.Unlock()
	if mihomoapi.IsGeoUpdating(err) {
		return GeoInfo{Updated: geoUpdated(), Updating: true}, nil
	}
	return b.GeoInfo(), err
}
