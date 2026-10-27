package router

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

type preference struct {
	Enabled bool         `json:"enabled"`
	Access  AccessPolicy `json:"access"`
}

func readPreference(path string) (preference, error) {
	var p preference
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		p.Access, err = p.Access.normalized()
		return p, err
	}
	if err == nil {
		err = json.Unmarshal(body, &p)
	}
	if err == nil {
		p.Access, err = p.Access.normalized()
	}
	return p, err
}

// FirewallControl is the private interface to the independently running guard.
// Pause is a bounded configuration transaction: a crashed management process
// cannot leave interception suspended indefinitely.
type FirewallControl interface {
	Get() (Status, error)
	Pause() (Status, error)
	Resume() (Status, error)
	Reconcile() (Status, error)
}

// Guard lives with the core supervisor, never with the web management process.
// It owns every firewall mutation and keeps working when management is offline.
type Guard struct {
	mu          sync.Mutex
	path        string
	client      func() *mihomoapi.Client
	discover    func() (lan, error)
	fw          firewall
	dns         dnsForwarder
	status      Status
	pausedUntil time.Time
	cleared     bool
	changed     chan struct{}
}

func NewGuard(home string, client func() *mihomoapi.Client) *Guard {
	return &Guard{path: filepath.Join(home, "router.json"), client: client,
		discover: discoverLAN, fw: firewall{exec: run}, dns: dnsForwarder{exec: run, path: dnsState(home)}, changed: make(chan struct{}, 1)}
}

func (g *Guard) Get() (Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.status, nil
}

func (g *Guard) Notify() {
	select {
	case g.changed <- struct{}{}:
	default:
	}
}

func (g *Guard) Watch(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.changed:
		case <-ticker.C:
		}
		_, _ = g.Reconcile()
	}
}

func (g *Guard) clear() error {
	var firewallErr error
	if !g.cleared {
		firewallErr = g.fw.clear()
		if firewallErr == nil {
			g.cleared = true
		}
	}
	g.status.Active = false
	g.status.Interface = ""
	// A missing/broken netfilter backend must not prevent restoring DNS after
	// a rejected activation or core failure. Keep retrying both independently.
	return errors.Join(firewallErr, g.dns.restore())
}

func (g *Guard) Pause() (Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pausedUntil = time.Now().Add(time.Minute)
	err := g.clear()
	return g.status, err
}

func (g *Guard) Resume() (Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pausedUntil = time.Time{}
	return g.reconcile()
}

func (g *Guard) Reconcile() (Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.reconcile()
}

func (g *Guard) reconcile() (Status, error) {
	p, err := readPreference(g.path)
	g.status.Enabled, g.status.Access, g.status.Error = p.Enabled, p.Access, ""
	client := g.client()
	if err == nil && (!p.Enabled || client == nil || time.Now().Before(g.pausedUntil)) {
		err = g.clear()
	} else if err == nil {
		var l lan
		l, err = g.discover()
		if err == nil {
			err = ready(client, l)
		}
		if err == nil {
			err = g.dns.apply(l)
		}
		if err == nil {
			err = dnsReady()
		}
		if err == nil {
			g.cleared = false
			err = g.fw.apply(l, p.Access)
		}
		if err == nil {
			g.status.Active, g.status.Interface = true, l.Device
		}
	}
	if err != nil {
		err = errors.Join(err, g.clear())
		g.status.Error = err.Error()
	}
	return g.status, err
}

func ready(c *mihomoapi.Client, l lan) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ports, err := c.Listening(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(ports, redirectPort) || !slices.Contains(ports, dnsPort) {
		return errors.New("LAN proxy listeners are not ready")
	}
	// A changed LAN address needs a validated core reload before reattachment.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(l.Address, strconv.Itoa(dnsPort)), time.Second)
	if err == nil {
		conn.Close()
	}
	return err
}

// Cleanup runs after the supervisor has stopped, including a forced procd kill.
func Cleanup() error {
	f := firewall{exec: run}
	d := dnsForwarder{exec: run, path: dnsState(appdir.Root())}
	return errors.Join(f.clear(), d.restore())
}
