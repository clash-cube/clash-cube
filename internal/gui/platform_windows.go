package gui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/localhost-copilot/clashcube/internal/winutil"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// Windows uses the native title bar and taskbar; AppKit-only decoration is
// intentionally absent. Panel positioning and resizing stay with Wails.
func glide(*application.WebviewWindow, int, int, [4]float64) bool { return false }
func activateApp()                                                {}
func setDock(bool, bool)                                          {}
func dockPolicy(bool) application.ActivationPolicy                { return application.ActivationPolicyRegular }
func trayPlay()                                                   {}
func styleTrayMenu()                                              {}
func watchTrayMenu()                                              {}
func appIcon(string) string                                       { return "" }

func setTray(on bool, up, down, letter string) {
	application.InvokeAsync(func() {
		if m := theTrayMenu; m != nil && m.h.tray != nil {
			setWindowsTrayState(m.h.tray, on, letter)
			label := "ClashCube"
			if letter != "" {
				label += " · " + letter
			}
			if up != "" || down != "" {
				label += "\n↑ " + up + "  ↓ " + down
			}
			m.h.tray.SetTooltip(label)
		}
	})
}

var getKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

func optionHeld() bool { value, _, _ := getKeyState.Call(0x12); return value&0x8000 != 0 }

var sysLang = sync.OnceValue(func() bool {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	return err == nil && len(languages) > 0 && strings.HasPrefix(languages[0], "zh")
})

func systemPrefersChinese() bool { return sysLang() }

type App struct {
	Name       string `json:"name"`
	Bundle     string `json:"bundle"`
	Executable string `json:"executable"`
}

func runningApps() []App {
	out := []App{}
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(h)
	entry := windows.ProcessEntry32{Size: uint32(processEntrySize)}
	seen := map[string]bool{}
	for err = windows.Process32First(h, &entry); err == nil; err = windows.Process32Next(h, &entry) {
		path := winutil.ProcessPath(entry.ProcessID)
		key := strings.ToLower(path)
		if path != "" && !seen[key] {
			seen[key] = true
			out = append(out, App{Name: filepath.Base(path), Executable: path})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
func outerBundle(string) string { return "" }
func appAt(path string) (App, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return App{}, false
	}
	return App{Name: filepath.Base(path), Executable: path}, true
}
