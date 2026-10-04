package backend

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/localhost-copilot/mihomobar/internal/appdir"
	"github.com/localhost-copilot/mihomobar/internal/netpath"
	"github.com/localhost-copilot/mihomobar/internal/profiles"
	"github.com/localhost-copilot/mihomobar/internal/settings"
	"github.com/localhost-copilot/mihomobar/internal/wifi"
)

// netMachine stands in for the Wi-Fi, the path and the helper.
type netMachine struct {
	wifi   wifi.Status
	path   netpath.Path
	helper bool
}

func networkBackend(t *testing.T) (*Backend, profiles.Profile, profiles.Profile, *netMachine) {
	t.Helper()
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	fake.reset()
	if err := appdir.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := profiles.AddDefault([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/second.yaml"
	if err := os.WriteFile(path, []byte(second), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := profiles.ImportFile(path, "Office")
	if err != nil {
		t.Fatal(err)
	}
	s := settings.Defaults()
	s.Profile, s.MixedPort, s.NetworkAuto = first.ID, freePort(t), true
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	m := &netMachine{wifi: wifi.Status{State: "connected", SSID: "Home", Interface: "en0"}}
	ow, op, oh := readWiFi, readPath, helperInstalled
	readWiFi = func() wifi.Status { return m.wifi }
	readPath = func() netpath.Path { return m.path }
	helperInstalled = func() (bool, bool) { return m.helper, m.helper }
	t.Cleanup(func() { readWiFi, readPath, helperInstalled = ow, op, oh })
	b := New("test", "test", []byte(base), nil)
	t.Cleanup(b.Shutdown)
	return b, first, next, m
}

// settle samples the network twice, as two watcher ticks.
func settle(b *Backend, primary string) {
	b.checkNetwork(primary)
	b.checkNetwork(primary)
}

func on(v bool) *bool { return &v }

func TestNetworkRulesReturnToBaseline(t *testing.T) {
	b, _, _, m := networkBackend(t)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if err := b.SetSystemProxy(true); err != nil {
		t.Fatal(err)
	}
	s, err := b.SetNetworkRules([]settings.NetworkRule{
		{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Mode: "direct", SystemProxy: on(false)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// the other networks' rule is added, last, with what the Mac had
	other := s.NetworkRules[len(s.NetworkRules)-1]
	if len(s.NetworkRules) != 2 || other.Match != "other" || other.Actions.Mode != "rule" || other.Actions.SystemProxy == nil || !*other.Actions.SystemProxy {
		t.Fatalf("baseline: %+v", s.NetworkRules)
	}

	b.checkNetwork("en0 192.168.1.1")
	if settings.Load().Mode != "rule" {
		t.Fatal("applied before the network settled")
	}
	b.checkNetwork("en0 192.168.1.1")
	if st := settings.Load(); st.Mode != "direct" || st.SystemProxy {
		t.Fatalf("home not applied: %s %v", st.Mode, st.SystemProxy)
	}
	if on, _ := fake.state(); on {
		t.Fatal("system proxy still set at home")
	}
	if n := b.State().Network; n.Match != "ssid:Home" || n.Kind != "wifi" {
		t.Fatalf("state: %+v", n)
	}

	// an unmatched network returns to the baseline, not to what home set
	m.wifi.SSID = "Cafe"
	settle(b, "en0 10.1.1.1")
	if st := settings.Load(); st.Mode != "rule" || !st.SystemProxy {
		t.Fatalf("cafe kept home's settings: %s %v", st.Mode, st.SystemProxy)
	}
	if on, _ := fake.state(); !on {
		t.Fatal("system proxy not set again")
	}

	// an unreadable name for a moment is not a network change
	m.wifi = wifi.Status{State: "unavailable", Interface: "en0"}
	settle(b, "en0 10.1.1.1")
	m.wifi = wifi.Status{State: "connected", SSID: "Cafe", Interface: "en0"}
	b.checkNetwork("en0 10.1.1.1")
	if events := b.Events(); len(events) != 2 {
		t.Fatalf("events: %+v", events)
	}
}

// Another Wi-Fi on the same interface and router address is a network
// change the watcher can't see; a name only now readable isn't one.
func TestWiFiSwitchBehindSameRouter(t *testing.T) {
	b, _, _, m := networkBackend(t)
	changes := func() int {
		n := 0
		for _, e := range b.Events() {
			if strings.HasPrefix(e.Text, "Network changed to") {
				n++
			}
		}
		return n
	}
	settle(b, "en0 192.168.1.1")
	if changes() != 0 {
		t.Fatal("the first network counted as a change")
	}
	m.wifi.SSID = "Office"
	settle(b, "en0 192.168.1.1")
	if changes() != 1 {
		t.Fatalf("switch not seen: %+v", b.Events())
	}
	b.mu.Lock()
	scheduled := b.resetTimer != nil
	b.mu.Unlock()
	if !scheduled {
		t.Fatal("no reset scheduled")
	}
	m.wifi = wifi.Status{State: "permission", Interface: "en0"}
	settle(b, "en0 192.168.1.1")
	m.wifi = wifi.Status{State: "connected", SSID: "Office", Interface: "en0"}
	settle(b, "en0 192.168.1.1")
	if changes() != 1 {
		t.Fatalf("a name becoming readable counted as a switch: %+v", b.Events())
	}
}

func TestNetworkRulesManualChanges(t *testing.T) {
	b, _, _, m := networkBackend(t)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SetNetworkRules([]settings.NetworkRule{
		{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Mode: "direct", SystemProxy: on(true)}},
	}); err != nil {
		t.Fatal(err)
	}
	settle(b, "en0 192.168.1.1")
	if err := b.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	if n := b.State().Network; !slices.Equal(n.Manual, []string{"mode"}) {
		t.Fatalf("manual: %v", n.Manual)
	}
	// the rest still follows the rule
	if err := b.setSystemProxy(false); err != nil {
		t.Fatal(err)
	}
	b.ResumeNetworkAuto()
	if st := settings.Load(); st.Mode != "direct" || !st.SystemProxy {
		t.Fatalf("resume: %s %v", st.Mode, st.SystemProxy)
	}

	if err := b.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	settle(b, "en0 192.168.1.1")
	if settings.Load().Mode != "global" {
		t.Fatal("the same network undid a manual change")
	}
	// set back to the rule's value, it follows the rule again
	if err := b.SetMode("direct"); err != nil {
		t.Fatal(err)
	}
	if n := b.State().Network; len(n.Manual) != 0 {
		t.Fatalf("manual: %v", n.Manual)
	}
	if err := b.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	// a disconnection, then the same network again, is a new connection
	m.wifi = wifi.Status{State: "disconnected", Interface: "en0"}
	settle(b, "")
	m.wifi = wifi.Status{State: "connected", SSID: "Home", Interface: "en0"}
	settle(b, "en0 192.168.1.1")
	if settings.Load().Mode != "direct" || len(b.State().Network.Manual) != 0 {
		t.Fatal("reconnecting kept the manual change")
	}
}

func TestNetworkRulesProfileAndGroups(t *testing.T) {
	b, first, next, m := networkBackend(t)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	client := b.core.Client()
	if _, err := b.SetNetworkRules([]settings.NetworkRule{
		{Match: "wired", Actions: settings.NetworkActions{Profile: next.ID, Groups: map[string]string{"Choose": "hk"}}},
	}); err != nil {
		t.Fatal(err)
	}
	// wired: the primary interface isn't the Wi-Fi one, whatever the Wi-Fi
	settle(b, "en7 10.0.0.1")
	if settings.Load().Profile != next.ID || b.core.Client() != client {
		t.Fatal("profile not reloaded in place")
	}
	all, err := client.Proxies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if all["Choose"].Now != "hk" {
		t.Fatalf("group: %s", all["Choose"].Now)
	}
	if err := b.SelectProxy("Choose", "DIRECT"); err != nil {
		t.Fatal(err)
	}
	if n := b.State().Network; n.Kind != "wired" || !slices.Equal(n.Manual, []string{"group:Choose"}) {
		t.Fatalf("state: %+v", n)
	}
	// a restart keeps the manual choice; a new network takes the rule's
	if err := b.Restart(); err != nil {
		t.Fatal(err)
	}
	all, _ = b.core.Client().Proxies(context.Background())
	if all["Choose"].Now != "DIRECT" {
		t.Fatal("restart undid the manual choice")
	}

	m.wifi = wifi.Status{State: "connected", SSID: "Cafe", Interface: "en0"}
	settle(b, "en0 10.1.1.1")
	if settings.Load().Profile != first.ID {
		t.Fatal("other networks did not return to the first profile")
	}
	// back on the wire, with the Wi-Fi gone
	m.wifi.State = "disconnected"
	settle(b, "en7 10.0.0.1")
	if settings.Load().Profile != next.ID {
		t.Fatal("wired not applied again")
	}
}

func TestNetworkRulesNeverPromptOrStart(t *testing.T) {
	b, _, _, m := networkBackend(t)
	if _, err := b.SetNetworkRules([]settings.NetworkRule{
		{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Mode: "direct", Tun: on(true)}},
	}); err != nil {
		t.Fatal(err)
	}
	settle(b, "en0 192.168.1.1")
	st := settings.Load()
	if st.Mode != "direct" || st.Tun || b.core.Client() != nil {
		t.Fatalf("mode %s, tun %v, core %v", st.Mode, st.Tun, b.core.Client() != nil)
	}
	events := b.Events()
	if len(events) != 1 || events[0].Level != "warning" || !strings.Contains(events[0].Args["error"], "helper") {
		t.Fatalf("events: %+v", events)
	}
	// with the helper, a stopped core takes TUN when it next starts
	m.helper = true
	b.ResumeNetworkAuto()
	if st := settings.Load(); !st.Tun || !st.ServiceMode || b.core.Client() != nil {
		t.Fatal("TUN not set for the next start")
	}
}

func TestNetworkRulesChecked(t *testing.T) {
	b, first, next, _ := networkBackend(t)
	for _, rules := range [][]settings.NetworkRule{
		{{Match: "ssid", SSID: ""}},
		{{Match: "ssid", SSID: strings.Repeat("网", 11)}},
		{{Match: "ssid", SSID: "Home"}, {Match: "ssid", SSID: "Home"}},
		{{Match: "wired"}, {Match: "wired"}},
		{{Match: "nearby"}},
		{{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Profile: "missing"}}},
		{{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Mode: "fast"}}},
	} {
		if _, err := b.SetNetworkRules(rules); err == nil {
			t.Fatalf("accepted %+v", rules)
		}
	}
	if len(settings.Load().NetworkRules) != 0 {
		t.Fatal("a refused edit was saved")
	}
	s, err := b.SetNetworkRules([]settings.NetworkRule{
		{Match: "other"},
		{Match: "wired", SSID: "x"},
		{Match: "ssid", SSID: "Home", Actions: settings.NetworkActions{Profile: next.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, r := range s.NetworkRules {
		keys = append(keys, ruleKey(r))
	}
	if !slices.Equal(keys, []string{"ssid:Home", "wired", "other"}) || s.NetworkRules[2].Actions.Profile != first.ID {
		t.Fatalf("order or baseline: %+v", s.NetworkRules)
	}
	// a removed profile doesn't hold up editing the rest
	if err := b.RemoveProfile(next.ID); err != nil {
		t.Fatal(err)
	}
	s.NetworkRules[1].Actions.Mode = "direct"
	if _, err := b.SetNetworkRules(s.NetworkRules); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SetNetworkAuto(false); err != nil {
		t.Fatal(err)
	}
}

func TestSavingData(t *testing.T) {
	b, _, _, m := networkBackend(t)
	b.checkNetwork("en0 172.20.10.1")
	if b.SavingData() {
		t.Fatal("saving on an ordinary network")
	}
	m.path.Expensive = true
	b.checkNetwork("en0 172.20.10.1")
	if !b.SavingData() || !b.State().Network.SavingData {
		t.Fatal("not saving on a hotspot")
	}
	if _, err := settings.Update(func(s *settings.Settings) { s.SaveData = false }); err != nil {
		t.Fatal(err)
	}
	b.checkNetwork("en0 172.20.10.1")
	if b.SavingData() {
		t.Fatal("saving with SaveData off")
	}
}
