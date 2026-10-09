package helper

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"time"

	winio "github.com/tailscale/go-winio"
)

// A fresh, unguessable pipe per GUI session avoids a shared privileged service.
var SocketPath = func() string {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(err)
	}
	return `\\.\pipe\clashcube-` + hex.EncodeToString(nonce[:])
}()

func dial(path string) (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(path, &timeout)
}
