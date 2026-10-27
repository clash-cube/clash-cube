package backend

import (
	"errors"
	"strings"

	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// Patch is a partial update: the keys of p (as settings.json names them)
// that are present are changed.
type Patch struct {
	MixedPort       *int      `json:"mixedPort,omitempty"`
	AllowLan        *bool     `json:"allowLan,omitempty"`
	IPv6            *bool     `json:"ipv6,omitempty"`
	LogLevel        *string   `json:"logLevel,omitempty"`
	TunStack        *string   `json:"tunStack,omitempty"`
	ICMPForwarding  *bool     `json:"icmpForwarding,omitempty"`
	GuardIPv6       *bool     `json:"guardIPv6,omitempty"`
	GuardDNS        *bool     `json:"guardDNS,omitempty"`
	BlockSTUN       *bool     `json:"blockSTUN,omitempty"`
	BlockQUIC       *bool     `json:"blockQUIC,omitempty"`
	DNSRespectRules *bool     `json:"dnsRespectRules,omitempty"`
	UnifiedDelay    *bool     `json:"unifiedDelay,omitempty"`
	Bypass          *[]string `json:"bypass,omitempty"`
	AutoStart       *bool     `json:"autoStart,omitempty"`
	LaunchAtLogin   *bool     `json:"launchAtLogin,omitempty"`
	Theme           *string   `json:"theme,omitempty"`
	Lang            *string   `json:"lang,omitempty"`
	Dock            *string   `json:"dock,omitempty"`
	TestURL         *string   `json:"testUrl,omitempty"`
	TraySpeed       *bool     `json:"traySpeed,omitempty"`
	FindProcess     *bool     `json:"findProcess,omitempty"`
	Notify          *bool     `json:"notify,omitempty"`
	SaveData        *bool     `json:"saveData,omitempty"`
	AIServices      *[]string `json:"aiServices,omitempty"`
	AutoUpdateApp   *bool     `json:"autoUpdateApp,omitempty"`
	SubscriptionUA  *string   `json:"subscriptionUa,omitempty"`
}

// PatchFields validates and applies a partial settings update. Core settings
// use the same validation and rollback as other backend changes.
func (b *Backend) PatchFields(p Patch) (settings.Settings, error) {
	if p.MixedPort != nil && (*p.MixedPort < 1 || *p.MixedPort > 65535) {
		return settings.Load(), errors.New("port must be 1–65535")
	}
	if p.Bypass != nil {
		var clean []string
		for _, b := range *p.Bypass {
			if b = strings.TrimSpace(b); b != "" {
				clean = append(clean, b)
			}
		}
		p.Bypass = &clean
	}
	if p.SubscriptionUA != nil {
		ua := strings.TrimSpace(*p.SubscriptionUA)
		if !profiles.ValidUserAgent(ua) {
			return settings.Load(), errors.New("a user agent must be one line of at most 200 characters")
		}
		p.SubscriptionUA = &ua
	}
	out, err := b.PatchSettings(func(st *settings.Settings) {
		set(&st.MixedPort, p.MixedPort)
		set(&st.AllowLan, p.AllowLan)
		set(&st.IPv6, p.IPv6)
		set(&st.LogLevel, p.LogLevel)
		set(&st.TunStack, p.TunStack)
		set(&st.ICMPForwarding, p.ICMPForwarding)
		set(&st.GuardIPv6, p.GuardIPv6)
		set(&st.GuardDNS, p.GuardDNS)
		set(&st.BlockSTUN, p.BlockSTUN)
		set(&st.BlockQUIC, p.BlockQUIC)
		set(&st.DNSRespectRules, p.DNSRespectRules)
		set(&st.UnifiedDelay, p.UnifiedDelay)
		set(&st.Bypass, p.Bypass)
		set(&st.AutoStart, p.AutoStart)
		set(&st.LaunchAtLogin, p.LaunchAtLogin)
		set(&st.Theme, p.Theme)
		set(&st.Lang, p.Lang)
		set(&st.Dock, p.Dock)
		set(&st.TestURL, p.TestURL)
		set(&st.TraySpeed, p.TraySpeed)
		set(&st.FindProcess, p.FindProcess)
		set(&st.Notify, p.Notify)
		set(&st.SaveData, p.SaveData)
		set(&st.AIServices, p.AIServices)
		set(&st.AutoUpdateApp, p.AutoUpdateApp)
		set(&st.SubscriptionUA, p.SubscriptionUA)
	})
	return out, err
}

func set[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
