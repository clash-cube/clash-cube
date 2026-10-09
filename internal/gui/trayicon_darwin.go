package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func setTrayIcon(tray *application.SystemTray, icon []byte) { tray.SetTemplateIcon(icon) }
