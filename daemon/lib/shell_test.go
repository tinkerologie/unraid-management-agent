package lib

import (
	"strings"
	"testing"
	"time"
)

func TestExecCommand(t *testing.T) {
	// Test successful command
	lines, err := ExecCommand("echo", "hello", "world")
	if err != nil {
		t.Fatalf("ExecCommand failed: %v", err)
	}
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}
	if lines[0] != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", lines[0])
	}
}

func TestExecCommandWithTimeout(t *testing.T) {
	// Test command timeout (sleep for 2 seconds with 1 second timeout)
	_, err := ExecCommandWithTimeout(1*time.Second, "sleep", "2")
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestExecCommandOutput(t *testing.T) {
	// Test successful command with output
	output, err := ExecCommandOutput("echo", "test output")
	if err != nil {
		t.Fatalf("ExecCommandOutput failed: %v", err)
	}
	if !strings.Contains(output, "test output") {
		t.Errorf("Expected 'test output' in output, got: %s", output)
	}
}

func TestCommandExists(t *testing.T) {
	// Test for commands that should exist
	if !CommandExists("echo") {
		t.Error("echo command should exist")
	}
	if !CommandExists("ls") {
		t.Error("ls command should exist")
	}

	// Test for command that shouldn't exist
	if CommandExists("this-command-definitely-does-not-exist-12345") {
		t.Error("Non-existent command should not exist")
	}
}

func TestExecCommandFailure(t *testing.T) {
	// Test command that doesn't exist
	_, err := ExecCommand("command-that-does-not-exist")
	if err == nil {
		t.Fatal("Expected error for non-existent command")
	}
}

func TestExecCommandOutputFailure(t *testing.T) {
	// Test command that returns non-zero exit code
	output, err := ExecCommandOutput("false")
	if err == nil {
		t.Fatal("Expected error for failed command")
	}
	// Output might be empty but error should indicate failure
	_ = output
}

func TestExecCommandOutputNonExistent(t *testing.T) {
	// Test command that doesn't exist
	_, err := ExecCommandOutput("command-that-does-not-exist")
	if err == nil {
		t.Fatal("Expected error for non-existent command")
	}
}

func TestExecCommandWithTimeoutSuccess(t *testing.T) {
	// Test command completes within timeout
	lines, err := ExecCommandWithTimeout(5*time.Second, "echo", "quick")
	if err != nil {
		t.Fatalf("ExecCommandWithTimeout failed: %v", err)
	}
	if len(lines) != 1 || lines[0] != "quick" {
		t.Errorf("Expected ['quick'], got %v", lines)
	}
}

func TestExecCommandWithTimeoutMultiLine(t *testing.T) {
	// Test command with multiple lines of output
	lines, err := ExecCommandWithTimeout(5*time.Second, "printf", "line1\nline2\nline3")
	if err != nil {
		t.Fatalf("ExecCommandWithTimeout failed: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
}

func TestExecCommandWithTimeoutNonExistent(t *testing.T) {
	// Test non-existent command
	_, err := ExecCommandWithTimeout(5*time.Second, "command-that-does-not-exist")
	if err == nil {
		t.Fatal("Expected error for non-existent command")
	}
}
