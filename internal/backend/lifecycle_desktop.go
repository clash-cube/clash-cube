//go:build !android

package backend

import "time"

const coreStartTimeout = 20 * time.Second

func (b *Backend) shutdownCore()    { _ = b.core.Stop() }
func (b *Backend) attachCore() bool { return false }
