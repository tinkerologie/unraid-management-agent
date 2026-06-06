package alerting

import "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"

// BuiltinRules returns rules that ship ENABLED by default. They are seeded once
// (idempotent by ID) on engine start, so a user may later disable or delete them
// and the change persists across restarts.
func BuiltinRules() []dto.AlertRule {
	return []dto.AlertRule{
		{
			ID:              "subsystem-degraded",
			Name:            "Agent data source degraded",
			Expression:      "DegradedSubsystemCount > 0",
			Severity:        "warning",
			Enabled:         true,
			Channels:        []string{"unraid"},
			CooldownMinutes: 60,
		},
	}
}

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

// RuleFromTemplate returns an enabled AlertRule built from the template with the
// given id. channels are the notification targets; if empty it defaults to
// ["unraid"] (an Unraid system notification) so the rule notifies somewhere.
// The returned rule's ID equals the template id, making enable idempotent
// (re-enabling updates the same rule rather than creating duplicates).
// ok is false if no template has that id.
func RuleFromTemplate(id string, channels []string) (rule dto.AlertRule, ok bool) {
	for _, t := range AlertRuleTemplates() {
		if t.ID == id {
			r := t
			r.Enabled = true
			if len(channels) == 0 {
				r.Channels = []string{"unraid"}
			} else {
				r.Channels = channels
			}
			return r, true
		}
	}
	return dto.AlertRule{}, false
}
