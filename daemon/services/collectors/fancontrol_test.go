package collectors

import "testing"

// The published fan status must always carry external-control state so the
// dashboard/MQTT feed can show when the agent is deferring to a third-party
// fan plugin (even when the agent's own control is disabled).
func TestCollectorBuildStatusIncludesExternalControl(t *testing.T) {
	c := &FanControlCollector{}
	st := c.buildStatus()
	if st.ExternalControl == nil {
		t.Fatal("expected buildStatus to populate ExternalControl")
	}
}
