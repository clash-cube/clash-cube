// signhelper signs a ClashCube executable so that an installed helper
// updates itself to it without a password (see internal/updatesig).
//
//	go run ./scripts/signhelper -genkey            # once; prints the public key
//	go run ./scripts/signhelper -stamp N EXE > SIG # sign
//	go run ./scripts/signhelper -check EXE SIG     # verify with the built-in key
//	go run ./scripts/signhelper -manifest -version V -stamp N -base URL \
//	    -notes FILE arm64=ZIP amd64=ZIP > update.json  # the app's update manifest
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/appupdate"
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
	manifest := flag.Bool("manifest", false, "write the app's signed update manifest for ARCH=ZIP...")
	version := flag.String("version", "", "manifest: the release's version")
	base := flag.String("base", "", "manifest: the URL the zips are downloaded from, without the file name")
	page := flag.String("url", "", "manifest: the release page")
	notes := flag.String("notes", "", "manifest: a markdown file of release notes")
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
	if *manifest {
		if flag.NArg() == 0 || *stamp <= 0 || *version == "" || *base == "" {
			return errors.New("usage: -manifest -version V -stamp N -base URL [-url PAGE] [-notes FILE] ARCH=ZIP...")
		}
		key, err := readKey(*keyPath)
		if err != nil {
			return err
		}
		m := appupdate.Manifest{Version: strings.TrimPrefix(*version, "v"), Stamp: *stamp, URL: *page, Assets: map[string]appupdate.Asset{}}
		if *notes != "" {
			b, err := os.ReadFile(*notes)
			if err != nil {
				return err
			}
			m.Notes = string(b)
		}
		for _, arg := range flag.Args() {
			arch, zip, ok := strings.Cut(arg, "=")
			if !ok {
				return fmt.Errorf("%q is not ARCH=ZIP", arg)
			}
			b, err := os.ReadFile(zip)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			m.Assets[arch] = appupdate.Asset{
				URL:    strings.TrimSuffix(*base, "/") + "/" + filepath.Base(zip),
				Size:   int64(len(b)),
				SHA256: hex.EncodeToString(sum[:]),
			}
		}
		out, err := appupdate.Sign(key, m)
		if err != nil {
			return err
		}
		if _, err := appupdate.Open(out); err != nil {
			return fmt.Errorf("the key is not the built-in update key: %w", err)
		}
		_, err = os.Stdout.Write(append(out, '\n'))
		return err
	}
	if flag.NArg() != 1 || *stamp <= 0 {
		return errors.New("usage: -stamp N EXE")
	}
	key, err := readKey(*keyPath)
	if err != nil {
		return err
	}
	exe, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		return err
	}
	sig, err := updatesig.Sign(key, exe, *stamp)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(sig)
	return err
}

func readKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("bad key file " + path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
