package collectors

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func TestParseCPUSnapshot(t *testing.T) {
	valid := "cpu 1 2 3 4 5 6 7 8\n"
	tests := []struct {
		name, input string
		wantErr     bool
		wantCores   int
	}{
		{"multiple cores", valid + "cpu0 1 2 3 4 5 6 7 8\ncpu12 8 7 6 5 4 3 2 1\n", false, 2},
		{"missing aggregate", "cpu0 1 2 3 4 5 6 7 8\n", true, 0},
		{"empty", "", true, 0},
		{"short aggregate", "cpu 1 2 3\n", true, 0},
		{"short core", valid + "cpu0 1\n", true, 0},
		{"invalid aggregate counter", "cpu bad 2 3 4 5 6 7 8\n", true, 0},
		{"invalid core counter", valid + "cpu0 1 2 3 4 -5 6 7 8\n", true, 0},
		{"overflowing counter", "cpu 18446744073709551616 2 3 4 5 6 7 8\n", true, 0},
		{"ignore non numeric core suffix", valid + "cpufoo invalid\ncpu0x invalid\ncpu-1 invalid\ncpu１ invalid\n", false, 0},
		{"ignore other and blank lines", "\n\nintr 100\n" + valid + "ctxt 20\n", false, 0},
		{"guest excluded", "cpu 1 2 3 4 5 6 7 8 900 1000\n", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := parseCPUSnapshot(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if snapshot.aggregate != (cpuCounters{1, 2, 3, 4, 5, 6, 7, 8}) || snapshot.aggregate.total() != 36 || snapshot.aggregate.idle() != 9 {
				t.Errorf("unexpected counters: %+v", snapshot.aggregate)
			}
			if len(snapshot.cores) != tt.wantCores {
				t.Errorf("cores = %v, want count %d", snapshot.cores, tt.wantCores)
			}
			if tt.wantCores == 2 && snapshot.cores["cpu12"] != (cpuCounters{8, 7, 6, 5, 4, 3, 2, 1}) {
				t.Errorf("cpu12 counters = %v", snapshot.cores["cpu12"])
			}
		})
	}
	t.Run("scanner error after aggregate", func(t *testing.T) {
		wantErr := errors.New("read failure")
		_, err := parseCPUSnapshot(io.MultiReader(strings.NewReader(valid), cpuErrorReader{wantErr}))
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

type cpuErrorReader struct{ err error }

func (r cpuErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestCPUUsageDelta(t *testing.T) {
	tests := []struct {
		name              string
		previous, current cpuCounters
		want              float64
		status            cpuDeltaStatus
	}{
		{"known delta", cpuCounters{100, 0, 0, 100}, cpuCounters{200, 0, 0, 200}, 50, cpuDeltaValid},
		{"idle lower limit", cpuCounters{100, 0, 0, 100}, cpuCounters{100, 0, 0, 200}, 0, cpuDeltaValid},
		{"busy upper limit", cpuCounters{100, 0, 0, 100}, cpuCounters{200, 0, 0, 100}, 100, cpuDeltaValid},
		{"zero delta", cpuCounters{100, 0, 0, 100}, cpuCounters{100, 0, 0, 100}, 0, cpuDeltaZero},
		{"equal total with changed fields", cpuCounters{100, 0, 0, 100}, cpuCounters{110, 0, 0, 90}, 0, cpuDeltaZero},
		{"iowait decrease", cpuCounters{100, 0, 0, 100, 20}, cpuCounters{150, 0, 0, 150, 10}, 50, cpuDeltaValid},
		{"busy field decrease", cpuCounters{100, 0, 0, 100}, cpuCounters{90, 0, 0, 120}, 0, cpuDeltaValid},
		{"total reset", cpuCounters{1000, 0, 0, 1000}, cpuCounters{30, 0, 0, 70}, 30, cpuDeltaReset},
		{"reset to zero", cpuCounters{100, 0, 0, 100}, cpuCounters{}, 0, cpuDeltaZero},
		{"first since boot", cpuCounters{}, cpuCounters{1, 2, 3, 10, 10, 4, 5, 5}, 50, cpuDeltaValid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, status := cpuUsageDelta(tt.previous, tt.current)
			if math.Abs(got-tt.want) > 1e-9 || status != tt.status || math.IsNaN(got) || got < 0 || got > 100 {
				t.Errorf("usage/status = %v/%v, want %v/%v", got, status, tt.want, tt.status)
			}
		})
	}
}

type cpuSampleStep struct {
	input string
	err   error
	usage float64
	cores map[string]float64
}

type cpuTrackedReader struct {
	io.Reader
	closed *int
}

func (r cpuTrackedReader) Close() error { *r.closed++; return nil }

func cpuFixture(user, idle int, cores string) string {
	return fmt.Sprintf("cpu %d 0 0 %d 0 0 0 0\n%s", user, idle, cores)
}

func TestSystemCollectorCPUSampling(t *testing.T) {
	a := cpuFixture(100, 100, "cpu0 25 0 0 75 0 0 0 0\ncpu1 75 0 0 25 0 0 0 0\n")
	c := cpuFixture(150, 250, "cpu0 50 0 0 150 0 0 0 0\ncpu1 100 0 0 100 0 0 0 0\n")
	first := cpuSampleStep{input: a, usage: 50, cores: map[string]float64{"cpu0": 25, "cpu1": 75}}
	last := cpuSampleStep{input: c, usage: 25, cores: map[string]float64{"cpu0": 25, "cpu1": 25}}
	tests := []struct {
		name  string
		steps []cpuSampleStep
	}{
		{"same snapshot and one read each cycle", []cpuSampleStep{first, last}},
		{"open failure spans gap", []cpuSampleStep{first, {err: errors.New("open failure")}, last}},
		{"malformed core commits nothing", []cpuSampleStep{first, {input: cpuFixture(500, 500, "cpu0 bad\n"), err: errors.New("parse")}, last}},
		{"missing aggregate commits nothing", []cpuSampleStep{first, {input: "cpu0 500 0 0 500 0 0 0 0\n", err: errors.New("parse")}, last}},
		{"aggregate zero retains both results and baseline", []cpuSampleStep{first,
			{input: cpuFixture(120, 80, "cpu2 50 0 0 50 0 0 0 0\n"), usage: 50, cores: first.cores}, last}},
		{"initial zero does not establish baseline", []cpuSampleStep{{input: cpuFixture(0, 0, "cpu0 0 0 0 0 0 0 0 0\n")}, first}},
		{"new removed and returned cores", []cpuSampleStep{first,
			{input: cpuFixture(150, 250, "cpu0 50 0 0 150 0 0 0 0\ncpu2 30 0 0 70 0 0 0 0\n"), usage: 25, cores: map[string]float64{"cpu0": 25, "cpu2": 30}},
			{input: cpuFixture(200, 300, "cpu1 80 0 0 120 0 0 0 0\n"), usage: 50, cores: map[string]float64{"cpu1": 40}}}},
		{"per core zero retains result and baseline", []cpuSampleStep{first,
			{input: cpuFixture(150, 250, "cpu0 50 0 0 50 0 0 0 0\n"), usage: 25, cores: map[string]float64{"cpu0": 25}},
			{input: cpuFixture(200, 300, "cpu0 50 0 0 150 0 0 0 0\n"), usage: 50, cores: map[string]float64{"cpu0": 25}}}},
		{"reset commits new baseline", []cpuSampleStep{first,
			{input: cpuFixture(30, 70, "cpu0 10 0 0 40 0 0 0 0\n"), usage: 30, cores: map[string]float64{"cpu0": 20}},
			{input: cpuFixture(80, 120, "cpu0 60 0 0 90 0 0 0 0\n"), usage: 50, cores: map[string]float64{"cpu0": 50}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewSystemCollector(nil)
			calls, closes := 0, 0
			collector.openCPUStat = func() (io.ReadCloser, error) {
				if calls >= len(tt.steps) {
					t.Fatal("extra CPU read")
				}
				step := tt.steps[calls]
				calls++
				if step.input == "" && step.err != nil {
					return nil, step.err
				}
				return cpuTrackedReader{strings.NewReader(step.input), &closes}, nil
			}
			wantCloses := 0
			for i, step := range tt.steps {
				baseline := collector.prevCPU
				usage, cores, err := collector.sampleCPU()
				if (err != nil) != (step.err != nil) {
					t.Fatalf("step %d: error = %v, want %v", i, err, step.err)
				}
				if step.input != "" || step.err == nil {
					wantCloses++
				}
				if calls != i+1 || closes != wantCloses {
					t.Fatalf("step %d: reads/closes = %d/%d, want %d/%d", i, calls, closes, i+1, wantCloses)
				}
				if err != nil {
					if !reflect.DeepEqual(collector.prevCPU, baseline) {
						t.Fatal("failed snapshot changed baseline")
					}
					continue
				}
				if math.Abs(usage-step.usage) > 1e-9 || !reflect.DeepEqual(cores, step.cores) {
					t.Fatalf("step %d: usage/cores = %v/%v, want %v/%v", i, usage, cores, step.usage, step.cores)
				}
				// Mutating a published result must not corrupt retained results.
				for name := range cores {
					cores[name] = -999
				}
			}
		})
	}
}

func TestSystemCollectorCPUInstancesAndConcurrency(t *testing.T) {
	collectors := []*SystemCollector{NewSystemCollector(nil), NewSystemCollector(nil)}
	for i, c := range collectors {
		input := cpuFixture(25+50*i, 75-50*i, "")
		c.openCPUStat = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(input)), nil }
	}
	var wg sync.WaitGroup
	for i, c := range collectors {
		for range 4 {
			wg.Go(func() {
				usage, _, err := c.sampleCPU()
				if err != nil || usage != float64(25+50*i) {
					t.Errorf("instance %d: usage/error = %v/%v", i, usage, err)
				}
			})
		}
	}
	wg.Wait()
}

// Exercise the DTO wiring and partial-success path without starting collectors
// or invoking installed hardware utilities.
func TestSystemCollectorCPUInfoPartialSuccess(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("CPU read fails=%v", fail), func(t *testing.T) {
			collector := NewSystemCollector(&domain.Context{Version: "test-version"})
			collector.sysfsRoot = t.TempDir()
			calls := 0
			collector.openCPUStat = func() (io.ReadCloser, error) {
				calls++
				if fail {
					return nil, errors.New("CPU unavailable")
				}
				return io.NopCloser(strings.NewReader(cpuFixture(25, 75, "cpu0 40 0 0 60 0 0 0 0\n"))), nil
			}
			info, err := collector.collectSystemInfo()
			if err != nil || info == nil {
				t.Fatalf("collectSystemInfo = %v, %v", info, err)
			}
			if calls != 1 || info.AgentVersion != "test-version" || info.Hostname == "" || info.Timestamp.IsZero() {
				t.Fatalf("partial system info missing or reads=%d: %+v", calls, info)
			}
			if fail {
				if info.CPUUsage != 0 || info.CPUPerCore != nil {
					t.Fatalf("failed CPU sample populated metrics: %+v", info)
				}
			} else if info.CPUUsage != 25 || !reflect.DeepEqual(info.CPUPerCore, map[string]float64{"cpu0": 40}) {
				t.Fatalf("CPU DTO fields = %v/%v", info.CPUUsage, info.CPUPerCore)
			}
		})
	}
}

func TestParseCPUSnapshotStopsBeforeLongIntrLine(t *testing.T) {
	input := "cpu  1 2 3 4 5 6 7 8\ncpu0 1 2 3 4 5 6 7 8\nintr " + strings.Repeat("0 ", 100000) + "\nctxt 1\n"
	snapshot, err := parseCPUSnapshot(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseCPUSnapshot: %v", err)
	}
	if _, ok := snapshot.cores["cpu0"]; !ok {
		t.Fatal("cpu0 missing")
	}
}
