package backend

import (
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

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// The country database is the core's own: the file and the URL mihomo
// uses by default, in its home, so GEOIP rules and "Update GEO databases"
// share the copy the overview's flags are looked up in.
const (
	geoIPName = "geoip.metadb"
	geoIPURL  = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"
	geoIPMax  = 64 << 20
)

var geo struct {
	sync.Mutex
	r        *maxminddb.Reader
	path     string
	mtime    time.Time
	fetching bool
	failedAt time.Time
}

// geoIPPath is the database the core would load: mihomo takes the first
// of these it finds in its home.
func geoIPPath() string {
	home := appdir.CoreHome()
	if es, err := os.ReadDir(home); err == nil {
		for _, e := range es {
			for _, n := range []string{"Country.mmdb", "geoip.db", geoIPName} {
				if !e.IsDir() && strings.EqualFold(e.Name(), n) {
					return filepath.Join(home, e.Name())
				}
			}
		}
	}
	return filepath.Join(home, geoIPName)
}

// country is ip's ISO country code, upper case; "" when unknown, or while
// the database is still being fetched.
func country(ip string) string {
	addr := net.ParseIP(ip)
	if addr == nil {
		return ""
	}
	r := geoReader()
	if r == nil {
		return ""
	}
	var rec any
	if r.Lookup(addr, &rec) != nil {
		return ""
	}
	return strings.ToUpper(countryOf(rec))
}

// countryOf reads the three layouts mihomo accepts: MaxMind's record,
// sing-geoip's code, and MetaCubeX's code or codes.
func countryOf(rec any) string {
	switch v := rec.(type) {
	case string:
		return v
	case []any:
		for _, c := range v {
			if s, ok := c.(string); ok && len(s) == 2 {
				return s
			}
		}
	case map[string]any:
		if c, ok := v["country"].(map[string]any); ok {
			s, _ := c["iso_code"].(string)
			return s
		}
	}
	return ""
}

// geoReader opens the database, again when the core has replaced it, and
// fetches it in the background when there is none.
func geoReader() *maxminddb.Reader {
	geo.Lock()
	defer geo.Unlock()
	p := geoIPPath()
	fi, err := os.Stat(p)
	if err != nil {
		if !geo.fetching && time.Since(geo.failedAt) > 5*time.Minute {
			geo.fetching = true
			go fetchGeoIP(p)
		}
		return nil
	}
	if geo.r == nil || geo.path != p || !fi.ModTime().Equal(geo.mtime) {
		r, err := maxminddb.Open(p)
		if err != nil {
			return nil
		}
		if geo.r != nil {
			_ = geo.r.Close()
		}
		geo.r, geo.path, geo.mtime = r, p, fi.ModTime()
	}
	return geo.r
}

func fetchGeoIP(path string) {
	err := downloadGeoIP(path)
	geo.Lock()
	geo.fetching = false
	if err != nil {
		geo.failedAt = time.Now()
	}
	geo.Unlock()
}

// downloadGeoIP fetches the database through the core's mixed port, as
// GitHub may need a proxy, or directly when nothing listens there; it is
// checked to open before it takes the place of anything.
func downloadGeoIP(path string) error {
	tr := &http.Transport{}
	if port := settings.Load().MixedPort; listening(port) {
		tr.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: net.JoinHostPort(proxyHost, strconv.Itoa(port))})
	}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr, Timeout: 2 * time.Minute}).Get(geoIPURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("geoip: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, geoIPMax+1))
	if err != nil {
		return err
	}
	if len(b) > geoIPMax {
		return errors.New("geoip: too large")
	}
	r, err := maxminddb.FromBytes(b)
	if err != nil {
		return fmt.Errorf("geoip: %w", err)
	}
	_ = r.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
