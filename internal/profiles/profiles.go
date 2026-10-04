// Package profiles keeps the user's mihomo configurations: imported from a
// subscription URL or a local file, one YAML per profile, with what is known
// about each in index.json.
package profiles

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

type Profile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	URL      string    `json:"url,omitempty"` // empty for a local file
	Interval int       `json:"interval"`      // auto update, in hours; 0 never
	Updated  time.Time `json:"updated"`
	// From the subscription-userinfo header, in bytes and Unix seconds.
	Upload   int64 `json:"upload,omitempty"`
	Download int64 `json:"download,omitempty"`
	Total    int64 `json:"total,omitempty"`
	Expire   int64 `json:"expire,omitempty"`
}

func (p Profile) Path() string { return filepath.Join(appdir.Profiles(), p.ID+".yaml") }

var (
	mu        sync.Mutex
	userAgent       = "clash.meta/clashcube"
	maxSize   int64 = 32 << 20
)

// SetUserAgent sets what subscriptions are fetched as.
func SetUserAgent(ua string) { userAgent = ua }

func indexPath() string { return filepath.Join(appdir.Profiles(), "index.json") }

func load() []Profile {
	var ps []Profile
	if b, err := os.ReadFile(indexPath()); err == nil {
		_ = json.Unmarshal(b, &ps)
	}
	return ps
}

func save(ps []Profile) error {
	b, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(appdir.Profiles(), 0o755); err != nil {
		return err
	}
	tmp := indexPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, indexPath())
}

// List is every profile, in the order they were added.
func List() []Profile {
	mu.Lock()
	defer mu.Unlock()
	ps := load()
	if ps == nil {
		ps = []Profile{}
	}
	return ps
}

// Get is the profile with id.
func Get(id string) (Profile, bool) {
	for _, p := range List() {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ImportURL downloads a subscription and adds it as a profile.
func ImportURL(raw, name string, interval int) (Profile, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Profile{}, errors.New("not an http(s) URL")
	}
	body, info, err := fetch(u.String())
	if err != nil {
		return Profile{}, err
	}
	p := Profile{ID: newID(), Name: name, URL: u.String(), Interval: interval}
	if p.Name == "" {
		p.Name = info.name
	}
	if p.Name == "" {
		p.Name = u.Host
	}
	info.apply(&p)
	return add(p, body)
}

// ImportFile copies a local YAML in as a profile.
func ImportFile(path, name string) (Profile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return add(Profile{ID: newID(), Name: name}, body)
}

// AddDefault writes the built-in starter profile, for a first run.
func AddDefault(body []byte) (Profile, error) {
	return add(Profile{ID: "default", Name: "Default"}, body)
}

func add(p Profile, body []byte) (Profile, error) {
	if err := Validate(body); err != nil {
		return Profile{}, err
	}
	p.Updated = time.Now()
	mu.Lock()
	defer mu.Unlock()
	if err := os.WriteFile(p.Path(), body, 0o600); err != nil {
		return Profile{}, err
	}
	ps := append(load(), p)
	return p, save(ps)
}

// Duplicate copies a profile as a local one named name: a subscription's
// file is replaced on every update, its copy is the user's to edit.
func Duplicate(id, name string) (Profile, error) {
	p, ok := Get(id)
	if !ok {
		return Profile{}, errors.New("no such profile")
	}
	body, err := os.ReadFile(p.Path())
	if err != nil {
		return Profile{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = p.Name
	}
	return add(Profile{ID: newID(), Name: strings.TrimSpace(name)}, body)
}

// Update downloads a subscription profile again.
func Update(id string) (Profile, error) {
	p, ok := Get(id)
	if !ok {
		return Profile{}, errors.New("no such profile")
	}
	if p.URL == "" {
		return p, errors.New("a local profile has nothing to update from")
	}
	body, info, err := fetch(p.URL)
	if err != nil {
		return p, err
	}
	if err := Validate(body); err != nil {
		return p, err
	}
	mu.Lock()
	defer mu.Unlock()
	if err := os.WriteFile(p.Path(), body, 0o600); err != nil {
		return p, err
	}
	ps := load()
	for i := range ps {
		if ps[i].ID == id {
			info.apply(&ps[i])
			ps[i].Updated = time.Now()
			p = ps[i]
		}
	}
	return p, save(ps)
}

// Edit renames a profile or changes its update interval.
func Edit(id, name string, interval int) (Profile, error) {
	mu.Lock()
	defer mu.Unlock()
	ps := load()
	for i := range ps {
		if ps[i].ID == id {
			if strings.TrimSpace(name) != "" {
				ps[i].Name = strings.TrimSpace(name)
			}
			ps[i].Interval = max(0, interval)
			return ps[i], save(ps)
		}
	}
	return Profile{}, errors.New("no such profile")
}

// Remove deletes a profile and its file.
func Remove(id string) error {
	mu.Lock()
	defer mu.Unlock()
	ps := load()
	out := ps[:0]
	for _, p := range ps {
		if p.ID == id {
			_ = os.Remove(p.Path())
			continue
		}
		out = append(out, p)
	}
	return save(out)
}

// Due is the subscriptions whose auto update interval has passed.
func Due(now time.Time) []Profile {
	var due []Profile
	for _, p := range List() {
		if p.URL != "" && p.Interval > 0 && now.Sub(p.Updated) >= time.Duration(p.Interval)*time.Hour {
			due = append(due, p)
		}
	}
	return due
}

// Validate checks body looks like a mihomo configuration: a YAML mapping with
// proxies, proxy-providers or rules.
func Validate(body []byte) error {
	var m map[string]any
	if err := yaml.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("not a valid YAML configuration: %w", err)
	}
	for _, k := range []string{"proxies", "proxy-providers", "proxy-groups", "rules"} {
		if _, ok := m[k]; ok {
			return nil
		}
	}
	return errors.New("not a mihomo configuration (no proxies, providers or rules)")
}

type fetched struct {
	name                            string
	upload, download, total, expire int64
	hasInfo                         bool
}

func (f fetched) apply(p *Profile) {
	if f.hasInfo {
		p.Upload, p.Download, p.Total, p.Expire = f.upload, f.download, f.total, f.expire
	}
}

var client = &http.Client{Timeout: 30 * time.Second}

func fetch(u string) ([]byte, fetched, error) {
	var info fetched
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, info, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, info, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, info, err
	}
	if int64(len(body)) > maxSize {
		return nil, info, errors.New("subscription is too large")
	}
	info.name = nameFrom(resp.Header)
	if ui := resp.Header.Get("Subscription-Userinfo"); ui != "" {
		info.upload, info.download, info.total, info.expire = parseUserinfo(ui)
		info.hasInfo = true
	}
	return body, info, nil
}

// nameFrom is the profile-title or Content-Disposition file name, if any.
func nameFrom(h http.Header) string {
	if t := h.Get("Profile-Title"); t != "" {
		if b64, ok := strings.CutPrefix(t, "base64:"); ok {
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				return string(b)
			}
		}
		return t
	}
	if _, params, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		n := params["filename"]
		return strings.TrimSuffix(n, filepath.Ext(n))
	}
	return ""
}

var userinfoRe = regexp.MustCompile(`(upload|download|total|expire)\s*=\s*(\d+)`)

func parseUserinfo(s string) (up, down, total, expire int64) {
	for _, m := range userinfoRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.ParseInt(m[2], 10, 64)
		switch m[1] {
		case "upload":
			up = n
		case "download":
			down = n
		case "total":
			total = n
		case "expire":
			expire = n
		}
	}
	return
}
