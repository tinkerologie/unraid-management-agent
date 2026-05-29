package dto

import "time"

// HardwareInfo contains comprehensive hardware information
type HardwareInfo struct {
	BIOS          *BIOSInfo          `json:"bios,omitempty"`
	Baseboard     *BaseboardInfo     `json:"baseboard,omitempty"`
	Chassis       *ChassisInfo       `json:"chassis,omitempty"`
	CPU           *CPUHardwareInfo   `json:"cpu,omitempty"`
	Cache         []CPUCacheInfo     `json:"cache,omitempty"`
	MemoryArray   *MemoryArrayInfo   `json:"memory_array,omitempty"`
	MemoryDevices []MemoryDeviceInfo `json:"memory_devices,omitempty"`
	TPM           *TPMInfo           `json:"tpm,omitempty"`
	Boot          *BootInfo          `json:"boot,omitempty"`
	Timestamp     time.Time          `json:"timestamp"`
}

// ChassisInfo contains chassis (enclosure) information from dmidecode type 3 / sysfs.
// New in Unraid 7.3, which surfaces the chassis serial number in System Info.
type ChassisInfo struct {
	Manufacturer string `json:"manufacturer,omitempty" example:"Supermicro"`
	Type         string `json:"type,omitempty" example:"Tower"`
	Version      string `json:"version,omitempty" example:"0123456789"`
	SerialNumber string `json:"serial_number,omitempty" example:"C1234567890"`
	AssetTag     string `json:"asset_tag,omitempty" example:"To be filled by O.E.M."`
}

// TPMInfo describes the Trusted Platform Module state. Unraid 7.3 bundles
// tpm2-tools and can tie licensing to a TPM 2.0 chip.
type TPMInfo struct {
	Present      bool   `json:"present" example:"true"`               // A TPM device exists (/sys/class/tpm/tpm0)
	Version      string `json:"version,omitempty" example:"2.0"`      // "2.0" or "1.2" when detectable
	Manufacturer string `json:"manufacturer,omitempty" example:"IFX"` // Vendor ID when available
}

// BootInfo describes the Unraid boot device. Unraid 7.3 added internal boot
// (NVMe/SSD/eMMC) and optional ZFS-mirrored boot pools; terminology changed
// from "Flash" to "Boot".
type BootInfo struct {
	DeviceType   string `json:"device_type" example:"usb"`                 // "usb" (flash) or "internal"
	Device       string `json:"device,omitempty" example:"sda1"`           // Backing device of the boot partition
	FileSystem   string `json:"filesystem,omitempty" example:"vfat"`       // Boot partition filesystem
	BootPool     string `json:"boot_pool,omitempty" example:"boot"`        // ZFS boot pool name when present
	BootPoolType string `json:"boot_pool_type,omitempty" example:"mirror"` // "single", "mirror" when on a ZFS boot pool
}

// BIOSInfo contains BIOS information from dmidecode
type BIOSInfo struct {
	Vendor          string   `json:"vendor,omitempty" example:"American Megatrends International, LLC."`
	Version         string   `json:"version,omitempty" example:"1.4"`
	ReleaseDate     string   `json:"release_date,omitempty" example:"12/25/2023"`
	Address         string   `json:"address,omitempty" example:"0xF0000"`
	RuntimeSize     string   `json:"runtime_size,omitempty" example:"64 KB"`
	ROMSize         string   `json:"rom_size,omitempty" example:"16 MB"`
	Characteristics []string `json:"characteristics,omitempty"`
	Revision        string   `json:"revision,omitempty" example:"5.14"`
}

// BaseboardInfo contains motherboard information from dmidecode
type BaseboardInfo struct {
	Manufacturer      string   `json:"manufacturer,omitempty" example:"Supermicro"`
	ProductName       string   `json:"product_name,omitempty" example:"X11SCL-F"`
	Version           string   `json:"version,omitempty" example:"1.02"`
	SerialNumber      string   `json:"serial_number,omitempty" example:"OM123456789"`
	AssetTag          string   `json:"asset_tag,omitempty" example:"To be filled by O.E.M."`
	Features          []string `json:"features,omitempty"`
	LocationInChassis string   `json:"location_in_chassis,omitempty" example:"Default string"`
	Type              string   `json:"type,omitempty" example:"Motherboard"`
}

// CPUHardwareInfo contains detailed CPU hardware information from dmidecode
type CPUHardwareInfo struct {
	SocketDesignation string   `json:"socket_designation,omitempty" example:"CPU 1"`
	Family            string   `json:"family,omitempty" example:"Core i7"`
	Manufacturer      string   `json:"manufacturer,omitempty" example:"Intel(R) Corporation"`
	Signature         string   `json:"signature,omitempty" example:"Type 0, Family 6, Model 158, Stepping 10"`
	Flags             []string `json:"flags,omitempty"`
	Voltage           string   `json:"voltage,omitempty" example:"1.0 V"`
	ExternalClock     int      `json:"external_clock_mhz,omitempty" example:"100"`
	MaxSpeed          int      `json:"max_speed_mhz,omitempty" example:"4900"`
	CurrentSpeed      int      `json:"current_speed_mhz,omitempty" example:"3600"`
	Status            string   `json:"status,omitempty" example:"Populated, Enabled"`
	Upgrade           string   `json:"upgrade,omitempty" example:"Socket LGA1151"`
	SerialNumber      string   `json:"serial_number,omitempty" example:"To Be Filled By O.E.M."`
	AssetTag          string   `json:"asset_tag,omitempty" example:"To Be Filled By O.E.M."`
	PartNumber        string   `json:"part_number,omitempty" example:"To Be Filled By O.E.M."`
	CoreEnabled       int      `json:"core_enabled,omitempty" example:"8"`
	ThreadCount       int      `json:"thread_count,omitempty" example:"8"`
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
