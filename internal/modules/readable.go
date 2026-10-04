package modules

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// ReadableYAML displays supplementary Unicode characters (including emoji)
// literally in double-quoted YAML scalars. yaml.v3 escapes these on output.
// Work from parser positions so comments, plain/block scalars and literal
// backslashes in regexps retain their meaning and formatting.
func ReadableYAML(body string) string {
	if !strings.Contains(body, `\U`) {
		return body
	}
	var doc yaml.Node
	if yaml.Unmarshal([]byte(body), &doc) != nil {
		return body
	}
	source := []rune(body)
	lines := []int{0}
	for i, r := range source {
		if r == '\n' {
			lines = append(lines, i+1)
		}
	}
	type edit struct {
		start int
		value rune
	}
	var edits []edit
	var visit func(*yaml.Node)
	visit = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && n.Style&yaml.DoubleQuotedStyle != 0 {
			i := lines[n.Line-1] + n.Column - 1
			// A node's position may start at a tag or anchor before its quote.
			for i < len(source) && source[i] != '"' {
				i++
			}
			for i++; i < len(source) && source[i] != '"'; i++ {
				if source[i] != '\\' {
					continue
				}
				if i+9 < len(source) && source[i+1] == 'U' {
					v, err := strconv.ParseUint(string(source[i+2:i+10]), 16, 32)
					if err == nil && v > 0xffff && v <= unicode.MaxRune && unicode.IsPrint(rune(v)) {
						edits = append(edits, edit{i, rune(v)})
					}
					i += 9
				} else {
					// Consume escaped backslashes/quotes as a pair; \\U is literal text.
					i++
				}
			}
		}
		for _, child := range n.Content {
			visit(child)
		}
	}
	visit(&doc)
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out strings.Builder
	start := 0
	for _, e := range edits {
		out.WriteString(string(source[start:e.start]))
		out.WriteRune(e.value)
		start = e.start + 10
	}
	out.WriteString(string(source[start:]))
	return out.String()
}
