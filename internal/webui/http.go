// Package webui exposes the app to an authenticated LuCI bridge. It never
// listens on TCP; the root-only Unix socket is the trust boundary.
package webui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type handler struct {
	methods map[string]any
	events  *Broker
}

// NewRPCHandler serves an explicit private-service allowlist using the same
// strict argument decoding as the LuCI bridge. It has no event endpoint.
func NewRPCHandler(methods map[string]any) http.Handler { return handler{methods: methods} }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		reply(w, 405, nil, errors.New("POST required"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	defer r.Body.Close()
	if r.URL.Path == "/events" {
		if h.events == nil {
			reply(w, 404, nil, errors.New("unknown method"))
			return
		}
		var c cursor
		if err := decode(r.Body, &c); err != nil {
			reply(w, 400, nil, err)
			return
		}
		reply(w, 200, h.events.Poll(r.Context(), c), nil)
		return
	}
	method, ok := h.methods[strings.TrimPrefix(r.URL.Path, "/rpc/")]
	if !strings.HasPrefix(r.URL.Path, "/rpc/") || !ok {
		reply(w, 404, nil, errors.New("unknown method"))
		return
	}
	var args []json.RawMessage
	if err := decode(r.Body, &args); err != nil {
		reply(w, 400, nil, err)
		return
	}
	fn := reflect.ValueOf(method)
	if len(args) != fn.Type().NumIn() {
		reply(w, 400, nil, errors.New("incorrect argument count"))
		return
	}
	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		p := reflect.New(fn.Type().In(i))
		if err := decode(bytes.NewReader(arg), p.Interface()); err != nil {
			reply(w, 400, nil, fmt.Errorf("argument %d: %w", i+1, err))
			return
		}
		in[i] = p.Elem()
	}
	out := fn.Call(in)
	var value any
	if len(out) > 0 && out[len(out)-1].Type().Implements(reflect.TypeFor[error]()) {
		e := out[len(out)-1]
		if !e.IsNil() {
			reply(w, 422, nil, e.Interface().(error))
			return
		}
		out = out[:len(out)-1]
	}
	if len(out) > 0 {
		value = out[0].Interface()
	}
	reply(w, 200, value, nil)
}

func decode(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}

func reply(w http.ResponseWriter, status int, value any, err error) {
	var body []byte
	if err != nil {
		body, _ = json.Marshal(map[string]string{"error": err.Error()})
	} else {
		body, err = json.Marshal(value)
	}
	if err != nil && body == nil {
		status = 500
		body = []byte(`{"error":"could not encode response"}`)
	}
	// The Lua bridge reads to EOF. An explicit length avoids chunked framing.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// Listen creates a private socket and refuses to replace a live server or a
// non-socket file. Stale sockets after a power loss are safe to remove.
func Listen(path string) (*net.UnixListener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("socket path is not a socket")
		}
		c, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			c.Close()
			return nil, errors.New("ClashCube is already running")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
