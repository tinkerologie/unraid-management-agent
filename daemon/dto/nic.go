package dto

import "time"

// Network card collection states.
const (
	// NICStatePending means no collection has completed yet (or the nic
	// collector is disabled).
	NICStatePending = "pending"
	// NICStateOK means a collection completed.
	NICStateOK = "ok"
)

// Temperature sensor kinds reported in NICTemperature.Sensor.
const (
	// NICSensorASIC is the controller ASIC (e.g. mlx5 "asic").
	NICSensorASIC = "asic"
	// NICSensorPHY is the PHY (e.g. atlantic "PHY Temperature").
	NICSensorPHY = "phy"
	// NICSensorMAC is the MAC (e.g. atlantic "MAC Temperature").
	NICSensorMAC = "mac"
	// NICSensorModule is a transceiver module cage sensor (e.g. mlx5 "Module0").
	NICSensorModule = "module"
	// NICSensorOther is any other hwmon temperature of the card.
	NICSensorOther = "other"
)

// NICList describes the physical (PCI) network cards, their temperatures and
// ports, and the optics plugged into those ports.
// @Description Physical network cards with hwmon temperatures, port link state and SFP/QSFP optic diagnostics (DDM).
type NICList struct {
	// State is "pending" or "ok".
	State string `json:"state" example:"ok"`
	// Cards lists one entry per PCI slot (ports of a multi-function card are grouped).
	Cards []NICCard `json:"cards"`
	// Errors lists non-fatal problems hit while collecting.
	Errors []string `json:"errors,omitempty"`
	// CollectionDurationMs is how long the collection cycle took.
	CollectionDurationMs int64 `json:"collection_duration_ms" example:"12"`
	// Timestamp is when the data was collected.
	Timestamp time.Time `json:"timestamp"`
}

// NICCard is one physical network card: the PCI functions sharing a slot.
type NICCard struct {
	// ID is the PCI slot without the function (domain:bus:device), the stable card identifier.
	ID string `json:"id" example:"0000:41:00"`
	// Vendor is the PCI vendor name (lspci), or the vendor ID when unknown.
	Vendor string `json:"vendor,omitempty" example:"Mellanox Technologies"`
	// Model is a short product name derived from DeviceName.
	Model string `json:"model,omitempty" example:"ConnectX-6 Dx"`
	// DeviceName is the full PCI device name (lspci).
	DeviceName string `json:"device_name,omitempty" example:"MT2892 Family [ConnectX-6 Dx]"`
	// SubsystemName is the PCI subsystem name (lspci), when known.
	SubsystemName string `json:"subsystem_name,omitempty" example:"Device 0016"`
	// VendorID, DeviceID, SubsystemVendorID and SubsystemDeviceID are the PCI IDs (hex, no prefix).
	VendorID          string `json:"vendor_id,omitempty" example:"15b3"`
	DeviceID          string `json:"device_id,omitempty" example:"101d"`
	SubsystemVendorID string `json:"subsystem_vendor_id,omitempty" example:"15b3"`
	SubsystemDeviceID string `json:"subsystem_device_id,omitempty" example:"0016"`
	// Driver is the kernel driver bound to the card's first function.
	Driver string `json:"driver,omitempty" example:"mlx5_core"`
	// FirmwareVersion is the firmware version the driver reports (ethtool -i).
	FirmwareVersion string `json:"firmware_version,omitempty" example:"22.47.3576 (MT_0000000359)"`
	// Temperatures lists the card's hwmon temperature sensors.
	Temperatures []NICTemperature `json:"temperatures"`
	// Ports lists the card's network interfaces, ordered by PCI function and port.
	Ports []NICPort `json:"ports"`
}

// NICTemperature is one hwmon temperature sensor of a network card.
type NICTemperature struct {
	// Sensor is the sensor kind: "asic", "phy", "mac", "module" or "other".
	Sensor string `json:"sensor" example:"asic"`
	// Label is the hwmon label (temp*_label), or "temp<N>" when the driver sets none.
	Label string `json:"label" example:"asic"`
	// Interface is the first port of the sensor's PCI function (every sensor but
	// the card-wide ASIC).
	Interface string `json:"interface,omitempty" example:"eth0"`
	// PCIAddress is the PCI function whose hwmon chip reported the sensor.
	PCIAddress string `json:"pci_address" example:"0000:41:00.0"`
	// Celsius is the current reading.
	Celsius float64 `json:"celsius" example:"88"`
	// HighestCelsius is the highest reading since the driver loaded (temp*_highest), when reported.
	HighestCelsius *float64 `json:"highest_celsius,omitempty" example:"90"`
	// MaxCelsius is the high limit (temp*_max), when reported.
	MaxCelsius *float64 `json:"max_celsius,omitempty"`
	// CritCelsius is the critical limit (temp*_crit), when reported.
	CritCelsius *float64 `json:"crit_celsius,omitempty" example:"105"`
}

// NICPort is one network interface of a card.
type NICPort struct {
	// ID is the stable port identifier: the PCI function address, with "-<dev_port>"
	// appended for the second and later ports of a single function (e.g. mlx4).
	ID string `json:"id" example:"0000:41:00.0"`
	// Interface is the kernel interface name.
	Interface string `json:"interface" example:"eth0"`
	// PCIAddress is the PCI function address.
	PCIAddress string `json:"pci_address" example:"0000:41:00.0"`
	// DevPort is the port number within the PCI function (sysfs dev_port).
	DevPort int `json:"dev_port" example:"0"`
	// OperState is the RFC 2863 operational state (up, down, dormant, ...).
	OperState string `json:"operstate" example:"up"`
	// Carrier is true when the link is up.
	Carrier bool `json:"carrier" example:"true"`
	// SpeedMbps is the negotiated link speed, when the link is up.
	SpeedMbps *int `json:"speed_mbps,omitempty" example:"100000"`
	// Duplex is "full" or "half", when the link is up.
	Duplex string `json:"duplex,omitempty" example:"full"`
	// MTU is the interface MTU.
	MTU int `json:"mtu,omitempty" example:"1500"`
	// Master is the bond or bridge the interface is enslaved to, if any.
	Master string `json:"master,omitempty" example:"bond0"`
	// Module describes the plugged transceiver (absent for BASE-T ports and empty cages).
	Module *NICModule `json:"module,omitempty"`
}

// NICModule describes a pluggable transceiver (SFP/QSFP) and its digital
// diagnostics (DDM/DOM). The vendor serial number is never reported.
type NICModule struct {
	// Identifier is the SFF-8024 module type (e.g. "SFP/SFP+/SFP28", "QSFP28").
	Identifier string `json:"identifier" example:"QSFP28"`
	// Standard is the management interface decoded: "SFF-8472", "SFF-8636" or "SFF-8436"
	// (empty when the module type is not decoded, e.g. CMIS).
	Standard string `json:"standard,omitempty" example:"SFF-8636"`
	// Connector is the SFF-8024 connector type.
	Connector string `json:"connector,omitempty" example:"No separable connector"`
	// Compliance is the Ethernet compliance code (e.g. "100GBASE-SR4 or 25GBASE-SR").
	Compliance string `json:"compliance,omitempty" example:"100G AOC or 25GAUI C2M AOC (BER 5e-5)"`
	// VendorName, VendorPN and VendorRev identify the module.
	VendorName string `json:"vendor_name,omitempty" example:"CISCO-PROLINE"`
	VendorPN   string `json:"vendor_pn,omitempty" example:"QSFP-100G-AOC-3M"`
	VendorRev  string `json:"vendor_rev,omitempty" example:"D1"`
	// VendorOUI is the vendor's IEEE company ID (empty when all zero).
	VendorOUI string `json:"vendor_oui,omitempty" example:"00:02:c9"`
	// WavelengthNm is the nominal laser wavelength (optical modules only).
	WavelengthNm *float64 `json:"wavelength_nm,omitempty" example:"850"`
	// DDMSupported is true when the module reports digital diagnostics.
	DDMSupported bool `json:"ddm_supported" example:"true"`
	// DataNotReady is true while the module flags its monitors as not ready
	// (powering up or resetting); readings and flags are omitted then.
	DataNotReady bool `json:"data_not_ready,omitempty"`
	// TemperatureCelsius is the module's internal temperature.
	TemperatureCelsius *float64 `json:"temperature_celsius,omitempty" example:"50.43"`
	// VoltageVolts is the module's supply voltage.
	VoltageVolts *float64 `json:"voltage_volts,omitempty" example:"3.2676"`
	// RxPowerType is "average" or "oma" (optical modulation amplitude).
	RxPowerType string `json:"rx_power_type,omitempty" example:"average"`
	// Lanes holds per-lane laser bias and optical power (1 lane for SFP, 4 for QSFP).
	Lanes []NICModuleLane `json:"lanes,omitempty"`
	// Alarms and Warnings list the alarm and warning flags that are set, e.g.
	// "temperature_high" or "lane2_rx_power_low". SFF-8636 flags are latched:
	// they report events since the previous read.
	Alarms   []string `json:"alarms"`
	Warnings []string `json:"warnings"`
	// Thresholds are the module's alarm and warning limits, when it reports them.
	Thresholds *NICModuleThresholds `json:"thresholds,omitempty"`
	// Error explains why the module could not be read or decoded, if it could not.
	Error string `json:"error,omitempty"`
}

// NICModuleLane is one lane (channel) of a transceiver.
type NICModuleLane struct {
	// Lane is the lane number, starting at 1.
	Lane int `json:"lane" example:"1"`
	// TxBiasMA is the laser bias current in mA.
	TxBiasMA *float64 `json:"tx_bias_ma,omitempty" example:"6.846"`
	// TxPowerMW and TxPowerDBm are the transmitted optical power (absent when not measured).
	TxPowerMW  *float64 `json:"tx_power_mw,omitempty" example:"1.3692"`
	TxPowerDBm *float64 `json:"tx_power_dbm,omitempty" example:"1.36"`
	// RxPowerMW and RxPowerDBm are the received optical power. dBm is floored at
	// -40 (the 0.1 µW resolution of the measurement) when no light is received.
	RxPowerMW  *float64 `json:"rx_power_mw,omitempty" example:"2.3136"`
	RxPowerDBm *float64 `json:"rx_power_dbm,omitempty" example:"3.64"`
}

// NICModuleThresholds are a transceiver's alarm and warning limits.
type NICModuleThresholds struct {
	TemperatureCelsius *NICThreshold `json:"temperature_celsius,omitempty"`
	VoltageVolts       *NICThreshold `json:"voltage_volts,omitempty"`
	TxBiasMA           *NICThreshold `json:"tx_bias_ma,omitempty"`
	TxPowerDBm         *NICThreshold `json:"tx_power_dbm,omitempty"`
	RxPowerDBm         *NICThreshold `json:"rx_power_dbm,omitempty"`
}

// NICThreshold is one set of alarm and warning limits.
type NICThreshold struct {
	HighAlarm   float64 `json:"high_alarm" example:"90"`
	HighWarning float64 `json:"high_warning" example:"80"`
	LowWarning  float64 `json:"low_warning" example:"-10"`
	LowAlarm    float64 `json:"low_alarm" example:"-20"`
}

// NewPendingNICList returns the placeholder served before the first collection
// completes (or while the nic collector is disabled).
func NewPendingNICList(now time.Time) *NICList {
	return &NICList{
		State:     NICStatePending,
		Cards:     []NICCard{},
		Timestamp: now,
	}
}
