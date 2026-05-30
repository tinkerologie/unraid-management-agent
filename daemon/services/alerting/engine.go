package alerting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	GetPluginUpdatesCache() *dto.PluginList
}

// Engine orchestrates alert rule evaluation and notification dispatch.
// It periodically builds an AlertEnv from cached collector data, evaluates
// all enabled rules via the Evaluator, and dispatches notifications via the Dispatcher.
type Engine struct {
	store      *Store
	evaluator  *Evaluator
	dispatcher *Dispatcher
	provider   DataProvider
	history    *MetricsHistory

	mu           sync.RWMutex
	alertHistory []dto.AlertEvent
}

// NewEngine creates and initializes the alerting engine.
func NewEngine(store *Store, provider DataProvider) *Engine {
	return &Engine{
		store:        store,
		evaluator:    NewEvaluator(),
		dispatcher:   NewDispatcher(),
		provider:     provider,
		history:      NewMetricsHistory(240, time.Hour),
		alertHistory: make([]dto.AlertEvent, 0, MaxHistoryEvents),
	}
}

// Start begins the alert evaluation loop. It blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	// Top-level recovery for startup preamble (store loading, ticker creation)
	defer func() {
		if r := recover(); r != nil {
			logger.LogPanicWithStack("Alerting engine (top-level)", r)
			panic(r)
		}
	}()

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
						logger.LogPanicWithStack("Alerting engine", r)
					}
				}()
				e.evaluate()
			}()
		}
	}
}

// evaluate runs one evaluation cycle for all enabled rules.
func (e *Engine) evaluate() {
	now := time.Now()
	e.sampleHistory(now)
	env := e.buildEnv()
	e.overlayTrends(&env)
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
	for i := len(e.alertHistory) - 1; i >= 0; i-- {
		ev := e.alertHistory[i]
		if ev.RuleID == rule.ID && ev.State == "firing" {
			return time.Since(ev.FiredAt) < cooldown
		}
	}
	return false
}

// addHistory appends an event to the alertHistory ring buffer.
func (e *Engine) addHistory(event dto.AlertEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.alertHistory) >= MaxHistoryEvents {
		// Drop oldest
		e.alertHistory = e.alertHistory[1:]
	}
	e.alertHistory = append(e.alertHistory, event)
}

// GetHistory returns a copy of recent alert events.
func (e *Engine) GetHistory() []dto.AlertEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	events := make([]dto.AlertEvent, len(e.alertHistory))
	copy(events, e.alertHistory)
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

// smartRawInt parses the integer portion of a SMART RawValue string (e.g. "5" or "5 (raw)").
func smartRawInt(raw string) int {
	s := strings.Fields(raw)
	if len(s) == 0 {
		return 0
	}
	v, err := strconv.Atoi(s[0])
	if err != nil {
		return 0
	}
	return v
}

// sampleHistory records one tick of metrics into the trend history.
func (e *Engine) sampleHistory(now time.Time) {
	if sys := e.provider.GetSystemCache(); sys != nil {
		e.history.Record("cpu_temp", "", sys.CPUTemp, now)
	}
	if arr := e.provider.GetArrayCache(); arr != nil {
		e.history.Record("array_used_pct", "", arr.UsedPercent, now)
	}

	diskIDs := map[string]bool{}
	if disks := e.provider.GetDisksCache(); disks != nil {
		for _, d := range disks {
			if d.ID == "" {
				continue
			}
			id := d.ID
			diskIDs[id] = true
			e.history.Record("disk_temp", id, d.Temperature, now)
			e.history.Record("disk_used_pct", id, d.UsagePercent, now)
			e.history.Record("disk_errors", id, float64(d.SMARTErrors), now)

			// Extract reallocated (ID 5) and pending (ID 197) from SMARTAttributes
			var reallocated, pending float64
			for _, attr := range d.SMARTAttributes {
				switch attr.ID {
				case 5:
					reallocated = float64(smartRawInt(attr.RawValue))
				case 197:
					pending = float64(smartRawInt(attr.RawValue))
				}
			}
			e.history.Record("reallocated", id, reallocated, now)
			e.history.Record("pending", id, pending, now)
		}
	}
	for _, metric := range []string{"disk_temp", "disk_used_pct", "disk_errors", "reallocated", "pending"} {
		e.history.pruneEntities(metric, diskIDs)
	}

	containerIDs := map[string]bool{}
	if containers := e.provider.GetDockerCache(); containers != nil {
		for _, c := range containers {
			if c.ID == "" {
				continue
			}
			containerIDs[c.ID] = true
			e.history.Record("restart_count", c.ID, float64(c.RestartCount), now)
		}
	}
	e.history.pruneEntities("restart_count", containerIDs)
}

// overlayTrends computes trend/predictive fields from the MetricsHistory and
// overlays them onto env. Called after buildEnv() each evaluation tick.
func (e *Engine) overlayTrends(env *dto.AlertEnv) {
	e.history.mu.RLock()
	defer e.history.mu.RUnlock()

	// Global series
	if s := e.history.globalSeries["cpu_temp"]; len(s) >= 2 {
		env.CPUTempSlopePerMin = e.history.slope(s) * 60
	}
	if s := e.history.globalSeries["array_used_pct"]; len(s) >= 2 {
		eta := e.history.etaToThreshold(s, 100)
		if eta > 0 {
			env.ArrayFillETAHours = eta
		}
	}

	// Per-disk series
	var maxDiskTempSlope float64
	var minDiskFillETA float64 // soonest (smallest positive) = worst-case
	for _, s := range e.history.entitySeries["disk_temp"] {
		if len(s) < 2 {
			continue
		}
		sl := e.history.slope(s) * 60
		if sl > maxDiskTempSlope {
			maxDiskTempSlope = sl
		}
	}
	env.MaxDiskTempSlopePerMin = maxDiskTempSlope

	for _, s := range e.history.entitySeries["disk_used_pct"] {
		if len(s) < 2 {
			continue
		}
		eta := e.history.etaToThreshold(s, 100)
		if eta > 0 && (minDiskFillETA == 0 || eta < minDiskFillETA) {
			minDiskFillETA = eta
		}
	}
	env.MaxDiskFillETAHours = minDiskFillETA

	// Max reallocated and pending sectors (last sample value)
	for _, s := range e.history.entitySeries["reallocated"] {
		if len(s) == 0 {
			continue
		}
		v := int(s[len(s)-1].v)
		if v > env.MaxReallocatedSectors {
			env.MaxReallocatedSectors = v
		}
	}
	for _, s := range e.history.entitySeries["pending"] {
		if len(s) == 0 {
			continue
		}
		v := int(s[len(s)-1].v)
		if v > env.MaxPendingSectors {
			env.MaxPendingSectors = v
		}
	}

	// DiskErrorsIncreasing: any per-disk reallocated/pending/disk_errors slope > 0
	for _, metric := range []string{"reallocated", "pending", "disk_errors"} {
		for _, s := range e.history.entitySeries[metric] {
			if len(s) >= 2 && e.history.slope(s) > 0 {
				env.DiskErrorsIncreasing = true
				break
			}
		}
		if env.DiskErrorsIncreasing {
			break
		}
	}

	// Per-container restart slope
	var maxRestartSlope float64
	for _, s := range e.history.entitySeries["restart_count"] {
		if len(s) < 2 {
			continue
		}
		sl := e.history.slope(s) * 3600
		if sl > maxRestartSlope {
			maxRestartSlope = sl
		}
	}
	env.MaxContainerRestartsPerHour = maxRestartSlope
}

// QueryHistory returns the samples and summary statistics for a metric series.
// Pass entity="" for global (non-entity) metrics such as cpu_temp or array_used_pct.
func (e *Engine) QueryHistory(metric, entity string) dto.MetricHistoryResult {
	s := e.history.SeriesSnapshot(metric, entity)
	res := dto.MetricHistoryResult{Metric: metric, Entity: entity, Count: len(s)}
	if len(s) == 0 {
		return res
	}
	res.Slope = e.history.slope(s)
	res.Min = s[0].v
	res.Max = s[0].v
	var sum float64
	for _, p := range s {
		res.Samples = append(res.Samples, dto.MetricSample{TimeUnix: p.t.Unix(), Value: p.v})
		if p.v < res.Min {
			res.Min = p.v
		}
		if p.v > res.Max {
			res.Max = p.v
		}
		sum += p.v
	}
	res.Avg = sum / float64(len(s))
	res.Last = s[len(s)-1].v
	return res
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
		env.SwapUsedPct = sys.SwapUsage
		env.SwapTotalBytes = sys.SwapTotal
		env.SwapUsedBytes = sys.SwapUsed
		env.SwapFreeBytes = sys.SwapFree
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
			if c.UpdateAvailable != nil && *c.UpdateAvailable {
				env.ContainerUpdatesAvailable++
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
		env.BootPoolHealthy = true // default: no boot pool present
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
			env.ZFSCorruptedFiles += len(p.CorruptedFiles)
			if p.IsBootPool {
				env.BootPoolHealth = p.Health
				env.BootPoolHealthy = p.Health == "ONLINE"
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

	// Plugin updates
	if plugins := e.provider.GetPluginUpdatesCache(); plugins != nil {
		for _, p := range plugins.Plugins {
			if p.UpdateAvailable {
				env.PluginUpdatesAvailable++
			}
		}
	}

	return env
}
