package lib

import (
	"testing"
)

func TestParseNICOffloadsOutput(t *testing.T) {
	input := `Features for eth0:
rx-checksumming: on
tx-checksumming: on
scatter-gather: on
tcp-segmentation-offload: on
generic-segmentation-offload: on
generic-receive-offload: on
large-receive-offload: off [fixed]
rx-vlan-offload: on
tx-vlan-offload: on [fixed]
`

	settings := parseNICOffloadsOutput(input)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RxChecksumming", settings.RxChecksumming, "on"},
		{"TxChecksumming", settings.TxChecksumming, "on"},
		{"ScatterGather", settings.ScatterGather, "on"},
		{"TCPSegmentOffload", settings.TCPSegmentOffload, "on"},
		{"GenericSegmentOffload", settings.GenericSegmentOffload, "on"},
		{"GenericReceiveOffload", settings.GenericReceiveOffload, "on"},
		{"LargeReceiveOffload", settings.LargeReceiveOffload, "off"},
		{"RxVlanOffload", settings.RxVlanOffload, "on"},
		{"TxVlanOffload", settings.TxVlanOffload, "on"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestParseNICOffloadsOutput_AllOff(t *testing.T) {
	input := `Features for eth0:
rx-checksumming: off
tx-checksumming: off
scatter-gather: off
tcp-segmentation-offload: off
generic-segmentation-offload: off
generic-receive-offload: off
large-receive-offload: off
rx-vlan-offload: off
tx-vlan-offload: off
`
	settings := parseNICOffloadsOutput(input)
	if settings.RxChecksumming != "off" {
		t.Errorf("RxChecksumming = %q, want %q", settings.RxChecksumming, "off")
	}
	if settings.GenericReceiveOffload != "off" {
		t.Errorf("GenericReceiveOffload = %q, want %q", settings.GenericReceiveOffload, "off")
	}
}

func TestParseNICOffloadsOutput_Empty(t *testing.T) {
	settings := parseNICOffloadsOutput("")
	if settings.RxChecksumming != "" {
		t.Errorf("expected empty fields for empty input, got RxChecksumming=%q", settings.RxChecksumming)
	}
}

func TestParseNICRingBuffersOutput(t *testing.T) {
	input := `Ring parameters for eth0:
Pre-set maximums:
RX:		4096
RX Mini:	n/a
RX Jumbo:	n/a
TX:		4096
Current hardware settings:
RX:		256
RX Mini:	n/a
RX Jumbo:	n/a
TX:		256
`

	buffers := parseNICRingBuffersOutput(input)

	if buffers.RxMax != 4096 {
		t.Errorf("RxMax = %d, want 4096", buffers.RxMax)
	}
	if buffers.TxMax != 4096 {
		t.Errorf("TxMax = %d, want 4096", buffers.TxMax)
	}
	if buffers.RxCurrent != 256 {
		t.Errorf("RxCurrent = %d, want 256", buffers.RxCurrent)
	}
	if buffers.TxCurrent != 256 {
		t.Errorf("TxCurrent = %d, want 256", buffers.TxCurrent)
	}
}

func TestParseNICRingBuffersOutput_AsymmetricValues(t *testing.T) {
	input := `Ring parameters for eth0:
Pre-set maximums:
RX:		8192
TX:		2048
Current hardware settings:
RX:		512
TX:		128
`

	buffers := parseNICRingBuffersOutput(input)

	if buffers.RxMax != 8192 {
		t.Errorf("RxMax = %d, want 8192", buffers.RxMax)
	}
	if buffers.TxMax != 2048 {
		t.Errorf("TxMax = %d, want 2048", buffers.TxMax)
	}
	if buffers.RxCurrent != 512 {
		t.Errorf("RxCurrent = %d, want 512", buffers.RxCurrent)
	}
	if buffers.TxCurrent != 128 {
		t.Errorf("TxCurrent = %d, want 128", buffers.TxCurrent)
	}
}

func TestParseNICRingBuffersOutput_Empty(t *testing.T) {
	buffers := parseNICRingBuffersOutput("")
	if buffers.RxMax != 0 || buffers.TxMax != 0 || buffers.RxCurrent != 0 || buffers.TxCurrent != 0 {
		t.Errorf("expected all zeros for empty input, got %+v", buffers)
	}
}

func TestParseNICRingBuffersOutput_NA(t *testing.T) {
	// Some interfaces report n/a for ring buffers
	input := `Ring parameters for lo:
Pre-set maximums:
RX:		n/a
RX Mini:	n/a
RX Jumbo:	n/a
TX:		n/a
Current hardware settings:
RX:		n/a
RX Mini:	n/a
RX Jumbo:	n/a
TX:		n/a
`
	buffers := parseNICRingBuffersOutput(input)
	if buffers.RxMax != 0 || buffers.TxMax != 0 {
		t.Errorf("expected zeros for n/a values, got RxMax=%d, TxMax=%d", buffers.RxMax, buffers.TxMax)
	}
}

func TestParseNICRingBuffersOutput_NoHeaders(t *testing.T) {
	// Newer ethtool versions omit section headers — first RX/TX are max, second are current.
	input := `Ring parameters for eth0:
RX:                     4096
RX Mini:                n/a
RX Jumbo:               n/a
TX:                     4096
TX push buff len:       n/a
HDS thresh:             n/a
RX:                     256
RX Mini:                n/a
RX Jumbo:               n/a
TX:                     256
RX Buf Len:             n/a
CQE Size:               n/a
TX Push:                off
RX Push:                off
TX push buff len:       n/a
TCP data split:         n/a
HDS thresh:             n/a
`

	buffers := parseNICRingBuffersOutput(input)

	if buffers.RxMax != 4096 {
		t.Errorf("RxMax = %d, want 4096", buffers.RxMax)
	}
	if buffers.TxMax != 4096 {
		t.Errorf("TxMax = %d, want 4096", buffers.TxMax)
	}
	if buffers.RxCurrent != 256 {
		t.Errorf("RxCurrent = %d, want 256", buffers.RxCurrent)
	}
	if buffers.TxCurrent != 256 {
		t.Errorf("TxCurrent = %d, want 256", buffers.TxCurrent)
	}
}
