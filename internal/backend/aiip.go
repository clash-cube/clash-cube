package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/settings"
)

// AIIPDetails describes the observed IP using Net.Coffee's database.
// Network is set when the API returned another address in a shared CIDR
// cache entry; those attributes describe the network, not this exact IP.
type AIIPDetails struct {
	City     string `json:"city"`
	Region   string `json:"region"`
	Operator string `json:"operator"`
	ASN      int    `json:"asn"`
	Kind     string `json:"kind"` // Residential | Datacenter | Mobile | Business | Unknown
	Network  string `json:"network"`
}

type aiIPEntry struct {
	details *AIIPDetails
	err     error
	expires time.Time
}

// Shared by services with the same exit IP. Failures are briefly cached
// too, to avoid repeatedly hitting an unavailable or rate-limited API.
type aiIPCache struct {
	mu      sync.Mutex
	entries map[string]aiIPEntry
	pending map[string]chan struct{}
}

func (c *aiIPCache) get(ctx context.Context, ip string, force bool, now time.Time, lookup func(context.Context, string) (*AIIPDetails, error)) (*AIIPDetails, error) {
	c.mu.Lock()
	if done := c.pending[ip]; done != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return c.get(ctx, ip, false, now, lookup)
		}
	}
	if entry, ok := c.entries[ip]; ok && !force && now.Before(entry.expires) {
		c.mu.Unlock()
		return entry.details, entry.err
	}
	if c.entries == nil {
		c.entries = make(map[string]aiIPEntry)
		c.pending = make(map[string]chan struct{})
	}
	done := make(chan struct{})
	c.pending[ip] = done
	c.mu.Unlock()
	details, err := lookup(ctx, ip)
	ttl := 10 * time.Minute
	if err != nil {
		ttl = 30 * time.Second
	}
	c.mu.Lock()
	// Bound memory even if an automatic group rotates through many IPs.
	for key, entry := range c.entries {
		if !now.Before(entry.expires) {
			delete(c.entries, key)
		}
	}
	if len(c.entries) >= 64 {
		var oldest string
		for key, entry := range c.entries {
			if oldest == "" || entry.expires.Before(c.entries[oldest].expires) {
				oldest = key
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[ip] = aiIPEntry{details, err, now.Add(ttl)}
	delete(c.pending, ip)
	close(done)
	c.mu.Unlock()
	return details, err
}

func lookupAIIP(ctx context.Context, ip string) (*AIIPDetails, error) {
	tr := &http.Transport{Proxy: http.ProxyURL(&url.URL{
		Scheme: "http", Host: net.JoinHostPort(proxyHost, strconv.Itoa(settings.Load().MixedPort)),
	})}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 4 * time.Second}
	return fetchAIIP(ctx, client, "https://ip.net.coffee/api/iprisk/", ip)
}

func fetchAIIP(ctx context.Context, client *http.Client, endpoint, ip string) (*AIIPDetails, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return nil, errors.New("IP attributes require a public IP")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+url.PathEscape(addr.String()), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IP attributes: HTTP %d", resp.StatusCode)
	}
	var data struct {
		IP           string `json:"ip"`
		CIDR         string `json:"cidr"`
		City         string `json:"city"`
		Region       string `json:"region"`
		Company      string `json:"company_name"`
		Organization string `json:"asOrganization"`
		ASN          int    `json:"asn"`
		CompanyType  string `json:"company_type"`
		Residential  bool   `json:"isResidential"`
		Datacenter   bool   `json:"is_datacenter"`
		Mobile       bool   `json:"is_mobile"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&data); err != nil {
		return nil, err
	}
	returned, err := netip.ParseAddr(data.IP)
	if err != nil {
		return nil, errors.New("IP attributes returned no valid IP")
	}
	out := &AIIPDetails{City: data.City, Region: data.Region, Operator: data.Company, ASN: data.ASN, Kind: "Unknown"}
	if returned.Unmap() != addr.Unmap() {
		prefix, err := netip.ParsePrefix(data.CIDR)
		if err != nil || !prefix.Contains(addr) || !prefix.Contains(returned) {
			return nil, errors.New("IP attributes returned an unrelated IP")
		}
		out.Network = prefix.String()
	}
	if out.Operator == "" {
		out.Operator = data.Organization
	}
	switch {
	case data.Datacenter || data.CompanyType == "hosting":
		out.Kind = "Datacenter"
	case data.Mobile:
		out.Kind = "Mobile"
	case data.Residential:
		out.Kind = "Residential"
	case data.CompanyType == "business":
		out.Kind = "Business"
	}
	return out, nil
}
