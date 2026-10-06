package backend

import (
	"strings"

	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
)

// Line is a line of the merged configuration shown against the profile:
// Op is "+" for a line the profile doesn't have, "-" for one of the
// profile's that is gone, "" for one kept, and Source what added or
// removed it (runtimecfg.Layer's, or "stale" for a difference between the
// configuration the core has and what the settings make now).
type Line struct {
	Text   string `json:"text"`
	Op     string `json:"op,omitempty"`
	Source string `json:"source,omitempty"`
}

// changes is final shown against the first of layers, each line added or
// removed put down to the layer that did it. The profile's line a later
// layer takes away is shown where it stood, before what took its place.
func changes(layers []runtimecfg.Layer, final string) []Line {
	if len(layers) == 0 {
		return nil
	}
	type tag struct {
		base int // the profile's line it is, or -1
		src  string
	}
	base := splitLines(readable(layers[0].Body))
	prev, cur := base, make([]tag, len(base))
	for i := range cur {
		cur[i] = tag{base: i}
	}
	gone := make([]string, len(base)) // what removed each, if anything
	removed := make([]bool, len(base))
	step := func(src string, next []string) {
		keep := match(prev, next)
		used := make([]bool, len(prev))
		tags := make([]tag, len(next))
		for i, j := range keep {
			if j >= 0 {
				tags[i], used[j] = cur[j], true
			} else {
				tags[i] = tag{base: -1, src: src}
			}
		}
		for j, u := range used {
			if b := cur[j].base; !u && b >= 0 {
				removed[b], gone[b] = true, src
			}
		}
		prev, cur = next, tags
	}
	for _, l := range layers[1:] {
		step(l.Source, splitLines(readable(l.Body)))
	}
	step("stale", splitLines(final))

	// the next of the profile's lines kept at or after each line
	anchor := make([]int, len(cur)+1)
	anchor[len(cur)] = len(base)
	for i := len(cur) - 1; i >= 0; i-- {
		if anchor[i] = anchor[i+1]; cur[i].base >= 0 {
			anchor[i] = cur[i].base
		}
	}
	out := make([]Line, 0, len(cur))
	d := 0
	flush := func(upto int) {
		for ; d < upto; d++ {
			if removed[d] {
				out = append(out, Line{Text: base[d], Op: "-", Source: gone[d]})
			}
		}
	}
	for i, t := range cur {
		flush(anchor[i])
		if t.base >= 0 {
			d = t.base + 1
			out = append(out, Line{Text: prev[i]})
		} else {
			out = append(out, Line{Text: prev[i], Op: "+", Source: t.src})
		}
	}
	flush(len(base))
	return out
}

func splitLines(s string) []string {
	if s = strings.TrimSuffix(s, "\n"); s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// match is, for each line of b, the line of a it keeps in a shortest edit
// from a to b, or -1 for one added.
func match(a, b []string) []int {
	keep := make([]int, len(b))
	for i := range keep {
		keep[i] = -1
	}
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		keep[p] = p
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		keep[len(b)-1-s] = len(a) - 1 - s
		s++
	}
	for _, xy := range myers(a[p:len(a)-s], b[p:len(b)-s]) {
		keep[p+xy[1]] = p + xy[0]
	}
	return keep
}

// Past this many edits the middle is taken as replaced whole: Myers keeps
// a row per edit, which grows as their square.
const maxEdits = 2000

// myers is the lines a and b have in common in a shortest edit between
// them (Myers, 1986), as pairs of their indices.
func myers(a, b []string) [][2]int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil
	}
	max := n + m
	v := make([]int, 2*max+2)
	var trace [][]int // v before each round, for k in [-d, d]
	for d := 0; d <= max && d <= maxEdits; d++ {
		trace = append(trace, append([]int(nil), v[max-d:max+d+1]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[max+k-1] < v[max+k+1]) {
				x = v[max+k+1]
			} else {
				x = v[max+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[max+k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m)
			}
		}
	}
	return nil
}

func backtrack(trace [][]int, x, y int) [][2]int {
	var pairs [][2]int
	for d := len(trace) - 1; d > 0; d-- {
		v, k := trace[d], x-y
		pk := k - 1
		if k == -d || (k != d && v[k-1+d] < v[k+1+d]) {
			pk = k + 1
		}
		px := v[pk+d]
		py := px - pk
		for x > px && y > py {
			x, y = x-1, y-1
			pairs = append(pairs, [2]int{x, y})
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		pairs = append(pairs, [2]int{x, y})
	}
	return pairs
}
