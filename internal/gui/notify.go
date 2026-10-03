package gui

import (
	"context"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/localhost-copilot/mihomobar/internal/backend"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

var (
	notifier     *notifications.NotificationService
	notifierOnce sync.Once
	notifierOK   bool
)

// startNotifications readies the notification centre. It needs a bundle
// (bin/MihomoBar.app); a bare binary goes without, which isn't an error.
func (h *host) startNotifications() {
	n := notifications.New()
	if err := n.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		log.Println("notifications:", err)
		return
	}
	n.OnNotificationResponse(func(notifications.NotificationResult) {
		application.InvokeAsync(func() { h.showMain("events") })
	})
	notifier = n
}

// notify posts an event as a notification, asking for permission the
// first time.
func (h *host) notify(e backend.Event) {
	if !settings.Load().Notify {
		return
	}
	if notifier == nil {
		scriptNotify(eventText(e))
		return
	}
	notifierOnce.Do(func() {
		ok, err := notifier.CheckNotificationAuthorization()
		if err == nil && !ok {
			ok, err = notifier.RequestNotificationAuthorization()
		}
		notifierOK = err == nil && ok
		if !notifierOK {
			log.Println("notifications: falling back to osascript:", err)
		}
	})
	title, body := eventText(e)
	if !notifierOK {
		scriptNotify(title, body)
		return
	}
	if err := notifier.SendNotification(notifications.NotificationOptions{
		ID:       "event-" + strconv.Itoa(e.ID),
		Title:    title,
		Body:     body,
		ThreadID: e.Kind,
	}); err != nil {
		log.Println("notify:", err)
	}
}

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

// eventText is an event as a notification: a title for its kind and its
// text, translated, with the arguments filled in.
func eventText(e backend.Event) (title, body string) {
	text := e.Text
	if zh() {
		if t, ok := eventsZh[text]; ok {
			text = t
		}
	}
	for k, v := range e.Args {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	switch e.Kind {
	case "core":
		title = tr("Core", "内核")
	case "proxy":
		title = tr("System Proxy", "系统代理")
	case "profile":
		title = tr("Profiles", "配置")
	case "network":
		title = tr("Network Rules", "网络规则")
	default:
		title = "MihomoBar"
	}
	return title, text
}

// The Chinese of the events that notify; the page has its own (i18n.ts).
var eventsZh = map[string]string{
	"The core stopped: {error}":                                        "内核已停止：{error}",
	"Another app changed the system proxy":                             "其他应用修改了系统代理",
	"Couldn't update {name}: {error}":                                  "无法更新 {name}：{error}",
	"The updated profile was refused, the previous one stays: {error}": "更新后的配置未通过校验，继续使用之前的配置：{error}",
	"Applied the settings for Wi-Fi {ssid}":                            "已应用 Wi-Fi {ssid} 的设置",
	"Applied the settings for wired networks":                          "已应用有线网络的设置",
	"Applied the settings for other networks":                          "已应用其他网络的设置",
	"Couldn't apply all of the settings for Wi-Fi {ssid}: {error}":     "Wi-Fi {ssid} 的设置未能全部应用：{error}",
	"Couldn't apply all of the settings for wired networks: {error}":   "有线网络的设置未能全部应用：{error}",
	"Couldn't apply all of the settings for other networks: {error}":   "其他网络的设置未能全部应用：{error}",
}
