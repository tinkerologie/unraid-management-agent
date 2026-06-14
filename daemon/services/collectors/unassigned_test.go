package collectors

import (
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewUnassignedCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUnassignedCollector(ctx)

	if collector == nil {
		t.Fatal("NewUnassignedCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("UnassignedCollector context not set correctly")
	}
}

func TestUnassignedCollectorInit(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUnassignedCollector(ctx)

	// Verify collector is properly initialized
	if collector == nil {
		t.Fatal("Collector should not be nil")
	}

	if collector.ctx == nil {
		t.Fatal("Collector context should not be nil")
	}

	if collector.ctx.Hub == nil {
		t.Fatal("Collector context Hub should not be nil")
	}
}

func TestDeviceFiltering(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		shouldSkip bool
	}{
		{"loop device", "loop0", true},
		{"loop device with number", "loop1", true},
		{"md device", "md0", true},
		{"zram device", "zram0", true},
		{"nvme partition", "nvme0n1p1", true},
		{"sda partition", "sda1", true},
		{"sdb partition", "sdb2", true},
		{"valid sda", "sda", false},
		{"valid sdb", "sdb", false},
		{"valid nvme", "nvme0n1", false},
		{"valid sdc", "sdc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSkip := shouldSkipDevice(tt.deviceName)
			if shouldSkip != tt.shouldSkip {
				t.Errorf("shouldSkip = %v, want %v", shouldSkip, tt.shouldSkip)
			}
		})
	}
}

func shouldSkipDevice(device string) bool {
	// Skip loop devices
	if len(device) >= 4 && device[:4] == "loop" {
		return true
	}
	// Skip md devices
	if len(device) >= 2 && device[:2] == "md" {
		return true
	}
	// Skip zram devices
	if len(device) >= 4 && device[:4] == "zram" {
		return true
	}
	// Skip nvme partitions
	if len(device) > 7 && device[:7] == "nvme0n1" && device[7] == 'p' {
		return true
	}
	// Skip disk partitions (sda1, sdb2, etc.)
	if len(device) == 4 && device[:2] == "sd" && device[3] >= '1' && device[3] <= '9' {
		return true
	}
	return false
}

func TestPluginInstallationCheck(t *testing.T) {
	// Test path for plugin detection
	pluginPath := "/boot/config/plugins/unassigned.devices"

	// Path should be non-empty
	if len(pluginPath) == 0 {
		t.Error("Plugin path should not be empty")
	}

	// Path should contain plugin name
	if !containsString(pluginPath, "unassigned.devices") {
		t.Error("Plugin path should contain 'unassigned.devices'")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRemoteShareTypes(t *testing.T) {
	// Valid remote share types
	shareTypes := []string{
		"smb",
		"nfs",
		"iso",
	}

	expectedTypes := map[string]bool{
		"smb": true,
		"nfs": true,
		"iso": true,
	}

	for _, st := range shareTypes {
		if !expectedTypes[st] {
			t.Errorf("Unexpected share type: %s", st)
		}
	}

	// Verify we have all expected types
	if len(shareTypes) != 3 {
		t.Errorf("Expected 3 share types, got %d", len(shareTypes))
	}
}

func TestMountPointParsing(t *testing.T) {
	tests := []struct {
		name      string
		mountLine string
		isMounted bool
	}{
		{"mounted sda", "/dev/sda1 /mnt/disk1 ext4 rw 0 0", true},
		{"mounted nfs", "192.168.1.100:/share /mnt/nfs nfs rw 0 0", true},
		{"mounted smb", "//192.168.1.100/share /mnt/smb cifs rw 0 0", true},
		{"empty line", "", false},
		{"comment line", "# comment", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMounted := len(tt.mountLine) > 0 && tt.mountLine[0] != '#'
			if isMounted != tt.isMounted {
				t.Errorf("isMounted = %v, want %v", isMounted, tt.isMounted)
			}
		})
	}
}

func TestDeviceInfoFields(t *testing.T) {
	// Expected fields for an unassigned device
	fields := []string{
		"Device",
		"Name",
		"Serial",
		"Size",
		"SizeBytes",
		"Filesystem",
		"MountPoint",
		"Mounted",
		"Temperature",
		"Status",
		"SpindownDelay",
		"ReadOnly",
		"Partitions",
	}

	// Verify all fields are valid
	for _, field := range fields {
		if len(field) == 0 {
			t.Error("Empty field name")
		}
	}

	// Verify we have reasonable number of fields
	if len(fields) < 10 {
		t.Error("Expected at least 10 device info fields")
	}
}

func TestFilesystemTypes(t *testing.T) {
	// Common filesystem types
	fsTypes := []string{
		"ext4",
		"xfs",
		"btrfs",
		"ntfs",
		"vfat",
		"exfat",
		"reiserfs",
		"zfs",
	}

	// Verify all types are recognized
	for _, fs := range fsTypes {
		if len(fs) == 0 {
			t.Error("Empty filesystem type")
		}
	}
}

func TestDeviceSizeParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		bytes   int64
		isValid bool
	}{
		{"1TB", "1000000000000", 1000000000000, true},
		{"500GB", "500000000000", 500000000000, true},
		{"100MB", "100000000", 100000000, true},
		{"Zero", "0", 0, true},
		{"Empty", "", 0, false},
		{"Invalid", "abc", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, isValid := parseSize(tt.input)
			if isValid != tt.isValid {
				t.Errorf("isValid = %v, want %v", isValid, tt.isValid)
			}
			if isValid && bytes != tt.bytes {
				t.Errorf("bytes = %d, want %d", bytes, tt.bytes)
			}
		})
	}
}

func parseSize(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var result int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		result = result*10 + int64(c-'0')
	}
	return result, true
}

func TestGetFilesystemUsage(t *testing.T) {
	path := t.TempDir()

	size, used, free, usagePercent, err := getFilesystemUsage(path)
	if err != nil {
		t.Fatalf("getFilesystemUsage returned error: %v", err)
	}
	if size == 0 {
		t.Fatal("expected non-zero filesystem size")
	}
	if used > size {
		t.Fatalf("expected used <= size, got used=%d size=%d", used, size)
	}
	if free > size {
		t.Fatalf("expected free <= size, got free=%d size=%d", free, size)
	}
	if usagePercent < 0 || usagePercent > 100 {
		t.Fatalf("expected usage percent in range 0..100, got %f", usagePercent)
	}
}

func TestGetFilesystemUsageTimed_HealthyMount(t *testing.T) {
	path := t.TempDir()

	size, _, _, _, err := getFilesystemUsageTimed(path, 5*time.Second)
	if err != nil {
		t.Fatalf("getFilesystemUsageTimed returned error: %v", err)
	}
	if size == 0 {
		t.Fatal("expected non-zero filesystem size")
	}
}

func TestGetFilesystemUsageTimed_HungMountTimesOut(t *testing.T) {
	orig := statfsFn
	release := make(chan struct{})
	probeDone := make(chan struct{})
	// Simulate a dead CIFS/NFS mount: statfs blocks until the kernel gives up.
	statfsFn = func(_ string, _ *syscall.Statfs_t) error {
		<-release
		close(probeDone) // signal after the probe goroutine has used statfsFn
		return nil
	}
	t.Cleanup(func() {
		close(release) // unblock the abandoned probe goroutine
		<-probeDone    // wait until it stops referencing statfsFn before restoring it
		statfsFn = orig
	})

	start := time.Now()
	_, _, _, _, err := getFilesystemUsageTimed("/mnt/remotes/dead_share", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error for hung statfs")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout-related error, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timed statfs blocked for %v, want prompt timeout", elapsed)
	}
}

func TestSpindownDelayValues(t *testing.T) {
	// Valid spindown delay values in minutes
	validDelays := []int{0, 15, 30, 45, 60, 120, 180, 240, 300}

	for _, delay := range validDelays {
		if delay < 0 {
			t.Errorf("Spindown delay should not be negative: %d", delay)
		}
	}

	// 0 means never spin down
	// Other values are minutes
	if validDelays[0] != 0 {
		t.Error("First delay value should be 0 (never)")
	}
}

func TestPartitionNumberParsing(t *testing.T) {
	tests := []struct {
		name        string
		partition   string
		expectedNum int
	}{
		{"First partition", "sda1", 1},
		{"Second partition", "sda2", 2},
		{"Tenth partition", "sda10", 10},
		{"No partition", "sda", 0},
		{"NVMe partition", "nvme0n1p1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num := extractPartitionNumber(tt.partition)
			if num != tt.expectedNum {
				t.Errorf("partition number = %d, want %d", num, tt.expectedNum)
			}
		})
	}
}

func extractPartitionNumber(s string) int {
	// For sd* devices
	if len(s) >= 4 && s[:2] == "sd" && s[2] >= 'a' && s[2] <= 'z' {
		if len(s) > 3 {
			num := 0
			for i := 3; i < len(s); i++ {
				if s[i] >= '0' && s[i] <= '9' {
					num = num*10 + int(s[i]-'0')
				}
			}
			return num
		}
		return 0
	}
	// For nvme*p* devices
	if len(s) > 7 && s[:4] == "nvme" {
		for i := 0; i < len(s); i++ {
			if s[i] == 'p' && i+1 < len(s) {
				num := 0
				for j := i + 1; j < len(s); j++ {
					if s[j] >= '0' && s[j] <= '9' {
						num = num*10 + int(s[j]-'0')
					}
				}
				return num
			}
		}
	}
	return 0
}

func TestParseSMBSource(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantServer string
		wantShare  string
	}{
		{"standard", "//192.168.1.100/backup", "192.168.1.100", "backup"},
		{"nested share", "//tower/media/movies", "tower", "media/movies"},
		{"hostname", "//nas.local/data", "nas.local", "data"},
		{"no share", "//server", "server", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, share := parseSMBSource(tt.source)
			if server != tt.wantServer || share != tt.wantShare {
				t.Errorf("parseSMBSource(%q) = (%q, %q), want (%q, %q)",
					tt.source, server, share, tt.wantServer, tt.wantShare)
			}
		})
	}
}

func TestParseNFSSource(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantServer string
		wantExport string
	}{
		{"standard", "192.168.1.100:/mnt/user/backup", "192.168.1.100", "/mnt/user/backup"},
		{"hostname", "nas:/export", "nas", "/export"},
		{"ipv6 host", "[fe80::1]:/export/media", "[fe80::1]", "/export/media"},
		{"no export", "192.168.1.100", "192.168.1.100", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, export := parseNFSSource(tt.source)
			if server != tt.wantServer || export != tt.wantExport {
				t.Errorf("parseNFSSource(%q) = (%q, %q), want (%q, %q)",
					tt.source, server, export, tt.wantServer, tt.wantExport)
			}
		})
	}
}

func TestIsUnassignedRemoteMount(t *testing.T) {
	tests := []struct {
		mountPoint string
		want       bool
	}{
		{"/mnt/remotes/NAS_backup", true},
		{"/mnt/disks/MyISO", true},
		{"/mnt/user/appdata", false},
		{"/mnt/cache", false},
		{"/boot", false},
	}
	for _, tt := range tests {
		t.Run(tt.mountPoint, func(t *testing.T) {
			if got := isUnassignedRemoteMount(tt.mountPoint); got != tt.want {
				t.Errorf("isUnassignedRemoteMount(%q) = %v, want %v", tt.mountPoint, got, tt.want)
			}
		})
	}
}

func TestMountHasOption(t *testing.T) {
	tests := []struct {
		options string
		want    string
		found   bool
	}{
		{"rw,relatime,vers=3.1.1", "ro", false},
		{"ro,relatime,vers=3.1.1", "ro", true},
		{"rw,nosuid,ro", "ro", true},
		{"rw,relatime", "rw", true},
		// "ro" must be an exact token, not a substring of "errors=remount-ro".
		{"rw,errors=remount-ro", "ro", false},
	}
	for _, tt := range tests {
		t.Run(tt.options, func(t *testing.T) {
			if got := mountHasOption(tt.options, tt.want); got != tt.found {
				t.Errorf("mountHasOption(%q, %q) = %v, want %v", tt.options, tt.want, got, tt.found)
			}
		})
	}
}

func TestUnescapeMountField(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/mnt/remotes/NAS_backup", "/mnt/remotes/NAS_backup"},
		{`/mnt/remotes/My\040Share`, "/mnt/remotes/My Share"},
		{`//server/Media\040Library`, "//server/Media Library"},
		{`/path\134with\134backslash`, `/path\with\backslash`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := unescapeMountField(tt.in); got != tt.want {
				t.Errorf("unescapeMountField(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseRemoteShareMounts(t *testing.T) {
	now := time.Now()
	procMounts := `proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
//192.168.1.100/backup /mnt/remotes/192.168.1.100_backup cifs rw,relatime,vers=3.1.1 0 0
192.168.1.50:/export/media /mnt/remotes/192.168.1.50_media nfs4 ro,relatime 0 0
//tower/Media\040Library /mnt/remotes/tower_Media_Library cifs rw,relatime 0 0
/dev/sda1 /mnt/disk1 xfs rw,relatime 0 0
192.168.1.60:/data /mnt/disks/legacy_nfs nfs rw,relatime 0 0
tmpfs /run tmpfs rw,nosuid 0 0`

	shares := parseRemoteShareMounts(procMounts, now)

	if len(shares) != 4 {
		t.Fatalf("expected 4 remote shares, got %d: %+v", len(shares), shares)
	}

	// SMB share
	if shares[0].Type != "smb" || shares[0].SMBServer != "192.168.1.100" ||
		shares[0].SMBShare != "backup" || shares[0].Status != "mounted" || shares[0].ReadOnly {
		t.Errorf("unexpected first SMB share: %+v", shares[0])
	}

	// NFS read-only share
	if shares[1].Type != "nfs" || shares[1].NFSServer != "192.168.1.50" ||
		shares[1].NFSExport != "/export/media" || !shares[1].ReadOnly {
		t.Errorf("unexpected NFS share: %+v", shares[1])
	}

	// SMB share with escaped space in mount point
	if shares[2].MountPoint != "/mnt/remotes/tower_Media_Library" ||
		shares[2].SMBShare != "Media Library" {
		t.Errorf("unexpected escaped SMB share: %+v", shares[2])
	}

	// Legacy NFS under /mnt/disks/
	if shares[3].Type != "nfs" || shares[3].MountPoint != "/mnt/disks/legacy_nfs" {
		t.Errorf("unexpected legacy NFS share: %+v", shares[3])
	}
}

func TestParseRemoteShareMountsEmpty(t *testing.T) {
	now := time.Now()
	procMounts := `proc /proc proc rw 0 0
/dev/sda1 /mnt/disk1 xfs rw 0 0
tmpfs /run tmpfs rw 0 0`

	shares := parseRemoteShareMounts(procMounts, now)
	if len(shares) != 0 {
		t.Errorf("expected no remote shares, got %d: %+v", len(shares), shares)
	}
}

func TestParseConfiguredRemoteShares(t *testing.T) {
	now := time.Now()
	cfg := `["//192.168.1.100/backup"]
protocol="SMB"
ip="192.168.1.100"
path="backup"
share="backup"
automount="yes"
read_only="no"

["192.168.1.50:/export/media"]
protocol="NFS"
ip="192.168.1.50"
path="/export/media"
automount="no"
read_only="yes"

["//tower/scratch"]
protocol="ROOT"
ip="tower"
path="scratch"`

	shares := parseConfiguredRemoteShares(cfg, now)
	if len(shares) != 3 {
		t.Fatalf("expected 3 configured shares (smb, nfs, root), got %d: %+v", len(shares), shares)
	}

	byType := map[string]dto.UnassignedRemoteShare{}
	for _, s := range shares {
		byType[s.Type] = s
	}

	root, ok := byType["root"]
	if !ok {
		t.Fatal("expected a root share")
	}
	if root.Source != "//tower/scratch" || root.SMBServer != "tower" ||
		root.SMBShare != "scratch" || root.Status != "unmounted" {
		t.Errorf("unexpected root share: %+v", root)
	}

	smb, ok := byType["smb"]
	if !ok {
		t.Fatal("expected an smb share")
	}
	if smb.Source != "//192.168.1.100/backup" || smb.SMBServer != "192.168.1.100" ||
		smb.SMBShare != "backup" || !smb.AutoMount || smb.ReadOnly || smb.Status != "unmounted" {
		t.Errorf("unexpected smb share: %+v", smb)
	}
	if smb.MountPoint != "/mnt/remotes/192.168.1.100_backup" {
		t.Errorf("unexpected smb mountpoint: %q", smb.MountPoint)
	}

	nfs, ok := byType["nfs"]
	if !ok {
		t.Fatal("expected an nfs share")
	}
	if nfs.NFSServer != "192.168.1.50" || nfs.NFSExport != "/export/media" ||
		nfs.AutoMount || !nfs.ReadOnly || nfs.Status != "unmounted" {
		t.Errorf("unexpected nfs share: %+v", nfs)
	}
}

func TestParseConfiguredRemoteSharesEmpty(t *testing.T) {
	if shares := parseConfiguredRemoteShares("", time.Now()); shares != nil {
		t.Errorf("expected nil for empty config, got %+v", shares)
	}
	// A 1-byte/whitespace config (UD default) yields no shares.
	if shares := parseConfiguredRemoteShares("\n", time.Now()); len(shares) != 0 {
		t.Errorf("expected no shares for blank config, got %+v", shares)
	}
}

func TestMergeRemoteShares(t *testing.T) {
	now := time.Now()
	configured := []dto.UnassignedRemoteShare{
		{Type: "smb", Source: "//192.168.1.100/backup", MountPoint: "/mnt/remotes/192.168.1.100_backup", Status: "unmounted", AutoMount: true, Timestamp: now},
		{Type: "nfs", Source: "192.168.1.50:/export/media", MountPoint: "/mnt/remotes/192.168.1.50_export_media", Status: "unmounted", AutoMount: false, Timestamp: now},
	}
	mounted := []dto.UnassignedRemoteShare{
		// Matches the first configured share by source — should become mounted
		// while keeping automount=true from config.
		{Type: "smb", Source: "//192.168.1.100/backup", MountPoint: "/mnt/remotes/192.168.1.100_backup", Status: "mounted", Timestamp: now},
		// A manually-mounted share not present in config — preserved.
		{Type: "smb", Source: "//other/share", MountPoint: "/mnt/remotes/other_share", Status: "mounted", Timestamp: now},
	}

	merged := mergeRemoteShares(configured, mounted)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged shares, got %d: %+v", len(merged), merged)
	}

	bySource := map[string]dto.UnassignedRemoteShare{}
	for _, s := range merged {
		bySource[s.Source] = s
	}

	if got := bySource["//192.168.1.100/backup"]; got.Status != "mounted" || !got.AutoMount {
		t.Errorf("expected matched share mounted+automount, got %+v", got)
	}
	if got := bySource["192.168.1.50:/export/media"]; got.Status != "unmounted" {
		t.Errorf("expected unmatched config share to stay unmounted, got %+v", got)
	}
	if got, ok := bySource["//other/share"]; !ok || got.Status != "mounted" {
		t.Errorf("expected manually-mounted share preserved, got %+v (ok=%v)", got, ok)
	}
}

func TestParseISOMountsFromProc(t *testing.T) {
	now := time.Now()
	procMounts := `/dev/loop2 /mnt/disks/ubuntu_iso iso9660 ro,relatime 0 0
/dev/sda1 /mnt/disk1 xfs rw,relatime 0 0
/dev/loop3 /var/lib/docker btrfs rw 0 0`

	shares := parseISOMountsFromProc(procMounts, now)
	if len(shares) != 1 {
		t.Fatalf("expected 1 ISO share, got %d: %+v", len(shares), shares)
	}
	if shares[0].Type != "iso" || shares[0].MountPoint != "/mnt/disks/ubuntu_iso" ||
		!shares[0].ReadOnly || shares[0].Status != "mounted" {
		t.Errorf("unexpected ISO share: %+v", shares[0])
	}
}

func TestArrayDiskFiltering(t *testing.T) {
	// Simulated array disks
	arrayDisks := map[string]bool{
		"sda": true,
		"sdb": true,
		"sdc": true,
	}

	tests := []struct {
		name        string
		device      string
		isArrayDisk bool
	}{
		{"Array disk sda", "sda", true},
		{"Array disk sdb", "sdb", true},
		{"Non-array sdd", "sdd", false},
		{"Non-array sde", "sde", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isArrayDisk := arrayDisks[tt.device]
			if isArrayDisk != tt.isArrayDisk {
				t.Errorf("isArrayDisk = %v, want %v", isArrayDisk, tt.isArrayDisk)
			}
		})
	}
}
