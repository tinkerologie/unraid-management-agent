package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTempDir(t *testing.T) {
	dir, cleanup := TempDir(t)
	defer cleanup()

	// Check directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("TempDir returned dir that doesn't exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("TempDir returned a file, not a directory")
	}

	// Check prefix
	if !strings.Contains(dir, "unraid-test-") {
		t.Errorf("TempDir name should contain 'unraid-test-', got: %s", dir)
	}
}

func TestWriteFile(t *testing.T) {
	dir, cleanup := TempDir(t)
	defer cleanup()

	content := "test content"
	path := WriteFile(t, dir, "test.txt", content)

	// Check file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if info.IsDir() {
		t.Error("WriteFile created a directory, not a file")
	}

	// Check content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(data) != content {
		t.Errorf("WriteFile content = %q, want %q", string(data), content)
	}
}

func TestWriteFileNested(t *testing.T) {
	dir, cleanup := TempDir(t)
	defer cleanup()

	content := "nested content"
	path := WriteFile(t, dir, "subdir/nested/test.txt", content)

	// Check nested directories were created
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("Nested directory not created: %v", err)
	}

	// Check content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read nested file: %v", err)
	}
	if string(data) != content {
		t.Errorf("Nested file content = %q, want %q", string(data), content)
	}
}

func TestReadFileContent(t *testing.T) {
	dir, cleanup := TempDir(t)
	defer cleanup()

	content := "read me"
	path := WriteFile(t, dir, "read.txt", content)

	result := ReadFileContent(t, path)
	if result != content {
		t.Errorf("ReadFileContent = %q, want %q", result, content)
	}
}

func TestSampleProcMeminfo(t *testing.T) {
	content := SampleProcMeminfo()
	if content == "" {
		t.Error("SampleProcMeminfo returned empty string")
	}
	if !strings.Contains(content, "MemTotal:") {
		t.Error("SampleProcMeminfo should contain 'MemTotal:'")
	}
	if !strings.Contains(content, "MemFree:") {
		t.Error("SampleProcMeminfo should contain 'MemFree:'")
	}
}

func TestSampleProcStat(t *testing.T) {
	content := SampleProcStat()
	if content == "" {
		t.Error("SampleProcStat returned empty string")
	}
	if !strings.HasPrefix(content, "cpu ") {
		t.Error("SampleProcStat should start with 'cpu '")
	}
	if !strings.Contains(content, "cpu0") {
		t.Error("SampleProcStat should contain 'cpu0'")
	}
}

func TestSampleProcUptime(t *testing.T) {
	content := SampleProcUptime()
	if content == "" {
		t.Error("SampleProcUptime returned empty string")
	}
	parts := strings.Fields(content)
	if len(parts) != 2 {
		t.Errorf("SampleProcUptime should have 2 values, got %d", len(parts))
	}
}

func TestSampleProcCPUInfo(t *testing.T) {
	content := SampleProcCPUInfo()
	if content == "" {
		t.Error("SampleProcCPUInfo returned empty string")
	}
	if !strings.Contains(content, "processor") {
		t.Error("SampleProcCPUInfo should contain 'processor'")
	}
	if !strings.Contains(content, "model name") {
		t.Error("SampleProcCPUInfo should contain 'model name'")
	}
}

func TestSampleDisksINI(t *testing.T) {
	content := SampleDisksINI()
	if content == "" {
		t.Error("SampleDisksINI returned empty string")
	}
	if !strings.Contains(content, "[disk") {
		t.Error("SampleDisksINI should contain disk sections")
	}
}

func TestSampleINIFile(t *testing.T) {
	content := SampleINIFile()
	if content == "" {
		t.Error("SampleINIFile returned empty string")
	}
	if !strings.Contains(content, "name=") {
		t.Error("SampleINIFile should contain 'name='")
	}
}

func TestSampleDockerPSOutput(t *testing.T) {
	content := SampleDockerPSOutput()
	if content == "" {
		t.Error("SampleDockerPSOutput returned empty string")
	}
	if !strings.Contains(content, "ID") {
		t.Error("SampleDockerPSOutput should contain 'ID'")
	}
}

func TestSampleVirshListOutput(t *testing.T) {
	content := SampleVirshListOutput()
	if content == "" {
		t.Error("SampleVirshListOutput returned empty string")
	}
	if !strings.Contains(content, "Id") {
		t.Error("SampleVirshListOutput should contain 'Id'")
	}
}

func TestSampleArrayINI(t *testing.T) {
	content := SampleArrayINI()
	if content == "" {
		t.Error("SampleArrayINI returned empty string")
	}
	if !strings.Contains(content, "mdState=") {
		t.Error("SampleArrayINI should contain 'mdState='")
	}
}

func TestSampleZFSPoolOutput(t *testing.T) {
	content := SampleZFSPoolOutput()
	if content == "" {
		t.Error("SampleZFSPoolOutput returned empty string")
	}
	if !strings.Contains(content, "NAME") {
		t.Error("SampleZFSPoolOutput should contain 'NAME'")
	}
}

func TestSampleZFSDatasetOutput(t *testing.T) {
	content := SampleZFSDatasetOutput()
	if content == "" {
		t.Error("SampleZFSDatasetOutput returned empty string")
	}
	if !strings.Contains(content, "MOUNTPOINT") {
		t.Error("SampleZFSDatasetOutput should contain 'MOUNTPOINT'")
	}
}

func TestSampleSharesINI(t *testing.T) {
	content := SampleSharesINI()
	if content == "" {
		t.Error("SampleSharesINI returned empty string")
	}
	if !strings.Contains(content, "[appdata]") {
		t.Error("SampleSharesINI should contain '[appdata]' section")
	}
}

func TestSampleUPSOutput(t *testing.T) {
	content := SampleUPSOutput()
	if content == "" {
		t.Error("SampleUPSOutput returned empty string")
	}
	if !strings.Contains(content, "UPS") || !strings.Contains(content, "STATUS") {
		t.Error("SampleUPSOutput should contain UPS information")
	}
}

func TestSampleSensorsOutput(t *testing.T) {
	content := SampleSensorsOutput()
	if content == "" {
		t.Error("SampleSensorsOutput returned empty string")
	}
	if !strings.Contains(content, "temp") {
		t.Error("SampleSensorsOutput should contain temperature information")
	}
}

func TestSampleEthtoolOutput(t *testing.T) {
	content := SampleEthtoolOutput()
	if content == "" {
		t.Error("SampleEthtoolOutput returned empty string")
	}
	if !strings.Contains(content, "Speed") {
		t.Error("SampleEthtoolOutput should contain 'Speed'")
	}
}

func TestSampleDmidecodeOutput(t *testing.T) {
	content := SampleDmidecodeOutput()
	if content == "" {
		t.Error("SampleDmidecodeOutput returned empty string")
	}
	if !strings.Contains(content, "BIOS") {
		t.Error("SampleDmidecodeOutput should contain 'BIOS'")
	}
}

func TestSampleNetworkINI(t *testing.T) {
	content := SampleNetworkINI()
	if content == "" {
		t.Error("SampleNetworkINI returned empty string")
	}
	if !strings.Contains(content, "eth0") {
		t.Error("SampleNetworkINI should contain 'eth0'")
	}
}

func TestSampleNvidiaSMIOutput(t *testing.T) {
	content := SampleNvidiaSMIOutput()
	if content == "" {
		t.Error("SampleNvidiaSMIOutput returned empty string")
	}
	if !strings.Contains(content, "NVIDIA") {
		t.Error("SampleNvidiaSMIOutput should contain 'NVIDIA'")
	}
}

func TestTempDirCleanup(t *testing.T) {
	var testDir string

	// Create temp dir
	dir, cleanup := TempDir(t)
	testDir = dir

	// Write a file to make sure cleanup removes it
	WriteFile(t, dir, "test.txt", "content")

	// Run cleanup
	cleanup()

	// Verify directory is removed
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("TempDir cleanup should have removed the directory")
	}
}
