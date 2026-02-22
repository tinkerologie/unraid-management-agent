// Package lib provides utility functions for the Unraid Management Agent.
package lib

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// EmhttpdSocket is the path to the emhttpd Unix domain socket.
// The socket provides an HTTP interface to the Unraid emhttpd daemon
// for array management operations (start, stop, parity check, etc.).
var EmhttpdSocket = "/var/run/emhttpd.socket"

// ProcMdcmd is the path to the /proc/mdcmd interface.
// Writing commands directly to this proc file is the most efficient way
// to issue md (array) commands, as it bypasses shell execution entirely.
var ProcMdcmd = "/proc/mdcmd"

// VarIniPath is the path to var.ini for reading the CSRF token.
var VarIniPath = "/var/local/emhttp/var.ini"

// emhttpdClient is a reusable HTTP client configured for Unix socket communication.
var emhttpdClient *http.Client

func init() {
	emhttpdClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", EmhttpdSocket, 10*time.Second)
			},
		},
		Timeout: 30 * time.Second,
	}
}

// MdcmdWrite writes a command directly to /proc/mdcmd, bypassing shell execution.
// This is equivalent to what the mdcmd shell script does: echo "$*" > /proc/mdcmd
// Returns nil on success, or an error if the write fails.
func MdcmdWrite(args ...string) error {
	command := strings.Join(args, " ") + "\n"

	// #nosec G304 - ProcMdcmd is a controlled path to /proc/mdcmd
	f, err := os.OpenFile(ProcMdcmd, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", ProcMdcmd, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(command); err != nil {
		return fmt.Errorf("failed to write command to %s: %w", ProcMdcmd, err)
	}

	logger.Debug("Emhttp: Wrote command to %s: %s", ProcMdcmd, strings.TrimSpace(command))
	return nil
}

// EmhttpdRequest sends an HTTP request to the emhttpd daemon via its Unix socket.
// It reads the CSRF token from var.ini and includes it in the request.
// The params map contains the command parameters (e.g., {"cmdStart": "Start"}).
func EmhttpdRequest(params map[string]string) error {
	// Read CSRF token from var.ini
	csrfToken, err := readCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to read CSRF token: %w", err)
	}

	// Build query parameters
	values := url.Values{}
	values.Set("csrf_token", csrfToken)
	for k, v := range params {
		values.Set(k, v)
	}

	reqURL := fmt.Sprintf("http://localhost/update.htm?%s", values.Encode())

	resp, err := emhttpdClient.Get(reqURL)
	if err != nil {
		return fmt.Errorf("emhttpd socket request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain and discard response body
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("emhttpd returned status %d", resp.StatusCode)
	}

	logger.Debug("Emhttp: Socket request successful: %v", params)
	return nil
}

// readCSRFToken reads the CSRF token from var.ini.
// The token is stored as: csrf_token="HEXSTRING"
func readCSRFToken() (string, error) {
	// #nosec G304 - VarIniPath is a controlled constant path
	data, err := os.ReadFile(VarIniPath)
	if err != nil {
		return "", fmt.Errorf("failed to read var.ini: %w", err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "csrf_token="); ok {
			token := after
			token = strings.Trim(token, `"`)
			if token != "" {
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("csrf_token not found in var.ini")
}

// IsEmhttpdAvailable checks if the emhttpd socket is available.
func IsEmhttpdAvailable() bool {
	info, err := os.Stat(EmhttpdSocket)
	if err != nil {
		return false
	}
	return info.Mode().Type() == os.ModeSocket
}

// IsProcMdcmdAvailable checks if /proc/mdcmd is available for direct writes.
func IsProcMdcmdAvailable() bool {
	_, err := os.Stat(ProcMdcmd)
	return err == nil
}
