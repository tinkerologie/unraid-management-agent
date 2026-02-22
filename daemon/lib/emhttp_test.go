package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMdcmdWrite_ProcNotAvailable(t *testing.T) {
	// Point to a non-existent path
	original := ProcMdcmd
	ProcMdcmd = "/tmp/nonexistent_proc_mdcmd"
	defer func() { ProcMdcmd = original }()

	err := MdcmdWrite("start")
	if err == nil {
		t.Fatal("expected error when /proc/mdcmd is not available")
	}
	if !contains(err.Error(), "failed to open") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMdcmdWrite_Success(t *testing.T) {
	// Create temp file to simulate /proc/mdcmd
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mdcmd")
	if err := os.WriteFile(tmpFile, nil, 0666); err != nil {
		t.Fatal(err)
	}

	original := ProcMdcmd
	ProcMdcmd = tmpFile
	defer func() { ProcMdcmd = original }()

	err := MdcmdWrite("start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the command was written
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "start\n" {
		t.Errorf("expected 'start\\n', got %q", string(data))
	}
}

func TestMdcmdWrite_MultipleArgs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mdcmd")
	if err := os.WriteFile(tmpFile, nil, 0666); err != nil {
		t.Fatal(err)
	}

	original := ProcMdcmd
	ProcMdcmd = tmpFile
	defer func() { ProcMdcmd = original }()

	err := MdcmdWrite("check", "NOCORRECT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "check NOCORRECT\n" {
		t.Errorf("expected 'check NOCORRECT\\n', got %q", string(data))
	}
}

func TestMdcmdWrite_SpindownDisk(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mdcmd")
	if err := os.WriteFile(tmpFile, nil, 0666); err != nil {
		t.Fatal(err)
	}

	original := ProcMdcmd
	ProcMdcmd = tmpFile
	defer func() { ProcMdcmd = original }()

	err := MdcmdWrite("spindown", "disk1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "spindown disk1\n" {
		t.Errorf("expected 'spindown disk1\\n', got %q", string(data))
	}
}

func TestReadCSRFToken_FileNotFound(t *testing.T) {
	original := VarIniPath
	VarIniPath = "/tmp/nonexistent_var_ini"
	defer func() { VarIniPath = original }()

	_, err := readCSRFToken()
	if err == nil {
		t.Fatal("expected error when var.ini not found")
	}
}

func TestReadCSRFToken_TokenNotInFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "var.ini")
	if err := os.WriteFile(tmpFile, []byte("key=value\n"), 0644); err != nil {
		t.Fatal(err)
	}

	original := VarIniPath
	VarIniPath = tmpFile
	defer func() { VarIniPath = original }()

	_, err := readCSRFToken()
	if err == nil {
		t.Fatal("expected error when csrf_token not found")
	}
	if !contains(err.Error(), "csrf_token not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadCSRFToken_Success(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "var.ini")
	content := `mdState="STARTED"
csrf_token="ABC123DEF456"
mdNumDisks="5"
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	original := VarIniPath
	VarIniPath = tmpFile
	defer func() { VarIniPath = original }()

	token, err := readCSRFToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ABC123DEF456" {
		t.Errorf("expected 'ABC123DEF456', got %q", token)
	}
}

func TestIsEmhttpdAvailable_NotExists(t *testing.T) {
	original := EmhttpdSocket
	EmhttpdSocket = "/tmp/nonexistent_socket"
	defer func() { EmhttpdSocket = original }()

	if IsEmhttpdAvailable() {
		t.Error("expected false when socket doesn't exist")
	}
}

func TestIsProcMdcmdAvailable_NotExists(t *testing.T) {
	original := ProcMdcmd
	ProcMdcmd = "/tmp/nonexistent_proc_mdcmd"
	defer func() { ProcMdcmd = original }()

	if IsProcMdcmdAvailable() {
		t.Error("expected false when /proc/mdcmd doesn't exist")
	}
}

func TestIsProcMdcmdAvailable_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "mdcmd")
	if err := os.WriteFile(tmpFile, nil, 0666); err != nil {
		t.Fatal(err)
	}

	original := ProcMdcmd
	ProcMdcmd = tmpFile
	defer func() { ProcMdcmd = original }()

	if !IsProcMdcmdAvailable() {
		t.Error("expected true when file exists")
	}
}

func TestEmhttpdRequest_NoSocket(t *testing.T) {
	// Set up valid CSRF token
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "var.ini")
	if err := os.WriteFile(tmpFile, []byte(`csrf_token="TESTTOKEN"`), 0644); err != nil {
		t.Fatal(err)
	}
	original := VarIniPath
	VarIniPath = tmpFile
	defer func() { VarIniPath = original }()

	// Socket won't exist — expect connection error
	err := EmhttpdRequest(map[string]string{"cmdStatus": "Get"})
	if err == nil {
		t.Fatal("expected error when socket not available")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
