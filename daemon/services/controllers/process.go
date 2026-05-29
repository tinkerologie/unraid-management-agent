package controllers

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ProcessController provides operations for listing running processes.
type ProcessController struct{}

// NewProcessController creates a new process controller.
func NewProcessController() *ProcessController {
	return &ProcessController{}
}

// ListProcesses returns all running processes on the system.
func (pc *ProcessController) ListProcesses(sortBy string, limit int) (*dto.ProcessList, error) {
	logger.Debug("Process: Listing running processes (sortBy=%s, limit=%d)", sortBy, limit)

	// Use ps to get process list with key fields
	// aux format: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
	lines, err := lib.ExecCommand("/bin/ps", "aux", "--sort=-pcpu")
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %w", err)
	}

	result := &dto.ProcessList{
		Processes: make([]dto.ProcessInfo, 0),
		Timestamp: time.Now(),
	}

	for i, line := range lines {
		// Skip header line
		if i == 0 {
			continue
		}

		proc, err := parseProcessLine(line)
		if err != nil {
			continue
		}

		result.Processes = append(result.Processes, *proc)
	}

	result.TotalCount = len(result.Processes)

	// Sort if requested (ps already sorts by CPU by default)
	switch sortBy {
	case "memory":
		sortProcessesByMemory(result.Processes)
	case "pid":
		sortProcessesByPID(result.Processes)
	}

	// Apply limit
	if limit > 0 && limit < len(result.Processes) {
		result.Processes = result.Processes[:limit]
	}

	logger.Debug("Process: Found %d processes", result.TotalCount)
	return result, nil
}

// procIOSampleInterval is the delay between the two /proc/<pid>/io reads used
// to derive a per-process I/O rate.
const procIOSampleInterval = 500 * time.Millisecond

// ListProcessIO returns the top processes by current disk I/O rate, derived
// from two samples of /proc/<pid>/io. This is a lightweight, native alternative
// to spawning iotop-c (bundled in Unraid 7.3) and incurs no always-on cost.
func (pc *ProcessController) ListProcessIO(limit int) (*dto.ProcessList, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative, got %d", limit)
	}
	logger.Debug("Process: Sampling per-process disk I/O (limit=%d)", limit)

	first := sampleProcIO()
	time.Sleep(procIOSampleInterval)
	second := sampleProcIO()

	elapsed := procIOSampleInterval.Seconds()
	result := &dto.ProcessList{
		Processes: make([]dto.ProcessInfo, 0, len(second)),
		Timestamp: time.Now(),
	}

	for pid, cur := range second {
		prev, ok := first[pid]
		if !ok || cur.read < prev.read || cur.write < prev.write {
			continue // process started mid-sample or counters reset
		}

		readRate := uint64(float64(cur.read-prev.read) / elapsed)
		writeRate := uint64(float64(cur.write-prev.write) / elapsed)
		if readRate == 0 && writeRate == 0 {
			continue
		}

		result.Processes = append(result.Processes, dto.ProcessInfo{
			PID:                  pid,
			Command:              readProcComm(pid),
			DiskReadBytesPerSec:  readRate,
			DiskWriteBytesPerSec: writeRate,
		})
	}

	result.TotalCount = len(result.Processes)
	sortProcessesByIO(result.Processes)

	if limit > 0 && limit < len(result.Processes) {
		result.Processes = result.Processes[:limit]
	}

	logger.Debug("Process: %d processes with active I/O", result.TotalCount)
	return result, nil
}

// procIOCounters holds cumulative read/write bytes for a process.
type procIOCounters struct {
	read  uint64
	write uint64
}

// sampleProcIO reads cumulative disk I/O counters for every process from
// /proc/<pid>/io. Processes whose io file is unreadable are skipped.
func sampleProcIO() map[int]procIOCounters {
	samples := make(map[int]procIOCounters)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return samples
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory
		}

		// #nosec G304 -- path is /proc/<numeric-pid>/io, bounded to the proc fs.
		data, err := os.ReadFile("/proc/" + entry.Name() + "/io")
		if err != nil {
			continue
		}

		var c procIOCounters
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			switch fields[0] {
			case "read_bytes:":
				c.read, _ = strconv.ParseUint(fields[1], 10, 64)
			case "write_bytes:":
				c.write, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
		samples[pid] = c
	}

	return samples
}

// readProcComm reads the process command name from /proc/<pid>/comm.
func readProcComm(pid int) string {
	// #nosec G304 -- path is /proc/<numeric-pid>/comm, bounded to the proc fs.
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// sortProcessesByIO sorts processes by total disk I/O rate descending.
func sortProcessesByIO(procs []dto.ProcessInfo) {
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].DiskReadBytesPerSec+procs[i].DiskWriteBytesPerSec >
			procs[j].DiskReadBytesPerSec+procs[j].DiskWriteBytesPerSec
	})
}

// parseProcessLine parses a single line from ps aux output.
func parseProcessLine(line string) (*dto.ProcessInfo, error) {
	// ps aux fields: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
	// Fields are whitespace-separated, but COMMAND can contain spaces
	fields := strings.Fields(line)
	if len(fields) < 11 {
		return nil, fmt.Errorf("invalid process line: too few fields")
	}

	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("invalid PID: %w", err)
	}

	cpuPercent, _ := strconv.ParseFloat(fields[2], 64)
	memPercent, _ := strconv.ParseFloat(fields[3], 64)
	vsz, _ := strconv.ParseUint(fields[4], 10, 64)
	rss, _ := strconv.ParseUint(fields[5], 10, 64)

	// Command is everything from field 10 onwards (may contain spaces)
	command := strings.Join(fields[10:], " ")

	return &dto.ProcessInfo{
		PID:           pid,
		User:          fields[0],
		CPUPercent:    cpuPercent,
		MemoryPercent: memPercent,
		VSZBytes:      vsz * 1024, // VSZ is in KB
		RSSBytes:      rss * 1024, // RSS is in KB
		TTY:           fields[6],
		State:         fields[7],
		Started:       fields[8],
		Time:          fields[9],
		Command:       command,
	}, nil
}

// sortProcessesByMemory sorts processes by memory usage descending.
func sortProcessesByMemory(procs []dto.ProcessInfo) {
	for i := range procs {
		for j := i + 1; j < len(procs); j++ {
			if procs[j].MemoryPercent > procs[i].MemoryPercent {
				procs[i], procs[j] = procs[j], procs[i]
			}
		}
	}
}

// sortProcessesByPID sorts processes by PID ascending.
func sortProcessesByPID(procs []dto.ProcessInfo) {
	for i := range procs {
		for j := i + 1; j < len(procs); j++ {
			if procs[j].PID < procs[i].PID {
				procs[i], procs[j] = procs[j], procs[i]
			}
		}
	}
}
