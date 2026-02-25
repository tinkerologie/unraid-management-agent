package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// mockDataProvider implements DataProvider for testing.
type mockDataProvider struct {
	system     *dto.SystemInfo
	array      *dto.ArrayStatus
	disks      []dto.DiskInfo
	containers []dto.ContainerInfo
	vms        []dto.VMInfo
	ups        *dto.UPSStatus
}

func (m *mockDataProvider) GetSystemCache() *dto.SystemInfo              { return m.system }
func (m *mockDataProvider) GetArrayCache() *dto.ArrayStatus              { return m.array }
func (m *mockDataProvider) GetDisksCache() []dto.DiskInfo                { return m.disks }
func (m *mockDataProvider) GetDockerCache() []dto.ContainerInfo          { return m.containers }
func (m *mockDataProvider) GetVMsCache() []dto.VMInfo                    { return m.vms }
func (m *mockDataProvider) GetUPSCache() *dto.UPSStatus                  { return m.ups }
func (m *mockDataProvider) GetGPUCache() []*dto.GPUMetrics               { return nil }
func (m *mockDataProvider) GetZFSPoolsCache() []dto.ZFSPool              { return nil }
func (m *mockDataProvider) GetNetworkCache() []dto.NetworkInfo           { return nil }
func (m *mockDataProvider) GetNUTCache() *dto.NUTResponse                { return nil }
func (m *mockDataProvider) GetNotificationsCache() *dto.NotificationList { return nil }

func newMockProvider() *mockDataProvider {
	return &mockDataProvider{
		system: &dto.SystemInfo{
			CPUUsage:        75.5,
			RAMUsage:        60.0,
			RAMUsed:         8000000000,
			RAMTotal:        16000000000,
			RAMFree:         8000000000,
			CPUTemp:         55.0,
			MotherboardTemp: 40.0,
			Uptime:          86400,
		},
		array: &dto.ArrayStatus{
			State:               "Started",
			UsedPercent:         45.0,
			FreeBytes:           5000000000000,
			TotalBytes:          10000000000000,
			ParityValid:         true,
			ParityCheckStatus:   "idle",
			ParityCheckProgress: 0,
			NumDisks:            6,
			NumParityDisks:      1,
		},
		disks: []dto.DiskInfo{
			{Temperature: 35.0, UsagePercent: 50.0, SMARTErrors: 0},
			{Temperature: 42.0, UsagePercent: 75.0, SMARTErrors: 0},
			{Temperature: 38.0, UsagePercent: 30.0, SMARTErrors: 2},
		},
		containers: []dto.ContainerInfo{
			{State: "running"},
			{State: "running"},
			{State: "exited"},
		},
		vms: []dto.VMInfo{
			{State: "running"},
			{State: "shut off"},
		},
		ups: &dto.UPSStatus{
			Status:        "OL",
			BatteryCharge: 100.0,
			LoadPercent:   25.0,
			RuntimeLeft:   7200,
		},
	}
}

func TestEngineBuildEnv(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	env := engine.buildEnv()

	// System
	if env.CPU != 75.5 {
		t.Errorf("expected CPU 75.5, got %f", env.CPU)
	}
	if env.RAMUsedPct != 60.0 {
		t.Errorf("expected RAMUsedPct 60, got %f", env.RAMUsedPct)
	}
	if env.CPUTemp != 55.0 {
		t.Errorf("expected CPUTemp 55, got %f", env.CPUTemp)
	}

	// Array
	if env.ArrayState != "Started" {
		t.Errorf("expected ArrayState Started, got %s", env.ArrayState)
	}
	if env.NumDisks != 6 {
		t.Errorf("expected 6 disks, got %d", env.NumDisks)
	}

	// Disk aggregates
	if env.MaxDiskTemp != 42.0 {
		t.Errorf("expected max disk temp 42, got %f", env.MaxDiskTemp)
	}
	if env.MaxDiskUsedPct != 75.0 {
		t.Errorf("expected max disk used pct 75, got %f", env.MaxDiskUsedPct)
	}
	if env.TotalDiskErrors != 2 {
		t.Errorf("expected 2 disk errors, got %d", env.TotalDiskErrors)
	}

	// Docker
	if env.ContainerCount != 3 {
		t.Errorf("expected 3 containers, got %d", env.ContainerCount)
	}
	if env.RunningContainers != 2 {
		t.Errorf("expected 2 running containers, got %d", env.RunningContainers)
	}
	if env.StoppedContainers != 1 {
		t.Errorf("expected 1 stopped container, got %d", env.StoppedContainers)
	}

	// VMs
	if env.VMCount != 2 {
		t.Errorf("expected 2 VMs, got %d", env.VMCount)
	}
	if env.RunningVMs != 1 {
		t.Errorf("expected 1 running VM, got %d", env.RunningVMs)
	}

	// UPS
	if env.UPSStatus != "OL" {
		t.Errorf("expected UPS status OL, got %s", env.UPSStatus)
	}
	if env.UPSBatteryCharge != 100.0 {
		t.Errorf("expected UPS battery 100, got %f", env.UPSBatteryCharge)
	}
	if env.UPSRuntimeLeft != 7200.0 {
		t.Errorf("expected UPS runtime left 7200, got %f", env.UPSRuntimeLeft)
	}
}

func TestEngineBuildEnvNilCaches(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := &mockDataProvider{} // All nil
	engine := NewEngine(store, provider)

	env := engine.buildEnv()

	if env.CPU != 0 {
		t.Errorf("expected CPU 0 with nil cache, got %f", env.CPU)
	}
	if env.ArrayState != "" {
		t.Errorf("expected empty ArrayState with nil cache, got %s", env.ArrayState)
	}
	if env.ContainerCount != 0 {
		t.Errorf("expected 0 containers with nil cache, got %d", env.ContainerCount)
	}
}

func TestEngineHistory(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	// Add events
	for range 5 {
		engine.addHistory(dto.AlertEvent{
			RuleID:   "test",
			RuleName: "Test",
			State:    "firing",
			FiredAt:  time.Now(),
		})
	}

	history := engine.GetHistory()
	if len(history) != 5 {
		t.Errorf("expected 5 history events, got %d", len(history))
	}
}

func TestEngineHistoryOverflow(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	// Overflow the ring buffer
	for range MaxHistoryEvents + 20 {
		engine.addHistory(dto.AlertEvent{
			RuleID: "overflow",
			State:  "firing",
		})
	}

	history := engine.GetHistory()
	if len(history) != MaxHistoryEvents {
		t.Errorf("expected max %d history events, got %d", MaxHistoryEvents, len(history))
	}
}

func TestEngineCooldown(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	rule := dto.AlertRule{
		ID:              "cooldown-test",
		CooldownMinutes: 5,
	}

	// No history — should not be cooling down
	if engine.isCoolingDown(rule) {
		t.Error("expected no cooldown with empty history")
	}

	// Add a recent firing event
	engine.addHistory(dto.AlertEvent{
		RuleID:  "cooldown-test",
		State:   "firing",
		FiredAt: time.Now(),
	})

	// Should now be cooling down
	if !engine.isCoolingDown(rule) {
		t.Error("expected cooldown after recent firing event")
	}
}

func TestEngineStartStop(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		engine.Start(ctx)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop within timeout")
	}
}

func TestEngineEvaluateIntegration(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	provider := newMockProvider()
	engine := NewEngine(store, provider)

	// Create a rule that should fire (CPU is 75.5, rule checks > 50)
	store.CreateRule(dto.AlertRule{
		ID:         "cpu-test",
		Name:       "CPU Over 50",
		Expression: "CPU > 50",
		Severity:   "warning",
		Channels:   []string{}, // No channels to avoid dispatch errors
		Enabled:    true,
	})

	engine.compileEnabledRules()
	engine.evaluate()

	// Check that the rule is firing
	statuses := engine.GetStatuses()
	found := false
	for _, s := range statuses {
		if s.RuleID == "cpu-test" && s.State == "firing" {
			found = true
		}
	}
	if !found {
		t.Error("expected cpu-test rule to be firing")
	}

	// Check history has an event
	history := engine.GetHistory()
	if len(history) < 1 {
		t.Error("expected at least 1 history event")
	}
}
