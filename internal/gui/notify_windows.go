package gui

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"log"
)

func scriptNotify(title, body string) {
	if notifier != nil {
		if err := notifier.SendNotification(notifications.NotificationOptions{Title: title, Body: body}); err == nil {
			return
		}
	}
	log.Printf("notification: %s: %s", title, body)
}
