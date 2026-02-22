package controllers

import (
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
