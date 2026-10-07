package core

import (
	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/hub/route"
	"github.com/metacubex/mihomo/tunnel"
)

// Route inspection stays in the core so providers, nested groups and mode
// selection use the same rule matcher as live traffic, without dialing targets.
func init() {
	route.Register(func(r chi.Router) {
		r.Get("/clashcube/route", func(w http.ResponseWriter, r *http.Request) {
			rule, chain, err := tunnel.InspectRoute(r.URL.Query().Get("target"))
			if err != nil {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, render.M{"message": err.Error()})
				return
			}
			result := render.M{"chains": chain, "rule": "", "rulePayload": ""}
			if rule != nil {
				result["rule"], result["rulePayload"] = rule.RuleType().String(), rule.Payload()
			}
			render.JSON(w, r, result)
		})
	})
}
