package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/localhost-copilot/clashferry/internal/helper"
	"github.com/localhost-copilot/clashferry/internal/mihomoapi"
	"github.com/localhost-copilot/clashferry/internal/netpath"
	"github.com/localhost-copilot/clashferry/internal/profiles"
	"github.com/localhost-copilot/clashferry/internal/settings"
	"github.com/localhost-copilot/clashferry/internal/wifi"
)

// The machine, as variables so tests can stand in for it.
var (
	readWiFi        = wifi.Current
	readPath        = netpath.Current
	helperInstalled = helper.Installed
)

// Network is the network the Mac is on and what the rules make of it
// (docs/design.md §10.3.3).
type Network struct {
	WiFi wifi.Status `json:"wifi"`
	Kind string      `json:"kind"` // wifi | wired | "" offline
	// the rule in effect, as ruleKey names it; "" offline or not settled
	Match string `json:"match"`
	// the fields the rule sets that were changed by hand since: profile,
	// mode, systemProxy, tun, group:<name>
	Manual      []string `json:"manual"`
	Expensive   bool     `json:"expensive"`
	Constrained bool     `json:"constrained"`
	// SaveData is on and the network is expensive or constrained
	SavingData bool `json:"savingData"`
}

// netWatch is what the network watcher keeps between samples; under mu.
type netWatch struct {
	seen      Network // the last sample
	candidate string  // the identity sampled last, settling
	samples   int
	identity  string // the settled network; a new one is applied and clears manual
	match     string // the settled network's rule
	manual    map[string]bool
}

// ruleKey names a rule as Network.Match does.
func ruleKey(r settings.NetworkRule) string {
	if r.Match == "ssid" {
		return "ssid:" + r.SSID
	}
	return r.Match
}

// matchRule is the rule for a network: its SSID's, else the wired rule on
// a wired primary interface, else the other networks' one.
func matchRule(rules []settings.NetworkRule, ssid string, wired bool) string {
	has := func(k string) bool {
		return slices.ContainsFunc(rules, func(r settings.NetworkRule) bool { return ruleKey(r) == k })
	}
	switch {
	case ssid != "" && has("ssid:"+ssid):
		return "ssid:" + ssid
	case wired && has("wired"):
		return "wired"
	}
	return "other"
}

// effective is what a rule sets, over the other networks' rule: that one is
// the baseline every field a rule sets returns to elsewhere.
func effective(rules []settings.NetworkRule, match string) settings.NetworkActions {
	var out settings.NetworkActions
	for _, k := range []string{"other", match} {
		for _, r := range rules {
			if ruleKey(r) != k {
				continue
			}
			a := r.Actions
			out.Profile = cmpOr(a.Profile, out.Profile)
			out.Mode = cmpOr(a.Mode, out.Mode)
			if a.SystemProxy != nil {
				out.SystemProxy = a.SystemProxy
			}
			if a.Tun != nil {
				out.Tun = a.Tun
			}
			for g, m := range a.Groups {
				if out.Groups == nil {
					out.Groups = map[string]string{}
				}
				out.Groups[g] = m
			}
		}
		if match == "other" {
			break
		}
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// manages says whether a sets field, and to what.
func manages(a settings.NetworkActions, field string) (string, bool) {
	switch {
	case field == "profile":
		return a.Profile, a.Profile != ""
	case field == "mode":
		return a.Mode, a.Mode != ""
	case field == "systemProxy" && a.SystemProxy != nil:
		return fmt.Sprint(*a.SystemProxy), true
	case field == "tun" && a.Tun != nil:
		return fmt.Sprint(*a.Tun), true
	case strings.HasPrefix(field, "group:"):
		m, ok := a.Groups[strings.TrimPrefix(field, "group:")]
		return m, ok
	}
	return "", false
}

// checkNetwork samples the network once; the watcher calls it every tick
// with the primary interface and router. A network counts once it reads
// the same twice in a row; a new one has its rule applied.
func (b *Backend) checkNetwork(primary string) {
	w, p := readWiFi(), readPath()
	iface, _, _ := strings.Cut(primary, " ")
	ssid := ""
	if w.State == "connected" {
		ssid = w.SSID
	}
	wired := iface != "" && iface != w.Interface
	n := Network{WiFi: w, Expensive: p.Expensive, Constrained: p.Constrained}
	switch {
	case wired:
		n.Kind = "wired"
	case iface != "" || ssid != "":
		n.Kind = "wifi"
	}
	s := settings.Load()
	n.SavingData = s.SaveData && (p.Expensive || p.Constrained)

	// on Wi-Fi with its name unreadable for now: not a network change
	transient := !wired && iface != "" && ssid == "" && w.State == "unavailable"
	identity := ""
	if n.Kind != "" {
		identity = ssid + "|" + primary
	}

	b.mu.Lock()
	nw := &b.net
	changed := fmt.Sprint(nw.seen) != fmt.Sprint(n)
	nw.seen = n
	apply, rejoined := false, false
	if !transient {
		if identity != nw.candidate {
			nw.candidate, nw.samples = identity, 0
		}
		nw.samples++
		if nw.samples == 2 && identity != nw.identity {
			// another Wi-Fi behind the same interface and router address:
			// the watcher sees no change, so it is reset from here. An SSID
			// that was unreadable before (no permission) isn't another network.
			i := strings.LastIndex(nw.identity, "|") // SSIDs may hold a "|"
			rejoined = ssid != "" && i > 0 && nw.identity[i+1:] == primary
			nw.identity, nw.manual = identity, nil
			nw.match = ""
			if identity != "" {
				nw.match = matchRule(s.NetworkRules, ssid, wired)
			}
			apply, changed = nw.match != "", true
		}
	}
	b.mu.Unlock()
	if changed {
		b.emitState()
	}
	if apply {
		b.applyNetwork()
	}
	if rejoined {
		iface, router, _ := strings.Cut(primary, " ")
		b.event("network", "info", "Network changed to {name}, router {router}", map[string]string{"name": ssid + " (" + iface + ")", "router": router}, false)
		b.scheduleReset()
	}
}

// network is Network for the GUI, with only the manual changes that still
// stand against the rule.
func (b *Backend) network(s settings.Settings) Network {
	b.mu.Lock()
	n, match, manual := b.net.seen, b.net.match, b.net.manual
	b.mu.Unlock()
	n.Manual = []string{}
	if !s.NetworkAuto {
		return n
	}
	n.Match = match
	a := effective(s.NetworkRules, match)
	for f := range manual {
		if _, ok := manages(a, f); ok {
			n.Manual = append(n.Manual, f)
		}
	}
	slices.Sort(n.Manual)
	return n
}

// SavingData says whether background traffic should wait.
func (b *Backend) SavingData() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.net.seen.SavingData
}

// noteManual records a change made by hand to something the rule in effect
// sets: it stands until the network changes, or the rule is applied again.
// Set back to the rule's value, it follows the rule again.
func (b *Backend) noteManual(field, value string) {
	s := settings.Load()
	if !s.NetworkAuto {
		return
	}
	b.mu.Lock()
	match := b.net.match
	b.mu.Unlock()
	want, ok := manages(effective(s.NetworkRules, match), field)
	if match == "" || !ok {
		return
	}
	b.mu.Lock()
	if want == value {
		delete(b.net.manual, field)
	} else {
		if b.net.manual == nil {
			b.net.manual = map[string]bool{}
		}
		b.net.manual[field] = true
	}
	b.mu.Unlock()
	b.emitState()
}

// applyNetwork sets what the rule in effect sets, except what was changed
// by hand: the profile first (a reload), then its groups, the mode, the
// system proxy and TUN. A failed step is reported and the rest still done.
// A stopped core is left stopped.
func (b *Backend) applyNetwork() {
	b.applyMu.Lock()
	defer b.applyMu.Unlock()
	s := settings.Load()
	b.mu.Lock()
	match, manual := b.net.match, maps.Clone(b.net.manual)
	shutdown := b.shutdown
	b.mu.Unlock()
	if !s.NetworkAuto || match == "" || shutdown {
		return
	}
	a := effective(s.NetworkRules, match)
	var errs []error
	changed := false
	if a.Profile != "" && !manual["profile"] && a.Profile != s.Profile {
		if _, ok := profiles.Get(a.Profile); !ok {
			errs = append(errs, errors.New("its profile no longer exists"))
		} else {
			b.opMu.Lock()
			err := b.useProfile(a.Profile)
			b.opMu.Unlock()
			errs, changed = append(errs, err), changed || err == nil
		}
	}
	c, err := b.applyGroups(a.Groups, manual)
	errs, changed = append(errs, err), changed || c
	if a.Mode != "" && !manual["mode"] && a.Mode != settings.Load().Mode {
		err := b.setMode(a.Mode)
		errs, changed = append(errs, err), changed || err == nil
	}
	if on := a.SystemProxy; on != nil && !manual["systemProxy"] && *on != settings.Load().SystemProxy {
		err := b.setSystemProxy(*on)
		errs, changed = append(errs, err), changed || err == nil
	}
	if on := a.Tun; on != nil && !manual["tun"] && *on != settings.Load().Tun {
		err := b.applyTun(*on)
		errs, changed = append(errs, err), changed || err == nil
	}

	where, args := "other networks", map[string]string{}
	switch {
	case match == "wired":
		where = "wired networks"
	case strings.HasPrefix(match, "ssid:"):
		where, args["ssid"] = "Wi-Fi {ssid}", strings.TrimPrefix(match, "ssid:")
	}
	if err := errors.Join(errs...); err != nil {
		log.Println("network rules:", err)
		args["error"] = strings.ReplaceAll(err.Error(), "\n", "; ")
		b.event("network", "warning", "Couldn't apply all of the settings for "+where+": {error}", args, true)
	} else if changed {
		b.event("network", "info", "Applied the settings for "+where, args, true)
	}
	b.emitState()
}

func ptr[T any](v T) *T { return &v }

// applyTun turns TUN on or off for a rule. It never asks for a password:
// without the helper it is left as it is.
func (b *Backend) applyTun(on bool) error {
	if on {
		if running, current := helperInstalled(); !running || !current {
			return errors.New("TUN needs the privileged helper; install it in Settings")
		}
	}
	if b.core.Client() == nil {
		_, err := settings.Update(func(s *settings.Settings) {
			s.Tun = on
			s.ServiceMode = s.ServiceMode || on
		})
		return err
	}
	return b.setTun(on, "")
}

// applyGroups selects each group's member, when the core runs; whether any
// moved.
func (b *Backend) applyGroups(groups map[string]string, manual map[string]bool) (bool, error) {
	c := b.core.Client()
	if c == nil || len(groups) == 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	all, err := c.Proxies(ctx)
	if err != nil {
		return false, err
	}
	var errs []error
	moved := false
	for _, g := range slices.Sorted(maps.Keys(groups)) {
		m := groups[g]
		if manual["group:"+g] {
			continue
		}
		p, ok := all[g]
		if !ok || p.Type != "Selector" || !slices.Contains(p.All, m) {
			errs = append(errs, fmt.Errorf("the profile has no group %s with %s", g, m))
			continue
		}
		if p.Now != m {
			err := selectProxy(c, g, m)
			errs, moved = append(errs, err), moved || err == nil
		}
	}
	return moved, errors.Join(errs...)
}

// followGroups selects the rule's members again after the core started or
// took another profile, whose groups start from their own choices.
func (b *Backend) followGroups() {
	s := settings.Load()
	b.mu.Lock()
	match, manual := b.net.match, maps.Clone(b.net.manual)
	b.mu.Unlock()
	if !s.NetworkAuto || match == "" {
		return
	}
	if _, err := b.applyGroups(effective(s.NetworkRules, match).Groups, manual); err != nil {
		log.Println("network rules:", err)
	}
}

// selectProxy picks a group's member, closing the group's connections so
// the switch shows at once.
func selectProxy(c *mihomoapi.Client, group, name string) error {
	ctx := context.Background()
	if err := c.Select(ctx, group, name); err != nil {
		return err
	}
	if conns, err := c.Connections(ctx); err == nil {
		for _, cn := range conns.Connections {
			if slices.Contains(cn.Chains, group) {
				_ = c.CloseConnection(ctx, cn.ID)
			}
		}
	}
	return nil
}

// SelectProxy picks a group's member by hand.
func (b *Backend) SelectProxy(group, name string) error {
	c, err := b.Client()
	if err != nil {
		return err
	}
	if err := selectProxy(c, group, name); err != nil {
		return err
	}
	b.noteManual("group:"+group, name)
	return nil
}

// SetNetworkAuto turns the network rules on or off; turned on, the rule for
// the network the Mac is on is applied at once.
func (b *Backend) SetNetworkAuto(on bool) (settings.Settings, error) {
	s, err := settings.Update(func(s *settings.Settings) { s.NetworkAuto = on })
	if err != nil {
		return s, err
	}
	b.ResumeNetworkAuto()
	return settings.Load(), nil
}

// ResumeNetworkAuto drops the manual changes and applies the rule again.
func (b *Backend) ResumeNetworkAuto() {
	b.mu.Lock()
	b.net.manual = nil
	b.mu.Unlock()
	b.applyNetwork()
	b.emitState()
}

// SetNetworkRules replaces the rules and applies the one in effect.
func (b *Backend) SetNetworkRules(rules []settings.NetworkRule) (settings.Settings, error) {
	before := settings.Load()
	rules, err := b.normalRules(before, rules)
	if err != nil {
		return before, err
	}
	if _, err := settings.Update(func(s *settings.Settings) { s.NetworkRules = rules }); err != nil {
		return before, err
	}
	b.mu.Lock()
	if b.net.identity != "" {
		ssid, _, _ := strings.Cut(b.net.identity, "|")
		b.net.match = matchRule(rules, ssid, b.net.seen.Kind == "wired")
	}
	b.mu.Unlock()
	b.ResumeNetworkAuto()
	return settings.Load(), nil
}

// normalRules checks rules and puts them in order: Wi-Fi networks as
// given, then wired, then the other networks' rule, which is always there.
// Whatever a rule sets, the other networks' rule sets too, to what the Mac
// has now, so that leaving a network always returns somewhere known.
func (b *Backend) normalRules(before settings.Settings, in []settings.NetworkRule) ([]settings.NetworkRule, error) {
	known := map[string]bool{}
	for _, r := range before.NetworkRules {
		known[r.Actions.Profile] = true
	}
	var ssids []settings.NetworkRule
	var wired, other *settings.NetworkRule
	seen := map[string]bool{}
	for _, r := range in {
		r.Actions.Groups = maps.Clone(r.Actions.Groups)
		switch r.Match {
		case "ssid":
			if r.SSID == "" || len(r.SSID) > 32 || !utf8.ValidString(r.SSID) {
				return nil, errors.New("Wi-Fi names must contain 1–32 UTF-8 bytes")
			}
		case "wired", "other":
			r.SSID = ""
		default:
			return nil, fmt.Errorf("unknown network %q", r.Match)
		}
		if seen[ruleKey(r)] {
			return nil, errors.New("Each network can have only one rule")
		}
		seen[ruleKey(r)] = true
		switch r.Actions.Mode {
		case "", "rule", "global", "direct":
		default:
			return nil, fmt.Errorf("unknown mode %q", r.Actions.Mode)
		}
		if p := r.Actions.Profile; p != "" && !known[p] {
			if _, ok := profiles.Get(p); !ok {
				return nil, errors.New("no such profile")
			}
		}
		for g, m := range r.Actions.Groups {
			if g == "" || m == "" {
				delete(r.Actions.Groups, g)
			}
		}
		if len(r.Actions.Groups) == 0 {
			r.Actions.Groups = nil
		}
		switch r.Match {
		case "ssid":
			ssids = append(ssids, r)
		case "wired":
			wired = &r
		default:
			other = &r
		}
	}
	if other == nil {
		other = &settings.NetworkRule{Match: "other"}
	}
	out := ssids
	if wired != nil {
		out = append(out, *wired)
	}

	// the baseline for each field some rule sets
	o := &other.Actions
	var now map[string]mihomoapi.Proxy
	if c := b.core.Client(); c != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		now, _ = c.Proxies(ctx)
		cancel()
	}
	for _, r := range out {
		a := r.Actions
		if a.Profile != "" && o.Profile == "" {
			o.Profile = before.Profile
		}
		if a.Mode != "" && o.Mode == "" {
			o.Mode = "rule"
			if a.Mode == "rule" {
				o.Mode = before.Mode
			}
		}
		if a.SystemProxy != nil && o.SystemProxy == nil {
			o.SystemProxy = ptr(!*a.SystemProxy)
		}
		if a.Tun != nil && o.Tun == nil {
			o.Tun = ptr(!*a.Tun)
		}
		for g := range a.Groups {
			if _, ok := o.Groups[g]; !ok && now[g].Now != "" {
				if o.Groups == nil {
					o.Groups = map[string]string{}
				}
				o.Groups[g] = now[g].Now
			}
		}
	}
	return append(out, *other), nil
}
