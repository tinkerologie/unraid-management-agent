package lib

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// The QSFP28 fixtures are `ethtool -m <if> raw on` dumps of two 100G AOCs in a
// Mellanox ConnectX-6 Dx (mlx5_core), with the vendor serial number bytes
// (196-211) overwritten by "REDACTED". The matching ethtool text output is in
// the *.ethtool.txt files next to them.
func loadQSFPFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sff/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 640 {
		t.Fatalf("fixture %s is %d bytes, want 640", name, len(b))
	}
	return b
}

func fv(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDecodeSFF8636Fixture(t *testing.T) {
	data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
	m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)

	if m.Error != "" {
		t.Fatalf("unexpected error %q", m.Error)
	}
	checks := map[string][2]string{
		"identifier": {m.Identifier, "QSFP28"},
		"standard":   {m.Standard, "SFF-8636"},
		"connector":  {m.Connector, "No separable connector"},
		"compliance": {m.Compliance, "100G AOC or 25GAUI C2M AOC (BER 5e-5)"},
		"vendor":     {m.VendorName, "CISCO-PROLINE"},
		"pn":         {m.VendorPN, "QSFP-100G-AOC-3M"},
		"rev":        {m.VendorRev, "D1"},
		"oui":        {m.VendorOUI, ""},
		"rx type":    {m.RxPowerType, "average"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if !m.DDMSupported || fv(m.WavelengthNm) != 850 {
		t.Errorf("ddm=%v wavelength=%v", m.DDMSupported, fv(m.WavelengthNm))
	}
	// Same values ethtool printed for this dump.
	if fv(m.TemperatureCelsius) != 51.01 || fv(m.VoltageVolts) != 3.2688 {
		t.Errorf("temperature=%v voltage=%v", fv(m.TemperatureCelsius), fv(m.VoltageVolts))
	}
	if len(m.Lanes) != 4 {
		t.Fatalf("lanes = %d, want 4", len(m.Lanes))
	}
	l1, l4 := m.Lanes[0], m.Lanes[3]
	if l1.Lane != 1 || fv(l1.RxPowerMW) != 2.3192 || fv(l1.RxPowerDBm) != 3.65 {
		t.Errorf("lane 1 rx = %v mW / %v dBm", fv(l1.RxPowerMW), fv(l1.RxPowerDBm))
	}
	if fv(l4.RxPowerMW) != 2.0848 || fv(l4.RxPowerDBm) != 3.19 {
		t.Errorf("lane 4 rx = %v mW / %v dBm", fv(l4.RxPowerMW), fv(l4.RxPowerDBm))
	}
	if fv(l4.TxBiasMA) != 6.972 || fv(l4.TxPowerMW) != 1.3944 || fv(l4.TxPowerDBm) != 1.44 {
		t.Errorf("lane 4 bias=%v tx=%v mW / %v dBm", fv(l4.TxBiasMA), fv(l4.TxPowerMW), fv(l4.TxPowerDBm))
	}
	if len(m.Alarms) != 0 || len(m.Warnings) != 0 {
		t.Errorf("alarms=%v warnings=%v", m.Alarms, m.Warnings)
	}

	th := m.Thresholds
	if th == nil {
		t.Fatal("no thresholds")
	}
	want := map[string]dto.NICThreshold{
		"temperature": {HighAlarm: 90, HighWarning: 80, LowWarning: -10, LowAlarm: -20},
		"voltage":     {HighAlarm: 3.62, HighWarning: 3.6, LowWarning: 3, LowAlarm: 2.95},
		"bias":        {HighAlarm: 14, HighWarning: 12, LowWarning: 1.5, LowAlarm: 1},
		"tx power":    {HighAlarm: 8.13, HighWarning: 7.99, LowWarning: -6.99, LowAlarm: -10},
		"rx power":    {HighAlarm: 8.13, HighWarning: 7.99, LowWarning: -10, LowAlarm: -13.01},
	}
	got := map[string]*dto.NICThreshold{
		"temperature": th.TemperatureCelsius, "voltage": th.VoltageVolts, "bias": th.TxBiasMA,
		"tx power": th.TxPowerDBm, "rx power": th.RxPowerDBm,
	}
	for name, w := range want {
		if got[name] == nil || *got[name] != w {
			t.Errorf("%s thresholds = %+v, want %+v", name, got[name], w)
		}
	}

	// The serial number is never decoded.
	j, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(j), "REDACTED") || strings.Contains(string(j), "serial") {
		t.Errorf("serial number leaked into %s", j)
	}
}

func TestDecodeSFF8636SecondPort(t *testing.T) {
	m := DecodeModuleEEPROM(ModuleTypeSFF8636, loadQSFPFixture(t, "qsfp28_aoc_port1.bin"))
	if m.Error != "" || fv(m.TemperatureCelsius) != 52.18 || len(m.Lanes) != 4 {
		t.Errorf("error=%q temperature=%v lanes=%d", m.Error, fv(m.TemperatureCelsius), len(m.Lanes))
	}
}

func TestDecodeSFF8636Flags(t *testing.T) {
	data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
	data[6] = 0x90  // temperature high alarm, low warning
	data[7] = 0x60  // voltage low alarm, high warning
	data[9] = 0x48  // lane 1 RX low alarm, lane 2 RX high alarm
	data[10] = 0x01 // lane 4 RX low warning
	data[12] = 0x20 // lane 3 bias high warning
	data[14] = 0x04 // lane 4 TX power low alarm
	m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)

	wantAlarms := []string{"temperature_high", "voltage_low", "lane1_rx_power_low", "lane2_rx_power_high", "lane4_tx_power_low"}
	wantWarnings := []string{"temperature_low", "voltage_high", "lane3_tx_bias_high", "lane4_rx_power_low"}
	slices.Sort(wantAlarms)
	slices.Sort(wantWarnings)
	gotAlarms, gotWarnings := slices.Sorted(slices.Values(m.Alarms)), slices.Sorted(slices.Values(m.Warnings))
	if !slices.Equal(gotAlarms, wantAlarms) {
		t.Errorf("alarms = %v, want %v", gotAlarms, wantAlarms)
	}
	if !slices.Equal(gotWarnings, wantWarnings) {
		t.Errorf("warnings = %v, want %v", gotWarnings, wantWarnings)
	}
}

func TestDecodeSFF8636Variants(t *testing.T) {
	t.Run("SFF-8436, no TX power monitoring, flat memory", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		data[0] = 0x0D
		data[2] |= 0x04    // flat memory: no page 03h thresholds
		data[220] = 0x08   // no TX power measurement
		data[14] = 0x04    // TX power flags are ignored without TX power
		data[131] = 0x04   // 40GBASE-SR4
		data[165] = 0x00   // OUI 00:02:c9
		data[166] = 0x02   //
		data[167] = 0xc9   //
		data[148+13] = 0x7 // non-printable byte in the vendor name is dropped
		m := DecodeModuleEEPROM(ModuleTypeSFF8436, data)
		if m.Standard != "SFF-8436" || m.Identifier != "QSFP+" || m.Compliance != "40GBASE-SR4" || m.VendorOUI != "00:02:c9" {
			t.Errorf("standard=%q identifier=%q compliance=%q oui=%q", m.Standard, m.Identifier, m.Compliance, m.VendorOUI)
		}
		if m.VendorName != "CISCO-PROLINE" {
			t.Errorf("vendor = %q", m.VendorName)
		}
		if m.Thresholds != nil {
			t.Errorf("flat-memory module has thresholds %+v", m.Thresholds)
		}
		if m.Lanes[0].TxPowerMW != nil || len(m.Alarms) != 0 {
			t.Errorf("tx power=%v alarms=%v", m.Lanes[0].TxPowerMW, m.Alarms)
		}
	})

	t.Run("paged module without TX power has no TX power thresholds", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		data[220] = 0x00 // OMA, no TX power
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)
		if m.Thresholds == nil || m.Thresholds.TxPowerDBm != nil || m.Thresholds.RxPowerDBm == nil {
			t.Errorf("thresholds = %+v", m.Thresholds)
		}
		if m.RxPowerType != "oma" {
			t.Errorf("rx power type = %q", m.RxPowerType)
		}
	})

	t.Run("short dump without page 03h", func(t *testing.T) {
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, loadQSFPFixture(t, "qsfp28_aoc_port0.bin")[:256])
		if m.Error != "" || m.Thresholds != nil || len(m.Lanes) != 4 {
			t.Errorf("error=%q thresholds=%+v lanes=%d", m.Error, m.Thresholds, len(m.Lanes))
		}
	})

	t.Run("passive copper cable has no diagnostics", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		data[147] = 0xA0 // copper cable, unequalized
		data[220] = 0x00
		data[131] = 0x80
		data[192] = 0x0B // 100GBASE-CR4
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)
		if m.DDMSupported || m.TemperatureCelsius != nil || m.WavelengthNm != nil || len(m.Lanes) != 0 || m.Thresholds != nil {
			t.Errorf("unexpected diagnostics: %+v", m)
		}
		if m.Compliance != "100GBASE-CR4 or 25GBASE-CR CA-L" {
			t.Errorf("compliance = %q", m.Compliance)
		}
	})

	t.Run("active copper cable with temperature monitoring", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		data[147] = 0xB0
		data[220] = 0x20
		data[6] = 0x80
		data[192] = 0x7F // unknown extended code
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)
		if !m.DDMSupported || m.TemperatureCelsius == nil || len(m.Lanes) != 0 || m.Thresholds != nil {
			t.Errorf("unexpected diagnostics: %+v", m)
		}
		if !slices.Equal(m.Alarms, []string{"temperature_high"}) {
			t.Errorf("alarms = %v", m.Alarms)
		}
		if m.Compliance != "extended compliance 0x7f" {
			t.Errorf("compliance = %q", m.Compliance)
		}
	})

	t.Run("dark lanes read -40 dBm", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		binary.BigEndian.PutUint16(data[34:], 0)
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)
		if fv(m.Lanes[0].RxPowerMW) != 0 || fv(m.Lanes[0].RxPowerDBm) != -40 {
			t.Errorf("lane 1 rx = %v mW / %v dBm", fv(m.Lanes[0].RxPowerMW), fv(m.Lanes[0].RxPowerDBm))
		}
	})

	t.Run("unknown identifier, connector and no wavelength", func(t *testing.T) {
		data := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
		data[0] = 0x99
		data[130] = 0x55
		data[186], data[187] = 0, 0
		m := DecodeModuleEEPROM(ModuleTypeSFF8636, data)
		if m.Identifier != "0x99" || m.Connector != "0x55" || m.WavelengthNm != nil {
			t.Errorf("identifier=%q connector=%q wavelength=%v", m.Identifier, m.Connector, m.WavelengthNm)
		}
	})
}

// sfpEEPROM builds an SFF-8472 SFP+ dump (A0h + A2h) with internally calibrated
// diagnostics: 35.5 °C, 3.3 V, 6 mA bias, 0.5 mW TX, 0.3 mW RX.
func sfpEEPROM() []byte {
	b := make([]byte, 512)
	b[0] = 0x03 // SFP/SFP+/SFP28
	b[2] = 0x07 // LC
	b[3] = 0x10 // 10GBASE-SR
	copy(b[20:36], "FS              ")
	copy(b[37:40], []byte{0x00, 0x90, 0x65})
	copy(b[40:56], "SFP-10GSR-85    ")
	copy(b[56:60], "A   ")
	binary.BigEndian.PutUint16(b[60:], 850)
	copy(b[68:84], "SECRETSERIAL0001")
	b[92] = 0x68 // DDM, internally calibrated, average RX power
	b[93] = 0x80 // alarm/warning flags implemented

	a2 := b[256:]
	put := func(off int, v uint16) { binary.BigEndian.PutUint16(a2[off:], v) }
	// Thresholds: high alarm, low alarm, high warning, low warning.
	for i, v := range []int16{80 * 256, -10 * 256, 75 * 256, -5 * 256} {
		put(2*i, uint16(v))
	}
	for i, v := range []uint16{36000, 29700, 34650, 31350} { // 3.6/2.97/3.465/3.135 V
		put(8+2*i, v)
	}
	for i, v := range []uint16{6000, 1000, 5000, 1500} { // 12/2/10/3 mA
		put(16+2*i, v)
	}
	for i, v := range []uint16{10000, 1000, 7943, 1259} { // 1 mW, 0.1 mW, ...
		put(24+2*i, v)
	}
	for i, v := range []uint16{10000, 100, 7943, 158} {
		put(32+2*i, v)
	}
	put(96, uint16(int16(35.5*256)))
	put(98, 33000)
	put(100, 3000)
	put(102, 5000)
	put(104, 3000)
	a2[112] = 0x88 // temperature high alarm, bias high alarm
	a2[113] = 0x40 // RX power low alarm
	a2[116] = 0x20 // voltage high warning
	a2[117] = 0x80 // RX power high warning
	return b
}

func TestDecodeSFF8472(t *testing.T) {
	m := DecodeModuleEEPROM(ModuleTypeSFF8472, sfpEEPROM())
	if m.Error != "" || m.Standard != "SFF-8472" || m.Identifier != "SFP/SFP+/SFP28" || m.Connector != "LC" {
		t.Fatalf("unexpected module %+v", m)
	}
	if m.Compliance != "10GBASE-SR" || m.VendorName != "FS" || m.VendorPN != "SFP-10GSR-85" || m.VendorRev != "A" || m.VendorOUI != "00:90:65" {
		t.Errorf("identity: %+v", m)
	}
	if fv(m.WavelengthNm) != 850 || !m.DDMSupported || m.RxPowerType != "average" {
		t.Errorf("wavelength=%v ddm=%v rx type=%q", fv(m.WavelengthNm), m.DDMSupported, m.RxPowerType)
	}
	if fv(m.TemperatureCelsius) != 35.5 || fv(m.VoltageVolts) != 3.3 {
		t.Errorf("temperature=%v voltage=%v", fv(m.TemperatureCelsius), fv(m.VoltageVolts))
	}
	if len(m.Lanes) != 1 {
		t.Fatalf("lanes = %d", len(m.Lanes))
	}
	l := m.Lanes[0]
	if fv(l.TxBiasMA) != 6 || fv(l.TxPowerMW) != 0.5 || fv(l.TxPowerDBm) != -3.01 || fv(l.RxPowerMW) != 0.3 || fv(l.RxPowerDBm) != -5.23 {
		t.Errorf("lane = %+v", l)
	}
	if !slices.Equal(m.Alarms, []string{"temperature_high", "lane1_tx_bias_high", "lane1_rx_power_low"}) {
		t.Errorf("alarms = %v", m.Alarms)
	}
	if !slices.Equal(m.Warnings, []string{"voltage_high", "lane1_rx_power_high"}) {
		t.Errorf("warnings = %v", m.Warnings)
	}
	th := m.Thresholds
	if th == nil || *th.TemperatureCelsius != (dto.NICThreshold{HighAlarm: 80, LowAlarm: -10, HighWarning: 75, LowWarning: -5}) {
		t.Fatalf("temperature thresholds = %+v", th)
	}
	if th.VoltageVolts.HighAlarm != 3.6 || th.TxBiasMA.LowAlarm != 2 || th.TxPowerDBm.HighAlarm != 0 || th.TxPowerDBm.LowAlarm != -10 || th.RxPowerDBm.LowAlarm != -20 {
		t.Errorf("thresholds = v%+v b%+v tx%+v rx%+v", th.VoltageVolts, th.TxBiasMA, th.TxPowerDBm, th.RxPowerDBm)
	}
	j, _ := json.Marshal(m)
	if strings.Contains(string(j), "SECRETSERIAL") {
		t.Errorf("serial number leaked into %s", j)
	}
}

func TestDecodeSFF8472ExternalCalibration(t *testing.T) {
	b := sfpEEPROM()
	b[92] = 0x50 // DDM, externally calibrated, OMA
	b[93] = 0    // no alarm/warning flags
	a2 := b[256:]
	putF := func(off int, v float32) { binary.BigEndian.PutUint32(a2[off:], math.Float32bits(v)) }
	putU := func(off int, v uint16) { binary.BigEndian.PutUint16(a2[off:], v) }
	putS := func(off int, v int16) { binary.BigEndian.PutUint16(a2[off:], uint16(v)) }
	putF(56, 0)    // Rx_PWR(4)
	putF(60, 0)    // Rx_PWR(3)
	putF(64, 0)    // Rx_PWR(2)
	putF(68, 2)    // Rx_PWR(1): RX power doubles
	putF(72, -100) // Rx_PWR(0)
	putU(76, 384)  // bias slope 1.5
	putU(78, 0)
	putU(80, 256) // TX slope 1
	putS(82, -6000)
	putU(84, 256) // temperature slope 1, offset +1 °C
	putU(86, 256)
	putU(88, 256) // voltage slope 1, offset -0.01 V
	putS(90, -100)

	m := DecodeModuleEEPROM(ModuleTypeSFF8472, b)
	l := m.Lanes[0]
	if fv(m.TemperatureCelsius) != 36.5 || fv(m.VoltageVolts) != 3.29 || fv(l.TxBiasMA) != 9 {
		t.Errorf("temperature=%v voltage=%v bias=%v", fv(m.TemperatureCelsius), fv(m.VoltageVolts), fv(l.TxBiasMA))
	}
	// TX: 5000 - 6000 clamps at 0; RX: 2*3000 - 100 = 5900 (0.59 mW).
	if fv(l.TxPowerMW) != 0 || fv(l.TxPowerDBm) != -40 || fv(l.RxPowerMW) != 0.59 {
		t.Errorf("tx=%v mW / %v dBm rx=%v mW", fv(l.TxPowerMW), fv(l.TxPowerDBm), fv(l.RxPowerMW))
	}
	if m.RxPowerType != "oma" || len(m.Alarms) != 0 || len(m.Warnings) != 0 {
		t.Errorf("rx type=%q alarms=%v warnings=%v", m.RxPowerType, m.Alarms, m.Warnings)
	}
	if m.Thresholds.TemperatureCelsius.HighAlarm != 81 {
		t.Errorf("calibrated temperature high alarm = %v", m.Thresholds.TemperatureCelsius.HighAlarm)
	}

	// A negative RX polynomial result clamps at 0.
	putU(104, 0)
	if m := DecodeModuleEEPROM(ModuleTypeSFF8472, b); fv(m.Lanes[0].RxPowerMW) != 0 {
		t.Errorf("rx = %v", fv(m.Lanes[0].RxPowerMW))
	}
}

func TestDecodeSFF8472Variants(t *testing.T) {
	cases := []struct {
		name       string
		eepromType uint32
		mutate     func(b []byte) []byte
		check      func(t *testing.T, m *dto.NICModule)
	}{
		{"SFF-8079 has no diagnostics", ModuleTypeSFF8079, func(b []byte) []byte { return b[:256] },
			func(t *testing.T, m *dto.NICModule) {
				if m.Standard != "SFF-8079" || m.DDMSupported || m.Lanes != nil || m.Thresholds != nil {
					t.Errorf("%+v", m)
				}
			}},
		{"SFF-8472 type with only A0h", ModuleTypeSFF8472, func(b []byte) []byte { return b[:256] },
			func(t *testing.T, m *dto.NICModule) {
				if m.DDMSupported {
					t.Errorf("%+v", m)
				}
			}},
		{"passive copper cable", ModuleTypeSFF8472, func(b []byte) []byte {
			b[3], b[8], b[92] = 0, 0x04, 0
			return b
		}, func(t *testing.T, m *dto.NICModule) {
			if m.Compliance != "Passive copper cable" || m.WavelengthNm != nil || m.DDMSupported {
				t.Errorf("%+v", m)
			}
		}},
		{"active cable", ModuleTypeSFF8472, func(b []byte) []byte {
			b[3], b[8] = 0, 0x08
			return b
		}, func(t *testing.T, m *dto.NICModule) {
			if m.Compliance != "Active cable" {
				t.Errorf("compliance = %q", m.Compliance)
			}
		}},
		{"Ethernet compliance", ModuleTypeSFF8472, func(b []byte) []byte {
			b[3], b[6] = 0, 0x09
			return b
		}, func(t *testing.T, m *dto.NICModule) {
			if m.Compliance != "1000BASE-T, 1000BASE-SX" {
				t.Errorf("compliance = %q", m.Compliance)
			}
		}},
		{"extended compliance (SFP28)", ModuleTypeSFF8472, func(b []byte) []byte {
			b[36] = 0x02
			return b
		}, func(t *testing.T, m *dto.NICModule) {
			if m.Compliance != "100GBASE-SR4 or 25GBASE-SR" {
				t.Errorf("compliance = %q", m.Compliance)
			}
		}},
		{"no compliance, connector or wavelength", ModuleTypeSFF8472, func(b []byte) []byte {
			b[2], b[3], b[60], b[61] = 0, 0, 0, 0
			return b
		}, func(t *testing.T, m *dto.NICModule) {
			if m.Compliance != "" || m.Connector != "" || m.WavelengthNm != nil {
				t.Errorf("%+v", m)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, DecodeModuleEEPROM(tc.eepromType, tc.mutate(sfpEEPROM())))
		})
	}
}

func TestDecodeModuleEEPROMErrors(t *testing.T) {
	if m := DecodeModuleEEPROM(ModuleTypeSFF8636, nil); m.Error == "" || m.Identifier != "Unknown" {
		t.Errorf("empty: %+v", m)
	}
	if m := DecodeModuleEEPROM(ModuleTypeSFF8636, make([]byte, 100)); !strings.Contains(m.Error, "too short") {
		t.Errorf("short QSFP: %+v", m)
	}
	if m := DecodeModuleEEPROM(ModuleTypeSFF8472, make([]byte, 100)); !strings.Contains(m.Error, "too short") {
		t.Errorf("short SFP: %+v", m)
	}
	if m := DecodeModuleEEPROM(0x9, []byte{0x18}); m.Error != "" || m.Standard != "" || m.Identifier != "QSFP-DD" {
		t.Errorf("CMIS: %+v", m)
	}
	// Alarms and warnings serialise as empty arrays, not null.
	j, _ := json.Marshal(DecodeModuleEEPROM(0x9, []byte{0x18}))
	if !strings.Contains(string(j), `"alarms":[]`) || !strings.Contains(string(j), `"warnings":[]`) {
		t.Errorf("json = %s", j)
	}
}

func TestMwToDBm(t *testing.T) {
	for _, tc := range []struct{ mw, want float64 }{
		{-1, -40}, {0, -40}, {0.000001, -40}, {1, 0}, {2, 3.01}, {0.0001, -40},
	} {
		if got := mwToDBm(tc.mw); !near(got, tc.want) {
			t.Errorf("mwToDBm(%v) = %v, want %v", tc.mw, got, tc.want)
		}
	}
}

func TestDecodeDataNotReady(t *testing.T) {
	qsfp := loadQSFPFixture(t, "qsfp28_aoc_port0.bin")
	qsfp[2] |= 0x01 // Data_Not_Ready
	qsfp[6] = 0x80
	m := DecodeModuleEEPROM(ModuleTypeSFF8636, qsfp)
	if !m.DataNotReady || !m.DDMSupported || m.VendorPN != "QSFP-100G-AOC-3M" {
		t.Errorf("not-ready QSFP: %+v", m)
	}
	if m.TemperatureCelsius != nil || m.Lanes != nil || len(m.Alarms) != 0 || m.Thresholds != nil {
		t.Errorf("not-ready QSFP reports monitors: %+v", m)
	}

	sfp := sfpEEPROM()
	sfp[256+110] = 0x01 // Data_Ready_Bar
	m = DecodeModuleEEPROM(ModuleTypeSFF8472, sfp)
	if !m.DataNotReady || m.TemperatureCelsius != nil || m.Lanes != nil || len(m.Alarms) != 0 || m.Thresholds != nil {
		t.Errorf("not-ready SFP: %+v", m)
	}
}

func TestDecodeSFF8472InvalidCalibration(t *testing.T) {
	for name, bits := range map[string]uint32{
		"NaN":  0x7fc00000,
		"+Inf": 0x7f800000,
		"-Inf": 0xff800000,
	} {
		t.Run(name, func(t *testing.T) {
			b := sfpEEPROM()
			b[92] = 0x50 // externally calibrated
			binary.BigEndian.PutUint32(b[256+64:], bits)
			m := DecodeModuleEEPROM(ModuleTypeSFF8472, b)
			if m.Error == "" || m.TemperatureCelsius != nil || m.Lanes != nil {
				t.Errorf("module = %+v", m)
			}
			// The module (and so the whole card list) still serialises.
			if _, err := json.Marshal(m); err != nil {
				t.Errorf("marshal: %v", err)
			}
		})
	}
}
