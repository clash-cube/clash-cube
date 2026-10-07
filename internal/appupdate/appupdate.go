// Package appupdate keeps ClashCube current. Each GitHub release carries
// update.json: the version, the build stamp and every architecture's zipped
// app with its SHA-256, signed with the update key that also signs the
// helper (internal/updatesig).
//
// The app is only signed ad hoc, so macOS can't say who built a download;
// the signed manifest says it instead. A zip whose hash it lists is the
// release, whichever server or proxy it came through. The new bundle then
// takes the old one's place, and the running copy goes on until it quits.
package appupdate

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

// Repo publishes the releases.
const Repo = "clash-cube/clash-cube"

// ManifestName is the signed manifest's file name in a release.
const ManifestName = "update.json"

// Manifest describes one release.
type Manifest struct {
	Version string           `json:"version"` // "0.2.0", no v
	Stamp   int64            `json:"stamp"`   // the build's updatesig stamp
	Notes   string           `json:"notes"`   // markdown
	URL     string           `json:"url"`     // the release page
	Assets  map[string]Asset `json:"assets"`  // the zipped app, by GOARCH
}

// Asset is one downloadable file of a release.
type Asset struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// envelope is update.json: the manifest as it was signed, and the signature.
type envelope struct {
	Manifest json.RawMessage `json:"manifest"`
	Sig      []byte          `json:"sig"`
}

// pubKey verifies manifests and executables; tests use their own.
var pubKey = updatesig.Key

// message keeps a manifest's signature from passing for a helper's.
func message(raw []byte) []byte {
	return append([]byte("clashcube-app-update\n"), raw...)
}

// Sign wraps manifest in a signed update.json.
func Sign(key ed25519.PrivateKey, m Manifest) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{Manifest: raw, Sig: ed25519.Sign(key, message(raw))})
}

// Open checks update.json's signature and returns its manifest.
func Open(b []byte) (*Manifest, error) {
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil || len(e.Manifest) == 0 {
		return nil, errors.New("update manifest is malformed")
	}
	pub := pubKey()
	if len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, message(e.Manifest), e.Sig) {
		return nil, errors.New("update manifest is not signed with the update key")
	}
	var m Manifest
	if err := json.Unmarshal(e.Manifest, &m); err != nil {
		return nil, errors.New("update manifest is malformed")
	}
	if parse(m.Version) == nil || m.Stamp <= 0 {
		return nil, errors.New("update manifest has no version")
	}
	return &m, nil
}

// Latest finds the newest release's manifest and checks it.
// CLASHCUBE_UPDATE_FEED names a manifest to use instead, for trying an
// update against a local server.
func Latest(ctx context.Context, c *http.Client) (*Manifest, error) {
	url := os.Getenv("CLASHCUBE_UPDATE_FEED")
	if url == "" {
		var err error
		if url, err = manifestURL(ctx, c); err != nil {
			return nil, err
		}
	}
	b, err := get(ctx, c, url, 1<<20)
	if err != nil {
		return nil, err
	}
	return Open(b)
}

// manifestURL is where the highest-versioned published release keeps its
// manifest. Prereleases count: every release so far is one.
func manifestURL(ctx context.Context, c *http.Client) (string, error) {
	b, err := get(ctx, c, "https://api.github.com/repos/"+Repo+"/releases?per_page=20", 4<<20)
	if err != nil {
		return "", err
	}
	var releases []struct {
		Tag    string `json:"tag_name"`
		Draft  bool   `json:"draft"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(b, &releases); err != nil {
		return "", fmt.Errorf("releases: %w", err)
	}
	best, url := "", ""
	for _, r := range releases {
		if r.Draft || parse(r.Tag) == nil || (best != "" && !Newer(r.Tag, best)) {
			continue
		}
		for _, a := range r.Assets {
			if a.Name == ManifestName {
				best, url = r.Tag, a.URL
			}
		}
	}
	if url == "" {
		return "", errors.New("no release has an update manifest")
	}
	return url, nil
}

func get(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", req.URL.Host, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// Released reports whether v is a release rather than a build from source
// ("dev", "0bcb2cc-dirty", git describe's "v0.1.0-3-g0bcb2cc"); only
// releases replace themselves.
func Released(v string) bool {
	s := parse(v)
	return s != nil && s.pre == ""
}

// Newer reports whether version a comes after b. A pre-release comes
// before the release it leads up to.
func Newer(a, b string) bool {
	x, y := parse(a), parse(b)
	if x == nil || y == nil {
		return false
	}
	for i := range 3 {
		if x.n[i] != y.n[i] {
			return x.n[i] > y.n[i]
		}
	}
	switch {
	case x.pre == y.pre:
		return false
	case x.pre == "":
		return true
	case y.pre == "":
		return false
	}
	return x.pre > y.pre
}

type semver struct {
	n   [3]int
	pre string
}

func parse(v string) *semver {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		s.n[i] = n
	}
	s.pre = pre
	return &s
}
