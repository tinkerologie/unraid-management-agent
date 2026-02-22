package api

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Common log file locations on Unraid
// Expanded to match Unraid GraphQL API coverage per issue #28
var commonLogPaths = []string{
	// Core system logs
	"/var/log/syslog",
	"/var/log/dmesg",
	"/var/log/messages",
	"/var/log/cron",
	"/var/log/debug",
	"/var/log/btmp",
	"/var/log/lastlog",
	"/var/log/wtmp",

	// Unraid-specific logs
	"/var/log/docker.log",
	"/var/log/libvirt/libvirtd.log",
	"/var/log/unraid-management-agent.log",
	"/var/log/graphql-api.log",
	"/var/log/unraid-api.log",
	"/var/log/recycle.log",
	"/var/log/dhcplog",
	"/var/log/pkgtools/script.log",
	"/var/log/mover.log",

	// UPS logs
	"/var/log/apcupsd.events",
	"/var/log/nohup.out",

	// Web server logs
	"/var/log/nginx/error.log",
	"/var/log/nginx/access.log",

	// VFS and share logs
	"/var/log/vfsd.log",
	"/var/log/smbd.log",
	"/var/log/nfsd.log",

	// Plugin and system logs
	"/var/log/plugins",
	"/var/log/samba/log.smbd",
	"/var/log/samba/log.nmbd",
}

// listLogFiles returns a list of available log files
func (s *Server) listLogFiles() []dto.LogFile {
	var logs []dto.LogFile

	// Check common log paths
	for _, path := range commonLogPaths {
		if info, err := os.Stat(path); err == nil {
			logs = append(logs, dto.LogFile{
				Name:       filepath.Base(path),
				Path:       path,
				Size:       info.Size(),
				ModifiedAt: info.ModTime(),
			})
		}
	}

	// Check plugin logs
	pluginLogsDir := "/boot/config/plugins"
	if entries, err := os.ReadDir(pluginLogsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				logsPath := filepath.Join(pluginLogsDir, entry.Name(), "logs")
				if logEntries, err := os.ReadDir(logsPath); err == nil {
					for _, logEntry := range logEntries {
						if !logEntry.IsDir() && strings.HasSuffix(logEntry.Name(), ".log") {
							fullPath := filepath.Join(logsPath, logEntry.Name())
							if info, err := os.Stat(fullPath); err == nil {
								logs = append(logs, dto.LogFile{
									Name:       fmt.Sprintf("%s/%s", entry.Name(), logEntry.Name()),
									Path:       fullPath,
									Size:       info.Size(),
									ModifiedAt: info.ModTime(),
								})
							}
						}
					}
				}
			}
		}
	}

	return logs
}

// getLogContent retrieves log file content with optional pagination
func (s *Server) getLogContent(path, linesParam, startParam string) (*dto.LogFileContent, error) {
	// Validate path (prevent directory traversal)
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid path: directory traversal not allowed")
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("log file not found: %s", path)
	}

	// Read file
	file, err := os.Open(path) // #nosec G304 - path is validated above
	if err != nil {
		logger.Error("Failed to open log file %s: %v", path, err)
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Error("Failed to close log file %s: %v", path, err)
		}
	}()

	// Read all lines
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Failed to read log file %s: %v", path, err)
		return nil, fmt.Errorf("failed to read log file: %v", err)
	}

	totalLines := len(allLines)

	// Parse pagination parameters
	var startLine, numLines int
	startSpecified := startParam != ""
	linesSpecified := linesParam != ""

	if startSpecified {
		if val, err := strconv.Atoi(startParam); err == nil {
			startLine = val
		}
	}
	if linesSpecified {
		if val, err := strconv.Atoi(linesParam); err == nil {
			numLines = val
		}
	}

	// Default: return all lines if no pagination specified
	if !linesSpecified && !startSpecified {
		return &dto.LogFileContent{
			Path:          path,
			Content:       strings.Join(allLines, "\n"),
			Lines:         allLines,
			TotalLines:    totalLines,
			LinesReturned: totalLines,
			StartLine:     0,
			EndLine:       totalLines,
		}, nil
	}

	// If only lines specified (no start), return last N lines (tail behavior)
	if linesSpecified && !startSpecified {
		if numLines > totalLines {
			numLines = totalLines
		}
		startLine = totalLines - numLines
	}

	// Validate and adjust range
	if startLine < 0 {
		startLine = 0
	}
	if startLine >= totalLines {
		return &dto.LogFileContent{
			Path:          path,
			Content:       "",
			Lines:         []string{},
			TotalLines:    totalLines,
			LinesReturned: 0,
			StartLine:     startLine,
			EndLine:       startLine,
		}, nil
	}

	endLine := min(startLine+numLines, totalLines)

	selectedLines := allLines[startLine:endLine]

	return &dto.LogFileContent{
		Path:          path,
		Content:       strings.Join(selectedLines, "\n"),
		Lines:         selectedLines,
		TotalLines:    totalLines,
		LinesReturned: len(selectedLines),
		StartLine:     startLine,
		EndLine:       endLine,
	}, nil
}
