package modules

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dlclark/regexp2"
	"go.yaml.in/yaml/v3"
)

// Route is a module that sends one service through a policy of its own,
// as Surge's service groups do. Like every module it is laid over every
// profile, and its group takes the nodes of the one in use: all of them, a
// region's, or picked ones. Picked nodes are named per profile, since a
// name belongs to one subscription, and keywords hold for all. A group
// with no node refuses its connections rather than letting them out
// direct: picking says these nodes and no others.
type Route struct {
	Service  string              `json:"service"`            // a Services name
	Policy   string              `json:"policy"`             // select, url-test or DIRECT
	Region   string              `json:"region,omitempty"`   // a Regions key
	Pick     bool                `json:"pick,omitempty"`     // only Nodes and Keywords
	Nodes    map[string][]string `json:"nodes,omitempty"`    // by profile ID: names, matched whole
	Keywords []string            `json:"keywords,omitempty"` // matched anywhere in a name, any case
	Upstream map[string]string   `json:"upstream,omitempty"` // front proxy, by profile ID
}

// Service is what a route matches: the rules that pick out its traffic.
type Service struct {
	Name   string   `json:"name"` // also its group's
	Hint   string   `json:"hint"`
	Rules  []string `json:"rules"`            // TYPE,payload, with options after the policy
	Region string   `json:"region,omitempty"` // the one it starts with
}

var Services = []Service{
	{Name: "Google", Hint: "Search, Gmail, Maps and Google's other sites and apps", Rules: []string{"GEOSITE,google"}},
	{Name: "YouTube", Hint: "Videos, Shorts and YouTube Music", Rules: []string{"GEOSITE,youtube"}},
	{Name: "OpenAI", Hint: "ChatGPT and the OpenAI API, which refuse some regions", Rules: []string{
		"PROCESS-NAME,ChatGPT", "DOMAIN-SUFFIX,ai.com", "GEOSITE,openai",
	}, Region: "us"},
	{Name: "Claude", Hint: "Claude apps, Anthropic domains and IP ranges, including shared telemetry services", Rules: []string{
		"PROCESS-NAME,Claude", "PROCESS-NAME,Claude Helper", "PROCESS-NAME,claude",
		// Keep core domains inline so coverage does not depend on the geosite version.
		"DOMAIN-SUFFIX,anthropic.com", "DOMAIN-SUFFIX,claude.ai", "DOMAIN-SUFFIX,claude.com",
		"DOMAIN-SUFFIX,clau.de", "DOMAIN-SUFFIX,claudeusercontent.com",
		"DOMAIN-SUFFIX,claudemcpclient.com", "DOMAIN-SUFFIX,claudemcpcontent.com",
		"DOMAIN,servd-anthropic-website.b-cdn.net",
		// These shared services deliberately follow the reference profile's Claude policy.
		"DOMAIN-SUFFIX,sentry.io", "DOMAIN-SUFFIX,statsigapi.net", "DOMAIN-SUFFIX,datadoghq.com",
		"DOMAIN-SUFFIX,intercom.io", "DOMAIN-SUFFIX,intercomcdn.com",
		"DOMAIN,anthropic.com.cdn.cloudflare.net", "DOMAIN,anthropic.auth0.com", "DOMAIN,anthropic-com.ghost.io",
		"DOMAIN,browser-intake-us5-datadoghq.com", "DOMAIN,cdn.usefathom.com",
		"DOMAIN-KEYWORD,datadog", "DOMAIN-KEYWORD,sift", "DOMAIN-KEYWORD,sentry",
		"GEOSITE,anthropic",
		"IP-CIDR,160.79.104.0/21,no-resolve", "IP-CIDR6,2607:6bc0::/32,no-resolve", "IP-ASN,399358,no-resolve",
	}, Region: "us"},
	{Name: "AI services", Hint: "Non-Chinese AI services and GrowthBook; place above OpenAI and Claude so their rules take priority", Rules: []string{
		"DOMAIN-SUFFIX,growthbook.io", "GEOSITE,category-ai-chat-!cn",
	}, Region: "us"},
	// Telegram's apps connect by address: its published ranges
	// (core.telegram.org/resources/cidr.txt)
	{Name: "Telegram", Hint: "Telegram's sites and the addresses its apps connect to", Rules: []string{
		"GEOSITE,telegram",
		"IP-CIDR,91.108.4.0/22,no-resolve", "IP-CIDR,91.108.8.0/21,no-resolve", "IP-CIDR,91.108.16.0/21,no-resolve",
		"IP-CIDR,91.108.56.0/22,no-resolve", "IP-CIDR,95.161.64.0/20,no-resolve", "IP-CIDR,149.154.160.0/20,no-resolve",
		"IP-CIDR,185.76.151.0/24,no-resolve", "IP-CIDR6,2001:67c:4e8::/48,no-resolve", "IP-CIDR6,2001:b28:f23c::/46,no-resolve",
	}},
}

// Region picks nodes by name, as subscriptions write them.
type Region struct {
	Key    string `json:"key"`
	Name   string `json:"name"` // English, translated on the page
	Filter string `json:"-"`
}

// Regions' latin codes stand alone, so "us" doesn't match Russia.
var Regions = []Region{
	{"hk", "Hong Kong", `(?i)港|🇭🇰|hong ?kong|(^|[^a-z])hk([^a-z]|$)`},
	{"tw", "Taiwan", `(?i)台湾|台北|🇹🇼|taiwan|taipei|(^|[^a-z])tw([^a-z]|$)`},
	{"jp", "Japan", `(?i)日本|东京|大阪|🇯🇵|japan|tokyo|osaka|(^|[^a-z])jp([^a-z]|$)`},
	{"sg", "Singapore", `(?i)新加坡|狮城|🇸🇬|singapore|(^|[^a-z])sg([^a-z]|$)`},
	{"us", "United States", `(?i)美国|洛杉矶|圣何塞|硅谷|西雅图|纽约|🇺🇸|united states|america|los angeles|san jose|silicon valley|seattle|new york|(^|[^a-z])us([^a-z]|$)`},
}

func service(name string) (Service, bool) {
	for _, s := range Services {
		if s.Name == name {
			return s, true
		}
	}
	return Service{}, false
}

func region(key string) (Region, bool) {
	for _, r := range Regions {
		if r.Key == key {
			return r, true
		}
	}
	return Region{}, false
}

// Check refuses a route to a service, policy or region the app doesn't know.
func (r Route) Check() error {
	if _, ok := service(r.Service); !ok {
		return fmt.Errorf("no service %q", r.Service)
	}
	switch r.Policy {
	case "select", "url-test", "DIRECT":
	default:
		return fmt.Errorf("no policy %q", r.Policy)
	}
	if _, ok := region(r.Region); r.Region != "" && !ok {
		return fmt.Errorf("no region %q", r.Region)
	}
	if r.Region != "" && r.Pick {
		return errors.New("a route takes a region or picked nodes, not both")
	}
	if !r.Pick && (len(r.Nodes) > 0 || len(r.Keywords) > 0) {
		return errors.New("nodes and keywords are for picked nodes")
	}
	all := append([]string{}, r.Keywords...)
	for id, name := range r.Upstream {
		if id == "" {
			return errors.New("upstream without a profile")
		}
		all = append(all, name)
	}
	for id, names := range r.Nodes {
		if id == "" {
			return errors.New("picked nodes without a profile")
		}
		all = append(all, names...)
	}
	for _, n := range all {
		if strings.TrimSpace(n) == "" || strings.ContainsAny(n, "\r\n") {
			return fmt.Errorf("bad node name or keyword %q", n)
		}
	}
	return nil
}

// none matches no name: picking with nothing picked for the profile
const none = "(?!)"

// filter is the regexp the route's group picks the profile's nodes with,
// "" for all.
func (r Route) filter(profile string) string {
	return r.NodeFilter(profile, "")
}

// NodeFilter matches effective node names, optionally after a private prefix.
func (r Route) NodeFilter(profile, prefix string) string {
	// mihomo splits filters at backticks, including ones inside node names.
	quote := func(s string) string { return strings.ReplaceAll(regexp2.Escape(s), "`", `\x60`) }
	start := "^" + quote(prefix)
	if reg, ok := region(r.Region); ok {
		if prefix != "" {
			return start + ".*?(?:" + strings.ReplaceAll(reg.Filter, "(^|", "((?<="+quote(prefix)+")|") + ")"
		}
		return reg.Filter
	}
	if !r.Pick {
		if prefix != "" {
			return start
		}
		return ""
	}
	nodes := r.Nodes[profile]
	if len(nodes) == 0 && len(r.Keywords) == 0 {
		return none
	}
	var alts []string
	if len(nodes) > 0 {
		var names []string
		for _, n := range nodes {
			names = append(names, quote(prefix+n))
		}
		alts = append(alts, "^(?:"+strings.Join(names, "|")+")$")
	}
	if len(r.Keywords) > 0 {
		var kws []string
		for _, k := range r.Keywords {
			kws = append(kws, quote(k))
		}
		keywordFilter := "(?i:" + strings.Join(kws, "|") + ")"
		if prefix != "" {
			keywordFilter = start + ".*?" + keywordFilter
		}
		alts = append(alts, keywordFilter)
	}
	return strings.Join(alts, "|")
}

// Takes says whether the route's group takes the node of that name, in
// the profile of that ID.
func (r Route) Takes(profile, name string) bool {
	f := r.filter(profile)
	if f == "" {
		return true
	}
	ok, _ := regexp2.MustCompile(f, regexp2.None).MatchString(name)
	return ok
}

// Suffix sets the route's group apart from a profile's group of the same
// name, which it can't share. The page looks for it the same way.
const Suffix = " (ClashCube)"

// Group is the name of the group the route sends its service to, given
// the names the configuration has already; "" when it goes direct.
func (r Route) Group(taken func(string) bool) string {
	if r.Policy == "DIRECT" {
		return ""
	}
	if taken != nil && taken(r.Service) {
		return r.Service + Suffix
	}
	return r.Service
}

// GroupIn is the name the route's group has in a configuration that has
// the names has: set apart if the profile had its name.
func (r Route) GroupIn(has func(string) bool) string {
	if r.Policy == "DIRECT" {
		return ""
	}
	if has(r.Service + Suffix) {
		return r.Service + Suffix
	}
	return r.Service
}

// Body is the module the route makes over the profile of that ID, in a
// configuration with the names taken has (nil for none). declared identifies
// nodes defined in top-level proxies, which can be referenced directly. Without
// that context, names stay filtered so provider nodes remain valid references.
func (r Route) Body(profile string, taken, declared func(string) bool) (string, error) {
	if err := r.Check(); err != nil {
		return "", err
	}
	s, _ := service(r.Service)
	group := r.Group(taken)
	policy := group
	if policy == "" {
		policy = "DIRECT"
	}
	var rules []string
	for _, rule := range s.Rules {
		// options such as no-resolve come after the policy
		f := strings.SplitN(rule, ",", 3)
		line := f[0] + "," + f[1] + "," + policy
		if len(f) == 3 {
			line += "," + f[2]
		}
		rules = append(rules, line)
	}
	m := map[string]any{"prepend-rules": rules}
	if group != "" {
		// include-all takes the profile's nodes and its providers'. With
		// none in the scope the group holds REJECT: the configuration
		// still loads, and the service isn't let out direct (mihomo's
		// default, COMPATIBLE).
		g := map[string]any{"name": group, "type": r.Policy, "include-all": true, "empty-fallback": "REJECT"}
		if r.Pick && len(r.Keywords) == 0 {
			var direct, dynamic []string
			seen := map[string]bool{}
			for _, name := range r.Nodes[profile] {
				if seen[name] {
					continue
				}
				seen[name] = true
				if declared != nil && declared(name) {
					direct = append(direct, name)
				} else {
					dynamic = append(dynamic, name)
				}
			}
			if len(dynamic) > 0 {
				// Provider nodes are not in mihomo's top-level proxy map.
				// Missing picks also stay here: no match safely falls back to REJECT.
				remaining := Route{Pick: true, Nodes: map[string][]string{profile: dynamic}}
				g["filter"] = remaining.filter(profile)
			} else {
				delete(g, "include-all")
				if len(direct) == 0 {
					direct = []string{"REJECT"}
				}
			}
			if len(direct) > 0 {
				g["proxies"] = direct
			}
		} else if f := r.filter(profile); f != "" {
			g["filter"] = f
		}
		m["append-proxy-groups"] = []any{g}
	}
	b, err := yaml.Marshal(m)
	return ReadableYAML(string(b)), err
}

// Matches says whether the region takes the node of that name.
func (r Region) Matches(name string) bool { return Route{Region: r.Key}.Takes("", name) }

// CopyPicks gives the profile to the nodes picked for the profile from,
// as a copy of it has the same nodes.
func CopyPicks(from, to string) error {
	return editPicks(func(r *Route) bool {
		changed := false
		if names, ok := r.Nodes[from]; ok {
			r.Nodes[to] = append([]string{}, names...)
			changed = true
		}
		if name, ok := r.Upstream[from]; ok {
			r.Upstream[to] = name
			changed = true
		}
		return changed
	})
}

// ForgetPicks drops the nodes picked for a profile that is gone.
func ForgetPicks(profile string) error {
	return editPicks(func(r *Route) bool {
		_, nodes := r.Nodes[profile]
		_, upstream := r.Upstream[profile]
		delete(r.Nodes, profile)
		delete(r.Upstream, profile)
		return nodes || upstream
	})
}

func editPicks(edit func(*Route) bool) error {
	ms := List()
	changed := false
	for _, m := range ms {
		if m.Route == nil {
			continue
		}
		changed = edit(m.Route) || changed
	}
	if !changed {
		return nil
	}
	return Save(ms)
}
