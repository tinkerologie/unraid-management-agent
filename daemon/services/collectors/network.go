package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type NetworkCollector struct {
	ctx *domain.Context
}

func NewNetworkCollector(ctx *domain.Context) *NetworkCollector {
	return &NetworkCollector{ctx: ctx}
}

func (c *NetworkCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting network collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Network collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Network collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Network collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

func (c *NetworkCollector) Collect() {
	logger.Debug("Collecting network data...")

	// Collect network interfaces
	interfaces, err := c.collectNetworkInterfaces()
	if err != nil {
		logger.Error("Network: Failed to collect interface data: %v", err)
		return
	}

	logger.Debug("Network: Successfully collected %d interfaces, publishing event", len(interfaces))
	// Publish event
	c.ctx.Hub.Pub(interfaces, "network_list_update")
	logger.Debug("Network: Published network_list_update event with %d interfaces", len(interfaces))
}

func (c *NetworkCollector) collectNetworkInterfaces() ([]dto.NetworkInfo, error) {
	logger.Debug("Network: Starting collection from /proc/net/dev and /sys/class/net")
	var interfaces []dto.NetworkInfo

	// Parse /proc/net/dev for bandwidth stats
	stats, err := c.parseNetDev()
	if err != nil {
		logger.Error("Network: Failed to parse /proc/net/dev: %v", err)
		return nil, err
	}

	// Get interface details from /sys/class/net
	for ifName, ifStats := range stats {
		// Skip loopback
		if ifName == "lo" {
			continue
		}

		netInfo := dto.NetworkInfo{
			Name:            ifName,
			BytesReceived:   ifStats.BytesReceived,
			BytesSent:       ifStats.BytesSent,
			PacketsReceived: ifStats.PacketsReceived,
			PacketsSent:     ifStats.PacketsSent,
			ErrorsReceived:  ifStats.ErrorsReceived,
			ErrorsSent:      ifStats.ErrorsSent,
			Timestamp:       time.Now(),
		}

		// Get MAC address
		netInfo.MACAddress = c.getMACAddress(ifName)

		// Get IP address
		netInfo.IPAddress = c.getIPAddress(ifName)

		// Get link speed
		netInfo.Speed = c.getLinkSpeed(ifName)

		// Get operational state
		netInfo.State = c.getOperState(ifName)

		interfaces = append(interfaces, netInfo)
	}

	logger.Debug("Network: Parsed %d interfaces successfully", len(interfaces))
	return interfaces, nil
}

type netStats struct {
	BytesReceived   uint64
	PacketsReceived uint64
	ErrorsReceived  uint64
	BytesSent       uint64
	PacketsSent     uint64
	ErrorsSent      uint64
}

func (c *NetworkCollector) parseNetDev() (map[string]netStats, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing network stats file: %v", err)
		}
	}()

	stats := make(map[string]netStats)
	scanner := bufio.NewScanner(file)

	// Skip header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		ifName := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])

		if len(fields) < 16 {
			continue
		}

		stats[ifName] = netStats{
			BytesReceived:   parseUint64(fields[0]),
			PacketsReceived: parseUint64(fields[1]),
			ErrorsReceived:  parseUint64(fields[2]),
			BytesSent:       parseUint64(fields[8]),
			PacketsSent:     parseUint64(fields[9]),
			ErrorsSent:      parseUint64(fields[10]),
		}
	}

	return stats, scanner.Err()
}

func (c *NetworkCollector) getMACAddress(ifName string) string {
	path := fmt.Sprintf("/sys/class/net/%s/address", ifName)
	//nolint:gosec // G304: Path is constructed from /sys/class/net system directory, ifName from trusted source
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (c *NetworkCollector) getIPAddress(ifName string) string {
	// Use ip command to get IP address
	output, err := lib.ExecCommandOutput("ip", "-4", "addr", "show", ifName)
	if err != nil {
		return ""
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Return IP without CIDR notation
				ip := strings.Split(fields[1], "/")[0]
				return ip
			}
		}
	}
	return ""
}

func (c *NetworkCollector) getLinkSpeed(ifName string) int {
	path := fmt.Sprintf("/sys/class/net/%s/speed", ifName)
	//nolint:gosec // G304: Path is constructed from /sys/class/net system directory, ifName from trusted source
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	speed, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return speed
}

func (c *NetworkCollector) getOperState(ifName string) string {
	path := fmt.Sprintf("/sys/class/net/%s/operstate", ifName)
	//nolint:gosec // G304: Path is constructed from /sys/class/net system directory, ifName from trusted source
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

func parseUint64(s string) uint64 {
	val, _ := strconv.ParseUint(s, 10, 64)
	return val
}
