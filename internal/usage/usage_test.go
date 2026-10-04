package usage

import (
	"testing"
	"time"
)

func TestRecordAndQuery(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	day1 := time.Date(2026, 3, 1, 9, 30, 0, 0, time.Local)
	s.Record(day1, 100, 1000, []Item{
		{Dim: App, Key: "/A.app\x00A", Name: "A", Path: "/A.app", Up: 100, Down: 1000, New: true},
		{Dim: Network, Key: "wired", Up: 100, Down: 1000, New: true},
	})
	s.Record(day1.Add(time.Hour), 10, 20, []Item{{Dim: App, Key: "/A.app\x00A", Name: "A", Up: 10, Down: 20}})
	// the next day writes the first out
	day2 := day1.AddDate(0, 0, 1)
	s.Record(day2, 1, 2, []Item{{Dim: Host, Key: "x.com", Name: "x.com", Up: 1, Down: 2, New: true}})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	// a fresh store reads it all back from disk
	s = Open(dir)
	r, err := s.Query("2026-03-01", "2026-03-01", -1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Up != 110 || r.Down != 1020 || r.Conns != 1 || len(r.Bars) != 24 || r.Bars[9].Down != 1000 || r.Bars[10].Down != 20 {
		t.Fatalf("day: %+v", r)
	}
	if a := r.Dims[App]; len(a) != 1 || a[0].Up != 110 || a[0].Conns != 1 || a[0].Path != "/A.app" {
		t.Fatalf("apps: %+v", a)
	}
	r, _ = s.Query("2026-03-01", "2026-03-01", 10)
	if r.Down != 20 || r.Dims[App][0].Down != 20 {
		t.Fatalf("hour: %+v", r)
	}
	r, _ = s.Query("2026-02-28", "2026-03-02", 5)
	if r.Hour != -1 || len(r.Bars) != 3 || r.Bars[1].Down != 1020 || r.Bars[2].Down != 2 || r.Down != 1022 {
		t.Fatalf("range: %+v", r)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if r, _ = s.Query("2026-03-01", "2026-03-02", -1); r.Down != 0 {
		t.Fatalf("cleared: %+v", r)
	}
}

func TestTrimKeepsLargest(t *testing.T) {
	m := map[string]*Entry{}
	for i := range maxEntries + 5 {
		m[string(rune('a'+i%26))+string(rune(i))] = &Entry{Down: int64(i)}
	}
	trim(m)
	if len(m) != maxEntries {
		t.Fatal(len(m))
	}
	for _, e := range m {
		if e.Down < 5 {
			t.Fatal("kept a small entry")
		}
	}
}
