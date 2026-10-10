package gui

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/autostart"
	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/core"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

//go:embed all:dist
var dist embed.FS

//go:embed default.yaml
var defaultYAML []byte

//go:embed tray.png
var trayIcon []byte

//go:embed tray-off.png
var trayIconOff []byte

// The panel's width and the bounds of its height, in points.
const panelWidth, panelMin, panelMax, panelStart = 380, 200, 640, 480

// The curve the panel glides on, the page's --ease.
var glideCurve = [4]float64{.2, .8, .2, 1}

type host struct {
	app   *application.App
	b     *backend.Backend
	panel *application.WebviewWindow
	main  *application.WebviewWindow
	tray  *application.SystemTray
	menu  *trayMenu
	keys  *shortcuts

	panelHeight    int
	closing        atomic.Bool // a full-screen main window leaving it, to hide after
	mainReady      chan struct{}
	mainReadyOnce  sync.Once
	panelReady     chan struct{}
	panelReadyOnce sync.Once
	importMu       sync.Mutex
	imports        []profiles.ImportRequest
	webpage        webpageRule

	trayMu           sync.Mutex
	trayOn           bool
	trayUp, trayDown string
	trayLetter       string // the mode's, while the core runs
}

// Run starts the GUI.
func Run(version string) error {
	h := &host{mainReady: make(chan struct{}), panelReady: make(chan struct{})}
	h.keys = &shortcuts{h: h, set: map[string]string{}}
	h.b = backend.New(version, core.Version(), defaultYAML, sink{h})
	s := settings.Load()
	var instance *application.SingleInstanceOptions
	if runtime.GOOS == "windows" {
		root, _ := filepath.Abs(appdir.Root())
		id := sha256.Sum256([]byte(strings.ToLower(root)))
		instance = &application.SingleInstanceOptions{
			UniqueID: fmt.Sprintf("clashcube-%x", id[:16]),
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				application.InvokeAsync(func() {
					if h.main != nil {
						h.showMain("")
					}
				})
			},
		}
	}

	assets, _ := fs.Sub(dist, "dist")
	h.app = application.New(application.Options{
		Name:           "ClashCube",
		Icon:           trayIcon,
		SingleInstance: instance,
		Description:    "A menu bar app for mihomo",
		Services: []application.Service{
			application.NewService(&AppService{h}),
			application.NewService(&ProxyService{h: h}),
			application.NewService(&ProfileService{h}),
			application.NewService(&SettingsService{h}),
		},
		Assets:  application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:     application.MacOptions{ActivationPolicy: dockPolicy(s.Dock == "always")},
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(appdir.Root(), "webview")},
		OnShutdown: func() {
			h.b.Shutdown()
			h.installOnQuit()
		},
		ErrorHandler: func(err error) { log.Println("clashcube:", err) },
	})
	// Acquire the instance lock before touching shared settings or launching
	// background work; a second Windows instance simply raises the first.
	if err := h.b.Init(); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	s = settings.Load()
	if os.Getenv("CLASHCUBE_HOME") == "" {
		syncLaunchAtLogin(&s)
	}

	h.panel = h.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "panel",
		Title:           "ClashCube",
		URL:             "/?mode=panel",
		Width:           panelWidth,
		Height:          panelStart,
		Hidden:          true,
		Frameless:       true,
		AlwaysOnTop:     true,
		DisableResize:   true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		BackgroundType:  application.BackgroundTypeTranslucent,
		Mac: application.MacWindow{
			Backdrop:     application.MacBackdropTranslucent,
			CornerRadius: 12,
		},
	})
	h.panelHeight = panelStart
	if runtime.GOOS == "windows" {
		h.panel.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(*application.WindowEvent) {
			h.panelReadyOnce.Do(func() { close(h.panelReady) })
		})
	} else {
		close(h.panelReady)
	}

	width, height := 900, 620
	if len(s.Window) == 2 && s.Window[0] >= 680 && s.Window[1] >= 460 {
		width, height = s.Window[0], s.Window[1]
	}
	h.main = h.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "ClashCube",
		URL:       "/" + startView(),
		Width:     width,
		Height:    height,
		MinWidth:  680,
		MinHeight: 460,
		Hidden:    true,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInset,
			CollectionBehavior: application.MacWindowCollectionBehaviorMoveToActiveSpace |
				application.MacWindowCollectionBehaviorFullScreenPrimary,
		},
		BackgroundColour: application.NewRGB(244, 244, 246),
	})
	h.rememberSize()
	if runtime.GOOS == "windows" {
		h.main.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(*application.WindowEvent) {
			h.mainReadyOnce.Do(func() { close(h.mainReady) })
		})
	} else {
		close(h.mainReady)
	}
	// closing the window keeps the app in the menu bar
	h.main.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		if h.main.IsFullscreen() {
			h.closing.Store(true)
			h.main.UnFullscreen()
			time.AfterFunc(3*time.Second, func() {
				if h.closing.Swap(false) {
					application.InvokeAsync(h.hideMain)
				}
			})
			return
		}
		h.hideMain()
	})
	h.main.OnWindowEvent(events.Mac.WindowDidExitFullScreen, func(*application.WindowEvent) {
		if h.closing.Swap(false) {
			h.hideMain()
		}
	})
	// the Dock icon opens the window (and not the panel, which Wails would
	// show along with every other hidden window)
	h.app.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(e *application.ApplicationEvent) {
		h.showMain("")
		e.Cancel()
	})

	h.tray = h.app.SystemTray.New()
	setTrayIcon(h.tray, trayIconOff)
	h.tray.SetTooltip("ClashCube")
	h.menu = newTrayMenu(h)
	h.tray.SetMenu(h.app.NewMenu()) // replaced by the first rebuild
	// the menu takes the panel's place; opening it doesn't take focus
	// from the panel, so HideOnFocusLost wouldn't hide it
	h.tray.OnRightClick(func() {
		h.tray.HideWindow()
		if runtime.GOOS == "windows" {
			h.menu.tracking(true)
			defer h.menu.tracking(false)
		}
		h.tray.OpenMenu()
	})
	h.tray.AttachWindow(h.panel).WindowOffset(6)
	h.tray.OnClick(func() {
		trayPlay()
		h.showPanel(true)
	})

	h.app.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) {
		go h.b.Woke()
	})
	h.app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		h.receiveImportLink(e.Context().URL())
	})
	h.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if settings.Load().Notify {
			application.InvokeAsync(h.startNotifications)
		}
		h.menu.refresh()
		h.keys.apply()
		go h.b.Boot()
		switch os.Getenv("CLASHCUBE_SHOW") {
		case "main":
			h.showMain("")
		case "panel":
			h.showPanel(false)
		case "menu":
			time.AfterFunc(2500*time.Millisecond, func() { application.InvokeAsync(h.tray.OpenMenu) })
		case "webpage":
			const sample = "https://news.example.co.uk/2026/10/story"
			h.showWebpageRule(Webpage{URL: sample, Rules: webpageRules(sample)})
		default:
			// Opened by hand, the tray icon is easy to miss; at login, stay there.
			if runtime.GOOS == "windows" && !slices.Contains(os.Args[1:], autostart.LoginArg) {
				h.showMain("")
			}
		}
	})
	return h.app.Run()
}

// startView is the page the window opens on, $CLASHCUBE_VIEW (for screenshots).
func startView() string {
	if v := os.Getenv("CLASHCUBE_VIEW"); v != "" {
		v, hash, _ := strings.Cut(v, "#")
		if hash != "" {
			hash = "#" + hash
		}
		return "?view=" + v + hash
	}
	return ""
}

func setLaunchAtLogin(on bool) error { return autostart.Set(on) }

// syncLaunchAtLogin makes the system's login item match the settings. On
// Windows the user can also turn it off in Task Manager, which wins; an
// entry that stays on is rewritten, for a moved executable or an older
// entry without autostart.LoginArg.
func syncLaunchAtLogin(s *settings.Settings) {
	on := autostart.Enabled()
	switch {
	case runtime.GOOS == "windows" && s.LaunchAtLogin && !on:
		if next, err := settings.Update(func(s *settings.Settings) { s.LaunchAtLogin = false }); err == nil {
			*s = next
		}
	case s.LaunchAtLogin != on, runtime.GOOS == "windows" && on:
		_ = setLaunchAtLogin(s.LaunchAtLogin)
	}
}

// rememberSize keeps the window's size once a resize settles.
func (h *host) rememberSize() {
	var t *time.Timer
	h.main.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		if t != nil {
			t.Stop()
		}
		t = time.AfterFunc(500*time.Millisecond, func() {
			if h.main.IsFullscreen() || h.main.IsMaximised() || h.main.IsMinimised() {
				return
			}
			w, ht := h.main.Size()
			if w < 680 || ht < 460 {
				return
			}
			_, _ = settings.Update(func(s *settings.Settings) { s.Window = []int{w, ht} })
		})
	})
}

func (h *host) showMain(view string) {
	select {
	case <-h.mainReady:
	default:
		// As in magpie, wait for WebView2's first navigation, not ApplicationStarted.
		go func() { <-h.mainReady; application.InvokeAsync(func() { h.showMain(view) }) }()
		return
	}
	h.closing.Store(false)
	h.panel.Hide()
	if view != "" {
		h.main.EmitEvent("navigate", view)
	}
	h.dock(true)
	h.main.Show()
	h.main.Focus()
	activateApp()
}

func (h *host) showPanel(toggle bool) {
	select {
	case <-h.panelReady:
		if toggle {
			h.tray.ToggleWindow()
		} else {
			h.tray.ShowWindow()
		}
	default:
		go func() { <-h.panelReady; application.InvokeAsync(func() { h.showPanel(toggle) }) }()
	}
}

func (h *host) hideMain() {
	h.main.Hide()
	h.dock(false)
}

// dock shows the app in the Dock as the settings say, given whether the
// main window is up.
func (h *host) dock(shown bool) {
	switch settings.Load().Dock {
	case "always":
		setDock(true, shown)
	case "never":
		setDock(false, false)
	default:
		setDock(shown, shown)
	}
}

// fitPanel resizes the tray panel to its page, held under the menu bar.
func (h *host) fitPanel(height, ms int) {
	height = min(max(height, panelMin), panelMax)
	if s, err := h.panel.GetScreen(); err == nil && s != nil && s.WorkArea.Height > 0 {
		height = min(height, s.WorkArea.Height-16)
	}
	if height == h.panelHeight {
		return
	}
	h.panelHeight = height
	if h.panel.IsVisible() && ms > 0 && glide(h.panel, height, ms, glideCurve) {
		return
	}
	h.panel.SetSize(panelWidth, height)
	if h.panel.IsVisible() {
		_ = h.tray.PositionWindow(h.panel, 6)
	}
}

// stateChanged keeps the tray icon and menu in step with the core.
func (h *host) stateChanged(st backend.State) {
	// filled only while traffic is actually taken over (docs/design.md §10.3)
	on := st.Core == "running" && ((st.SystemProxy && !st.ProxyLost) || st.Tun)
	stopped := st.Core != "running"
	letter := ""
	if !stopped {
		letter = modeLetter(st.Mode)
	}
	h.trayMu.Lock()
	changed := on != h.trayOn
	h.trayOn = on
	h.trayLetter = letter
	if stopped {
		h.trayUp, h.trayDown = "", ""
	}
	icon, up, down := h.trayIcon(), h.trayUp, h.trayDown
	h.trayMu.Unlock()
	application.InvokeAsync(func() {
		if h.tray == nil {
			return
		}
		if changed {
			setTrayIcon(h.tray, icon)
		}
		setTray(on, up, down, letter)
		if h.menu != nil {
			h.menu.update(st)
		}
	})
}

// trafficChanged shows the speed beside the icon.
func (h *host) trafficChanged(t mihomoapi.Traffic) {
	up, down := "", ""
	if settings.Load().TraySpeed {
		up, down = speed(t.Up), speed(t.Down)
	}
	h.trayMu.Lock()
	same := up == h.trayUp && down == h.trayDown
	h.trayUp, h.trayDown = up, down
	on, letter := h.trayOn, h.trayLetter
	h.trayMu.Unlock()
	if !same {
		setTray(on, up, down, letter)
	}
}

// modeLetter is the mode's mark on the tray icon, as Surge's.
func modeLetter(mode string) string {
	switch mode {
	case "global":
		return "G"
	case "direct":
		return "D"
	}
	return "R"
}

// trayIcon is the icon for the current state; trayMu must be held.
func (h *host) trayIcon() []byte {
	if h.trayOn {
		return trayIcon
	}
	return trayIconOff
}

// speed is a rate as the menu bar shows it, three digits at most: "52 KB/s",
// "1.2 MB/s", "12 MB/s".
func speed(b int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v, i := float64(max(b, 0)), 0
	for v >= 999.5 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i < 2 || v >= 9.95 {
		return fmt.Sprintf("%.0f %s/s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s/s", v, units[i])
}

func (h *host) relabelTray() {
	if h.menu != nil {
		h.menu.relabel()
	}
}
