package lib

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// Module EEPROM layouts, as reported by the kernel's ETHTOOL_GMODULEINFO
// (ETH_MODULE_SFF_* in linux/ethtool.h).
const (
	// ModuleTypeSFF8079 is an SFP without diagnostics: page A0h only (256 bytes).
	ModuleTypeSFF8079 uint32 = 0x1
	// ModuleTypeSFF8472 is an SFP with diagnostics: A0h followed by A2h (512 bytes).
	ModuleTypeSFF8472 uint32 = 0x2
	// ModuleTypeSFF8636 is a QSFP28/QSFP+: lower page and upper page 00h (256
	// bytes), followed by upper pages 01h-03h when the module is paged (640 bytes).
	ModuleTypeSFF8636 uint32 = 0x3
	// ModuleTypeSFF8436 is a QSFP+ with the SFF-8636 layout.
	ModuleTypeSFF8436 uint32 = 0x4
)

const (
	sffPageLen = 256
	// sff8636Page03 is the offset of upper page 03h (thresholds) in a 640-byte dump.
	sff8636Page03 = 512
	sff8636MaxLen = 640
	// minDBm is the floor reported for optical power: 0.1 µW, the resolution of
	// the measurement, which a dead lane or dark fibre reads as zero.
	minDBm = -40.0
)

// sffIdentifiers are the SFF-8024 identifier values (table 4-1) seen on NICs.
var sffIdentifiers = map[byte]string{
	0x00: "Unknown",
	0x01: "GBIC",
	0x02: "Soldered module",
	0x03: "SFP/SFP+/SFP28",
	0x0C: "QSFP",
	0x0D: "QSFP+",
	0x11: "QSFP28",
	0x18: "QSFP-DD",
	0x19: "OSFP",
	0x1A: "SFP-DD",
	0x1B: "DSFP",
	0x1E: "QSFP+ (CMIS)",
}

// sffConnectors are the SFF-8024 connector types (table 4-3).
var sffConnectors = map[byte]string{
	0x01: "SC",
	0x02: "Fibre Channel Style 1 copper",
	0x03: "Fibre Channel Style 2 copper",
	0x04: "BNC/TNC",
	0x05: "Fibre Channel coax",
	0x06: "Fiber Jack",
	0x07: "LC",
	0x08: "MT-RJ",
	0x09: "MU",
	0x0A: "SG",
	0x0B: "Optical pigtail",
	0x0C: "MPO 1x12",
	0x0D: "MPO 2x16",
	0x20: "HSSDC II",
	0x21: "Copper pigtail",
	0x22: "RJ45",
	0x23: "No separable connector",
	0x24: "MXC 2x16",
	0x25: "CS optical connector",
	0x26: "SN optical connector",
	0x27: "MPO 2x12",
	0x28: "MPO 1x16",
}

// sffExtendedCompliance are the SFF-8024 extended specification compliance
// codes (table 4-4), used by QSFP28 and SFP28 modules.
var sffExtendedCompliance = map[byte]string{
	0x01: "100G AOC or 25GAUI C2M AOC (BER 5e-5)",
	0x02: "100GBASE-SR4 or 25GBASE-SR",
	0x03: "100GBASE-LR4 or 25GBASE-LR",
	0x04: "100GBASE-ER4 or 25GBASE-ER",
	0x05: "100GBASE-SR10",
	0x06: "100G CWDM4",
	0x07: "100G PSM4",
	0x08: "100G ACC or 25GAUI C2M ACC (BER 5e-5)",
	0x0B: "100GBASE-CR4 or 25GBASE-CR CA-L",
	0x0C: "25GBASE-CR CA-S",
	0x0D: "25GBASE-CR CA-N",
	0x10: "40GBASE-ER4",
	0x11: "4x10GBASE-SR",
	0x12: "40G PSM4",
	0x16: "10GBASE-T with SFI electrical interface",
	0x17: "100G CLR4",
	0x18: "100G AOC or 25GAUI C2M AOC (BER 1e-12)",
	0x19: "100G ACC or 25GAUI C2M ACC (BER 1e-12)",
	0x1A: "100GE-DWDM2",
	0x1B: "100G 1550nm WDM",
	0x1C: "10GBASE-T Short Reach",
	0x1D: "5GBASE-T",
	0x1E: "2.5GBASE-T",
	0x1F: "40G SWDM4",
	0x20: "100G SWDM4",
	0x21: "100G PAM4 BiDi",
	0x25: "100GBASE-DR",
	0x26: "100G-FR or 100GBASE-FR1",
	0x27: "100G-LR or 100GBASE-LR1",
}

// sffBit names one bit of a compliance byte.
type sffBit struct {
	mask byte
	name string
}

// sff8636Compliance is SFF-8636 byte 131 (10/40G Ethernet compliance), bit 7
// meaning "see the extended compliance code in byte 192".
var sff8636Compliance = []sffBit{
	{0x40, "10GBASE-LRM"}, {0x20, "10GBASE-LR"}, {0x10, "10GBASE-SR"},
	{0x08, "40GBASE-CR4"}, {0x04, "40GBASE-SR4"}, {0x02, "40GBASE-LR4"},
	{0x01, "40G Active Cable (XLPPI)"},
}

// sff8472TenGCompliance is SFF-8472 A0h byte 3, bits 7-4.
var sff8472TenGCompliance = []sffBit{
	{0x80, "10GBASE-ER"}, {0x40, "10GBASE-LRM"}, {0x20, "10GBASE-LR"}, {0x10, "10GBASE-SR"},
}

// sff8472EthernetCompliance is SFF-8472 A0h byte 6.
var sff8472EthernetCompliance = []sffBit{
	{0x80, "BASE-PX"}, {0x40, "BASE-BX10"}, {0x20, "100BASE-FX"}, {0x10, "100BASE-LX/LX10"},
	{0x08, "1000BASE-T"}, {0x04, "1000BASE-CX"}, {0x02, "1000BASE-LX"}, {0x01, "1000BASE-SX"},
}

// DecodeModuleEEPROM decodes the EEPROM of a pluggable module as returned by
// ETHTOOL_GMODULEEEPROM. It supports SFF-8472/SFF-8079 (SFP) and
// SFF-8636/SFF-8436 (QSFP); other layouts report the identifier only. The
// vendor serial number is never decoded.
func DecodeModuleEEPROM(eepromType uint32, data []byte) *dto.NICModule {
	m := &dto.NICModule{Alarms: []string{}, Warnings: []string{}}
	if len(data) == 0 {
		m.Identifier = sffIdentifier(0)
		m.Error = "empty module EEPROM"
		return m
	}
	m.Identifier = sffIdentifier(data[0])
	switch eepromType {
	case ModuleTypeSFF8079, ModuleTypeSFF8472:
		decodeSFF8472(eepromType, data, m)
	case ModuleTypeSFF8636, ModuleTypeSFF8436:
		decodeSFF8636(eepromType, data, m)
	default:
		// Other layouts (CMIS QSFP-DD/OSFP, ...) report the identifier only.
	}
	return m
}

// ---------- SFF-8636 / SFF-8436 (QSFP) ----------

func decodeSFF8636(eepromType uint32, data []byte, m *dto.NICModule) {
	if len(data) < sffPageLen {
		m.Error = fmt.Sprintf("module EEPROM too short (%d bytes)", len(data))
		return
	}
	m.Standard = "SFF-8636"
	if eepromType == ModuleTypeSFF8436 {
		m.Standard = "SFF-8436"
	}
	m.Connector = sffConnector(data[130])
	m.Compliance = sff8636ComplianceName(data[131], data[192])
	m.VendorName = sffString(data[148:164])
	m.VendorOUI = sffOUI(data[165:168])
	m.VendorPN = sffString(data[168:184])
	m.VendorRev = sffString(data[184:186])

	// Transmitter technology (byte 147, bits 7-4) 0xA and above are copper
	// cables, whose bytes 186-187 hold cable attenuation instead of a wavelength.
	optical := data[147]&0xF0 < 0xA0
	if wl := binary.BigEndian.Uint16(data[186:188]); optical && wl != 0 {
		m.WavelengthNm = ptrFloat(float64(wl) / 20)
	}

	// Byte 220 advertises temperature (bit 5), voltage (bit 4) and TX power
	// (bit 2) monitoring, but many optics leave bits 5/4 clear while reporting
	// both, so every optical module is treated as having diagnostics.
	diagType := data[220]
	m.DDMSupported = optical || diagType&0x34 != 0
	if !m.DDMSupported {
		return
	}
	// Status byte 2 bit 0 (Data_Not_Ready): monitors are invalid while the
	// module powers up or resets.
	if data[2]&0x01 != 0 {
		m.DataNotReady = true
		return
	}
	m.TemperatureCelsius = ptrFloat(round(float64(be16s(data[22:24]))/256, 2))
	m.VoltageVolts = ptrFloat(round(float64(binary.BigEndian.Uint16(data[26:28]))/10000, 4))
	decodeFlags(data[6], "temperature", m)
	decodeFlags(data[7], "voltage", m)
	if !optical {
		return
	}

	m.RxPowerType = rxPowerType(diagType&0x08 != 0)
	txPower := diagType&0x04 != 0
	for i := range 4 {
		lane := dto.NICModuleLane{
			Lane:     i + 1,
			TxBiasMA: ptrFloat(round(float64(binary.BigEndian.Uint16(data[42+2*i:]))*0.002, 3)),
		}
		lane.RxPowerMW, lane.RxPowerDBm = opticalPower(float64(binary.BigEndian.Uint16(data[34+2*i:])))
		if txPower {
			lane.TxPowerMW, lane.TxPowerDBm = opticalPower(float64(binary.BigEndian.Uint16(data[50+2*i:])))
		}
		m.Lanes = append(m.Lanes, lane)
	}
	// Per-lane flags: two lanes per byte (lane 1/3 in the high nibble).
	laneFlags := func(b byte, i int) byte {
		if i%2 == 1 {
			return b << 4
		}
		return b
	}
	for i := range 4 {
		prefix := fmt.Sprintf("lane%d_", i+1)
		decodeFlags(laneFlags(data[9+i/2], i), prefix+"rx_power", m)
		decodeFlags(laneFlags(data[11+i/2], i), prefix+"tx_bias", m)
		if txPower {
			decodeFlags(laneFlags(data[13+i/2], i), prefix+"tx_power", m)
		}
	}

	// Thresholds live in upper page 03h, present only on paged modules
	// (status byte 2 bit 2 "flat memory" clear) in a full 640-byte dump.
	if len(data) >= sff8636MaxLen && data[2]&0x04 == 0 {
		p := data[sff8636Page03:]
		th := &dto.NICModuleThresholds{
			TemperatureCelsius: thresholds(p[0:8], tempValue),
			VoltageVolts:       thresholds(p[16:24], voltageValue),
			RxPowerDBm:         thresholds(p[48:56], powerDBmValue),
			TxBiasMA:           thresholds(p[56:64], biasValue),
		}
		if txPower {
			th.TxPowerDBm = thresholds(p[64:72], powerDBmValue)
		}
		m.Thresholds = th
	}
}

func sff8636ComplianceName(code, extended byte) string {
	if code&0x80 != 0 {
		return extendedComplianceName(extended)
	}
	return bitNames(code, sff8636Compliance)
}

// ---------- SFF-8472 / SFF-8079 (SFP) ----------

func decodeSFF8472(eepromType uint32, data []byte, m *dto.NICModule) {
	if len(data) < sffPageLen {
		m.Error = fmt.Sprintf("module EEPROM too short (%d bytes)", len(data))
		return
	}
	m.Standard = "SFF-8472"
	if eepromType == ModuleTypeSFF8079 {
		m.Standard = "SFF-8079"
	}
	m.Connector = sffConnector(data[2])
	m.Compliance = sff8472ComplianceName(data)
	m.VendorName = sffString(data[20:36])
	m.VendorOUI = sffOUI(data[37:40])
	m.VendorPN = sffString(data[40:56])
	m.VendorRev = sffString(data[56:60])
	// Byte 8 bits 3/2 mark active/passive cables, whose bytes 60-61 hold
	// cable compliance instead of a wavelength.
	if wl := binary.BigEndian.Uint16(data[60:62]); data[8]&0x0C == 0 && wl != 0 {
		m.WavelengthNm = ptrFloat(float64(wl))
	}

	diagType := data[92]
	m.DDMSupported = eepromType == ModuleTypeSFF8472 && len(data) >= 2*sffPageLen && diagType&0x40 != 0
	if !m.DDMSupported {
		return
	}
	a2 := data[sffPageLen : 2*sffPageLen]
	// A2h byte 110 bit 0 (Data_Ready_Bar): monitors are invalid until it clears.
	if a2[110]&0x01 != 0 {
		m.DataNotReady = true
		return
	}
	cal, ok := newSFF8472Calibration(a2, diagType&0x10 != 0)
	if !ok {
		m.Error = "invalid external calibration constants"
		return
	}

	m.TemperatureCelsius = ptrFloat(round(cal.temp(a2[96:98]), 2))
	m.VoltageVolts = ptrFloat(round(cal.voltage(a2[98:100]), 4))
	m.RxPowerType = rxPowerType(diagType&0x08 != 0)
	lane := dto.NICModuleLane{Lane: 1, TxBiasMA: ptrFloat(round(cal.bias(a2[100:102]), 3))}
	lane.TxPowerMW, lane.TxPowerDBm = opticalPower(cal.txPower(a2[102:104]))
	lane.RxPowerMW, lane.RxPowerDBm = opticalPower(cal.rxPower(a2[104:106]))
	m.Lanes = []dto.NICModuleLane{lane}

	// Alarm (bytes 112-113) and warning (bytes 116-117) flags, when implemented
	// (A0h byte 93 bit 7). Each flag byte holds high/low pairs.
	if data[93]&0x80 != 0 {
		decodeSFF8472Flags(a2[112], a2[113], &m.Alarms)
		decodeSFF8472Flags(a2[116], a2[117], &m.Warnings)
	}

	m.Thresholds = &dto.NICModuleThresholds{
		TemperatureCelsius: calibratedThresholds(a2[0:8], cal.temp),
		VoltageVolts:       calibratedThresholds(a2[8:16], cal.voltage),
		TxBiasMA:           calibratedThresholds(a2[16:24], cal.bias),
		TxPowerDBm:         calibratedThresholds(a2[24:32], func(b []byte) float64 { return mwToDBm(cal.txPower(b) / 10000) }),
		RxPowerDBm:         calibratedThresholds(a2[32:40], func(b []byte) float64 { return mwToDBm(cal.rxPower(b) / 10000) }),
	}
}

func sff8472ComplianceName(data []byte) string {
	if name := extendedComplianceName(data[36]); name != "" {
		return name
	}
	if name := bitNames(data[3], sff8472TenGCompliance); name != "" {
		return name
	}
	if name := bitNames(data[6], sff8472EthernetCompliance); name != "" {
		return name
	}
	switch {
	case data[8]&0x04 != 0:
		return "Passive copper cable"
	case data[8]&0x08 != 0:
		return "Active cable"
	}
	return ""
}

// decodeSFF8472Flags appends the names of the set flags in an SFF-8472 alarm
// or warning byte pair: temperature, voltage, bias and TX power high/low in
// the first byte, RX power high/low in bits 7/6 of the second.
func decodeSFF8472Flags(first, second byte, out *[]string) {
	names := []struct {
		b    byte
		mask byte
		name string
	}{
		{first, 0x80, "temperature_high"}, {first, 0x40, "temperature_low"},
		{first, 0x20, "voltage_high"}, {first, 0x10, "voltage_low"},
		{first, 0x08, "lane1_tx_bias_high"}, {first, 0x04, "lane1_tx_bias_low"},
		{first, 0x02, "lane1_tx_power_high"}, {first, 0x01, "lane1_tx_power_low"},
		{second, 0x80, "lane1_rx_power_high"}, {second, 0x40, "lane1_rx_power_low"},
	}
	for _, n := range names {
		if n.b&n.mask != 0 {
			*out = append(*out, n.name)
		}
	}
}

// sff8472Calibration applies SFF-8472 external calibration (A2h bytes 56-91)
// to raw A/D values. With internal calibration it is the identity.
type sff8472Calibration struct {
	external bool
	rx       [5]float64 // Rx_PWR(0) .. Rx_PWR(4)
	biasSlope, biasOffset,
	txSlope, txOffset,
	tempSlope, tempOffset,
	voltSlope, voltOffset float64
}

// newSFF8472Calibration reads the calibration constants. It reports false
// when an RX power coefficient is NaN or infinite (a corrupt EEPROM), which
// would otherwise poison every derived value. Finite float32 coefficients
// applied to 16-bit readings cannot overflow a float64.
func newSFF8472Calibration(a2 []byte, external bool) (sff8472Calibration, bool) {
	c := sff8472Calibration{external: external}
	if !external {
		return c, true
	}
	for i := range 5 {
		// Rx_PWR(4) is at 56, Rx_PWR(0) at 72, as big-endian IEEE 754 floats.
		v := float64(math.Float32frombits(binary.BigEndian.Uint32(a2[56+4*i:])))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return c, false
		}
		c.rx[4-i] = v
	}
	slope := func(off int) float64 { return float64(binary.BigEndian.Uint16(a2[off:])) / 256 }
	offset := func(off int) float64 { return float64(be16s(a2[off:])) }
	c.biasSlope, c.biasOffset = slope(76), offset(78)
	c.txSlope, c.txOffset = slope(80), offset(82)
	c.tempSlope, c.tempOffset = slope(84), offset(86)
	c.voltSlope, c.voltOffset = slope(88), offset(90)
	return c, true
}

// linear applies slope/offset calibration to an unsigned raw value.
func (c sff8472Calibration) linear(b []byte, slope, offset float64) float64 {
	raw := float64(binary.BigEndian.Uint16(b))
	if !c.external {
		return raw
	}
	return math.Max(0, raw*slope+offset)
}

// temp returns °C.
func (c sff8472Calibration) temp(b []byte) float64 {
	raw := float64(be16s(b))
	if c.external {
		raw = raw*c.tempSlope + c.tempOffset
	}
	return raw / 256
}

// voltage returns volts.
func (c sff8472Calibration) voltage(b []byte) float64 {
	return c.linear(b, c.voltSlope, c.voltOffset) / 10000
}

// bias returns mA.
func (c sff8472Calibration) bias(b []byte) float64 {
	return c.linear(b, c.biasSlope, c.biasOffset) * 0.002
}

// txPower returns units of 0.1 µW.
func (c sff8472Calibration) txPower(b []byte) float64 {
	return c.linear(b, c.txSlope, c.txOffset)
}

// rxPower returns units of 0.1 µW, using the fourth-order calibration
// polynomial when externally calibrated.
func (c sff8472Calibration) rxPower(b []byte) float64 {
	raw := float64(binary.BigEndian.Uint16(b))
	if !c.external {
		return raw
	}
	v := 0.0
	for i := 4; i >= 0; i-- {
		v = v*raw + c.rx[i]
	}
	return math.Max(0, v)
}

// ---------- Shared helpers ----------

// decodeFlags appends the names of the set flags in the high nibble of b
// (bit 7 high alarm, 6 low alarm, 5 high warning, 4 low warning).
func decodeFlags(b byte, name string, m *dto.NICModule) {
	if b&0x80 != 0 {
		m.Alarms = append(m.Alarms, name+"_high")
	}
	if b&0x40 != 0 {
		m.Alarms = append(m.Alarms, name+"_low")
	}
	if b&0x20 != 0 {
		m.Warnings = append(m.Warnings, name+"_high")
	}
	if b&0x10 != 0 {
		m.Warnings = append(m.Warnings, name+"_low")
	}
}

// thresholds decodes high alarm, low alarm, high warning, low warning (in
// that EEPROM order) from 8 bytes.
func thresholds(b []byte, value func([]byte) float64) *dto.NICThreshold {
	return &dto.NICThreshold{
		HighAlarm:   value(b[0:2]),
		LowAlarm:    value(b[2:4]),
		HighWarning: value(b[4:6]),
		LowWarning:  value(b[6:8]),
	}
}

func calibratedThresholds(b []byte, value func([]byte) float64) *dto.NICThreshold {
	return thresholds(b, func(v []byte) float64 { return round(value(v), 4) })
}

func tempValue(b []byte) float64 {
	return round(float64(be16s(b))/256, 2)
}

func voltageValue(b []byte) float64 {
	return round(float64(binary.BigEndian.Uint16(b))/10000, 4)
}

func biasValue(b []byte) float64 {
	return round(float64(binary.BigEndian.Uint16(b))*0.002, 3)
}

func powerDBmValue(b []byte) float64 {
	return mwToDBm(float64(binary.BigEndian.Uint16(b)) / 10000)
}

// opticalPower converts a power reading in units of 0.1 µW to mW and dBm.
func opticalPower(tenthMicroWatts float64) (*float64, *float64) {
	mw := tenthMicroWatts / 10000
	return ptrFloat(round(mw, 4)), ptrFloat(mwToDBm(mw))
}

// mwToDBm converts mW to dBm, floored at minDBm (no light reads as zero).
func mwToDBm(mw float64) float64 {
	if mw <= 0 {
		return minDBm
	}
	return math.Max(minDBm, round(10*math.Log10(mw), 2))
}

func rxPowerType(average bool) string {
	if average {
		return "average"
	}
	return "oma"
}

func sffIdentifier(id byte) string {
	if name, ok := sffIdentifiers[id]; ok {
		return name
	}
	return fmt.Sprintf("0x%02x", id)
}

func sffConnector(code byte) string {
	if code == 0 {
		return ""
	}
	if name, ok := sffConnectors[code]; ok {
		return name
	}
	return fmt.Sprintf("0x%02x", code)
}

func extendedComplianceName(code byte) string {
	if code == 0 {
		return ""
	}
	if name, ok := sffExtendedCompliance[code]; ok {
		return name
	}
	return fmt.Sprintf("extended compliance 0x%02x", code)
}

// bitNames joins the names of the set bits.
func bitNames(b byte, bits []sffBit) string {
	var names []string
	for _, bit := range bits {
		if b&bit.mask != 0 {
			names = append(names, bit.name)
		}
	}
	return strings.Join(names, ", ")
}

// sffString decodes a space-padded ASCII field, dropping non-printable bytes.
func sffString(b []byte) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, string(b))
	return strings.TrimSpace(s)
}

// sffOUI formats an IEEE company ID, or "" when unset.
func sffOUI(b []byte) string {
	if b[0] == 0 && b[1] == 0 && b[2] == 0 {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x", b[0], b[1], b[2])
}

// be16s reads a big-endian two's complement 16-bit value.
func be16s(b []byte) int16 {
	return int16(binary.BigEndian.Uint16(b)) //nolint:gosec // G115: reinterpreting the bits is the intent
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func ptrFloat(v float64) *float64 {
	return &v
}
