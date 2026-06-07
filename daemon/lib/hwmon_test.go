package lib

import "testing"

func TestDiscoverHwmonTempSensorsPlausibility(t *testing.T) {
	// Pure-logic guard: ensures unreliable labels and out-of-range temps are
	// flagged (not silently dropped). Real sysfs scanning is integration-tested
	// on hardware.
	if !classifyTempSensorPlausible("Tctl", 45.0) {
		t.Errorf("normal CPU sensor should be plausible")
	}
	if classifyTempSensorPlausible("AUXTIN", 45.0) {
		t.Errorf("unreliable label should be flagged implausible")
	}
	if classifyTempSensorPlausible("Core 0", 200.0) {
		t.Errorf("out-of-range temp should be flagged implausible")
	}
}
