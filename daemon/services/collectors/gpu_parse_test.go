package collectors

import (
	"strings"
	"testing"
)

func TestParseNvidiaGPUCSV(t *testing.T) {
	t.Run("single GPU with full fields", func(t *testing.T) {
		// index,pci.bus_id,uuid,name,temp,util,mem.used,mem.total,power,fan
		out := "0, 00000000:01:00.0, GPU-abc, NVIDIA GeForce RTX 3080, 55, 30, 2048, 10240, 220.5, 40\n"
		gpus, err := parseNvidiaGPUCSV(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 1 {
			t.Fatalf("expected 1 GPU, got %d", len(gpus))
		}
		g := gpus[0]
		if g.Index != 0 {
			t.Errorf("Index = %d, want 0", g.Index)
		}
		if g.PCIID != "00000000:01:00.0" {
			t.Errorf("PCIID = %q", g.PCIID)
		}
		if g.UUID != "GPU-abc" {
			t.Errorf("UUID = %q", g.UUID)
		}
		if g.Name != "NVIDIA GeForce RTX 3080" {
			t.Errorf("Name = %q", g.Name)
		}
		if g.Temperature != 55 {
			t.Errorf("Temperature = %v, want 55", g.Temperature)
		}
		if g.UtilizationGPU != 30 {
			t.Errorf("UtilizationGPU = %v, want 30", g.UtilizationGPU)
		}
		// 2048 MiB -> bytes
		if g.MemoryUsed != 2048*1024*1024 {
			t.Errorf("MemoryUsed = %d, want %d", g.MemoryUsed, 2048*1024*1024)
		}
		if g.MemoryTotal != 10240*1024*1024 {
			t.Errorf("MemoryTotal = %d, want %d", g.MemoryTotal, 10240*1024*1024)
		}
		// 2048/10240 * 100 = 20
		if g.UtilizationMemory != 20 {
			t.Errorf("UtilizationMemory = %v, want 20", g.UtilizationMemory)
		}
		if g.PowerDraw != 220.5 {
			t.Errorf("PowerDraw = %v, want 220.5", g.PowerDraw)
		}
		if g.FanSpeed != 40 {
			t.Errorf("FanSpeed = %v, want 40", g.FanSpeed)
		}
		if g.Vendor != "nvidia" || !g.Available {
			t.Errorf("Vendor/Available wrong: %q/%v", g.Vendor, g.Available)
		}
	})

	t.Run("multiple GPUs", func(t *testing.T) {
		out := "0, 00000000:01:00.0, GPU-a, Card A, 50, 10, 1024, 8192, 100, 30\n" +
			"1, 00000000:02:00.0, GPU-b, Card B, 60, 20, 2048, 8192, 150, 50\n"
		gpus, err := parseNvidiaGPUCSV(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 2 {
			t.Fatalf("expected 2 GPUs, got %d", len(gpus))
		}
		if gpus[0].Name != "Card A" || gpus[1].Name != "Card B" {
			t.Errorf("names wrong: %q, %q", gpus[0].Name, gpus[1].Name)
		}
		if gpus[1].Index != 1 {
			t.Errorf("second GPU index = %d, want 1", gpus[1].Index)
		}
	})

	t.Run("short record is skipped", func(t *testing.T) {
		out := "0, 00000000:01:00.0, GPU-a, Card A, 50\n"
		gpus, err := parseNvidiaGPUCSV(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 0 {
			t.Errorf("expected 0 GPUs for short record, got %d", len(gpus))
		}
	})

	t.Run("non-numeric and N/A fields leave zero values", func(t *testing.T) {
		out := "0, pci, uuid, Card, [N/A], [N/A], notanumber, 0, [N/A], [N/A]\n"
		gpus, err := parseNvidiaGPUCSV(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 1 {
			t.Fatalf("expected 1 GPU, got %d", len(gpus))
		}
		g := gpus[0]
		if g.Temperature != 0 || g.UtilizationGPU != 0 || g.MemoryUsed != 0 || g.PowerDraw != 0 || g.FanSpeed != 0 {
			t.Errorf("expected zeroed numeric fields, got %+v", g)
		}
		// zero total memory must not divide by zero and leaves UtilizationMemory at 0
		if g.MemoryTotal != 0 || g.UtilizationMemory != 0 {
			t.Errorf("expected zero total/util memory, got total=%d util=%v", g.MemoryTotal, g.UtilizationMemory)
		}
	})

	t.Run("empty output yields no GPUs", func(t *testing.T) {
		gpus, err := parseNvidiaGPUCSV("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 0 {
			t.Errorf("expected 0 GPUs, got %d", len(gpus))
		}
	})

	t.Run("short record mixed with valid record keeps the valid one", func(t *testing.T) {
		// A short row must not abort the whole parse (FieldsPerRecord = -1);
		// the 10-field record is retained and the short one skipped.
		out := "0, 00000000:01:00.0, GPU-a, Card A, 50, 10, 1024, 8192, 100, 30\n" +
			"1, short, record\n"
		gpus, err := parseNvidiaGPUCSV(out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 1 {
			t.Fatalf("expected 1 GPU (short record skipped), got %d", len(gpus))
		}
		if gpus[0].Name != "Card A" {
			t.Errorf("retained GPU name = %q, want Card A", gpus[0].Name)
		}
	})
}

func TestCollectNvidiaGPU(t *testing.T) {
	metricsCSV := "0, 00000000:01:00.0, GPU-a, Card A, 50, 10, 1024, 8192, 100, 30\n" +
		"1, 00000000:02:00.0, GPU-b, Card B, 60, 20, 2048, 8192, 150, 50\n"

	c := &GPUCollector{
		nvidiaExec: func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if strings.Contains(a, "driver_version") {
					return "535.104.05\n", nil
				}
			}
			return metricsCSV, nil
		},
	}

	gpus, err := c.collectNvidiaGPU()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(gpus))
	}
	// Driver version is queried once and applied to every GPU.
	for i, g := range gpus {
		if g.DriverVersion != "535.104.05" {
			t.Errorf("gpu[%d] DriverVersion = %q, want 535.104.05", i, g.DriverVersion)
		}
	}
}

func TestCollectNvidiaGPUQueryError(t *testing.T) {
	c := &GPUCollector{
		nvidiaExec: func(_ string, _ ...string) (string, error) {
			return "", errFakeNvidia
		},
	}
	if _, err := c.collectNvidiaGPU(); err == nil {
		t.Fatal("expected error when nvidia-smi query fails, got nil")
	}
}

var errFakeNvidia = errFake("nvidia-smi not available")

type errFake string

func (e errFake) Error() string { return string(e) }
