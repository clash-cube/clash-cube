package usage

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Bar is one hour of a day, or one day of a range.
type Bar struct {
	Key  string `json:"key"` // the hour ("0"–"23") or the date
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// Row is an entry over the report's period.
type Row struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Conns int    `json:"conns"`
}

// Report is the traffic of the days from..to (dates, inclusive), or of
// one hour of a single day.
type Report struct {
	From  string           `json:"from"`
	To    string           `json:"to"`
	Hour  int              `json:"hour"` // -1: the whole period
	Up    int64            `json:"up"`
	Down  int64            `json:"down"`
	Conns int              `json:"conns"`
	Bars  []Bar            `json:"bars"`
	Dims  map[string][]Row `json:"dims"`
}

// Query reports the days from..to; one day is in hours, and hour (0–23)
// narrows its entries to that hour. A range of days ignores hour.
func (s *Store) Query(from, to string, hour int) (Report, error) {
	a, err := time.ParseInLocation(dateLayout, from, time.Local)
	if err != nil {
		return Report{}, err
	}
	b, err := time.ParseInLocation(dateLayout, to, time.Local)
	if err != nil {
		return Report{}, err
	}
	if b.Before(a) {
		a, b = b, a
	}
	if b.Sub(a) > (keepDays+1)*24*time.Hour {
		a = b.AddDate(0, 0, -keepDays)
	}
	single := a.Equal(b)
	if !single || hour < 0 || hour > 23 {
		hour = -1
	}
	r := Report{From: a.Format(dateLayout), To: b.Format(dateLayout), Hour: hour, Bars: []Bar{}, Dims: map[string][]Row{}}
	rows := map[string]map[string]*Row{}
	for _, k := range Dims {
		rows[k] = map[string]*Row{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		date := d.Format(dateLayout)
		day := s.get(date)
		if day == nil {
			if !single {
				r.Bars = append(r.Bars, Bar{Key: date})
			}
			continue
		}
		if single {
			for h, v := range day.Hours {
				r.Bars = append(r.Bars, Bar{Key: strconv.Itoa(h), Up: v[0], Down: v[1]})
			}
		} else {
			r.Bars = append(r.Bars, Bar{Key: date, Up: day.Up, Down: day.Down})
		}
		if hour >= 0 {
			r.Up += day.Hours[hour][0]
			r.Down += day.Hours[hour][1]
		} else {
			r.Up += day.Up
			r.Down += day.Down
			r.Conns += day.Conns
		}
		for dim, m := range day.Dims {
			out := rows[dim]
			if out == nil {
				continue
			}
			for key, e := range m {
				up, down, conns := e.Up, e.Down, e.Conns
				if hour >= 0 {
					v := e.Hours[hour]
					up, down, conns = v[0], v[1], 0
					if up == 0 && down == 0 {
						continue
					}
				}
				row := out[key]
				if row == nil {
					row = &Row{Key: key, Name: e.Name, Path: e.Path}
					out[key] = row
				}
				row.Up += up
				row.Down += down
				row.Conns += conns
			}
		}
	}
	if single && len(r.Bars) == 0 {
		for h := range 24 {
			r.Bars = append(r.Bars, Bar{Key: strconv.Itoa(h)})
		}
	}
	for dim, m := range rows {
		list := make([]Row, 0, len(m))
		for _, row := range m {
			list = append(list, *row)
		}
		slices.SortFunc(list, func(x, y Row) int {
			return cmp.Or(cmp.Compare(y.Up+y.Down, x.Up+x.Down), strings.Compare(x.Name, y.Name))
		})
		r.Dims[dim] = list
	}
	return r, nil
}
