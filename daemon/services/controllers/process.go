package controllers

import (
	"fmt"
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
