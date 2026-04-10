package lib

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

func TestExecCommandStdout(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantOut    string
		wantErr    bool
		wantErrMsg string
		mockStdout string
		mockStderr string
		mockExit   int
		mockCmdErr bool
	}{
		{
			name:       "success stdout-only",
			args:       []string{"echo", "hello stdout"},
			wantOut:    "hello stdout\n",
			wantErr:    false,
			mockStdout: "hello stdout\n",
			mockExit:   0,
		},
		{
			name:       "stdout with stderr present",
			args:       []string{"bash", "-c", "echo stdout_only; echo stderr_warning >&2"},
			wantOut:    "stdout_only\n",
			wantErr:    false,
			mockStdout: "stdout_only\n",
			mockStderr: "stderr_warning\n",
			mockExit:   0,
		},
		{
			name:       "non-zero exit returning wrapped command failed error",
			args:       []string{"false"},
			wantOut:    "",
			wantErr:    true,
			wantErrMsg: "command failed",
			mockStdout: "",
			mockExit:   1,
		},
		{
			name:       "non-existent command returns error",
			args:       []string{"command-that-does-not-exist-xyz"},
			wantOut:    "",
			wantErr:    true,
			wantErrMsg: "",
			mockCmdErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original execCommand and restore after test
			origExecCommand := execCommand
			t.Cleanup(func() {
				execCommand = origExecCommand
			})

			// Mock execCommand using the test helper process pattern
			execCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				// Use the current test binary as the command to execute
				cs := []string{"-test.run=TestHelperProcess", "--", name}
				cs = append(cs, arg...)
				cmd := exec.Command(os.Args[0], cs...)
				cmd.Env = []string{
					"GO_TEST_HELPER_PROCESS=1",
					"MOCK_STDOUT=" + tt.mockStdout,
					"MOCK_STDERR=" + tt.mockStderr,
					fmt.Sprintf("MOCK_EXIT=%d", tt.mockExit),
					fmt.Sprintf("MOCK_CMD_ERR=%t", tt.mockCmdErr),
				}
				return cmd
			}

			// Execute the function under test
			var out string
			var err error
			if len(tt.args) > 0 {
				out, err = ExecCommandStdout(tt.args[0], tt.args[1:]...)
			} else {
				out, err = ExecCommandStdout("")
			}

			// Assert error expectations
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("expected error containing %q, got %v", tt.wantErrMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}

			// Assert output expectations
			if out != tt.wantOut {
				t.Errorf("expected output %q, got %q", tt.wantOut, out)
			}
		})
	}
}

// TestHelperProcess is not a real test - it's a helper process used by TestExecCommandStdout
// to mock command execution. When GO_TEST_HELPER_PROCESS is set, it simulates a command
// by outputting the mock data and exiting with the mock exit code.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	// Check if we should simulate a command not found error
	if os.Getenv("MOCK_CMD_ERR") == "true" {
		fmt.Fprintf(os.Stderr, "executable file not found in $PATH\n")
		os.Exit(127)
	}

	// Write stdout
	stdout := os.Getenv("MOCK_STDOUT")
	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}

	// Write stderr
	stderr := os.Getenv("MOCK_STDERR")
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	// Exit with the specified exit code
	exitCode := 0
	if exitStr := os.Getenv("MOCK_EXIT"); exitStr != "" {
		if ec, err := strconv.Atoi(exitStr); err == nil {
			exitCode = ec
		}
	}
	os.Exit(exitCode)
}