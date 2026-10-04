package backend

import (
	"testing"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/usage"
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

func TestUsageMeter(t *testing.T) {
	var m usageMeter
	conn := func(id string, up, down int64) mihomoapi.Connection {
		return mihomoapi.Connection{ID: id, Upload: up, Download: down, Chains: []string{"HK", "Proxy"},
			Metadata: mihomoapi.Metadata{Host: "a.com", ProcessPath: "/Applications/A.app/Contents/MacOS/A"}}
	}
	net := usage.Item{Dim: usage.Network, Key: "wired"}
	// the first sample is the baseline
	if up, down, items := m.sample(mihomoapi.Connections{UploadTotal: 50, DownloadTotal: 500, Connections: []mihomoapi.Connection{conn("1", 50, 500), conn("3", 1, 1)}}, nil, net); up != 0 || down != 0 || len(items) != 1 {
		t.Fatal(up, down, items)
	}
	// 2 is new; 3 closed with 10 more down; 4 opened and closed in between
	closed := []mihomoapi.Connection{conn("3", 1, 11), conn("4", 0, 90)}
	up, down, items := m.sample(mihomoapi.Connections{UploadTotal: 80, DownloadTotal: 1000, Connections: []mihomoapi.Connection{conn("1", 60, 700), conn("2", 20, 200), conn("3", 1, 5)}}, closed, net)
	if up != 30 || down != 500 || len(m.seen) != 2 {
		t.Fatal(up, down)
	}
	byDim := map[string]int64{}
	newConns := 0
	for _, it := range items {
		byDim[it.Dim+" "+it.Name] += it.Down
		if it.New && it.Dim == usage.App {
			newConns++
		}
	}
	if byDim["app A"] != 500 || byDim["host a.com"] != 500 || byDim["policy HK"] != 500 || byDim["network "] != 500 || newConns != 2 {
		t.Fatal(byDim, newConns)
	}
	// the core started again: its totals restart
	if up, down, _ := m.sample(mihomoapi.Connections{UploadTotal: 5, DownloadTotal: 7}, nil, net); up != 5 || down != 7 {
		t.Fatal(up, down)
	}
}
