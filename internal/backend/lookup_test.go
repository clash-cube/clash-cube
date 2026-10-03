package backend

import "testing"

func TestSplitTarget(t *testing.T) {
	for in, want := range map[string]struct {
		host string
		port int
	}{
		"Example.com":               {"example.com", 443},
		"example.com:80":            {"example.com", 80},
		"https://a.b.example/x?y=1": {"a.b.example", 443},
		"http://example.com:8080/":  {"example.com", 8080},
		"[2001:db8::1]:53":          {"2001:db8::1", 53},
		"1.1.1.1":                   {"1.1.1.1", 443},
	} {
		h, p, err := splitTarget(in)
		if err != nil || h != want.host || p != want.port {
			t.Errorf("%q: %q %d %v", in, h, p, err)
		}
	}
	for _, bad := range []string{"", "a b", "example.com:0", "example.com:x"} {
		if _, _, err := splitTarget(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
