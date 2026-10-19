//go:build !android

package runtimecfg

import "github.com/localhost-copilot/clashcube/internal/settings"

func platformPolicy(map[string]any, map[string]any, settings.Settings) {}
func MixedPort(s settings.Settings) int                                { return s.MixedPort }
