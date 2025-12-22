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
func TestSystemInfoMemoryParsing(t *testing.T) {
	// Test parsing /proc/meminfo format
	meminfo := `MemTotal:       32653968 kB
MemFree:        15234568 kB
MemAvailable:   20123456 kB
Buffers:          512000 kB
Cached:          4876900 kB
`
	lines := strings.Split(meminfo, "\n")
	result := make(map[string]uint64)

	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])
		valueStr = strings.TrimSuffix(valueStr, " kB")
		value, _ := strconv.ParseUint(valueStr, 10, 64)
		result[key] = value
	}

	if result["MemTotal"] != 32653968 {
		t.Errorf("MemTotal = %d, want 32653968", result["MemTotal"])
	}
	if result["MemFree"] != 15234568 {
		t.Errorf("MemFree = %d, want 15234568", result["MemFree"])
	}
}

func TestSystemInfoCPUStatParsing(t *testing.T) {
	// Test parsing /proc/stat format
	stat := `cpu  10132153 290696 3084719 46828483 16683 0 25195 0 0 0
cpu0 1292830 36410 386526 5765120 3479 0 11149 0 0 0
cpu1 1291881 36252 385618 5764888 2500 0 3146 0 0 0
`
	lines := strings.Split(stat, "\n")

	var totalCPU []uint64
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				for i := 1; i < len(parts); i++ {
					val, _ := strconv.ParseUint(parts[i], 10, 64)
					totalCPU = append(totalCPU, val)
				}
			}
			break
		}
	}

	if len(totalCPU) < 4 {
		t.Errorf("Expected at least 4 CPU values, got %d", len(totalCPU))
	}
	if totalCPU[0] != 10132153 {
		t.Errorf("User time = %d, want 10132153", totalCPU[0])
	}
}

func TestSystemUptimeFormatting(t *testing.T) {
	tests := []struct {
		seconds  int64
		expected string
	}{
		{60, "0d 0h 1m"},
		{3600, "0d 1h 0m"},
		{86400, "1d 0h 0m"},
		{90061, "1d 1h 1m"},
		{172800, "2d 0h 0m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			days := tt.seconds / 86400
			hours := (tt.seconds % 86400) / 3600
			minutes := (tt.seconds % 3600) / 60
			formatted := formatUptime(days, hours, minutes)
			if formatted != tt.expected {
				t.Errorf("formatUptime(%d) = %q, want %q", tt.seconds, formatted, tt.expected)
			}
		})
	}
}

func formatUptime(days, hours, minutes int64) string {
	return formatInt64ForUptime(days) + "d " + formatInt64ForUptime(hours) + "h " + formatInt64ForUptime(minutes) + "m"
}

func formatInt64ForUptime(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestSystemLoadAverageParsing(t *testing.T) {
	// Test parsing /proc/loadavg format
	loadavg := "0.15 0.25 0.35 1/234 5678"
	parts := strings.Fields(loadavg)

	if len(parts) < 3 {
		t.Fatalf("Invalid loadavg format")
	}

	load1, _ := strconv.ParseFloat(parts[0], 64)
	load5, _ := strconv.ParseFloat(parts[1], 64)
	load15, _ := strconv.ParseFloat(parts[2], 64)

	if load1 != 0.15 {
		t.Errorf("Load1 = %f, want 0.15", load1)
	}
	if load5 != 0.25 {
		t.Errorf("Load5 = %f, want 0.25", load5)
	}
	if load15 != 0.35 {
		t.Errorf("Load15 = %f, want 0.35", load15)
	}
}
