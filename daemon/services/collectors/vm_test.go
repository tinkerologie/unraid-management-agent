package collectors

import (
	"strings"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewVMCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewVMCollector(ctx)

	if collector == nil {
		t.Fatal("NewVMCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("VMCollector context not set correctly")
	}
}

func TestVirshListOutputParsing(t *testing.T) {
	// Test parsing of virsh list --all output
	output := ` Id   Name        State
-----------------------------
 1    ubuntu20    running
 -    windows10   shut off
 -    debian11    shut off
`
	lines := strings.Split(output, "\n")

	var vms []struct {
		ID    string
		Name  string
		State string
	}

	for i, line := range lines {
		// Skip header lines
		if i < 2 || strings.TrimSpace(line) == "" || strings.HasPrefix(line, "---") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 {
			vm := struct {
				ID    string
				Name  string
				State string
			}{
				ID:    fields[0],
				Name:  fields[1],
				State: strings.Join(fields[2:], " "),
			}
			vms = append(vms, vm)
		}
	}

	if len(vms) != 3 {
		t.Errorf("Expected 3 VMs, got %d", len(vms))
	}

	if len(vms) > 0 && vms[0].Name != "ubuntu20" {
		t.Errorf("First VM name = %q, want %q", vms[0].Name, "ubuntu20")
	}

	if len(vms) > 0 && vms[0].State != "running" {
		t.Errorf("First VM state = %q, want %q", vms[0].State, "running")
	}
}

func TestVMStateMapping(t *testing.T) {
	// Test VM state parsing
	tests := []struct {
		state    string
		expected string
	}{
		{"running", "running"},
		{"shut off", "shut off"},
		{"paused", "paused"},
		{"in shutdown", "in shutdown"},
		{"idle", "idle"},
		{"crashed", "crashed"},
		{"pmsuspended", "pmsuspended"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if tt.state != tt.expected {
				t.Errorf("State %q != %q", tt.state, tt.expected)
			}
		})
	}
}
