package profiles

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// Document is a source configuration and its content revision. Managed
// subscriptions are readable; their editable copies are separate profiles.
type Document struct {
	Body     string `json:"body"`
	Revision string `json:"revision"`
	ReadOnly bool   `json:"readOnly"`
}

func document(p Profile) (Document, error) {
	body, err := os.ReadFile(p.Path())
	if err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(body)
	return Document{Body: string(body), Revision: hex.EncodeToString(sum[:]), ReadOnly: p.URL != ""}, nil
}

// ReadDocument resolves IDs through the profile index, never as caller paths.
func ReadDocument(id string) (Document, error) {
	mu.Lock()
	defer mu.Unlock()
	for _, p := range load() {
		if p.ID == id {
			return document(p)
		}
	}
	return Document{}, errors.New("no such profile")
}

// SaveContent atomically replaces a local source only if its content still
// matches revision. Core validation and application belong to the backend.
func SaveContent(id, revision, body string) (Document, error) {
	if int64(len(body)) > maxSize {
		return Document{}, errors.New("profile is too large")
	}
	if err := Validate([]byte(body)); err != nil {
		return Document{}, err
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range load() {
		if p.ID != id {
			continue
		}
		before, err := document(p)
		if err != nil {
			return Document{}, err
		}
		if before.ReadOnly {
			return Document{}, errors.New("Subscription configurations are read-only; make an editable copy")
		}
		if before.Revision != revision {
			return Document{}, errors.New("Configuration changed on disk; reload it before saving")
		}
		if err := writeContent(p.Path(), []byte(body)); err != nil {
			return Document{}, err
		}
		return document(p)
	}
	return Document{}, errors.New("no such profile")
}

func writeContent(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(body) // CreateTemp creates mode-0600 files.
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
