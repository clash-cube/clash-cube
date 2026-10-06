package backend

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

func TestRefusedLine(t *testing.T) {
	body := []byte(`mixed-port: 7890
proxies:
    - name: a
      type: ss
    - name: b
      type: ss
    - name: a
      type: ss
proxy-groups:
    - name: z
      type: select
    - name: g
      type: select
proxy-providers:
    sub:
        type: http
sub-rules:
    s1:
        - MATCH,DIRECT
        - DOMAIN,x.com,NOPE
dns:
    fake-ip-filter:
        - '*.lan'
        - bad,x
listeners:
    - name: in
rules:
    - DOMAIN,a.com,DIRECT
    - DOMAIN,a.com,NOPE
`)
	for _, c := range []struct {
		msg  string
		want int
	}{
		{"rules[1] [DOMAIN,a.com,NOPE] error: proxy [NOPE] not found", 29},
		{"proxy 1: ss cipher: bad initialize error", 5},
		{"proxy a is the duplicate name", 7},
		{"proxy group[0]: g: 'zz' not found", 12},
		{"proxy group[3]: nope: 'zz' not found", 0},
		{"parse proxy provider sub error: bad", 15},
		{"sub-rules[s1][1] [DOMAIN,x.com,NOPE] error: proxy [NOPE] not found", 20},
		{"dns.fake-ip-filter[1] [bad,x] error: invalid", 24},
		{"listener 0: bad", 26},
		{"yaml: line 7: did not find expected key", 7},
		{"  line 1: cannot unmarshal !!str `abc` into int", 1},
		{"DNS NameServer[0] unsupport scheme: bad", 0},
	} {
		if got := refusedLine(body, c.msg); got != c.want {
			t.Errorf("%s: line %d, want %d", c.msg, got, c.want)
		}
	}
}

func TestBrokenLine(t *testing.T) {
	for _, c := range []struct {
		body string
		want int
	}{
		{"a: 1\nb:\n  c: 2\nrules:\n  - X\n# note\n- Y\n", 7},
		{"a: 1\nb: \"two\n  lines\"\nc: [1,\n  2]\nd: 3\n", 0},
		{"a: 1\nb: [1, 2\nc: 3\n", 2},
		{"x: 1\n", 0},
	} {
		if got := brokenLine([]byte(c.body)); got != c.want {
			t.Errorf("%q: line %d, want %d", c.body, got, c.want)
		}
	}
}

func TestReadable(t *testing.T) {
	got := readable([]byte("secret: x\nrules:\n    - \"RULE-SET,a,\\U0001F9E0 Claude\"\n    - \"a\\\\U0001F9E0\"\n"))
	want := "secret: x\nrules:\n    - \"RULE-SET,a,🧠 Claude\"\n    - \"a\\\\U0001F9E0\"\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMatch(t *testing.T) {
	lcs := func(a, b []string) int {
		t := make([][]int, len(a)+1)
		for i := range t {
			t[i] = make([]int, len(b)+1)
		}
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					t[i][j] = t[i+1][j+1] + 1
				} else {
					t[i][j] = max(t[i+1][j], t[i][j+1])
				}
			}
		}
		return t[0][0]
	}
	r := rand.New(rand.NewPCG(1, 2))
	for n := 0; n < 500; n++ {
		a, b := make([]string, r.IntN(12)), make([]string, r.IntN(12))
		for i := range a {
			a[i] = string(rune('a' + r.IntN(3)))
		}
		for i := range b {
			b[i] = string(rune('a' + r.IntN(3)))
		}
		keep, kept, last := match(a, b), 0, -1
		for i, j := range keep {
			if j < 0 {
				continue
			}
			if j <= last || a[j] != b[i] {
				t.Fatalf("%v → %v: bad match %v", a, b, keep)
			}
			last, kept = j, kept+1
		}
		if want := lcs(a, b); kept != want {
			t.Fatalf("%v → %v: kept %d, want %d", a, b, kept, want)
		}
	}
}

func TestChanges(t *testing.T) {
	layers := []runtimecfg.Layer{
		{Source: "", Body: []byte("mixed-port: 7890\nrules:\n    - MATCH,DIRECT\n")},
		{Source: "module:m", Body: []byte("mixed-port: 7890\nrules:\n    - DOMAIN,a.com,DIRECT\n    - MATCH,DIRECT\n")},
		{Source: "settings", Body: []byte("mixed-port: 7891\nrules:\n    - DOMAIN,a.com,DIRECT\n    - MATCH,DIRECT\nsecret: s\n")},
	}
	got := changes(layers, "mixed-port: 7891\nrules:\n    - DOMAIN,a.com,DIRECT\n    - MATCH,DIRECT\nsecret: s\nlog-level: info\n")
	want := []Line{
		{Text: "mixed-port: 7890", Op: "-", Source: "settings"},
		{Text: "mixed-port: 7891", Op: "+", Source: "settings"},
		{Text: "rules:"},
		{Text: "    - DOMAIN,a.com,DIRECT", Op: "+", Source: "module:m"},
		{Text: "    - MATCH,DIRECT"},
		{Text: "secret: s", Op: "+", Source: "settings"},
		{Text: "log-level: info", Op: "+", Source: "stale"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}
