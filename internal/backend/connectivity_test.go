package backend

import "testing"

func TestRoundMS(t *testing.T) {
	for in, want := range map[float64]float64{
		0.001: 0.01, 0.234: 0.23, 0.456: 0.46, 0.996: 1, 1.4: 1, 1.6: 2, 270.4: 270,
	} {
		if got := roundMS(in); got != want {
			t.Errorf("roundMS(%v) = %v, want %v", in, got, want)
		}
	}
}
