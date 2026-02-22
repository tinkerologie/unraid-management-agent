package collectors

import (
	"testing"

	"github.com/digitalocean/go-libvirt"
	mobycontainer "github.com/moby/moby/api/types/container"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== DiskCollector: enrichWithRole =====

func TestEnrichWithRole(t *testing.T) {
	c := &DiskCollector{}

	tests := []struct {
		name     string
		diskName string
		diskID   string
		wantRole string
	}{
		{"parity disk by name", "parity", "", "parity"},
		{"parity disk by ID", "", "parity", "parity"},
		{"parity2 disk by name", "parity2", "", "parity2"},
		{"parity2 disk by ID", "", "Parity2", "parity2"},
		{"parity2 before parity match", "parity2-disk", "", "parity2"},
		{"data disk by name", "disk1", "", "data"},
		{"data disk by ID", "", "disk5", "data"},
		{"cache disk by name", "cache", "", "cache"},
		{"cache disk by ID", "", "cache1", "cache"},
		{"pool disk by name", "pool", "", "pool"},
		{"pool disk by ID", "", "pool_ssd", "pool"},
		{"unknown role", "sda", "sata-drive", "unknown"},
		{"empty names", "", "", "unknown"},
		{"case insensitive name", "PARITY", "", "parity"},
		{"case insensitive ID", "", "DISK1", "data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disk := &dto.DiskInfo{Name: tt.diskName, ID: tt.diskID}
			c.enrichWithRole(disk)
			if disk.Role != tt.wantRole {
				t.Errorf("enrichWithRole() role = %q, want %q", disk.Role, tt.wantRole)
			}
		})
	}
}

// ===== DockerCollector: convertPorts =====

func TestConvertPorts(t *testing.T) {
	c := &DockerCollector{}

	tests := []struct {
		name string
		in   []mobycontainer.PortSummary
		want []dto.PortMapping
	}{
		{
			name: "single port mapping",
			in: []mobycontainer.PortSummary{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			want: []dto.PortMapping{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
		},
		{
			name: "multiple ports",
			in: []mobycontainer.PortSummary{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
				{PrivatePort: 53, PublicPort: 53, Type: "udp"},
			},
			want: []dto.PortMapping{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
				{PrivatePort: 53, PublicPort: 53, Type: "udp"},
			},
		},
		{
			name: "exposed but not mapped",
			in: []mobycontainer.PortSummary{
				{PrivatePort: 3000, PublicPort: 0, Type: "tcp"},
			},
			want: []dto.PortMapping{
				{PrivatePort: 3000, PublicPort: 0, Type: "tcp"},
			},
		},
		{
			name: "empty ports",
			in:   []mobycontainer.PortSummary{},
			want: []dto.PortMapping{},
		},
		{
			name: "nil ports",
			in:   nil,
			want: []dto.PortMapping{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.convertPorts(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("convertPorts() len = %d, want %d", len(got), len(tt.want))
			}
			for i, g := range got {
				w := tt.want[i]
				if g.PrivatePort != w.PrivatePort || g.PublicPort != w.PublicPort || g.Type != w.Type {
					t.Errorf("convertPorts()[%d] = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

// ===== ShareCollector: determineStorage, isSMBExported, isNFSExported, determineMoverAction =====

func TestDetermineStorage(t *testing.T) {
	c := &ShareCollector{}

	tests := []struct {
		useCache string
		want     string
	}{
		{"no", "array"},
		{"only", "cache"},
		{"yes", "cache+array"},
		{"prefer", "cache+array"},
		{"", "unknown"},
		{"something-unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run("useCache="+tt.useCache, func(t *testing.T) {
			got := c.determineStorage(tt.useCache)
			if got != tt.want {
				t.Errorf("determineStorage(%q) = %q, want %q", tt.useCache, got, tt.want)
			}
		})
	}
}

func TestIsSMBExported(t *testing.T) {
	c := &ShareCollector{}

	tests := []struct {
		name     string
		export   string
		security string
		want     bool
	}{
		{"public security", "", "public", true},
		{"private security", "", "private", true},
		{"secure security", "", "secure", true},
		{"smb in export", "smb", "", true},
		{"-e flag in export", "-e", "", true},
		{"no export", "", "", false},
		{"nfs only", "nfs", "", false},
		{"empty both", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isSMBExported(tt.export, tt.security)
			if got != tt.want {
				t.Errorf("isSMBExported(%q, %q) = %v, want %v", tt.export, tt.security, got, tt.want)
			}
		})
	}
}

func TestIsNFSExported(t *testing.T) {
	c := &ShareCollector{}

	tests := []struct {
		name   string
		export string
		want   bool
	}{
		{"nfs in export", "nfs", true},
		{"-n flag in export", "-n", true},
		{"empty", "", false},
		{"smb only", "smb", false},
		{"-e flag", "-e", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isNFSExported(tt.export)
			if got != tt.want {
				t.Errorf("isNFSExported(%q) = %v, want %v", tt.export, got, tt.want)
			}
		})
	}
}

func TestDetermineMoverAction_Additional(t *testing.T) {
	c := &ShareCollector{}

	tests := []struct {
		name       string
		useCache   string
		cachePool  string
		cachePool2 string
		want       string
	}{
		{"pool to pool", "yes", "cache", "slowpool", "cache->slowpool"},
		{"cache to array", "yes", "cache", "", "cache->array"},
		{"prefer cache to array", "prefer", "cache", "", "cache->array"},
		{"cache only - no mover", "only", "", "", ""},
		{"array only - no mover", "no", "", "", ""},
		{"yes without cache pool", "yes", "", "", ""},
		{"prefer without cache pool", "prefer", "", "", ""},
		{"unknown useCache", "something", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.determineMoverAction(tt.useCache, tt.cachePool, tt.cachePool2)
			if got != tt.want {
				t.Errorf("determineMoverAction(%q, %q, %q) = %q, want %q", tt.useCache, tt.cachePool, tt.cachePool2, got, tt.want)
			}
		})
	}
}

// ===== ZFSCollector: parseDatasetLine, parseSnapshotLine =====

func TestParseDatasetLine(t *testing.T) {
	c := &ZFSCollector{}

	tests := []struct {
		name    string
		line    string
		wantNil bool
		check   func(t *testing.T, d *dto.ZFSDataset)
	}{
		{
			name: "valid dataset line",
			line: "tank/data\tfilesystem\t1073741824\t2147483648\t536870912\t1.50x\t/mnt/tank/data\t0\t0\tlz4\toff",
			check: func(t *testing.T, d *dto.ZFSDataset) {
				if d.Name != "tank/data" {
					t.Errorf("Name = %q, want %q", d.Name, "tank/data")
				}
				if d.Type != "filesystem" {
					t.Errorf("Type = %q, want %q", d.Type, "filesystem")
				}
				if d.UsedBytes != 1073741824 {
					t.Errorf("UsedBytes = %d, want %d", d.UsedBytes, 1073741824)
				}
				if d.AvailableBytes != 2147483648 {
					t.Errorf("AvailableBytes = %d, want %d", d.AvailableBytes, 2147483648)
				}
				if d.ReferencedBytes != 536870912 {
					t.Errorf("ReferencedBytes = %d", d.ReferencedBytes)
				}
				if d.CompressRatio != 1.50 {
					t.Errorf("CompressRatio = %f, want 1.50", d.CompressRatio)
				}
				if d.Mountpoint != "/mnt/tank/data" {
					t.Errorf("Mountpoint = %q", d.Mountpoint)
				}
				if d.Compression != "lz4" {
					t.Errorf("Compression = %q, want %q", d.Compression, "lz4")
				}
				if d.Readonly {
					t.Error("Readonly should be false for 'off'")
				}
			},
		},
		{
			name: "readonly dataset",
			line: "tank/readonly\tfilesystem\t0\t0\t0\t1.00\t-\t0\t0\tzstd\ton",
			check: func(t *testing.T, d *dto.ZFSDataset) {
				if !d.Readonly {
					t.Error("Readonly should be true for 'on'")
				}
				if d.Mountpoint != "" {
					t.Errorf("Mountpoint should be empty for '-', got %q", d.Mountpoint)
				}
			},
		},
		{
			name: "compression ratio without x suffix",
			line: "tank/test\tfilesystem\t0\t0\t0\t2.35\t-\t0\t0\toff\toff",
			check: func(t *testing.T, d *dto.ZFSDataset) {
				if d.CompressRatio != 2.35 {
					t.Errorf("CompressRatio = %f, want 2.35", d.CompressRatio)
				}
			},
		},
		{
			name:    "too few fields",
			line:    "tank/data\tfilesystem\t0",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.parseDatasetLine(tt.line)
			if tt.wantNil {
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestParseSnapshotLine(t *testing.T) {
	c := &ZFSCollector{}

	tests := []struct {
		name    string
		line    string
		wantNil bool
		check   func(t *testing.T, s *dto.ZFSSnapshot)
	}{
		{
			name: "valid snapshot line",
			line: "tank/data@backup1\t1048576\t536870912\t1700000000",
			check: func(t *testing.T, s *dto.ZFSSnapshot) {
				if s.Name != "tank/data@backup1" {
					t.Errorf("Name = %q, want %q", s.Name, "tank/data@backup1")
				}
				if s.Dataset != "tank/data" {
					t.Errorf("Dataset = %q, want %q", s.Dataset, "tank/data")
				}
				if s.UsedBytes != 1048576 {
					t.Errorf("UsedBytes = %d, want %d", s.UsedBytes, 1048576)
				}
				if s.ReferencedBytes != 536870912 {
					t.Errorf("ReferencedBytes = %d, want %d", s.ReferencedBytes, 536870912)
				}
				if s.CreationTime.Unix() != 1700000000 {
					t.Errorf("CreationTime = %d, want %d", s.CreationTime.Unix(), 1700000000)
				}
			},
		},
		{
			name: "nested dataset snapshot",
			line: "pool/sub/dataset@daily-2024\t0\t0\t1700000000",
			check: func(t *testing.T, s *dto.ZFSSnapshot) {
				if s.Dataset != "pool/sub/dataset" {
					t.Errorf("Dataset = %q", s.Dataset)
				}
			},
		},
		{
			name:    "no @ separator",
			line:    "tank/data\t0\t0\t0",
			wantNil: true,
		},
		{
			name:    "too few fields",
			line:    "tank/data@snap1\t0",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.parseSnapshotLine(tt.line)
			if tt.wantNil {
				if got != nil {
					t.Error("expected nil result")
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// ===== VMCollector: stateToString =====

func TestStateToString(t *testing.T) {
	c := &VMCollector{}

	tests := []struct {
		state libvirt.DomainState
		want  string
	}{
		{libvirt.DomainRunning, "running"},
		{libvirt.DomainBlocked, "blocked"},
		{libvirt.DomainPaused, "paused"},
		{libvirt.DomainShutdown, "shutdown"},
		{libvirt.DomainShutoff, "shut off"},
		{libvirt.DomainCrashed, "crashed"},
		{libvirt.DomainPmsuspended, "suspended"},
		{libvirt.DomainState(255), "unknown"},
		{libvirt.DomainState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := c.stateToString(tt.state)
			if got != tt.want {
				t.Errorf("stateToString(%d) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}
