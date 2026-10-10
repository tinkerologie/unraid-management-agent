package collectors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNetworkConfig(t *testing.T) {
	const cfg = `# Unraid network config
[eth0]
TYPE="access"
IPADDR="192.168.1.10"
NETMASK="255.255.255.0"
GATEWAY="192.168.1.1"
BONDING_MODE="active-backup"
BONDING_SLAVES="eth0 eth1"
BRIDGE_MEMBERS="eth0 eth2"
VLAN_ID="20"

[br0]
TYPE="bridge"
IPADDR="10.0.0.5"
`

	t.Run("reads matching interface section", func(t *testing.T) {
		got, err := parseNetworkConfig(strings.NewReader(cfg), "eth0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "access" {
			t.Errorf("Type = %q, want access", got.Type)
		}
		if got.IPAddress != "192.168.1.10" {
			t.Errorf("IPAddress = %q", got.IPAddress)
		}
		if got.Netmask != "255.255.255.0" || got.Gateway != "192.168.1.1" {
			t.Errorf("netmask/gateway wrong: %q/%q", got.Netmask, got.Gateway)
		}
		if got.BondingMode != "active-backup" {
			t.Errorf("BondingMode = %q", got.BondingMode)
		}
		if len(got.BondSlaves) != 2 || got.BondSlaves[0] != "eth0" {
			t.Errorf("BondSlaves = %v", got.BondSlaves)
		}
		if len(got.BridgeMembers) != 2 || got.BridgeMembers[1] != "eth2" {
			t.Errorf("BridgeMembers = %v", got.BridgeMembers)
		}
		if got.VLANID != 20 {
			t.Errorf("VLANID = %d, want 20", got.VLANID)
		}
	})

	t.Run("second interface section", func(t *testing.T) {
		got, err := parseNetworkConfig(strings.NewReader(cfg), "br0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Type != "bridge" || got.IPAddress != "10.0.0.5" {
			t.Errorf("br0 parsed wrong: %+v", got)
		}
	})

	t.Run("missing interface returns error", func(t *testing.T) {
		_, err := parseNetworkConfig(strings.NewReader(cfg), "eth9")
		if err == nil {
			t.Fatal("expected error for missing interface, got nil")
		}
	})

	t.Run("empty input returns not-found error", func(t *testing.T) {
		_, err := parseNetworkConfig(strings.NewReader(""), "eth0")
		if err == nil {
			t.Fatal("expected error for empty config, got nil")
		}
	})
}

func TestParseSystemSettings(t *testing.T) {
	const cfg = `# ident
NAME="Tower"
COMMENT="Media server"
MODEL="Custom"
TIMEZONE="Europe/London"
DATE_FORMAT="%Y-%m-%d"
TIME_FORMAT="%H:%M:%S"
SECURITY="user"
UNKNOWN="ignored"
`
	got, err := parseSystemSettings(strings.NewReader(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ServerName != "Tower" {
		t.Errorf("ServerName = %q, want Tower", got.ServerName)
	}
	if got.Description != "Media server" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Model != "Custom" {
		t.Errorf("Model = %q", got.Model)
	}
	if got.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q", got.Timezone)
	}
	if got.DateFormat != "%Y-%m-%d" || got.TimeFormat != "%H:%M:%S" {
		t.Errorf("date/time format wrong: %q/%q", got.DateFormat, got.TimeFormat)
	}
	if got.SecurityMode != "user" {
		t.Errorf("SecurityMode = %q", got.SecurityMode)
	}
}

func TestParseSystemSettingsEmpty(t *testing.T) {
	got, err := parseSystemSettings(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ServerName != "" {
		t.Errorf("expected empty settings, got %+v", got)
	}
}

func TestConfigCollectorGetNetworkConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "network.cfg"),
		[]byte("[eth0]\nTYPE=\"access\"\nIPADDR=\"192.168.1.10\"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	c := &ConfigCollector{bootConfigDir: dir}

	got, err := c.GetNetworkConfig("eth0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Type != "access" || got.IPAddress != "192.168.1.10" {
		t.Errorf("parsed wrong: %+v", got)
	}

	// Missing file yields the not-found error path.
	empty := &ConfigCollector{bootConfigDir: t.TempDir()}
	if _, err := empty.GetNetworkConfig("eth0"); err == nil {
		t.Error("expected error when network.cfg is absent")
	}
}

func TestConfigCollectorGetSystemSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ident.cfg"),
		[]byte("NAME=\"Tower\"\nMODEL=\"Custom\"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	c := &ConfigCollector{bootConfigDir: dir}

	got, err := c.GetSystemSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ServerName != "Tower" || got.Model != "Custom" {
		t.Errorf("parsed wrong: %+v", got)
	}

	missing := &ConfigCollector{bootConfigDir: t.TempDir()}
	if _, err := missing.GetSystemSettings(); err == nil {
		t.Error("expected error when ident.cfg is absent")
	}
}

func TestParseDockerSettings(t *testing.T) {
	const cfg = `DOCKER_ENABLED="yes"
DOCKER_IMAGE_FILE="/mnt/user/system/docker/docker.img"
DOCKER_DEFAULT_NETWORK="bridge"
DOCKER_CUSTOM_NETWORKS="br0,br1"
`
	got, err := parseDockerSettings(strings.NewReader(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.ImagePath != "/mnt/user/system/docker/docker.img" {
		t.Errorf("ImagePath = %q", got.ImagePath)
	}
	if got.DefaultNetwork != "bridge" {
		t.Errorf("DefaultNetwork = %q", got.DefaultNetwork)
	}
	if len(got.CustomNetworks) != 2 || got.CustomNetworks[1] != "br1" {
		t.Errorf("CustomNetworks = %v", got.CustomNetworks)
	}
}

func TestConfigCollectorGetDockerSettingsMissingFileDefaults(t *testing.T) {
	// A missing docker.cfg must return a disabled default, not an error.
	c := &ConfigCollector{bootConfigDir: t.TempDir()}
	got, err := c.GetDockerSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Enabled {
		t.Errorf("expected disabled default, got %+v", got)
	}
}

func TestParseVMSettings(t *testing.T) {
	const cfg = `SERVICE="enable"
PCI_DEVICES="0000:01:00.0,0000:01:00.1"
USB_DEVICES="1-1"
SOMETHING_ELSE="stored"
`
	got, err := parseVMSettings(strings.NewReader(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if len(got.PCIDevices) != 2 || got.PCIDevices[0] != "0000:01:00.0" {
		t.Errorf("PCIDevices = %v", got.PCIDevices)
	}
	if len(got.USBDevices) != 1 || got.USBDevices[0] != "1-1" {
		t.Errorf("USBDevices = %v", got.USBDevices)
	}
	if got.DefaultSettings["SOMETHING_ELSE"] != "stored" {
		t.Errorf("DefaultSettings[SOMETHING_ELSE] = %q, want stored", got.DefaultSettings["SOMETHING_ELSE"])
	}
}

func TestConfigCollectorGetVMSettingsMissingFileDefaults(t *testing.T) {
	c := &ConfigCollector{bootConfigDir: t.TempDir()}
	got, err := c.GetVMSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Enabled {
		t.Errorf("expected disabled default, got %+v", got)
	}
}

func TestParseDiskSettings(t *testing.T) {
	const cfg = `spindownDelay="30"
startArray="yes"
spinupGroups="no"
shutdownTimeout="90"
defaultFsType="xfs"
`
	got, err := parseDiskSettings(strings.NewReader(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SpindownDelay != 30 {
		t.Errorf("SpindownDelay = %d, want 30", got.SpindownDelay)
	}
	if !got.StartArray {
		t.Error("StartArray = false, want true")
	}
	if got.SpinupGroups {
		t.Error("SpinupGroups = true, want false")
	}
	if got.ShutdownTimeout != 90 {
		t.Errorf("ShutdownTimeout = %d, want 90", got.ShutdownTimeout)
	}
	if got.DefaultFsType != "xfs" {
		t.Errorf("DefaultFsType = %q, want xfs", got.DefaultFsType)
	}
}

func TestConfigCollectorGetDiskSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "disk.cfg"),
		[]byte("defaultFsType=\"zfs\"\nstartArray=\"yes\"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	c := &ConfigCollector{bootConfigDir: dir}

	got, err := c.GetDiskSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DefaultFsType != "zfs" || !got.StartArray {
		t.Errorf("parsed wrong: %+v", got)
	}

	// Disk config is required: a missing file is an error (unlike docker/vm).
	missing := &ConfigCollector{bootConfigDir: t.TempDir()}
	if _, err := missing.GetDiskSettings(); err == nil {
		t.Error("expected error when disk.cfg is absent")
	}
}
