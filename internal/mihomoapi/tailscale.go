package mihomoapi

import (
	"context"
	"net/http"
	"net/url"

	"github.com/metacubex/mihomo/component/tailnet"
)

type TailscaleStatus = tailnet.Status
type TailscalePing = tailnet.Ping

func (c *Client) TailscaleLogin(ctx context.Context, name string) error {
	return c.req(ctx, http.MethodPost, "/clashcube/tailscale/login?"+url.Values{"name": {name}}.Encode(), nil, nil)
}
func (c *Client) TailscaleLogout(ctx context.Context, name string) error {
	return c.req(ctx, http.MethodPost, "/clashcube/tailscale/logout?"+url.Values{"name": {name}}.Encode(), nil, nil)
}
func (c *Client) TailscalePing(ctx context.Context, name, ip string) (TailscalePing, error) {
	var v TailscalePing
	err := c.req(ctx, http.MethodPost, "/clashcube/tailscale/ping?"+url.Values{"name": {name}, "ip": {ip}}.Encode(), nil, &v)
	return v, err
}
