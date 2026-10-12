//go:build !linux

package lib

import "errors"

// ReadModuleEEPROM is only implemented on Linux.
func ReadModuleEEPROM(string) (*ModuleEEPROM, error) {
	return nil, ErrModuleUnsupported
}

// ReadFirmwareVersion is only implemented on Linux.
func ReadFirmwareVersion(string) (string, error) {
	return "", errors.ErrUnsupported
}
