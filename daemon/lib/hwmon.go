package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HwmonBasePath is the sysfs base path for hwmon devices.
const HwmonBasePath = "/sys/class/hwmon"

// MaxHwmonDevices is the upper bound for hwmon device scanning.
const MaxHwmonDevices = 10

// MaxFanChannels is the upper bound for fan channel scanning per hwmon device.
const MaxFanChannels = 20

// MaxPlausibleRPM is the upper bound for plausible fan RPM readings.
// Values above this are treated as bogus sensor data. Even extreme server
// fans (Delta, Nidec) rarely exceed 15 000 RPM; 25 000 gives headroom.
const MaxPlausibleRPM = 25000

// HwmonFan holds the discovered sysfs paths and current readings for a fan channel.
type HwmonFan struct {
	ID         string
	Name       string
	RPM        int
	PWMValue   int
	PWMPercent int
	Mode       int // pwm_enable: 0=off, 1=manual, 2=auto
	HasPWM     bool
	HwmonDir   string
	FanIndex   int
}

// DiscoverHwmonFans scans /sys/class/hwmon for fan devices with their readings.
func DiscoverHwmonFans() []HwmonFan {
	var fans []HwmonFan

	for i := range MaxHwmonDevices {
		hwmonDir := filepath.Join(HwmonBasePath, fmt.Sprintf("hwmon%d", i))
		if _, err := os.Stat(hwmonDir); err != nil {
			continue
		}

		for j := 1; j < MaxFanChannels; j++ {
			inputPath := filepath.Join(hwmonDir, fmt.Sprintf("fan%d_input", j))
			if _, err := os.Stat(inputPath); err != nil {
				continue
			}

			fanID := fmt.Sprintf("hwmon%d_fan%d", i, j)
			fan := HwmonFan{
				ID:       fanID,
				HwmonDir: hwmonDir,
				FanIndex: j,
			}

			// Read RPM — filter bogus sensor readings
			rawRPM := ReadSysfsInt(inputPath)
			if rawRPM > MaxPlausibleRPM {
				rawRPM = 0 // treat implausible value as non-detected
			}
			fan.RPM = rawRPM

			// Read label
			labelPath := filepath.Join(hwmonDir, fmt.Sprintf("fan%d_label", j))
			fan.Name = ReadSysfsString(labelPath)
			if fan.Name == "" {
				fan.Name = fmt.Sprintf("Fan %d", j)
			}

			// Read PWM
			pwmPath := filepath.Join(hwmonDir, fmt.Sprintf("pwm%d", j))
			if _, err := os.Stat(pwmPath); err == nil {
				fan.HasPWM = true
				fan.PWMValue = ReadSysfsInt(pwmPath)
				fan.PWMPercent = PWMToPct(fan.PWMValue)

				enablePath := filepath.Join(hwmonDir, fmt.Sprintf("pwm%d_enable", j))
				fan.Mode = ReadSysfsInt(enablePath)
			}

			fans = append(fans, fan)
		}
	}

	return fans
}

// ReadSysfsInt reads an integer from a sysfs file. Returns 0 on any error.
func ReadSysfsInt(path string) int {
	// #nosec G304 -- path is constructed from bounded /sys/class/hwmon indices
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return val
}

// ReadSysfsString reads a trimmed string from a sysfs file. Returns "" on error.
func ReadSysfsString(path string) string {
	// #nosec G304 -- path is constructed from bounded /sys/class/hwmon indices
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteSysfs writes a string value to a sysfs file.
func WriteSysfs(path, value string) error {
	// #nosec G306 -- sysfs files require specific permissions to write
	return os.WriteFile(path, []byte(value), 0o644)
}

// PWMToPct converts a raw PWM value (0-255) to a percentage (0-100).
func PWMToPct(pwm int) int {
	if pwm <= 0 {
		return 0
	}
	if pwm >= 255 {
		return 100
	}
	return (pwm * 100) / 255
}

// PctToPWM converts a percentage (0-100) to a raw PWM value (0-255).
func PctToPWM(pct int) int {
	if pct <= 0 {
		return 0
	}
	if pct >= 100 {
		return 255
	}
	return (pct * 255) / 100
}

// unreliableTempLabels lists sensor labels commonly present on motherboard
// chipsets (Nuvoton NCT6775/6776/6779, ITE IT8xxx, etc.) that report phantom
// or stuck readings when no physical sensor is connected to the header.
var unreliableTempLabels = []string{
	"AUXTIN",    // auxiliary temp inputs — often unpopulated headers
	"SYSTIN",    // system temperature input — frequently bogus on Nuvoton chips
	"intrusion", // case intrusion detect — not a temperature sensor
}

// isUnreliableTempLabel returns true if the label matches a known-unreliable
// sensor type that should be excluded from safety decisions.
func isUnreliableTempLabel(label string) bool {
	upper := strings.ToUpper(label)
	for _, bad := range unreliableTempLabels {
		if strings.Contains(upper, strings.ToUpper(bad)) {
			return true
		}
	}
	return false
}

// IsPlausibleTempC returns true if the temperature is within a physically
// plausible range. Rejects values outside -40 °C … 125 °C, including the
// common I²C/SMBus "not connected" sentinel of 127–128 °C.
func IsPlausibleTempC(tempC float64) bool {
	return tempC > -40.0 && tempC < 125.0
}

// ReadMaxHwmonTemp scans hwmon temp*_input files and returns the highest
// temperature in °C.  It filters out:
//   - readings outside the -40 °C … 125 °C plausible range
//   - sensors with known-unreliable labels (AUXTIN, SYSTIN, intrusion)
func ReadMaxHwmonTemp() float64 {
	maxTemp := 0.0
	for i := range MaxHwmonDevices {
		hwmonDir := filepath.Join(HwmonBasePath, fmt.Sprintf("hwmon%d", i))
		for j := 1; j <= 20; j++ {
			// Skip sensors with labels known to produce unreliable data
			labelPath := filepath.Join(hwmonDir, fmt.Sprintf("temp%d_label", j))
			if label := ReadSysfsString(labelPath); label != "" && isUnreliableTempLabel(label) {
				continue
			}

			tempPath := filepath.Join(hwmonDir, fmt.Sprintf("temp%d_input", j))
			raw := ReadSysfsInt(tempPath)
			if raw == 0 {
				continue
			}
			tempC := float64(raw) / 1000.0
			if !IsPlausibleTempC(tempC) {
				continue
			}
			if tempC > maxTemp {
				maxTemp = tempC
			}
		}
	}
	return maxTemp
}
