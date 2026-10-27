package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticExportDoesNotCopySystemSecrets(t *testing.T) {
	exec := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "ubus" && args[2] == "board":
			return []byte(`{"kernel":"test-kernel","release":{"description":"QWRT"},"serial":"private-serial","secret":"private-controller"}`), nil
		case name == "ubus":
			return []byte(`{"uptime":123,"memory":{"total":1024,"available":512},"subscription":"private-subscription"}`), nil
		case name == "opkg":
			return []byte("clashcube-core - 1.2.3-1\nprivate-token - credential\n"), nil
		case name == "iptables":
			return []byte("iptables (legacy)"), nil
		default:
			return nil, nil
		}
	}
	home := t.TempDir()
	d := collectDiagnostics(exec, home, home, home)
	if d.System != "QWRT" || d.MemoryFree != 512 || len(d.Packages) != 1 || d.Storage[0].Total == 0 {
		t.Fatal("diagnostic projection lost useful state")
	}
	body, _ := json.Marshal(d)
	if strings.Contains(string(body), "private-") || strings.Contains(string(body), "credential") {
		t.Fatal("diagnostic export leaked unapproved system fields")
	}
}
