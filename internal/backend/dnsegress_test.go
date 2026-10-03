package backend

import "testing"

func TestParseWhoami(t *testing.T) {
	for _, txt := range [][]string{
		{`"ecs" "104.202.107.0/24/24"`, `"ns" "2607:f8b0:4004:1007::120"`, `"ip" "104.202.107.92"`}, // the core
		{"ecs104.202.107.0/24/24", "ns2607:f8b0:4004:1007::120", "ip104.202.107.92"},                // Go's resolver
	} {
		ns, ecs := parseWhoami(txt)
		if ns != "2607:f8b0:4004:1007::120" || ecs != "104.202.107.0/24/24" {
			t.Errorf("parseWhoami(%q) = %q, %q", txt, ns, ecs)
		}
	}
}
