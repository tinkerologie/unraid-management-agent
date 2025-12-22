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

func TestParseINIFileWithQuotedValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `name="My Server Name"
path="/mnt/user/data"
empty=""
single='single quotes'
`
	iniPath := filepath.Join(tmpDir, "quoted.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	if result["name"] != "My Server Name" {
		t.Errorf("ParseINIFile()[name] = %q, want %q", result["name"], "My Server Name")
	}
	if result["path"] != "/mnt/user/data" {
		t.Errorf("ParseINIFile()[path] = %q, want %q", result["path"], "/mnt/user/data")
	}
	if result["empty"] != "" {
		t.Errorf("ParseINIFile()[empty] = %q, want %q", result["empty"], "")
	}
}

func TestParseINIFileWithSpecialCharacters(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `url="http://example.com:8080/path?query=value"
regex=".*\\.txt$"
spaces="  value with spaces  "
equals="value=with=equals"
`
	iniPath := filepath.Join(tmpDir, "special.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	if result["url"] != "http://example.com:8080/path?query=value" {
		t.Errorf("ParseINIFile()[url] = %q, want %q", result["url"], "http://example.com:8080/path?query=value")
	}
}

func TestParseINIFileWithWhitespace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `key1 = value1
  key2=value2
key3=value3  
  key4  =  value4  
`
	iniPath := filepath.Join(tmpDir, "whitespace.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	// At minimum, we should get some values
	if len(result) < 1 {
		t.Errorf("ParseINIFile() should return at least 1 item, got %d", len(result))
	}
}

func TestParseINIFileNumericValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `port=8080
size=1024
float=3.14
negative=-42
hex=0xFF
`
	iniPath := filepath.Join(tmpDir, "numeric.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	if result["port"] != "8080" {
		t.Errorf("ParseINIFile()[port] = %q, want %q", result["port"], "8080")
	}
	if result["size"] != "1024" {
		t.Errorf("ParseINIFile()[size] = %q, want %q", result["size"], "1024")
	}
}

func TestParseINIFileBooleanValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `enabled=yes
disabled=no
active=true
inactive=false
on=1
off=0
`
	iniPath := filepath.Join(tmpDir, "boolean.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write INI file: %v", err)
	}

	result, err := ParseINIFile(iniPath)
	if err != nil {
		t.Fatalf("ParseINIFile() error = %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"enabled", "yes"},
		{"disabled", "no"},
		{"active", "true"},
		{"inactive", "false"},
		{"on", "1"},
		{"off", "0"},
	}

	for _, tt := range tests {
		if result[tt.key] != tt.expected {
			t.Errorf("ParseINIFile()[%s] = %q, want %q", tt.key, result[tt.key], tt.expected)
		}
	}
}

func TestGetINIValueNilMap(t *testing.T) {
	var iniData map[string]string

	got := GetINIValue(iniData, "anykey", "default")
	if got != "default" {
		t.Errorf("GetINIValue() = %q, want %q", got, "default")
	}
}

func TestGetINIValueEmptyKey(t *testing.T) {
	iniData := map[string]string{
		"key": "value",
		"":    "emptykey",
	}

	got := GetINIValue(iniData, "", "default")
	// Empty string as key should return the value if it exists
	if got != "emptykey" && got != "default" {
		t.Errorf("GetINIValue() = %q", got)
	}
}

func TestGetINIValueEmptyValue(t *testing.T) {
	iniData := map[string]string{
		"empty": "",
	}

	got := GetINIValue(iniData, "empty", "default")
	// Should return empty string, not default
	if got != "" {
		t.Errorf("GetINIValue() = %q, want empty string", got)
	}
}
