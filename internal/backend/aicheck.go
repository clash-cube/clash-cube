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

// AIEgress is the address a service sees, asked of it once.
type AIEgress struct {
	IP          string       `json:"ip"`
	Loc         string       `json:"loc"`
	Chain       []string     `json:"chain"`
	Unsupported bool         `json:"unsupported"` // the service doesn't serve Loc
	Details     *AIIPDetails `json:"details"`     // optional Net.Coffee metadata
}

type aiService struct {
	name string
	// Representative TCP/443 targets, not suffix patterns. The first host
	// provides the egress trace. Shared telemetry may intentionally follow
	// a different service's rules; include it to reveal that split.
	hosts []string
	// countries the service's supported list leaves out, among those a
	// node is likely in: Claude's from anthropic.com/supported-countries,
	// OpenAI's from help.openai.com/en/articles/7947663 (October 2026)
	unsupported []string
}

// Coverage reviewed against the supplied clash-config-mix-no-tuic config
// and MetaCubeX/meta-rules-dat's geo/geosite/{openai,anthropic}.list on
// 2026-10-06. Keep exact third-party endpoints; sample suffix rules using
// their base domain and known application hosts. This is not an exhaustive
// expansion of wildcards, nor a test of UDP, PROCESS-NAME or IP/ASN rules.
var aiServices = []aiService{
	{"OpenAI", []string{
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
	}, []string{"CN", "HK", "MO", "RU", "BY", "IR", "KP", "CU", "SY", "VE"}},
	{"Claude", []string{
		"claude.ai", "api.anthropic.com", "claude.com", "platform.claude.com", "anthropic.com",
		"clau.de", "claude.dev", "claudeusercontent.com", "claudemcpclient.com", "claudemcpcontent.com",
		"servd-anthropic-website.b-cdn.net", "anthropic.com.cdn.cloudflare.net",
		"anthropic.auth0.com", "anthropic-com.ghost.io", "cdn.usefathom.com",
		// The supplied config includes shared telemetry suffixes and keyword
		// rules. Base domains sample those rules without inventing app hosts.
		"sentry.io", "statsigapi.net", "datadoghq.com", "sift.com",
		"intercom.io", "intercomcdn.com", "api-iam.intercom.io",
		"browser-intake-us5-datadoghq.com", "http-intake.logs.us5.datadoghq.com",
	}, []string{"CN", "HK", "MO", "RU", "BY", "IR", "KP", "CU", "SY", "VE", "AF", "MM", "YE"}},
}

func aiServiceNamed(name string) (aiService, error) {
	for _, s := range aiServices {
		if s.name == name {
			return s, nil
		}
	}
	return aiService{}, fmt.Errorf("unknown service %q", name)
}

// AIRoutes reads where the rules send each of the service's names.
func (b *Backend) AIRoutes(service string) (AIRoute, error) {
	s, err := aiServiceNamed(service)
	if err != nil {
		return AIRoute{}, err
	}
	c, err := b.Client()
	if err != nil {
		return AIRoute{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return aiRoutes(ctx, c, s.name, s.hosts, 443), nil
}

func aiRoutes(ctx context.Context, c *mihomoapi.Client, service string, hosts []string, port int) AIRoute {
	out := AIRoute{Service: service, Hosts: make([]AIHost, len(hosts)), Auto: []string{}}
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := routeOf(ctx, c, net.JoinHostPort(h, fmt.Sprint(port)))
			out.Hosts[i] = AIHost{Host: h, Rule: r.Rule, RulePayload: r.RulePayload, Chain: r.Chains}
			if err != nil {
				out.Hosts[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
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

// AIEgress asks the service's first name, through the core, the address
// it sees: one plain request for Cloudflare's trace, which its edge
// answers.
func (b *Backend) AIEgress(service string, force bool) (AIEgress, error) {
	s, err := aiServiceNamed(service)
	if err != nil {
		return AIEgress{}, err
	}
	c, err := b.Client()
	if err != nil {
		return AIEgress{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := aiEgress(ctx, c, s, "https://"+s.hosts[0]+"/cdn-cgi/trace")
	if err != nil {
		return e, err
	}
	// Enrichment is optional: an unavailable database must not hide the
	// service's observed address or replace it with a cached subnet address.
	e.Details, _ = b.aiIPs.get(ctx, e.IP, force, time.Now(), lookupAIIP)
	return e, nil
}

func aiEgress(ctx context.Context, c *mihomoapi.Client, s aiService, traceURL string) (AIEgress, error) {
	body, chain := throughCore(ctx, c, http.MethodGet, traceURL)
	if chain == nil {
		return AIEgress{}, errors.New("the request through the core failed")
	}
	e := AIEgress{Chain: chain}
	e.IP, e.Loc = parseTrace(body)
	if net.ParseIP(e.IP) == nil {
		return AIEgress{}, errors.New("the service returned no valid egress IP")
	}
	e.Unsupported = slices.Contains(s.unsupported, e.Loc)
	return e, nil
}
