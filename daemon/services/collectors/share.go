package collectors

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type ShareCollector struct {
	ctx *domain.Context
}

func NewShareCollector(ctx *domain.Context) *ShareCollector {
	return &ShareCollector{ctx: ctx}
}

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
	logger.Debug("Share: Starting collection from %s", common.SharesIni)
	var shares []dto.ShareInfo

	// Parse shares.ini
	file, err := os.Open(common.SharesIni)
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

	logger.Debug("Share: Parsed %d shares successfully", len(shares))
	return shares, nil
}
