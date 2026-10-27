package router

import (
	"errors"
	"strings"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"go.yaml.in/yaml/v3"
)

func TestRouterOverlayPreservesPoliciesAndBindsOnlyLAN(t *testing.T) {
	m := &Manager{discover: func() (lan, error) {
		return lan{Device: "br-lan", Address: "192.168.50.1", Prefix: "192.168.50.0/24"}, nil
	}}
	m.enabled.Store(true)
	profile := []byte("listeners: [{name: existing, type: http, port: 9000}]\ndns: {nameserver: [9.9.9.9], enhanced-mode: fake-ip}\nrules: [\"MATCH,DIRECT\"]\n")
	overlay := runtimecfg.Overlay{Source: "router", Apply: m.overlay}
	body, err := runtimecfg.Build("p", profile, settings.Defaults(), runtimecfg.Controller{Addr: "127.0.0.1:5000", Secret: "private"}, nil, nil, overlay)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	listeners := config["listeners"].([]any)
	if len(listeners) != 2 || listeners[0].(map[string]any)["name"] != "existing" {
		t.Fatal("user listeners lost")
	}
	listener := listeners[1].(map[string]any)
	if listener["listen"] != "192.168.50.1" || listener["type"] != "redir" {
		t.Fatalf("unsafe binding: %v", listener)
	}
	dns := config["dns"].(map[string]any)
	if dns["enhanced-mode"] != "redir-host" || dns["ipv6"] != false || dns["listen"] != "192.168.50.1:1053" {
		t.Fatalf("DNS incompatible with TCP-only interception: %v", dns)
	}
	if dns["nameserver"].([]any)[0] != "9.9.9.9" || config["rules"].([]any)[0] != "MATCH,DIRECT" || config["secret"] != "private" {
		t.Fatal("overlay changed routing, upstream DNS or controller credentials")
	}
	layers, err := runtimecfg.Layers("p", profile, settings.Defaults(), runtimecfg.Controller{}, nil, nil, overlay)
	if err != nil || layers[len(layers)-1].Source != "router" {
		t.Fatal("preview lost overlay provenance")
	}
}

func TestRouterRejectsConflictingListenersAndUnavailableLAN(t *testing.T) {
	m := &Manager{discover: func() (lan, error) { return lan{Address: "192.168.1.1"}, nil }}
	// Disabled routers do not probe interfaces or modify ordinary configs.
	m.discover = func() (lan, error) { return lan{}, errors.New("no LAN") }
	if err := m.overlay(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	m.enabled.Store(true)
	if err := m.overlay(map[string]any{}); err == nil {
		t.Fatal("accepted missing LAN")
	}
	m.discover = func() (lan, error) { return lan{Address: "192.168.1.1"}, nil }
	for _, config := range []map[string]any{
		{"mixed-port": 7898},
		{"listeners": []any{map[string]any{"port": 1053}}},
	} {
		if err := m.overlay(config); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("accepted port collision: %v", err)
		}
	}
}

func TestRouterExplicitProxyKeepsLoopbackAndAvoidsWildcardWANBinding(t *testing.T) {
	m := &Manager{discover: func() (lan, error) { return lan{Address: "192.168.1.1"}, nil }}
	config := map[string]any{"allow-lan": true, "mixed-port": 7897, "port": 8080, "socks-port": 1080, "bind-address": "*"}
	if err := m.overlay(config); err != nil {
		t.Fatal(err)
	}
	if config["bind-address"] != "127.0.0.1" {
		t.Fatal("top-level proxy still binds WAN")
	}
	listeners := config["listeners"].([]any)
	if len(listeners) != 3 {
		t.Fatal("LAN access lost an explicit proxy protocol")
	}
	for _, item := range listeners {
		if item.(map[string]any)["listen"] != "192.168.1.1" {
			t.Fatal("explicit proxy listener is not scoped to LAN")
		}
	}
	if _, ok := config["dns"]; ok {
		t.Fatal("explicit proxy access enabled transparent DNS interception")
	}
}
