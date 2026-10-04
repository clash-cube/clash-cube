package modules

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestReadableYAMLPreservesMeaning(t *testing.T) {
	body := `# Keep comments: "\U0001F1FA"
中文: "\U0001F1FA\U0001F1F8\\ US\\ \\(01\\)"
literal: "\\U0001F1FA"
single: '\U0001F1FA'
plain: \U0001F1FA
block: |
  "\U0001F1FA"
tagged: !!str &node "\U0001F680"
alias: *node
"\U0001F600": "quote: \"\U0001F680\"; control: \U00000001"
multiline: "first
  \U0001F600"
`
	for _, body := range []string{body, strings.ReplaceAll(body, "\n", "\r\n")} {
		got := ReadableYAML(body)
		if !strings.Contains(got, `中文: "🇺🇸\\ US\\ \\(01\\)"`) || !strings.Contains(got, `!!str &node "🚀"`) || !strings.Contains(got, `"😀": "quote: \"🚀\"; control: \U00000001"`) {
			t.Fatalf("emoji were not made readable: %s", got)
		}
		for _, untouched := range []string{`# Keep comments: "\U0001F1FA"`, `literal: "\\U0001F1FA"`, `single: '\U0001F1FA'`, `plain: \U0001F1FA`, `  "\U0001F1FA"`} {
			if !strings.Contains(got, untouched) {
				t.Errorf("changed literal text %q", untouched)
			}
		}
		var before, after any
		if err := yaml.Unmarshal([]byte(body), &before); err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal([]byte(got), &after); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("changed YAML meaning: before %v, after %v", before, after)
		}
		if ReadableYAML(got) != got {
			t.Error("conversion is not idempotent")
		}
	}
	invalid := `name: "\UFFFFFFFF"`
	if ReadableYAML(invalid) != invalid {
		t.Error("invalid YAML was changed")
	}
}

func TestRouteEmojiFilter(t *testing.T) {
	r := Route{Service: "Claude", Policy: "select", Pick: true, Keywords: []string{"🇺🇸", `\U0001F680`}, Nodes: map[string][]string{"p": {"🇸🇬 SG (01)"}}}
	body, err := r.Body("p", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "🇸🇬") || !strings.Contains(body, "🇺🇸") {
		t.Fatalf("escaped flags in route YAML: %s", body)
	}
	g := mustGroup(t, r)
	if g["filter"] != r.filter("p") {
		t.Fatalf("filter changed after YAML round trip: %s", g["filter"])
	}
}
