package core

import (
	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/hub/route"
)

func init() {
	// mihomo only logs a listener it can't open and its API lists none,
	// so the GUI asks which ports this process really holds
	route.Register(func(r chi.Router) {
		r.Get("/clashcube/listening", func(w http.ResponseWriter, r *http.Request) {
			render.JSON(w, r, map[string]any{"ports": listeningPorts()})
		})
	})
}
