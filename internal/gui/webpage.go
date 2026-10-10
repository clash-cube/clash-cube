package gui

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/net/publicsuffix"

	"github.com/localhost-copilot/clashcube/internal/userrules"
)

// Webpage is the page "Add Rule for Current Webpage" was asked for in,
// for its window to show: the address, or why there is none, and the
// rules it suggests, the first one the default.
type Webpage struct {
	URL string `json:"url"`
	// "automation" when macOS doesn't let ClashCube ask the browser, "none"
	// when no page is open, else what went wrong; "" with a URL
	Error string           `json:"error"`
	Rules []userrules.Rule `json:"rules"`
}

var (
	errAutomation = errors.New("automation not allowed")
	errNoPage     = errors.New("no page open")
)

// webpageRule is the window adding a rule for the page in front, after
// Surge's; one at a time.
type webpageRule struct {
	mu   sync.Mutex
	page Webpage
	win  *application.WebviewWindow
}

// webpageRules is what a page's address suggests: the site's domain and
// the host itself, or an IP address.
func webpageRules(raw string) []userrules.Rule {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			return []userrules.Rule{{Type: "IP-CIDR", Payload: host + "/32"}}
		}
		return []userrules.Rule{{Type: "IP-CIDR6", Payload: host + "/128"}}
	}
	site, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		site = host
	}
	return []userrules.Rule{{Type: "DOMAIN-SUFFIX", Payload: site}, {Type: "DOMAIN", Payload: host}}
}

// addWebpageRule reads the page in front in browser and opens the window
// for it; off the main thread, as macOS may ask the user first.
func (h *host) addWebpageRule(browser string) {
	var page Webpage
	raw, err := browserURL(browser)
	switch {
	case errors.Is(err, errAutomation):
		page.Error = "automation"
	case errors.Is(err, errNoPage):
		page.Error = "none"
	case err != nil:
		page.Error = err.Error()
	default:
		page.URL, page.Rules = raw, webpageRules(raw)
	}
	application.InvokeAsync(func() { h.showWebpageRule(page) })
}

// showWebpageRule opens the window for page, in place of one still open;
// on the main thread.
func (h *host) showWebpageRule(page Webpage) {
	w := &h.webpage
	w.mu.Lock()
	w.page = page
	old := w.win
	w.win = nil
	w.mu.Unlock()
	if old != nil {
		old.Close()
	}
	win := h.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                "webpage-rule",
		Title:               tr("Add Rule for Current Webpage", "为当前网页添加规则"),
		URL:                 "/?mode=webpage",
		Width:               460,
		Height:              480,
		DisableResize:       true,
		AlwaysOnTop:         true,
		InitialPosition:     application.WindowCentered,
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInset,
			CollectionBehavior: application.MacWindowCollectionBehaviorMoveToActiveSpace |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
		BackgroundColour: application.NewRGB(244, 244, 246),
	})
	// once it closes, the browser is in front again, unless the main
	// window is up
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		w.mu.Lock()
		mine := w.win == win
		if mine {
			w.win = nil
		}
		w.mu.Unlock()
		if mine && !h.main.IsVisible() {
			hideApp()
		}
	})
	w.mu.Lock()
	w.win = win
	w.mu.Unlock()
	win.Show()
	win.Focus()
	activateApp()
}

// Webpage is the page the rule window is for.
func (s *AppService) Webpage() Webpage {
	s.h.webpage.mu.Lock()
	defer s.h.webpage.mu.Unlock()
	return s.h.webpage.page
}

// OpenAutomationSettings opens the pane where ClashCube may be let ask
// browsers for their page.
func (s *AppService) OpenAutomationSettings() error {
	return s.h.app.Browser.OpenURL("x-apple.systempreferences:com.apple.preference.security?Privacy_Automation")
}
