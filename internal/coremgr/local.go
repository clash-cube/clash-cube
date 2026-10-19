//go:build !android

package coremgr

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

// LocalRunner runs the core as a child of this process, as this user.
type LocalRunner struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan error
	keep io.WriteCloser // our end of the core's stdin; closing it (or dying) ends the core
}

func (l *LocalRunner) Test(home, config string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "core", "-t", "-d", home, "-f", config)
	configureProcess(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(lastMeaningful(string(out), err))
	}
	return nil
}

func lastMeaningful(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			l = strings.TrimPrefix(l, "clashcube: ")
			return l
		}
	}
	return err.Error()
}

func (l *LocalRunner) Start(home, config string, ctl runtimecfg.Controller, output func(string)) (<-chan error, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd != nil {
		return nil, errors.New("core already running")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "core", "-d", home, "-f", config, "-ext-ctl", ctl.Addr)
	cmd.Env = append(os.Environ(), "CLASHCUBE_SECRET="+ctl.Secret, "CLASHCUBE_WATCH_STDIN=1")
	configureProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		stdin.Close()
		pr.Close()
		pw.Close()
		return nil, err
	}
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			output(sc.Text())
		}
	}()
	done := make(chan error, 1)
	l.cmd, l.done, l.keep = cmd, done, stdin
	go func() {
		err := cmd.Wait()
		pw.Close()
		l.mu.Lock()
		if l.cmd == cmd {
			l.cmd = nil
		}
		l.mu.Unlock()
		done <- err
		close(done)
	}()
	return done, nil
}

func (l *LocalRunner) Stop() error {
	l.mu.Lock()
	cmd, done, keep := l.cmd, l.done, l.keep
	l.mu.Unlock()
	if cmd == nil {
		return nil
	}
	// EOF also works on Windows, where os.Process.Signal cannot send SIGTERM.
	_ = keep.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	return nil
}
