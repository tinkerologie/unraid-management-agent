package alerting

import "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"

// AlertRuleTemplates returns curated, DISABLED-by-default rule templates that use
// the trend/predictive metrics. Users review and enable (and assign channels).
func AlertRuleTemplates() []dto.AlertRule {
	return []dto.AlertRule{
		{ID: "tmpl-array-fill", Name: "Array filling soon (< 72h)", Expression: "ArrayFillETAHours > 0 && ArrayFillETAHours < 72", Severity: "warning", Enabled: false, CooldownMinutes: 360},
		{ID: "tmpl-disk-temp-climb", Name: "Disk temperature climbing", Expression: "MaxDiskTempSlopePerMin > 1", Severity: "warning", Enabled: false, CooldownMinutes: 30},
		{ID: "tmpl-container-flapping", Name: "Container flapping", Expression: "MaxContainerRestartsPerHour >= 5", Severity: "warning", Enabled: false, CooldownMinutes: 30},
		{ID: "tmpl-smart-reallocated", Name: "Disk reallocated sectors detected", Expression: "MaxReallocatedSectors > 0", Severity: "critical", Enabled: false, CooldownMinutes: 1440},
		{ID: "tmpl-disk-errors-rising", Name: "Disk errors increasing", Expression: "DiskErrorsIncreasing", Severity: "critical", Enabled: false, CooldownMinutes: 720},
	}
}
