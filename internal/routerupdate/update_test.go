package routerupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type updateFixture struct {
	manager      *Manager
	manifest     Manifest
	signed       []byte
	data         []byte
	corrupt      bool
	installed    string
	installs     int
	installError bool
	key          ed25519.PrivateKey
}

func fixture(t *testing.T) *updateFixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &updateFixture{manager: New(t.TempDir()), data: []byte("verified package fixture"), installed: "1.0.0-1", key: key}
	f.manager.key = pub
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed.json" {
			_, _ = w.Write(f.signed)
			return
		}
		if f.corrupt {
			_, _ = w.Write([]byte("corrupt package"))
			return
		}
		_, _ = w.Write(f.data)
	}))
	t.Cleanup(server.Close)
	f.manager.client = server.Client()
	sum := sha256.Sum256(f.data)
	f.manifest = Manifest{Version: "1.1.0-1", BaseURL: server.URL, Packages: []Package{{Name: "clashcube-core", File: "clashcube-core_1.1.0-1_aarch64_generic.ipk", Bytes: int64(len(f.data)), InstalledBytes: 1024, SHA256: hex.EncodeToString(sum[:])}}}
	f.signed, err = Sign(key, f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	f.manager.exec = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "/etc/init.d/clashcube-update":
			return nil, nil
		case name == "opkg" && args[0] == "list-installed":
			return []byte("clashcube-core - " + f.installed + "\n"), nil
		case name == "opkg" && args[0] == "compare-versions":
			// Model opkg's non-strict ">" too, so current-version rejection
			// exercises the installed tool's comparison semantics.
			if args[1] == f.installed && args[2] == ">" {
				return nil, nil
			}
			if args[1] == f.manifest.Version && f.installed != f.manifest.Version {
				return nil, nil
			}
			return nil, errors.New("not newer")
		case name == "opkg" && args[0] == "install":
			data, err := os.ReadFile(args[1])
			if err != nil || string(data) != string(f.data) {
				t.Error("installer received unverified data")
			}
			f.installs++
			if f.installError {
				return []byte("private-opkg-credential"), errors.New("install failed")
			}
			f.installed = f.manifest.Version
			return nil, nil
		default:
			t.Errorf("unexpected update command: %s %v", name, args)
			return nil, errors.New("unexpected command")
		}
	}
	return f
}

func (f *updateFixture) feed() string { return f.manifest.BaseURL + "/feed.json" }

func TestRouterManifestRejectsTamperingAndOtherSigningDomains(t *testing.T) {
	f := fixture(t)
	if _, err := Open(f.signed, f.manager.key); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(f.signed), "1.1.0-1", "9.9.9-9", 1)
	if _, err := Open([]byte(changed), f.manager.key); err == nil {
		t.Fatal("accepted a tampered manifest")
	}
	var e envelope
	if err := json.Unmarshal(f.signed, &e); err != nil {
		t.Fatal(err)
	}
	e.Sig = ed25519.Sign(f.key, append([]byte("clashcube-app-update\n"), e.Manifest...))
	body, _ := json.Marshal(e)
	if _, err := Open(body, f.manager.key); err == nil {
		t.Fatal("accepted a desktop signature as a router signature")
	}
	f.manifest.Packages[0].File = "../untrusted.ipk"
	if _, err := Sign(f.key, f.manifest); err == nil {
		t.Fatal("accepted an arbitrary package path")
	}
}

func TestCoreUpdateRejectsCorruptDownloadBeforeInstallationAndCanRetry(t *testing.T) {
	f := fixture(t)
	s, err := f.manager.Check(f.feed())
	if err != nil || !s.Available {
		t.Fatal("valid release unavailable", err)
	}
	f.corrupt = true
	if _, err := f.manager.Apply(f.feed()); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Run(context.Background()); err == nil {
		t.Fatal("accepted a corrupt download")
	}
	if f.installs != 0 || f.installed != "1.0.0-1" {
		t.Fatal("corrupt download changed the installed core")
	}
	// A new management process observes the failed worker through private state.
	reconnected := New(f.manager.dir)
	reconnected.key, reconnected.exec = f.manager.key, f.manager.exec
	s, err = reconnected.State()
	if err != nil || s.Job.Phase != "failed" {
		t.Fatal("worker failure was lost after management restart", err)
	}
	f.corrupt = false
	if _, err := f.manager.Apply(f.feed()); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err = reconnected.State()
	if err != nil || s.Available || s.Job.Phase != "succeeded" || s.Installed != f.manifest.Version || f.installs != 1 {
		t.Fatal("verified update did not complete", err)
	}
	if _, err := f.manager.Apply(f.feed()); err == nil {
		t.Fatal("reinstalled a current version")
	}
	if _, err := os.Stat(filepath.Join(f.manager.dir, "core.ipk")); !os.IsNotExist(err) {
		t.Fatal("finished update kept its RAM staging file")
	}
}

func TestApplySerializesConcurrentJobsAndRejectsFeedChanges(t *testing.T) {
	f := fixture(t)
	if _, err := f.manager.Check(f.feed()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.Apply(f.feed() + "?changed=1"); err == nil {
		t.Fatal("installed from a feed the user had not checked")
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() { defer wait.Done(); _, err := f.manager.Apply(f.feed()); results <- err }()
	}
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("queued %d concurrent updates, want one", success)
	}
	if _, err := f.manager.Check(f.feed()); err == nil {
		t.Fatal("replaced a manifest during a queued transaction")
	}
}

func TestAbandonedWorkerIsRecoverableAndPrivateLogsStayPrivate(t *testing.T) {
	f := fixture(t)
	if _, err := f.manager.Check(f.feed()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.Apply(f.feed()); err != nil {
		t.Fatal(err)
	}
	j := f.manager.job()
	j.Started = time.Now().Add(-time.Minute).Unix()
	if err := f.manager.saveJob(j); err != nil {
		t.Fatal(err)
	}
	s, err := f.manager.State()
	if err != nil || s.Job.Phase != "failed" {
		t.Fatal("dead worker left the updater locked", err)
	}
	f.installError = true
	if _, err := f.manager.Apply(f.feed()); err != nil {
		t.Fatal(err)
	}
	if err := f.manager.Run(context.Background()); err == nil {
		t.Fatal("accepted a failed opkg transaction")
	}
	s, _ = f.manager.State()
	body, _ := json.Marshal(s)
	if strings.Contains(string(body), "private-opkg-credential") || strings.Contains(string(body), "workerPID") {
		t.Fatal("browser state leaked private worker data")
	}
	fi, err := os.Stat(filepath.Join(f.manager.dir, "opkg.log"))
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("private opkg log permissions", err)
	}
}

func TestConcurrentApplyCannotStealAReservedJobBeforeItIsPublished(t *testing.T) {
	f := fixture(t)
	if _, err := f.manager.Check(f.feed()); err != nil {
		t.Fatal(err)
	}
	exec := f.manager.exec
	reserved, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	f.manager.exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "opkg" && args[0] == "list-installed" && blocked.CompareAndSwap(false, true) {
			close(reserved)
			<-release
		}
		return exec(ctx, name, args...)
	}
	first := make(chan error, 1)
	go func() { _, err := f.manager.Apply(f.feed()); first <- err }()
	<-reserved
	_, secondErr := f.manager.Apply(f.feed())
	close(release)
	firstErr := <-first
	if secondErr == nil || firstErr != nil {
		t.Fatal("concurrent Apply stole a reserved transaction", firstErr, secondErr)
	}
}
