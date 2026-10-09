package helper

const (
	Label      = "com.localhost-copilot.clashcube.helper"
	SocketPath = "/var/run/clashcube-helper.sock"
	BinaryPath = "/Library/PrivilegedHelperTools/" + Label
	PlistPath  = "/Library/LaunchDaemons/" + Label + ".plist"
)
