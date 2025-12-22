package logger

import (
	"testing"
)

func TestSetLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected LogLevel
	}{
		{"set debug", LevelDebug, LevelDebug},
		{"set info", LevelInfo, LevelInfo},
		{"set warning", LevelWarning, LevelWarning},
		{"set error", LevelError, LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLevel(tt.level)
			if GetLevel() != tt.expected {
				t.Errorf("GetLevel() = %v, want %v", GetLevel(), tt.expected)
			}
		})
	}
	// Reset to default
	SetLevel(LevelWarning)
}

func TestGetLevel(t *testing.T) {
	// Save current level
	originalLevel := GetLevel()

	tests := []struct {
		name     string
		setLevel LogLevel
	}{
		{"debug level", LevelDebug},
		{"info level", LevelInfo},
		{"warning level", LevelWarning},
		{"error level", LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLevel(tt.setLevel)
			if got := GetLevel(); got != tt.setLevel {
				t.Errorf("GetLevel() = %v, want %v", got, tt.setLevel)
			}
		})
	}

	// Restore original level
	SetLevel(originalLevel)
}

func TestLogLevelConstants(t *testing.T) {
	// Verify log level ordering
	if LevelDebug >= LevelInfo {
		t.Error("LevelDebug should be less than LevelInfo")
	}
	if LevelInfo >= LevelWarning {
		t.Error("LevelInfo should be less than LevelWarning")
	}
	if LevelWarning >= LevelError {
		t.Error("LevelWarning should be less than LevelError")
	}
}

func TestLoggingFunctions(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()

	t.Run("Info at debug level", func(t *testing.T) {
		SetLevel(LevelDebug)
		// These should not panic - they just log
		Info("test info message")
	})

	t.Run("Success at debug level", func(t *testing.T) {
		SetLevel(LevelDebug)
		Success("test success message")
	})

	t.Run("Warning at warning level", func(t *testing.T) {
		SetLevel(LevelWarning)
		Warning("test warning message")
	})

	t.Run("Error at error level", func(t *testing.T) {
		SetLevel(LevelError)
		Error("test error message")
	})

	t.Run("Debug at debug level", func(t *testing.T) {
		SetLevel(LevelDebug)
		Debug("test debug message")
	})

	t.Run("Plain logging", func(t *testing.T) {
		Plain("test plain message")
	})

	t.Run("Blue alias", func(t *testing.T) {
		SetLevel(LevelDebug)
		Blue("test blue message")
	})

	t.Run("Yellow alias", func(t *testing.T) {
		SetLevel(LevelWarning)
		Yellow("test yellow message")
	})

	// Restore original level
	SetLevel(originalLevel)
}

func TestLogLevelFiltering(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()

	t.Run("Info suppressed at warning level", func(t *testing.T) {
		SetLevel(LevelWarning)
		// This should be suppressed - no way to verify output without capturing stderr
		// but it shouldn't panic
		Info("this should be suppressed")
	})

	t.Run("Debug suppressed at info level", func(t *testing.T) {
		SetLevel(LevelInfo)
		Debug("this should be suppressed")
	})

	t.Run("Warning suppressed at error level", func(t *testing.T) {
		SetLevel(LevelError)
		Warning("this should be suppressed")
	})

	// Restore original level
	SetLevel(originalLevel)
}

func TestColorConstants(t *testing.T) {
	// Verify color codes are not empty
	colors := map[string]string{
		"ColorReset":  ColorReset,
		"ColorRed":    ColorRed,
		"ColorGreen":  ColorGreen,
		"ColorYellow": ColorYellow,
		"ColorBlue":   ColorBlue,
		"ColorPurple": ColorPurple,
		"ColorCyan":   ColorCyan,
		"ColorWhite":  ColorWhite,
	}

	for name, color := range colors {
		if color == "" {
			t.Errorf("%s should not be empty", name)
		}
	}
}

func TestLogWithFormatArgs(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()
	SetLevel(LevelDebug)

	// Test with format arguments - should not panic
	Info("message with %s and %d", "string", 42)
	Success("success %v", true)
	Warning("warning %f", 3.14)
	Error("error %x", 255)
	Debug("debug %#v", map[string]int{"a": 1})
	Plain("plain %q", "quoted")

	// Restore original level
	SetLevel(originalLevel)
}

func TestGreenFunction(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()
	SetLevel(LevelDebug)

	// Test Green function - should not panic
	Green("green message %s", "test")
	Green("green message without args")

	// Restore original level
	SetLevel(originalLevel)
}

func TestLightGreenFunction(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()
	SetLevel(LevelDebug)

	// Test LightGreen function - should not panic
	LightGreen("light green message %s", "test")
	LightGreen("light green message without args")

	// With higher level (should not log but not panic)
	SetLevel(LevelWarning)
	LightGreen("should not appear")

	// Restore original level
	SetLevel(originalLevel)
}

func TestPrintfFunction(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()
	SetLevel(LevelDebug)

	// Test Printf function - should not panic
	Printf("printf message %s", "test")
	Printf("printf message without args")

	// With higher level (should not log but not panic)
	SetLevel(LevelWarning)
	Printf("should not appear")

	// Restore original level
	SetLevel(originalLevel)
}

func TestPrintlnFunction(t *testing.T) {
	// Save original level
	originalLevel := GetLevel()
	SetLevel(LevelDebug)

	// Test Println function - should not panic
	Println("println message", "with", "multiple", "args")
	Println("println single arg")
	Println()

	// With higher level (should not log but not panic)
	SetLevel(LevelWarning)
	Println("should not appear")

	// Restore original level
	SetLevel(originalLevel)
}

func TestSprintfFunction(t *testing.T) {
	// Test Sprintf function
	result := Sprintf("formatted %s with %d", "string", 42)
	expected := "formatted string with 42"

	if result != expected {
		t.Errorf("Sprintf() = %q, want %q", result, expected)
	}

	// Test without format args
	result2 := Sprintf("no format args")
	if result2 != "no format args" {
		t.Errorf("Sprintf() = %q, want %q", result2, "no format args")
	}
}
