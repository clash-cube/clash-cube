package backend

import (
	"testing"

	"github.com/localhost-copilot/clashferry/internal/settings"
)

func TestParseNetwork(t *testing.T) {
	out := `<dictionary> {
  PrimaryInterface : en0
  PrimaryService : FC9A4679-82E2-4C3C-9329-F023D88830F2
  Router : 192.168.50.1
}
`
	if got := parseNetwork(out); got != "en0 192.168.50.1" {
		t.Errorf("got %q", got)
	}
	if got := parseNetwork("  No such key\n"); got != "" {
		t.Errorf("offline: got %q", got)
	}
}

func TestSwitches(t *testing.T) {
	prev := map[string]string{"Auto": "hk", "Fall": "jp"}
	now := map[string]string{"Auto": "sg", "Fall": "jp", "New": "us"}
	got := switches(prev, now)
	if len(got) != 1 || got[0] != [3]string{"Auto", "hk", "sg"} {
		t.Errorf("got %v", got)
	}
	if len(switches(nil, now)) != 0 {
		t.Error("a first look is no switch")
	}
}

type eventSink struct {
	nopSink
	events chan Event
}

func (s eventSink) Event(e Event) { s.events <- e }

// Another app pointing the system proxy elsewhere is noticed on the
// second look, said once, and the icon no longer claims the traffic; our
// setting it again clears that.
func TestProxyLost(t *testing.T) {
	t.Setenv("CLASHFERRY_HOME", t.TempDir())
	fake.reset()
	port := freePort(t)
	if _, err := settings.Update(func(s *settings.Settings) { s.MixedPort, s.SystemProxy = port, true }); err != nil {
		t.Fatal(err)
	}
	sink := eventSink{nopSink{make(chan State, 64)}, make(chan Event, 8)}
	b := New("test", "test", []byte(base), sink)
	if err := b.applyProxy(true); err != nil {
		t.Fatal(err)
	}
	misses := 0
	b.checkProxy(&misses, true)
	if len(sink.events) != 0 || b.State().ProxyLost {
		t.Fatal("ours taken for lost")
	}

	fake.set("127.0.0.1", port+1) // another app
	b.checkProxy(&misses, true)
	if len(sink.events) != 0 {
		t.Fatal("lost on the first look")
	}
	b.checkProxy(&misses, true)
	if e := <-sink.events; e.Kind != "proxy" || !e.Notify {
		t.Errorf("event %+v", e)
	}
	if !b.State().ProxyLost {
		t.Error("state doesn't say lost")
	}
	b.checkProxy(&misses, true)
	b.checkProxy(&misses, true)
	if len(sink.events) != 0 {
		t.Error("said twice")
	}

	if err := b.applyProxy(true); err != nil {
		t.Fatal(err)
	}
	if b.State().ProxyLost {
		t.Error("still lost once set again")
	}
}
