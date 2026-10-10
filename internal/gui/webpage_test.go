package gui

import (
	"reflect"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/userrules"
)

func TestWebpageRules(t *testing.T) {
	for raw, want := range map[string][]userrules.Rule{
		"https://x.com/someone/status/1":   {{Type: "DOMAIN-SUFFIX", Payload: "x.com"}, {Type: "DOMAIN", Payload: "x.com"}},
		"https://news.BBC.co.uk./a":        {{Type: "DOMAIN-SUFFIX", Payload: "bbc.co.uk"}, {Type: "DOMAIN", Payload: "news.bbc.co.uk"}},
		"http://192.168.1.1:8080/":         {{Type: "IP-CIDR", Payload: "192.168.1.1/32"}},
		"http://[2001:db8::1]/":            {{Type: "IP-CIDR6", Payload: "2001:db8::1/128"}},
		"http://localhost:3000/":           {{Type: "DOMAIN-SUFFIX", Payload: "localhost"}, {Type: "DOMAIN", Payload: "localhost"}},
		"about:blank":                      nil,
		"file:///Users/someone/index.html": nil,
	} {
		if got := webpageRules(raw); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", raw, got, want)
		}
	}
}
