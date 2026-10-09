package dto

import "time"

// Storage topology collection states.
const (
	// StorageTopologyStatePending means no collection has completed yet (or the
	// storage_topology collector is disabled).
	StorageTopologyStatePending = "pending"
	// StorageTopologyStateOK means a collection completed using at least one source.
	StorageTopologyStateOK = "ok"
	// StorageTopologyStateUnsupported means neither storcli nor sg_ses with SES
	// devices is available on this system.
	StorageTopologyStateUnsupported = "unsupported"
)

// StorageTopology describes the SAS storage topology: RAID/HBA controllers and
// their ports, SES enclosures (shelves/backplanes) with their power supplies,
// fans, sensors, I/O modules and cabling, and the drives behind them.
// @Description SAS storage topology from storcli (Broadcom/LSI MegaRAID and HBAs) and sg_ses (SCSI Enclosure Services).
type StorageTopology struct {
	// State is one of "pending", "ok" or "unsupported".
	State string `json:"state" example:"ok"`
	// Sources reports which data sources were available for this collection.
	Sources StorageTopologySources `json:"sources"`
	// Controllers lists the storcli-managed controllers (empty without storcli).
	Controllers []StorageController `json:"controllers"`
	// Enclosures lists SES enclosures, merged across storcli and every SES path.
	Enclosures []StorageEnclosure `json:"enclosures"`
	// Drives lists the physical drives reported by storcli (empty without storcli).
	Drives []StorageDrive `json:"drives"`
	// Summary holds topology-wide counts derived from the lists above.
	Summary StorageTopologySummary `json:"summary"`
	// Errors lists non-fatal problems hit while collecting (timeouts, parse errors).
	Errors []string `json:"errors,omitempty"`
	// CollectionDurationMs is how long the collection cycle took.
	CollectionDurationMs int64 `json:"collection_duration_ms" example:"230"`
	// Timestamp is when the data was collected.
	Timestamp time.Time `json:"timestamp"`
}

// StorageTopologySources reports the data sources used for a collection.
type StorageTopologySources struct {
	// Storcli is true when storcli was found and reported at least one controller.
	Storcli bool `json:"storcli" example:"true"`
	// StorcliPath is the storcli binary that was used.
	StorcliPath string `json:"storcli_path,omitempty" example:"/sbin/storcli"`
	// StorcliVersion is the storcli CLI version string.
	StorcliVersion string `json:"storcli_version,omitempty" example:"007.3305.0000.0000 Jan 23, 2025"`
	// SES is true when sg_ses (with JSON output support) read at least one SES device.
	SES bool `json:"ses" example:"true"`
	// SESDevices is the number of SES devices (one per enclosure I/O module path) read.
	SESDevices int `json:"ses_devices" example:"4"`
}

// StorageController is a storcli-managed RAID controller or HBA.
type StorageController struct {
	// ID is the stable controller identifier: the serial number when known, else "c<index>".
	ID string `json:"id" example:"SPC1234567"`
	// Index is the storcli controller index (/cN).
	Index int `json:"index" example:"0"`
	// Model is the controller model.
	Model string `json:"model" example:"MegaRAID 9580-8i8e"`
	// SerialNumber is the controller serial number.
	SerialNumber string `json:"serial_number,omitempty"`
	// SASAddress is the controller SAS address ("0x" + 16 lowercase hex digits).
	SASAddress string `json:"sas_address,omitempty" example:"0x500062b200000000"`
	// FirmwareVersion is the controller firmware version.
	FirmwareVersion string `json:"firmware_version,omitempty" example:"5.310.02-4101"`
	// FirmwarePackage is the firmware package build.
	FirmwarePackage string `json:"firmware_package,omitempty" example:"52.31.0-5827"`
	// BIOSVersion is the option ROM/BIOS version.
	BIOSVersion string `json:"bios_version,omitempty"`
	// DriverName is the Linux driver bound to the controller.
	DriverName string `json:"driver_name,omitempty" example:"megaraid_sas"`
	// DriverVersion is the Linux driver version.
	DriverVersion string `json:"driver_version,omitempty"`
	// Personality is the controller personality (e.g. "JBOD-Mode", "RAID-Mode").
	Personality string `json:"personality,omitempty" example:"JBOD-Mode"`
	// Status is the controller status reported by storcli (e.g. "Optimal").
	Status string `json:"status,omitempty" example:"Optimal"`
	// TemperatureCelsius is the ROC (RAID-on-chip) or controller temperature.
	TemperatureCelsius *float64 `json:"temperature_celsius,omitempty" example:"62"`
	// PCIAddress is the PCI address of the controller (domain:bus:device.function).
	PCIAddress string `json:"pci_address,omitempty" example:"0000:02:00.0"`
	// PCIeLinkSpeed is the negotiated PCIe link speed from sysfs.
	PCIeLinkSpeed string `json:"pcie_link_speed,omitempty" example:"16.0 GT/s PCIe"`
	// PCIeLinkWidth is the negotiated PCIe link width (lanes).
	PCIeLinkWidth *int `json:"pcie_link_width,omitempty" example:"8"`
	// PCIeMaxLinkSpeed is the maximum PCIe link speed supported by the slot/device.
	PCIeMaxLinkSpeed string `json:"pcie_max_link_speed,omitempty" example:"16.0 GT/s PCIe"`
	// PCIeMaxLinkWidth is the maximum PCIe link width.
	PCIeMaxLinkWidth *int `json:"pcie_max_link_width,omitempty" example:"8"`
	// MemoryCorrectableErrors is the controller memory correctable error count.
	MemoryCorrectableErrors *int `json:"memory_correctable_errors,omitempty" example:"0"`
	// MemoryUncorrectableErrors is the controller memory uncorrectable error count.
	MemoryUncorrectableErrors *int `json:"memory_uncorrectable_errors,omitempty" example:"0"`
	// BBUStatus is the battery/CacheVault status ("NA" when absent).
	BBUStatus string `json:"bbu_status,omitempty" example:"NA"`
	// PhysicalDrives is the number of physical drives attached to the controller.
	PhysicalDrives int `json:"physical_drives" example:"43"`
	// Ports lists the connected SAS ports (phys grouped by port number).
	Ports []StorageControllerPort `json:"ports"`
	// Phys lists every controller phy, connected or not.
	Phys []StorageControllerPhy `json:"phys"`
}

// StorageControllerPort is a connected (possibly wide) SAS port of a controller.
type StorageControllerPort struct {
	// Port is the storcli port number.
	Port int `json:"port" example:"0"`
	// Phys lists the phys that form this port.
	Phys []int `json:"phys"`
	// Width is the number of linked phys (lanes) in the port.
	Width int `json:"width" example:"4"`
	// LinkRateGbps is the lowest negotiated link rate across the port's phys.
	LinkRateGbps *float64 `json:"link_rate_gbps,omitempty" example:"12"`
	// AttachedSASAddress is the SAS address of the attached device.
	AttachedSASAddress string `json:"attached_sas_address,omitempty" example:"0x500a098000000000"`
	// AttachedDeviceType is the attached device type (e.g. "Edge Expander").
	AttachedDeviceType string `json:"attached_device_type,omitempty" example:"Edge Expander"`
	// AttachedEnclosureID is the ID of the enclosure whose I/O module is attached, when resolved.
	AttachedEnclosureID string `json:"attached_enclosure_id,omitempty"`
	// AttachedIOM is the index of the attached enclosure I/O module, when resolved.
	AttachedIOM *int `json:"attached_iom,omitempty" example:"1"`
}

// StorageControllerPhy is a single controller phy (lane).
type StorageControllerPhy struct {
	// Phy is the phy number.
	Phy int `json:"phy" example:"0"`
	// Port is the port this phy belongs to; nil when the phy has no valid port.
	Port *int `json:"port,omitempty" example:"0"`
	// Connected is true when the phy has a link to a device.
	Connected bool `json:"connected" example:"true"`
	// LinkRateGbps is the negotiated link rate; nil when not linked.
	LinkRateGbps *float64 `json:"link_rate_gbps,omitempty" example:"12"`
	// AttachedSASAddress is the SAS address of the attached device.
	AttachedSASAddress string `json:"attached_sas_address,omitempty"`
	// AttachedDeviceType is the attached device type.
	AttachedDeviceType string `json:"attached_device_type,omitempty"`
	// AttachedPhy is the phy identifier on the attached device.
	AttachedPhy *int `json:"attached_phy,omitempty"`
	// Enabled is false when the phy has been disabled on the controller.
	Enabled bool `json:"enabled" example:"true"`
}

// StorageEnclosure is a SES enclosure (disk shelf or backplane).
type StorageEnclosure struct {
	// ID is the stable enclosure identifier: the enclosure logical identifier
	// (16 lowercase hex digits) when known, else "c<controller>-e<eid>" or the SES device name.
	ID string `json:"id" example:"50050cc100000000"`
	// LogicalID is the SES enclosure logical identifier ("0x" + 16 hex digits).
	LogicalID string `json:"logical_id,omitempty" example:"0x50050cc100000000"`
	// Vendor is the enclosure vendor.
	Vendor string `json:"vendor,omitempty" example:"NETAPP"`
	// Product is the enclosure product identification.
	Product string `json:"product,omitempty" example:"DS424IOM12A"`
	// SerialNumber is the enclosure (chassis) serial number.
	SerialNumber string `json:"serial_number,omitempty"`
	// ControllerIndex is the storcli controller the enclosure is attached to.
	ControllerIndex *int `json:"controller_index,omitempty" example:"0"`
	// EnclosureDeviceID is the storcli enclosure device ID (EID).
	EnclosureDeviceID *int `json:"enclosure_device_id,omitempty" example:"242"`
	// PartnerDeviceID is the storcli device ID of the partner (second I/O module) path.
	PartnerDeviceID *int `json:"partner_device_id,omitempty" example:"241"`
	// ConnectorName is the storcli connector description (e.g. "C1 x4 & C2 x4").
	ConnectorName string `json:"connector_name,omitempty" example:"C1 x4 & C2 x4"`
	// PortMode is the storcli port mode (e.g. "Multipath").
	PortMode string `json:"port_mode,omitempty" example:"Multipath"`
	// Status is the worst element status (OK, Noncritical, Critical, Unrecoverable) or the storcli state.
	Status string `json:"status" example:"OK"`
	// Slots is the number of drive slots.
	Slots int `json:"slots" example:"24"`
	// SlotsPopulated is the number of occupied drive slots, when known.
	SlotsPopulated *int `json:"slots_populated,omitempty" example:"22"`
	// SESDevices lists the SES devices (one per I/O module path) used for this enclosure.
	SESDevices []StorageSESDevice `json:"ses_devices"`
	// PowerSupplies lists the power supply elements.
	PowerSupplies []EnclosurePowerSupply `json:"power_supplies"`
	// Fans lists the cooling elements.
	Fans []EnclosureFan `json:"fans"`
	// TemperatureSensors lists temperature sensor elements (degrees Celsius).
	TemperatureSensors []EnclosureSensor `json:"temperature_sensors"`
	// VoltageSensors lists voltage sensor elements (volts).
	VoltageSensors []EnclosureSensor `json:"voltage_sensors"`
	// CurrentSensors lists current sensor elements (amps).
	CurrentSensors []EnclosureSensor `json:"current_sensors"`
	// IOMs lists the enclosure services controller electronics (I/O modules, "SIMs" in storcli).
	IOMs []EnclosureIOM `json:"ioms"`
	// Connectors lists the SAS connector elements with their cable/attachment details.
	Connectors []EnclosureConnector `json:"connectors"`
	// SlotDetails lists the device slot elements.
	SlotDetails []EnclosureSlot `json:"slot_details"`
	// IOMFirmwareMismatch is true when this enclosure's I/O modules run different firmware.
	IOMFirmwareMismatch bool `json:"iom_firmware_mismatch" example:"false"`
	// IOMFirmwareDiffersFromPeers is true when another enclosure of the same vendor and
	// product runs different I/O module firmware.
	IOMFirmwareDiffersFromPeers bool `json:"iom_firmware_differs_from_peers" example:"true"`
	// Redundancy describes the host path redundancy of the enclosure.
	Redundancy EnclosureRedundancy `json:"redundancy"`
	// Problems lists the current problems detected in this enclosure.
	Problems []string `json:"problems"`
}

// StorageSESDevice is one SES access path (one I/O module) to an enclosure.
type StorageSESDevice struct {
	// Device is the SCSI generic device name.
	Device string `json:"device" example:"sg4"`
	// Revision is the INQUIRY product revision of the SES target (I/O module firmware).
	Revision string `json:"revision,omitempty" example:"0281"`
	// ReportingIOM is the index of the I/O module that serves this SES path.
	ReportingIOM *int `json:"reporting_iom,omitempty" example:"1"`
	// ExpanderSASAddress is the SAS address of that I/O module's expander.
	ExpanderSASAddress string `json:"expander_sas_address,omitempty"`
}

// EnclosurePowerSupply is a SES power supply element.
type EnclosurePowerSupply struct {
	Index        int    `json:"index" example:"0"`
	Status       string `json:"status" example:"OK"`
	Problem      bool   `json:"problem" example:"false"`
	Description  string `json:"description,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`
	Firmware     string `json:"firmware,omitempty" example:"0311"`
	PartNumber   string `json:"part_number,omitempty"`
	// RatedWatts is the rated output power, when the enclosure reports it.
	RatedWatts       *int `json:"rated_watts,omitempty" example:"580"`
	Fail             bool `json:"fail"`
	ACFail           bool `json:"ac_fail"`
	DCFail           bool `json:"dc_fail"`
	OverTempFail     bool `json:"over_temp_fail"`
	TempWarning      bool `json:"temp_warning"`
	DCOverVoltage    bool `json:"dc_over_voltage"`
	DCUnderVoltage   bool `json:"dc_under_voltage"`
	DCOverCurrent    bool `json:"dc_over_current"`
	Off              bool `json:"off"`
	PredictedFailure bool `json:"predicted_failure"`
}

// EnclosureFan is a SES cooling element.
type EnclosureFan struct {
	Index       int    `json:"index" example:"0"`
	Status      string `json:"status" example:"OK"`
	Problem     bool   `json:"problem" example:"false"`
	Description string `json:"description,omitempty"`
	// RPM is the actual fan speed; nil when only a speed band is known (storcli fallback).
	RPM *int `json:"rpm,omitempty" example:"3300"`
	// Speed describes the speed band (e.g. "at lowest speed", "Low Speed").
	Speed string `json:"speed,omitempty" example:"at lowest speed"`
	Fail  bool   `json:"fail"`
	Off   bool   `json:"off"`
}

// EnclosureSensor is a SES temperature, voltage or current sensor element.
type EnclosureSensor struct {
	Index       int    `json:"index" example:"0"`
	Status      string `json:"status" example:"OK"`
	Problem     bool   `json:"problem" example:"false"`
	Description string `json:"description,omitempty"`
	// Value is the reading (°C, V or A); nil when the element reports no valid reading.
	Value     *float64 `json:"value,omitempty" example:"25"`
	Fail      bool     `json:"fail"`
	WarnOver  bool     `json:"warn_over"`
	WarnUnder bool     `json:"warn_under"`
	CritOver  bool     `json:"crit_over"`
	CritUnder bool     `json:"crit_under"`
}

// EnclosureIOM is a SES enclosure services controller electronics element (I/O module).
type EnclosureIOM struct {
	Index       int    `json:"index" example:"0"`
	Status      string `json:"status" example:"OK"`
	Problem     bool   `json:"problem" example:"false"`
	Description string `json:"description,omitempty"`
	// SerialNumber, Firmware and PartNumber come from the element descriptor when the
	// enclosure publishes them; Firmware falls back to the SES target's INQUIRY revision.
	SerialNumber string `json:"serial_number,omitempty"`
	Firmware     string `json:"firmware,omitempty" example:"0281"`
	PartNumber   string `json:"part_number,omitempty"`
	// ExpanderSASAddress is the SAS address of the module's expander, when known.
	ExpanderSASAddress string `json:"expander_sas_address,omitempty"`
	// HostVisible is true when the host reaches this module's SES service directly.
	HostVisible bool `json:"host_visible" example:"true"`
	Fail        bool `json:"fail"`
}

// EnclosureConnector is a SES SAS connector element and what is plugged into it.
type EnclosureConnector struct {
	Index       int    `json:"index" example:"7"`
	Status      string `json:"status" example:"OK"`
	Problem     bool   `json:"problem" example:"false"`
	Description string `json:"description,omitempty"`
	// Type is the connector type (e.g. "Mini SAS HD 4x receptacle (SFF-8644) [max 4 phys]").
	Type string `json:"type,omitempty"`
	// Installed is false for connector positions that are not populated.
	Installed bool `json:"installed" example:"true"`
	Fail      bool `json:"fail"`
	// AttachedSASAddress is the SAS address at the far end of the cable, when the
	// enclosure publishes it.
	AttachedSASAddress string `json:"attached_sas_address,omitempty"`
	// AttachedPhy is the phy number at the far end, when published.
	AttachedPhy *int `json:"attached_phy,omitempty" example:"0"`
	// AttachedKind is "controller" or "enclosure" when the far end was resolved.
	AttachedKind string `json:"attached_kind,omitempty" example:"controller"`
	// AttachedID is the controller or enclosure ID at the far end.
	AttachedID string `json:"attached_id,omitempty"`
	// AttachedPort is the controller port at the far end (controller attachments).
	AttachedPort *int `json:"attached_port,omitempty" example:"0"`
	// AttachedIOM is the I/O module index at the far end (enclosure attachments).
	AttachedIOM       *int   `json:"attached_iom,omitempty"`
	CableVendor       string `json:"cable_vendor,omitempty"`
	CablePartNumber   string `json:"cable_part_number,omitempty"`
	CableSerialNumber string `json:"cable_serial_number,omitempty"`
}

// EnclosureSlot is a SES device slot element.
type EnclosureSlot struct {
	Index   int    `json:"index" example:"0"`
	Status  string `json:"status" example:"OK"`
	Problem bool   `json:"problem" example:"false"`
	// Occupied is true when the slot holds a device.
	Occupied bool `json:"occupied" example:"true"`
	// Fault is true when the enclosure senses or was asked to show a fault for the slot.
	Fault bool `json:"fault"`
	// Identify is true when the identify (locate) indicator is on.
	Identify bool `json:"identify"`
}

// EnclosureRedundancy describes how many host paths reach an enclosure.
type EnclosureRedundancy struct {
	// ExpectedPaths is the number of installed I/O modules (each can provide a host path).
	ExpectedPaths int `json:"expected_paths" example:"2"`
	// ActivePaths is the number of distinct host paths currently in use: distinct
	// controller ports used by the enclosure's drives, else distinct SES paths.
	ActivePaths int `json:"active_paths" example:"2"`
	// SinglePathDrives is the number of dual-ported drives with fewer than two active paths.
	SinglePathDrives int `json:"single_path_drives" example:"0"`
	// Degraded is true when the enclosure has redundant I/O modules but lost a path.
	Degraded bool `json:"degraded" example:"false"`
	// Reasons explains why Degraded is true.
	Reasons []string `json:"reasons"`
}

// StorageDrive is a physical drive reported by storcli.
type StorageDrive struct {
	// ControllerIndex is the storcli controller index.
	ControllerIndex int `json:"controller_index" example:"0"`
	// EnclosureDeviceID is the storcli enclosure device ID (EID); nil for direct-attached drives.
	EnclosureDeviceID *int `json:"enclosure_device_id,omitempty" example:"242"`
	// EnclosureID is the ID of the enclosure in Enclosures, when known.
	EnclosureID string `json:"enclosure_id,omitempty"`
	// Slot is the slot number.
	Slot int `json:"slot" example:"0"`
	// DeviceID is the storcli device ID (DID).
	DeviceID *int `json:"device_id,omitempty" example:"97"`
	// Device is the Linux block device name, matched by serial number.
	Device string `json:"device,omitempty" example:"sdy"`
	// State is the storcli drive state (e.g. "Onln", "JBOD", "UGood").
	State string `json:"state,omitempty" example:"Onln"`
	// Interface is the drive interface (SAS, SATA, NVMe).
	Interface string `json:"interface,omitempty" example:"SAS"`
	// Media is the media type (HDD, SSD).
	Media        string `json:"media,omitempty" example:"HDD"`
	Model        string `json:"model,omitempty" example:"ST24000NM007H"`
	Vendor       string `json:"vendor,omitempty" example:"SEAGATE"`
	SerialNumber string `json:"serial_number,omitempty"`
	WWN          string `json:"wwn,omitempty"`
	Firmware     string `json:"firmware,omitempty" example:"EE05"`
	Size         string `json:"size,omitempty" example:"21.828 TB"`
	// TemperatureCelsius is the drive temperature reported through the controller.
	TemperatureCelsius *float64 `json:"temperature_celsius,omitempty" example:"37"`
	// MaxLinkRateGbps is the drive's maximum interface speed ("Device Speed").
	MaxLinkRateGbps *float64 `json:"max_link_rate_gbps,omitempty" example:"12"`
	// LinkRateGbps is the negotiated link speed.
	LinkRateGbps *float64 `json:"link_rate_gbps,omitempty" example:"6"`
	// BelowMaxLinkRate is true when the negotiated link speed is below the drive's maximum.
	BelowMaxLinkRate        bool `json:"below_max_link_rate" example:"true"`
	MediaErrors             *int `json:"media_errors,omitempty" example:"0"`
	OtherErrors             *int `json:"other_errors,omitempty" example:"0"`
	PredictiveFailures      *int `json:"predictive_failures,omitempty" example:"0"`
	UnrecoveredMediumErrors *int `json:"unrecovered_medium_errors,omitempty" example:"0"`
	ShieldCounter           *int `json:"shield_counter,omitempty" example:"0"`
	// SMARTAlert is true when the drive flagged a S.M.A.R.T. alert.
	SMARTAlert bool `json:"smart_alert" example:"false"`
	// Multipath is true when the controller sees the drive through more than one path.
	Multipath bool `json:"multipath" example:"true"`
	// ControllerPorts lists the controller port used by each path, in path order.
	ControllerPorts []int `json:"controller_ports"`
	// Ports lists the drive's own SAS ports.
	Ports []StorageDrivePort `json:"ports"`
	// ActivePaths is the number of drive ports in the "Active" state.
	ActivePaths int `json:"active_paths" example:"2"`
}

// StorageDrivePort is one SAS port of a drive.
type StorageDrivePort struct {
	Port         int      `json:"port" example:"0"`
	Status       string   `json:"status,omitempty" example:"Active"`
	LinkRateGbps *float64 `json:"link_rate_gbps,omitempty" example:"6"`
	SASAddress   string   `json:"sas_address,omitempty"`
}

// StorageTopologySummary holds topology-wide counts.
type StorageTopologySummary struct {
	Controllers int `json:"controllers" example:"1"`
	Enclosures  int `json:"enclosures" example:"2"`
	Drives      int `json:"drives" example:"43"`
	// DrivesBelowMaxLinkRate counts drives negotiated below their maximum interface speed.
	DrivesBelowMaxLinkRate int `json:"drives_below_max_link_rate" example:"26"`
	// DrivesWithMediaErrors counts drives with a non-zero media error count.
	DrivesWithMediaErrors int `json:"drives_with_media_errors" example:"0"`
	// DrivesWithOtherErrors counts drives with a non-zero "other" (transport/link) error count.
	DrivesWithOtherErrors int `json:"drives_with_other_errors" example:"0"`
	// DrivesWithPredictiveFailure counts drives with predictive failures or a S.M.A.R.T. alert.
	DrivesWithPredictiveFailure int `json:"drives_with_predictive_failure" example:"0"`
	// SinglePathDrives counts dual-ported drives in redundant enclosures with one active path.
	SinglePathDrives int `json:"single_path_drives" example:"0"`
	// EnclosuresWithProblems counts enclosures with at least one problem.
	EnclosuresWithProblems int `json:"enclosures_with_problems" example:"0"`
}

// NewPendingStorageTopology returns the placeholder served before the first
// collection completes (or while the storage_topology collector is disabled).
func NewPendingStorageTopology(now time.Time) *StorageTopology {
	return &StorageTopology{
		State:       StorageTopologyStatePending,
		Controllers: []StorageController{},
		Enclosures:  []StorageEnclosure{},
		Drives:      []StorageDrive{},
		Timestamp:   now,
	}
}
