package controllers

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewProcessController(t *testing.T) {
	pc := NewProcessController()
	if pc == nil {
		t.Fatal("NewProcessController returned nil")
	}
}

func TestParseProcessLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		check   func(t *testing.T, p *dto.ProcessInfo)
	}{
		{
			name: "standard ps aux line",
			line: "root         1  0.0  0.1 168404 11628 ?        Ss   Jun10   0:45 /sbin/init",
			check: func(t *testing.T, p *dto.ProcessInfo) {
				if p.User != "root" {
					t.Errorf("User = %q, want %q", p.User, "root")
				}
				if p.PID != 1 {
					t.Errorf("PID = %d, want %d", p.PID, 1)
				}
				if p.CPUPercent != 0.0 {
					t.Errorf("CPUPercent = %f, want %f", p.CPUPercent, 0.0)
				}
				if p.MemoryPercent != 0.1 {
					t.Errorf("MemoryPercent = %f, want %f", p.MemoryPercent, 0.1)
				}
				if p.VSZBytes != 168404*1024 {
					t.Errorf("VSZBytes = %d, want %d", p.VSZBytes, 168404*1024)
				}
				if p.RSSBytes != 11628*1024 {
					t.Errorf("RSSBytes = %d, want %d", p.RSSBytes, 11628*1024)
				}
				if p.TTY != "?" {
					t.Errorf("TTY = %q, want %q", p.TTY, "?")
				}
				if p.State != "Ss" {
					t.Errorf("State = %q, want %q", p.State, "Ss")
				}
				if p.Started != "Jun10" {
					t.Errorf("Started = %q, want %q", p.Started, "Jun10")
				}
				if p.Time != "0:45" {
					t.Errorf("Time = %q, want %q", p.Time, "0:45")
				}
				if p.Command != "/sbin/init" {
					t.Errorf("Command = %q, want %q", p.Command, "/sbin/init")
				}
			},
		},
		{
			name: "command with spaces",
			line: "nobody    1234 25.5 12.3 523456 125432 ?        Sl   10:00   1:23 /usr/bin/python3 /opt/app/server.py --config /etc/conf.d",
			check: func(t *testing.T, p *dto.ProcessInfo) {
				if p.User != "nobody" {
					t.Errorf("User = %q, want %q", p.User, "nobody")
				}
				if p.PID != 1234 {
					t.Errorf("PID = %d, want %d", p.PID, 1234)
				}
				if p.CPUPercent != 25.5 {
					t.Errorf("CPUPercent = %f, want %f", p.CPUPercent, 25.5)
				}
				if p.MemoryPercent != 12.3 {
					t.Errorf("MemoryPercent = %f, want %f", p.MemoryPercent, 12.3)
				}
				if p.Command != "/usr/bin/python3 /opt/app/server.py --config /etc/conf.d" {
					t.Errorf("Command = %q, want command with args", p.Command)
				}
			},
		},
		{
			name: "tty with pts",
			line: "user     5678  1.2  0.5  34567  12345 pts/0    R+   12:34   0:01 top",
			check: func(t *testing.T, p *dto.ProcessInfo) {
				if p.TTY != "pts/0" {
					t.Errorf("TTY = %q, want %q", p.TTY, "pts/0")
				}
				if p.State != "R+" {
					t.Errorf("State = %q, want %q", p.State, "R+")
				}
			},
		},
		{
			name:    "too few fields",
			line:    "root 1 0.0 0.1",
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
		},
		{
			name:    "invalid PID",
			line:    "root     abc  0.0  0.1 168404 11628 ?        Ss   Jun10   0:45 /sbin/init",
			wantErr: true,
		},
		{
			name: "zero values for CPU/MEM",
			line: "root         2  0.0  0.0      0     0 ?        S    Jun10   0:00 [kthreadd]",
			check: func(t *testing.T, p *dto.ProcessInfo) {
				if p.PID != 2 {
					t.Errorf("PID = %d, want %d", p.PID, 2)
				}
				if p.CPUPercent != 0.0 {
					t.Errorf("CPUPercent = %f, want 0.0", p.CPUPercent)
				}
				if p.MemoryPercent != 0.0 {
					t.Errorf("MemoryPercent = %f, want 0.0", p.MemoryPercent)
				}
				if p.Command != "[kthreadd]" {
					t.Errorf("Command = %q, want %q", p.Command, "[kthreadd]")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc, err := parseProcessLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProcessLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil && proc != nil {
				tt.check(t, proc)
			}
		})
	}
}

func TestSortProcessesByMemory(t *testing.T) {
	procs := []dto.ProcessInfo{
		{PID: 1, MemoryPercent: 1.5},
		{PID: 2, MemoryPercent: 10.0},
		{PID: 3, MemoryPercent: 5.0},
		{PID: 4, MemoryPercent: 0.1},
	}

	sortProcessesByMemory(procs)

	// Verify descending order
	expected := []float64{10.0, 5.0, 1.5, 0.1}
	for i, exp := range expected {
		if procs[i].MemoryPercent != exp {
			t.Errorf("sortProcessesByMemory: position %d = %f, want %f", i, procs[i].MemoryPercent, exp)
		}
	}
}

func TestSortProcessesByMemory_Empty(t *testing.T) {
	procs := []dto.ProcessInfo{}
	sortProcessesByMemory(procs) // should not panic
}

func TestSortProcessesByMemory_SingleElement(t *testing.T) {
	procs := []dto.ProcessInfo{{PID: 1, MemoryPercent: 5.0}}
	sortProcessesByMemory(procs)
	if procs[0].MemoryPercent != 5.0 {
		t.Errorf("expected 5.0, got %f", procs[0].MemoryPercent)
	}
}

func TestSortProcessesByMemory_AlreadySorted(t *testing.T) {
	procs := []dto.ProcessInfo{
		{PID: 1, MemoryPercent: 10.0},
		{PID: 2, MemoryPercent: 5.0},
		{PID: 3, MemoryPercent: 1.0},
	}
	sortProcessesByMemory(procs)
	if procs[0].MemoryPercent != 10.0 || procs[1].MemoryPercent != 5.0 || procs[2].MemoryPercent != 1.0 {
		t.Error("sortProcessesByMemory changed already sorted list")
	}
}

func TestSortProcessesByPID(t *testing.T) {
	procs := []dto.ProcessInfo{
		{PID: 500},
		{PID: 1},
		{PID: 100},
		{PID: 50},
	}

	sortProcessesByPID(procs)

	// Verify ascending order
	expected := []int{1, 50, 100, 500}
	for i, exp := range expected {
		if procs[i].PID != exp {
			t.Errorf("sortProcessesByPID: position %d = %d, want %d", i, procs[i].PID, exp)
		}
	}
}

func TestSortProcessesByPID_Empty(t *testing.T) {
	procs := []dto.ProcessInfo{}
	sortProcessesByPID(procs) // should not panic
}

func TestSortProcessesByPID_SingleElement(t *testing.T) {
	procs := []dto.ProcessInfo{{PID: 42}}
	sortProcessesByPID(procs)
	if procs[0].PID != 42 {
		t.Errorf("expected PID 42, got %d", procs[0].PID)
	}
}

func TestSortProcessesByPID_EqualValues(t *testing.T) {
	procs := []dto.ProcessInfo{
		{PID: 100, User: "a"},
		{PID: 100, User: "b"},
	}
	sortProcessesByPID(procs) // should not panic with equal values
	if procs[0].PID != 100 || procs[1].PID != 100 {
		t.Error("sort altered equal PID values unexpectedly")
	}
}
