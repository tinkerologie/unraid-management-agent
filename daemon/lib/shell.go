package lib

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// These helpers execute binaries directly without shell expansion. Callers must
// pass fixed internal commands or values that have already been validated.

// ExecCommand executes a shell command with timeout
func ExecCommand(command string, args ...string) ([]string, error) {
	return ExecCommandWithTimeout(60*time.Second, command, args...)
}

// ExecCommandWithTimeout executes a command with a specific timeout
func ExecCommandWithTimeout(timeout time.Duration, command string, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	var lines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return lines, fmt.Errorf("error reading output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return lines, fmt.Errorf("command timed out after %v", timeout)
		}
		return lines, fmt.Errorf("command failed: %w", err)
	}

	return lines, nil
}

// ExecCommandOutput executes a command and returns combined output
func ExecCommandOutput(command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return string(output), nil
}

// ExecCommandOutputWithContext executes a command and returns combined output,
// honouring the caller's context for cancellation.
func ExecCommandOutputWithContext(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}
	return string(output), nil
}

// CommandExists checks if a command exists in PATH
func CommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
