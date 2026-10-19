package gui

import (
	"embed"
	"errors"
	"io/fs"
	"sync"

	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/userrules"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:dist
var dist embed.FS

//go:embed default.yaml
var defaultYAML []byte

type host struct {
	app      *application.App
	b        *backend.Backend
	main     *application.WebviewWindow
	importMu sync.Mutex
	imports  []profiles.ImportRequest
}

// Supplied from the fork's version.go by the Android build, without linking
// mihomo's package globals into the UI library.
var bundledCoreVersion = "unknown"

func Run(version string) error {
	h := &host{}
	h.b = backend.New(version, bundledCoreVersion, defaultYAML, sink{h})
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		return err
	}
	h.app = application.New(application.Options{
		Name: "ClashCube",
		Services: []application.Service{
			application.NewService(&AppService{h}),
			application.NewService(&ProxyService{h: h}),
			application.NewService(&ProfileService{h}),
			application.NewService(&SettingsService{h}),
		},
		Assets:     application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		OnShutdown: h.b.Shutdown,
	})
	if err := h.b.Init(); err != nil {
		return err
	}
	// Wails Android always routes requests to the first registered window.
	h.main = h.app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "main", Title: "ClashCube", URL: "/"})
	h.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { go h.b.Boot() })
	return h.app.Run()
}

func (h *host) showMain(view string) {
	if view != "" {
		h.main.EmitEvent("navigate", view)
	}
}
func (h *host) hidePanel()                          {}
func (h *host) fitPanel(int, int)                   {}
func (h *host) dock(bool)                           {}
func (h *host) relabelTray()                        {}
func (h *host) stateChanged(backend.State)          {}
func (h *host) trafficChanged(mihomoapi.Traffic)    {}
func (h *host) notify(backend.Event)                {}
func (h *host) eventChanged(backend.Event)          {}
func (h *host) latencyChanged(backend.LatencyEvent) {}

var errAndroidFeature = errors.New("not implemented on Android yet")

func setLaunchAtLogin(bool) error       { return errAndroidFeature }
func openPath(string, bool, bool) error { return errAndroidFeature }
func appsDirectory() string             { return "" }
func appIcon(string) string             { return "" }

type App struct {
	Name       string `json:"name"`
	Bundle     string `json:"bundle"`
	Executable string `json:"executable"`
}

func runningApps() []App       { return []App{} }
func appAt(string) (App, bool) { return App{}, false }
func tr(en, cn string) string {
	if settings.Load().Lang == "zh" {
		return cn
	}
	return en
}

func (*SettingsService) Hotkeys() []Hotkey { return []Hotkey{} }
func (*SettingsService) SetHotkey(string, string) ([]Hotkey, error) {
	return []Hotkey{}, errAndroidFeature
}
func (*SettingsService) RecordHotkey(bool) {}
func (*SettingsService) UseRecommendedHotkeys() (Recommended, error) {
	return Recommended{}, errAndroidFeature
}

type Webpage struct {
	URL   string           `json:"url"`
	Error string           `json:"error"`
	Rules []userrules.Rule `json:"rules"`
}

func (*AppService) Webpage() Webpage                    { return Webpage{Error: errAndroidFeature.Error()} }
func (*AppService) OpenAutomationSettings() error       { return errAndroidFeature }
func (s *AppService) AppUpdate() backend.AppUpdate      { return s.h.b.AppUpdate() }
func (s *AppService) CheckAppUpdate() backend.AppUpdate { return s.h.b.CheckAppUpdate() }
func (*AppService) RestartToUpdate() error              { return errAndroidFeature }
