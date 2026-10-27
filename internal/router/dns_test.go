package router

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDNSRecoveryRestoresOriginalUpstreamsAfterProcessLoss(t *testing.T) {
	for _, failReload := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "partial activation"}[failReload], func(t *testing.T) {
			values := map[string]any{".type": "dnsmasq", ".name": "cfg0", "server": []any{"/corp/10.0.0.1", "9.9.9.9"}, "domain": "lan"}
			original, _ := json.Marshal(values)
			fail := failReload
			exec := fakeDNSUCI(t, values, &fail)
			path := filepath.Join(t.TempDir(), "undo.json")
			d := dnsForwarder{exec: exec, path: path}
			err := d.apply(lan{Address: "192.168.1.1"})
			if (err != nil) != failReload {
				t.Fatal("unexpected activation result", err)
			}
			if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0600 {
				t.Fatal("undo record not private")
			}
			// A fresh process must recover without relying on in-memory state.
			recovered := dnsForwarder{exec: exec, path: path}
			if err := recovered.restore(); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(values)
			if string(after) != string(original) {
				t.Fatalf("DNS settings lost: %s -> %s", original, after)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("completed recovery kept undo record")
			}
		})
	}
}

func TestDNSRestorePreservesAdministratorEditsAndRetriesReload(t *testing.T) {
	values := map[string]any{".type": "dnsmasq", ".name": "cfg0", "noresolv": "0"}
	fail := false
	d := dnsForwarder{exec: fakeDNSUCI(t, values, &fail), path: filepath.Join(t.TempDir(), "undo.json")}
	if err := d.apply(lan{Address: "192.168.1.1"}); err != nil {
		t.Fatal(err)
	}
	values["server"] = []any{"8.8.8.8"}
	fail = true
	if err := d.restore(); err == nil {
		t.Fatal("accepted failed dnsmasq reload")
	}
	if err := d.restore(); err != nil {
		t.Fatal("could not finish interrupted recovery", err)
	}
	if !reflect.DeepEqual(values["server"], []any{"8.8.8.8"}) || values["noresolv"] != "0" {
		t.Fatal("recovery overwrote administrator edits", values)
	}
}

func TestDNSRestoresEvenWhenFirewallCleanupFails(t *testing.T) {
	values := map[string]any{".type": "dnsmasq", ".name": "cfg0", "server": []any{"9.9.9.9"}}
	fail := false
	d := dnsForwarder{exec: fakeDNSUCI(t, values, &fail), path: filepath.Join(t.TempDir(), "undo.json")}
	if err := d.apply(lan{Address: "192.168.1.1"}); err != nil {
		t.Fatal(err)
	}
	unavailable := errors.New("netfilter unavailable")
	g := Guard{dns: d, fw: firewall{exec: func(string, ...string) ([]byte, error) { return nil, unavailable }}}
	if err := g.clear(); !errors.Is(err, unavailable) {
		t.Fatal("firewall failure was hidden", err)
	}
	if !reflect.DeepEqual(values["server"], []any{"9.9.9.9"}) {
		t.Fatal("firewall failure stranded DNS on the stopped core")
	}
	if g.cleared {
		t.Fatal("failed firewall cleanup will not be retried")
	}
}

func fakeDNSUCI(t *testing.T, values map[string]any, failReload *bool) command {
	t.Helper()
	return func(name string, args ...string) ([]byte, error) {
		if name == "/etc/init.d/dnsmasq" {
			if *failReload {
				*failReload = false
				return nil, errors.New("reload failed")
			}
			return nil, nil
		}
		if name != "ubus" || len(args) != 4 {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		var request struct {
			Values map[string]any `json:"values"`
			Option string         `json:"option"`
		}
		if err := json.Unmarshal([]byte(args[3]), &request); err != nil {
			t.Fatal(err)
		}
		switch args[2] {
		case "get":
			return json.Marshal(map[string]any{"values": values})
		case "set":
			for k, v := range request.Values {
				values[k] = v
			}
		case "delete":
			delete(values, request.Option)
		default:
			t.Fatalf("unexpected UCI operation: %v", args)
		}
		return []byte("{}"), nil
	}
}

func TestRouterDNSRejectsForwardingLoops(t *testing.T) {
	for _, upstream := range []string{"127.0.0.1", "udp://192.168.1.1:53", "tcp://localhost:53", "192.168.1.1:1053", "system"} {
		if err := checkDNSLoops(map[string]any{"nameserver": []any{upstream}}, "192.168.1.1"); err == nil {
			t.Fatalf("accepted DNS loop via %s", upstream)
		}
	}
	if err := checkDNSLoops(map[string]any{"nameserver": []any{"https://dns.example/dns-query", "9.9.9.9"}}, "192.168.1.1"); err != nil {
		t.Fatal(err)
	}
}
