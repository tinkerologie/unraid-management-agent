package controllers

import (
	"errors"
	"sort"
	"strings"
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
	// Every service returned by ValidServiceNames should have a mapping in
	// serviceMap, except ftp, which inetd starts (no rc script).
	names := ValidServiceNames()
	for _, name := range names {
		if name == ftpService {
			if _, ok := serviceMap[name]; ok {
				t.Errorf("ftp must not map to an rc script")
			}
			continue
		}
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

// TestGetServiceStatus_FTP checks FTP status comes from the port 21 listener, not an rc script.
func TestGetServiceStatus_FTP(t *testing.T) {
	const header = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	listening := header + "   1: 00000000:0015 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 52536051 1 0 100 0 0 10 0\n"
	tests := []struct {
		name    string
		service string
		tcp     string
		want    bool
	}{
		{"inetd listening on port 21", "ftp", listening, true},
		{"upper case name", "FTP", listening, true},
		{"nothing on port 21", "ftp", header, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &ServiceController{
				statusOutput: func(string, ...string) (string, error) {
					t.Fatal("ftp status must not run an rc script")
					return "", nil
				},
				readFile: func(name string) ([]byte, error) {
					if name == "/proc/net/tcp" {
						return []byte(tt.tcp), nil
					}
					return nil, errors.New("no such file")
				},
			}
			running, err := sc.GetServiceStatus(tt.service)
			if err != nil {
				t.Fatalf("GetServiceStatus returned error: %v", err)
			}
			if running != tt.want {
				t.Errorf("running = %v, want %v", running, tt.want)
			}
		})
	}
}

// TestGetServiceStatus_FTPDefaultReader runs the FTP status check with the real file reader.
func TestGetServiceStatus_FTPDefaultReader(t *testing.T) {
	// Reads the real /proc/net/tcp tables (absent on macOS); only checks
	// that no rc script is needed and no error is returned.
	if _, err := NewServiceController().GetServiceStatus("ftp"); err != nil {
		t.Fatalf("GetServiceStatus(ftp) returned error: %v", err)
	}
}

// TestFTPActionsUnsupported checks FTP start/stop/restart return ErrServiceActionUnsupported.
func TestFTPActionsUnsupported(t *testing.T) {
	sc := NewServiceController()
	for name, action := range map[string]func(string) error{
		"start":   sc.StartService,
		"stop":    sc.StopService,
		"restart": sc.RestartService,
	} {
		t.Run(name, func(t *testing.T) {
			err := action("FTP")
			if !errors.Is(err, ErrServiceActionUnsupported) {
				t.Fatalf("err = %v, want ErrServiceActionUnsupported", err)
			}
			if !strings.Contains(err.Error(), "Settings > FTP Server") {
				t.Errorf("error %q does not point to Settings > FTP Server", err)
			}
		})
	}
}

func TestServiceControllerActionsWithMockExec(t *testing.T) {
	var executedCmd string
	var executedAction string
	sc := &ServiceController{
		exec: func(command string, args ...string) ([]string, error) {
			executedCmd = command
			if len(args) > 0 {
				executedAction = args[0]
			}
			return nil, nil
		},
	}

	t.Run("start service success", func(t *testing.T) {
		err := sc.StartService("docker")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if executedCmd != "/etc/rc.d/rc.docker" || executedAction != "start" {
			t.Errorf("exec = (%q, %q), want (/etc/rc.d/rc.docker, start)", executedCmd, executedAction)
		}
	})

	t.Run("stop service success", func(t *testing.T) {
		err := sc.StopService("samba")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if executedCmd != "/etc/rc.d/rc.samba" || executedAction != "stop" {
			t.Errorf("exec = (%q, %q), want (/etc/rc.d/rc.samba, stop)", executedCmd, executedAction)
		}
	})

	t.Run("restart service success", func(t *testing.T) {
		err := sc.RestartService("wireguard")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if executedCmd != "/etc/rc.d/rc.wireguard" || executedAction != "restart" {
			t.Errorf("exec = (%q, %q), want (/etc/rc.d/rc.wireguard, restart)", executedCmd, executedAction)
		}
	})

	t.Run("unknown service error", func(t *testing.T) {
		err := sc.StartService("nonexistent-service")
		if err == nil || !strings.Contains(err.Error(), "unknown service") {
			t.Errorf("expected unknown service error, got: %v", err)
		}
	})

	t.Run("exec error propagation", func(t *testing.T) {
		failSc := &ServiceController{
			exec: func(command string, args ...string) ([]string, error) {
				return nil, errors.New("exec crashed")
			},
		}
		err := failSc.StartService("nginx")
		if err == nil || !strings.Contains(err.Error(), "exec crashed") {
			t.Errorf("expected exec error propagation, got: %v", err)
		}
	})
}
