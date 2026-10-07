package gui

import (
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/localhost-copilot/clashcube/internal/appupdate"
	"github.com/localhost-copilot/clashcube/internal/backend"
)

func updatePrompt() string {
	return tr("ClashCube is updating itself.", "ClashCube 正在更新。")
}

// AppUpdate is where updating the app stands, with the download's progress.
func (s *AppService) AppUpdate() backend.AppUpdate { return s.h.b.AppUpdate() }

// CheckAppUpdate looks for a new release now; one that is newer starts
// downloading.
func (s *AppService) CheckAppUpdate() backend.AppUpdate { return s.h.b.CheckAppUpdate() }

// RestartToUpdate puts the downloaded version in place, asking for the
// administrator's password where the app's folder needs it, and opens it
// once this one has quit.
func (s *AppService) RestartToUpdate() error {
	if err := s.h.b.InstallAppUpdate(true, updatePrompt()); err != nil {
		return err
	}
	if err := appupdate.Relaunch(s.h.b.AppBundle()); err != nil {
		log.Println("app update:", err)
	}
	application.InvokeAsync(s.h.app.Quit)
	return nil
}

// installOnQuit puts a downloaded update in place as the app quits, so the
// next launch is the new version. It never asks for a password: a folder
// that needs one waits for Restart to Update.
func (h *host) installOnQuit() {
	if h.b.AppUpdate().State == "ready" {
		if err := h.b.InstallAppUpdate(false, ""); err != nil && !appupdate.NeedsAdmin(err) {
			log.Println("app update:", err)
		}
	}
}
