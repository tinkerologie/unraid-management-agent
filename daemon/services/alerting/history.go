package alerting

import (
	"math"
	"sync"
	"time"
)

// sample is one timestamped metric reading.
type sample struct {
	t time.Time
	v float64
}

// MetricsHistory holds bounded in-memory ring buffers of metric samples for
// trend/ETA computation. Tier-0: memory-only, never persisted. Sampled on the
// alert eval tick. Thread-safe for concurrent reads (history query API).
type MetricsHistory struct {
	mu       sync.RWMutex
	maxCount int
	maxAge   time.Duration

	globalSeries map[string][]sample
	entitySeries map[string]map[string][]sample
}

// NewMetricsHistory creates a history bounded by maxCount samples and maxAge per series.
func NewMetricsHistory(maxCount int, maxAge time.Duration) *MetricsHistory {
	return &MetricsHistory{
		maxCount:     maxCount,
		maxAge:       maxAge,
		globalSeries: map[string][]sample{},
		entitySeries: map[string]map[string][]sample{},
	}
}

func (h *MetricsHistory) recordAt(metric, entity string, v float64, t time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if entity == "" {
		h.globalSeries[metric] = h.appendBounded(h.globalSeries[metric], sample{t, v})
		return
	}
	if h.entitySeries[metric] == nil {
		h.entitySeries[metric] = map[string][]sample{}
	}
	h.entitySeries[metric][entity] = h.appendBounded(h.entitySeries[metric][entity], sample{t, v})
}

// Record records a sample at the given time (caller passes now).
func (h *MetricsHistory) Record(metric, entity string, v float64, now time.Time) {
	h.recordAt(metric, entity, v, now)
}

func (h *MetricsHistory) appendBounded(s []sample, x sample) []sample {
	s = append(s, x)
	cutoff := x.t.Add(-h.maxAge)
	start := 0
	for start < len(s) && s[start].t.Before(cutoff) {
		start++
	}
	if len(s)-start > h.maxCount {
		start = len(s) - h.maxCount
	}
	if start == 0 {
		return s
	}
	trimmed := make([]sample, len(s)-start)
	copy(trimmed, s[start:])
	return trimmed
}

// pruneEntities drops per-entity series for a metric whose entity is not in keep.
func (h *MetricsHistory) pruneEntities(metric string, keep map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.entitySeries[metric]
	for id := range m {
		if !keep[id] {
			delete(m, id)
		}
	}
}

// slope returns least-squares slope in value-units per SECOND. 0 if <2 points.
func (h *MetricsHistory) slope(s []sample) float64 {
	if len(s) < 2 {
		return 0
	}
	t0 := s[0].t
	var n, sx, sy, sxx, sxy float64
	for _, p := range s {
		x := p.t.Sub(t0).Seconds()
		y := p.v
		n++
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

// etaToThreshold returns hours until the series, extrapolated linearly, reaches
// threshold. Returns -1 if not trending toward it (flat/wrong direction).
func (h *MetricsHistory) etaToThreshold(s []sample, threshold float64) float64 {
	if len(s) < 2 {
		return -1
	}
	sl := h.slope(s)
	cur := s[len(s)-1].v
	diff := threshold - cur
	if sl == 0 || (diff > 0) != (sl > 0) {
		return -1
	}
	seconds := diff / sl
	if seconds <= 0 || math.IsInf(seconds, 0) || math.IsNaN(seconds) {
		return -1
	}
	return seconds / 3600.0
}

// SeriesSnapshot returns a copy of a series (global if entity=="") for the query API.
func (h *MetricsHistory) SeriesSnapshot(metric, entity string) []sample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var src []sample
	if entity == "" {
		src = h.globalSeries[metric]
	} else {
		src = h.entitySeries[metric][entity]
	}
	out := make([]sample, len(src))
	copy(out, src)
	return out
}
