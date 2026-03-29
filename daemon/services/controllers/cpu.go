package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// CPUController manages CPU scaling governor settings.
type CPUController struct {
	available        bool
	originalGovernor string
}

// NewCPUController creates a new CPU controller. Call Initialize() to check hardware.
func NewCPUController() *CPUController {
	return &CPUController{}
}

// Initialize checks whether CPU frequency scaling is available on this system.
func (c *CPUController) Initialize() error {
	gov, err := lib.ReadCPUGovernor()
	if err != nil {
		return fmt.Errorf("cpu frequency scaling not available: %w", err)
	}
	c.originalGovernor = gov
	logger.Info("CPU controller initialized — current governor: %s", gov)
	c.available = true
	return nil
}

// SetGovernor validates and applies the given CPU scaling governor to all cores.
func (c *CPUController) SetGovernor(governor string) error {
	if !c.available {
		return fmt.Errorf("cpu frequency scaling is not available on this system")
	}
	if err := lib.ValidateCPUGovernor(governor); err != nil {
		return err
	}
	if err := lib.WriteCPUGovernor(governor); err != nil {
		return fmt.Errorf("setting cpu governor: %w", err)
	}
	logger.Info("CPU governor set to %s", governor)
	return nil
}

// Shutdown restores the original CPU scaling governor that was active at initialization.
func (c *CPUController) Shutdown() {
	if !c.available || c.originalGovernor == "" {
		return
	}
	if err := lib.WriteCPUGovernor(c.originalGovernor); err != nil {
		logger.Error("CPU control: failed to restore governor to %s: %v", c.originalGovernor, err)
		return
	}
	logger.Info("CPU control: Shutdown complete, governor restored to %s", c.originalGovernor)
}
