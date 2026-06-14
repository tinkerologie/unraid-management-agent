package collectors

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// moverLogPath is the path to the mover log file. It is a package-level variable
// (not a constant) so that tests can override it with a fixture file path.
var moverLogPath = "/var/log/mover.log"

// moverStartupStagger delays the first collection so it does not pile onto boot.
const moverStartupStagger = 10 * time.Second

// MoverCollector periodically reads mover state from var.ini and last-run
// statistics from /var/log/mover.log. It publishes on TopicMoverUpdate whenever
// the active flag, finish time, or file count changes (dedupe).
type MoverCollector struct {
	appCtx *domain.Context
	// CheckFn performs the actual data collection. The constructor installs a
	// default implementation; callers may replace it for testing.
	CheckFn func() (*dto.MoverStatus, error)
	lastSig string
}

// NewMoverCollector creates a new MoverCollector with the default CheckFn installed.
func NewMoverCollector(ctx *domain.Context) *MoverCollector {
	c := &MoverCollector{appCtx: ctx}
	c.CheckFn = c.defaultCheck
	return c
}

// Start begins the periodic mover status collection after a startup stagger.
func (c *MoverCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting mover collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(moverStartupStagger):
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Mover collector", r)
			}
		}()
		collectWithWatchdog(ctx, "Mover", interval, c.Collect)
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Mover collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Mover collector", r)
					}
				}()
				collectWithWatchdog(ctx, "Mover", interval, c.Collect)
			}()
		}
	}
}

// Collect runs a mover status check and publishes the result only when the
// dedupe signature changes (active + last-run-finish + files-moved).
func (c *MoverCollector) Collect() {
	if c.CheckFn == nil {
		logger.Warning("Mover: CheckFn not set, skipping collect")
		return
	}

	result, err := c.CheckFn()
	if err != nil {
		logger.Warning("Mover: check failed: %v", err)
		return
	}
	if result == nil {
		return
	}

	// Dedupe: only publish when active flag, last finish time, or file count changes.
	sig := fmt.Sprintf("%v|%s|%d", result.Active, result.LastRunFinish, result.LastRunFilesMoved)
	if sig == c.lastSig {
		logger.Debug("Mover: no change (active=%v finish=%s files=%d), skipping publish",
			result.Active, result.LastRunFinish, result.LastRunFilesMoved)
		return
	}
	c.lastSig = sig

	domain.Publish(c.appCtx.Hub, constants.TopicMoverUpdate, result)
	logger.Info("Mover: published (active=%v, files=%d, bytes=%d)",
		result.Active, result.LastRunFilesMoved, result.LastRunBytesMoved)
}

// defaultCheck reads mover state from var.ini and last-run stats from moverLogPath.
func (c *MoverCollector) defaultCheck() (*dto.MoverStatus, error) {
	result := &dto.MoverStatus{
		Timestamp: time.Now(),
	}

	// Read Active + Schedule from var.ini
	sc := NewSettingsCollector()
	if ms, err := sc.GetMoverSettings(); err == nil {
		result.Active = ms.Active
		result.Schedule = ms.Schedule
	} else {
		logger.Debug("Mover: could not read var.ini mover settings: %v", err)
	}

	// Parse last-run stats from mover.log
	// #nosec G304 -- moverLogPath is a package-level variable pointing to the well-known system log.
	f, err := os.Open(moverLogPath)
	if err != nil {
		// Log is absent (mover never ran or logging disabled) — not an error.
		logger.Debug("Mover: log not readable (%v); returning zero last-run values", err)
		return result, nil
	}
	defer f.Close() //nolint:errcheck

	start, finish, files, bytes := parseMoverLog(f)
	if !start.IsZero() {
		result.LastRunStart = start.UTC().Format(time.RFC3339)
	}
	if !finish.IsZero() {
		result.LastRunFinish = finish.UTC().Format(time.RFC3339)
	}
	if !start.IsZero() && !finish.IsZero() && finish.After(start) {
		result.LastRunDurationSeconds = int(finish.Sub(start).Seconds())
	}
	result.LastRunFilesMoved = files
	result.LastRunBytesMoved = bytes
	// CurrentThroughputMBs: always 0 (live throughput is out of scope for conservative version)

	return result, nil
}

// parseMoverLog scans a mover log and extracts the most recent run's start time,
// finish time, file count, and byte count.
//
// Assumed log format (Unraid 6.x / 7.x mover script):
//
//	Started (Mon May 30 03:40:00 UTC 2026)
//	Finished (Mon May 30 03:52:00 UTC 2026)
//	Mover: 1024 files (5368709120 bytes) moved
//
// The parser is defensive: it recognises only lines that match the patterns below
// and returns zero-values for any fields absent from the log. Unrecognised lines
// are silently skipped so that format variations do not cause hard failures.
func parseMoverLog(r io.Reader) (start, finish time.Time, files, bytes uint64) {
	// Regex patterns for the three line types we recognise.
	// Using UNIX date format: "Day Mon DD HH:MM:SS TZ YYYY"
	startRe := regexp.MustCompile(`[Ss]tarted\s+\((.+?)\)`)
	finishRe := regexp.MustCompile(`[Ff]inished\s+\((.+?)\)`)
	// "Mover: N files (M bytes) moved" or "moved N files, M bytes"
	filesRe := regexp.MustCompile(`(\d+)\s+files?`)
	bytesRe := regexp.MustCompile(`(\d+)\s+bytes?`)

	// We want the LAST run, so we keep scanning and overwrite on each match.
	var (
		latestStart  time.Time
		latestFinish time.Time
		latestFiles  uint64
		latestBytes  uint64
	)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if m := startRe.FindStringSubmatch(line); len(m) == 2 {
			if t, err := parseUnixDate(m[1]); err == nil {
				latestStart = t
				// Reset finish/files/bytes so they belong to this run.
				latestFinish = time.Time{}
				latestFiles = 0
				latestBytes = 0
			}
			continue
		}

		if m := finishRe.FindStringSubmatch(line); len(m) == 2 {
			if t, err := parseUnixDate(m[1]); err == nil {
				latestFinish = t
			}
			continue
		}

		// Lines with file/byte counts (may appear on the same line or separately).
		if fm := filesRe.FindStringSubmatch(line); len(fm) == 2 {
			if n, err := strconv.ParseUint(fm[1], 10, 64); err == nil {
				latestFiles = n
			}
		}
		if bm := bytesRe.FindStringSubmatch(line); len(bm) == 2 {
			if n, err := strconv.ParseUint(bm[1], 10, 64); err == nil {
				latestBytes = n
			}
		}
	}

	return latestStart, latestFinish, latestFiles, latestBytes
}

// parseUnixDate parses a UNIX `date`-style string such as
// "Mon May 30 03:40:00 UTC 2026". It falls back to several common variants.
func parseUnixDate(s string) (time.Time, error) {
	// Normalise whitespace.
	s = strings.Join(strings.Fields(s), " ")

	formats := []string{
		"Mon Jan 2 15:04:05 MST 2006",
		"Mon Jan  2 15:04:05 MST 2006",
		"Mon Jan 2 15:04:05 2006",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("mover: cannot parse date %q", s)
}
