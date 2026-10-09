package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := pubKey
	pubKey = func() ed25519.PublicKey { return pub }
	t.Cleanup(func() { pubKey = old })
	return priv
}

func TestManifestSignature(t *testing.T) {
	key := testKey(t)
	m := Manifest{Version: "0.2.0", Stamp: 42, Assets: map[string]Asset{"arm64": {URL: "https://x/a.zip", SHA256: strings.Repeat("a", 64)}}}
	b, err := Sign(key, m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(b)
	if err != nil || got.Version != "0.2.0" || got.Assets["arm64"].URL != "https://x/a.zip" {
		t.Fatalf("open: %+v %v", got, err)
	}

	// another key
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if b, _ := Sign(other, m); b != nil {
		if _, err := Open(b); err == nil {
			t.Error("accepted a manifest signed with another key")
		}
	}
	// a changed asset URL or hash
	var e envelope
	json.Unmarshal(b, &e)
	e.Manifest = []byte(strings.Replace(string(e.Manifest), "https://x/a.zip", "https://evil/a.zip", 1))
	changed, _ := json.Marshal(e)
	if _, err := Open(changed); err == nil {
		t.Error("accepted a changed manifest")
	}
	// a helper signature is not a manifest signature
	var helper envelope
	json.Unmarshal(b, &helper)
	helper.Sig = ed25519.Sign(key, helper.Manifest)
	raw, _ := json.Marshal(helper)
	if _, err := Open(raw); err == nil {
		t.Error("accepted a signature without the manifest's prefix")
	}
	// no key built in
	pubKey = func() ed25519.PublicKey { return nil }
	if _, err := Open(b); err == nil {
		t.Error("accepted without a key")
	}
}

func TestLatestPicksHighestRelease(t *testing.T) {
	key := testKey(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			json.NewEncoder(w).Encode([]map[string]any{
				{"tag_name": "v0.3.0", "draft": true, "assets": []asset{{"update.json", srv.URL + "/draft"}}},
				{"tag_name": "v0.1.9", "assets": []asset{{"update.json", srv.URL + "/old"}}},
				{"tag_name": "v0.2.0", "assets": []asset{{"update.json", srv.URL + "/new"}}},
				{"tag_name": "v0.2.1", "assets": []asset{{"ClashCube.dmg", srv.URL + "/dmg"}}},
			})
		case "/new":
			b, _ := Sign(key, Manifest{Version: "0.2.0", Stamp: 2})
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &http.Client{Transport: rewrite{srv.URL}}
	m, err := Latest(context.Background(), c)
	if err != nil || m.Version != "0.2.0" {
		t.Fatalf("latest: %+v %v", m, err)
	}
}

// rewrite sends GitHub's API to the test server.
type rewrite struct{ base string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.github.com" {
		u := r.base + "/releases"
		nr, _ := http.NewRequestWithContext(req.Context(), req.Method, u, nil)
		req = nr
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestDownloadChecksum(t *testing.T) {
	body := []byte("the app")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	sum := sha256.Sum256(body)
	path := filepath.Join(t.TempDir(), "a.zip")
	var seen int64
	if err := download(context.Background(), srv.Client(), Asset{URL: srv.URL, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}, path, func(d, _ int64) { seen = d }); err != nil {
		t.Fatal(err)
	}
	if seen != int64(len(body)) {
		t.Errorf("progress said %d", seen)
	}
	if err := download(context.Background(), srv.Client(), Asset{URL: srv.URL, SHA256: strings.Repeat("0", 64)}, path, nil); err == nil {
		t.Error("accepted a download that doesn't match its checksum")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("kept a download that doesn't match its checksum")
	}
}

func TestVersions(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{
		{"0.2.0", "v0.1.4", true},
		{"0.1.4", "v0.1.4", false},
		{"0.1.10", "0.1.9", true},
		{"0.1.4", "0.1.4-rc1", true},
		{"0.1.4-rc1", "0.1.4", false},
		{"dev", "0.1.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
	for v, want := range map[string]bool{"v0.1.4": true, "0.1.4": true, "dev": false, "v0.1.4-3-g0bcb2cc": false, "v0.1.4-dirty": false, "0bcb2cc": false} {
		if Released(v) != want {
			t.Errorf("Released(%q) != %v", v, want)
		}
	}
}
