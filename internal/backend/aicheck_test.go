package backend

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/localhost-copilot/clashcube/internal/settings"
)

func TestJudgeRoutes(t *testing.T) {
	h := func(chain ...string) AIHost { return AIHost{Chain: chain} }
	failed := AIHost{Error: "no route"}
	for _, tc := range []struct {
		name          string
		hosts         []AIHost
		node, verdict string
	}{
		{"one node", []AIHost{h("us1", "Claude"), h("us1", "Proxy")}, "us1", "consistent"},
		{"refused telemetry doesn't count", []AIHost{h("us1", "Claude"), h("REJECT")}, "us1", "consistent"},
		{"a failed name doesn't count", []AIHost{h("us1", "Claude"), failed}, "us1", "consistent"},
		{"two nodes", []AIHost{h("us1", "Claude"), h("hk1", "Proxy"), h("hk1", "Proxy")}, "hk1", "split"},
		{"a tie goes to the first name's", []AIHost{h("us1", "Claude"), h("hk1", "Proxy")}, "us1", "split"},
		{"telemetry direct", []AIHost{h("us1", "Claude"), h("DIRECT")}, "us1", "direct"},
		{"all direct", []AIHost{h("DIRECT"), h("DIRECT", "Main")}, "DIRECT", "direct"},
		{"all refused", []AIHost{h("REJECT"), h("REJECT-DROP")}, "", "refused"},
		{"none routed", []AIHost{failed, failed}, "", "failed"},
	} {
		node, verdict := judgeRoutes(tc.hosts)
		if node != tc.node || verdict != tc.verdict {
			t.Errorf("%s: got %q %q, want %q %q", tc.name, node, verdict, tc.node, tc.verdict)
		}
	}
}

// The check reads each name's route from the core: the rule it matched,
// the chain, the groups that pick for themselves, and refusals.
func TestAIRoutesThroughCore(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a core")
	}
	fake.reset()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fl=1\nip=203.0.113.9\nloc=HK\n")
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort, s.AutoStart, s.Mode = freePort(t), false, "rule" }); err != nil {
		t.Fatal(err)
	}
	const profile = `
geodata-mode: false
geo-auto-update: false
hosts: { a.test: 127.0.0.1, b.test: 127.0.0.1, t.test: 127.0.0.1 }
proxies: []
proxy-groups:
  - { name: Claude, type: select, proxies: [DIRECT] }
  - { name: Auto, type: url-test, proxies: [DIRECT], url: "http://127.0.0.1:1/", interval: 3600 }
rules:
  - DOMAIN,a.test,Claude
  - DOMAIN,b.test,Auto
  - DOMAIN,r.test,REJECT
  - DOMAIN,t.test,Claude
  - MATCH,DIRECT
`
	b := New("test", "test", []byte(profile), nopSink{make(chan State, 64)})
	if err := b.Init(); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(b.Shutdown)
	c, _ := b.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	n, _ := strconv.Atoi(port)
	r := aiRoutes(ctx, c, "Claude", []string{"a.test", "b.test", "r.test"}, n)
	if got := r.Hosts[0]; got.Error != "" || got.Rule != "Domain" || got.RulePayload != "a.test" || !reflect.DeepEqual(got.Chain, []string{"DIRECT", "Claude"}) {
		t.Errorf("a.test: %+v", got)
	}
	if got := r.Hosts[1].Chain; !reflect.DeepEqual(got, []string{"DIRECT", "Auto"}) {
		t.Errorf("b.test chain %q", got)
	}
	if got := r.Hosts[2]; got.Error != "" || !reflect.DeepEqual(got.Chain, []string{"REJECT"}) {
		t.Errorf("r.test wasn't refused: %+v", got)
	}
	if r.Node != "DIRECT" || r.Verdict != "direct" {
		t.Errorf("verdict %q %q, want DIRECT direct", r.Node, r.Verdict)
	}
	if !reflect.DeepEqual(r.Auto, []string{"Auto"}) {
		t.Errorf("auto groups %q, want [Auto]", r.Auto)
	}

	s, _ := aiServiceNamed("Claude")
	e, err := aiEgress(ctx, c, s, "http://t.test:"+port+"/cdn-cgi/trace")
	if err != nil {
		t.Fatal(err)
	}
	if e.IP != "203.0.113.9" || e.Loc != "HK" || !e.Unsupported || !reflect.DeepEqual(e.Chain, []string{"DIRECT", "Claude"}) {
		t.Errorf("egress %+v", e)
	}
}
