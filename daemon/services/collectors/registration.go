package collectors

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"gopkg.in/ini.v1"
)

// RegistrationCollector collects Unraid registration/license information
type RegistrationCollector struct {
	ctx        *domain.Context
	varIniPath string // path to var.ini; injectable for tests
}

// NewRegistrationCollector creates a new registration collector
func NewRegistrationCollector(ctx *domain.Context) *RegistrationCollector {
	return &RegistrationCollector{ctx: ctx, varIniPath: constants.VarIni}
}

// Start begins collecting registration information at the specified interval
func (c *RegistrationCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting registration collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Registration collector", r)
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
						logger.LogPanicWithStack("Registration collector", r)
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
	logger.Debug("Registration: Reading from %s", c.varIniPath)

	// Parse var.ini for registration information
	cfg, err := ini.Load(c.varIniPath)
	if err != nil {
		logger.Error("Registration: Failed to load file: %v", err)
		return nil, err
	}

	// Get the default section (unnamed section)
	registration := parseRegistration(cfg.Section(""), time.Now())

	logger.Debug("Registration: Parsed - type=%s, state=%s, server=%s",
		registration.Type, registration.State, registration.ServerName)

	return registration, nil
}

// parseRegistration builds the registration from the var.ini default section.
// regTm is when the key was registered, not when it expires. Unraid's webGui
// reads regTm2 as the expiry of trial and expired keys and regExp as the
// update-eligibility date, which is empty for keys that never lose updates.
func parseRegistration(section *ini.Section, now time.Time) *dto.Registration {
	registration := &dto.Registration{Timestamp: now}

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

	expired := strings.Contains(registration.Type, "expired")
	if ts, ok := unixKey(section, "regTm2"); ok && (registration.Type == "trial" || expired) {
		registration.Expiration = ts
	}
	if ts, ok := unixKey(section, "regExp"); ok {
		registration.UpdateExpiration = ts
	}

	switch {
	case expired:
		registration.State = "expired"
	case registration.Type == "trial":
		if !registration.Expiration.IsZero() && now.After(registration.Expiration) {
			registration.State = "expired"
		} else {
			registration.State = "trial"
		}
	case registration.Type == "unknown":
		registration.State = "invalid"
	default:
		registration.State = "valid"
	}

	return registration
}

// unixKey reads a Unix timestamp key, reporting false when it is missing,
// empty, zero, or not a number.
func unixKey(section *ini.Section, key string) (time.Time, bool) {
	if !section.HasKey(key) {
		return time.Time{}, false
	}
	ts, err := strconv.ParseInt(strings.Trim(section.Key(key).String(), `"`), 10, 64)
	if err != nil || ts <= 0 {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}
