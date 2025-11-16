package dto

import "time"

// HardwareInfo contains comprehensive hardware information
type HardwareInfo struct {
	BIOS          *BIOSInfo          `json:"bios,omitempty"`
	Baseboard     *BaseboardInfo     `json:"baseboard,omitempty"`
	CPU           *CPUHardwareInfo   `json:"cpu,omitempty"`
	Cache         []CPUCacheInfo     `json:"cache,omitempty"`
	MemoryArray   *MemoryArrayInfo   `json:"memory_array,omitempty"`
	MemoryDevices []MemoryDeviceInfo `json:"memory_devices,omitempty"`
	Timestamp     time.Time          `json:"timestamp"`
}

// BIOSInfo contains BIOS information from dmidecode
type BIOSInfo struct {
	Vendor          string   `json:"vendor,omitempty"`
	Version         string   `json:"version,omitempty"`
	ReleaseDate     string   `json:"release_date,omitempty"`
	Address         string   `json:"address,omitempty"`
	RuntimeSize     string   `json:"runtime_size,omitempty"`
	ROMSize         string   `json:"rom_size,omitempty"`
	Characteristics []string `json:"characteristics,omitempty"`
	Revision        string   `json:"revision,omitempty"`
}

// BaseboardInfo contains motherboard information from dmidecode
type BaseboardInfo struct {
	Manufacturer      string   `json:"manufacturer,omitempty"`
	ProductName       string   `json:"product_name,omitempty"`
	Version           string   `json:"version,omitempty"`
	SerialNumber      string   `json:"serial_number,omitempty"`
	AssetTag          string   `json:"asset_tag,omitempty"`
	Features          []string `json:"features,omitempty"`
	LocationInChassis string   `json:"location_in_chassis,omitempty"`
	Type              string   `json:"type,omitempty"`
}

// CPUHardwareInfo contains detailed CPU hardware information from dmidecode
type CPUHardwareInfo struct {
	SocketDesignation string   `json:"socket_designation,omitempty"`
	Family            string   `json:"family,omitempty"`
	Manufacturer      string   `json:"manufacturer,omitempty"`
	Signature         string   `json:"signature,omitempty"`
	Flags             []string `json:"flags,omitempty"`
	Voltage           string   `json:"voltage,omitempty"`
	ExternalClock     int      `json:"external_clock_mhz,omitempty"`
	MaxSpeed          int      `json:"max_speed_mhz,omitempty"`
	CurrentSpeed      int      `json:"current_speed_mhz,omitempty"`
	Status            string   `json:"status,omitempty"`
	Upgrade           string   `json:"upgrade,omitempty"`
	SerialNumber      string   `json:"serial_number,omitempty"`
	AssetTag          string   `json:"asset_tag,omitempty"`
	PartNumber        string   `json:"part_number,omitempty"`
	CoreEnabled       int      `json:"core_enabled,omitempty"`
	ThreadCount       int      `json:"thread_count,omitempty"`
	Characteristics   []string `json:"characteristics,omitempty"`
}

// CPUCacheInfo contains CPU cache information from dmidecode
type CPUCacheInfo struct {
	Level               int      `json:"level"`
	SocketDesignation   string   `json:"socket_designation,omitempty"`
	Configuration       string   `json:"configuration,omitempty"`
	OperationalMode     string   `json:"operational_mode,omitempty"`
	Location            string   `json:"location,omitempty"`
	InstalledSize       string   `json:"installed_size,omitempty"`
	MaximumSize         string   `json:"maximum_size,omitempty"`
	SupportedSRAMTypes  []string `json:"supported_sram_types,omitempty"`
	InstalledSRAMType   string   `json:"installed_sram_type,omitempty"`
	ErrorCorrectionType string   `json:"error_correction_type,omitempty"`
	SystemType          string   `json:"system_type,omitempty"`
	Associativity       string   `json:"associativity,omitempty"`
}

// MemoryArrayInfo contains physical memory array information from dmidecode
type MemoryArrayInfo struct {
	Location            string `json:"location,omitempty"`
	Use                 string `json:"use,omitempty"`
	ErrorCorrectionType string `json:"error_correction_type,omitempty"`
	MaximumCapacity     string `json:"maximum_capacity,omitempty"`
	NumberOfDevices     int    `json:"number_of_devices,omitempty"`
}

// MemoryDeviceInfo contains individual memory device information from dmidecode
type MemoryDeviceInfo struct {
	Locator           string `json:"locator,omitempty"`
	BankLocator       string `json:"bank_locator,omitempty"`
	Size              string `json:"size,omitempty"`
	FormFactor        string `json:"form_factor,omitempty"`
	Type              string `json:"type,omitempty"`
	TypeDetail        string `json:"type_detail,omitempty"`
	Speed             string `json:"speed,omitempty"`
	Manufacturer      string `json:"manufacturer,omitempty"`
	SerialNumber      string `json:"serial_number,omitempty"`
	AssetTag          string `json:"asset_tag,omitempty"`
	PartNumber        string `json:"part_number,omitempty"`
	Rank              int    `json:"rank,omitempty"`
	ConfiguredSpeed   string `json:"configured_speed,omitempty"`
	MinimumVoltage    string `json:"minimum_voltage,omitempty"`
	MaximumVoltage    string `json:"maximum_voltage,omitempty"`
	ConfiguredVoltage string `json:"configured_voltage,omitempty"`
	DataWidth         int    `json:"data_width,omitempty"`
	TotalWidth        int    `json:"total_width,omitempty"`
}
