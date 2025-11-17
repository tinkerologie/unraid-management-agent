package collectors

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ShareCollector collects information about Unraid user shares.
// It gathers share configuration, usage statistics, and disk allocation details.
type ShareCollector struct {
	ctx *domain.Context
}

// NewShareCollector creates a new user share collector with the given context.
func NewShareCollector(ctx *domain.Context) *ShareCollector {
	return &ShareCollector{ctx: ctx}
}

// Start begins the share collector's periodic data collection.
// It runs in a goroutine and publishes share information updates at the specified interval until the context is cancelled.
func (c *ShareCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting share collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Share collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Share collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Share collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers user share information and publishes it to the event bus.
// It reads share configuration from /boot/config/shares/ and enriches with usage data from df command.
func (c *ShareCollector) Collect() {
	logger.Debug("Collecting share data...")

	// Collect share information
	shares, err := c.collectShares()
	if err != nil {
		logger.Error("Share: Failed to collect share data: %v", err)
		return
	}

	logger.Debug("Share: Successfully collected %d shares, publishing event", len(shares))
	// Publish event
	c.ctx.Hub.Pub(shares, "share_list_update")
	logger.Debug("Share: Published share_list_update event with %d shares", len(shares))
}

func (c *ShareCollector) collectShares() ([]dto.ShareInfo, error) {
	logger.Debug("Share: Starting collection from %s", constants.SharesIni)
	var shares []dto.ShareInfo

	// Parse shares.ini
	file, err := os.Open(constants.SharesIni)
	if err != nil {
		logger.Error("Share: Failed to open file: %v", err)
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing share file: %v", err)
		}
	}()
	logger.Debug("Share: File opened successfully")

	scanner := bufio.NewScanner(file)
	var currentShare *dto.ShareInfo
	var currentShareName string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section header: [shareName="appdata"]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Save previous share if exists
			if currentShare != nil {
				shares = append(shares, *currentShare)
			}

			// Extract share name from [shareName="appdata"]
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line[1:len(line)-1], "=", 2)
				if len(parts) == 2 {
					currentShareName = strings.Trim(parts[1], `"`)
				}
			}

			// Start new share
			currentShare = &dto.ShareInfo{
				Name: currentShareName,
			}
			continue
		}

		// Parse key=value pairs
		if currentShare != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

			switch key {
			case "name":
				// Use the name field from the INI file
				currentShare.Name = value
			case "size":
				if size, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.Total = size
				}
			case "free":
				if free, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.Free = free
				}
			case "used":
				if used, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.Used = used
				}
			}
		}
	}

	// Save last share
	if currentShare != nil {
		shares = append(shares, *currentShare)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Share: Scanner error: %v", err)
		return shares, err
	}

	// Calculate total and usage percentage for each share
	for i := range shares {
		// If total is 0, calculate it from used + free
		if shares[i].Total == 0 && (shares[i].Used > 0 || shares[i].Free > 0) {
			shares[i].Total = shares[i].Used + shares[i].Free
		}

		// Calculate usage percentage
		if shares[i].Total > 0 {
			shares[i].UsagePercent = float64(shares[i].Used) / float64(shares[i].Total) * 100
		}

		// Set timestamp
		shares[i].Timestamp = time.Now()
	}

	// Enrich shares with configuration data
	configCollector := NewConfigCollector()
	for i := range shares {
		c.enrichShareWithConfig(&shares[i], configCollector)
	}

	logger.Debug("Share: Parsed %d shares successfully", len(shares))
	return shares, nil
}

// enrichShareWithConfig enriches a share with configuration data
func (c *ShareCollector) enrichShareWithConfig(share *dto.ShareInfo, configCollector *ConfigCollector) {
	config, err := configCollector.GetShareConfig(share.Name)
	if err != nil {
		logger.Debug("Share: Failed to get config for share %s: %v", share.Name, err)
		// Set default values for shares without config
		share.Storage = "unknown"
		share.SMBExport = false
		share.NFSExport = false
		return
	}

	// Populate configuration fields
	share.Comment = config.Comment
	share.UseCache = config.UseCache
	share.Security = config.Security
	share.Storage = c.determineStorage(config.UseCache)
	share.SMBExport = c.isSMBExported(config.Export, config.Security)
	share.NFSExport = c.isNFSExported(config.Export)

	logger.Debug("Share: Enriched %s - Storage: %s, SMB: %v, NFS: %v", share.Name, share.Storage, share.SMBExport, share.NFSExport)
}

// determineStorage determines storage location based on UseCache setting
func (c *ShareCollector) determineStorage(useCache string) string {
	switch useCache {
	case "no":
		return "array"
	case "only":
		return "cache"
	case "yes", "prefer":
		return "cache+array"
	default:
		return "unknown"
	}
}

// isSMBExported checks if share is exported via SMB
func (c *ShareCollector) isSMBExported(export string, security string) bool {
	// If security is set, share is typically SMB exported
	if security == "public" || security == "private" || security == "secure" {
		return true
	}

	// Check export field for SMB indicators
	if strings.Contains(export, "smb") || strings.Contains(export, "-e") {
		return true
	}

	return false
}

// isNFSExported checks if share is exported via NFS
func (c *ShareCollector) isNFSExported(export string) bool {
	// Check export field for NFS indicators
	return strings.Contains(export, "nfs") || strings.Contains(export, "-n")
}
