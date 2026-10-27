// signrouter turns a package.py manifest into a signed OpenWrt core-update feed.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/routerupdate"
	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "signrouter:", err)
		os.Exit(1)
	}
}

func run() error {
	home, _ := os.UserHomeDir()
	defaultKey := os.Getenv("CLASHCUBE_UPDATE_KEY")
	if defaultKey == "" {
		defaultKey = filepath.Join(home, ".config", "clashcube", "update.key")
	}
	keyPath := flag.String("key", defaultKey, "existing release-signing key")
	base := flag.String("base", "", "HTTP(S) directory containing the IPKs")
	out := flag.String("out", "", "signed feed output path")
	flag.Parse()
	if flag.NArg() != 1 || *base == "" || *out == "" {
		return errors.New("usage: signrouter -base URL -out router-update.json packages.json")
	}
	body, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		return err
	}
	var m routerupdate.Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return err
	}
	m.BaseURL = *base
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return errors.New("cannot read the existing release-signing key")
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("invalid release-signing key")
	}
	signed, err := routerupdate.Sign(ed25519.NewKeyFromSeed(seed), m)
	if err != nil {
		return err
	}
	if _, err := routerupdate.Open(signed, updatesig.Key()); err != nil {
		return errors.New("signing key does not match the built-in public key")
	}
	return os.WriteFile(*out, append(signed, '\n'), 0644)
}
