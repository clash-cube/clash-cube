//go:build android

package main

import (
	"log"

	"github.com/localhost-copilot/clashcube/internal/gui"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var version = "dev"

func init() { application.RegisterAndroidMain(main) }
func main() {
	if err := gui.Run(version); err != nil {
		log.Printf("ClashCube startup: %v", err)
	}
}
