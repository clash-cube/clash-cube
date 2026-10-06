package backend

import (
	"bytes"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/userrules"
)

// Refusal is the last configuration the core, or the merge before it,
// refused: the error, and the line of Source it points to (0 when it
// names none). Source is "runtime" for the merged configuration and
// "profile" for the profile in use, which was not YAML.
type Refusal struct {
	Error  string `json:"error"`
	Line   int    `json:"line"`
	Source string `json:"source"`
	body   []byte // the refused text; the file is written over after
}

// RuntimeView is the configuration the core was last given, and the last
// one refused since, as written, the controller's secret too. Lines is
// Body against the profile it was made from, when the profile can be read.
type RuntimeView struct {
	Body    string   `json:"body"`
	Lines   []Line   `json:"lines,omitempty"`
	Refusal *Refusal `json:"refusal,omitempty"`
	Refused string   `json:"refused,omitempty"`
}

func (b *Backend) RuntimeConfig() (RuntimeView, error) {
	body, err := os.ReadFile(appdir.RuntimeConfig())
	if err != nil && !os.IsNotExist(err) {
		return RuntimeView{}, err
	}
	v := RuntimeView{Body: readable(body)}
	if len(body) > 0 {
		v.Lines = changes(b.layers(), v.Body)
	}
	b.mu.Lock()
	if r := b.refusal; r != nil {
		v.Refusal, v.Refused = r, readable(r.body)
	}
	b.mu.Unlock()
	return v, nil
}

// layers is the runtime configuration built again from what it is made
// of now, a source at a time; nil when the profile can't be read.
func (b *Backend) layers() []runtimecfg.Layer {
	s := settings.Load()
	p, ok := profiles.Get(s.Profile)
	if !ok {
		return nil
	}
	body, err := os.ReadFile(p.Path())
	if err != nil {
		return nil
	}
	l, _ := runtimecfg.Layers(p.ID, body, s, b.core.Controller(), userrules.List(), modules.List())
	return l
}

// test has the core check the runtime configuration, keeping what it
// refused and where.
func (b *Backend) test() error {
	err := b.core.Test()
	if err == nil {
		b.setRefusal(nil)
		return nil
	}
	body, _ := os.ReadFile(appdir.RuntimeConfig())
	msg := testFailed.ReplaceAllString(err.Error(), "")
	b.setRefusal(&Refusal{Error: msg, Line: refusedLine(body, msg), Source: "runtime", body: body})
	return err
}

// refuseProfile keeps the profile in use at path, which isn't YAML, with
// the line its parser named.
func (b *Backend) refuseProfile(path string, err error) {
	body, _ := os.ReadFile(path)
	msg, line := strings.TrimPrefix(err.Error(), "profile: "), brokenLine(body)
	if line > 0 {
		// the parser's own line is where the block began; it misleads
		msg = yamlLine.ReplaceAllString(msg, "line "+strconv.Itoa(line)+":")
	} else if m := yamlLine.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
	}
	b.setRefusal(&Refusal{Error: msg, Line: line, Source: "profile", body: body})
}

// DismissRefusal forgets the last refusal, once the user has seen it.
func (b *Backend) DismissRefusal() { b.setRefusal(nil) }

func (b *Backend) setRefusal(r *Refusal) {
	b.mu.Lock()
	same := b.refusal == nil && r == nil
	b.refusal = r
	b.mu.Unlock()
	if !same {
		b.emitState()
	}
}

// readable is body to show, with emoji as themselves. The YAML encoder
// escapes what lies beyond the BMP as \UXXXXXXXX in double-quoted strings,
// where YAML takes it as written too; a backslash escaped before it is
// left alone.
func readable(b []byte) string {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		if b[i+1] == 'U' && i+10 <= len(b) {
			if r, err := strconv.ParseUint(string(b[i+2:i+10]), 16, 32); err == nil && r >= 0x10000 && utf8.ValidRune(rune(r)) {
				out = utf8.AppendRune(out, rune(r))
				i += 9
				continue
			}
		}
		out = append(out, b[i], b[i+1])
		i++
	}
	return string(out)
}

// What mihomo's errors say of where they are, and the item each points to.
var (
	yamlLine   = regexp.MustCompile(`\bline (\d+):`)
	testFailed = regexp.MustCompile(`^(clashcube: )?configuration test failed: `)
	errPaths   = []struct {
		re   *regexp.Regexp
		find func(root *yaml.Node, m []string) *yaml.Node
	}{
		{regexp.MustCompile(`^rules\[(\d+)\]`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(r, "rules"), m[1]) }},
		{regexp.MustCompile(`^sub-rules\[(.+?)\]\[(\d+)\]`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(key(r, "sub-rules"), m[1]), m[2]) }},
		{regexp.MustCompile(`^proxy (\d+):`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(r, "proxies"), m[1]) }},
		{regexp.MustCompile(`^proxy (.+) is the duplicate name`), func(r *yaml.Node, m []string) *yaml.Node { return named(key(r, "proxies"), m[1], 1) }},
		// the groups are sorted before they are parsed: the index is of
		// that order, so the name, which most of these give, is used
		{regexp.MustCompile(`^proxy group\[\d+\]: (.+?): `), func(r *yaml.Node, m []string) *yaml.Node { return named(key(r, "proxy-groups"), m[1], 0) }},
		{regexp.MustCompile(`^proxy group (.+): the duplicate name`), func(r *yaml.Node, m []string) *yaml.Node { return named(key(r, "proxy-groups"), m[1], 1) }},
		{regexp.MustCompile(`^proxy group (\d+): missing name`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(r, "proxy-groups"), m[1]) }},
		{regexp.MustCompile(`^parse proxy provider (.+) error`), func(r *yaml.Node, m []string) *yaml.Node { return keyNode(key(r, "proxy-providers"), m[1]) }},
		{regexp.MustCompile(`^listener (\d+):`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(r, "listeners"), m[1]) }},
		{regexp.MustCompile(`^dns\.fake-ip-filter\[(\d+)\]`), func(r *yaml.Node, m []string) *yaml.Node { return item(key(key(r, "dns"), "fake-ip-filter"), m[1]) }},
	}
)

// refusedLine is the line of body the core's error about it points to.
func refusedLine(body []byte, msg string) int {
	if m := yamlLine.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	var doc yaml.Node
	if yaml.Unmarshal(body, &doc) != nil || len(doc.Content) == 0 {
		return 0
	}
	for _, p := range errPaths {
		if m := p.re.FindStringSubmatch(msg); m != nil {
			if n := p.find(doc.Content[0], m); n != nil {
				return n.Line
			}
			return 0
		}
	}
	return 0
}

// brokenLine is the line where the parser found body isn't YAML. Its
// error names where the block holding the mistake begins, which can be far
// above it. The parser stops at the same place in any start of body that
// reaches the mistake, with the same error, so this is the shortest such.
func brokenLine(body []byte) int {
	parse := func(b []byte) error { var v any; return yaml.Unmarshal(b, &v) }
	whole := parse(body)
	if whole == nil {
		return 0
	}
	lines := bytes.SplitAfter(body, []byte("\n"))
	return 1 + sort.Search(len(lines), func(n int) bool {
		err := parse(bytes.Join(lines[:n+1], nil))
		return err != nil && err.Error() == whole.Error()
	})
}

// key is the value of k in mapping n.
func key(n *yaml.Node, k string) *yaml.Node {
	if kn := keyNode(n, k); kn != nil {
		return n.Content[indexOf(n.Content, kn)+1]
	}
	return nil
}

func keyNode(n *yaml.Node, k string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == k {
			return n.Content[i]
		}
	}
	return nil
}

func indexOf(ns []*yaml.Node, n *yaml.Node) int {
	for i, x := range ns {
		if x == n {
			return i
		}
	}
	return -1
}

// item is the i-th element of sequence n.
func item(n *yaml.Node, i string) *yaml.Node {
	idx, err := strconv.Atoi(i)
	if n == nil || n.Kind != yaml.SequenceNode || err != nil || idx < 0 || idx >= len(n.Content) {
		return nil
	}
	return n.Content[idx]
}

// named is the element of sequence n whose name is name, skipping the
// first skip of them (a duplicate is the second).
func named(n *yaml.Node, name string, skip int) *yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	for _, x := range n.Content {
		if v := key(x, "name"); v != nil && v.Value == name {
			if skip == 0 {
				return x
			}
			skip--
		}
	}
	return nil
}
