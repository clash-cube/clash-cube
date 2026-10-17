package core

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/component/tailnet"
	"github.com/metacubex/mihomo/hub/route"
)

func init() { route.Register(registerTailscale) }

// These routes inherit the controller's random bearer secret. Never expose the
// tsnet local API, preferences or persistent keys through the GUI API.
func registerTailscale(r chi.Router) {
	r.Get("/clashcube/tailscale", tailscaleStatus)
	r.Post("/clashcube/tailscale/{action}", tailscaleAction)
}

func tailscaleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("stream") != "true" {
		render.JSON(w, r, tailnet.Statuses())
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	enc := json.NewEncoder(w)
	for {
		changed := tailnet.Changed()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := enc.Encode(tailnet.Statuses()); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		}
	}
}

func tailscaleAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	node := tailnet.Get(r.URL.Query().Get("name"))
	if node == nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"message": "Tailscale connection is not active"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var result any = render.M{}
	var err error
	switch chi.URLParam(r, "action") {
	case "login":
		err = node.Login(ctx)
	case "logout":
		err = node.Logout(ctx)
	case "ping":
		var ip netip.Addr
		ip, err = netip.ParseAddr(r.URL.Query().Get("ip"))
		if err == nil {
			result, err = node.Ping(ctx, ip)
		}
	default:
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"message": "unknown Tailscale action"})
		return
	}
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"message": err.Error()})
		return
	}
	render.JSON(w, r, result)
}
