package scheduler

import (
	"sync"
	"time"
)

// ReadWindow is how long a REST or MCP read keeps a collector active.
const ReadWindow = 5 * time.Minute

// Demand tracks which collectors have consumers: WebSocket clients subscribed
// to their topics, the MQTT client while connected, and REST or MCP reads in
// the last ReadWindow. It is safe for concurrent use.
type Demand struct {
	mu    sync.Mutex
	clock Clock
	// wsTopics counts WebSocket clients per topic; wsAll counts clients
	// without a topic filter, which receive every topic.
	wsTopics map[string]int
	wsAll    int
	mqtt     bool
	mqttFor  map[string]bool
	reads    map[string]time.Time
	onChange func(collectors []string)
}

// NewDemand returns an empty tracker.
func NewDemand(clock Clock) *Demand {
	return &Demand{clock: clock, wsTopics: map[string]int{}, mqttFor: map[string]bool{}, reads: map[string]time.Time{}}
}

// SetOnChange registers fn to receive the collectors whose active state
// changed. fn is called without the tracker's lock held.
func (d *Demand) SetOnChange(fn func(collectors []string)) {
	d.mu.Lock()
	d.onChange = fn
	d.mu.Unlock()
}

// Active reports whether the collector has a consumer.
func (d *Demand) Active(collector string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.activeLocked(collector)
}

// activeLocked is Active with d.mu held.
func (d *Demand) activeLocked(collector string) bool {
	if d.wsAll > 0 || (d.mqtt && d.mqttFor[collector]) {
		return true
	}
	if p, ok := policies[collector]; ok {
		for _, t := range p.Topics {
			if d.wsTopics[t] > 0 {
				return true
			}
		}
	}
	at, ok := d.reads[collector]
	return ok && d.clock.Now().Sub(at) < ReadWindow
}

// WSAdd records a WebSocket client with the given topic filter; nil means
// every topic.
func (d *Demand) WSAdd(topics map[string]bool) { d.ws(topics, 1) }

// WSRemove forgets a WebSocket client with the given topic filter.
func (d *Demand) WSRemove(topics map[string]bool) { d.ws(topics, -1) }

// ws adds delta clients with the given topic filter.
func (d *Demand) ws(topics map[string]bool, delta int) {
	var affected []string
	if topics == nil {
		affected = CollectorNames()
	} else {
		for t := range topics {
			if name, ok := CollectorForTopic(t); ok {
				affected = append(affected, name)
			}
		}
	}
	d.update(affected, func() {
		if topics == nil {
			d.wsAll += delta
			return
		}
		for t := range topics {
			d.wsTopics[t] += delta
			if d.wsTopics[t] <= 0 {
				delete(d.wsTopics, t)
			}
		}
	})
}

// SetMQTT records whether the MQTT client is connected and which collectors
// it publishes.
func (d *Demand) SetMQTT(connected bool, collectors map[string]bool) {
	d.mu.Lock()
	affected := make([]string, 0, len(collectors)+len(d.mqttFor))
	for name := range d.mqttFor {
		affected = append(affected, name)
	}
	d.mu.Unlock()
	for name := range collectors {
		affected = append(affected, name)
	}
	d.update(affected, func() {
		d.mqtt = connected
		d.mqttFor = collectors
	})
}

// RecordRead marks the collectors as read now.
func (d *Demand) RecordRead(collectors ...string) {
	d.update(collectors, func() {
		now := d.clock.Now()
		for _, name := range collectors {
			d.reads[name] = now
		}
	})
}

// update applies change and reports the affected collectors whose active
// state flipped.
func (d *Demand) update(affected []string, change func()) {
	d.mu.Lock()
	before := make(map[string]bool, len(affected))
	for _, name := range affected {
		before[name] = d.activeLocked(name)
	}
	change()
	var changed []string
	for name, was := range before {
		if d.activeLocked(name) != was {
			changed = append(changed, name)
		}
	}
	fn := d.onChange
	d.mu.Unlock()
	if fn != nil && len(changed) > 0 {
		fn(changed)
	}
}
