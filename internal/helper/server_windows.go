package helper

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

func Main(args []string, version string) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: helper serve --data DIR --pipe PIPE --parent PID")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("helper requires elevation")
	}
	fs := flag.NewFlagSet("helper", flag.ContinueOnError)
	data := fs.String("data", "", "user data directory")
	pipe := fs.String("pipe", "", "session pipe")
	parentID := fs.Uint("parent", 0, "GUI process ID")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !filepath.IsAbs(*data) || strings.HasPrefix(*data, `\\`) || !strings.HasPrefix(*pipe, `\\.\pipe\clashcube-`) || *parentID == 0 {
		return errors.New("invalid helper arguments")
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(*parentID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	var token windows.Token
	if err := windows.OpenProcessToken(parent, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	// The pipe ACL is the IPC authorization boundary: only the launching user,
	// SYSTEM and administrators can connect. Remote pipe clients are rejected by winio.
	l, err := winio.ListenPipe(*pipe, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + user.User.Sid.String() + ")"})
	if err != nil {
		return err
	}
	defer l.Close()
	s := &windowsServer{data: filepath.Clean(*data), version: version, hash: selfHash(), close: l.Close}
	defer s.runner.Stop()
	go func() {
		windows.WaitForSingleObject(parent, windows.INFINITE)
		l.Close()
		s.runner.Stop()
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			return nil
		}
		go s.handle(conn)
	}
}

type windowsServer struct {
	data, version, hash string
	close               func() error
	mu                  sync.Mutex
	runner              coremgr.LocalRunner
}

// Only the fixed runtime config under the data directory authorized by UAC is
// accepted. Reject reparse points in every component, including the data root.
func validateWindowsStart(data string, req Request) error {
	home := filepath.Join(data, "core")
	config := filepath.Join(home, "runtime.yaml")
	if !strings.EqualFold(filepath.Clean(req.Home), home) || !strings.EqualFold(filepath.Clean(req.Config), config) {
		return errors.New("helper refuses paths outside the runtime configuration")
	}
	for path := config; ; path = filepath.Dir(path) {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attrs, err := windows.GetFileAttributes(p)
		if err != nil {
			return err
		}
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("helper refuses reparse points")
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	fi, err := os.Stat(config)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("runtime configuration is not a regular file")
	}
	host, port, err := net.SplitHostPort(req.Ctl)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || host != "127.0.0.1" || n < 1 || n > 65535 || len(req.Secret) < 32 {
		return errors.New("invalid core controller")
	}
	return nil
}

func (s *windowsServer) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(io.LimitReader(conn, 64<<10))
	var req Request
	if decoder.Decode(&req) != nil {
		return
	}
	var writeMu sync.Mutex
	write := func(value any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = json.NewEncoder(conn).Encode(value)
	}
	switch req.Op {
	case "version":
		write(Response{OK: true, Version: s.version, Hash: s.hash})
	case "stop":
		s.mu.Lock()
		defer s.mu.Unlock()
		err := s.runner.Stop()
		write(Response{OK: err == nil})
		if s.close != nil {
			_ = s.close()
		}
	case "start":
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := validateWindowsStart(s.data, req); err != nil {
			write(Response{Error: err.Error()})
			return
		}
		// Serialize the initial response before output, as the client expects it first.
		writeMu.Lock()
		done, err := s.runner.Start(req.Home, req.Config, runtimecfg.Controller{Addr: req.Ctl, Secret: req.Secret}, func(line string) { write(Event{Line: line}) })
		if err != nil {
			writeMu.Unlock()
			write(Response{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(Response{OK: true})
		writeMu.Unlock()
		conn.SetReadDeadline(time.Time{})
		defer s.runner.Stop()
		readerDone := make(chan struct{})
		go func() {
			defer close(readerDone)
			// The session owns its core. Disconnect, malformed input, or stop all
			// end it; no second command may redirect the privileged process.
			var stop Request
			_ = decoder.Decode(&stop)
			s.runner.Stop()
		}()
		err = <-done
		message := ""
		if err != nil {
			message = err.Error()
		}
		write(Event{Exit: &message})
		conn.Close()
		// Finish the session's stop before another start can acquire s.mu.
		<-readerDone
	default:
		write(Response{Error: fmt.Sprintf("unsupported helper operation %q", req.Op)})
	}
}
