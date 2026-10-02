// MihomoBar is a macOS menu bar app for mihomo. One binary runs in three
// roles: the GUI (default), the proxy core (`core`), and the privileged
// helper daemon (`helper`). See docs/design.md.
package main

import (
	"fmt"
	"os"

	"github.com/localhost-copilot/mihomobar/internal/core"
	"github.com/localhost-copilot/mihomobar/internal/gui"
	"github.com/localhost-copilot/mihomobar/internal/helper"
)

// version is set by the build (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	args := os.Args[1:]
	role := ""
	if len(args) > 0 {
		role = args[0]
	}
	var err error
	switch role {
	case "core":
		err = core.Main(args[1:])
	case "helper":
		err = helper.Main(args[1:], version)
	case "version", "-v", "--version":
		fmt.Println("mihomobar", version, "mihomo", core.Version())
	default:
		err = gui.Run(version)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mihomobar:", err)
		os.Exit(1)
	}
}
