package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestMCPNullableDiskMeasurements(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.disks = []dto.DiskInfo{
		{ID: "standby", Name: "standby", Status: "DISK_OK", SMARTStatus: "UNKNOWN", SpinState: "standby"},
		{ID: "hot", Name: "hot", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(60.0)},
		{ID: "failed", Name: "failed", Status: "DISK_OK", SMARTStatus: "FAILED"},
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()
	_, text := callToolJSON(t, cs, "get_diagnostic_summary", nil)
	var summary struct {
		DiskIssues []struct {
			ID          string   `json:"id"`
			Temperature *float64 `json:"temperature"`
		} `json:"disk_issues"`
	}
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.DiskIssues) != 2 || summary.DiskIssues[0].ID != "hot" || summary.DiskIssues[1].Temperature != nil {
		t.Errorf("diagnostic issues lost unknown readings: %s", text)
	}
	if !strings.Contains(text, `"temperature": null`) && !strings.Contains(text, `"temperature":null`) {
		t.Error("missing disk temperature should stay null")
	}
	_, text = callToolJSON(t, cs, "find_root_cause", map[string]any{})
	if !strings.Contains(text, "hot") || strings.Contains(text, "standby") || strings.Contains(text, "%!") {
		t.Errorf("incorrect temperature root cause: %s", text)
	}
}
