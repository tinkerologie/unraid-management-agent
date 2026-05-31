package agent

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestLoadConfigDefaultsWhenMissing(t *testing.T) {
	cfg := LoadConfig(t.TempDir())
	if cfg.Enabled {
		t.Fatal("expected disabled default config")
	}
}

func TestBuildServiceNilWhenDisabled(t *testing.T) {
	cfg := dto.DefaultAgentConfig() // Enabled=false
	svc, err := BuildService(cfg, t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if svc != nil {
		t.Fatal("expected nil service when disabled")
	}
}
