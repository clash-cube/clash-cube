package helper

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/runtimecfg"
)

func dial(sock string) (*net.UnixConn, error) {
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return c.(*net.UnixConn), nil
}

func call(sock string, req Request) (Response, error) {
	c, err := dial(sock)
	if err != nil {
		return Response{}, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(c).Decode(&resp); err != nil {
		return Response{}, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// Status asks the installed helper for its version and executable hash.
func Status() (Response, error) { return call(SocketPath, Request{Op: "version"}) }

// Runner runs the core through the helper, as root: coremgr's service mode.
type Runner struct {
	Socket string // SocketPath if empty
	// Test runs configuration checks, which need no root.
	Test_ func(home, config string) error

	mu   sync.Mutex
	conn *net.UnixConn
	done chan error
}

func (r *Runner) sock() string {
	if r.Socket != "" {
		return r.Socket
	}
	return SocketPath
}

func (r *Runner) Test(home, config string) error { return r.Test_(home, config) }

func (r *Runner) Start(home, config string, ctl runtimecfg.Controller, output func(string)) (<-chan error, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		return nil, errors.New("core already running")
	}
	c, err := dial(r.sock())
	if err != nil {
		return nil, errors.New("the privileged helper is not running: " + err.Error())
	}
	enc := json.NewEncoder(c)
	if err := enc.Encode(Request{Op: "start", Home: home, Config: config, Ctl: ctl.Addr, Secret: ctl.Secret}); err != nil {
		c.Close()
		return nil, err
	}
	rd := bufio.NewReader(c)
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := rd.ReadBytes('\n')
	if err != nil {
		c.Close()
		return nil, err
	}
	_ = c.SetReadDeadline(time.Time{})
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil || !resp.OK {
		c.Close()
		if resp.Error != "" {
			return nil, errors.New("helper: " + resp.Error)
		}
		return nil, errors.New("helper: bad answer")
	}
	done := make(chan error, 1)
	r.conn, r.done = c, done
	go func() {
		var exit error = errors.New("helper connection lost")
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				break
			}
			var ev Event
			if json.Unmarshal(line, &ev) != nil {
				continue
			}
			if ev.Exit != nil {
				exit = nil
				if *ev.Exit != "" {
					exit = errors.New(*ev.Exit)
				}
				break
			}
			output(ev.Line)
		}
		r.mu.Lock()
		if r.conn == c {
			r.conn = nil
		}
		r.mu.Unlock()
		c.Close()
		done <- exit
		close(done)
	}()
	return done, nil
}

func (r *Runner) Stop() error {
	r.mu.Lock()
	c, done := r.conn, r.done
	r.mu.Unlock()
	if c == nil {
		return nil
	}
	_ = json.NewEncoder(c).Encode(Request{Op: "stop"})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.Close()
		<-done
	}
	return nil
}
