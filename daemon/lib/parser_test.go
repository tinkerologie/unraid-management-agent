package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseINIFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("valid INI file", func(t *testing.T) {
		content := `version="7.2.0"
name="Tower"
port=80
enabled=yes
`
		iniPath := filepath.Join(tmpDir, "test.ini")
		if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write INI file: %v", err)
		}

		result, err := ParseINIFile(iniPath)
		if err != nil {
			t.Fatalf("ParseINIFile() error = %v", err)
		}

		expected := map[string]string{
			"version": "7.2.0",
			"name":    "Tower",
			"port":    "80",
			"enabled": "yes",
		}

		for k, v := range expected {
			if result[k] != v {
				t.Errorf("ParseINIFile()[%q] = %q, want %q", k, result[k], v)
			}
		}
	})

	t.Run("empty INI file", func(t *testing.T) {
		iniPath := filepath.Join(tmpDir, "empty.ini")
		if err := os.WriteFile(iniPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write INI file: %v", err)
		}

		result, err := ParseINIFile(iniPath)
		if err != nil {
			t.Fatalf("ParseINIFile() error = %v", err)
		}

		if len(result) != 0 {
			t.Errorf("ParseINIFile() returned %d items, want 0", len(result))
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := ParseINIFile("/nonexistent/file.ini")
		if err == nil {
			t.Error("ParseINIFile() expected error for non-existent file")
		}
	})
}

func TestGetINIValue(t *testing.T) {
	iniData := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	tests := []struct {
		key          string
		defaultValue string
		expected     string
	}{
		{"key1", "default", "value1"},
		{"key2", "default", "value2"},
		{"key3", "default", "default"},
		{"missing", "fallback", "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := GetINIValue(iniData, tt.key, tt.defaultValue)
			if got != tt.expected {
				t.Errorf("GetINIValue(%q, %q) = %q, want %q", tt.key, tt.defaultValue, got, tt.expected)
			}
		})
	}
}

func TestGetINIValueEmptyMap(t *testing.T) {
	iniData := map[string]string{}

	got := GetINIValue(iniData, "anykey", "default")
	if got != "default" {
		t.Errorf("GetINIValue() = %q, want %q", got, "default")
	}
}

func TestParseINIFileWithSections(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Note: ParseINIFile only reads the default (unnamed) section
	content := `globalkey=globalvalue
[section1]
key1=value1
[section2]
key2=value2
`
	iniPath := filepath.Join(tmpDir, "sections.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	// Should only get the global key (default section)
	if result["globalkey"] != "globalvalue" {
		t.Errorf("ParseINIFile()[globalkey] = %q, want %q", result["globalkey"], "globalvalue")
	}
}

func TestParseINIFileWithComments(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `# This is a comment
; This is also a comment
key1=value1
# Another comment
key2=value2
`
	iniPath := filepath.Join(tmpDir, "comments.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	if len(result) != 2 {
		t.Errorf("ParseINIFile() returned %d items, want 2", len(result))
	}

	if result["key1"] != "value1" {
		t.Errorf("ParseINIFile()[key1] = %q, want %q", result["key1"], "value1")
	}
	if result["key2"] != "value2" {
		t.Errorf("ParseINIFile()[key2] = %q, want %q", result["key2"], "value2")
	}
}
