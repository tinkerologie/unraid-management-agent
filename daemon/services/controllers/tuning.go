package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// TuningController manages system tuning parameters (turbo boost, disk cache, inotify).
type TuningController struct {
	turboAvailable    bool
	originalTurbo     bool
	originalDiskCache *lib.DiskCacheSettings
	originalInotify   *lib.InotifySettings
}

// NewTuningController creates a new tuning controller. Call Initialize() to detect hardware.
func NewTuningController() *TuningController {
	return &TuningController{}
}

// Initialize detects tuning capabilities and saves original values for restore on shutdown.
func (c *TuningController) Initialize() error {
	turbo := lib.ReadTurboBoost()
	c.turboAvailable = turbo.Available
	c.originalTurbo = turbo.Enabled
	if turbo.Available {
		logger.Info("Tuning controller: turbo boost available (%s), currently %s",
			turbo.Vendor, boolToOnOff(turbo.Enabled))
	}

	dc, err := lib.ReadDiskCacheSettings()
	if err == nil {
		c.originalDiskCache = dc
		logger.Debug("Tuning controller: disk cache settings saved")
	} else {
		logger.Debug("Tuning controller: failed to read disk cache settings: %v", err)
	}

	ino, err := lib.ReadInotifySettings()
	if err == nil {
		c.originalInotify = ino
		logger.Debug("Tuning controller: inotify settings saved")
	} else {
		logger.Debug("Tuning controller: failed to read inotify settings: %v", err)
	}

	logger.Info("Tuning controller initialized")
	return nil
}

// SetTurboBoost enables or disables CPU turbo/performance boost.
func (c *TuningController) SetTurboBoost(enabled bool) error {
	if !c.turboAvailable {
		return fmt.Errorf("turbo/boost control is not available on this system")
	}
	if err := lib.WriteTurboBoost(enabled); err != nil {
		return fmt.Errorf("setting turbo boost: %w", err)
	}
	logger.Info("Turbo boost set to %s", boolToOnOff(enabled))
	return nil
}

// SetDiskCache writes vm.dirty_* kernel parameters.
func (c *TuningController) SetDiskCache(bgRatio, ratio, writebackCenti, expireCenti int) error {
	if bgRatio < 0 || bgRatio > 100 {
		return fmt.Errorf("dirty_background_ratio must be 0-100, got %d", bgRatio)
	}
	if ratio < 0 || ratio > 100 {
		return fmt.Errorf("dirty_ratio must be 0-100, got %d", ratio)
	}
	if bgRatio > ratio {
		return fmt.Errorf("dirty_background_ratio (%d) must not exceed dirty_ratio (%d)", bgRatio, ratio)
	}
	if writebackCenti < 0 {
		return fmt.Errorf("dirty_writeback_centisecs must be non-negative, got %d", writebackCenti)
	}
	if expireCenti < 0 {
		return fmt.Errorf("dirty_expire_centisecs must be non-negative, got %d", expireCenti)
	}

	// Order writes to avoid kernel rejection: dirty_background_ratio must be <= dirty_ratio.
	// If increasing bgRatio above current dirty_ratio, write dirty_ratio first.
	currentDirtyRatio, err := lib.ReadSysctlInt("vm.dirty_ratio")
	if err != nil || bgRatio > currentDirtyRatio {
		// Write ratio first so bgRatio <= ratio holds at every step
		if err := lib.WriteSysctl("vm.dirty_ratio", fmt.Sprintf("%d", ratio)); err != nil {
			return fmt.Errorf("setting dirty_ratio: %w", err)
		}
		if err := lib.WriteSysctl("vm.dirty_background_ratio", fmt.Sprintf("%d", bgRatio)); err != nil {
			return fmt.Errorf("setting dirty_background_ratio: %w", err)
		}
	} else {
		// Write bgRatio first (safe because bgRatio <= currentDirtyRatio)
		if err := lib.WriteSysctl("vm.dirty_background_ratio", fmt.Sprintf("%d", bgRatio)); err != nil {
			return fmt.Errorf("setting dirty_background_ratio: %w", err)
		}
		if err := lib.WriteSysctl("vm.dirty_ratio", fmt.Sprintf("%d", ratio)); err != nil {
			return fmt.Errorf("setting dirty_ratio: %w", err)
		}
	}
	if err := lib.WriteSysctl("vm.dirty_writeback_centisecs", fmt.Sprintf("%d", writebackCenti)); err != nil {
		return fmt.Errorf("setting dirty_writeback_centisecs: %w", err)
	}
	if err := lib.WriteSysctl("vm.dirty_expire_centisecs", fmt.Sprintf("%d", expireCenti)); err != nil {
		return fmt.Errorf("setting dirty_expire_centisecs: %w", err)
	}

	logger.Info("Disk cache updated: bg_ratio=%d, ratio=%d, writeback=%dcs, expire=%dcs",
		bgRatio, ratio, writebackCenti, expireCenti)
	return nil
}

// SetInotifyLimits writes inotify kernel parameters.
func (c *TuningController) SetInotifyLimits(maxWatches, maxInstances, maxEvents int) error {
	if maxWatches < 1 {
		return fmt.Errorf("max_user_watches must be positive, got %d", maxWatches)
	}
	if maxInstances < 1 {
		return fmt.Errorf("max_user_instances must be positive, got %d", maxInstances)
	}
	if maxEvents < 1 {
		return fmt.Errorf("max_queued_events must be positive, got %d", maxEvents)
	}

	if err := lib.WriteSysctl("fs.inotify.max_user_watches", fmt.Sprintf("%d", maxWatches)); err != nil {
		return fmt.Errorf("setting max_user_watches: %w", err)
	}
	if err := lib.WriteSysctl("fs.inotify.max_user_instances", fmt.Sprintf("%d", maxInstances)); err != nil {
		return fmt.Errorf("setting max_user_instances: %w", err)
	}
	if err := lib.WriteSysctl("fs.inotify.max_queued_events", fmt.Sprintf("%d", maxEvents)); err != nil {
		return fmt.Errorf("setting max_queued_events: %w", err)
	}

	logger.Info("Inotify limits updated: watches=%d, instances=%d, events=%d",
		maxWatches, maxInstances, maxEvents)
	return nil
}

// Shutdown restores original tuning parameters.
func (c *TuningController) Shutdown() {
	if c.turboAvailable {
		if err := lib.WriteTurboBoost(c.originalTurbo); err != nil {
			logger.Error("Tuning: failed to restore turbo boost: %v", err)
		} else {
			logger.Info("Tuning: turbo boost restored to %s", boolToOnOff(c.originalTurbo))
		}
	}

	if c.originalDiskCache != nil {
		dc := c.originalDiskCache
		if err := c.SetDiskCache(dc.DirtyBackgroundRatio, dc.DirtyRatio, dc.DirtyWritebackCenti, dc.DirtyExpireCenti); err != nil {
			logger.Error("Tuning: failed to restore disk cache settings: %v", err)
		} else {
			logger.Info("Tuning: disk cache settings restored")
		}
	}

	if c.originalInotify != nil {
		ino := c.originalInotify
		if err := c.SetInotifyLimits(ino.MaxUserWatches, ino.MaxUserInstances, ino.MaxQueuedEvents); err != nil {
			logger.Error("Tuning: failed to restore inotify limits: %v", err)
		} else {
			logger.Info("Tuning: inotify limits restored")
		}
	}

	logger.Info("Tuning controller: shutdown complete")
}

func boolToOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
