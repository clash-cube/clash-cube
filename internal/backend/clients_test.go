package backend

import (
	"testing"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
)

func TestClientMeter(t *testing.T) {
	const chrome = "/Applications/Google Chrome.app"
	helper := chrome + "/Contents/Frameworks/Google Chrome Framework.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"
	t0 := time.Unix(1000, 0)
	conn := func(id, path string, start time.Time, up, down int64) mihomoapi.Connection {
		return mihomoapi.Connection{ID: id, Start: start, Upload: up, Download: down,
			Metadata: mihomoapi.Metadata{ProcessPath: path, Process: "p-" + id, SourceIP: "192.168.1.9"}}
	}

	var m ClientMeter
	first := m.Sample(t0, []mihomoapi.Connection{
		conn("a", chrome+"/Contents/MacOS/Google Chrome", t0.Add(-time.Minute), 100, 1000),
		conn("b", "/usr/bin/curl", t0.Add(-time.Minute), 0, 5000),
	})
	if len(first) != 2 || first[0].Name != "p-b" || first[0].Down != 0 {
		t.Fatalf("first sample = %+v, want curl first by bytes, no speed yet", first)
	}

	got := m.Sample(t0.Add(2*time.Second), []mihomoapi.Connection{
		// a moved 200 bytes up and 1800 down
		conn("a", chrome+"/Contents/MacOS/Google Chrome", t0.Add(-time.Minute), 300, 2800),
		// c is Chrome's helper, opened since: all its bytes count
		conn("c", helper, t0.Add(time.Second), 0, 4000),
		conn("b", "/usr/bin/curl", t0.Add(-time.Minute), 0, 5000),
		conn("d", "", t0.Add(time.Second), 10, 10),
	})
	want := []ClientRate{
		{Name: "Google Chrome", Path: chrome, Up: 100, Down: 2900},
		{Name: "p-d", Up: 5, Down: 5},
		{Name: "p-b", Path: "/usr/bin/curl"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		g := got[i]
		g.total = 0
		if g != want[i] {
			t.Errorf("rate %d = %+v, want %+v", i, g, want[i])
		}
	}
}

func TestAppOf(t *testing.T) {
	for _, c := range []struct {
		md   mihomoapi.Metadata
		name string
	}{
		// the core's own: no process, no source
		{mihomoapi.Metadata{Type: "Inner"}, "mihomo"},
		{mihomoapi.Metadata{Type: "HTTP", SourceIP: "192.168.1.9"}, "192.168.1.9"},
		{mihomoapi.Metadata{Type: "Tun"}, "Tun"},
	} {
		if name, _ := appOf(c.md); name != c.name {
			t.Errorf("appOf(%+v) = %q, want %q", c.md, name, c.name)
		}
	}
}
