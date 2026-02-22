package services

import (
	"sync"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestGetDefaultInterval_AllNames(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)

	tests := []struct {
		name     string
		expected int
	}{
		{"system", 5},
		{"array", 10},
		{"disk", 30},
		{"docker", 10},
		{"vm", 10},
		{"ups", 10},
		{"nut", 10},
		{"gpu", 10},
		{"shares", 60},
		{"network", 15},
		{"hardware", 60},
		{"zfs", 30},
		{"notification", 30},
		{"registration", 300},
		{"unassigned", 60},
		{"unknown_collector", 30}, // fallback
		{"", 30},                  // empty name fallback
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cm.getDefaultInterval(tt.name)
			if got != tt.expected {
				t.Errorf("getDefaultInterval(%q) = %d, want %d", tt.name, got, tt.expected)
			}
		})
	}
}

func TestBroadcastStateChange_EventPublished(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)

	// Register a collector first
	cm.Register("test-broadcast", func(dctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	// Subscribe to the collector_state_change topic
	ch := ctx.Hub.Sub("collector_state_change")

	// Enable the collector — this calls broadcastStateChange internally
	err := cm.EnableCollector("test-broadcast")
	if err != nil {
		t.Fatalf("EnableCollector failed: %v", err)
	}

	// Wait for the event
	select {
	case msg := <-ch:
		event, ok := msg.(dto.CollectorStateEvent)
		if !ok {
			t.Fatalf("Expected dto.CollectorStateEvent, got %T", msg)
		}
		if event.Collector != "test-broadcast" {
			t.Errorf("Event.Collector = %q, want %q", event.Collector, "test-broadcast")
		}
		if !event.Enabled {
			t.Error("Expected event.Enabled=true for EnableCollector")
		}
		if event.Event != "collector_state_change" {
			t.Errorf("Event.Event = %q, want %q", event.Event, "collector_state_change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for collector_state_change event")
	}

	// Now disable and check the event
	err = cm.DisableCollector("test-broadcast")
	if err != nil {
		t.Fatalf("DisableCollector failed: %v", err)
	}

	select {
	case msg := <-ch:
		event, ok := msg.(dto.CollectorStateEvent)
		if !ok {
			t.Fatalf("Expected dto.CollectorStateEvent, got %T", msg)
		}
		if event.Collector != "test-broadcast" {
			t.Errorf("Event.Collector = %q, want %q", event.Collector, "test-broadcast")
		}
		if event.Enabled {
			t.Error("Expected event.Enabled=false for DisableCollector")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for disable state change event")
	}

	ctx.Hub.Unsub(ch)
	cm.StopAll()
}

func TestCollectorManager_RegisterAllCollectors(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)

	cm.RegisterAllCollectors()

	names := cm.GetCollectorNames()
	expectedNames := []string{
		"system", "array", "disk", "docker", "vm", "ups", "nut",
		"gpu", "shares", "network", "hardware", "zfs", "notification",
		"registration", "unassigned",
	}

	if len(names) != len(expectedNames) {
		t.Errorf("RegisterAllCollectors registered %d collectors, want %d", len(names), len(expectedNames))
	}

	// Verify each expected name is present
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, expected := range expectedNames {
		if !nameSet[expected] {
			t.Errorf("Missing collector: %q", expected)
		}
	}
}

func TestCollectorManager_UpdateIntervalRestartsRunning(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)

	cm.Register("interval-test", func(dctx *domain.Context) Collector {
		return &mockCollector{}
	}, 10, false)

	// Enable the collector
	err := cm.EnableCollector("interval-test")
	if err != nil {
		t.Fatalf("EnableCollector failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	status1, _ := cm.GetStatus("interval-test")
	if status1.Status != "running" {
		t.Fatalf("Expected running status, got %q", status1.Status)
	}

	// Update interval while running
	err = cm.UpdateInterval("interval-test", 20)
	if err != nil {
		t.Fatalf("UpdateInterval failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	status2, _ := cm.GetStatus("interval-test")
	if status2.Interval != 20 {
		t.Errorf("Interval = %d, want 20", status2.Interval)
	}
	if status2.Status != "running" {
		t.Errorf("Expected running after interval update, got %q", status2.Status)
	}

	cm.StopAll()
}
