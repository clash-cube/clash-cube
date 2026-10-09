package sysproxy

import (
	"path"
	"testing"
)

func TestBypassSubnetBoundaries(t *testing.T) {
	patterns, err := bypassPatterns([]string{"172.16.0.0/12", "192.168.1.128/25", "*.local"})
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{"172.15.255.255": false, "172.16.0.0": true, "172.31.255.255": true, "172.32.0.0": false, "192.168.1.127": false, "192.168.1.128": true, "192.168.1.255": true, "192.168.2.0": false, "printer.local": true} {
		got := false
		for _, pattern := range patterns {
			if match, _ := path.Match(pattern, host); match {
				got = true
			}
		}
		if got != want {
			t.Errorf("bypass %s: got %v", host, got)
		}
	}
	if _, err := bypassPatterns([]string{"fd00::/8"}); err == nil {
		t.Error("unsupported subnet silently accepted")
	}
}

func TestProxyOwnership(t *testing.T) {
	for _, tt := range []struct {
		server string
		own    bool
	}{
		{"127.0.0.1:7890", true},
		{"http=127.0.0.1:7890;https=127.0.0.1:7890;socks=127.0.0.1:7890", true},
		{"http=127.0.0.1:7890;https=127.0.0.1:8080", false},
		{"socks=127.0.0.1:7890", false},
		{"127.0.0.1:78901", false},
	} {
		if got := proxyMatches(tt.server, "127.0.0.1", 7890); got != tt.own {
			t.Errorf("%q: got %v", tt.server, got)
		}
	}
}
