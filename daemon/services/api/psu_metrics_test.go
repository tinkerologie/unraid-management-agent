package api

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestPSUMetricsDropMissingAndDisconnectedReadings(t *testing.T) {
	cache := &CacheStore{}
	registry := prometheus.NewRegistry()
	registry.MustRegister(psuMetricsCollector{cache: cache})
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 0 {
		t.Fatal("pending cache fabricated metrics")
	}
	zero := 0.0
	cache.psuCache.Store(&dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "psu-a", State: dto.PSUStateOK, Sensors: []dto.PSUSensor{
		{Name: "output_power", Kind: "power", Value: &zero},
		{Name: "input_power", Kind: "power"},
		{Name: "fan_speed", Kind: "fan", Value: &zero},
	}}}})
	families, err = registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, family := range families {
		seen[family.GetName()] = true
		if family.GetName() == "unraid_psu_power_watts" {
			if len(family.Metric) != 1 || family.Metric[0].GetGauge().GetValue() != 0 {
				t.Fatal("nil and valid zero confused")
			}
		}
	}
	if !seen["unraid_psu_power_watts"] || !seen["unraid_psu_fan_rpm"] {
		t.Fatal("valid zero metrics dropped")
	}
	cache.psuCache.Store(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now()))
	families, err = registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].GetName() != "unraid_psu_state" {
		t.Fatalf("disconnected readings remain: %+v", families)
	}
}
