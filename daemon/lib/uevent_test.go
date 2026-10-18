package lib

import (
	"reflect"
	"testing"
)

func TestParseUevent(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		want   Uevent
		wantOK bool
	}{
		{
			name: "block disk add",
			msg:  "add@/devices/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0/block/sdb\x00ACTION=add\x00DEVPATH=/devices/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0/block/sdb\x00SUBSYSTEM=block\x00MAJOR=8\x00MINOR=16\x00DEVNAME=sdb\x00DEVTYPE=disk\x00SEQNUM=4321\x00",
			want: Uevent{
				Action:    "add",
				DevPath:   "/devices/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0/block/sdb",
				Subsystem: "block",
				DevType:   "disk",
				Env: map[string]string{
					"ACTION": "add", "DEVPATH": "/devices/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0/block/sdb",
					"SUBSYSTEM": "block", "MAJOR": "8", "MINOR": "16", "DEVNAME": "sdb", "DEVTYPE": "disk", "SEQNUM": "4321",
				},
			},
			wantOK: true,
		},
		{
			name: "usb remove without trailing NUL",
			msg:  "remove@/devices/pci0000:00/0000:00:14.0/usb1/1-2\x00ACTION=remove\x00DEVPATH=/devices/pci0000:00/0000:00:14.0/usb1/1-2\x00SUBSYSTEM=usb\x00DEVTYPE=usb_device\x00PRODUCT=781/5581/100",
			want: Uevent{
				Action:    "remove",
				DevPath:   "/devices/pci0000:00/0000:00:14.0/usb1/1-2",
				Subsystem: "usb",
				DevType:   "usb_device",
				Env: map[string]string{
					"ACTION": "remove", "DEVPATH": "/devices/pci0000:00/0000:00:14.0/usb1/1-2",
					"SUBSYSTEM": "usb", "DEVTYPE": "usb_device", "PRODUCT": "781/5581/100",
				},
			},
			wantOK: true,
		},
		{
			name: "header only",
			msg:  "change@/devices/virtual/net/br0\x00",
			want: Uevent{
				Action:  "change",
				DevPath: "/devices/virtual/net/br0",
				Env:     map[string]string{},
			},
			wantOK: true,
		},
		{
			name: "value containing equals and malformed pairs skipped",
			msg:  "add@/x\x00ACTION=add\x00SUBSYSTEM=pci\x00MODALIAS=a=b\x00novalue\x00=orphan\x00",
			want: Uevent{
				Action:    "add",
				DevPath:   "/x",
				Subsystem: "pci",
				Env:       map[string]string{"ACTION": "add", "SUBSYSTEM": "pci", "MODALIAS": "a=b"},
			},
			wantOK: true,
		},
		{name: "libudev message", msg: "libudev\x00\xfe\xed\xca\xfe\x28\x00\x00\x00", wantOK: false},
		{name: "empty", msg: "", wantOK: false},
		{name: "no header", msg: "ACTION=add\x00SUBSYSTEM=block\x00", wantOK: false},
		{name: "empty action", msg: "@/devices/x\x00", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseUevent([]byte(tt.msg))
			if ok != tt.wantOK {
				t.Fatalf("ParseUevent ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseUevent = %+v, want %+v", got, tt.want)
			}
		})
	}
}
