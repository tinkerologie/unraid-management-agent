package main

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func TestApplyFileConfigStorageTopologyInterval(t *testing.T) {
	orig := cli.IntervalStorageTopology
	t.Cleanup(func() { cli.IntervalStorageTopology = orig })

	interval := 900
	applyFileConfig(&domain.FileConfig{Intervals: &domain.FileConfigIntervals{StorageTopology: &interval}})
	if cli.IntervalStorageTopology != 900 {
		t.Errorf("IntervalStorageTopology = %d, want 900", cli.IntervalStorageTopology)
	}
	if !validCollectorNames["storage_topology"] {
		t.Error("storage_topology must be a valid collector name for --disable-collectors")
	}
}

func TestApplyFileConfigNICInterval(t *testing.T) {
	orig := cli.IntervalNIC
	t.Cleanup(func() { cli.IntervalNIC = orig })

	interval := 120
	applyFileConfig(&domain.FileConfig{Intervals: &domain.FileConfigIntervals{NIC: &interval}})
	if cli.IntervalNIC != 120 {
		t.Errorf("IntervalNIC = %d, want 120", cli.IntervalNIC)
	}
	if !validCollectorNames["nic"] {
		t.Error("nic must be a valid collector name for --disable-collectors")
	}
}

func TestPSUFileConfigAndExplicitDisable(t *testing.T) {
	orig := cli.IntervalPSU
	t.Cleanup(func() { cli.IntervalPSU = orig })
	for _, interval := range []int{120, 0} {
		applyFileConfig(&domain.FileConfig{Intervals: &domain.FileConfigIntervals{PSU: &interval}})
		if cli.IntervalPSU != interval {
			t.Fatalf("PSU interval = %d want %d", cli.IntervalPSU, interval)
		}
	}
	if !validCollectorNames["psu"] || !isHardwareGated("psu") {
		t.Fatal("PSU is missing from configuration registries")
	}
	if auto := resolveAutoCollectors("psu", map[string]bool{"psu": true}, map[string]int{"psu": 0}, nil); auto["psu"] {
		t.Fatal("explicit disable must win over auto")
	}
}

func TestApplyFileConfigAllowUnauthenticatedWrites(t *testing.T) {
	orig := cli.AllowUnauthenticatedWrites
	t.Cleanup(func() { cli.AllowUnauthenticatedWrites = orig })

	cli.AllowUnauthenticatedWrites = false
	applyFileConfig(&domain.FileConfig{})
	if cli.AllowUnauthenticatedWrites {
		t.Error("AllowUnauthenticatedWrites must stay false when the file does not set it")
	}

	allow := true
	applyFileConfig(&domain.FileConfig{AllowUnauthenticatedWrites: &allow})
	if !cli.AllowUnauthenticatedWrites {
		t.Error("AllowUnauthenticatedWrites = false, want true from the config file")
	}
}

func TestWriteAccessWarning(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		allow   bool
		wantSub string
	}{
		{"token set", "secret-token", false, ""},
		{"token set with opt-out", "secret-token", true, ""},
		{"no token locks writes", "", false, "network write access is disabled"},
		{"whitespace token locks writes", "  ", false, "network write access is disabled"},
		{"no token with opt-out warns", "", true, "anyone on the network can control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeAccessWarning(tt.token, tt.allow)
			if tt.wantSub == "" {
				if got != "" {
					t.Errorf("writeAccessWarning() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("writeAccessWarning() = %q, want it to contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestApplyWriteAccess(t *testing.T) {
	for _, allow := range []bool{false, true} {
		appCtx := &domain.Context{}
		applyWriteAccess(appCtx, "", allow)
		if appCtx.AllowUnauthenticatedWrites != allow {
			t.Errorf("AllowUnauthenticatedWrites = %v, want %v", appCtx.AllowUnauthenticatedWrites, allow)
		}
		if appCtx.WritesLocked() == allow {
			t.Errorf("WritesLocked() = %v with no token and opt-out %v", appCtx.WritesLocked(), allow)
		}
	}
}

func TestWANIPLookupSetting(t *testing.T) {
	parse := func(t *testing.T, args ...string) bool {
		t.Helper()
		c := cli
		parser, err := kong.New(&c, kong.Exit(func(int) { t.Fatal("kong exited") }))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(append(args, "boot")); err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		return c.WANIPLookup
	}

	// Unset WAN_IP_LOOKUP for the default case; t.Setenv restores it afterwards.
	t.Setenv("WAN_IP_LOOKUP", "")
	if err := os.Unsetenv("WAN_IP_LOOKUP"); err != nil {
		t.Fatal(err)
	}
	if !parse(t) {
		t.Error("WAN IP lookup must default to enabled")
	}
	if parse(t, "--wan-ip-lookup=false") {
		t.Error("--wan-ip-lookup=false must disable the lookup")
	}
	t.Setenv("WAN_IP_LOOKUP", "false")
	if parse(t) {
		t.Error("WAN_IP_LOOKUP=false must disable the lookup")
	}

	orig := cli.WANIPLookup
	t.Cleanup(func() { cli.WANIPLookup = orig })
	cli.WANIPLookup = true
	applyFileConfig(&domain.FileConfig{})
	if !cli.WANIPLookup {
		t.Error("an omitted wan_ip_lookup must keep the parsed value")
	}
	off := false
	applyFileConfig(&domain.FileConfig{WANIPLookup: &off})
	if cli.WANIPLookup {
		t.Error("wan_ip_lookup: false in the config file must disable the lookup")
	}
}

func TestApplyWANIPLookup(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		appCtx := &domain.Context{}
		applyWANIPLookup(appCtx, enabled)
		if appCtx.WANIPLookup != enabled {
			t.Errorf("WANIPLookup = %v, want %v", appCtx.WANIPLookup, enabled)
		}
	}
}

func TestBuildVMGuestConfig(t *testing.T) {
	ps := []string{"powershell.exe", "-File", "rtss.ps1", "fps"}
	interval := func(n int) *int { return &n }

	t.Run("nothing configured", func(t *testing.T) {
		got := buildVMGuestConfig("", nil)
		if len(got.VMs) != 0 || len(got.Sensors) != 0 || got.Interval != vmGuestDefaultInterval {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("flag VMs, no file", func(t *testing.T) {
		got := buildVMGuestConfig(" Windows 11 ,,bad/name,Windows 11,ubuntu", nil)
		if !reflect.DeepEqual(got.VMs, []string{"Windows 11", "ubuntu"}) {
			t.Errorf("VMs = %q", got.VMs)
		}
	})

	t.Run("sensor VM names are trimmed like the VM list", func(t *testing.T) {
		got := buildVMGuestConfig("", &domain.FileConfigVMGuest{
			VMs:     []string{" Windows 11"},
			Sensors: []domain.FileConfigVMGuestSensor{{VM: " Windows 11 ", Name: "fps", Command: ps}},
		})
		if len(got.Sensors) != 1 || got.Sensors[0].VM != "Windows 11" {
			t.Errorf("Sensors = %+v", got.Sensors)
		}
	})

	t.Run("file with sensors", func(t *testing.T) {
		got := buildVMGuestConfig("", &domain.FileConfigVMGuest{
			VMs:      []string{"Windows 11"},
			Interval: interval(3),
			Sensors: []domain.FileConfigVMGuestSensor{
				{VM: "Windows 11", Name: "fps", Command: ps, Interval: 1},
				{VM: "Windows 11", Name: "game", Command: ps, JSON: true},
				{VM: "Windows 11", Name: "fps", Command: ps},
				{VM: "Windows 11", Name: "game_score", Command: ps},
				{VM: "Windows 11", Name: "gamer", Command: ps},
				{VM: "Other", Name: "x", Command: ps},
				{VM: "Windows 11", Name: "Bad Name", Command: ps},
				{VM: "Windows 11", Name: "empty"},
				{VM: "Windows 11", Name: "blank", Command: []string{""}},
				{VM: "Windows 11", Name: "plainattr", Command: ps, Attributes: map[string]string{"a": "b"}},
				{VM: "Windows 11", Name: "emptykey", Command: ps, JSON: true, Attributes: map[string]string{" ": "b"}},
				{VM: "Windows 11", Name: "emptytarget", Command: ps, JSON: true, Attributes: map[string]string{"a": ""}},
				{VM: "Windows 11", Name: "chain", Command: ps, JSON: true, Attributes: map[string]string{"a": "b", "b": "c"}},
				{VM: "Windows 11", Name: "selfattr", Command: ps, JSON: true, Attributes: map[string]string{"a": "a"}},
				{VM: "Windows 11", Name: "okattr", Command: ps, JSON: true, Attributes: map[string]string{"graph": "avg"}},
			},
		})
		if got.Interval != vmGuestMinInterval {
			t.Errorf("Interval = %d, want %d", got.Interval, vmGuestMinInterval)
		}
		want := []domain.VMGuestSensorConfig{
			{VM: "Windows 11", Name: "fps", Command: ps, Interval: vmGuestMinSensorSecs},
			{VM: "Windows 11", Name: "game", Command: ps, Interval: vmGuestDefaultInterval, JSON: true},
			{VM: "Windows 11", Name: "gamer", Command: ps, Interval: vmGuestDefaultInterval},
			{VM: "Windows 11", Name: "okattr", Command: ps, Interval: vmGuestDefaultInterval, JSON: true, Attributes: map[string]string{"graph": "avg"}},
		}
		if !reflect.DeepEqual(got.Sensors, want) {
			t.Errorf("Sensors = %+v, want %+v", got.Sensors, want)
		}
	})

	t.Run("attributes are normalised and collisions rejected", func(t *testing.T) {
		mk := func(name string, attrs map[string]string) domain.FileConfigVMGuestSensor {
			return domain.FileConfigVMGuestSensor{VM: "Windows 11", Name: name, Command: ps, JSON: true, Attributes: attrs}
		}
		got := buildVMGuestConfig("", &domain.FileConfigVMGuest{
			VMs: []string{"Windows 11"},
			Sensors: []domain.FileConfigVMGuestSensor{
				mk("ok", map[string]string{"Graph-X": "AVG"}),
				mk("casedup", map[string]string{"Graph": "avg", "graph": "avg"}),
				mk("punctdup", map[string]string{"a-b": "avg", "a_b": "avg"}),
				mk("targetattr", map[string]string{"Avg": "x", "a": "AVG"}),
			},
		})
		want := []domain.VMGuestSensorConfig{
			{VM: "Windows 11", Name: "ok", Command: ps, Interval: vmGuestDefaultInterval, JSON: true, Attributes: map[string]string{"graph_x": "avg"}},
		}
		if !reflect.DeepEqual(got.Sensors, want) {
			t.Errorf("Sensors = %+v, want %+v", got.Sensors, want)
		}
	})

	t.Run("flag replaces file VMs", func(t *testing.T) {
		got := buildVMGuestConfig("ubuntu", &domain.FileConfigVMGuest{
			VMs:      []string{"Windows 11"},
			Interval: interval(60),
			Sensors:  []domain.FileConfigVMGuestSensor{{VM: "Windows 11", Name: "fps", Command: ps}},
		})
		if !reflect.DeepEqual(got.VMs, []string{"ubuntu"}) || len(got.Sensors) != 0 || got.Interval != 60 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestVMGuestFileConfig(t *testing.T) {
	if vmGuestFileConfig(nil) != nil {
		t.Error("want nil for nil file config")
	}
	section := &domain.FileConfigVMGuest{VMs: []string{"a"}}
	if vmGuestFileConfig(&domain.FileConfig{VMGuestAgent: section}) != section {
		t.Error("want the vm_guest_agent section")
	}
}

// TestConfigFilePrecedence guards #257: a config file fills only settings not
// set on the command line or through a non-empty environment variable.
func TestConfigFilePrecedence(t *testing.T) {
	yes, no := true, false
	yamlToken := "yaml-token-1234567890"
	fileInterval := 300
	envs := []string{"READ_ONLY", "API_TOKEN", "ALLOW_UNAUTHENTICATED_WRITES", "WAN_IP_LOOKUP", "INTERVAL_NIC"}

	tests := []struct {
		name  string
		args  []string
		env   map[string]string
		file  domain.FileConfig
		check func(t *testing.T)
	}{
		{
			name: "file overrides default", file: domain.FileConfig{ReadOnly: &yes, AllowUnauthenticatedWrites: &yes},
			check: func(t *testing.T) {
				if !cli.ReadOnly || !cli.AllowUnauthenticatedWrites {
					t.Errorf("ReadOnly=%v AllowUnauthenticatedWrites=%v, want both true from the file", cli.ReadOnly, cli.AllowUnauthenticatedWrites)
				}
			},
		},
		{
			name: "env beats file", env: map[string]string{"READ_ONLY": "true", "WAN_IP_LOOKUP": "false"},
			file: domain.FileConfig{ReadOnly: &no, WANIPLookup: &yes},
			check: func(t *testing.T) {
				if !cli.ReadOnly || cli.WANIPLookup {
					t.Errorf("ReadOnly=%v WANIPLookup=%v, want the env values true/false", cli.ReadOnly, cli.WANIPLookup)
				}
			},
		},
		{
			name: "flag beats file", args: []string{"--read-only=false"}, file: domain.FileConfig{ReadOnly: &yes},
			check: func(t *testing.T) {
				if cli.ReadOnly {
					t.Error("ReadOnly=true, want the --read-only=false flag to win over the file")
				}
			},
		},
		{
			name: "non-empty env token beats file", env: map[string]string{"API_TOKEN": "env-token-1234567890"},
			file: domain.FileConfig{APIToken: &yamlToken},
			check: func(t *testing.T) {
				if cli.APIToken != "env-token-1234567890" {
					t.Errorf("APIToken=%q, want the env token", cli.APIToken)
				}
			},
		},
		{
			name: "int flag beats file", args: []string{"--interval-nic=120"},
			file: domain.FileConfig{Intervals: &domain.FileConfigIntervals{NIC: &fileInterval}},
			check: func(t *testing.T) {
				if cli.IntervalNIC != 120 {
					t.Errorf("IntervalNIC=%d, want the --interval-nic=120 flag", cli.IntervalNIC)
				}
			},
		},
		{
			name: "int env beats file", env: map[string]string{"INTERVAL_NIC": "90"},
			file: domain.FileConfig{Intervals: &domain.FileConfigIntervals{NIC: &fileInterval}},
			check: func(t *testing.T) {
				if cli.IntervalNIC != 90 {
					t.Errorf("IntervalNIC=%d, want the INTERVAL_NIC=90 env value", cli.IntervalNIC)
				}
			},
		},
		{
			name: "int file overrides default",
			file: domain.FileConfig{Intervals: &domain.FileConfigIntervals{NIC: &fileInterval}},
			check: func(t *testing.T) {
				if cli.IntervalNIC != fileInterval {
					t.Errorf("IntervalNIC=%d, want the file value %d", cli.IntervalNIC, fileInterval)
				}
			},
		},
		{
			name: "empty env token does not discard the file token", env: map[string]string{"API_TOKEN": ""},
			file: domain.FileConfig{APIToken: &yamlToken},
			check: func(t *testing.T) {
				if cli.APIToken != yamlToken {
					t.Errorf("APIToken=%q, want the file token (an empty env var is not an explicit setting)", cli.APIToken)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved, savedExplicit := cli, explicitFlags
			t.Cleanup(func() { cli, explicitFlags = saved, savedExplicit })
			for _, e := range envs {
				t.Setenv(e, "")
				if err := os.Unsetenv(e); err != nil {
					t.Fatal(err)
				}
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			// The same startup path as main: parseCLI, then the config file.
			if _, _, err := parseCLI(append(tt.args, "boot"), kong.Exit(func(int) { t.Fatal("kong exited") })); err != nil {
				t.Fatal(err)
			}
			applyFileConfig(&tt.file)
			tt.check(t)
		})
	}
}

// TestEmptyBoolEnvDoesNotBlockStartup checks an empty boolean env var is
// treated as unset instead of failing Kong's parse (#257).
func TestEmptyBoolEnvDoesNotBlockStartup(t *testing.T) {
	saved, savedExplicit := cli, explicitFlags
	t.Cleanup(func() { cli, explicitFlags = saved, savedExplicit })
	t.Setenv("READ_ONLY", "")

	if _, _, err := parseCLI([]string{"boot"}, kong.Exit(func(int) { t.Fatal("kong exited") })); err != nil {
		t.Fatalf("parse with empty READ_ONLY failed: %v", err)
	}
	if _, ok := os.LookupEnv("READ_ONLY"); ok {
		t.Error("empty READ_ONLY was not unset")
	}
	yes := true
	applyFileConfig(&domain.FileConfig{ReadOnly: &yes})
	if !cli.ReadOnly {
		t.Error("read_only from the config file must apply when READ_ONLY is empty")
	}
}

// TestHardwareSpecificCollectorsDisabledByDefault guards #260: the network
// card and SAS storage topology collectors need hardware most servers lack.
func TestHardwareSpecificCollectorsDisabledByDefault(t *testing.T) {
	for _, env := range []string{"INTERVAL_NIC", "INTERVAL_STORAGE_TOPOLOGY"} {
		t.Setenv(env, "")
		if err := os.Unsetenv(env); err != nil {
			t.Fatal(err)
		}
	}
	c := cli
	parser, err := kong.New(&c, kong.Exit(func(int) { t.Fatal("kong exited") }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"boot"}); err != nil {
		t.Fatal(err)
	}
	if c.IntervalNIC != 0 || c.IntervalStorageTopology != 0 {
		t.Errorf("defaults: INTERVAL_NIC=%d INTERVAL_STORAGE_TOPOLOGY=%d, want both 0 (disabled)",
			c.IntervalNIC, c.IntervalStorageTopology)
	}

	t.Setenv("INTERVAL_NIC", "60")
	t.Setenv("INTERVAL_STORAGE_TOPOLOGY", "300")
	c = cli
	parser, err = kong.New(&c, kong.Exit(func(int) { t.Fatal("kong exited") }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"boot"}); err != nil {
		t.Fatal(err)
	}
	if c.IntervalNIC != 60 || c.IntervalStorageTopology != 300 {
		t.Errorf("enabled via env: got %d/%d, want 60/300", c.IntervalNIC, c.IntervalStorageTopology)
	}
}

// TestParseCLIRejectsInvalidValue checks parse errors are returned (main
// reports them with FatalIfErrorf) and no explicit flags are recorded.
func TestParseCLIRejectsInvalidValue(t *testing.T) {
	saved, savedExplicit := cli, explicitFlags
	t.Cleanup(func() { cli, explicitFlags = saved, savedExplicit })
	explicitFlags = nil

	_, _, err := parseCLI([]string{"--interval-nic=abc", "boot"}, kong.Exit(func(int) { t.Fatal("kong exited") }))
	if err == nil {
		t.Fatal("parseCLI accepted --interval-nic=abc")
	}
	if explicitFlags != nil {
		t.Error("explicit flags were recorded for a failed parse")
	}
}

// TestResolveAutoCollectors covers parsing and the precedence of the automatic-mode list.
func TestResolveAutoCollectors(t *testing.T) {
	zero := map[string]int{"nic": 0, "storage_topology": 0, "gpu": 0, "ups": 0, "nut": 0, "zfs": 0}
	tests := []struct {
		name      string
		csv       string
		disabled  map[string]bool
		intervals map[string]int
		fromFile  map[string]bool
		want      []string
	}{
		{name: "empty keeps current behaviour", csv: "", want: nil},
		{name: "parses, trims and lowercases", csv: " NIC, storage_topology ,,gpu", want: []string{"gpu", "nic", "storage_topology"}},
		{name: "ignores collectors that are not hardware-gated", csv: "nic,system,docker,bogus", want: []string{"nic"}},
		{name: "disabled list wins", csv: "nic,ups", disabled: map[string]bool{"ups": true}, want: []string{"nic"}},
		{name: "config.yml interval wins", csv: "nic,zfs", fromFile: map[string]bool{"zfs": true}, want: []string{"nic"}},
		{name: "explicit nonzero interval wins", csv: "nic,gpu", intervals: map[string]int{"nic": 0, "gpu": 60}, want: []string{"nic"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervals := tt.intervals
			if intervals == nil {
				intervals = zero
			}
			got := resolveAutoCollectors(tt.csv, tt.disabled, intervals, tt.fromFile)
			var names []string
			for name := range got {
				names = append(names, name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.want) {
				t.Errorf("got %v, want %v", names, tt.want)
			}
		})
	}
}

// TestApplyFileConfigAutoCollectors checks config.yml auto_collectors and interval tracking.
func TestApplyFileConfigAutoCollectors(t *testing.T) {
	origAuto, origNIC := cli.AutoCollectors, cli.IntervalNIC
	origFile := fileConfigIntervals
	t.Cleanup(func() {
		cli.AutoCollectors, cli.IntervalNIC = origAuto, origNIC
		fileConfigIntervals = origFile
	})
	fileConfigIntervals = map[string]bool{}

	list, nic := "nic,gpu", 0
	applyFileConfig(&domain.FileConfig{AutoCollectors: &list, Intervals: &domain.FileConfigIntervals{NIC: &nic}})
	if cli.AutoCollectors != "nic,gpu" {
		t.Errorf("AutoCollectors = %q, want the file value", cli.AutoCollectors)
	}
	if !fileConfigIntervals["nic"] || fileConfigIntervals["gpu"] {
		t.Errorf("fileConfigIntervals = %v, want only nic", fileConfigIntervals)
	}
	got := resolveAutoCollectors(cli.AutoCollectors, nil, map[string]int{"nic": 0, "gpu": 0}, fileConfigIntervals)
	if got["nic"] || !got["gpu"] {
		t.Errorf("auto = %v: an explicit config.yml interval (even 0) must take nic out of automatic mode", got)
	}
}

// TestHardwareGatedCollectors checks every hardware collector is gated except disabled ones.
func TestHardwareGatedCollectors(t *testing.T) {
	got := hardwareGatedCollectors(map[string]bool{"ups": true, "docker": true})
	var names []string
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)
	want := []string{"gpu", "nic", "nut", "psu", "storage_topology", "zfs"}
	if !slices.Equal(names, want) {
		t.Errorf("gated = %v, want %v (every hardware collector except the disabled ups)", names, want)
	}
}

// TestFanControlIntervalDefault guards #277: fan control status defaults to
// 60s, and explicit values (including 5 and 0) are kept.
func TestFanControlIntervalDefault(t *testing.T) {
	if constants.IntervalFanControl != 60 {
		t.Fatalf("constants.IntervalFanControl = %d, want 60", constants.IntervalFanControl)
	}
	tests := []struct {
		name string
		env  string
		args []string
		want int
	}{
		{name: "default", want: constants.IntervalFanControl},
		{name: "env 5", env: "5", want: 5},
		{name: "env 0 disables", env: "0", want: 0},
		{name: "flag 5", args: []string{"--interval-fan-control=5"}, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("INTERVAL_FAN_CONTROL", tt.env)
			if tt.env == "" {
				if err := os.Unsetenv("INTERVAL_FAN_CONTROL"); err != nil {
					t.Fatal(err)
				}
			}
			c := cli
			parser, err := kong.New(&c, kong.Exit(func(int) { t.Fatal("kong exited") }))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parser.Parse(append(tt.args, "boot")); err != nil {
				t.Fatal(err)
			}
			if c.IntervalFanControl != tt.want {
				t.Errorf("IntervalFanControl = %d, want %d", c.IntervalFanControl, tt.want)
			}
		})
	}
}
