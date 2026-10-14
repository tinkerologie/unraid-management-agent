package collectors

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// upscSmartUPSX3000 is `upsc ups` from an APC Smart-UPS X 3000 on usbhid-ups
// (serial numbers replaced). It reports battery data but no ups.load,
// ups.realpower, ups.realpower.nominal, ups.power or input/output voltage.
const upscSmartUPSX3000 = `battery.charge: 100
battery.charge.low: 10
battery.charge.warning: 50
battery.runtime: 623
battery.runtime.low: 150
battery.type: PbAc
battery.voltage: 134.7
battery.voltage.nominal: 120.0
device.mfr: American Power Conversion
device.model: Smart-UPS X 3000
device.serial: AS0000000000
device.type: ups
driver.name: usbhid-ups
driver.state: quiet
driver.version: 2.8.5
ups.beeper.status: enabled
ups.delay.shutdown: 20
ups.mfr: American Power Conversion
ups.model: Smart-UPS X 3000
ups.productid: 0003
ups.status: OL
ups.timer.reboot: -1
ups.timer.shutdown: -1
ups.vendorid: 051d
`

// upscAP7752ATS is `upsc ats` from an APC AP7752 automatic transfer switch on
// snmp-ups (serial replaced): no battery, load or power readings at all.
const upscAP7752ATS = `device.mfr: APC
device.model: AP7752
device.serial: 5A0000T00000
device.type: ats
driver.name: snmp-ups
input.1.voltage: 118
input.2.voltage: 123
input.voltage.nominal: 120
output.frequency: 60
ups.status: OL
`

// fakeExec returns an execOutput function that answers each "command args"
// line from outputs, and fails for anything else.
func fakeExec(outputs map[string]string) func(string, ...string) (string, error) {
	return func(command string, args ...string) (string, error) {
		key := strings.Join(append([]string{command}, args...), " ")
		if out, ok := outputs[key]; ok {
			return out, nil
		}
		return "", errors.New("unexpected command: " + key)
	}
}

func TestParseOptionalFloat(t *testing.T) {
	tests := []struct {
		in   string
		want *float64
	}{
		{"25.5", new(25.5)},
		{" 0 ", new(0.0)},
		{"-1", new(-1.0)},
		{"", nil},
		{"n/a", nil},
		{"NaN", nil},
		{"Inf", nil},
		{"-Inf", nil},
	}
	for _, tt := range tests {
		got := parseOptionalFloat(tt.in)
		if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
			t.Errorf("parseOptionalFloat(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseOptionalInt(t *testing.T) {
	tests := []struct {
		in   string
		want *int
	}{
		{"623", new(623)},
		{"623.0", new(623)},
		{"-1", new(-1)},
		{"", nil},
		{"unknown", nil},
		{"1e100", nil},
		{"-1e100", nil},
	}
	for _, tt := range tests {
		got := parseOptionalInt(tt.in)
		if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
			t.Errorf("parseOptionalInt(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestDerivePower(t *testing.T) {
	tests := []struct {
		name          string
		nominal, load *float64
		want          *float64
	}{
		{"both known", new(800.0), new(13.0), new(104.0)},
		{"zero load is a real 0 W", new(800.0), new(0.0), new(0.0)},
		{"no load reading", new(800.0), nil, nil},
		{"no nominal rating", nil, new(13.0), nil},
		{"nominal rating of 0", new(0.0), new(13.0), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derivePower(tt.nominal, tt.load)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("derivePower() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUPSCollectorNUTMissingReadingsAreNull(t *testing.T) {
	c := &UPSCollector{execOutput: fakeExec(map[string]string{
		"upsc -l localhost":      "ups\nups-gpu\nats\n",
		"upsc ups@localhost":     upscSmartUPSX3000,
		"upsc ups-gpu@localhost": "battery.charge: 1\n",
	})}

	status, err := c.collectNUT()
	if err != nil {
		t.Fatalf("collectNUT() error = %v", err)
	}
	if status.BatteryCharge == nil || *status.BatteryCharge != 100 {
		t.Errorf("BatteryCharge = %v, want 100", status.BatteryCharge)
	}
	if status.RuntimeLeft == nil || *status.RuntimeLeft != 623 {
		t.Errorf("RuntimeLeft = %v, want 623", status.RuntimeLeft)
	}
	if status.LoadPercent != nil || status.PowerWatts != nil || status.NominalPower != nil {
		t.Errorf("unreported readings = load %v, power %v, nominal %v; want all nil",
			status.LoadPercent, status.PowerWatts, status.NominalPower)
	}
	if status.Status != "OL" || status.Model != "Smart-UPS X 3000" {
		t.Errorf("status/model = %q/%q", status.Status, status.Model)
	}
	if status.BatteryVoltage == nil || *status.BatteryVoltage != 134.7 || status.SerialNumber != "AS0000000000" {
		t.Errorf("battery voltage/serial = %v/%q, want 134.7/AS0000000000", status.BatteryVoltage, status.SerialNumber)
	}
	if status.InputVoltage != nil || status.OutputVoltage != nil || status.LineFrequency != nil || status.Temperature != nil ||
		status.TimeOnBattery != nil || status.CumulativeTimeOnBattery != nil || status.TransferCount != nil {
		t.Errorf("unreported telemetry must be nil: %+v", status)
	}

	body, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, field := range []string{`"load_percent":null`, `"power_watts":null`, `"nominal_power_watts":null`,
		`"input_voltage":null`, `"output_voltage":null`, `"line_frequency":null`, `"temperature_celsius":null`,
		`"time_on_battery_seconds":null`, `"cumulative_time_on_battery_seconds":null`, `"transfer_count":null`} {
		if !strings.Contains(string(body), field) {
			t.Errorf("JSON %s does not contain %s", body, field)
		}
	}
}

func TestUPSCollectorNUTPower(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   *float64
	}{
		{"ups.realpower is used as reported", "ups.load: 20\nups.realpower: 150\nups.realpower.nominal: 800\n", new(150.0)},
		{"derived from nominal and load", "ups.load: 13\nups.realpower.nominal: 800\n", new(104.0)},
		{"zero load with nominal is 0 W", "ups.load: 0\nups.realpower.nominal: 800\n", new(0.0)},
		{"load without nominal is unknown", "ups.load: 13\n", nil},
		{"VA rating is not a watt rating", "ups.load: 50\nups.power.nominal: 1500\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &UPSCollector{execOutput: fakeExec(map[string]string{
				"upsc -l localhost":  "ups\n",
				"upsc ups@localhost": "ups.status: OL\n" + tt.output,
			})}
			status, err := c.collectNUT()
			if err != nil {
				t.Fatalf("collectNUT() error = %v", err)
			}
			if (status.PowerWatts == nil) != (tt.want == nil) ||
				(status.PowerWatts != nil && *status.PowerWatts != *tt.want) {
				t.Errorf("PowerWatts = %v, want %v", status.PowerWatts, tt.want)
			}
		})
	}
}

func TestUPSCollectorNUTListFallbackAndErrors(t *testing.T) {
	// upsc -l localhost fails, plain upsc -l works.
	c := &UPSCollector{execOutput: fakeExec(map[string]string{
		"upsc -l":            "ups\n",
		"upsc ups@localhost": "ups.status: OB\nbattery.charge: 80\n",
	})}
	status, err := c.collectNUT()
	if err != nil {
		t.Fatalf("collectNUT() error = %v", err)
	}
	if status.Status != "OB" || status.BatteryCharge == nil || *status.BatteryCharge != 80 {
		t.Errorf("status = %q, battery = %v", status.Status, status.BatteryCharge)
	}

	for name, outputs := range map[string]map[string]string{
		"no device list":     {},
		"empty device list":  {"upsc -l localhost": "\n"},
		"device query fails": {"upsc -l localhost": "ups\n"},
	} {
		c := &UPSCollector{execOutput: fakeExec(outputs)}
		if _, err := c.collectNUT(); err == nil {
			t.Errorf("%s: collectNUT() error = nil, want error", name)
		}
	}
}

func TestUPSCollectorAPC(t *testing.T) {
	full := `STATUS   : ONLINE
LOADPCT  : 25.0 Percent
BCHARGE  : 100.0 Percent
TIMELEFT : 45.0 Minutes
NOMPOWER : 900 Watts
MODEL    : Back-UPS RS 1500G
`
	c := &UPSCollector{execOutput: fakeExec(map[string]string{"apcaccess": full})}
	status, err := c.collectAPC()
	if err != nil {
		t.Fatalf("collectAPC() error = %v", err)
	}
	if status.LoadPercent == nil || *status.LoadPercent != 25 ||
		status.BatteryCharge == nil || *status.BatteryCharge != 100 ||
		status.RuntimeLeft == nil || *status.RuntimeLeft != 2700 ||
		status.NominalPower == nil || *status.NominalPower != 900 ||
		status.PowerWatts == nil || *status.PowerWatts != 225 {
		t.Errorf("collectAPC() = load %v, battery %v, runtime %v, nominal %v, power %v",
			status.LoadPercent, status.BatteryCharge, status.RuntimeLeft, status.NominalPower, status.PowerWatts)
	}

	// A UPS that reports neither load nor nominal power.
	sparse := "STATUS   : ONLINE\nBCHARGE  : 97.0 Percent\nTIMELEFT : n/a\n"
	// An absurd TIMELEFT that overflows int once converted to seconds is unknown.
	c = &UPSCollector{execOutput: fakeExec(map[string]string{"apcaccess": "TIMELEFT : 1e300 Minutes\n"})}
	if status, err = c.collectAPC(); err != nil || status.RuntimeLeft != nil {
		t.Errorf("collectAPC() overflowing TIMELEFT = %v, %v; want nil runtime", status.RuntimeLeft, err)
	}
	c = &UPSCollector{execOutput: fakeExec(map[string]string{"apcaccess": sparse})}
	status, err = c.collectAPC()
	if err != nil {
		t.Fatalf("collectAPC() error = %v", err)
	}
	if status.LoadPercent != nil || status.NominalPower != nil || status.PowerWatts != nil || status.RuntimeLeft != nil {
		t.Errorf("unreported readings = load %v, nominal %v, power %v, runtime %v; want all nil",
			status.LoadPercent, status.NominalPower, status.PowerWatts, status.RuntimeLeft)
	}
	if status.BatteryCharge == nil || *status.BatteryCharge != 97 {
		t.Errorf("BatteryCharge = %v, want 97", status.BatteryCharge)
	}

	c = &UPSCollector{execOutput: fakeExec(nil)}
	if _, err := c.collectAPC(); err == nil {
		t.Error("collectAPC() error = nil when apcaccess fails")
	}
}

func TestUPSCollectorRunDefaultsToExec(t *testing.T) {
	out, err := (&UPSCollector{}).run("echo", "ok")
	if err != nil || strings.TrimSpace(out) != "ok" {
		t.Errorf("run(echo ok) = %q, %v", out, err)
	}
}

func TestNUTCollectStatusMissingReadingsAreNull(t *testing.T) {
	c := &NUTCollector{execOutput: fakeExec(map[string]string{
		"upsc ups@localhost": upscSmartUPSX3000,
		"upsc ats@localhost": upscAP7752ATS,
	})}

	ups, err := c.collectStatus("ups", "localhost")
	if err != nil {
		t.Fatalf("collectStatus(ups) error = %v", err)
	}
	if ups.LoadPercent != nil || ups.RealPower != nil || ups.RealPowerNominal != nil ||
		ups.ApparentPower != nil || ups.ApparentPowerNominal != nil ||
		ups.InputVoltage != nil || ups.OutputVoltage != nil {
		t.Errorf("unreported readings should be nil: load %v realpower %v nominal %v apparent %v/%v in %v out %v",
			ups.LoadPercent, ups.RealPower, ups.RealPowerNominal, ups.ApparentPower,
			ups.ApparentPowerNominal, ups.InputVoltage, ups.OutputVoltage)
	}
	if ups.BatteryCharge == nil || *ups.BatteryCharge != 100 ||
		ups.BatteryRuntime == nil || *ups.BatteryRuntime != 623 ||
		ups.BatteryVoltage == nil || *ups.BatteryVoltage != 134.7 ||
		ups.DelayShutdown == nil || *ups.DelayShutdown != 20 ||
		ups.TimerShutdown == nil || *ups.TimerShutdown != -1 {
		t.Errorf("reported readings: battery %v runtime %v voltage %v delay %v timer %v",
			ups.BatteryCharge, ups.BatteryRuntime, ups.BatteryVoltage, ups.DelayShutdown, ups.TimerShutdown)
	}
	if ups.Model != "Smart-UPS X 3000" || ups.Type != "ups" || ups.StatusText != "Online" {
		t.Errorf("identity = %q/%q/%q", ups.Model, ups.Type, ups.StatusText)
	}

	body, err := json.Marshal(ups)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, field := range []string{`"load_percent":null`, `"realpower_watts":null`, `"apparent_power_va":null`, `"input_voltage":null`} {
		if !strings.Contains(string(body), field) {
			t.Errorf("JSON does not contain %s", field)
		}
	}

	ats, err := c.collectStatus("ats", "localhost")
	if err != nil {
		t.Fatalf("collectStatus(ats) error = %v", err)
	}
	if ats.BatteryCharge != nil || ats.BatteryRuntime != nil || ats.LoadPercent != nil || ats.InputVoltage != nil {
		t.Errorf("ATS readings should be nil: battery %v runtime %v load %v input %v",
			ats.BatteryCharge, ats.BatteryRuntime, ats.LoadPercent, ats.InputVoltage)
	}
	if ats.InputVoltageNominal == nil || *ats.InputVoltageNominal != 120 ||
		ats.OutputFrequency == nil || *ats.OutputFrequency != 60 || ats.Type != "ats" {
		t.Errorf("ATS reported readings: nominal %v frequency %v type %q",
			ats.InputVoltageNominal, ats.OutputFrequency, ats.Type)
	}
}

func TestNUTCollectStatusAllReadings(t *testing.T) {
	vars := map[string]float64{
		"battery.charge": 95, "battery.charge.low": 10, "battery.charge.warning": 50,
		"battery.voltage": 27.1, "battery.voltage.nominal": 24,
		"input.voltage": 230.5, "input.voltage.nominal": 230, "input.frequency": 50,
		"input.transfer.high": 280, "input.transfer.low": 160, "input.current": 1.2,
		"output.voltage": 229.8, "output.frequency": 50.1, "output.current": 1.1,
		"ups.load": 30, "ups.realpower": 270, "ups.realpower.nominal": 900,
		"ups.power": 300, "ups.power.nominal": 1500,
		"battery.runtime": 1800, "battery.runtime.low": 120,
		"ups.delay.shutdown": 20, "ups.delay.start": 30,
		"ups.timer.shutdown": -1, "ups.timer.start": 0,
	}
	var b strings.Builder
	for k, v := range vars {
		b.WriteString(k + ": " + strconv.FormatFloat(v, 'f', -1, 64) + "\n")
	}
	c := &NUTCollector{execOutput: fakeExec(map[string]string{"upsc ups@localhost": b.String()})}
	s, err := c.collectStatus("ups", "localhost")
	if err != nil {
		t.Fatalf("collectStatus() error = %v", err)
	}

	floats := map[string]*float64{
		"battery.charge": s.BatteryCharge, "battery.charge.low": s.BatteryChargeLow,
		"battery.charge.warning": s.BatteryChargeWarning, "battery.voltage": s.BatteryVoltage,
		"battery.voltage.nominal": s.BatteryVoltageNominal, "input.voltage": s.InputVoltage,
		"input.voltage.nominal": s.InputVoltageNominal, "input.frequency": s.InputFrequency,
		"input.transfer.high": s.InputTransferHigh, "input.transfer.low": s.InputTransferLow,
		"input.current": s.InputCurrent, "output.voltage": s.OutputVoltage,
		"output.frequency": s.OutputFrequency, "output.current": s.OutputCurrent,
		"ups.load": s.LoadPercent, "ups.realpower": s.RealPower,
		"ups.realpower.nominal": s.RealPowerNominal, "ups.power": s.ApparentPower,
		"ups.power.nominal": s.ApparentPowerNominal,
	}
	for k, got := range floats {
		if got == nil || *got != vars[k] {
			t.Errorf("%s = %v, want %v", k, got, vars[k])
		}
	}
	ints := map[string]*int{
		"battery.runtime": s.BatteryRuntime, "battery.runtime.low": s.BatteryRuntimeLow,
		"ups.delay.shutdown": s.DelayShutdown, "ups.delay.start": s.DelayStart,
		"ups.timer.shutdown": s.TimerShutdown, "ups.timer.start": s.TimerStart,
	}
	for k, got := range ints {
		if got == nil || float64(*got) != vars[k] {
			t.Errorf("%s = %v, want %v", k, got, vars[k])
		}
	}
}

func TestNUTCollectStatusDerivedPower(t *testing.T) {
	c := &NUTCollector{execOutput: fakeExec(map[string]string{
		"upsc derived@localhost": "ups.load: 10\nups.realpower.nominal: 900\nups.power.nominal: 1500\n",
		"upsc garbled@localhost": "ups.load: n/a\nups.realpower.nominal: 900\nups.power.nominal: 1500\n",
	})}

	s, err := c.collectStatus("derived", "localhost")
	if err != nil {
		t.Fatalf("collectStatus() error = %v", err)
	}
	if s.RealPower == nil || *s.RealPower != 90 || s.ApparentPower == nil || *s.ApparentPower != 150 {
		t.Errorf("derived power = %v W / %v VA, want 90 / 150", s.RealPower, s.ApparentPower)
	}

	s, err = c.collectStatus("garbled", "localhost")
	if err != nil {
		t.Fatalf("collectStatus() error = %v", err)
	}
	if s.LoadPercent != nil || s.RealPower != nil || s.ApparentPower != nil {
		t.Errorf("unparseable load: load %v, power %v W / %v VA; want all nil", s.LoadPercent, s.RealPower, s.ApparentPower)
	}
	if s.RawVariables["ups.load"] != "n/a" {
		t.Errorf("raw ups.load = %q, want n/a", s.RawVariables["ups.load"])
	}

	if _, err := c.collectStatus("missing", "localhost"); err == nil {
		t.Error("collectStatus() error = nil when upsc fails")
	}
}

func TestNUTCollectStatusDefaultExec(t *testing.T) {
	// With no injected exec the real upsc is run. Test hosts have no NUT
	// server named this, so the query fails cleanly.
	if _, err := (&NUTCollector{}).collectStatus("uma-test-no-such-ups", "127.0.0.1"); err == nil {
		t.Skip("upsc unexpectedly answered for a non-existent UPS")
	}
}

// apcaccessTelemetry is apcaccess output with the extended telemetry (#193),
// modelled on a CyberPower PR1000ELCDRT1U plus BATTV/ITEMP from an APC unit.
const apcaccessTelemetry = `STATUS   : ONLINE
MODEL    : PR1000ELCDRT1U
LINEV    : 250.0 Volts
LOADPCT  : 13.0 Percent
OUTPUTV  : 249.5 Volts
BATTV    : 24.3 Volts
LINEFREQ : 49.9 Hz
ITEMP    : 29.2 C
SELFTEST : NO
NUMXFERS : 3
TONBATT  : 12 Seconds
CUMONBATT: 95 Seconds
SERIALNO : PZSQ0000000000
`

// TestParseAPCOutputTelemetry maps every extended apcupsd key (#193).
func TestParseAPCOutputTelemetry(t *testing.T) {
	status, err := parseAPCOutput(apcaccessTelemetry)
	if err != nil {
		t.Fatal(err)
	}
	floats := map[string]struct {
		got  *float64
		want float64
	}{
		"InputVoltage": {status.InputVoltage, 250}, "OutputVoltage": {status.OutputVoltage, 249.5},
		"BatteryVoltage": {status.BatteryVoltage, 24.3}, "LineFrequency": {status.LineFrequency, 49.9},
		"Temperature": {status.Temperature, 29.2},
	}
	for name, f := range floats {
		if f.got == nil || *f.got != f.want {
			t.Errorf("%s = %v, want %v", name, f.got, f.want)
		}
	}
	ints := map[string]struct {
		got  *int
		want int
	}{
		"TimeOnBattery": {status.TimeOnBattery, 12}, "CumulativeTimeOnBattery": {status.CumulativeTimeOnBattery, 95},
		"TransferCount": {status.TransferCount, 3},
	}
	for name, f := range ints {
		if f.got == nil || *f.got != f.want {
			t.Errorf("%s = %v, want %d", name, f.got, f.want)
		}
	}
	if status.SelfTest != "NO" || status.SerialNumber != "PZSQ0000000000" {
		t.Errorf("self test/serial = %q/%q", status.SelfTest, status.SerialNumber)
	}
}

// TestParseAPCOutputTelemetryInvalid keeps malformed or non-Celsius readings
// null and leaves empty strings out of the JSON.
func TestParseAPCOutputTelemetryInvalid(t *testing.T) {
	status, err := parseAPCOutput("STATUS   : ONLINE\nLINEV    : N/A\nOUTPUTV  : -- Volts\nLINEFREQ : bad Hz\nITEMP    : 84.6 F\nTONBATT  : x Seconds\nNUMXFERS : many\n")
	if err != nil {
		t.Fatal(err)
	}
	if status.InputVoltage != nil || status.OutputVoltage != nil || status.LineFrequency != nil ||
		status.Temperature != nil || status.TimeOnBattery != nil || status.TransferCount != nil || status.BatteryVoltage != nil {
		t.Errorf("malformed readings must be nil: %+v", status)
	}
	body, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"self_test"`) || strings.Contains(string(body), `"serial_number"`) {
		t.Errorf("empty self_test/serial_number must be omitted: %s", body)
	}
}

// TestParseNUTUpscOutputTelemetry maps every extended NUT variable (#193).
func TestParseNUTUpscOutputTelemetry(t *testing.T) {
	status, err := parseNUTUpscOutput(`ups.status: OL
input.voltage: 231.4
output.voltage: 230.0
battery.voltage: 27.1
input.frequency: 50.0
ups.temperature: 31.5
battery.runtime.elapsed: 0
input.transfers: 7
ups.test.result: Done and passed
device.serial: AS1234567890
input.1.voltage: 999
`)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		got  *float64
		want float64
	}{
		{"InputVoltage", status.InputVoltage, 231.4}, {"OutputVoltage", status.OutputVoltage, 230},
		{"BatteryVoltage", status.BatteryVoltage, 27.1}, {"LineFrequency", status.LineFrequency, 50},
		{"Temperature", status.Temperature, 31.5},
	}
	for _, c := range checks {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if status.TimeOnBattery == nil || *status.TimeOnBattery != 0 || status.TransferCount == nil || *status.TransferCount != 7 {
		t.Errorf("time on battery/transfers = %v/%v, want 0/7", status.TimeOnBattery, status.TransferCount)
	}
	if status.CumulativeTimeOnBattery != nil {
		t.Errorf("CumulativeTimeOnBattery = %v, want nil for NUT", status.CumulativeTimeOnBattery)
	}
	if status.SelfTest != "Done and passed" || status.SerialNumber != "AS1234567890" {
		t.Errorf("self test/serial = %q/%q", status.SelfTest, status.SerialNumber)
	}

	status, err = parseNUTUpscOutput("ups.serial: unknown\n")
	if err != nil {
		t.Fatal(err)
	}
	if status.SerialNumber != "" {
		t.Errorf("placeholder serial must be omitted, got %q", status.SerialNumber)
	}

	status, err = parseNUTUpscOutput("ups.serial: 3B1234X56789\n")
	if err != nil {
		t.Fatal(err)
	}
	if status.SerialNumber != "3B1234X56789" {
		t.Errorf("ups.serial-only SerialNumber = %q", status.SerialNumber)
	}
}
