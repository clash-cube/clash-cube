package gui

import (
	"log"
	"os/exec"
)

// scriptNotify posts a notification through osascript, for when the
// notification centre won't take ours: it refuses an ad-hoc signed app
// ("Notifications are not allowed for this application") without asking,
// and a bare binary has no bundle. The notification is Script Editor's, so
// a click doesn't open the app. The text goes in as arguments, never as
// script.
func scriptNotify(title, body string) {
	err := exec.Command("/usr/bin/osascript",
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run",
		title, body).Run()
	if err != nil {
		log.Println("notify:", err)
	}
}
