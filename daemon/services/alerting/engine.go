package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// EvalInterval is the frequency at which alert rules are evaluated.
	EvalInterval = 15 * time.Second

	// MaxHistoryEvents is the maximum number of alert events kept in memory.
	MaxHistoryEvents = 100
)

// DataProvider defines the interface for reading cached collector data.
// This is implemented by the API server's cache.
type DataProvider interface {
	GetSystemCache() *dto.SystemInfo
	GetArrayCache() *dto.ArrayStatus
	GetDisksCache() []dto.DiskInfo
	GetDockerCache() []dto.ContainerInfo
	GetVMsCache() []dto.VMInfo
	GetUPSCache() *dto.UPSStatus
	GetGPUCache() []*dto.GPUMetrics
	GetZFSPoolsCache() []dto.ZFSPool
	GetNetworkCache() []dto.NetworkInfo
	GetNUTCache() *dto.NUTResponse
	GetNotificationsCache() *dto.NotificationList
}

// Engine orchestrates alert rule evaluation and notification dispatch.
// It periodically builds an AlertEnv from cached collector data, evaluates
// all enabled rules via the Evaluator, and dispatches notifications via the Dispatcher.
type Engine struct {
	store      *Store
	evaluator  *Evaluator
	dispatcher *Dispatcher
	provider   DataProvider

	mu      sync.RWMutex
	history []dto.AlertEvent
}

// NewEngine creates and initializes the alerting engine.
func NewEngine(store *Store, provider DataProvider) *Engine {
	return &Engine{
		store:      store,
		evaluator:  NewEvaluator(),
		dispatcher: NewDispatcher(),
		provider:   provider,
		history:    make([]dto.AlertEvent, 0, MaxHistoryEvents),
	}
}

// Start begins the alert evaluation loop. It blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	// Load rules from disk
	if err := e.store.Load(); err != nil {
		logger.Error("Alerting: Failed to load rules: %v", err)
	}

	// Compile all loaded rules
	e.compileEnabledRules()

	logger.Info("Alerting: Engine started (eval interval: %s)", EvalInterval)

	ticker := time.NewTicker(EvalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Alerting: Engine stopped")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Alerting: PANIC during evaluation: %v", r)
					}
				}()
				e.evaluate()
			}()
		}
	}
}

// evaluate runs one evaluation cycle for all enabled rules.
func (e *Engine) evaluate() {
	env := e.buildEnv()
	rules := e.store.GetEnabledRules()

	results := e.evaluator.Evaluate(env, rules)
	for _, result := range results {
		if !result.Transitioned {
			continue
		}

		event := e.resultToEvent(result)

		// Check cooldown before dispatching firing alerts
		if event.State == "firing" && e.isCoolingDown(result.Rule) {
			logger.Debug("Alerting: Rule %s is in cooldown, skipping dispatch", result.Rule.ID)
			continue
		}

		e.addHistory(event)
		e.dispatcher.Dispatch(result.Rule, event)
	}
}

// resultToEvent converts an EvaluateResult into an AlertEvent for history/dispatch.
func (e *Engine) resultToEvent(result EvaluateResult) dto.AlertEvent {
	now := time.Now()
	event := dto.AlertEvent{
		RuleID:   result.Rule.ID,
		RuleName: result.Rule.Name,
		Severity: result.Rule.Severity,
		FiredAt:  now,
	}

	if result.NewState == "firing" {
		event.State = "firing"
		event.Message = fmt.Sprintf("Alert rule '%s' triggered (expression: %s)", result.Rule.Name, result.Rule.Expression)
	} else if result.PrevState == "firing" && result.NewState == "ok" {
		event.State = "resolved"
		event.ResolvedAt = now
		event.Message = fmt.Sprintf("Alert rule '%s' resolved", result.Rule.Name)
	}

	return event
}

// isCoolingDown checks if the rule is within its cooldown period since the last fire.
func (e *Engine) isCoolingDown(rule dto.AlertRule) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
	if cooldown == 0 {
		cooldown = 5 * time.Minute
	}

	// Search backward through history for the most recent firing event for this rule
	for i := len(e.history) - 1; i >= 0; i-- {
		ev := e.history[i]
		if ev.RuleID == rule.ID && ev.State == "firing" {
			return time.Since(ev.FiredAt) < cooldown
		}
	}
	return false
}

// addHistory appends an event to the history ring buffer.
func (e *Engine) addHistory(event dto.AlertEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.history) >= MaxHistoryEvents {
		// Drop oldest
		e.history = e.history[1:]
	}
	e.history = append(e.history, event)
}

// GetHistory returns a copy of recent alert events.
func (e *Engine) GetHistory() []dto.AlertEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	events := make([]dto.AlertEvent, len(e.history))
	copy(events, e.history)
	return events
}

// GetStatuses returns the current status of all enabled rules.
func (e *Engine) GetStatuses() []dto.AlertStatus {
	rules := e.store.GetEnabledRules()
	return e.evaluator.GetStatuses(rules)
}

// GetFiringAlerts returns only rules currently in the "firing" state.
func (e *Engine) GetFiringAlerts() []dto.AlertStatus {
	rules := e.store.GetEnabledRules()
	return e.evaluator.GetFiringAlerts(rules)
}

// RecompileRules recompiles all enabled rules. Call after rule CRUD operations.
func (e *Engine) RecompileRules() {
	e.compileEnabledRules()
}

// compileEnabledRules compiles all enabled rules from the store.
func (e *Engine) compileEnabledRules() {
	rules := e.store.GetEnabledRules()
	errs := e.evaluator.CompileRules(rules)
	logger.Info("Alerting: Compiled %d enabled rules (%d errors)", len(rules), len(errs))
}

// buildEnv constructs an AlertEnv from the current cached collector data.
func (e *Engine) buildEnv() dto.AlertEnv {
	env := dto.AlertEnv{}

	// System
	if sys := e.provider.GetSystemCache(); sys != nil {
		env.CPU = sys.CPUUsage
		env.RAMUsedPct = sys.RAMUsage
		env.RAMTotalBytes = sys.RAMTotal
		env.RAMUsedBytes = sys.RAMUsed
		env.RAMFreeBytes = sys.RAMFree
		env.CPUTemp = sys.CPUTemp
		env.MotherboardTemp = sys.MotherboardTemp
		env.Uptime = sys.Uptime
	}

	// Array
	if arr := e.provider.GetArrayCache(); arr != nil {
		env.ArrayState = arr.State
		env.ArrayUsedPct = arr.UsedPercent
		env.ArrayFreeBytes = arr.FreeBytes
		env.ArrayTotalBytes = arr.TotalBytes
		env.ParityValid = arr.ParityValid
		env.ParityCheckStatus = arr.ParityCheckStatus
		env.ParityCheckProgress = arr.ParityCheckProgress
		env.NumDisks = arr.NumDisks
		env.NumParityDisks = arr.NumParityDisks
	}

	// Disks — aggregate max temp, max usage, total errors
	if disks := e.provider.GetDisksCache(); disks != nil {
		for _, d := range disks {
			if d.Temperature > env.MaxDiskTemp {
				env.MaxDiskTemp = d.Temperature
			}
			if d.UsagePercent > env.MaxDiskUsedPct {
				env.MaxDiskUsedPct = d.UsagePercent
			}
			env.TotalDiskErrors += d.SMARTErrors
		}
	}

	// Docker
	if containers := e.provider.GetDockerCache(); containers != nil {
		env.ContainerCount = len(containers)
		for _, c := range containers {
			if c.State == "running" {
				env.RunningContainers++
			} else {
				env.StoppedContainers++
			}
		}
	}

	// VMs
	if vms := e.provider.GetVMsCache(); vms != nil {
		env.VMCount = len(vms)
		for _, v := range vms {
			if v.State == "running" {
				env.RunningVMs++
			}
		}
	}

	// UPS
	if ups := e.provider.GetUPSCache(); ups != nil {
		env.UPSStatus = ups.Status
		env.UPSBatteryCharge = ups.BatteryCharge
		env.UPSLoadPercent = ups.LoadPercent
		env.UPSRuntimeLeft = float64(ups.RuntimeLeft)
	}

	// GPU
	if gpus := e.provider.GetGPUCache(); gpus != nil {
		for _, g := range gpus {
			if g == nil {
				continue
			}
			env.GPUCount++
			if g.Temperature > env.MaxGPUTemp {
				env.MaxGPUTemp = g.Temperature
			}
			if g.UtilizationGPU > env.MaxGPUUtil {
				env.MaxGPUUtil = g.UtilizationGPU
			}
			env.TotalGPUPower += g.PowerDraw
		}
	}

	// ZFS pools
	if pools := e.provider.GetZFSPoolsCache(); pools != nil {
		env.ZFSPoolCount = len(pools)
		for _, p := range pools {
			if p.CapacityPct > env.MaxZFSPoolUsedPct {
				env.MaxZFSPoolUsedPct = p.CapacityPct
			}
			switch p.Health {
			case "DEGRADED":
				env.ZFSDegradedPools++
			case "FAULTED":
				env.ZFSFaultedPools++
			}
		}
	}

	// Network
	if nets := e.provider.GetNetworkCache(); nets != nil {
		env.NetworkIFCount = len(nets)
		for _, n := range nets {
			env.NetworkErrors += n.ErrorsReceived + n.ErrorsSent
		}
	}

	// NUT
	if nut := e.provider.GetNUTCache(); nut != nil && nut.Status != nil {
		env.NUTStatus = nut.Status.Status
		env.NUTBatteryCharge = nut.Status.BatteryCharge
		env.NUTBatteryRuntime = nut.Status.BatteryRuntime
		env.NUTLoadPercent = nut.Status.LoadPercent
	}

	// Notifications
	// NotificationOverview and NotificationCounts are value types (not pointers),
	// so the only nil-deref risk is notifs itself, which is guarded here.
	if notifs := e.provider.GetNotificationsCache(); notifs != nil {
		env.UnreadNotifications = notifs.Overview.Unread.Total
		env.WarningNotifications = notifs.Overview.Unread.Warning
		env.AlertNotifications = notifs.Overview.Unread.Alert
	}

	return env
}
