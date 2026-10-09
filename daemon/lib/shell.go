package lib

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// These helpers execute binaries directly without shell expansion. Callers must
// pass fixed internal commands or values that have already been validated.

// execCommand is a package-level variable that defaults to exec.CommandContext.
// It can be overridden in tests to inject mock command execution.
var execCommand = exec.CommandContext

// ExecCommand executes a shell command with timeout
func ExecCommand(command string, args ...string) ([]string, error) {
	return ExecCommandWithTimeout(60*time.Second, command, args...)
}

// ExecCommandWithTimeout executes a command with a specific timeout.
// stdout reading runs on a separate goroutine so a child process stuck in
// uninterruptible sleep (e.g. smartctl against an unresponsive disk) cannot
// block the caller past the configured deadline.
func ExecCommandWithTimeout(timeout time.Duration, command string, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	type readResult struct {
		lines []string
		err   error
	}
	resultCh := make(chan readResult, 1)

	go func() {
		var lines []string
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		resultCh <- readResult{lines: lines, err: scanner.Err()}
	}()

	var lines []string
	var scanErr error

	select {
	case res := <-resultCh:
		lines = res.lines
		scanErr = res.err
	case <-ctx.Done():
		// Context expired while reading — kill and clean up without blocking caller.
		_ = cmd.Process.Kill()
		_ = stdout.Close()
		select {
		case res := <-resultCh:
			lines = res.lines
		case <-time.After(10 * time.Millisecond):
		}
		go func() {
			<-resultCh
			_ = cmd.Wait()
		}()
		return lines, fmt.Errorf("command timed out after %v", timeout)
	}

	if scanErr != nil {
		_ = cmd.Wait()
		return lines, fmt.Errorf("error reading output: %w", scanErr)
	}

	if err := cmd.Wait(); err != nil {
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

// ExecCommandOutputWithTimeout executes a command and returns its combined
// output, killing it after timeout. WaitDelay keeps a child process that
// inherited the output pipe (e.g. one started by a shell script) from
// blocking the caller long after the deadline.
func ExecCommandOutputWithTimeout(timeout time.Duration, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	cmd.WaitDelay = time.Second
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

// ExecCommandStdout executes a command and returns only stdout, discarding
// stderr. Use this instead of ExecCommandOutput when the output is
// machine-parsed (e.g. JSON) and stderr warnings could contaminate results.
func ExecCommandStdout(command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := execCommand(ctx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}

// ExecCommandStdoutGuarded runs a command and returns only its stdout. The child
// is killed when ctx is cancelled or timeout elapses, and the call returns
// immediately in that case. The returned channel is closed once the child has
// actually been reaped, which can be much later than the return when the child
// is stuck in uninterruptible sleep (e.g. a storage tool waiting on stalled
// controller firmware). Callers that must never run two instances of a tool at
// once can poll that channel before starting the next invocation.
func ExecCommandStdoutGuarded(ctx context.Context, timeout time.Duration, command string, args ...string) (string, <-chan struct{}, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	done := make(chan struct{})

	cmd := execCommand(runCtx, command, args...) // #nosec G204 -- callers pass validated commands and arguments without shell interpolation
	cmd.WaitDelay = time.Second
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		cancel()
		close(done)
		return "", done, fmt.Errorf("failed to start command: %w", err)
	}

	result := make(chan error, 1)
	go func() {
		result <- cmd.Wait()
		close(done)
	}()

	var waitErr error
	select {
	case waitErr = <-result:
	case <-runCtx.Done():
	}
	ctxErr := runCtx.Err()
	cancel()
	if ctxErr != nil {
		// The context killed the child. stdout is not read: unless Wait has
		// returned, the copy goroutine may still be writing to it.
		return "", done, fmt.Errorf("command %s did not finish within %v: %w", command, timeout, ctxErr)
	}
	if waitErr != nil {
		return stdout.String(), done, fmt.Errorf("command failed: %w", waitErr)
	}
	return stdout.String(), done, nil
}

// CommandExists checks if a command exists in PATH
func CommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
