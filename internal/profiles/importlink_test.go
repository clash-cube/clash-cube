package profiles

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseImportLink(t *testing.T) {
	// Decode the outer URL exactly once: subscription tokens can themselves
	// contain percent escapes, plus signs and query separators.
	address := "https://example.com/sub?token=a%2Bb+c&flag=meta"
	link := "clash://install-config?url=" + url.QueryEscape(address) + "&name=" + url.QueryEscape("我的订阅")
	r, err := ParseImportLink(link)
	if err != nil || r.URL != address || r.Name != "我的订阅" {
		t.Fatalf("unexpected request: %#v, %v", r, err)
	}
	r, err = ParseImportLink("clash://install-config?url=http%3A%2F%2F127.0.0.1%3A8080%2Fprofile.yaml")
	if err != nil || r.Name != "" || r.URL != "http://127.0.0.1:8080/profile.yaml" {
		t.Fatalf("unnamed local subscription: %#v, %v", r, err)
	}
}

func TestParseImportLinkRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{
		"clash://install-config", "clash://install-config?url=",
		"clash://other?url=https://example.com",
		"other://install-config?url=https://example.com",
		"clash://install-config/extra?url=https://example.com",
		"clash://install-config?url=file:///tmp/profile.yaml",
		"clash://install-config?url=javascript:alert(1)",
		"clash://install-config?url=https://",
		"clash://install-config?url=https://example.com&url=https://other.com",
		"clash://install-config?url=https://example.com&name=a&name=b",
		"clash://install-config?url=%ZZ-secret",
	} {
		if _, err := ParseImportLink(raw); err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("invalid input accepted or leaked: %q: %v", raw, err)
		}
	}
}
