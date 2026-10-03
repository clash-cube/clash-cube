// Package mihomoapi is a client for mihomo's external controller (REST plus
// the streaming endpoints, read as JSON lines over plain HTTP).
package mihomoapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	base   string // http://127.0.0.1:port
	secret string
	http   *http.Client
	stream *http.Client
}

func New(addr, secret string) *Client {
	return &Client{
		base:   "http://" + addr,
		secret: secret,
		http:   &http.Client{Timeout: 15 * time.Second},
		stream: &http.Client{},
	}
}

// APIError is a non-2xx answer, with mihomo's message.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("mihomo: %d %s", e.Status, e.Message) }

func (c *Client) req(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if c.secret != "" {
		r.Header.Set("Authorization", "Bearer "+c.secret)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if json.Unmarshal(b, &e) != nil || e.Message == "" {
			e.Message = string(bytes.TrimSpace(b))
		}
		return &APIError{Status: resp.StatusCode, Message: e.Message}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	err := c.req(ctx, http.MethodGet, "/version", nil, &v)
	return v.Version, err
}

// Configs is the running general configuration (a subset).
type Configs struct {
	Mode      string `json:"mode"`
	MixedPort int    `json:"mixed-port"`
	AllowLan  bool   `json:"allow-lan"`
	IPv6      bool   `json:"ipv6"`
	LogLevel  string `json:"log-level"`
	Tun       struct {
		Enable bool   `json:"enable"`
		Stack  string `json:"stack"`
		Device string `json:"device"`
	} `json:"tun"`
}

func (c *Client) Configs(ctx context.Context) (Configs, error) {
	var cfg Configs
	err := c.req(ctx, http.MethodGet, "/configs", nil, &cfg)
	return cfg, err
}

// PatchConfigs changes running settings, e.g. {"mode":"global"}.
func (c *Client) PatchConfigs(ctx context.Context, patch map[string]any) error {
	return c.req(ctx, http.MethodPatch, "/configs", patch, nil)
}

// ReloadConfigs makes the core load the configuration at path again.
func (c *Client) ReloadConfigs(ctx context.Context, path string) error {
	return c.req(ctx, http.MethodPut, "/configs?force=true", map[string]string{"path": path}, nil)
}

// Proxy is a proxy or a group, as /proxies has it.
type Proxy struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	UDP      bool           `json:"udp"`
	Alive    bool           `json:"alive"`
	Now      string         `json:"now,omitempty"`
	All      []string       `json:"all,omitempty"`
	Hidden   bool           `json:"hidden,omitempty"`
	Icon     string         `json:"icon,omitempty"`
	TestURL  string         `json:"testUrl,omitempty"`
	Provider string         `json:"provider-name,omitempty"`
	History  []DelayHistory `json:"history"`
}

type DelayHistory struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"`
}

func (c *Client) Proxies(ctx context.Context) (map[string]Proxy, error) {
	var v struct {
		Proxies map[string]Proxy `json:"proxies"`
	}
	err := c.req(ctx, http.MethodGet, "/proxies", nil, &v)
	return v.Proxies, err
}

// ProxyProvider is a proxy provider, as /providers/proxies has it.
type ProxyProvider struct {
	Name             string            `json:"name"`
	VehicleType      string            `json:"vehicleType"` // HTTP | File | Inline | Compatible
	Proxies          []Proxy           `json:"proxies"`
	TestURL          string            `json:"testUrl"`
	UpdatedAt        time.Time         `json:"updatedAt"`
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
}

type SubscriptionInfo struct {
	Upload   int64 `json:"Upload"`
	Download int64 `json:"Download"`
	Total    int64 `json:"Total"`
	Expire   int64 `json:"Expire"`
}

func (c *Client) ProxyProviders(ctx context.Context) (map[string]ProxyProvider, error) {
	var v struct {
		Providers map[string]ProxyProvider `json:"providers"`
	}
	err := c.req(ctx, http.MethodGet, "/providers/proxies", nil, &v)
	return v.Providers, err
}

// ProviderProxyDelay tests one of a provider's proxies, which /proxies
// doesn't know; 0 with an error when it failed or timed out.
func (c *Client) ProviderProxyDelay(ctx context.Context, provider, name, testURL string, timeout time.Duration) (int, error) {
	q := url.Values{"url": {testURL}, "timeout": {strconv.Itoa(int(timeout.Milliseconds()))}}
	var v struct {
		Delay int `json:"delay"`
	}
	err := c.req(ctx, http.MethodGet, "/providers/proxies/"+url.PathEscape(provider)+"/"+url.PathEscape(name)+"/healthcheck?"+q.Encode(), nil, &v)
	return v.Delay, err
}

// UpdateProxyProvider fetches a provider again.
func (c *Client) UpdateProxyProvider(ctx context.Context, name string) error {
	return c.req(ctx, http.MethodPut, "/providers/proxies/"+url.PathEscape(name), nil, nil)
}

func (c *Client) Select(ctx context.Context, group, name string) error {
	return c.req(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": name}, nil)
}

// Delay tests one proxy; 0 with an error when it failed or timed out.
func (c *Client) Delay(ctx context.Context, name, testURL string, timeout time.Duration) (int, error) {
	q := url.Values{"url": {testURL}, "timeout": {strconv.Itoa(int(timeout.Milliseconds()))}}
	var v struct {
		Delay int `json:"delay"`
	}
	err := c.req(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+q.Encode(), nil, &v)
	return v.Delay, err
}

// GroupDelay tests every proxy in a group: name → delay, failures left out.
func (c *Client) GroupDelay(ctx context.Context, group, testURL string, timeout time.Duration) (map[string]int, error) {
	q := url.Values{"url": {testURL}, "timeout": {strconv.Itoa(int(timeout.Milliseconds()))}}
	v := map[string]int{}
	err := c.req(ctx, http.MethodGet, "/group/"+url.PathEscape(group)+"/delay?"+q.Encode(), nil, &v)
	return v, err
}

type Rule struct {
	Index   int        `json:"index"`
	Type    string     `json:"type"`
	Payload string     `json:"payload"`
	Proxy   string     `json:"proxy"`
	Size    int        `json:"size"`
	Extra   *RuleExtra `json:"extra,omitempty"`
}

// RuleExtra is what the core counts for a top-level rule.
type RuleExtra struct {
	Disabled  bool      `json:"disabled"`
	HitCount  uint64    `json:"hitCount"`
	HitAt     time.Time `json:"hitAt"`
	MissCount uint64    `json:"missCount"`
}

func (c *Client) Rules(ctx context.Context) ([]Rule, error) {
	var v struct {
		Rules []Rule `json:"rules"`
	}
	err := c.req(ctx, http.MethodGet, "/rules", nil, &v)
	return v.Rules, err
}

type Metadata struct {
	Network     string `json:"network"`
	Type        string `json:"type"`
	SourceIP    string `json:"sourceIP"`
	DestIP      string `json:"destinationIP"`
	SourcePort  string `json:"sourcePort"`
	DestPort    string `json:"destinationPort"`
	Host        string `json:"host"`
	SniffHost   string `json:"sniffHost"`
	Process     string `json:"process"`
	ProcessPath string `json:"processPath"`
	RemoteDest  string `json:"remoteDestination"`
}

type Connection struct {
	ID          string    `json:"id"`
	Metadata    Metadata  `json:"metadata"`
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Start       time.Time `json:"start"`
	Chains      []string  `json:"chains"`
	Rule        string    `json:"rule"`
	RulePayload string    `json:"rulePayload"`
}

type Connections struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        uint64       `json:"memory"`
}

func (c *Client) Connections(ctx context.Context) (Connections, error) {
	var v Connections
	err := c.req(ctx, http.MethodGet, "/connections", nil, &v)
	if v.Connections == nil {
		v.Connections = []Connection{}
	}
	return v, err
}

func (c *Client) CloseConnection(ctx context.Context, id string) error {
	return c.req(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}

func (c *Client) CloseAllConnections(ctx context.Context) error {
	return c.req(ctx, http.MethodDelete, "/connections", nil, nil)
}

func (c *Client) FlushFakeIP(ctx context.Context) error {
	return c.req(ctx, http.MethodPost, "/cache/fakeip/flush", nil, nil)
}

func (c *Client) FlushDNS(ctx context.Context) error {
	return c.req(ctx, http.MethodPost, "/cache/dns/flush", nil, nil)
}

func (c *Client) UpdateGeo(ctx context.Context) error {
	return c.req(ctx, http.MethodPost, "/upgrade/geo", nil, nil)
}

type Traffic struct {
	Up        int64 `json:"up"`
	Down      int64 `json:"down"`
	UpTotal   int64 `json:"upTotal"`
	DownTotal int64 `json:"downTotal"`
}

type Memory struct {
	InUse   uint64 `json:"inuse"`
	OSLimit uint64 `json:"oslimit"`
}

type Log struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Stream reads one JSON value per line from path until ctx ends or the
// connection drops, handing each to fn.
func Stream[T any](ctx context.Context, c *Client, path string, fn func(T)) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.secret != "" {
		r.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.stream.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return &APIError{Status: resp.StatusCode, Message: resp.Status}
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var v T
		if json.Unmarshal(sc.Bytes(), &v) == nil {
			fn(v)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// DNSQuery resolves name through the core's resolver.
func (c *Client) DNSQuery(ctx context.Context, name string) error {
	var v struct {
		Status int `json:"Status"`
	}
	if err := c.req(ctx, http.MethodGet, "/dns/query?name="+url.QueryEscape(name)+"&type=A", nil, &v); err != nil {
		return err
	}
	if v.Status != 0 {
		return fmt.Errorf("dns rcode %d", v.Status)
	}
	return nil
}

// DNSLookup resolves name's qtype records through the core's resolver and
// gives each answer's data as the zone file writes it.
func (c *Client) DNSLookup(ctx context.Context, name, qtype string) ([]string, error) {
	var v struct {
		Status int `json:"Status"`
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := c.req(ctx, http.MethodGet, "/dns/query?name="+url.QueryEscape(name)+"&type="+url.QueryEscape(qtype), nil, &v); err != nil {
		return nil, err
	}
	if v.Status != 0 {
		return nil, fmt.Errorf("dns rcode %d", v.Status)
	}
	out := make([]string, len(v.Answer))
	for i, a := range v.Answer {
		out[i] = a.Data
	}
	return out, nil
}
