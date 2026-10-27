package webui

import (
	"errors"
	"os"
	"regexp"

	"github.com/localhost-copilot/clashcube/internal/profiles"
)

var controllerLine = regexp.MustCompile(`(?m)^(secret|external-controller|external-controller-tls|external-controller-unix):[^\n]*`)

// Preserve diff line positions while keeping private controller credentials
// out of browser responses. Runtime YAML uses plain top-level keys.
func redactController(body string) string {
	return controllerLine.ReplaceAllString(body, "$1: [private]")
}

func readProfile(id string) (string, error) {
	p, ok := profiles.Get(id)
	if !ok {
		return "", errors.New("no such profile")
	}
	body, err := os.ReadFile(p.Path())
	return string(body), err
}
