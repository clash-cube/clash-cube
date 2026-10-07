package core

import (
	"context"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/component/ca"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/route"
	"github.com/metacubex/mihomo/tunnel"
)

// /clashcube/trace?proxy=<node>&url=<trace> fetches Cloudflare's trace
// through one node, as mihomo's delay test dials it, so a service with no
// trace of its own still learns the address that node shows. Only a
// /cdn-cgi/trace path is fetched, and only its first 4 KiB returned.
func init() {
	route.Register(func(r chi.Router) {
		r.Get("/clashcube/trace", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			u, err := url.Parse(q.Get("url"))
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "/cdn-cgi/trace" {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, render.M{"message": "not a trace URL"})
				return
			}
			p := findProxy(q.Get("proxy"))
			if p == nil {
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, render.M{"message": "no such proxy"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			body, err := traceThrough(ctx, p, u)
			if err != nil {
				render.Status(r, http.StatusBadGateway)
				render.JSON(w, r, render.M{"message": err.Error()})
				return
			}
			render.JSON(w, r, render.M{"body": body})
		})
	})
}

// findProxy looks a node up among the configured proxies and groups, then
// among the providers', where subscription nodes live.
func findProxy(name string) C.Proxy {
	if p, ok := tunnel.Proxies()[name]; ok {
		return p
	}
	for _, pd := range tunnel.Providers() {
		for _, p := range pd.Proxies() {
			if p.Name() == name {
				return p
			}
		}
	}
	return nil
}

func traceThrough(ctx context.Context, p C.Proxy, u *url.URL) (string, error) {
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	var md C.Metadata
	if err := md.SetRemoteAddress(net.JoinHostPort(u.Hostname(), port)); err != nil {
		return "", err
	}
	conn, err := p.DialContext(ctx, &md)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	tlsConfig, err := ca.GetTLSConfig(ca.Option{})
	if err != nil {
		return "", err
	}
	tr := &http.Transport{
		DialContext:       func(context.Context, string, string) (net.Conn, error) { return conn, nil },
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return string(b), err
}
