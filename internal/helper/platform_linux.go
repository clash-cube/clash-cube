package helper

import (
	"errors"
	"net"
)

// Linux services run under the system's service manager, without a desktop helper.
const SocketPath = "/var/run/clashcube-helper.sock"

var errUnsupported = errors.New("desktop privileged helpers are unavailable on Linux")

func dial(string) (net.Conn, error) { return nil, errUnsupported }
func Install(string, string) error  { return errUnsupported }
func Uninstall(string) error        { return errUnsupported }
