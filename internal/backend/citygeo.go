package backend

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// The city database is DB-IP's free City Lite (CC BY 4.0), for the globe
// to place connections at their cities. It is large, so it is only
// fetched once asked for, and kept apart from the core's databases.
const (
	cityURL = "https://download.db-ip.com/free/dbip-city-lite-%s.mmdb.gz"
	cityMax = 512 << 20
	// DB-IP publishes a month's at its start
	cityTTL = 35 * 24 * time.Hour
)

var city struct {
	sync.RWMutex // the reader, against closing it under a lookup
	r            *maxminddb.Reader
	// the rest under mu
	mu       sync.Mutex
	opened   bool
	cancel   context.CancelFunc
	got, of  int64 // bytes downloaded, of the size; of is 0 while unknown
	err      string
	failedAt time.Time
}

// CityGeoInfo is the city database's state, for Settings.
type CityGeoInfo struct {
	On          bool   `json:"on"`
	Ready       bool   `json:"ready"`
	Downloading bool   `json:"downloading"`
	Got         int64  `json:"got"`
	Of          int64  `json:"of"`
	Size        int64  `json:"size"`    // on disk
	Updated     int64  `json:"updated"` // unix ms; 0 when there is none
	Error       string `json:"error"`
}

func cityPath() string { return filepath.Join(appdir.Root(), "geo", "dbip-city-lite.mmdb") }

func (b *Backend) CityGeoInfo() CityGeoInfo {
	city.mu.Lock()
	defer city.mu.Unlock()
	i := CityGeoInfo{On: settings.Load().CityGeo, Downloading: city.cancel != nil, Got: city.got, Of: city.of, Error: city.err}
	if fi, err := os.Stat(cityPath()); err == nil {
		i.Ready, i.Size, i.Updated = true, fi.Size(), fi.ModTime().UnixMilli()
	}
	return i
}

// SetCityGeo turns the city database on, fetching it, or off, removing it.
func (b *Backend) SetCityGeo(on bool) (CityGeoInfo, error) {
	if _, err := b.PatchSettings(func(s *settings.Settings) { s.CityGeo = on }); err != nil {
		return b.CityGeoInfo(), err
	}
	city.mu.Lock()
	city.err, city.failedAt = "", time.Time{}
	if !on {
		if city.cancel != nil {
			city.cancel()
			city.cancel = nil
		}
		_ = os.Remove(cityPath())
		city.mu.Unlock()
		closeCity()
		return b.CityGeoInfo(), nil
	}
	city.mu.Unlock()
	if fi, err := os.Stat(cityPath()); err != nil || time.Since(fi.ModTime()) > cityTTL {
		b.fetchCity(true)
	}
	return b.CityGeoInfo(), nil
}

// UpdateCityGeo fetches the database again now.
func (b *Backend) UpdateCityGeo() CityGeoInfo {
	if settings.Load().CityGeo {
		b.fetchCity(true)
	}
	return b.CityGeoInfo()
}

// cityReady says whether the database is open, opening it when it is on
// disk and fetching a new month's in the background when it is old.
func cityReady() bool {
	city.RLock()
	r := city.r
	city.RUnlock()
	if r != nil {
		return true
	}
	city.mu.Lock()
	defer city.mu.Unlock()
	if city.opened {
		return false
	}
	city.opened = true // once; a download opens the new one
	r, err := maxminddb.Open(cityPath())
	if err != nil {
		return false
	}
	city.Lock()
	city.r = r
	city.Unlock()
	return true
}

// closeCity lets go of the database, for it to be removed or replaced.
func closeCity() {
	city.Lock()
	r := city.r
	city.r = nil
	city.Unlock()
	if r != nil {
		_ = r.Close()
	}
	city.mu.Lock()
	city.opened = false
	city.mu.Unlock()
}

// cityPlace is the city ip is in; false when it isn't known, or the
// database isn't open.
func cityPlace(ip string) (Place, bool) {
	addr := net.ParseIP(ip)
	if addr == nil {
		return Place{}, false
	}
	city.RLock()
	defer city.RUnlock()
	if city.r == nil {
		return Place{}, false
	}
	var rec struct {
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Location struct {
			Lat float64 `maxminddb:"latitude"`
			Lon float64 `maxminddb:"longitude"`
		} `maxminddb:"location"`
	}
	if city.r.Lookup(addr, &rec) != nil {
		return Place{}, false
	}
	// "San Francisco (Union Square)" is San Francisco on a globe
	name, _, _ := strings.Cut(rec.City.Names["en"], " (")
	if name == "" || len(rec.Country.ISOCode) != 2 || (rec.Location.Lat == 0 && rec.Location.Lon == 0) {
		return Place{}, false
	}
	return Place{Lat: rec.Location.Lat, Lon: rec.Location.Lon, CC: rec.Country.ISOCode, City: name}, true
}

// refreshCity fetches a new month's database when the one there is old,
// unless the network is metered; it runs with the app.
func (b *Backend) refreshCity() {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if settings.Load().CityGeo && !b.SavingData() {
			fi, err := os.Stat(cityPath())
			if err != nil || time.Since(fi.ModTime()) > cityTTL {
				b.fetchCity(false)
			}
		}
		<-t.C
	}
}

// fetchCity downloads the database in the background, unless a download
// runs, or, not asked for, the last one failed lately.
func (b *Backend) fetchCity(asked bool) {
	city.mu.Lock()
	defer city.mu.Unlock()
	if city.cancel != nil || (!asked && time.Since(city.failedAt) < time.Hour) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	city.cancel, city.got, city.of, city.err = cancel, 0, 0, ""
	go func() {
		err := downloadCity(ctx, cityPath(), time.Now())
		city.mu.Lock()
		if city.cancel != nil && ctx.Err() == nil {
			city.cancel = nil
			if err != nil {
				city.err, city.failedAt = err.Error(), time.Now()
			}
		}
		city.mu.Unlock()
		cancel()
		if err == nil {
			closeCity() // the new one opens at the next lookup
		}
	}()
}

// downloadCity fetches this month's database, or last month's when this
// one isn't out yet, through the core's mixed port when something listens
// there. It is unpacked beside path and checked to open before it takes
// the place of anything.
func downloadCity(ctx context.Context, path string, now time.Time) error {
	tr := &http.Transport{}
	if port := settings.Load().MixedPort; listening(port) {
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: net.JoinHostPort(proxyHost, strconv.Itoa(port))})
	}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr, Timeout: 15 * time.Minute}
	var resp *http.Response
	for _, m := range []time.Time{now, now.AddDate(0, 0, -now.Day())} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(cityURL, m.UTC().Format("2006-01")), nil)
		if err != nil {
			return err
		}
		if resp, err = hc.Do(req); err != nil {
			return err
		}
		if resp.StatusCode/100 == 2 {
			break
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		resp = nil
	}
	if resp == nil {
		return errors.New("HTTP 404")
	}
	defer resp.Body.Close()
	city.mu.Lock()
	city.of = max(resp.ContentLength, 0)
	city.mu.Unlock()
	zr, err := gzip.NewReader(&counted{r: resp.Body})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	n, err := io.Copy(f, io.LimitReader(zr, cityMax+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > cityMax {
		return errors.New("too large")
	}
	r, err := maxminddb.Open(tmp)
	if err != nil {
		return err
	}
	_ = r.Close()
	// under mu, so turning it off meanwhile removes it or keeps it out
	city.mu.Lock()
	defer city.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return os.Rename(tmp, path)
}

// counted tells the download's progress as it is read.
type counted struct{ r io.Reader }

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	city.mu.Lock()
	city.got += int64(n)
	city.mu.Unlock()
	return n, err
}
