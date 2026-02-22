package controllers

import (
	"testing"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"full sha256 ID", "sha256:4f0dc085151100000000000000000000000000000000000000000000deadbeef", "4f0dc0851511"},
		{"full hex ID without prefix", "4f0dc085151100000000000000000000000000000000000000000000deadbeef", "4f0dc0851511"},
		{"short 12 char ID", "4f0dc0851511", "4f0dc0851511"},
		{"shorter than 12", "4f0dc08515", "4f0dc08515"},
		{"empty string", "", ""},
		{"exactly sha256: prefix only", "sha256:", ""},
		{"sha256: prefix with short ID", "sha256:abcdef123456", "abcdef123456"},
		{"13 char ID", "4f0dc08515111", "4f0dc0851511"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortID(tt.id)
			if got != tt.want {
				t.Errorf("shortID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero bytes", 0, "0 B"},
		{"100 bytes", 100, "100 B"},
		{"1023 bytes", 1023, "1023 B"},
		{"exactly 1 KiB", 1024, "1.0 KiB"},
		{"1.5 KiB", 1536, "1.5 KiB"},
		{"exactly 1 MiB", 1024 * 1024, "1.0 MiB"},
		{"500 MiB", 500 * 1024 * 1024, "500.0 MiB"},
		{"exactly 1 GiB", 1024 * 1024 * 1024, "1.0 GiB"},
		{"2.5 GiB", int64(2.5 * 1024 * 1024 * 1024), "2.5 GiB"},
		{"10 GiB", 10 * 1024 * 1024 * 1024, "10.0 GiB"},
		{"1 byte", 1, "1 B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestNewDockerController_CloseWithoutInit(t *testing.T) {
	dc := NewDockerController()
	if dc == nil {
		t.Fatal("NewDockerController returned nil")
	}
	// Close without init should be safe
	if err := dc.Close(); err != nil {
		t.Errorf("Close on new controller returned error: %v", err)
	}
}
