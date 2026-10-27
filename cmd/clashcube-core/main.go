// clashcube-core is the independently deployed OpenWrt core service.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/core"
	"github.com/localhost-copilot/clashcube/internal/coreservice"
	"github.com/localhost-copilot/clashcube/internal/router"
)

func main() {
	if len(os.Args) > 1 {
		var err error
		switch os.Args[1] {
		case "core":
			err = core.Main(os.Args[2:])
		case "firewall-stop":
			err = router.Cleanup()
		case "firewall-sync":
			client := coreservice.NewClient()
			defer client.Close()
			_, err = client.Firewall().Reconcile()
		default:
			log.Fatal("unknown core service operation")
		}
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := coreservice.Serve(ctx, appdir.Root(), appdir.CoreHome(), core.Version()); err != nil {
		log.Fatal(err)
	}
}
