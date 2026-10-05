// Package helper is service mode's privileged side: `clashcube helper serve`
// runs as a root LaunchDaemon and starts the core as root (TUN needs it) for
// the one user it was installed for. See docs/design.md §2.3.
//
// The protocol is JSON lines over a unix socket. A client sends one Request;
// for "start" the connection then stays open and carries the core's output
// (Event.Line) until it exits (Event.Exit). Closing that connection, or
// sending {"op":"stop"} on it, stops the core.
package helper

const (
	Label      = "com.localhost-copilot.clashcube.helper"
	SocketPath = "/var/run/clashcube-helper.sock"
	BinaryPath = "/Library/PrivilegedHelperTools/" + Label
	PlistPath  = "/Library/LaunchDaemons/" + Label + ".plist"
)

type Request struct {
	Op     string `json:"op"` // version | start | stop | update
	Home   string `json:"home,omitempty"`
	Config string `json:"config,omitempty"`
	Ctl    string `json:"ctl,omitempty"`
	Secret string `json:"secret,omitempty"`
	Path   string `json:"path,omitempty"` // update: the app's executable
}

type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
	Hash    string `json:"hash,omitempty"` // sha256 of the helper's executable
}

type Event struct {
	Line string  `json:"line,omitempty"`
	Exit *string `json:"exit,omitempty"` // the core ended; "" when cleanly
}
