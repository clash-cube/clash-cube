package backend

import (
	"context"
	"sync"
	"time"

	"github.com/localhost-copilot/clashcube/internal/mihomoapi"
)

type aiEgressKey struct {
	client *mihomoapi.Client
	node   string
	url    string
}

type aiEgressEntry struct {
	done    chan struct{}
	body    string
	err     error
	expires time.Time
}

// A node's trace is shared across services, including concurrent checks and
// failures. Only raw trace data is cached: region policy and route chains are
// service-specific. A new core, config reload or network reset invalidates it.
type aiEgressCache struct {
	mu      sync.Mutex
	entries map[aiEgressKey]*aiEgressEntry
}

func (c *aiEgressCache) clear() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

func (c *aiEgressCache) get(ctx context.Context, client *mihomoapi.Client, node, traceURL string) (string, error) {
	key := aiEgressKey{client, node, traceURL}
	c.mu.Lock()
	for k, entry := range c.entries {
		if !entry.expires.IsZero() && time.Now().After(entry.expires) {
			delete(c.entries, k)
		}
	}
	if entry := c.entries[key]; entry != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-entry.done:
			return entry.body, entry.err
		}
	}
	if c.entries == nil {
		c.entries = make(map[aiEgressKey]*aiEgressEntry)
	}
	entry := &aiEgressEntry{done: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()
	body, err := client.Trace(ctx, node, traceURL)
	// An unusable response is also shared; don't multiply retries by the
	// number of services that happen to use this node.
	if err == nil {
		_, err = parseEgress(aiService{}, body, nil)
	}
	ttl := 3 * time.Minute
	if err != nil {
		ttl = 30 * time.Second
	}
	c.mu.Lock()
	entry.body, entry.err, entry.expires = body, err, time.Now().Add(ttl)
	close(entry.done)
	c.mu.Unlock()
	return body, err
}
