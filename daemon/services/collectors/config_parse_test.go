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
