//go:build !android

package gui

import "github.com/localhost-copilot/clashcube/internal/backend"

func (h *host) latencyChanged(e backend.LatencyEvent) {
	if h.menu != nil {
		h.menu.latencyChanged(e)
	}
}

func (h *host) eventChanged(e backend.Event) {
	if (e.Text == backend.ResetText || e.Text == backend.OfflineText) && h.menu != nil {
		h.menu.remeasure()
	}
}
