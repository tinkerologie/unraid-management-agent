package lib

import (
	"testing"
)

// Tests for public dmidecode functions that call ExecCommandOutput.
// On non-Linux systems these return errors, covering the error paths.

func TestParseDmidecodeType_ErrorPath(t *testing.T) {
	// dmidecode not available on macOS — tests error handling
	sections, err := ParseDmidecodeType("0")
	if err == nil {
		// On Linux with dmidecode, this may succeed
		if sections == nil {
			t.Error("Expected non-nil sections when no error")
		}
		return
	}
	// Error is expected on macOS
	if sections != nil {
		t.Error("Expected nil sections on error")
	}
}

func TestParseBIOSInfo_ErrorPath(t *testing.T) {
	info, err := ParseBIOSInfo()
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}

func TestParseBaseboardInfo_ErrorPath(t *testing.T) {
	info, err := ParseBaseboardInfo()
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}

func TestParseCPUInfo_ErrorPath(t *testing.T) {
	info, err := ParseCPUInfo()
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}

func TestParseCPUCacheInfo_ErrorPath(t *testing.T) {
	info, err := ParseCPUCacheInfo()
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}

func TestParseMemoryArrayInfo_ErrorPath(t *testing.T) {
	info, err := ParseMemoryArrayInfo()
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}

func TestParseMemoryDevices_ErrorPath(t *testing.T) {
	devices, err := ParseMemoryDevices()
	if err == nil {
		if devices == nil {
			t.Error("Expected non-nil devices when no error")
		}
		return
	}
	if devices != nil {
		t.Error("Expected nil devices on error")
	}
}

func TestParseEthtool_ErrorPath(t *testing.T) {
	info, err := ParseEthtool("eth0")
	if err == nil {
		if info == nil {
			t.Error("Expected non-nil info when no error")
		}
		return
	}
	if info != nil {
		t.Error("Expected nil info on error")
	}
}
