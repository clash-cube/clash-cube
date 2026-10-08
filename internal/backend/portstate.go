package backend

import (
	"context"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/localhost-copilot/clashcube/internal/modules"
	"github.com/localhost-copilot/clashcube/internal/settings"
)

// PortState is how a port module's port came up in the running core. The
// core only logs a listener it can't open and goes on, so it is asked which
// ports it really holds (/clashcube/listening).
type PortState struct {
	Port   int    `json:"port"`
	Status string `json:"status"`           // "open", "taken" (by another program) or "closed"
	Holder string `json:"holder,omitempty"` // the program holding a taken port, when known
}

// portStates is the state of each of ps given the ports the core listens
// on: one it doesn't hold is taken when something else does.
func portStates(listening []int, ps []modules.Port, free func(int) bool, holder func(int) string) []PortState {
	held := map[int]bool{}
	for _, p := range listening {
		held[p] = true
	}
	out := []PortState{}
	for _, p := range ps {
		st := PortState{Port: p.Port, Status: "open"}
		if !held[p.Port] {
			st.Status = "closed"
			if !free(p.Port) {
				st.Status, st.Holder = "taken", holder(p.Port)
			}
		}
		out = append(out, st)
	}
	return out
}

// portHolder names the program listening on port, "" when lsof can't see
// it (another user's, such as root's).
func portHolder(port int) string {
	out, _ := exec.Command("/usr/sbin/lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fc").Output()
	for _, line := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(line, "c"); ok && name != "" {
			return name
		}
	}
	return ""
}

// watchPorts finds out how the ports came up after the core started or
// took a configuration, and says so in the state. The core opens its
// listeners after its API answers, so a port not open yet is asked about
// again for a while before it is called closed.
func (b *Backend) watchPorts() {
	b.mu.Lock()
	b.portGen++
	gen := b.portGen
	b.mu.Unlock()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			c := b.core.Client()
			if c == nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			listening, err := c.Listening(ctx)
			cancel()
			if err != nil {
				return
			}
			s := settings.Load()
			st := portStates(listening, served(s.Profile, modules.List()), portFree, portHolder)
			settled := true
			for _, p := range st {
				settled = settled && p.Status != "closed"
			}
			if settled || time.Now().After(deadline) {
				b.mu.Lock()
				changed := gen == b.portGen && !reflect.DeepEqual(b.ports, st)
				if changed {
					b.ports = st
				}
				b.mu.Unlock()
				if changed {
					b.emitState()
				}
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// forgetPorts drops the ports' states, with the core stopped.
func (b *Backend) forgetPorts() {
	b.mu.Lock()
	b.portGen++
	b.ports = nil
	b.mu.Unlock()
}
