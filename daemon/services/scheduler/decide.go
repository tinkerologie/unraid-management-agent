package scheduler

import "time"

// Reason explains a collector's effective interval in GET /collectors/status.
type Reason string

// Reasons, in the order Decide applies them.
const (
	// ReasonManual: Manual mode, the configured interval.
	ReasonManual Reason = "manual"
	// ReasonActive: something consumes the data, the active interval.
	ReasonActive Reason = "active"
	// ReasonEvent: no consumer, change events trigger runs; the safety-net poll.
	ReasonEvent Reason = "event"
	// ReasonIdle: no consumer and no events, the idle interval.
	ReasonIdle Reason = "idle"
	// ReasonFloor: an enabled alert rule holds the collector at its floor.
	ReasonFloor Reason = "floor"
)

// lowPowerFactor stretches idle and safety-net intervals in low power mode.
const lowPowerFactor = 4

// Inputs are the scheduling inputs of one collector at one moment.
type Inputs struct {
	// Manual is Manual mode; Configured is then the interval to use.
	Manual     bool
	Configured time.Duration
	// Active reports a consumer (WebSocket subscriber, MQTT, recent read).
	Active bool
	// Floor reports that an enabled alert rule reads the collector.
	Floor    bool
	LowPower bool
	// LastRun is when the collector last ran; zero means never.
	LastRun time.Time
}

// Decision is when a collector should run next, and why.
type Decision struct {
	Interval time.Duration
	Reason   Reason
	// Next is LastRun+Interval, or the zero time to run now when it never ran.
	Next time.Time
}

// Decide applies the rules in order: Manual mode uses the configured interval;
// an active consumer the active interval; otherwise the safety-net poll for
// an event-driven collector or the idle interval (both x4 in low power mode);
// finally a faster required floor wins. Floors are never scaled.
func Decide(p Policy, in Inputs) Decision {
	var d Decision
	switch {
	case in.Manual:
		d = Decision{Interval: in.Configured, Reason: ReasonManual}
	case in.Active:
		d = Decision{Interval: p.Active, Reason: ReasonActive}
	default:
		d = Decision{Interval: p.Idle, Reason: ReasonIdle}
		if p.EventDriven {
			d.Reason = ReasonEvent
		}
		if in.LowPower {
			d.Interval *= lowPowerFactor
		}
	}
	if !in.Manual && in.Floor && p.Floor > 0 && p.Floor < d.Interval {
		d = Decision{Interval: p.Floor, Reason: ReasonFloor}
	}
	if !in.LastRun.IsZero() {
		d.Next = in.LastRun.Add(d.Interval)
	}
	return d
}

// BaselineReason is the reason reported for a collector that is not running:
// manual in Manual mode, otherwise event or idle.
func BaselineReason(p Policy, manual bool) Reason {
	switch {
	case manual:
		return ReasonManual
	case p.EventDriven:
		return ReasonEvent
	default:
		return ReasonIdle
	}
}
