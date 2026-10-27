package routerupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

type Job struct {
	Phase   string `json:"phase"`
	Version string `json:"version,omitempty"`
	PID     int    `json:"-"`
	Started int64  `json:"started,omitempty"`
	Error   string `json:"error,omitempty"`
}

// jobFile retains the worker pid privately, without exposing it in browser RPCs.
type jobFile struct {
	Job
	WorkerPID int `json:"workerPID"`
}
type checkedFeed struct {
	URL      string          `json:"url"`
	Envelope json.RawMessage `json:"envelope"`
}

type State struct {
	Installed string `json:"installed"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	Bytes     int64  `json:"bytes,omitempty"`
	Job       Job    `json:"job"`
}

type command func(context.Context, string, ...string) ([]byte, error)

type Manager struct {
	dir    string
	key    ed25519.PublicKey
	client *http.Client
	exec   command
}

func New(dir string) *Manager {
	return &Manager{dir: dir, key: updatesig.Key(), client: &http.Client{Timeout: 5 * time.Minute}, exec: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}}
}

func (m *Manager) save(name string, value any) error {
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return err
	}
	body, err := json.Marshal(value)
	if err == nil {
		err = os.WriteFile(filepath.Join(m.dir, name)+".tmp", body, 0600)
	}
	if err == nil {
		err = os.Rename(filepath.Join(m.dir, name)+".tmp", filepath.Join(m.dir, name))
	}
	return err
}

func (m *Manager) job() Job {
	var saved jobFile
	if body, err := os.ReadFile(filepath.Join(m.dir, "job.json")); err == nil {
		_ = json.Unmarshal(body, &saved)
	}
	if saved.Phase == "" {
		saved.Phase = "idle"
	}
	saved.Job.PID = saved.WorkerPID
	return saved.Job
}

func (m *Manager) saveJob(j Job) error { return m.save("job.json", jobFile{Job: j, WorkerPID: j.PID}) }

func (m *Manager) cached() (checkedFeed, Manifest, error) {
	var c checkedFeed
	body, err := os.ReadFile(filepath.Join(m.dir, "checked.json"))
	if err == nil {
		err = json.Unmarshal(body, &c)
	}
	if err != nil {
		return c, Manifest{}, err
	}
	manifest, err := Open(c.Envelope, m.key)
	return c, manifest, err
}

func (m *Manager) installed(ctx context.Context) (string, error) {
	body, err := m.exec(ctx, "opkg", "list-installed", "clashcube-core")
	if err != nil {
		return "", errors.New("cannot read the installed core package")
	}
	name, version, ok := strings.Cut(strings.TrimSpace(string(body)), " - ")
	if !ok || name != "clashcube-core" || !versionPattern.MatchString(version) {
		return "", errors.New("install clashcube-core with opkg before using core updates")
	}
	return version, nil
}

func (m *Manager) State() (State, error) {
	if job := m.job(); job.Phase == "queued" || job.Phase == "downloading" || job.Phase == "installing" {
		m.busy()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	installed, err := m.installed(ctx)
	s := State{Installed: installed, Job: m.job()}
	if err != nil {
		// opkg can temporarily remove the status entry while replacing the
		// package. Keep progress visible during that expected installation gap.
		if s.Job.Phase == "queued" || s.Job.Phase == "downloading" || s.Job.Phase == "installing" {
			return s, nil
		}
		return s, err
	}
	_, manifest, err := m.cached()
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	p, _ := manifest.Core()
	s.Latest, s.Bytes = manifest.Version, p.Bytes
	// opkg treats ">" as greater-or-equal; ">>" is strictly newer.
	_, err = m.exec(ctx, "opkg", "compare-versions", s.Latest, ">>", installed)
	s.Available = err == nil
	return s, nil
}

func (m *Manager) busy() bool {
	lock, err := os.Stat(filepath.Join(m.dir, "busy"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	j := m.job()
	// Apply reserves the lock before it can read opkg state and publish its
	// queued job. A concurrent request must not reap that fresh reservation.
	active := j.Phase == "queued" || j.Phase == "downloading" || j.Phase == "installing"
	if !active && time.Since(lock.ModTime()) >= 0 && time.Since(lock.ModTime()) < 30*time.Second {
		return true
	}
	if (j.PID != 0 && workerAlive(j.PID)) || (j.Phase == "queued" && time.Now().Unix()-j.Started >= 0 && time.Now().Unix()-j.Started < 30) {
		return true
	}
	// A killed worker cannot keep the UI locked forever after management restarts.
	j.Phase, j.Error = "failed", "Core update was interrupted"
	_ = m.saveJob(j)
	_ = os.Remove(filepath.Join(m.dir, "busy"))
	return false
}

func (m *Manager) Check(feed string) (State, error) {
	if err := ValidateURL(feed); err != nil {
		return State{}, err
	}
	if m.busy() {
		return State{}, errors.New("a core update is already in progress")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feed, nil)
	if err != nil {
		return State{}, errors.New("invalid core update feed")
	}
	response, err := m.client.Do(request)
	if err != nil {
		return State{}, errors.New("cannot fetch the core update manifest")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || response.StatusCode != http.StatusOK || len(body) > 1<<20 {
		return State{}, errors.New("cannot read the core update manifest")
	}
	if _, err := Open(body, m.key); err != nil {
		return State{}, err
	}
	if err := m.save("checked.json", checkedFeed{URL: feed, Envelope: body}); err != nil {
		return State{}, err
	}
	return m.State()
}

func (m *Manager) Apply(feed string) (State, error) {
	if m.busy() {
		return State{}, errors.New("a core update is already in progress")
	}
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return State{}, err
	}
	lock, err := os.OpenFile(filepath.Join(m.dir, "busy"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return State{}, errors.New("a core update is already in progress")
	}
	lock.Close()
	queued := false
	defer func() {
		if !queued {
			_ = os.Remove(filepath.Join(m.dir, "busy"))
		}
	}()
	cached, _, err := m.cached()
	if err != nil || cached.URL != feed {
		return State{}, errors.New("check the configured update feed before updating")
	}
	s, err := m.State()
	if err != nil {
		return s, err
	}
	if !s.Available {
		return s, errors.New("there is no newer core package to install")
	}
	j := Job{Phase: "queued", Version: s.Latest, Started: time.Now().Unix()}
	if err := m.saveJob(j); err != nil {
		return s, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.exec(ctx, "/etc/init.d/clashcube-update", "start"); err != nil {
		j.Phase, j.Error = "failed", "Cannot start the core update worker"
		_ = m.saveJob(j)
		return s, errors.New(j.Error)
	}
	queued = true
	s.Job = j
	return s, nil
}

// Run executes in a separate procd service. Stopping or upgrading management
// cannot terminate an in-progress opkg transaction. Inputs are fixed private
// files; no command, package URL or filesystem path is accepted from the CLI.
func (m *Manager) Run(ctx context.Context) (result error) {
	j := m.job()
	if j.Phase != "queued" {
		return errors.New("no queued core update")
	}
	j.PID, j.Phase = os.Getpid(), "downloading"
	if err := m.saveJob(j); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			j.Phase, j.Error = "failed", result.Error()
		} else {
			j.Phase, j.Error = "succeeded", ""
		}
		_ = m.saveJob(j)
		_ = os.Remove(filepath.Join(m.dir, "core.ipk"))
		_ = os.Remove(filepath.Join(m.dir, "busy"))
	}()
	_, manifest, err := m.cached()
	if err != nil {
		return errors.New("core update manifest validation failed")
	}
	if manifest.Version != j.Version {
		return errors.New("queued core version changed")
	}
	p, _ := manifest.Core()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.CoreURL(), nil)
	if err != nil {
		return errors.New("invalid core download URL")
	}
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("core package download failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("core package download failed: HTTP " + strconv.Itoa(response.StatusCode))
	}
	path := filepath.Join(m.dir, "core.ipk")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.New("cannot stage the core package in RAM")
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, p.Bytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != p.Bytes || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(p.SHA256) {
		return errors.New("core package checksum or size mismatch; installed core was not changed")
	}
	j.Phase = "installing"
	if err := m.saveJob(j); err != nil {
		return err
	}
	output, err := m.exec(ctx, "opkg", "install", path)
	if err != nil {
		_ = os.WriteFile(filepath.Join(m.dir, "opkg.log"), output, 0600)
		return errors.New("core package installation failed; check the private opkg log")
	}
	installed, err := m.installed(ctx)
	if err != nil || installed != manifest.Version {
		return errors.New("installed core version did not match the update")
	}
	return nil
}
