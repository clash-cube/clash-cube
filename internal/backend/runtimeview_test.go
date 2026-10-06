package backend

import "testing"

func TestMaskSecret(t *testing.T) {
	got := string(maskSecret([]byte("external-controller: 127.0.0.1:1234\nsecret: s3cr3t\ntun:\n    secret: kept\n")))
	if got != "external-controller: 127.0.0.1:1234\nsecret: \"********\"\ntun:\n    secret: kept\n" {
		t.Fatalf("got %q", got)
	}
}

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
	want := "secret: \"********\"\nrules:\n    - \"RULE-SET,a,🧠 Claude\"\n    - \"a\\\\U0001F9E0\"\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
