// Package usage keeps the traffic statistics across runs, as Surge's
// Traffic Statistics does: how much went through the core, hour by hour,
// and by app, host, policy and network. Each local day is one file in the
// usage directory; only today's is ever rewritten.
package usage

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// The dimensions an Item is counted under.
const (
	App     = "app"
	Host    = "host"
	Policy  = "policy"
	Network = "network"
)

var Dims = []string{App, Host, Policy, Network}

const (
	// a day keeps this many entries of each dimension; the rest is Other
	maxEntries = 200
	keepDays   = 90
	dateLayout = "2006-01-02"
)

// Entry is one app, host, policy or network on a day.
type Entry struct {
	Name  string           `json:"name"`
	Path  string           `json:"path,omitempty"` // an app's bundle or executable
	Up    int64            `json:"up"`
	Down  int64            `json:"down"`
	Conns int              `json:"conns"`
	Hours map[int][2]int64 `json:"hours,omitempty"` // hour → upload, download
}

// Day is a local day's traffic.
type Day struct {
	Date  string                       `json:"date"`
	Up    int64                        `json:"up"`
	Down  int64                        `json:"down"`
	Conns int                          `json:"conns"`
	Hours [24][2]int64                 `json:"hours"`
	Dims  map[string]map[string]*Entry `json:"dims"`
}

func newDay(date string) *Day {
	d := &Day{Date: date, Dims: map[string]map[string]*Entry{}}
	for _, k := range Dims {
		d.Dims[k] = map[string]*Entry{}
	}
	return d
}

// Item is bytes moved by one entry since the last sample. New says a
// connection opened, for the entry's count.
type Item struct {
	Dim, Key, Name, Path string
	Up, Down             int64
	New                  bool
}

type Store struct {
	dir   string
	mu    sync.Mutex
	today *Day
	dirty bool
	past  map[string]*Day // days before today, as read
}

func Open(dir string) *Store { return &Store{dir: dir, past: map[string]*Day{}} }

func (s *Store) file(date string) string { return filepath.Join(s.dir, date+".json") }

// Record adds a sample taken at: the core's total bytes since the last
// one, and what of them each entry moved.
func (s *Store) Record(at time.Time, up, down int64, items []Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.day(at)
	h := at.Hour()
	d.Up += up
	d.Down += down
	d.Hours[h][0] += up
	d.Hours[h][1] += down
	for _, it := range items {
		if it.Key == "" || it.Up == 0 && it.Down == 0 && !it.New {
			continue
		}
		m := d.Dims[it.Dim]
		if m == nil {
			m = map[string]*Entry{}
			d.Dims[it.Dim] = m
		}
		e := m[it.Key]
		if e == nil {
			e = &Entry{Name: it.Name, Path: it.Path, Hours: map[int][2]int64{}}
			m[it.Key] = e
		}
		e.Up += it.Up
		e.Down += it.Down
		hb := e.Hours[h]
		e.Hours[h] = [2]int64{hb[0] + it.Up, hb[1] + it.Down}
		if it.New {
			e.Conns++
			if it.Dim == Network {
				d.Conns++
			}
		}
		// a long day of many hosts is kept to its largest
		if len(m) > 2*maxEntries {
			trim(m)
		}
	}
	s.dirty = true
}

// day is the day at falls on, today's once it begins; the day before is
// written out first. Requires mu.
func (s *Store) day(at time.Time) *Day {
	date := at.Format(dateLayout)
	if s.today != nil && s.today.Date == date {
		return s.today
	}
	if s.today != nil {
		_ = s.save()
		s.past[s.today.Date] = s.today
		s.prune(at)
	}
	s.today = s.read(date)
	if s.today == nil {
		s.today = newDay(date)
	}
	return s.today
}

// trim keeps the largest entries.
func trim(m map[string]*Entry) {
	if len(m) <= maxEntries {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return cmp.Compare(m[b].Up+m[b].Down, m[a].Up+m[a].Down) })
	for _, k := range keys[maxEntries:] {
		delete(m, k)
	}
}

// Flush writes today's file if it changed.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	return s.save()
}

// save writes today out; requires mu.
func (s *Store) save() error {
	d := s.today
	if d == nil {
		return nil
	}
	for _, m := range d.Dims {
		trim(m)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp := s.file(d.Date) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.file(d.Date)); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// read loads a day's file; nil when there is none. Requires mu.
func (s *Store) read(date string) *Day {
	b, err := os.ReadFile(s.file(date))
	if err != nil {
		return nil
	}
	d := newDay(date)
	if json.Unmarshal(b, d) != nil || d.Date != date {
		return nil
	}
	for _, k := range Dims {
		if d.Dims[k] == nil {
			d.Dims[k] = map[string]*Entry{}
		}
	}
	return d
}

// get is the day of date, today's from memory; nil without one. Requires mu.
func (s *Store) get(date string) *Day {
	if s.today != nil && s.today.Date == date {
		return s.today
	}
	if d, ok := s.past[date]; ok {
		return d
	}
	d := s.read(date)
	s.past[date] = d
	return d
}

// prune removes the files of days older than keepDays. Requires mu.
func (s *Store) prune(now time.Time) {
	oldest := now.AddDate(0, 0, -keepDays).Format(dateLayout)
	files, _ := os.ReadDir(s.dir)
	for _, f := range files {
		date, ok := strings.CutSuffix(f.Name(), ".json")
		if ok && len(date) == len(dateLayout) && date < oldest {
			_ = os.Remove(filepath.Join(s.dir, f.Name()))
			delete(s.past, date)
		}
	}
}

// Clear forgets every day, today's too.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.today, s.dirty, s.past = nil, false, map[string]*Day{}
	files, _ := os.ReadDir(s.dir)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			if err := os.Remove(filepath.Join(s.dir, f.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
