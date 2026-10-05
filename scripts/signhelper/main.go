// signhelper signs a ClashCube executable so that an installed helper
// updates itself to it without a password (see internal/updatesig).
//
//	go run ./scripts/signhelper -genkey            # once; prints the public key
//	go run ./scripts/signhelper -stamp N EXE > SIG # sign
//	go run ./scripts/signhelper -check EXE SIG     # verify with the built-in key
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "signhelper:", err)
		os.Exit(1)
	}
}

func defaultKey() string {
	if p := os.Getenv("CLASHCUBE_UPDATE_KEY"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clashcube", "update.key")
}

func run() error {
	keyPath := flag.String("key", defaultKey(), "private key file")
	genkey := flag.Bool("genkey", false, "create the private key and print the public key")
	stamp := flag.Int64("stamp", 0, "build time (unix seconds)")
	check := flag.Bool("check", false, "verify EXE against SIG")
	flag.Parse()

	switch {
	case *genkey:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*keyPath), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(*keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"); err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return nil
	case *check:
		if flag.NArg() != 2 {
			return errors.New("usage: -check EXE SIG")
		}
		exe, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(flag.Arg(1))
		if err != nil {
			return err
		}
		st, err := updatesig.Verify(updatesig.Key(), exe, sig)
		if err != nil {
			return err
		}
		fmt.Println("ok, stamp", st)
		return nil
	}
	if flag.NArg() != 1 || *stamp <= 0 {
		return errors.New("usage: -stamp N EXE")
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("bad key file " + *keyPath)
	}
	exe, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		return err
	}
	sig, err := updatesig.Sign(ed25519.NewKeyFromSeed(seed), exe, *stamp)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(sig)
	return err
}
