package controllers

import (
	"errors"
	"sort"
	"testing"
)

func TestNewServiceController(t *testing.T) {
	sc := NewServiceController()
	if sc == nil {
		t.Fatal("NewServiceController returned nil")
	}
}

func TestValidServiceNames(t *testing.T) {
	names := ValidServiceNames()

	if len(names) == 0 {
		t.Fatal("ValidServiceNames returned empty list")
	}

	// Check that all expected services are present
	expected := map[string]bool{
		"docker": true, "libvirt": true, "smb": true, "nfs": true,
		"ftp": true, "sshd": true, "nginx": true, "syslog": true,
		"ntpd": true, "avahi": true, "wireguard": true,
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected service name: %s", name)
		}
		delete(expected, name)
	}

	for name := range expected {
		t.Errorf("missing expected service name: %s", name)
	}
}

func TestValidServiceNames_NoDuplicates(t *testing.T) {
	names := ValidServiceNames()
	seen := make(map[string]bool)
	for _, name := range names {
		if seen[name] {
			t.Errorf("duplicate service name: %s", name)
		}
		seen[name] = true
	}
}

func TestValidServiceNames_Stability(t *testing.T) {
	// Test that function returns consistent results
	names1 := ValidServiceNames()
	names2 := ValidServiceNames()

	if len(names1) != len(names2) {
		t.Fatalf("ValidServiceNames returned different lengths: %d vs %d", len(names1), len(names2))
	}

	sort.Strings(names1)
	sort.Strings(names2)

	for i := range names1 {
		if names1[i] != names2[i] {
			t.Errorf("ValidServiceNames inconsistent: %s vs %s at position %d", names1[i], names2[i], i)
		}
	}
}

func TestServiceMap_AllValidNamesHaveScripts(t *testing.T) {
	// Every service returned by ValidServiceNames should have a mapping in serviceMap
	names := ValidServiceNames()
	for _, name := range names {
		if _, ok := serviceMap[name]; !ok {
			t.Errorf("service %q in ValidServiceNames but not in serviceMap", name)
		}
	}
}

func TestServiceMap_AliasesExist(t *testing.T) {
	// Check that aliases map to same scripts
	aliases := map[string]string{
		"samba": "smb",
		"ssh":   "sshd",
		"ntp":   "ntpd",
	}

	for alias, primary := range aliases {
		aliasScript, aliasOK := serviceMap[alias]
		primaryScript, primaryOK := serviceMap[primary]
		if !aliasOK {
			t.Errorf("alias %q not found in serviceMap", alias)
			continue
		}
		if !primaryOK {
			t.Errorf("primary %q not found in serviceMap", primary)
			continue
		}
		if aliasScript != primaryScript {
			t.Errorf("alias %q maps to %q, but %q maps to %q", alias, aliasScript, primary, primaryScript)
		}
	}
}

func TestValidActions(t *testing.T) {
	expected := []string{"start", "stop", "restart", "status"}
	for _, action := range expected {
		if !validActions[action] {
			t.Errorf("expected action %q to be valid", action)
		}
	}

	invalid := []string{"kill", "enable", "disable", "reload", ""}
	for _, action := range invalid {
		if validActions[action] {
			t.Errorf("action %q should not be valid", action)
		}
	}
}

func TestServiceMap_ScriptsHaveValidPaths(t *testing.T) {
	for name, script := range serviceMap {
		if script == "" {
			t.Errorf("service %q has empty script path", name)
		}
		if script[0] != '/' {
			t.Errorf("service %q script path %q is not absolute", name, script)
		}
		if !hasPrefix(script, "/etc/rc.d/rc.") {
			t.Errorf("service %q script path %q doesn't follow /etc/rc.d/rc.* pattern", name, script)
		}
	}
}

// hasPrefix is a simple helper for string prefix checking.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestGetServiceStatus_WireGuard(t *testing.T) {
	statusErr := errors.New("command failed: exit status 1")
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		// rc.wireguard status exits 1 whether or not tunnels are up.
		{"tunnels up", "Active tunnels: wg0 wg1 wg2\n", true},
		{"single tunnel", "Active tunnels: wg0", true},
		{"no tunnels", "Active tunnels: none\n", false},
		{"empty tunnel list", "Active tunnels: \n", false},
		{"script missing", "", false},
		{"unexpected output", "Usage: rc.wireguard start|stop|status\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCommand string
			var gotArgs []string
			sc := &ServiceController{statusOutput: func(command string, args ...string) (string, error) {
				gotCommand, gotArgs = command, args
				return tt.output, statusErr
			}}
			running, err := sc.GetServiceStatus("wireguard")
			if err != nil {
				t.Fatalf("GetServiceStatus returned error: %v", err)
			}
			if running != tt.want {
				t.Errorf("running = %v, want %v", running, tt.want)
			}
			if gotCommand != "/etc/rc.d/rc.wireguard" || len(gotArgs) != 1 || gotArgs[0] != "status" {
				t.Errorf("ran %q %v, want /etc/rc.d/rc.wireguard [status]", gotCommand, gotArgs)
			}
		})
	}
}

func TestGetServiceStatus_RcScriptOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   bool
	}{
		{"running", "Samba is currently running.\n", nil, true},
		{"stopped exits non-zero", "Samba is not running.\n", errors.New("exit status 1"), false},
		{"no status keyword", "something else\n", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &ServiceController{statusOutput: func(string, ...string) (string, error) {
				return tt.output, tt.err
			}}
			running, err := sc.GetServiceStatus("smb")
			if err != nil {
				t.Fatalf("GetServiceStatus returned error: %v", err)
			}
			if running != tt.want {
				t.Errorf("running = %v, want %v", running, tt.want)
			}
		})
	}
}
