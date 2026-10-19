//go:build android && clashcube

package main

/*
#include <jni.h>
#include <stdlib.h>
static const char *readString(JNIEnv *e, jstring s) { return (*e)->GetStringUTFChars(e,s,0); }
static void releaseString(JNIEnv *e,jstring s,const char *p) { (*e)->ReleaseStringUTFChars(e,s,p); }
static jstring newString(JNIEnv *e,const char *s) { return (*e)->NewStringUTF(e,s); }
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sync"
	"syscall"
	"unsafe"

	"github.com/localhost-copilot/clashcube/internal/androidbridge"
	"github.com/localhost-copilot/clashcube/internal/core"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/process"
	MC "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/dns"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/listener"
	LC "github.com/metacubex/mihomo/listener/config"
)

func main() {}

type request struct {
	Op, Home, File, Addr, Secret string
	FD                           int
	IPv6                         bool
	DNS                          []string
}

var commands sync.Mutex

//export Java_com_clashcube_android_CoreNative_run
func Java_com_clashcube_android_CoreNative_run(env *C.JNIEnv, obj C.jobject, input C.jstring) (result C.jstring) {
	// No panic or pending Java exception may cross the JNI boundary.
	defer func() {
		if p := recover(); p != nil {
			result = reply(env, fmt.Errorf("core panic: %v", p))
		}
	}()
	text := C.readString(env, input)
	if text == nil {
		return reply(env, fmt.Errorf("missing native request"))
	}
	defer C.releaseString(env, input, text)
	var req request
	if err := json.Unmarshal([]byte(C.GoString(text)), &req); err != nil {
		return reply(env, err)
	}
	commands.Lock()
	defer commands.Unlock()
	return reply(env, run(req))
}

func reply(env *C.JNIEnv, err error) C.jstring {
	response := map[string]string{}
	if err != nil {
		response["error"] = err.Error()
	}
	b, _ := json.Marshal(response)
	s := C.CString(string(b))
	defer C.free(unsafe.Pointer(s))
	return C.newString(env, s)
}

func run(r request) error {
	switch r.Op {
	case "check":
		if err := core.Initialize(r.Home, r.File); err != nil {
			return err
		}
		return core.Validate()
	case "start":
		// Hooks are installed once before any core goroutine starts and live until
		// process exit. Never clear them while a dial could be in flight.
		dialer.DefaultSocketHook = func(network, address string, conn syscall.RawConn) error {
			return conn.Control(func(fd uintptr) { _ = androidbridge.Call(map[string]any{"op": "protect", "fd": int(fd)}, nil) })
		}
		process.DefaultPackageNameResolver = func(m *MC.Metadata) (string, error) {
			if m.RawSrcAddr == nil || m.RawDstAddr == nil {
				return "", process.ErrInvalidNetwork
			}
			protocol := 6
			if m.RawSrcAddr.Network() == "udp" {
				protocol = 17
			}
			var result struct {
				UID     int    `json:"uid"`
				Package string `json:"package"`
			}
			err := androidbridge.Call(map[string]any{"op": "owner", "protocol": protocol, "source": m.RawSrcAddr.String(), "target": m.RawDstAddr.String()}, &result)
			if result.UID >= 0 {
				m.Uid = uint32(result.UID)
			}
			return result.Package, err
		}
		var frozen LC.Tun
		var freeze sync.Once
		listener.AndroidTunConfig = func(t *LC.Tun) {
			freeze.Do(func() {
				value := LC.Tun{Enable: true, Stack: t.Stack, FileDescriptor: r.FD, MTU: 9000,
					Inet4Address: []netip.Prefix{netip.MustParsePrefix("172.19.0.1/30")}, DNSHijack: []string{"0.0.0.0:53", "tcp://any:53"}, DisableICMPForwarding: t.DisableICMPForwarding}
				if r.IPv6 {
					value.Inet6Address = []netip.Prefix{netip.MustParsePrefix("fdfe:dcba:9876::1/126")}
				}
				frozen = value
			})
			// Recreating a listener would close the service-owned FD. Builder/stack
			// changes therefore take effect only on a full service-process restart.
			*t = frozen
			t.Inet4Address = slices.Clone(frozen.Inet4Address)
			t.Inet6Address = slices.Clone(frozen.Inet6Address)
			t.DNSHijack = slices.Clone(frozen.DNSHijack)
		}
		if err := core.Initialize(r.Home, r.File); err != nil {
			return err
		}
		if err := core.Start(hub.WithExternalController(r.Addr), hub.WithSecret(r.Secret)); err != nil {
			return err
		}
		if !listener.GetTunConf().Enable {
			return fmt.Errorf("mihomo could not open Android TUN")
		}
		return nil
	case "dns":
		servers := make([]string, 0, len(r.DNS))
		for _, address := range r.DNS {
			servers = append(servers, net.JoinHostPort(address, "53"))
		}
		dns.UpdateSystemDNS(servers)
		dns.FlushCacheWithDefaultResolver()
		return nil
	case "stop":
		core.Shutdown()
		return nil
	default:
		return fmt.Errorf("unknown native core operation")
	}
}
