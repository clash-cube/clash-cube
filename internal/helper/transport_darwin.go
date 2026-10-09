package helper

import (
	"net"
	"time"
)

func dial(sock string) (net.Conn, error) { return net.DialTimeout("unix", sock, 2*time.Second) }
