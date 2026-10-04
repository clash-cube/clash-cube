package backend

import "testing"

func TestConnectivityHistory(t *testing.T) {
	b := &Backend{}
	b.record("proxy", 0) // not measured: not kept
	b.markBreak()        // before any measure: no break to show
	for i := 1; i <= latencyHistory+5; i++ {
		b.record("proxy", float64(i))
	}
	b.markBreak()
	b.record("proxy", -1)
	h := b.ConnectivityHistory()
	p := h["proxy"]
	if len(p) != latencyHistory {
		t.Fatalf("kept %d, want %d", len(p), latencyHistory)
	}
	if p[0].MS != 7 || p[0].Break {
		t.Errorf("oldest = %+v, want 7 without a break", p[0])
	}
	if last := p[len(p)-1]; last.MS != -1 || !last.Break {
		t.Errorf("newest = %+v, want a failure after a break", last)
	}
	if len(h["router"]) != 0 {
		t.Errorf("router has %d", len(h["router"]))
	}
	b.record("router", 0.3)
	if r := b.ConnectivityHistory()["router"]; len(r) != 1 || r[0].Break {
		t.Errorf("router = %+v, want one without a break", r)
	}
}

func TestRoundMS(t *testing.T) {
	for in, want := range map[float64]float64{
		0.001: 0.01, 0.234: 0.23, 0.456: 0.46, 0.996: 1, 1.4: 1, 1.6: 2, 270.4: 270,
	} {
		if got := roundMS(in); got != want {
			t.Errorf("roundMS(%v) = %v, want %v", in, got, want)
		}
	}
}
