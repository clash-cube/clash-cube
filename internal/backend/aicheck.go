package backend

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

// An AI service sees one user behind every name its apps talk to, its
// telemetry included, and an account whose requests come from several
// addresses, or from this Mac's own, looks shared or out of region. The
// check reads where the rules send each name: a connection through the
// core that the node opens and that is closed before anything (TLS, a
// name, a path) is sent, so the service sees no request.

// AIHost is where the rules send one of a service's names.
type AIHost struct {
	Host        string   `json:"host"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
	Chain       []string `json:"chain"` // node first; ["REJECT"] when refused
	Error       string   `json:"error,omitempty"`
}

// AIRoute is how a service's names leave.
type AIRoute struct {
	Service string   `json:"service"`
	Hosts   []AIHost `json:"hosts"`
	// the node most names leave by, the first name's on a tie
	Node string `json:"node"`
	// consistent: all leave by Node; split: by several nodes; direct:
	// some leave from this Mac; refused: all are refused; failed: none
	// could be routed
	Verdict string `json:"verdict"`
	// groups on the way that pick a node themselves (url-test, fallback,
	// load-balance), so the route may change later
	Auto []string `json:"auto"`
}

// AIEgress is the address a service sees through one node, asked of it
// once by a name that leaves by that node.
type AIEgress struct {
	Node        string       `json:"node"`
	Names       int          `json:"names"` // how many of the service's names leave by Node
	IP          string       `json:"ip"`
	Loc         string       `json:"loc"`
	Chain       []string     `json:"chain"`
	Unsupported bool         `json:"unsupported"` // the service doesn't serve Loc
	Details     *AIIPDetails `json:"details"`     // optional Net.Coffee metadata
	Error       string       `json:"error,omitempty"`
}

// AICheck is a service's routes, the address each of their nodes shows
// it, and what the two say together.
type AICheck struct {
	Route AIRoute `json:"route"`
	// The addresses are each node's, asked of Cloudflare through it rather
	// than of the service, which a node splitting by destination may not
	// show it.
	NodeEgress bool `json:"nodeEgress"`
	// one per node the names leave by, the one most leave by first
	Egress []AIEgress `json:"egress"`
	// the worst finding: consistent (one node, or nodes sharing one
	// address), direct (every name leaves from this Mac), partlyDirect,
	// split, unsupported (an address in a region the service doesn't
	// serve), refused or failed
	Status string `json:"status"`
	Level  string `json:"level"` // good | warn | bad | muted
}

type aiService struct {
	name string
	// Representative TCP/443 targets, not suffix patterns. The first host
	// provides the egress trace (unless nodeEgress) and wins a tie for the
	// main node. Shared telemetry may intentionally follow a different
	// service's rules; include it to reveal that split.
	hosts []string
	// countries the service's supported list leaves out, among those a
	// node is likely in: Claude's from anthropic.com/supported-countries,
	// OpenAI's from help.openai.com/en/articles/7947663 (October 2026)
	unsupported []string
	// The service's names don't answer Cloudflare's trace: ask it of
	// Cloudflare through each node instead.
	nodeEgress bool
}

// Coverage reviewed against the supplied clash-config-mix-no-tuic config
// and MetaCubeX/meta-rules-dat's geo/geosite/{openai,anthropic}.list on
// 2026-10-06. Keep exact third-party endpoints; sample suffix rules using
// their base domain and known application hosts. This is not an exhaustive
// expansion of wildcards, nor a test of UDP, PROCESS-NAME or IP/ASN rules.
var aiServices = []aiService{
	{name: "OpenAI", hosts: []string{
		"chatgpt.com", "chat.openai.com", "ab.chatgpt.com", "api.openai.com", "auth.openai.com",
		"ios.chat.openai.com", "cdn.oaistatic.com", "files.oaiusercontent.com",
		"openai.com", "oaistatic.com", "oaiusercontent.com", "oaistatsig.com",
		"chatgpt.site", "crixet.com", "chat.com", "sora.com", "ai.com",
		// Voice and realtime infrastructure; TCP only, not the UDP media path.
		"chatgpt.livekit.cloud", "host.livekit.cloud", "turn.livekit.cloud",
		// Exact CDN, telemetry and survey endpoints from the OpenAI ruleset.
		"openaiapi-site.azureedge.net", "openaiassets.blob.core.windows.net",
		"openaicom-api-bdcpf8c6d2e9atf6.z01.azurefd.net", "openaicom.imgix.net",
		"openaicomproductionae4b.blob.core.windows.net", "production-openaicom-storage.azureedge.net",
		"openai.com.cdn.cloudflare.net", "browser-intake-datadoghq.com",
		"o33249.ingest.sentry.io", "openai.qualtrics.com",
	}, unsupported: []string{"CN", "HK", "MO", "RU", "BY", "IR", "KP", "CU", "SY", "VE"}},
	{name: "Claude", hosts: []string{
		"claude.ai", "api.anthropic.com", "claude.com", "platform.claude.com", "anthropic.com",
		"clau.de", "claude.dev", "claudeusercontent.com", "claudemcpclient.com", "claudemcpcontent.com",
		"servd-anthropic-website.b-cdn.net", "anthropic.com.cdn.cloudflare.net",
		"anthropic.auth0.com", "anthropic-com.ghost.io", "cdn.usefathom.com",
		// The supplied config includes shared telemetry suffixes and keyword
		// rules. Base domains sample those rules without inventing app hosts.
		"sentry.io", "statsigapi.net", "datadoghq.com", "sift.com",
		"intercom.io", "intercomcdn.com", "api-iam.intercom.io",
		"browser-intake-us5-datadoghq.com", "http-intake.logs.us5.datadoghq.com",
	}, unsupported: []string{"CN", "HK", "MO", "RU", "BY", "IR", "KP", "CU", "SY", "VE", "AF", "MM", "YE"}},
	// The config's google-gemini ruleset covers Google's AI products as a
	// whole. Sample its 2026-10-06 list: every product, and every suffix
	// outside google.com and googleapis.com, which a broader rule placed
	// above the ruleset would catch as a whole. Every name is a real
	// connection through its node, so skip aliases under a sampled suffix.
	{name: "Google AI", nodeEgress: true, hosts: []string{
		// Gemini app
		"gemini.google.com", "gemini.google", "bard.google.com", "geminiweb-pa.clients6.google.com", "gemini.gstatic.com",
		// Gemini API and AI Studio
		"generativelanguage.googleapis.com", "ai.google.dev", "aistudio.google.com", "ai.studio",
		"alkalimakersuite-pa.clients6.google.com",
		// Gemini Code Assist and CLI
		"cloudcode-pa.googleapis.com", "cloudaicompanion.googleapis.com",
		// NotebookLM, Antigravity, Labs (ImageFX, Whisk), Jules, Opal, Flow,
		// Stitch, DeepMind and the generative AI portal
		"notebooklm.google.com", "notebooklm-pa.googleapis.com", "notebooklm.google", "notebook.google",
		"antigravity.google", "antigravity-pa.googleapis.com", "antigravity-unleash.goog",
		"labs.google", "aisandbox-pa.googleapis.com",
		"jules.google.com", "jules.google", "opal.google", "flow.google", "stitch.withgoogle.com",
		"deepmind.google", "deepmind.com", "generativeai.google",
		// Chrome DevTools AI and other client backends in the ruleset
		"aida.googleapis.com", "aicode.googleapis.com", "robinfrontend-pa.googleapis.com",
		"proactivebackend-pa.googleapis.com", "geller-pa.googleapis.com",
	}},
	// Meta AI's own names first, so they decide the main node, then the
	// Facebook site and CDN it signs in and loads assets through. The
	// broader facebook/instagram/whatsapp rulesets are not Meta AI.
	{name: "Meta AI", nodeEgress: true, hosts: []string{
		"meta.ai", "www.meta.ai", "facebook.com", "static.xx.fbcdn.net",
	}},
}

func aiServiceNamed(name string) (aiService, error) {
	for _, s := range aiServices {
		if s.name == name {
			return s, nil
		}
	}
	return aiService{}, fmt.Errorf("unknown service %q", name)
}

// AICheck reads where the rules send each of the service's names and asks
// the service, through every node they leave by, the address it sees.
// force bypasses the IP attribute cache.
func (b *Backend) AICheck(service string, force bool) (AICheck, error) {
	s, err := aiServiceNamed(service)
	if err != nil {
		return AICheck{}, err
	}
	c, err := b.Client()
	if err != nil {
		return AICheck{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out := aiCheck(ctx, c, s, 443)
	// Enrichment is optional: an unavailable database must not hide the
	// service's observed address or replace it with a cached subnet address.
	var wg sync.WaitGroup
	for i := range out.Egress {
		if e := &out.Egress[i]; e.IP != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.Details, _ = b.aiIPs.get(ctx, e.IP, force, time.Now(), lookupAIIP)
			}()
		}
	}
	wg.Wait()
	return out, nil
}

// aiCheck routes the service's names on port, then asks for the egress
// address through each node they leave by: the first name's trace runs
// alongside the routes, the other nodes' after them.
func aiCheck(ctx context.Context, c *mihomoapi.Client, s aiService, port int) AICheck {
	trace := func(host string) string {
		if port == 443 {
			return "https://" + host + "/cdn-cgi/trace"
		}
		return "http://" + net.JoinHostPort(host, fmt.Sprint(port)) + "/cdn-cgi/trace"
	}
	if s.nodeEgress {
		// Cloudflare's own trace stands in for the service's; in tests,
		// the first name's server answers it.
		u := proxyTrace
		if port != 443 {
			u = trace(s.hosts[0])
		}
		route := aiRoutes(ctx, c, s.name, s.hosts, port)
		return s.result(route, nodeEgress(ctx, c, s, route.Hosts, u))
	}
	var first AIEgress
	var firstErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		first, firstErr = aiEgress(ctx, c, s, trace(s.hosts[0]))
	}()
	route := aiRoutes(ctx, c, s.name, s.hosts, port)
	<-done

	egress := egressNodes(route.Hosts)
	var wg sync.WaitGroup
	for i := range egress {
		e := &egress[i]
		if firstErr == nil && len(first.Chain) > 0 && first.Chain[0] == e.Node {
			first.Node, first.Names = e.Node, e.Names
			*e = first
			continue
		}
		// any name of the node's will do; a few, as not every host answers
		// Cloudflare's trace
		var tries []string
		for _, h := range route.Hosts {
			if h.Error == "" && len(h.Chain) > 0 && h.Chain[0] == e.Node && h.Host != s.hosts[0] && len(tries) < 3 {
				tries = append(tries, h.Host)
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Error = "no name that leaves by this node answered"
			for _, h := range tries {
				if got, err := aiEgress(ctx, c, s, trace(h)); err == nil {
					got.Node, got.Names = e.Node, e.Names
					*e = got
					return
				}
			}
		}()
	}
	wg.Wait()
	return s.result(route, egress)
}

// nodeEgress asks Cloudflare's trace at traceURL through each node the
// names leave by, named to the core rather than reached by a name's rule.
func nodeEgress(ctx context.Context, c *mihomoapi.Client, s aiService, hosts []AIHost, traceURL string) []AIEgress {
	egress := egressNodes(hosts)
	var wg sync.WaitGroup
	for i := range egress {
		e := &egress[i]
		// the chain of the node's first name, for where the address is from
		var chain []string
		for _, h := range hosts {
			if h.Error == "" && len(h.Chain) > 0 && h.Chain[0] == e.Node {
				chain = h.Chain
				break
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			var got AIEgress
			body, err := c.Trace(ctx, e.Node, traceURL)
			if err == nil {
				got, err = parseEgress(s, body, chain)
			}
			if err != nil {
				e.Error = err.Error()
				return
			}
			got.Node, got.Names = e.Node, e.Names
			*e = got
		}()
	}
	wg.Wait()
	return egress
}

// result judges the routes with the addresses seen. Without a list of the
// regions the service leaves out, DIRECT shows it this Mac's region, which
// it may not serve.
func (s aiService) result(route AIRoute, egress []AIEgress) AICheck {
	status, level := judgeCheck(route, egress)
	if status == "direct" && len(s.unsupported) == 0 {
		level = "muted"
	}
	return AICheck{Route: route, NodeEgress: s.nodeEgress, Egress: egress, Status: status, Level: level}
}

// egressNodes lists the nodes routed names leave by, most names first and
// the first name's on a tie, as judgeRoutes picks the main node.
func egressNodes(hosts []AIHost) []AIEgress {
	out := []AIEgress{}
	for _, h := range hosts {
		if h.Error != "" || len(h.Chain) == 0 || refused(h.Chain[0]) {
			continue
		}
		i := slices.IndexFunc(out, func(e AIEgress) bool { return e.Node == h.Chain[0] })
		if i < 0 {
			out = append(out, AIEgress{Node: h.Chain[0]})
			i = len(out) - 1
		}
		out[i].Names++
	}
	slices.SortStableFunc(out, func(a, b AIEgress) int { return b.Names - a.Names })
	return out
}

// judgeCheck weighs the routes with the addresses the service saw. Names
// on several nodes are fine if those nodes share one address, and every
// name going DIRECT is fine where the service serves this Mac's region.
func judgeCheck(r AIRoute, egress []AIEgress) (status, level string) {
	switch r.Verdict {
	case "failed":
		return "failed", "muted"
	case "refused":
		return "refused", "warn"
	}
	for _, e := range egress {
		if e.Unsupported {
			return "unsupported", "bad"
		}
	}
	if len(egress) == 1 {
		if egress[0].Node == "DIRECT" {
			return "direct", "good"
		}
		return "consistent", "good"
	}
	ips := map[string]bool{}
	for _, e := range egress {
		if e.IP == "" {
			ips = nil
			break
		}
		ips[e.IP] = true
	}
	switch {
	case len(ips) == 1:
		return "consistent", "good"
	case r.Verdict == "direct":
		return "partlyDirect", "bad"
	}
	return "split", "warn"
}

func aiRoutes(ctx context.Context, c *mihomoapi.Client, service string, hosts []string, port int) AIRoute {
	out := AIRoute{Service: service, Hosts: make([]AIHost, len(hosts)), Auto: []string{}}
	targets := make([]string, len(hosts))
	for i, h := range hosts {
		targets[i] = net.JoinHostPort(h, fmt.Sprint(port))
	}
	for i, r := range routesOf(ctx, c, targets) {
		out.Hosts[i] = AIHost{Host: hosts[i], Rule: r.conn.Rule, RulePayload: r.conn.RulePayload, Chain: r.conn.Chains}
		if r.err != nil {
			out.Hosts[i].Error = r.err.Error()
		}
	}
	out.Node, out.Verdict = judgeRoutes(out.Hosts)
	if ps, err := c.Proxies(ctx); err == nil {
		for _, h := range out.Hosts {
			for _, g := range h.Chain {
				// a load balancer spreads names over its nodes by itself too
				typ := ps[g].Type
				if (autoGroups[typ] || typ == "LoadBalance") && !slices.Contains(out.Auto, g) {
					out.Auto = append(out.Auto, g)
				}
			}
		}
	}
	return out
}

func refused(node string) bool { return node == "REJECT" || node == "REJECT-DROP" }

// judgeRoutes names the node most of the routed names leave by and says
// whether they all do.
func judgeRoutes(hosts []AIHost) (node, verdict string) {
	count := map[string]int{}
	var order []string
	rejected := 0
	for _, h := range hosts {
		if h.Error != "" || len(h.Chain) == 0 {
			continue
		}
		n := h.Chain[0]
		if refused(n) {
			rejected++
			continue
		}
		if count[n] == 0 {
			order = append(order, n)
		}
		count[n]++
	}
	for _, n := range order {
		if count[n] > count[node] {
			node = n
		}
	}
	switch {
	case len(order) == 0 && rejected > 0:
		return "", "refused"
	case len(order) == 0:
		return "", "failed"
	case count["DIRECT"] > 0:
		return node, "direct"
	case len(order) > 1:
		return node, "split"
	}
	return node, "consistent"
}

// aiEgress asks one of the service's names, through the core, the address
// it sees: one plain request for Cloudflare's trace, which its edge
// answers.
func aiEgress(ctx context.Context, c *mihomoapi.Client, s aiService, traceURL string) (AIEgress, error) {
	body, chain := throughCore(ctx, c, http.MethodGet, traceURL)
	if chain == nil {
		return AIEgress{}, errors.New("the request through the core failed")
	}
	return parseEgress(s, body, chain)
}

// parseEgress reads the address and region from a trace fetched along
// chain.
func parseEgress(s aiService, body string, chain []string) (AIEgress, error) {
	e := AIEgress{Chain: chain}
	e.IP, e.Loc = parseTrace(body)
	if net.ParseIP(e.IP) == nil {
		return AIEgress{}, errors.New("the trace gave no valid egress IP")
	}
	e.Unsupported = slices.Contains(s.unsupported, e.Loc)
	return e, nil
}
