package lib

import "errors"

// ErrModuleUnsupported means the interface's driver cannot read module EEPROMs
// (e.g. BASE-T NICs, virtual interfaces).
var ErrModuleUnsupported = errors.New("module EEPROM not supported by driver")

// ErrNoModule means the driver supports module EEPROMs but no readable module
// is plugged in (empty cage, or a module type the driver cannot read).
var ErrNoModule = errors.New("no readable module")

// ModuleEEPROM is the raw EEPROM of a pluggable module, as returned by the
// kernel's ethtool module interface (the same data `ethtool -m <if> raw on` dumps).
type ModuleEEPROM struct {
	// Type is the EEPROM layout (ModuleTypeSFF8079 ... ModuleTypeSFF8436).
	Type uint32
	// Data is the EEPROM content: 256, 512 or 640 bytes depending on Type.
	Data []byte
}
