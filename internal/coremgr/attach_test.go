package coremgr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

type attachRunner struct {
	ctl    runtimecfg.Controller
	exited chan error
}

func (r *attachRunner) Attach() (runtimecfg.Controller, <-chan error, error) {
	return r.ctl, r.exited, nil
}
func (*attachRunner) Start(string, string, runtimecfg.Controller, func(string)) (<-chan error, error) {
	return nil, errors.New("attach must not start a replacement core")
}
func (*attachRunner) Stop() error               { return nil }
func (*attachRunner) Test(string, string) error { return nil }

func TestAttachAuthenticatesAndReportsServiceDeath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" || r.Header.Get("Authorization") != "Bearer private-service-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer server.Close()
	runner := &attachRunner{runtimecfg.Controller{Addr: strings.TrimPrefix(server.URL, "http://"), Secret: "private-service-secret"}, make(chan error, 1)}
	manager := New(runner, "unused", "unused")
	changed := make(chan struct{}, 4)
	manager.OnChange(func() { changed <- struct{}{} })
	attached, err := manager.Attach(context.Background())
	if err != nil || !attached || manager.Client() == nil {
		t.Fatalf("attach: %v, %v", attached, err)
	}
	<-changed
	if manager.Controller() != runner.ctl {
		t.Fatal("did not adopt service controller")
	}
	runner.exited <- errors.New("binder died")
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("service death not reported")
	}
	if status, message := manager.Status(); status != Crashed || message != "binder died" || manager.Client() != nil {
		t.Fatalf("death: %s %s", status, message)
	}
	manager = New(runner, "unused", "unused")
	manager.OnChange(func() { changed <- struct{}{} })
	if ok, err := manager.Attach(context.Background()); !ok || err != nil {
		t.Fatal(err)
	}
	<-changed
	runner.exited <- ErrServiceStopped
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("intentional stop not reported")
	}
	if status, _ := manager.Status(); status != Stopped {
		t.Fatalf("intentional stop: %s", status)
	}
	runner.ctl.Secret = "wrong"
	manager = New(runner, "unused", "unused")
	if ok, err := manager.Attach(context.Background()); ok || err == nil || manager.Client() != nil {
		t.Fatal("adopted unauthenticated controller")
	}
}
