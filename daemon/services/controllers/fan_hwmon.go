package controllers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// HwmonProvider reads and controls fans via Linux hwmon sysfs.
type HwmonProvider struct {
	fanMap map[string]hwmonFanPaths
}

// hwmonFanPaths holds the sysfs paths for a single fan channel.
type hwmonFanPaths struct {
	pwmPath    string
	enablePath string
	hwmonDir   string
	fanIndex   int
}

// NewHwmonProvider creates a new hwmon fan provider.
func NewHwmonProvider() *HwmonProvider {
	return &HwmonProvider{
		fanMap: make(map[string]hwmonFanPaths),
	}
}

// Discover scans hwmon sysfs for fan devices and caches their write paths.
func (p *HwmonProvider) Discover() {
	p.fanMap = make(map[string]hwmonFanPaths)

	for i := range lib.MaxHwmonDevices {
		hwmonDir := filepath.Join(lib.HwmonBasePath, fmt.Sprintf("hwmon%d", i))
		if _, err := os.Stat(hwmonDir); err != nil {
			continue
		}

		for j := 1; j < lib.MaxFanChannels; j++ {
			inputPath := filepath.Join(hwmonDir, fmt.Sprintf("fan%d_input", j))
			if _, err := os.Stat(inputPath); err != nil {
				continue
			}

			fanID := fmt.Sprintf("hwmon%d_fan%d", i, j)
			p.fanMap[fanID] = hwmonFanPaths{
				pwmPath:    filepath.Join(hwmonDir, fmt.Sprintf("pwm%d", j)),
				enablePath: filepath.Join(hwmonDir, fmt.Sprintf("pwm%d_enable", j)),
				hwmonDir:   hwmonDir,
				fanIndex:   j,
			}
		}
	}

	logger.Debug("Hwmon: Discovered %d fan channels", len(p.fanMap))
}

// ReadAll reads fan status using the shared lib discovery function.
func (p *HwmonProvider) ReadAll() []dto.FanDevice {
	hwmonFans := lib.DiscoverHwmonFans()
	fans := make([]dto.FanDevice, 0, len(hwmonFans))

	for _, hf := range hwmonFans {
		fan := dto.FanDevice{
			ID:         hf.ID,
			Name:       hf.Name,
			RPM:        hf.RPM,
			HwmonPath:  hf.HwmonDir,
			HwmonIndex: hf.FanIndex,
		}

		if hf.HasPWM {
			fan.Controllable = true
			fan.PWMValue = hf.PWMValue
			fan.PWMPercent = hf.PWMPercent
			fan.Mode = hwmonEnableToMode(hf.Mode)
		} else {
			fan.Mode = dto.FanModeAutomatic
		}

		fans = append(fans, fan)
	}

	return fans
}

// SetPWM writes a PWM duty cycle (0-255) to a fan's sysfs path.
func (p *HwmonProvider) SetPWM(fanID string, value int) error {
	paths, ok := p.fanMap[fanID]
	if !ok {
		return fmt.Errorf("fan %q not found in hwmon", fanID)
	}
	if _, err := os.Stat(paths.pwmPath); err != nil {
		return fmt.Errorf("fan %q does not support PWM control", fanID)
	}
	return lib.WriteSysfs(paths.pwmPath, strconv.Itoa(value))
}

// SetMode sets the PWM enable mode (0=off, 1=manual, 2=automatic).
func (p *HwmonProvider) SetMode(fanID string, mode dto.FanControlMode) error {
	paths, ok := p.fanMap[fanID]
	if !ok {
		return fmt.Errorf("fan %q not found in hwmon", fanID)
	}
	if _, err := os.Stat(paths.enablePath); err != nil {
		return fmt.Errorf("fan %q does not support mode control", fanID)
	}
	return lib.WriteSysfs(paths.enablePath, strconv.Itoa(modeToHwmonEnable(mode)))
}

// hwmonEnableToMode converts a pwm_enable sysfs value to a FanControlMode.
func hwmonEnableToMode(val int) dto.FanControlMode {
	switch val {
	case 0:
		return dto.FanModeOff
	case 1:
		return dto.FanModeManual
	default:
		return dto.FanModeAutomatic
	}
}

// modeToHwmonEnable converts a FanControlMode to a pwm_enable sysfs value.
func modeToHwmonEnable(mode dto.FanControlMode) int {
	switch mode {
	case dto.FanModeOff:
		return 0
	case dto.FanModeManual:
		return 1
	default:
		return 2
	}
}
