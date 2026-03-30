package lib

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// NICOffloadSettings contains NIC offload feature states parsed from `ethtool -k`.
type NICOffloadSettings struct {
	RxChecksumming        string `json:"rx_checksumming"`
	TxChecksumming        string `json:"tx_checksumming"`
	ScatterGather         string `json:"scatter_gather"`
	TCPSegmentOffload     string `json:"tcp_segmentation_offload"`
	GenericSegmentOffload string `json:"generic_segmentation_offload"`
	GenericReceiveOffload string `json:"generic_receive_offload"`
	LargeReceiveOffload   string `json:"large_receive_offload"`
	RxVlanOffload         string `json:"rx_vlan_offload"`
	TxVlanOffload         string `json:"tx_vlan_offload"`
}

// ParseNICOffloads parses `ethtool -k <interface>` output for offload settings.
func ParseNICOffloads(ifName string) (*NICOffloadSettings, error) {
	if !CommandExists("ethtool") {
		return nil, errors.New("ethtool command not found")
	}

	output, err := ExecCommandOutput("ethtool", "-k", ifName)
	if err != nil {
		return nil, fmt.Errorf("running ethtool -k: %w", err)
	}

	return parseNICOffloadsOutput(output), nil
}

// parseNICOffloadsOutput parses raw ethtool -k output into NICOffloadSettings.
func parseNICOffloadsOutput(output string) *NICOffloadSettings {
	settings := &NICOffloadSettings{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Strip "[fixed]", "[not requested]" etc. suffixes
		if idx := strings.Index(value, " "); idx > 0 {
			value = value[:idx]
		}

		switch key {
		case "rx-checksumming":
			settings.RxChecksumming = value
		case "tx-checksumming":
			settings.TxChecksumming = value
		case "scatter-gather":
			settings.ScatterGather = value
		case "tcp-segmentation-offload":
			settings.TCPSegmentOffload = value
		case "generic-segmentation-offload":
			settings.GenericSegmentOffload = value
		case "generic-receive-offload":
			settings.GenericReceiveOffload = value
		case "large-receive-offload":
			settings.LargeReceiveOffload = value
		case "rx-vlan-offload":
			settings.RxVlanOffload = value
		case "tx-vlan-offload":
			settings.TxVlanOffload = value
		}
	}

	return settings
}

// NICRingBuffers contains NIC ring buffer sizes parsed from `ethtool -g`.
type NICRingBuffers struct {
	RxMax     int `json:"rx_max"`
	RxCurrent int `json:"rx_current"`
	TxMax     int `json:"tx_max"`
	TxCurrent int `json:"tx_current"`
}

// ParseNICRingBuffers parses `ethtool -g <interface>` output for ring buffer sizes.
func ParseNICRingBuffers(ifName string) (*NICRingBuffers, error) {
	if !CommandExists("ethtool") {
		return nil, errors.New("ethtool command not found")
	}

	output, err := ExecCommandOutput("ethtool", "-g", ifName)
	if err != nil {
		return nil, fmt.Errorf("running ethtool -g: %w", err)
	}

	return parseNICRingBuffersOutput(output), nil
}

// parseNICRingBuffersOutput parses raw ethtool -g output into NICRingBuffers.
// Handles two formats: with explicit section headers ("Pre-set maximums:" /
// "Current hardware settings:") and without (positional — first RX/TX are
// max, second are current).
func parseNICRingBuffersOutput(output string) *NICRingBuffers {
	buffers := &NICRingBuffers{}
	lines := strings.Split(output, "\n")

	// Detect whether section headers are present.
	hasHeaders := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Pre-set maximums") {
			hasHeaders = true
			break
		}
	}

	if hasHeaders {
		parseRingBuffersWithHeaders(lines, buffers)
	} else {
		parseRingBuffersPositional(lines, buffers)
	}

	return buffers
}

// parseRingBuffersWithHeaders parses ethtool -g output that contains explicit
// "Pre-set maximums:" and "Current hardware settings:" section headers.
func parseRingBuffersWithHeaders(lines []string, buffers *NICRingBuffers) {
	inMax := false
	inCurrent := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Pre-set maximums") {
			inMax = true
			inCurrent = false
			continue
		}
		if strings.HasPrefix(trimmed, "Current hardware settings") {
			inMax = false
			inCurrent = true
			continue
		}

		key, val, ok := parseRingBufferLine(trimmed)
		if !ok {
			continue
		}

		switch {
		case key == "RX" && inMax:
			buffers.RxMax = val
		case key == "TX" && inMax:
			buffers.TxMax = val
		case key == "RX" && inCurrent:
			buffers.RxCurrent = val
		case key == "TX" && inCurrent:
			buffers.TxCurrent = val
		}
	}
}

// parseRingBuffersPositional parses ethtool -g output without section headers.
// First occurrence of RX/TX = maximums, second = current settings.
func parseRingBuffersPositional(lines []string, buffers *NICRingBuffers) {
	rxCount := 0
	txCount := 0

	for _, line := range lines {
		key, val, ok := parseRingBufferLine(strings.TrimSpace(line))
		if !ok {
			continue
		}

		switch key {
		case "RX":
			rxCount++
			switch rxCount {
			case 1:
				buffers.RxMax = val
			case 2:
				buffers.RxCurrent = val
			}
		case "TX":
			txCount++
			switch txCount {
			case 1:
				buffers.TxMax = val
			case 2:
				buffers.TxCurrent = val
			}
		}
	}
}

// parseRingBufferLine extracts the key and integer value from a single line.
// Returns false if the line is not a valid "KEY: VALUE" pair with a numeric value.
func parseRingBufferLine(trimmed string) (string, int, bool) {
	if trimmed == "" || !strings.Contains(trimmed, ":") {
		return "", 0, false
	}
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	key := strings.TrimSpace(parts[0])
	val, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", 0, false
	}
	return key, val, true
}
