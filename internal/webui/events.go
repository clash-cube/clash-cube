package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/backend"
	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
	"github.com/localhost-copilot/clashcube/internal/profiles"
)

type event struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Data any    `json:"data"`
}

type cursor struct {
	Stream string `json:"stream"`
	After  uint64 `json:"after"`
}

type batch struct {
	Stream string  `json:"stream"`
	After  uint64  `json:"after"`
	Reset  bool    `json:"reset"`
	Events []event `json:"events"`
}

// Broker keeps independent cursors for browser sessions. A bounded history and
// a per-process stream ID let slow clients and reconnects recover from snapshots.
type Broker struct {
	mu       sync.Mutex
	stream   string
	seq      uint64
	events   []event
	changed  chan struct{}
	state    *backend.State
	profiles []profiles.Profile
}

func NewBroker() *Broker {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return &Broker{stream: hex.EncodeToString(id[:]), changed: make(chan struct{})}
}

func (b *Broker) Publish(name string, data any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	b.events = append(b.events, event{b.seq, name, data})
	if len(b.events) > 256 {
		b.events = append([]event(nil), b.events[len(b.events)-256:]...)
	}
	close(b.changed)
	b.changed = make(chan struct{})
}

func (b *Broker) Poll(ctx context.Context, c cursor) batch {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		b.mu.Lock()
		out := batch{Stream: b.stream, After: b.seq, Events: []event{}}
		oldest := b.seq
		if len(b.events) > 0 {
			oldest = b.events[0].ID - 1
		}
		if c.Stream != b.stream || c.After < oldest || c.After > b.seq {
			out.Reset = true
			if b.state != nil {
				out.Events = append(out.Events, event{b.seq, "state", *b.state})
			}
			out.Events = append(out.Events, event{b.seq, "profiles", b.profiles})
		} else {
			for _, e := range b.events {
				if e.ID > c.After {
					out.Events = append(out.Events, e)
				}
			}
		}
		changed := b.changed
		b.mu.Unlock()
		if out.Reset || len(out.Events) > 0 {
			return out
		}
		select {
		case <-ctx.Done():
			return out
		case <-timer.C:
			return out
		case <-changed:
		}
	}
}

func (b *Broker) State(s backend.State) {
	b.mu.Lock()
	b.state = &s
	b.mu.Unlock()
	b.Publish("state", s)
}
func (b *Broker) Profiles(ps []profiles.Profile) {
	b.mu.Lock()
	b.profiles = ps
	b.mu.Unlock()
	b.Publish("profiles", ps)
}
func (b *Broker) Traffic(t mihomoapi.Traffic)     { b.Publish("traffic", t) }
func (b *Broker) Memory(m mihomoapi.Memory)       { b.Publish("memory", m) }
func (b *Broker) Log(l mihomoapi.Log)             { b.Publish("log", l) }
func (b *Broker) Event(e backend.Event)           { b.Publish("event", e) }
func (b *Broker) Latency(l backend.LatencySample) { b.Publish("connectivity", l) }
