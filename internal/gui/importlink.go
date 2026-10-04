package gui

import (
	"github.com/localhost-copilot/clashferry/internal/profiles"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func (h *host) receiveImportLink(raw string) {
	r, err := profiles.ParseImportLink(raw)
	if err != nil {
		r.Error = err.Error()
	}
	h.importMu.Lock()
	found := false
	for _, pending := range h.imports {
		if pending == r {
			found = true
			break
		}
	}
	if !found {
		h.imports = append(h.imports, r)
	}
	h.importMu.Unlock()
	// Keep requests until the main frontend subscribes and drains them. macOS
	// can deliver a cold-start URL before the webview has mounted.
	application.InvokeAsync(func() {
		h.showMain("")
		h.main.EmitEvent("import-request", true)
	})
}

// TakeImportRequests transfers pending links to the main window's import queue.
func (s *ProfileService) TakeImportRequests() []profiles.ImportRequest {
	s.h.importMu.Lock()
	defer s.h.importMu.Unlock()
	requests := s.h.imports
	s.h.imports = nil
	return requests
}
