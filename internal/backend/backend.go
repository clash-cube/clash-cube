// Package backend is the app without its windows: it owns the settings, the
// profiles, the core and the system proxy, and tells whoever listens (the
// GUI) when anything changes.
package backend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/appdir"
	"github.com/localhost-copilot/clashcube/internal/appupdate"
	"github.com/localhost-copilot/clashcube/internal/coremgr"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/profiles"
	"github.com/localhost-copilot/clashcube/internal/runtimecfg"
	"github.com/localhost-copilot/clashcube/internal/settings"
	"github.com/localhost-copilot/clashcube/internal/sysproxy"
	"github.com/localhost-copilot/clashcube/internal/usage"
	"github.com/localhost-copilot/clashcube/internal/userrules"
)

// State is what the GUI shows of the app at a glance.
type State struct {
	Core        string `json:"core"` // coremgr.Status
	CoreError   string `json:"coreError,omitempty"`
	CoreVersion string `json:"coreVersion"`
	AppVersion  string `json:"appVersion"`
	Mode        string `json:"mode"`
	SystemProxy bool   `json:"systemProxy"`
	// another app took the system proxy over; ours is no longer in effect
	ProxyLost   bool    `json:"proxyLost"`
	Tun         bool    `json:"tun"`
	TunStack    string  `json:"tunStack"`
	ServiceMode bool    `json:"serviceMode"`
	MixedPort   int     `json:"mixedPort"`
	Profile     string  `json:"profile"`
	ProfileName string  `json:"profileName"`
	Busy        string  `json:"busy,omitempty"` // what is being done, e.g. "restarting"
	Network     Network `json:"network"`
	// the last configuration refused, until one is taken
	Refusal *Refusal `json:"refusal,omitempty"`
	// how the port modules' ports came up (portstate.go)
	Ports []PortState `json:"ports"`
	// updating the app itself (appupdate.go)
	Update AppUpdate `json:"update"`
}

// Sink is told what changes; the GUI turns these into events.
type Sink interface {
	State(State)
	Traffic(mihomoapi.Traffic)
	Memory(mihomoapi.Memory)
	Log(mihomoapi.Log)
	Profiles([]profiles.Profile)
	Event(Event)
	Latency(LatencySample)
}

type Backend struct {
	Version     string
	CoreVersion string
	DefaultYAML []byte

	core *coremgr.Manager
	sink Sink

	opMu sync.Mutex // one start/stop/reload at a time
	mu   sync.Mutex
	busy string
	// streams runs while the core does; cancelled when it stops
	streamCancel context.CancelFunc
	// we turned the system proxy on, so we turn it off
	proxyOwned bool
	// another app took it over after we set it (watch.go)
	proxyLost bool
	logLevel  string
	logWatch  context.CancelFunc
	latency   latencyTester
	// coreops.go
	geoUpdating bool
	// connectivity.go: each item's recent measures, and which items'
	// next one follows a network change
	connHist  map[string][]LatencySample
	connBreak map[string]bool
	// globe.go
	globe    globeState
	aiIPs    aiIPCache
	aiEgress aiEgressCache

	// watch.go
	events       []Event
	eventSeq     int
	woke         time.Time
	resetTimer   *time.Timer
	groupNow     map[string]string
	groupClient  *mihomoapi.Client
	groupProfile string
	shutdown     bool
	done         chan struct{}

	// the profile file the running configuration was written from, so an
	// edit to it is noticed (watch.go)
	profileRead profileStamp
	// the last configuration refused, until one is taken (runtimeview.go)
	refusal *Refusal
	// portstate.go
	ports   []PortState
	portGen int

	// usage.go
	usage     *usage.Store
	usageOnce sync.Once

	// network.go
	net     netWatch
	applyMu sync.Mutex // one network rule applied at a time

	// appupdate.go
	upd       appUpdater
	installMu sync.Mutex // one install at a time
}

func New(version, coreVersion string, defaultYAML []byte, sink Sink) *Backend {
	b := &Backend{Version: version, CoreVersion: coreVersion, DefaultYAML: defaultYAML, sink: sink, done: make(chan struct{})}
	b.core = coremgr.New(localRunner, appdir.CoreHome(), appdir.RuntimeConfig())
	b.core.OnChange(b.coreChanged)
	b.core.OnLog(func(l string) {}) // the core's stdout duplicates /logs
	return b
}

// Init prepares the directories and the first profile; Start then runs the
// core if the settings say to.
func (b *Backend) Init() error {
	if err := appdir.Ensure(); err != nil {
		return err
	}
	s := settings.Load()
	// Windows elevation lasts only for the GUI session. Never start a local
	// unprivileged core with a TUN setting carried over from yesterday.
	if runtime.GOOS == "windows" && (s.Tun || s.ServiceMode) {
		var err error
		s, err = settings.Update(func(s *settings.Settings) { s.Tun = false; s.ServiceMode = false })
		if err != nil {
			return err
		}
	}
	if len(profiles.List()) == 0 {
		p, err := profiles.AddDefault(b.DefaultYAML)
		if err != nil {
			return err
		}
		s.Profile = p.ID
		_ = settings.Save(s)
	}
	if _, ok := profiles.Get(s.Profile); !ok {
		s.Profile = profiles.List()[0].ID
		_ = settings.Save(s)
	}
	profiles.SetUserAgent("clash.meta/" + b.CoreVersion + " clashcube/" + b.Version)
	// a system proxy left pointing at us by a crash is cleared, even when
	// it should be on: it points at a port nobody serves until the core
	// starts (and it may never, with AutoStart off or a broken profile).
	// Start sets it again.
	if proxyPointsAt(proxyHost, s.MixedPort) && !listening(s.MixedPort) {
		_ = proxyClear()
	}
	if bundle := appupdate.Bundle(); bundle != "" && appupdate.Stuck(bundle) == "" {
		b.upd.bundle = bundle
	}
	go b.autoUpdate()
	go b.appUpdateLoop()
	go b.refreshCity()
	return nil
}

// Boot starts the core if the settings say to, then restores the system proxy.
func (b *Backend) Boot() {
	go b.watch()
	s := settings.Load()
	if !s.AutoStart {
		b.emitState()
		return
	}
	if err := b.Start(); err != nil {
		log.Println("start core:", err)
	}
}

func (b *Backend) setBusy(what string) {
	b.mu.Lock()
	b.busy = what
	b.mu.Unlock()
	b.emitState()
}

func (b *Backend) State() State {
	s := settings.Load()
	st, errText := b.core.Status()
	b.mu.Lock()
	busy, lost, refusal, ports := b.busy, b.proxyLost, b.refusal, b.ports
	b.mu.Unlock()
	name := ""
	if p, ok := profiles.Get(s.Profile); ok {
		name = p.Name
	}
	return State{
		Core: string(st), CoreError: errText, CoreVersion: b.CoreVersion, AppVersion: b.Version,
		Mode: s.Mode, SystemProxy: s.SystemProxy, ProxyLost: lost && s.SystemProxy, Tun: s.Tun, TunStack: s.TunStack,
		ServiceMode: s.ServiceMode, MixedPort: s.MixedPort,
		Profile: s.Profile, ProfileName: name, Busy: busy, Network: b.network(s),
		Refusal: refusal, Update: b.AppUpdate(), Ports: append([]PortState{}, ports...),
	}
}

func (b *Backend) emitState() {
	if b.sink != nil {
		b.sink.State(b.State())
	}
}

func (b *Backend) coreChanged() {
	st, _ := b.core.Status()
	if st != coremgr.Running {
		b.stopStreams()
		b.forgetPorts()
		if st == coremgr.Crashed {
			go b.releaseProxy()
			_, errText := b.core.Status()
			b.event("core", "error", "The core stopped: {error}", map[string]string{"error": errText}, true)
		}
	}
	b.emitState()
}

// Client is the running core's API.
func (b *Backend) Client() (*mihomoapi.Client, error) {
	c := b.core.Client()
	if c == nil {
		return nil, errors.New("core is not running")
	}
	return c, nil
}

// writeRuntime writes the configuration the core runs from the profile in
// use and the settings; a new controller is made only for a fresh start.
func (b *Backend) writeRuntime(fresh bool) error {
	s := settings.Load()
	p, ok := profiles.Get(s.Profile)
	if !ok {
		return errors.New("no profile selected")
	}
	body, err := os.ReadFile(p.Path())
	if err != nil {
		return err
	}
	if fi, err := os.Stat(p.Path()); err == nil {
		b.mu.Lock()
		b.profileRead = profileStamp{p.ID, fi.ModTime(), fi.Size()}
		b.mu.Unlock()
	}
	ctl := b.core.Controller()
	if fresh || ctl.Addr == "" {
		if ctl, err = b.core.NewController(); err != nil {
			return err
		}
	}
	err = runtimecfg.Write(appdir.RuntimeConfig(), p.ID, body, s, ctl, userrules.List(), modules.List())
	if err != nil && strings.HasPrefix(err.Error(), "profile: ") {
		b.refuseProfile(p.Path(), err)
	}
	return err
}

// Start runs the core.
func (b *Backend) Start() error {
	b.opMu.Lock()
	err := b.start()
	b.opMu.Unlock()
	if err == nil {
		b.followGroups()
	}
	return err
}

func (b *Backend) start() error {
	b.setBusy("starting")
	defer b.setBusy("")
	b.pickRunner()
	if err := b.writeRuntime(true); err != nil {
		return err
	}
	if err := b.test(); err != nil {
		b.reportCrash(err)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := b.core.Start(ctx); err != nil {
		return err
	}
	if err := b.waitMixedPort(ctx, settings.Load().MixedPort); err != nil {
		_ = b.core.Stop()
		b.reportCrash(err)
		return err
	}
	b.startStreams()
	b.watchPorts()
	if settings.Load().SystemProxy {
		if err := b.applyProxy(true); err != nil {
			log.Println("system proxy:", err)
		}
	}
	return nil
}

// The controller starts before listeners. Do not claim a usable core or set
// the system proxy until this process really owns the mixed port.
func (b *Backend) waitMixedPort(ctx context.Context, port int) error {
	if port <= 0 { // no mixed port configured: nothing to wait for
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		c := b.core.Client()
		if c == nil {
			return errors.New("core stopped before the mixed port opened")
		}
		ports, err := c.Listening(ctx)
		if err == nil {
			for _, listening := range ports {
				if listening == port {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("core did not open mixed port %d: %w", port, ctx.Err())
		case <-ticker.C:
		}
	}
}

// reportCrash records a failure before the core ran (a bad config).
func (b *Backend) reportCrash(err error) {
	if b.sink != nil {
		st := b.State()
		st.Core, st.CoreError = string(coremgr.Crashed), err.Error()
		b.sink.State(st)
	}
}

// Stop ends the core, and the system proxy with it.
func (b *Backend) Stop() error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	return b.stop()
}

func (b *Backend) stop() error {
	b.setBusy("stopping")
	defer b.setBusy("")
	b.releaseProxy()
	return b.core.Stop()
}

func (b *Backend) Restart() error {
	b.opMu.Lock()
	b.setBusy("restarting")
	_ = b.core.Stop()
	err := b.start()
	b.opMu.Unlock()
	if err == nil {
		b.followGroups()
	}
	return err
}

// Reload makes the running core take the profile and settings again,
// without dropping its controller; a stopped core is left stopped.
func (b *Backend) Reload() error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	return b.reload()
}

// reload requires opMu; profile selection and its reload are one transaction.
func (b *Backend) reload() error {
	c := b.core.Client()
	if c == nil {
		return nil
	}
	b.setBusy("reloading")
	defer b.setBusy("")
	if err := b.writeRuntime(false); err != nil {
		return err
	}
	if err := b.test(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := c.ReloadConfigs(ctx, appdir.RuntimeConfig())
	if err == nil {
		b.aiEgress.clear()
		b.watchPorts()
	}
	return err
}

// Shutdown is the app quitting.
func (b *Backend) Shutdown() {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if b.shutdown {
		return
	}
	b.shutdown = true
	close(b.done)
	b.releaseProxy()
	_ = b.core.Stop()
	_ = b.Usage().Flush()
}

// SetMode switches rule / global / direct.
func (b *Backend) SetMode(mode string) error {
	if err := b.setMode(mode); err != nil {
		return err
	}
	b.noteManual("mode", mode)
	return nil
}

func (b *Backend) setMode(mode string) error {
	switch mode {
	case "rule", "global", "direct":
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
	if c := b.core.Client(); c != nil {
		if err := c.PatchConfigs(context.Background(), map[string]any{"mode": mode}); err != nil {
			return err
		}
		// a mode change leaves open connections on the old route
		_ = c.CloseAllConnections(context.Background())
	}
	_, err := settings.Update(func(s *settings.Settings) { s.Mode = mode })
	b.emitState()
	return err
}

// SetSystemProxy turns the system proxy on or off.
func (b *Backend) SetSystemProxy(on bool) error {
	if err := b.setSystemProxy(on); err != nil {
		return err
	}
	b.noteManual("systemProxy", strconv.FormatBool(on))
	return nil
}

func (b *Backend) setSystemProxy(on bool) error {
	if _, err := settings.Update(func(s *settings.Settings) { s.SystemProxy = on }); err != nil {
		return err
	}
	defer b.emitState()
	if !on {
		b.releaseProxy()
		return nil
	}
	if b.core.Client() == nil {
		return nil // set when the core starts
	}
	return b.applyProxy(true)
}

// The system proxy, as variables so tests leave the machine's alone.
var (
	proxySet      = sysproxy.Set
	proxyClear    = sysproxy.Clear
	proxyPointsAt = sysproxy.PointsAt
)

const proxyHost = "127.0.0.1"

// listening says whether something accepts connections on the local port.
func listening(port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(proxyHost, strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func (b *Backend) applyProxy(on bool) error {
	s := settings.Load()
	if !on {
		return proxyClear()
	}
	if err := proxySet(proxyHost, s.MixedPort, s.Bypass); err != nil {
		return err
	}
	b.mu.Lock()
	b.proxyOwned, b.proxyLost = true, false
	b.mu.Unlock()
	return nil
}

// releaseProxy turns the system proxy off if it is ours: set by this run,
// or pointing at our port (left by a run that didn't get to clear it).
func (b *Backend) releaseProxy() {
	b.mu.Lock()
	owned := b.proxyOwned
	b.proxyOwned, b.proxyLost = false, false
	b.mu.Unlock()
	if owned || proxyPointsAt(proxyHost, settings.Load().MixedPort) {
		_ = proxyClear()
	}
}

// PatchSettings changes settings; those the core runs with are applied.
// When the core refuses them, they are put back as they were, so the
// settings never claim what the core doesn't run with.
func (b *Backend) PatchSettings(fn func(*settings.Settings)) (settings.Settings, error) {
	before := settings.Load()
	after, err := settings.Update(fn)
	if err != nil {
		return after, err
	}
	// the mixed port can't take a port module's port, which the core
	// would then hold for the wrong listener
	if after.MixedPort != before.MixedPort {
		for _, m := range modules.List() {
			if m.Port != nil && m.Enabled && m.Port.Port == after.MixedPort {
				after, _ = settings.Update(func(s *settings.Settings) { s.MixedPort = before.MixedPort })
				return after, fmt.Errorf("port %d is used by the module %s", m.Port.Port, m.Name)
			}
		}
	}
	b.emitState()
	if coreSettings(before) != coreSettings(after) {
		if err := b.Reload(); err != nil {
			// unless another change came in meanwhile
			want := coreSettings(after)
			after, _ = settings.Update(func(s *settings.Settings) {
				if coreSettings(*s) == want {
					setCoreSettings(s, coreSettings(before))
				}
			})
			b.opMu.Lock()
			if b.core.Client() != nil {
				_ = b.writeRuntime(false) // what a crash restart would read
			}
			b.opMu.Unlock()
			b.emitState()
			return after, err
		}
	}
	if before.LogLevel != after.LogLevel {
		b.restartLogs()
	}
	proxyChanged := before.MixedPort != after.MixedPort || fmt.Sprint(before.Bypass) != fmt.Sprint(after.Bypass)
	if proxyChanged && after.SystemProxy && b.core.Client() != nil {
		_ = b.applyProxy(true)
	}
	return after, nil
}

// coreFields is the settings runtimecfg lays over the profile, which a
// change of has the core reload.
type coreFields struct {
	UnifiedDelay                                    bool
	MixedPort                                       int
	AllowLan, IPv6, ICMPForwarding, FindProcess     bool
	GuardIPv6, GuardDNS, BlockSTUN, DNSRespectRules bool
	LogLevel, TunStack                              string
}

func coreSettings(s settings.Settings) coreFields {
	return coreFields{
		UnifiedDelay: s.UnifiedDelay,
		MixedPort:    s.MixedPort, AllowLan: s.AllowLan, IPv6: s.IPv6, ICMPForwarding: s.ICMPForwarding, FindProcess: s.FindProcess,
		GuardIPv6: s.GuardIPv6, GuardDNS: s.GuardDNS, BlockSTUN: s.BlockSTUN, DNSRespectRules: s.DNSRespectRules,
		LogLevel: s.LogLevel, TunStack: s.TunStack,
	}
}

func setCoreSettings(s *settings.Settings, f coreFields) {
	s.UnifiedDelay = f.UnifiedDelay
	s.MixedPort, s.AllowLan, s.IPv6, s.ICMPForwarding, s.FindProcess = f.MixedPort, f.AllowLan, f.IPv6, f.ICMPForwarding, f.FindProcess
	s.GuardIPv6, s.GuardDNS, s.BlockSTUN, s.DNSRespectRules = f.GuardIPv6, f.GuardDNS, f.BlockSTUN, f.DNSRespectRules
	s.LogLevel, s.TunStack = f.LogLevel, f.TunStack
}

// UseProfile switches to another profile.
func (b *Backend) UseProfile(id string) error {
	b.opMu.Lock()
	err := b.useProfile(id)
	b.opMu.Unlock()
	if err != nil {
		return err
	}
	b.noteManual("profile", id)
	b.followGroups()
	return nil
}

func (b *Backend) useProfile(id string) error {
	if _, ok := profiles.Get(id); !ok {
		return errors.New("no such profile")
	}
	before := settings.Load().Profile
	if _, err := settings.Update(func(s *settings.Settings) { s.Profile = id }); err != nil {
		return err
	}
	if err := b.reload(); err != nil {
		_, _ = settings.Update(func(s *settings.Settings) { s.Profile = before })
		_ = b.writeRuntime(false) // what a crash restart would read
		b.emitState()
		return err
	}
	b.emitState()
	b.emitProfiles()
	return nil
}

func (b *Backend) emitProfiles() {
	if b.sink != nil {
		b.sink.Profiles(profiles.List())
	}
}

// ProfileChanged is called after a profile was imported, updated or removed;
// the one in use is reloaded.
func (b *Backend) ProfileChanged(id string) {
	b.emitProfiles()
	if id != "" && id == settings.Load().Profile {
		if err := b.Reload(); err != nil {
			log.Println("reload:", err)
			b.event("profile", "error", "The updated profile was refused, the previous one stays: {error}", map[string]string{"error": err.Error()}, true)
		}
	}
	b.emitState()
}

// RemoveProfile deletes a profile that is not in use.
func (b *Backend) RemoveProfile(id string) error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if id == settings.Load().Profile {
		return errors.New("the profile in use can't be removed")
	}
	if err := profiles.Remove(id); err != nil {
		return err
	}
	if err := modules.ForgetProfile(id); err != nil {
		log.Println("modules:", err)
	}
	b.emitProfiles()
	return nil
}

// DuplicateProfile copies a profile as a local one the user can edit, and
// its own modules and the nodes global ones picked for it; name is the
// copy's.
func (b *Backend) DuplicateProfile(id, name string) (profiles.Profile, error) {
	p, err := profiles.Duplicate(id, name)
	if err != nil {
		return p, err
	}
	if err := modules.CopyProfile(id, p.ID); err != nil {
		log.Println("modules:", err)
	}
	b.ProfileChanged("")
	return p, nil
}

// autoUpdate refreshes subscriptions whose interval has passed.
func (b *Backend) autoUpdate() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		for _, p := range profiles.Due(time.Now()) {
			if b.SavingData() {
				break // they stay due, for a network that isn't metered
			}
			if _, err := profiles.Update(p.ID); err != nil {
				log.Printf("update %s: %v", p.Name, err)
				b.event("profile", "warning", "Couldn't update {name}: {error}", map[string]string{"name": p.Name, "error": err.Error()}, true)
				continue
			}
			b.ProfileChanged(p.ID)
		}
		<-t.C
	}
}

// SetRules replaces the user's rules and has the core take them. Rules the
// core refuses are not kept: the previous ones are restored.
func (b *Backend) SetRules(rs []userrules.Rule) error {
	before := userrules.List()
	if err := userrules.Save(rs); err != nil {
		return err
	}
	if err := b.Reload(); err != nil {
		_ = userrules.Save(before)
		b.restoreRuntime()
		return err
	}
	return nil
}

// restoreRuntime writes the configuration again after a refused change was
// undone, so the file matches what the core runs.
func (b *Backend) restoreRuntime() {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	_ = b.writeRuntime(false)
}

// SetModules replaces the user's modules. The configuration they make is
// tested even with the core stopped, and taken at once when it runs; a
// configuration the core refuses restores the previous modules.
func (b *Backend) SetModules(ms []modules.Module) error {
	before := modules.List()
	if err := checkPorts(before, ms); err != nil {
		return err
	}
	if err := modules.Save(ms); err != nil {
		return err
	}
	if err := b.check(); err != nil {
		_ = modules.Save(before)
		b.restoreRuntime()
		return err
	}
	return nil
}

// check has the running core take the configuration again, or with it
// stopped, tests the configuration it would start with.
func (b *Backend) check() error {
	b.opMu.Lock()
	defer b.opMu.Unlock()
	if b.core.Client() != nil {
		return b.reload()
	}
	if err := b.writeRuntime(false); err != nil {
		return err
	}
	return b.test()
}

// AddRule puts r first among the user's rules.
func (b *Backend) AddRule(r userrules.Rule) error {
	if err := r.Check(); err != nil {
		return err
	}
	return b.SetRules(userrules.Added(userrules.List(), r))
}
