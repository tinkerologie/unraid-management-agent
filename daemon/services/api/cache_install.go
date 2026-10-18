package api

import (
	"context"
	"time"
)

// cacheInstall records when a topic's data last reached the cache (#280).
// gen increases with every install; changed is closed and replaced by it.
type cacheInstall struct {
	gen     uint64
	at      time.Time
	changed chan struct{}
}

// installLocked returns the topic's record, creating it; installMu must be held.
func (c *CacheStore) installLocked(topic string) *cacheInstall {
	if c.installs == nil {
		c.installs = map[string]*cacheInstall{}
	}
	in, ok := c.installs[topic]
	if !ok {
		in = &cacheInstall{changed: make(chan struct{})}
		c.installs[topic] = in
	}
	return in
}

// markInstalled records that the topic's data was just stored in the cache.
func (c *CacheStore) markInstalled(topic string, at time.Time) {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	in := c.installLocked(topic)
	in.gen++
	in.at = at
	close(in.changed)
	in.changed = make(chan struct{})
}

// CacheInstalled returns the topic's install generation and time; the zero
// time means its data never arrived.
func (c *CacheStore) CacheInstalled(topic string) (uint64, time.Time) {
	c.installMu.Lock()
	defer c.installMu.Unlock()
	in := c.installLocked(topic)
	return in.gen, in.at
}

// waitInstalled waits until the topic's generation passes after. It reports
// false when ctx ends first.
func (c *CacheStore) waitInstalled(ctx context.Context, topic string, after uint64) bool {
	for {
		c.installMu.Lock()
		in := c.installLocked(topic)
		gen, changed := in.gen, in.changed
		c.installMu.Unlock()
		if gen > after {
			return true
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return false
		}
	}
}
