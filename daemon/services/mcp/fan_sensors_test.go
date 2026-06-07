package mcp

import (
	"context"
	"testing"
)

func TestFanSensorsToolRegistered(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "get_fan_sensors" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected get_fan_sensors tool to be registered")
	}
}
