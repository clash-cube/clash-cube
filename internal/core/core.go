// Package core runs mihomo in this process: the `clashcube core` role. The
// GUI starts it as a child (or the helper does, as root, for TUN) and talks
// to it over the external controller it is given.
package core

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/metacubex/mihomo/component/updater"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/log"
	"go.uber.org/automaxprocs/maxprocs"
)

// Version is the bundled mihomo's.
func Version() string { return C.Version }

// Main is `clashcube core -d <home> -f <config> [-ext-ctl addr] [-secret s] [-t]`.
// The secret may also come in $CLASHCUBE_SECRET, which keeps it out of ps.
func Main(args []string) error {
	fs := flag.NewFlagSet("core", flag.ContinueOnError)
	home := fs.String("d", "", "mihomo home directory")
	file := fs.String("f", "", "configuration file")
	ctl := fs.String("ext-ctl", "", "override external controller address")
	secret := fs.String("secret", os.Getenv("CLASHCUBE_SECRET"), "override controller secret")
	test := fs.Bool("t", false, "test configuration and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *home == "" || *file == "" {
		return errors.New("core: -d and -f are required")
	}
	os.Unsetenv("CLASHCUBE_SECRET")
	// mihomo leaves the TUN without an IPv6 address when this Mac has no
	// global IPv6 at start; joining an IPv6 network later would then route
	// IPv6 around the TUN. darwin's utun always takes the address.
	os.Setenv("SKIP_SYSTEM_IPV6_CHECK", "1")

	guardResolver()
	_, _ = maxprocs.Set(maxprocs.Logger(func(string, ...any) {}))

	absHome, _ := filepath.Abs(*home)
	absFile, _ := filepath.Abs(*file)
	C.SetHomeDir(absHome)
	C.SetConfig(absFile)
	if err := config.Init(absHome); err != nil {
		return fmt.Errorf("init home: %w", err)
	}

	if *test {
		if _, err := executor.Parse(); err != nil {
			return fmt.Errorf("configuration test failed: %w", err)
		}
		fmt.Println("configuration test is successful")
		return nil
	}

	var opts []hub.Option
	if *ctl != "" {
		opts = append(opts, hub.WithExternalController(*ctl))
	}
	if *secret != "" {
		opts = append(opts, hub.WithSecret(*secret))
	}
	if err := hub.Parse(nil, opts...); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if updater.GeoAutoUpdate() {
		updater.RegisterGeoUpdater()
	}
	defer executor.Shutdown()

	// The GUI going away (its end of our stdin closing) ends us too, so a
	// crashed GUI never leaves a core holding the ports.
	parentGone := make(chan struct{})
	if os.Getenv("CLASHCUBE_WATCH_STDIN") == "1" {
		go func() {
			buf := make([]byte, 64)
			for {
				if _, err := os.Stdin.Read(buf); err != nil {
					close(parentGone)
					return
				}
			}
		}()
	}

	term := make(chan os.Signal, 1)
	hup := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(hup, syscall.SIGHUP)
	for {
		select {
		case <-term:
			return nil
		case <-parentGone:
			log.Warnln("parent process gone, exiting")
			return nil
		case <-hup:
			if err := hub.Parse(nil, opts...); err != nil {
				log.Errorln("reload config: %s", err)
			}
		}
	}
}

// guardResolver mirrors mihomo's main: nothing may reach the system resolver
// behind mihomo's own DNS.
func guardResolver() {
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("net.DefaultResolver must not be used in core")
	}
}
