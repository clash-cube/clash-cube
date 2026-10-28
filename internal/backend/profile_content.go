package backend

import (
	"errors"
	"fmt"

	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// SaveProfileContent saves a local source. The selected profile is checked even
// while stopped and hot-reloaded while running. Refusal restores the source and
// runtime, retaining the editor's revision so its draft can be corrected/retried.
// Other profiles receive YAML validation and are core-checked when selected.
func (b *Backend) SaveProfileContent(id, revision, body string) (profiles.Document, error) {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	before, err := profiles.ReadDocument(id)
	if err != nil {
		return profiles.Document{}, err
	}
	if before.Revision != revision {
		return profiles.Document{}, errors.New("Configuration changed on disk; reload it before saving")
	}
	next, err := profiles.SaveContent(id, revision, body)
	if err != nil {
		return profiles.Document{}, err
	}
	if id == settings.Load().Profile {
		check := func() error {
			if b.core.Client() != nil {
				return b.reload()
			}
			if err := b.writeRuntime(false); err != nil {
				return err
			}
			return b.test()
		}
		if err := check(); err != nil {
			if _, restoreErr := profiles.SaveContent(id, next.Revision, before.Body); restoreErr != nil {
				return profiles.Document{}, errors.Join(err, fmt.Errorf("could not restore previous configuration: %w", restoreErr))
			}
			// A reload may fail after the core has accepted it (for example,
			// while expiring device sessions). Apply the previous source again.
			if restoreErr := check(); restoreErr != nil {
				return profiles.Document{}, errors.Join(err, fmt.Errorf("could not restore previous runtime: %w", restoreErr))
			}
			return profiles.Document{}, err
		}
	}
	b.emitProfiles()
	b.emitState()
	return next, nil
}
