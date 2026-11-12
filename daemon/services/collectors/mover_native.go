package collectors

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

const (
	moverStatusTimeout = 5 * time.Second
	moverNativeSource  = "native"
	moverLegacySource  = "legacy"
)

type moverChecker struct {
	version func() string
	stat    func(string) (os.FileInfo, error)
	status  func(context.Context) error
	open    func(string) (io.ReadCloser, error)

	native atomic.Bool
}

var hostMover = &moverChecker{
	version: func() string {
		if v := platform.DetectUnraidVersion(); v != "" {
			return v
		}
		return readCurrentOSVersion()
	},
	stat: os.Stat,
	status: func(ctx context.Context) error {
		_, _, err := lib.ExecCommandStdoutGuarded(ctx, moverStatusTimeout, constants.MoverBin, "status")
		return err
	},
	open: func(path string) (io.ReadCloser, error) { return os.Open(path) }, // #nosec G304 -- only fixed Mover paths are opened.
}

func (m *moverChecker) activity(ctx context.Context, legacy bool) (active bool, source string) {
	if !m.supportsNative() {
		return legacy, moverLegacySource
	}
	err := m.status(ctx)
	if err == nil {
		return true, moverNativeSource
	}
	var exit *exec.ExitError
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, moverNativeSource
	}
	logger.Debug("Mover: native status unavailable")
	return legacy, moverLegacySource
}

// Cache confirmed native support. Retry detection when either system source is
// unavailable during startup or a plugin upgrade.
func (m *moverChecker) supportsNative() bool {
	if m.native.Load() {
		return true
	}
	version, _, _ := strings.Cut(m.version(), "-")
	fields := strings.Split(version, ".")
	if len(fields) < 2 {
		return false
	}
	major, majorErr := strconv.ParseUint(fields[0], 10, 32)
	minor, minorErr := strconv.ParseUint(fields[1], 10, 32)
	if majorErr != nil || minorErr != nil || major < 7 || (major == 7 && minor < 4) {
		return false
	}
	fi, err := m.stat(constants.MoverBin)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return false
	}
	m.native.Store(true)
	return true
}

func (m *moverChecker) pid() uint64 {
	f, err := m.open(constants.MoverPID)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(data) > 32 {
		return 0
	}
	id, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
	return id
}

func (m *moverChecker) progress(result *dto.MoverStatus) {
	pid := m.pid()
	if pid == 0 {
		return
	}
	f, err := m.open(constants.MoverProgress)
	if errors.Is(err, os.ErrNotExist) {
		return // No snapshot while idle, starting, or opted out.
	}
	if err != nil {
		logger.Debug("Mover: progress snapshot unavailable")
		return
	}
	defer func() { _ = f.Close() }()

	values, ok := parseMoverProgress(f)
	if !ok {
		logger.Debug("Mover: invalid progress snapshot")
		return
	}
	if values["MoverPid"] != pid || m.pid() != pid {
		return
	}

	total, remaining := values["TotalSize"], values["RemainSize"]
	moved := total - remaining
	result.ProgressTotalBytes = &total
	result.ProgressRemainingBytes = &remaining
	result.ProgressMovedBytes = &moved
	if total > 0 {
		percent := 100 * float64(moved) / float64(total)
		result.ProgressPercent = &percent
	}
	if rate := values["Throughput"]; rate > 0 {
		result.ThroughputBytesPerSecond = &rate
		result.CurrentThroughputMBs = float64(rate) / 1_000_000
	}
	if eta := values["ETA"]; eta > 0 {
		result.ETASeconds = &eta
	}
}

func parseMoverProgress(r io.Reader) (map[string]uint64, bool) {
	values := make(map[string]uint64, 5)
	limited := &io.LimitedReader{R: r, N: (64 << 10) + 1}
	scanner := bufio.NewScanner(limited)
	for scanner.Scan() {
		key, raw, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "TotalSize", "RemainSize", "Throughput", "ETA", "MoverPid":
			value, err := strconv.ParseUint(raw, 10, 64)
			if _, duplicate := values[key]; err != nil || duplicate {
				return nil, false
			}
			values[key] = value
		}
	}
	return values, scanner.Err() == nil && limited.N > 0 && len(values) == 5 && values["RemainSize"] <= values["TotalSize"]
}
