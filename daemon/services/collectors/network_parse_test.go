package collectors

import (
	"strings"
	"testing"
)

func TestParseNetDevStats(t *testing.T) {
	// Two header lines followed by interface rows, matching /proc/net/dev.
	const sample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  100     10    1    0    0     0          0         0      200     20    2    0    0     0       0          0
  eth0: 1000    50    5    0    0     0          0         0     2000    60    6    0    0     0       0          0
`

	stats, err := parseNetDevStats(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 interfaces, got %d: %v", len(stats), stats)
	}

	lo := stats["lo"]
	if lo.BytesReceived != 100 || lo.PacketsReceived != 10 || lo.ErrorsReceived != 1 {
		t.Errorf("lo receive fields wrong: %+v", lo)
	}
	if lo.BytesSent != 200 || lo.PacketsSent != 20 || lo.ErrorsSent != 2 {
		t.Errorf("lo transmit fields wrong: %+v", lo)
	}

	eth0 := stats["eth0"]
	if eth0.BytesReceived != 1000 || eth0.BytesSent != 2000 {
		t.Errorf("eth0 byte counters wrong: %+v", eth0)
	}
}

func TestParseNetDevStatsMalformed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected number of parsed interfaces
	}{
		{"empty input", "", 0},
		{"headers only", "h1\nh2\n", 0},
		{
			name:  "row without colon separator",
			input: "h1\nh2\n   bad row with no separator here\n",
			want:  0,
		},
		{
			name:  "row with too few fields",
			input: "h1\nh2\n  eth0: 1 2 3\n",
			want:  0,
		},
		{
			name: "valid row among malformed rows",
			input: "h1\nh2\n" +
				"  short: 1 2 3\n" +
				"  eth0: 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16\n",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := parseNetDevStats(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(stats) != tt.want {
				t.Errorf("got %d interfaces, want %d: %v", len(stats), tt.want, stats)
			}
		})
	}
}

func TestParseNetDevStatsNonNumericFields(t *testing.T) {
	// Non-numeric counters parse to zero (ParseUint error is swallowed), so the
	// interface is still recorded with zeroed fields.
	const sample = "h1\nh2\n" +
		"  eth0: abc def ghi 0 0 0 0 0 xyz 0 0 0 0 0 0 0\n"

	stats, err := parseNetDevStats(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eth0, ok := stats["eth0"]
	if !ok {
		t.Fatal("expected eth0 to be present")
	}
	if eth0.BytesReceived != 0 || eth0.BytesSent != 0 {
		t.Errorf("expected zeroed counters for non-numeric input, got %+v", eth0)
	}
}
