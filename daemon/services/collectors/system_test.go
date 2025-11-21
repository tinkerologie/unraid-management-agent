package collectors

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewSystemCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{
		Hub: hub,
		Config: domain.Config{
			Version: "1.0.0",
		},
	}

	collector := NewSystemCollector(ctx)

	if collector == nil {
		t.Fatal("NewSystemCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("SystemCollector context not set correctly")
	}
}

func TestSystemCollectorParseSensorsOutput(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	t.Run("parse coretemp output", func(t *testing.T) {
		output := `coretemp-isa-0000
Adapter: ISA adapter
Core 0:
  temp2_input: 45.000
  temp2_max: 100.000
  temp2_crit: 100.000
Core 1:
  temp3_input: 46.000
  temp3_max: 100.000
MB Temp:
  temp1_input: 38.000
`
		temps := collector.parseSensorsOutput(output)

		if len(temps) == 0 {
			t.Error("Expected temperatures to be parsed")
		}

		found := false
		for _, v := range temps {
			if v > 0 {
				found = true
				break
			}
		}
		if !found {
			t.Error("No valid temperatures found")
		}
	})

	t.Run("parse empty output", func(t *testing.T) {
		temps := collector.parseSensorsOutput("")

		if len(temps) != 0 {
			t.Errorf("Expected 0 temperatures, got %d", len(temps))
		}
	})
}

func TestSystemCollectorParseFanSpeeds(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	t.Run("parse fan output", func(t *testing.T) {
		output := `nct6792-isa-0a20
Adapter: ISA adapter
fan1_input: 1200
fan2_input: 800
`
		fans := collector.parseFanSpeeds(output)

		if len(fans) == 0 {
			t.Error("Expected fan speeds to be parsed")
		}
	})

	t.Run("parse empty output", func(t *testing.T) {
		fans := collector.parseFanSpeeds("")

		if len(fans) != 0 {
			t.Errorf("Expected 0 fan speeds, got %d", len(fans))
		}
	})
}

func TestSystemCollectorUptimeParsing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "system-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	uptimePath := filepath.Join(tmpDir, "uptime")
	if err := os.WriteFile(uptimePath, []byte("12345.67 98765.43"), 0644); err != nil {
		t.Fatalf("Failed to write uptime file: %v", err)
	}

	content, err := os.ReadFile(uptimePath)
	if err != nil {
		t.Fatalf("Failed to read uptime file: %v", err)
	}

	parts := strings.Split(strings.TrimSpace(string(content)), " ")
	if len(parts) < 1 {
		t.Fatal("Invalid uptime format")
	}

	uptime, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		t.Fatalf("Failed to parse uptime: %v", err)
	}

	if int64(uptime) != 12345 {
		t.Errorf("Uptime = %d, want 12345", int64(uptime))
	}
}

func TestSystemCollectorGetCPUSpecs(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	model, cores, threads, mhz := collector.getCPUSpecs()
	_ = model
	_ = cores
	_ = threads
	_ = mhz
}

func TestSystemCollectorIsHVMEnabled(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	result := collector.isHVMEnabled()
	_ = result
}

func TestSystemCollectorIsIOMMUEnabled(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	result := collector.isIOMMUEnabled()
	_ = result
}

func TestSystemCollectorGetOpenSSLVersion(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	version := collector.getOpenSSLVersion()
	_ = version
}

func TestSystemCollectorGetKernelVersion(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewSystemCollector(ctx)

	version := collector.getKernelVersion()
	_ = version
}
