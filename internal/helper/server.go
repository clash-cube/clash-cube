//go:build darwin

package helper

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/localhost-copilot/clashcube/internal/updatesig"
)

// Main is the helper role: `clashcube helper serve --uid N --data DIR`.
func Main(args []string, version string) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: clashcube helper serve --uid N --data DIR [--socket PATH]")
	}
	fs := flag.NewFlagSet("helper", flag.ContinueOnError)
	uid := fs.Int("uid", -1, "the user allowed to connect")
	data := fs.String("data", "", "that user's ClashCube data directory")
	sock := fs.String("socket", SocketPath, "socket path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *uid < 0 || *data == "" {
		return errors.New("helper: --uid and --data are required")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s := &server{uid: *uid, data: filepath.Clean(*data), version: version, self: exe, key: updatesig.Key(), stamp: updatesig.OwnStamp(), exit: os.Exit}
	return s.serve(*sock)
}

type server struct {
	uid     int
	data    string
	version string

	// for update: the executable to replace, the key builds are signed
	// with, this build's stamp, and how to end (launchd starts us again)
	self  string
	key   ed25519.PublicKey
	stamp int64
	exit  func(int)

	mu  sync.Mutex
	cur *running
}

type running struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func exeHash() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *server) serve(path string) error {
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	// only the user (and root) may even reach it; the peer check below is
	// what is relied on
	if os.Geteuid() == 0 {
		_ = os.Chown(path, s.uid, -1)
	}
	_ = os.Chmod(path, 0o600)
	log.Printf("helper %s listening on %s for uid %d", s.version, path, s.uid)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		s.stop()
		l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(c.(*net.UnixConn))
	}
}

// peerUID is the uid of the process at the other end of c.
func peerUID(c *net.UnixConn) (int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var cerr error
	err = raw.Control(func(fd uintptr) {
		var cred *unix.Xucred
		cred, cerr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if cerr == nil {
			uid = int(cred.Uid)
		}
	})
	if err != nil {
		return -1, err
	}
	return uid, cerr
}

func (s *server) handle(c *net.UnixConn) {
	defer c.Close()
	enc := json.NewEncoder(c)
	uid, err := peerUID(c)
	if err != nil || (uid != s.uid && uid != 0) {
		_ = enc.Encode(Response{Error: "not allowed"})
		log.Printf("refused a connection from uid %d (%v)", uid, err)
		return
	}
	rd := bufio.NewReader(c)
	line, err := rd.ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		_ = enc.Encode(Response{Error: "bad request"})
		return
	}
	switch req.Op {
	case "version":
		_ = enc.Encode(Response{OK: true, Version: s.version, Hash: exeHash()})
	case "stop":
		s.stop()
		_ = enc.Encode(Response{OK: true})
	case "start":
		s.start(req, c, rd, enc)
	case "update":
		if err := s.update(req.Path); err != nil {
			log.Printf("refused an update from %s: %v", req.Path, err)
			_ = enc.Encode(Response{Error: err.Error()})
			return
		}
		log.Printf("updated from %s; restarting", req.Path)
		_ = enc.Encode(Response{OK: true})
		c.Close()
		s.stop()
		s.exit(0)
	default:
		_ = enc.Encode(Response{Error: "unknown op"})
	}
}

// checkPaths keeps the core to the user's own runtime configuration: the
// helper runs nothing else, and reads nowhere else.
func (s *server) checkPaths(home, config string) error {
	wantHome := filepath.Join(s.data, "core")
	if filepath.Clean(home) != wantHome || filepath.Clean(config) != filepath.Join(wantHome, "runtime.yaml") {
		return fmt.Errorf("paths outside %s", wantHome)
	}
	for _, p := range []string{s.data, wantHome, config} {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", p)
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != s.uid && st.Uid != 0 {
			return fmt.Errorf("%s is not the user's", p)
		}
	}
	return nil
}

func (s *server) start(req Request, c *net.UnixConn, rd *bufio.Reader, enc *json.Encoder) {
	if err := s.checkPaths(req.Home, req.Config); err != nil {
		_ = enc.Encode(Response{Error: err.Error()})
		return
	}
	if host, _, err := net.SplitHostPort(req.Ctl); err != nil || host != "127.0.0.1" {
		_ = enc.Encode(Response{Error: "controller must be on 127.0.0.1"})
		return
	}
	s.stop() // one core at a time

	exe, err := os.Executable()
	if err != nil {
		_ = enc.Encode(Response{Error: err.Error()})
		return
	}
	cmd := exec.Command(exe, "core", "-d", req.Home, "-f", req.Config, "-ext-ctl", req.Ctl)
	cmd.Env = []string{"CLASHCUBE_SECRET=" + req.Secret, "HOME=" + filepath.Dir(s.data), "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = enc.Encode(Response{Error: err.Error()})
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		_ = enc.Encode(Response{Error: err.Error()})
		return
	}
	r := &running{cmd: cmd, done: make(chan struct{})}
	s.mu.Lock()
	s.cur = r
	s.mu.Unlock()

	var wmu sync.Mutex
	send := func(v any) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = enc.Encode(v)
	}
	send(Response{OK: true})

	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			send(Event{Line: sc.Text()})
		}
	}()
	go func() {
		r.err = cmd.Wait()
		s.giveBack(req.Home)
		close(r.done)
	}()

	// the client going away, or asking, stops the core
	stopped := make(chan struct{})
	go func() {
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				break
			}
			var q Request
			if json.Unmarshal(line, &q) == nil && q.Op == "stop" {
				break
			}
		}
		close(stopped)
	}()
	select {
	case <-stopped:
		s.stopRunning(r)
	case <-r.done:
	}
	<-r.done
	msg := ""
	if r.err != nil {
		msg = r.err.Error()
	}
	send(Event{Exit: &msg})
	s.mu.Lock()
	if s.cur == r {
		s.cur = nil
	}
	s.mu.Unlock()
}

func (s *server) stop() {
	s.mu.Lock()
	r := s.cur
	s.mu.Unlock()
	if r != nil {
		s.stopRunning(r)
	}
}

func (s *server) stopRunning(r *running) {
	select {
	case <-r.done:
		return
	default:
	}
	_ = r.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		_ = r.cmd.Process.Kill()
		<-r.done
	}
}

// giveBack hands what the root core wrote in home (cache.db, providers,
// geo databases) back to the user, so the user-mode core can use it again.
func (s *server) giveBack(home string) {
	if os.Geteuid() != 0 {
		return
	}
	_ = filepath.Walk(home, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid == 0 {
			_ = os.Lchown(p, s.uid, int(st.Gid))
		}
		return nil
	})
}

const maxExe = 512 << 20

// update replaces the helper with the app's executable at path, if a key
// this build trusts signed it and it is newer. The bytes checked are the
// bytes installed: path is read once, into memory.
func (s *server) update(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}
	exe, err := s.readOwn(path, maxExe)
	if err != nil {
		return err
	}
	sig, err := s.readOwn(updatesig.SigPath(path), 64<<10)
	if err != nil {
		return err
	}
	stamp, err := updatesig.Verify(s.key, exe, sig)
	if err != nil {
		return err
	}
	if stamp <= s.stamp {
		return errors.New("not newer than the helper")
	}
	tmp := s.self + ".new"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := f.Write(exe); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.self); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// readOwn reads a regular file, not a symlink, of the user's or root's,
// and at most limit bytes.
func (s *server) readOwn(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != s.uid && st.Uid != 0 {
		return nil, fmt.Errorf("%s is not the user's", path)
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("%s is too large", path)
	}
	return io.ReadAll(io.LimitReader(f, limit))
}
