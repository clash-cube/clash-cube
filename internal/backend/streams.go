package backend

import (
	"context"
	"time"

	"github.com/localhost-copilot/mihomobar/internal/mihomoapi"
	"github.com/localhost-copilot/mihomobar/internal/settings"
)

// startStreams follows the core's traffic, memory and logs while it runs.
func (b *Backend) startStreams() {
	c := b.core.Client()
	if c == nil || b.sink == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	if b.streamCancel != nil {
		b.streamCancel()
	}
	b.streamCancel = cancel
	b.mu.Unlock()
	go follow(ctx, func(ctx context.Context) error {
		return mihomoapi.Stream(ctx, c, "/traffic", b.sink.Traffic)
	})
	go follow(ctx, func(ctx context.Context) error {
		return mihomoapi.Stream(ctx, c, "/memory", b.sink.Memory)
	})
	b.restartLogs()
}

// restartLogs follows /logs at the level the settings ask for.
func (b *Backend) restartLogs() {
	c := b.core.Client()
	if c == nil || b.sink == nil {
		return
	}
	level := settings.Load().LogLevel
	if level == "" || level == "silent" {
		level = "info"
	}
	b.mu.Lock()
	parent := b.streamCancel
	if b.logWatch != nil {
		b.logWatch()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.logWatch = cancel
	b.logLevel = level
	b.mu.Unlock()
	if parent == nil {
		cancel()
		return
	}
	go follow(ctx, func(ctx context.Context) error {
		return mihomoapi.Stream(ctx, c, "/logs?level="+level, b.sink.Log)
	})
}

func (b *Backend) stopStreams() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.streamCancel != nil {
		b.streamCancel()
		b.streamCancel = nil
	}
	if b.logWatch != nil {
		b.logWatch()
		b.logWatch = nil
	}
}

// follow runs fn again after it drops, backing off, until ctx ends.
func follow(ctx context.Context, fn func(context.Context) error) {
	wait := 500 * time.Millisecond
	for ctx.Err() == nil {
		start := time.Now()
		_ = fn(ctx)
		if time.Since(start) > 10*time.Second {
			wait = 500 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 8*time.Second)
	}
}
