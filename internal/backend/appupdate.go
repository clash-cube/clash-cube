package backend

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appupdate"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// How often the app looks for a release of itself, and how long after it
// opens it first does.
const (
	appUpdateEvery = 6 * time.Hour
	appUpdateFirst = 30 * time.Second
)

// AppUpdate is where updating the app stands, for Settings and the tray.
type AppUpdate struct {
	// idle | checking | latest | downloading | ready | installed | available | source | error
	State   string `json:"state"`
	Latest  string `json:"latest,omitempty"`
	Notes   string `json:"notes,omitempty"`
	URL     string `json:"url,omitempty"`   // the release page
	Stuck   string `json:"stuck,omitempty"` // available: why it can't replace itself
	Error   string `json:"error,omitempty"`
	Done    int64  `json:"done,omitempty"` // downloading: bytes so far, of total (0 when unknown)
	Total   int64  `json:"total,omitempty"`
	Checked int64  `json:"checked,omitempty"` // unix ms of the last answer
}

type appUpdater struct {
	mu     sync.Mutex
	st     AppUpdate
	staged string // the unpacked app, waiting for Install
	bundle string // the .app to replace; "" when not in one or stuck
}

// updateClient asks GitHub through the core's mixed port when something
// listens there, as the other downloads do.
func updateClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = func(*http.Request) (*url.URL, error) {
		if port := settings.Load().MixedPort; listening(port) {
			return &url.URL{Scheme: "http", Host: net.JoinHostPort(proxyHost, strconv.Itoa(port))}, nil
		}
		return nil, nil
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Minute}
}

func updateCache() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "com.localhost-copilot.clashcube")
	}
	return filepath.Join(os.TempDir(), "clashcube")
}

// AppUpdate is the update's state.
func (b *Backend) AppUpdate() AppUpdate {
	b.upd.mu.Lock()
	defer b.upd.mu.Unlock()
	return b.upd.st
}

// appUpdateLoop looks for a release at start and then every few hours,
// while the settings allow it and the network isn't metered.
func (b *Backend) appUpdateLoop() {
	select {
	case <-time.After(appUpdateFirst):
	case <-b.done:
		return
	}
	for {
		if settings.Load().AutoUpdateApp && !b.SavingData() {
			b.CheckAppUpdate()
		}
		select {
		case <-time.After(appUpdateEvery):
		case <-b.done:
			return
		}
	}
}

// CheckAppUpdate asks for the newest release and, when it is newer and
// the app may replace itself, starts downloading it.
func (b *Backend) CheckAppUpdate() AppUpdate {
	u := &b.upd
	u.mu.Lock()
	// installed: the bundle is the new version already, and opens next time
	if u.st.State == "checking" || u.st.State == "downloading" || u.st.State == "installed" {
		defer u.mu.Unlock()
		return u.st
	}
	prev := u.st
	u.st = AppUpdate{State: "checking", Latest: prev.Latest, Notes: prev.Notes, URL: prev.URL, Checked: prev.Checked}
	u.mu.Unlock()
	b.emitState()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	m, err := appupdate.Latest(ctx, updateClient())
	cancel()

	u.mu.Lock()
	now := time.Now().UnixMilli()
	switch {
	case err != nil && u.staged != "":
		u.st = prev // what was downloaded is still there to install
	case err != nil:
		u.st = AppUpdate{State: "error", Error: err.Error(), Checked: prev.Checked}
	case u.staged != "" && prev.Latest == m.Version:
		u.st = prev
		u.st.Checked = now
	default:
		u.st = AppUpdate{Latest: m.Version, Notes: m.Notes, URL: m.URL, Checked: now}
		switch {
		case !appupdate.Released(b.Version):
			u.st.State = "source"
		case !appupdate.Newer(m.Version, b.Version):
			u.st.State = "latest"
		case u.bundle == "":
			u.st.State = "available"
			u.st.Stuck = appupdate.Stuck(appupdate.Bundle())
		default:
			u.st.State = "downloading"
			u.staged = ""
			go b.stageAppUpdate(m)
		}
	}
	st := u.st
	u.mu.Unlock()
	b.emitState()
	return st
}

func (b *Backend) stageAppUpdate(m *appupdate.Manifest) {
	u := &b.upd
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-b.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	dir := appupdate.StageDir(u.bundle, updateCache())
	staged, err := appupdate.Stage(ctx, updateClient(), m, runtime.GOARCH, dir, func(done, total int64) {
		// not emitted: the tray menu is rebuilt from every state, and the
		// page polls the progress while it shows it
		u.mu.Lock()
		u.st.Done, u.st.Total = done, total
		u.mu.Unlock()
	})
	u.mu.Lock()
	u.st.Done, u.st.Total = 0, 0
	if err != nil {
		log.Println("app update:", err)
		u.st.State, u.st.Error = "error", err.Error()
	} else {
		u.st.State, u.staged = "ready", staged
	}
	u.mu.Unlock()
	b.emitState()
	if err == nil {
		b.event("app", "info", "ClashCube {version} is ready; restart to update", map[string]string{"version": m.Version}, true)
	}
}

// InstallAppUpdate puts the downloaded version in place of the app. With
// ask it may ask for the administrator's password (prompt), where the
// app's folder needs one; without, such a folder waits for the next time.
// The caller then restarts the app.
func (b *Backend) InstallAppUpdate(ask bool, prompt string) error {
	u := &b.upd
	b.installMu.Lock()
	defer b.installMu.Unlock()
	// not under u.mu: the password prompt may stay up a while, and the
	// state is read meanwhile
	u.mu.Lock()
	staged, bundle := u.staged, u.bundle
	u.mu.Unlock()
	if staged == "" {
		return errors.New("no update is ready")
	}
	err := appupdate.Install(staged, bundle)
	if ask && appupdate.NeedsAdmin(err) {
		err = appupdate.InstallAsAdmin(staged, bundle, prompt)
	}
	if err != nil {
		if !errors.Is(err, appupdate.ErrCanceled) {
			log.Println("app update:", err)
		}
		return err
	}
	u.mu.Lock()
	u.staged = ""
	u.st.State = "installed"
	u.mu.Unlock()
	b.emitState()
	return nil
}

// AppBundle is the .app an installed update goes into.
func (b *Backend) AppBundle() string {
	b.upd.mu.Lock()
	defer b.upd.mu.Unlock()
	return b.upd.bundle
}
