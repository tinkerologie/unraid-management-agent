package alerting

import (
	"math"
	"testing"
	"time"
)

func ts(base time.Time, sec int) time.Time { return base.Add(time.Duration(sec) * time.Second) }

func TestMetricsHistory_SlopeAndETA(t *testing.T) {
	h := NewMetricsHistory(240, time.Hour)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i <= 60; i++ {
		h.recordAt("array_used_pct", "", 50.0+0.01*float64(i), ts(base, i))
	}
	sl := h.slope(h.globalSeries["array_used_pct"])
	if math.Abs(sl-0.01) > 1e-4 {
		t.Errorf("slope = %v, want ~0.01/s", sl)
	}
	eta := h.etaToThreshold(h.globalSeries["array_used_pct"], 100.0)
	if math.Abs(eta-1.372) > 0.05 {
		t.Errorf("eta = %v h, want ~1.37h", eta)
	}
	if got := h.etaToThreshold(h.globalSeries["array_used_pct"], 10.0); got != -1 {
		t.Errorf("eta to below-current = %v, want -1", got)
	}
}

func TestMetricsHistory_BoundedByCount(t *testing.T) {
	h := NewMetricsHistory(5, time.Hour)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 20; i++ {
		h.recordAt("cpu_temp", "", float64(i), ts(base, i))
	}
	if n := len(h.globalSeries["cpu_temp"]); n != 5 {
		t.Errorf("len = %d, want 5 (count cap)", n)
	}
}

func TestMetricsHistory_BoundedByAge(t *testing.T) {
	h := NewMetricsHistory(1000, 10*time.Second)
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 30; i++ {
		h.recordAt("cpu_temp", "", float64(i), ts(base, i))
	}
	// cutoff = newest(t=29s) - 10s = t=19s; samples dropped where t.Before(cutoff)
	// i.e. t < 19 → keeps t in [19,29] = 11 samples.
	if got := len(h.globalSeries["cpu_temp"]); got != 11 {
		t.Errorf("len = %d, want 11 (age cap keeps t in [19,29])", got)
	}
}

func TestMetricsHistory_PrunesVanishedEntities(t *testing.T) {
	h := NewMetricsHistory(240, time.Hour)
	base := time.Unix(1_700_000_000, 0)
	h.recordAt("disk_temp", "sda", 40, ts(base, 0))
	h.recordAt("disk_temp", "sdb", 41, ts(base, 0))
	h.pruneEntities("disk_temp", map[string]bool{"sda": true})
	if _, ok := h.entitySeries["disk_temp"]["sdb"]; ok {
		t.Error("sdb should be pruned")
	}
	if _, ok := h.entitySeries["disk_temp"]["sda"]; !ok {
		t.Error("sda should remain")
	}
}
