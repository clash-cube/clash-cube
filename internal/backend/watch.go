package backend

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/sysproxy"
)

// Event is something that happened, for the events list and, when Notify
// is set, a notification. Text is English, with {name}s filled from Args,
// so each page can translate it.
type Event struct {
	ID     int               `json:"id"`
	Time   time.Time         `json:"time"`
	Kind   string            `json:"kind"`  // core | proxy | network | profile | group
	Level  string            `json:"level"` // info | warning | error
	Text   string            `json:"text"`
	Args   map[string]string `json:"args,omitempty"`
	Notify bool              `json:"notify"`
}

const maxEvents = 200

// ResetText is the event of the network being reset, on which whoever
// shows the connectivity measures it again.
const ResetText = "Closed connections and flushed DNS"

func (b *Backend) event(kind, level, text string, args map[string]string, notify bool) {
	b.mu.Lock()
	b.eventSeq++
	e := Event{ID: b.eventSeq, Time: time.Now(), Kind: kind, Level: level, Text: text, Args: args, Notify: notify}
	if len(b.events) >= maxEvents {
		b.events = append(b.events[:0], b.events[1:]...)
	}
	b.events = append(b.events, e)
	b.mu.Unlock()
	if b.sink != nil {
		b.sink.Event(e)
	}
}

// Events is the recent events, oldest first.
func (b *Backend) Events() []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Event{}, b.events...)
}

// ClearEvents empties the events list.
func (b *Backend) ClearEvents() {
	b.mu.Lock()
	b.events = nil
	b.mu.Unlock()
}

// The machine, as variables so tests can stand in for it.
var (
	proxyEffective = sysproxy.Effective
	networkKey     = primaryNetwork
)

const watchEvery = 3 * time.Second

// watch follows the network, the system proxy and the automatic groups
// for as long as the app runs.
func (b *Backend) watch() {
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	last := time.Now().Round(0) // the wall clock, which runs on in sleep
	network := networkKey()
	misses := 0
	for i := 1; ; i++ {
		<-t.C
		now := time.Now().Round(0)
		if now.Sub(last) > watchEvery+30*time.Second {
			b.Woke()
		}
		last = now
		if k := networkKey(); k != network {
			network = k
			b.networkChanged(k)
		}
		b.checkProxy(&misses, network != "")
		if i%4 == 0 {
			b.checkGroups()
		}
	}
}

// primaryNetwork is the primary interface and its router, as
// "en0 192.168.1.1"; "" when offline. A tunnel (TUN's) is not a network
// change, so it reads as the network under it.
func primaryNetwork() string {
	cmd := exec.Command("/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader("show State:/Network/Global/IPv4\n")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseNetwork(string(out))
}

func parseNetwork(out string) string {
	var iface, router string
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, " : ")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "PrimaryInterface":
			iface = strings.TrimSpace(v)
		case "Router":
			router = strings.TrimSpace(v)
		}
	}
	if iface == "" {
		return ""
	}
	if strings.HasPrefix(iface, "utun") {
		if p := physicalInterface(); p != "" {
			iface, router = p, gateway()
		}
	}
	return strings.TrimSpace(iface + " " + router)
}

func (b *Backend) networkChanged(key string) {
	if key == "" {
		b.event("network", "info", "Network unavailable", nil, false)
		return
	}
	iface, router, _ := strings.Cut(key, " ")
	name := iface
	if svc := serviceOf(iface); svc != "" {
		name = svc + " (" + iface + ")"
	}
	b.event("network", "info", "Network changed to {name}, router {router}", map[string]string{"name": name, "router": router}, false)
	b.scheduleReset()
}

// Woke is the Mac waking from sleep.
func (b *Backend) Woke() {
	b.mu.Lock()
	again := time.Since(b.woke) < time.Minute
	b.woke = time.Now()
	b.mu.Unlock()
	if again {
		return
	}
	b.event("network", "info", "Woke from sleep", nil, false)
	b.scheduleReset()
}

// scheduleReset resets the core's network state once things settle: a
// wake and the network change after it are one reset.
func (b *Backend) scheduleReset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.resetTimer != nil {
		b.resetTimer.Stop()
	}
	b.resetTimer = time.AfterFunc(2*time.Second, b.resetNetwork)
}

// resetNetwork drops what the old network left behind: connections bound
// to it, which would hang until they time out, and its DNS answers. The
// system proxy is set again for a service that just came up.
func (b *Backend) resetNetwork() {
	c := b.core.Client()
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.FlushDNS(ctx)
	_ = c.CloseAllConnections(ctx)
	s := settings.Load()
	b.mu.Lock()
	owned := b.proxyOwned
	b.mu.Unlock()
	if s.SystemProxy && owned && !proxyEffective(proxyHost, s.MixedPort) {
		_ = b.applyProxy(true)
	}
	b.event("network", "info", ResetText, nil, false)
}

// checkProxy notices the system proxy taken by another app: twice in a
// row, so our own setting it, service by service, isn't taken for that.
// Without a network there is no proxy in effect to compare.
func (b *Backend) checkProxy(misses *int, online bool) {
	s := settings.Load()
	b.mu.Lock()
	owned := b.proxyOwned
	b.mu.Unlock()
	if !s.SystemProxy || !owned || !online || proxyEffective(proxyHost, s.MixedPort) {
		*misses = 0
		return
	}
	if *misses++; *misses < 2 {
		return
	}
	*misses = 0
	b.mu.Lock()
	b.proxyOwned, b.proxyLost = false, true
	b.mu.Unlock()
	b.event("proxy", "warning", "Another app changed the system proxy", nil, true)
	b.emitState()
}

// Groups that pick their proxy themselves.
var autoGroups = map[string]bool{"URLTest": true, "Fallback": true, "Smart": true}

// checkGroups notes an automatic group moving to another proxy.
func (b *Backend) checkGroups() {
	c := b.core.Client()
	if c == nil {
		b.mu.Lock()
		b.groupNow = nil
		b.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	proxies, err := c.Proxies(ctx)
	if err != nil {
		return
	}
	now := map[string]string{}
	for name, p := range proxies {
		if autoGroups[p.Type] && p.Now != "" {
			now[name] = p.Now
		}
	}
	key := settings.Load().Profile
	b.mu.Lock()
	prev := b.groupNow
	if b.groupClient != c || b.groupProfile != key {
		prev = nil // another core or profile: nothing moved
	}
	b.groupNow, b.groupClient, b.groupProfile = now, c, key
	b.mu.Unlock()
	for _, sw := range switches(prev, now) {
		b.event("group", "info", "{group} switched from {from} to {to}", map[string]string{"group": sw[0], "from": sw[1], "to": sw[2]}, false)
	}
}

// switches is the groups whose proxy changed, as [group, from, to].
func switches(prev, now map[string]string) [][3]string {
	var out [][3]string
	for g, to := range now {
		if from, ok := prev[g]; ok && from != to {
			out = append(out, [3]string{g, from, to})
		}
	}
	return out
}
