package devicepolicy

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixture() (map[string]any, Config) {
	raw := map[string]any{"proxies": []any{map[string]any{"name": "node", "type": "http", "server": "192.0.2.10", "port": 8080}}, "mode": "direct"}
	node := Catalog(raw, "profile", "/tmp")[0]
	c := Config{Revision: NewID(), LAN: LAN{Device: "br-lan", Address: "192.168.1.1", Prefix: "192.168.1.0/24"}, Policies: []Policy{{ID: NewID(), Name: "phone", MAC: "02:00:00:00:00:01", IP: "192.168.1.100", Slot: 1, Target: node.Target}}}
	return raw, c
}

func TestFixedAdapterSurvivesGlobalModeButRefusesIdentityChanges(t *testing.T) {
	for _, mode := range []string{"direct", "global", "rule"} {
		t.Run(mode, func(t *testing.T) {
			raw, c := fixture()
			raw["mode"] = mode
			if err := Overlay(raw, c, "profile", t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if !Loaded(raw, c.Policies[0]) {
				t.Fatal("device did not receive a pinned listener")
			}
			proxies := raw["proxies"].([]any)
			clone := proxies[1].(map[string]any)
			if clone["server"] != "192.0.2.10" || clone["name"] != c.Policies[0].Inbound() {
				t.Fatal("wrong pinned endpoint")
			}
		})
	}
	for _, change := range []string{"profile", "endpoint", "password", "paused", "missing"} {
		t.Run(change, func(t *testing.T) {
			raw, c := fixture()
			profile := "profile"
			switch change {
			case "profile":
				profile = "other"
			case "endpoint":
				raw["proxies"].([]any)[0].(map[string]any)["server"] = "192.0.2.11"
			case "password":
				raw["proxies"].([]any)[0].(map[string]any)["password"] = "changed"
			case "paused":
				c.Policies[0].Paused = true
			case "missing":
				delete(raw, "proxies")
			}
			if err := Overlay(raw, c, profile, t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if Loaded(raw, c.Policies[0]) {
				t.Fatal("changed identity or paused device remained routable")
			}
			for _, item := range raw["listeners"].([]any) {
				if item.(map[string]any)["proxy"] != "REJECT" {
					t.Fatal("missing node fell back instead of rejecting")
				}
			}
		})
	}
}

func TestPolicyStoreRejectsAmbiguousIdentity(t *testing.T) {
	_, c := fixture()
	home := t.TempDir()
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(home)
	if err != nil || loaded.Revision != c.Revision {
		t.Fatal("durable policy did not round trip", err)
	}
	for _, ip := range []string{"192.168.1.0", "192.168.1.255", "192.168.1.1", "192.168.2.100"} {
		c.Policies[0].IP = ip
		if c.Validate() == nil {
			t.Fatalf("accepted unsafe host %s", ip)
		}
	}
	_, c = fixture()
	duplicate := c.Policies[0]
	duplicate.ID = NewID()
	duplicate.Slot = 2
	c.Policies = append(c.Policies, duplicate)
	if c.Validate() == nil {
		t.Fatal("duplicate identity accepted")
	}
}

func TestFirewallVerificationRetainsExpressions(t *testing.T) {
	a := []byte(`{"nftables":[{"metainfo":{"version":"one"}},{"rule":{"handle":1,"expr":[{"drop":null}]}}]}`)
	b := []byte(`{"nftables":[{"metainfo":{"version":"two"}},{"rule":{"handle":99,"expr":[{"drop":null}]}}]}`)
	x, err := CanonicalShape(a)
	if err != nil {
		t.Fatal(err)
	}
	y, _ := CanonicalShape(b)
	if string(x) != string(y) {
		t.Fatal("metadata changed the firewall identity")
	}
	y, _ = CanonicalShape([]byte(strings.ReplaceAll(string(b), "drop", "accept")))
	if string(x) == string(y) {
		t.Fatal("changed verdict was not detected")
	}
}

func TestCatalogDoesNotExposeAdapterCredentials(t *testing.T) {
	raw, _ := fixture()
	raw["proxies"].([]any)[0].(map[string]any)["password"] = "private-credential"
	body, err := json.Marshal(Catalog(raw, "profile", t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "private-credential") || strings.Contains(string(body), "192.0.2.10") {
		t.Fatal("private adapter configuration escaped into the device catalog")
	}
}

func TestSharedProxyCannotBorrowManagementException(t *testing.T) {
	for _, port := range []any{443, "443", 80, 22, "442-444", "444-442", "1080/22,9999", "65558"} {
		raw, c := fixture()
		raw["listeners"] = []any{map[string]any{"name": "shared", "type": "mixed", "port": port}}
		if Overlay(raw, c, "profile", t.TempDir()) == nil {
			t.Fatalf("accepted shared proxy on administration port %v", port)
		}
	}
}

func TestReservedPortsIncludeListenerRangesAndDNS(t *testing.T) {
	for _, port := range []any{"18101", "18100-18102", "18102-18100", "[18101]", "1080/18101,20000", "83637"} {
		raw, c := fixture()
		raw["listeners"] = []any{map[string]any{"name": "shared", "type": "mixed", "port": port}}
		if err := Overlay(raw, c, "profile", t.TempDir()); err == nil {
			t.Fatalf("accepted overlapping listener %v", port)
		}
	}
	raw, c := fixture()
	raw["dns"] = map[string]any{"listen": "0.0.0.0:18201"}
	if Overlay(raw, c, "profile", t.TempDir()) == nil {
		t.Fatal("accepted a shared DNS socket on the protected relay port")
	}
}

func TestUnfingerprintableAdapterCannotBeSelected(t *testing.T) {
	raw, _ := fixture()
	raw["proxies"].([]any)[0].(map[string]any)["extension"] = make(chan int)
	nodes := Catalog(raw, "profile", t.TempDir())
	if len(nodes) != 1 || nodes[0].Reason == "" {
		t.Fatal("unsupported adapter hashed as an empty document")
	}
	if _, ok := Match(nodes, nodes[0].Target); ok {
		t.Fatal("unsafe fingerprint matched a fixed egress")
	}
}
