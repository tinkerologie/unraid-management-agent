package controllers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// UserScriptsBasePath is the base directory for user scripts
	UserScriptsBasePath = "/boot/config/plugins/user.scripts/scripts"
)

// ListUserScripts returns a list of all available user scripts
func ListUserScripts() ([]dto.UserScriptInfo, error) {
	scripts := []dto.UserScriptInfo{}

	// Check if user scripts directory exists
	if _, err := os.Stat(UserScriptsBasePath); os.IsNotExist(err) {
		logger.Warning("User scripts directory does not exist: %s", UserScriptsBasePath)
		return scripts, nil
	}

	// Read all subdirectories in the user scripts directory
	entries, err := os.ReadDir(UserScriptsBasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user scripts directory: %w", err)
	}

	for _, entry := range entries {
		// Skip files and macOS metadata files
		if !entry.IsDir() {
			continue
		}

		scriptName := entry.Name()
		scriptDir := filepath.Join(UserScriptsBasePath, scriptName)
		scriptPath := filepath.Join(scriptDir, "script")
		descriptionPath := filepath.Join(scriptDir, "description")

		// Check if script file exists
		scriptInfo, err := os.Stat(scriptPath)
		if err != nil {
			logger.Debug("Script file not found for %s: %v", scriptName, err)
			continue
		}

		// Read description if it exists
		description := ""
		// #nosec G304 - descriptionPath is constructed from trusted userscripts directory
		if descData, err := os.ReadFile(descriptionPath); err == nil {
			description = strings.TrimSpace(string(descData))
		}

		// Check if script is executable (has read permission at minimum)
		executable := scriptInfo.Mode().Perm()&0400 != 0

		scripts = append(scripts, dto.UserScriptInfo{
			Name:         scriptName,
			Description:  description,
			Path:         scriptPath,
			Executable:   executable,
			LastModified: scriptInfo.ModTime(),
		})
	}

	logger.Debug("Found %d user scripts", len(scripts))
	return scripts, nil
}

// ExecuteUserScript executes a user script with the specified options
func ExecuteUserScript(scriptName string, background bool, wait bool) (*dto.UserScriptExecuteResponse, error) {
	// Validate script name to prevent path traversal
	if err := lib.ValidateUserScriptName(scriptName); err != nil {
		return &dto.UserScriptExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid script name: %v", err),
		}, err
	}

	// Build script path
	scriptPath := filepath.Join(UserScriptsBasePath, scriptName, "script")

	// Verify script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return &dto.UserScriptExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Script not found: %s", scriptName),
		}, fmt.Errorf("script not found: %s", scriptName)
	}

	// Execute script based on options
	if background && !wait {
		// Background execution - don't wait for completion
		return executeScriptBackground(scriptPath, scriptName)
	}
	if wait {
		// Wait for completion and return output
		return executeScriptWait(scriptPath, scriptName)
	}
	// Default: background execution
	return executeScriptBackground(scriptPath, scriptName)
}

// executeScriptBackground executes a script in the background without waiting
// for completion. It uses os.StartProcess with direct arguments to avoid shell
// interpolation (CWE-78 prevention) and redirects stdio to /dev/null so the
// API request is not blocked.
func executeScriptBackground(scriptPath string, scriptName string) (*dto.UserScriptExecuteResponse, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		err = fmt.Errorf("open %s: %w", os.DevNull, err)
		logger.Error("Failed to prepare background execution for user script %s: %v", scriptName, err)
		return &dto.UserScriptExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to execute script: %v", err),
		}, err
	}
	defer func() {
		if closeErr := devNull.Close(); closeErr != nil {
			logger.Error("Failed to close %s for user script %s: %v", os.DevNull, scriptName, closeErr)
		}
	}()

	procAttr := &os.ProcAttr{
		Files: []*os.File{devNull, devNull, devNull},
	}

	proc, err := os.StartProcess("/bin/bash", []string{"bash", scriptPath}, procAttr)
	if err != nil {
		err = fmt.Errorf("start background script %s: %w", scriptName, err)
		logger.Error("Failed to execute user script %s in background: %v", scriptName, err)
		return &dto.UserScriptExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to execute script: %v", err),
		}, err
	}

	// Reap the child in a background goroutine to prevent zombie processes.
	go func() {
		if _, err := proc.Wait(); err != nil {
			logger.Warning("Background script %s exited with error: %v", scriptName, err)
		}
	}()

	logger.Info("User script %s started in background", scriptName)
	return &dto.UserScriptExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Script %s started in background", scriptName),
	}, nil
}

// executeScriptWait executes a script and waits for completion
func executeScriptWait(scriptPath string, scriptName string) (*dto.UserScriptExecuteResponse, error) {
	// Execute script and wait for completion
	startTime := time.Now()
	lines, err := lib.ExecCommand("bash", scriptPath)
	duration := time.Since(startTime)

	// Join output lines
	output := strings.Join(lines, "\n")

	if err != nil {
		logger.Error("User script %s failed after %v: %v", scriptName, duration, err)
		return &dto.UserScriptExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("Script execution failed: %v", err),
			Output:  output,
		}, err
	}

	logger.Info("User script %s completed successfully in %v", scriptName, duration)
	return &dto.UserScriptExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Script %s completed successfully", scriptName),
		Output:  output,
	}, nil
}
