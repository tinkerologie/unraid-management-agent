package collectors

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// prevNetStats holds the previous collection's byte counts for rate calculation.
type prevNetStats struct {
	bytesReceived uint64
	bytesSent     uint64
	timestamp     time.Time
}

// NetworkCollector collects network interface information including status, speed, and statistics.
// It gathers data from network interfaces, bonds, bridges, and VLANs.
type NetworkCollector struct {
	ctx       *domain.Context
	mu        sync.Mutex
	prevStats map[string]prevNetStats
}

// NewNetworkCollector creates a new network interface collector with the given context.
func NewNetworkCollector(ctx *domain.Context) *NetworkCollector {
	return &NetworkCollector{ctx: ctx, prevStats: make(map[string]prevNetStats)}
}

// Start begins the network collector's periodic data collection.
// It runs in a goroutine and publishes network interface updates at the specified interval until the context is cancelled.
func (c *NetworkCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting network collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Network collector", r)
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
						logger.LogPanicWithStack("Network collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers network interface information and publishes it to the event bus.
// It collects data from /sys/class/net and uses ethtool for detailed interface information.
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
	domain.Publish(c.ctx.Hub, constants.TopicNetworkListUpdate, interfaces)
	logger.Debug("Network: Published %s event with %d interfaces", constants.TopicNetworkListUpdate.Name, len(interfaces))
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
	sampleTime := time.Now()
	c.pruneGoneInterfaces(stats)

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
			Timestamp:       sampleTime,
		}

		// Get MAC address
		netInfo.MACAddress = c.getMACAddress(ifName)

		// Get IP address
		netInfo.IPAddress = c.getIPAddress(ifName)

		// Get link speed
		netInfo.Speed = c.getLinkSpeed(ifName)

		// Get operational state
		netInfo.State = c.getOperState(ifName)

		// Get ethtool information (enhanced network details)
		c.enrichWithEthtool(&netInfo, ifName)

		// Compute throughput rates from successive reads
		c.computeRates(ifName, sampleTime, &netInfo)

		interfaces = append(interfaces, netInfo)
	}

	logger.Debug("Network: Parsed %d interfaces successfully", len(interfaces))
	return interfaces, nil
}

// computeRates calculates RxBytesPerSec and TxBytesPerSec from the difference
// between the current byte counters and those recorded in the previous collection cycle.
// On the first call for a given interface, both rates remain zero.
// Counter resets and wraps (new count < previous count) are silently ignored.
// sampleTime must be captured once per scan cycle and passed consistently for all interfaces.
func (c *NetworkCollector) computeRates(ifName string, sampleTime time.Time, netInfo *dto.NetworkInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if prev, ok := c.prevStats[ifName]; ok {
		elapsed := sampleTime.Sub(prev.timestamp).Seconds()
		if elapsed > 0 {
			rx := float64(netInfo.BytesReceived) - float64(prev.bytesReceived)
			tx := float64(netInfo.BytesSent) - float64(prev.bytesSent)
			if rx >= 0 {
				netInfo.RxBytesPerSec = rx / elapsed
			}
			if tx >= 0 {
				netInfo.TxBytesPerSec = tx / elapsed
			}
		}
	}
	c.prevStats[ifName] = prevNetStats{
		bytesReceived: netInfo.BytesReceived,
		bytesSent:     netInfo.BytesSent,
		timestamp:     sampleTime,
	}
}

// pruneGoneInterfaces removes prevStats entries for interfaces no longer present in current.
func (c *NetworkCollector) pruneGoneInterfaces(current map[string]netStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for ifName := range c.prevStats {
		if _, ok := current[ifName]; !ok {
			delete(c.prevStats, ifName)
		}
	}
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
	// #nosec G304 -- path is constructed from /sys/class/net with a trusted interface name.
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (c *NetworkCollector) getIPAddress(ifName string) string {
	// Use Go's net package directly instead of spawning 'ip' command
	// This is faster and avoids process overhead
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return ""
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		// Check if it's an IP network address
		if ipNet, ok := addr.(*net.IPNet); ok {
			// Get IPv4 address only
			if ipv4 := ipNet.IP.To4(); ipv4 != nil {
				return ipv4.String()
			}
		}
	}
	return ""
}

func (c *NetworkCollector) getLinkSpeed(ifName string) int {
	path := fmt.Sprintf("/sys/class/net/%s/speed", ifName)
	// #nosec G304 -- path is constructed from /sys/class/net with a trusted interface name.
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
	// #nosec G304 -- path is constructed from /sys/class/net with a trusted interface name.
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

// enrichWithEthtool adds ethtool information to the network interface
func (c *NetworkCollector) enrichWithEthtool(netInfo *dto.NetworkInfo, ifName string) {
	// Parse ethtool information
	ethtoolInfo, err := lib.ParseEthtool(ifName)
	if err != nil {
		logger.Debug("Network: Failed to get ethtool info for %s: %v", ifName, err)
		return
	}

	// Populate ethtool fields
	netInfo.SupportedPorts = ethtoolInfo.SupportedPorts
	netInfo.SupportedLinkModes = ethtoolInfo.SupportedLinkModes
	netInfo.SupportedPauseFrame = ethtoolInfo.SupportedPauseFrame
	netInfo.SupportsAutoNeg = ethtoolInfo.SupportsAutoNeg
	netInfo.SupportedFECModes = ethtoolInfo.SupportedFECModes
	netInfo.AdvertisedLinkModes = ethtoolInfo.AdvertisedLinkModes
	netInfo.AdvertisedPauseFrame = ethtoolInfo.AdvertisedPauseFrame
	netInfo.AdvertisedAutoNeg = ethtoolInfo.AdvertisedAutoNeg
	netInfo.AdvertisedFECModes = ethtoolInfo.AdvertisedFECModes
	netInfo.Duplex = ethtoolInfo.Duplex
	netInfo.AutoNegotiation = ethtoolInfo.AutoNegotiation
	netInfo.Port = ethtoolInfo.Port
	netInfo.PHYAD = ethtoolInfo.PHYAD
	netInfo.Transceiver = ethtoolInfo.Transceiver
	netInfo.MDIX = ethtoolInfo.MDIX
	netInfo.SupportsWakeOn = ethtoolInfo.SupportsWakeOn
	netInfo.WakeOn = ethtoolInfo.WakeOn
	netInfo.MessageLevel = ethtoolInfo.MessageLevel
	netInfo.LinkDetected = ethtoolInfo.LinkDetected
	netInfo.MTU = ethtoolInfo.MTU

	logger.Debug("Network: Enriched %s with ethtool data - Duplex: %s, Link: %v", ifName, netInfo.Duplex, netInfo.LinkDetected)
}
