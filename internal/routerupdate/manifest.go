// Package routerupdate verifies and installs independent OpenWrt core IPKs.
// Package management, rather than the controller API, owns executable updates.
package routerupdate

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type Package struct {
	Name           string `json:"name"`
	File           string `json:"file"`
	Bytes          int64  `json:"bytes"`
	InstalledBytes int64  `json:"installedBytes"`
	SHA256         string `json:"sha256"`
}

type Manifest struct {
	Version  string    `json:"version"`
	BaseURL  string    `json:"baseURL"`
	Packages []Package `json:"packages"`
}

type envelope struct {
	Manifest json.RawMessage `json:"manifest"`
	Sig      []byte          `json:"sig"`
}

func ValidateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Fragment != "" {
		return errors.New("update URL must use HTTP or HTTPS without embedded credentials")
	}
	return nil
}

func message(body []byte) []byte { return append([]byte("clashcube-router-update\n"), body...) }

func Sign(key ed25519.PrivateKey, m Manifest) ([]byte, error) {
	if _, err := m.Core(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{Manifest: body, Sig: ed25519.Sign(key, message(body))})
}

func Open(body []byte, key ed25519.PublicKey) (Manifest, error) {
	var e envelope
	var m Manifest
	if json.Unmarshal(body, &e) != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, message(e.Manifest), e.Sig) {
		return m, errors.New("router update manifest signature is invalid")
	}
	if json.Unmarshal(e.Manifest, &m) != nil {
		return m, errors.New("router update manifest is malformed")
	}
	_, err := m.Core()
	return m, err
}

var versionPattern = regexp.MustCompile(`^[0-9][A-Za-z0-9.+~_-]{0,127}$`)

func (m Manifest) Core() (Package, error) {
	if !versionPattern.MatchString(m.Version) {
		return Package{}, errors.New("invalid router package version")
	}
	if err := ValidateURL(m.BaseURL); err != nil {
		return Package{}, err
	}
	var core Package
	for _, p := range m.Packages {
		if p.Name != "clashcube-core" {
			continue
		}
		if core.Name != "" {
			return core, errors.New("duplicate core update package")
		}
		core = p
	}
	sum, err := hex.DecodeString(core.SHA256)
	if err != nil || len(sum) != 32 || core.Bytes <= 0 || core.Bytes > 128<<20 || core.InstalledBytes <= 0 || core.InstalledBytes > 256<<20 || core.File != "clashcube-core_"+m.Version+"_aarch64_generic.ipk" {
		return core, errors.New("router update requires a valid ARM64 core package")
	}
	base, _ := url.Parse(strings.TrimSuffix(m.BaseURL, "/") + "/")
	if base.RawQuery != "" {
		return core, errors.New("update base URL must not contain a query")
	}
	return core, nil
}

func (m Manifest) CoreURL() string {
	return strings.TrimSuffix(m.BaseURL, "/") + "/clashcube-core_" + m.Version + "_aarch64_generic.ipk"
}
