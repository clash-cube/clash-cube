package coreservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/router"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

// Client is both the management-side core runner and the private firewall client.
// Closing it cancels observation only; stopping the core is an explicit RPC.
type Client struct {
	ctx       context.Context
	cancel    context.CancelFunc
	http      *http.Client
	transport *http.Transport
}

func NewClient() *Client { return newClient(Socket) }

func newClient(socket string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	return &Client{ctx: ctx, cancel: cancel, transport: transport, http: &http.Client{Transport: transport}}
}

func (c *Client) Close() {
	c.cancel()
	c.transport.CloseIdleConnections()
}

func (c *Client) call(method string, args []any, out any) error {
	if args == nil {
		args = []any{}
	}
	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(c.ctx, 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/rpc/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var result struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
			return fmt.Errorf("core service: HTTP %d", resp.StatusCode)
		}
		return errors.New(result.Error)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func (c *Client) Status() (Snapshot, error) {
	var status Snapshot
	err := c.call("Core/Status", nil, &status)
	return status, err
}

func (c *Client) Start(_, _ string, ctl runtimecfg.Controller, _ func(string)) (<-chan error, error) {
	if err := c.call("Core/Start", []any{ctl}, nil); err != nil {
		return nil, err
	}
	return c.Observe(ctl), nil
}

func (c *Client) Stop() error            { return c.call("Core/Stop", nil, nil) }
func (c *Client) Test(_, _ string) error { return c.call("Core/Test", nil, nil) }

// Observe follows the identity of one core instance, without owning its lifetime.
func (c *Client) Observe(ctl runtimecfg.Controller) <-chan error {
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				done <- c.ctx.Err()
				return
			case <-ticker.C:
			}
			status, err := c.Status()
			if err == nil && (status.Status != coremgr.Running || status.Controller != ctl) {
				err = errors.New("supervised core stopped or changed")
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()
	return done
}

type firewallClient struct{ c *Client }

func (c *Client) Firewall() router.FirewallControl { return firewallClient{c} }
func (f firewallClient) request(method string) (router.Status, error) {
	var status router.Status
	err := f.c.call("Firewall/"+method, nil, &status)
	return status, err
}
func (f firewallClient) Get() (router.Status, error)       { return f.request("Get") }
func (f firewallClient) Pause() (router.Status, error)     { return f.request("Pause") }
func (f firewallClient) Resume() (router.Status, error)    { return f.request("Resume") }
func (f firewallClient) Reconcile() (router.Status, error) { return f.request("Reconcile") }
