package backend

import "testing"

func TestCountryOf(t *testing.T) {
	for _, c := range []struct {
		rec  any
		want string
	}{
		{"cn", "cn"},        // sing-geoip
		{[]any{"cn"}, "cn"}, // MetaCubeX
		{[]any{"private", "cloudflare", "us"}, "us"},                        // MetaCubeX, with lists
		{map[string]any{"country": map[string]any{"iso_code": "SG"}}, "SG"}, // MaxMind
		{nil, ""},
	} {
		if got := countryOf(c.rec); got != c.want {
			t.Errorf("countryOf(%v) = %q, want %q", c.rec, got, c.want)
		}
	}
}
