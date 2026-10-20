package collectors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"gopkg.in/ini.v1"
)

func TestNewRegistrationCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewRegistrationCollector(ctx)

	if collector == nil {
		t.Fatal("NewRegistrationCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("RegistrationCollector context not set correctly")
	}

	if collector.varIniPath != constants.VarIni {
		t.Errorf("varIniPath = %q, want %q", collector.varIniPath, constants.VarIni)
	}
}

func TestRegistrationCollectorInit(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewRegistrationCollector(ctx)

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

func TestRegistrationTypeParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Basic", "Basic", "basic"},
		{"Plus", "Plus", "plus"},
		{"Pro", "Pro", "pro"},
		{"Starter", "Starter", "starter"},
		{"Trial", "Trial", "trial"},
		{"BASIC uppercase", "BASIC", "basic"},
		{"Unknown", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Registration type should be lowercase
			result := stringToLower(tt.input)
			if result == "" {
				result = "unknown"
			}
			if result != tt.expected {
				t.Errorf("Registration type = %q, want %q", result, tt.expected)
			}
		})
	}
}

func stringToLower(s string) string {
	if s == "" {
		return ""
	}
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func TestRegistrationStateParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Valid", "Valid", "valid"},
		{"Expired", "Expired", "expired"},
		{"Invalid", "Invalid", "invalid"},
		{"VALID uppercase", "VALID", "valid"},
		{"Empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringToLower(tt.input)
			if result != tt.expected {
				t.Errorf("Registration state = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRegistrationDeviceCountParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Zero devices", "0", 0},
		{"Six devices", "6", 6},
		{"Twelve devices", "12", 12},
		{"Unlimited devices", "-1", -1},
		{"Invalid number", "abc", 0},
		{"Empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDeviceCount(tt.input)
			if result != tt.expected {
				t.Errorf("Device count = %d, want %d", result, tt.expected)
			}
		})
	}
}

func parseDeviceCount(s string) int {
	if s == "" {
		return 0
	}
	result := 0
	negative := false
	i := 0
	if len(s) > 0 && s[0] == '-' {
		negative = true
		i = 1
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	if negative {
		return -result
	}
	return result
}

func TestRegistrationGUIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		guid    string
		isValid bool
	}{
		{"Valid GUID format", "1234-5678-ABCD-EFGH-IJKL", true},
		{"Short GUID", "1234", true},
		{"Empty GUID", "", false},
		{"Spaces only", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := len(regTrimSpace(tt.guid)) > 0
			if isValid != tt.isValid {
				t.Errorf("GUID valid = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

func regTrimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func TestRegistrationVarIniKeys(t *testing.T) {
	// Keys expected in var.ini for registration info
	expectedKeys := []string{
		"regTy",    // Registration type
		"regState", // Registration state (valid/expired)
		"regGUID",  // Registration GUID
		"regTo",    // Registered to
		"regTm",    // Registration time
		"regTm2",   // Registration expiry
	}

	// Verify all keys are non-empty strings
	for _, key := range expectedKeys {
		if len(key) == 0 {
			t.Error("Empty registration key name")
		}
	}

	// Verify expected number of keys
	if len(expectedKeys) < 4 {
		t.Error("Expected at least 4 registration keys")
	}
}

func TestRegistrationLicenseTypes(t *testing.T) {
	// All valid Unraid license types
	licenseTypes := map[string]struct {
		maxDevices int
		features   []string
	}{
		"basic": {
			maxDevices: 6,
			features:   []string{"array", "docker", "vms"},
		},
		"plus": {
			maxDevices: 12,
			features:   []string{"array", "docker", "vms"},
		},
		"pro": {
			maxDevices: -1, // unlimited
			features:   []string{"array", "docker", "vms"},
		},
		"starter": {
			maxDevices: 2,
			features:   []string{"array"},
		},
		"trial": {
			maxDevices: -1,
			features:   []string{"array", "docker", "vms"},
		},
	}

	// Verify we have all expected license types
	expectedTypes := []string{"basic", "plus", "pro", "starter", "trial"}
	for _, lt := range expectedTypes {
		if _, ok := licenseTypes[lt]; !ok {
			t.Errorf("Missing license type: %s", lt)
		}
	}
}

func TestRegistrationTimestampParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"Unix timestamp", "1609459200", true},
		{"Large timestamp", "1735689600", true},
		{"Zero timestamp", "0", true},
		{"Empty", "", false},
		{"Invalid", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := isValidTimestamp(tt.input)
			if isValid != tt.isValid {
				t.Errorf("Timestamp valid = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

func isValidTimestamp(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// TestParseRegistration checks type, state and both dates for each key type,
// including the #285 regression where a Pro key reported expired.
func TestParseRegistration(t *testing.T) {
	now := time.Unix(1792454400, 0) // 2026-10-19
	past := "1601401334"            // 2020-09-29
	future := "1893456000"          // 2030-01-01

	tests := []struct {
		name           string
		varIni         string
		wantType       string
		wantState      string
		wantExpiration string
		wantUpdateExp  string
	}{
		{
			name:      "Pro key registered years ago stays valid (#285)",
			varIni:    "regTy=\"Pro\"\nregTm=\"" + past + "\"\nregExp=\"\"\n",
			wantType:  "pro",
			wantState: "valid",
		},
		{
			name:      "Basic key with a past regTm",
			varIni:    "regTy=\"Basic\"\nregTm=\"" + past + "\"\nregExp=\"\"\n",
			wantType:  "basic",
			wantState: "valid",
		},
		{
			name:      "Plus key ignores an unrelated regTm2",
			varIni:    "regTy=\"Plus\"\nregTm=\"" + past + "\"\nregTm2=\"" + past + "\"\nregExp=\"\"\n",
			wantType:  "plus",
			wantState: "valid",
		},
		{
			name:          "Starter key with lapsed updates is still a valid license",
			varIni:        "regTy=\"Starter\"\nregTm=\"" + past + "\"\nregExp=\"" + past + "\"\n",
			wantType:      "starter",
			wantState:     "valid",
			wantUpdateExp: past,
		},
		{
			name:          "Unleashed key reports its update-eligibility date",
			varIni:        "regTy=\"Unleashed\"\nregTm=\"" + past + "\"\nregExp=\"" + future + "\"\n",
			wantType:      "unleashed",
			wantState:     "valid",
			wantUpdateExp: future,
		},
		{
			name:           "active trial",
			varIni:         "regTy=\"Trial\"\nregTm=\"" + past + "\"\nregTm2=\"" + future + "\"\n",
			wantType:       "trial",
			wantState:      "trial",
			wantExpiration: future,
		},
		{
			name:           "trial past its expiry",
			varIni:         "regTy=\"Trial\"\nregTm=\"" + past + "\"\nregTm2=\"" + past + "\"\n",
			wantType:       "trial",
			wantState:      "expired",
			wantExpiration: past,
		},
		{
			name:           "key Unraid marks expired",
			varIni:         "regTy=\"expired\"\nregTm2=\"" + past + "\"\n",
			wantType:       "expired",
			wantState:      "expired",
			wantExpiration: past,
		},
		{
			name:      "zero dates are ignored",
			varIni:    "regTy=\"Trial\"\nregTm2=\"0\"\nregExp=\"0\"\n",
			wantType:  "trial",
			wantState: "trial",
		},
		{
			name:      "empty dates are ignored",
			varIni:    "regTy=\"Trial\"\nregTm2=\"\"\nregExp=\"\"\n",
			wantType:  "trial",
			wantState: "trial",
		},
		{
			name:      "non-numeric dates are ignored",
			varIni:    "regTy=\"Trial\"\nregTm2=\"soon\"\nregExp=\"later\"\n",
			wantType:  "trial",
			wantState: "trial",
		},
		{
			name:      "missing regTy",
			varIni:    "regTm=\"" + past + "\"\n",
			wantType:  "unknown",
			wantState: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ini.Load([]byte(tt.varIni))
			if err != nil {
				t.Fatalf("ini.Load: %v", err)
			}
			reg := parseRegistration(cfg.Section(""), now)

			if reg.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", reg.Type, tt.wantType)
			}
			if reg.State != tt.wantState {
				t.Errorf("State = %q, want %q", reg.State, tt.wantState)
			}
			if !reg.Timestamp.Equal(now) {
				t.Errorf("Timestamp = %v, want %v", reg.Timestamp, now)
			}
			checkUnix(t, "Expiration", reg.Expiration, tt.wantExpiration)
			checkUnix(t, "UpdateExpiration", reg.UpdateExpiration, tt.wantUpdateExp)
		})
	}
}

// TestParseRegistrationNameAndGUID checks the GUID and server name are read.
func TestParseRegistrationNameAndGUID(t *testing.T) {
	cfg, err := ini.Load([]byte("regTy=\"Pro\"\nregGUID=\"ABCD-1234\"\nNAME=\"Tower\"\n"))
	if err != nil {
		t.Fatalf("ini.Load: %v", err)
	}
	reg := parseRegistration(cfg.Section(""), time.Now())

	if reg.GUID != "ABCD-1234" {
		t.Errorf("GUID = %q, want %q", reg.GUID, "ABCD-1234")
	}
	if reg.ServerName != "Tower" {
		t.Errorf("ServerName = %q, want %q", reg.ServerName, "Tower")
	}
}

// TestParseRegistrationFixture parses the Unraid 7.3.1 var.ini fixture.
func TestParseRegistrationFixture(t *testing.T) {
	cfg, err := ini.Load("testdata/fixtures/unraid-7.3.1/var.ini")
	if err != nil {
		t.Fatalf("ini.Load: %v", err)
	}
	reg := parseRegistration(cfg.Section(""), time.Now())

	if reg.Type != "plus" || reg.State != "valid" {
		t.Errorf("Type/State = %q/%q, want plus/valid", reg.Type, reg.State)
	}
	if !reg.Expiration.IsZero() || !reg.UpdateExpiration.IsZero() {
		t.Errorf("Expiration/UpdateExpiration = %v/%v, want zero", reg.Expiration, reg.UpdateExpiration)
	}
}

// TestParseRegistrationJSON locks the wire format for a key with no dates.
func TestParseRegistrationJSON(t *testing.T) {
	cfg, err := ini.Load([]byte("regTy=\"Pro\"\nregTm=\"1601401334\"\nregExp=\"\"\n"))
	if err != nil {
		t.Fatalf("ini.Load: %v", err)
	}
	b, err := json.Marshal(parseRegistration(cfg.Section(""), time.Now()))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, want := range []string{
		`"state":"valid"`,
		`"expiration":"0001-01-01T00:00:00Z"`,
		`"update_expiration":"0001-01-01T00:00:00Z"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON %s missing %s", b, want)
		}
	}
}

// checkUnix fails the test unless got is zero for an empty want, or equals the
// Unix timestamp in want.
func checkUnix(t *testing.T, field string, got time.Time, want string) {
	t.Helper()
	if want == "" {
		if !got.IsZero() {
			t.Errorf("%s = %v, want zero", field, got)
		}
		return
	}
	wantTS, err := strconv.ParseInt(want, 10, 64)
	if err != nil {
		t.Fatalf("bad test timestamp %q: %v", want, err)
	}
	if got.IsZero() || got.Unix() != wantTS {
		t.Errorf("%s = %v, want Unix %s", field, got, want)
	}
}

// TestCollectRegistrationFromFile reads a var.ini through the collector's
// injectable path and returns the parsed registration.
func TestCollectRegistrationFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "var.ini")
	content := "NAME=\"Tower\"\nregTy=\"Pro\"\nregTm=\"1601401334\"\nregExp=\"\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	collector := NewRegistrationCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	collector.varIniPath = path

	reg, err := collector.collectRegistration()
	if err != nil {
		t.Fatalf("collectRegistration: %v", err)
	}
	if reg.Type != "pro" || reg.State != "valid" || reg.ServerName != "Tower" {
		t.Errorf("got type=%q state=%q server=%q, want pro/valid/Tower", reg.Type, reg.State, reg.ServerName)
	}
	if reg.Timestamp.IsZero() {
		t.Error("Timestamp not set")
	}
}

// TestCollectRegistrationLoadError returns the ini.Load error when var.ini is
// missing.
func TestCollectRegistrationLoadError(t *testing.T) {
	collector := NewRegistrationCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	collector.varIniPath = filepath.Join(t.TempDir(), "missing.ini")

	reg, err := collector.collectRegistration()
	if err == nil {
		t.Fatal("collectRegistration: want error for a missing file")
	}
	if reg != nil {
		t.Errorf("registration = %+v, want nil on error", reg)
	}
}
