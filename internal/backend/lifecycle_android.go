package backend

import (
	"context"
	"log"
	"time"
)

const coreStartTimeout = 3 * time.Minute // includes the VPN consent Activity

// The foreground service, not the Activity, owns the VPN lifetime.
func (b *Backend) shutdownCore() {}
func (b *Backend) attachCore() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	attached, err := b.core.Attach(ctx)
	if err != nil {
		log.Println("attach Android core:", err)
		b.reportCrash(err)
		return true // never replace a service whose ownership is uncertain
	}
	if attached {
		b.startStreams()
		b.watchPorts()
		b.followGroups()
	}
	return attached
}
