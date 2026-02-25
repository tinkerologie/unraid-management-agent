package collectors

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"gopkg.in/ini.v1"
)

// RegistrationCollector collects Unraid registration/license information
type RegistrationCollector struct {
	ctx *domain.Context
}

// NewRegistrationCollector creates a new registration collector
func NewRegistrationCollector(ctx *domain.Context) *RegistrationCollector {
	return &RegistrationCollector{ctx: ctx}
}

// Start begins collecting registration information at the specified interval
func (c *RegistrationCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting registration collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Registration collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Registration collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Registration collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers registration information
func (c *RegistrationCollector) Collect() {
	logger.Debug("Collecting registration data...")

	registration, err := c.collectRegistration()
	if err != nil {
		logger.Error("Registration: Failed to collect registration info: %v", err)
		return
	}

	logger.Debug("Registration: Successfully collected, publishing event")
	domain.Publish(c.ctx.Hub, constants.TopicRegistrationUpdate, registration)
	logger.Debug("Registration: Published %s event - type=%s, state=%s", constants.TopicRegistrationUpdate.Name, registration.Type, registration.State)
}

// collectRegistration reads registration information from var.ini
func (c *RegistrationCollector) collectRegistration() (*dto.Registration, error) {
	logger.Debug("Registration: Reading from %s", constants.VarIni)

	registration := &dto.Registration{
		Timestamp: time.Now(),
	}

	// Parse var.ini for registration information
	cfg, err := ini.Load(constants.VarIni)
	if err != nil {
		logger.Error("Registration: Failed to load file: %v", err)
		return nil, err
	}

	// Get the default section (unnamed section)
	section := cfg.Section("")

	// Registration type (regTy)
	if section.HasKey("regTy") {
		regType := strings.Trim(section.Key("regTy").String(), `"`)
		registration.Type = strings.ToLower(regType)
	} else {
		registration.Type = "unknown"
	}

	// Registration GUID (regGUID)
	if section.HasKey("regGUID") {
		registration.GUID = strings.Trim(section.Key("regGUID").String(), `"`)
	}

	// Server name (NAME)
	if section.HasKey("NAME") {
		registration.ServerName = strings.Trim(section.Key("NAME").String(), `"`)
	}

	// Registration timestamp/expiration (regTm)
	if section.HasKey("regTm") {
		regTmStr := strings.Trim(section.Key("regTm").String(), `"`)
		if timestamp, err := strconv.ParseInt(regTmStr, 10, 64); err == nil {
			registration.Expiration = time.Unix(timestamp, 0)
			registration.UpdateExpiration = time.Unix(timestamp, 0)
		}
	}

	// Determine state based on expiration
	switch {
	case !registration.Expiration.IsZero():
		if time.Now().After(registration.Expiration) {
			registration.State = "expired"
		} else {
			registration.State = "valid"
		}
	case registration.Type == "trial":
		registration.State = "trial"
	case registration.Type == "lifetime" || registration.Type == "unleashed":
		registration.State = "valid"
	case registration.Type == "unknown":
		registration.State = "invalid"
	default:
		registration.State = "valid"
	}

	logger.Debug("Registration: Parsed - type=%s, state=%s, server=%s",
		registration.Type, registration.State, registration.ServerName)

	return registration, nil
}
