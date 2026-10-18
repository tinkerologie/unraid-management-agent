package collectors

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

// fakeUeventConn returns each message written to msgs from one Read. Close
// unblocks a pending Read, like the netlink socket.
type fakeUeventConn struct {
	msgs     chan []byte
	readErr  chan error
	done     chan struct{}
	closeOne sync.Once
}

func newFakeUeventConn() *fakeUeventConn {
	return &fakeUeventConn{msgs: make(chan []byte), readErr: make(chan error), done: make(chan struct{})}
}

func (f *fakeUeventConn) Read(p []byte) (int, error) {
	select {
	case m := <-f.msgs:
		return copy(p, m), nil
	case err := <-f.readErr:
		return 0, err
	case <-f.done:
		return 0, io.ErrClosedPipe
	}
}

func (f *fakeUeventConn) Close() error {
	f.closeOne.Do(func() { close(f.done) })
	return nil
}

func uevent(fields ...string) []byte {
	var b []byte
	for _, f := range fields {
		b = append(b, f...)
		b = append(b, 0)
	}
	return b
}

func TestUeventSource(t *testing.T) {
	first := newFakeUeventConn()
	second := newFakeUeventConn()
	opens := 0
	open := func() (io.ReadCloser, error) {
		opens++
		switch opens {
		case 1:
			return nil, errors.New("permission denied")
		case 2:
			return first, nil
		default:
			return second, nil
		}
	}
	src := fastSource(newUeventSource(open, BlockDeviceUevent), 20*time.Millisecond)

	rec := newTriggerRecorder()
	stop := runSource(t, src, rec.trigger)

	rec.wait(t) // catch-up after open
	first.msgs <- uevent("add@/devices/usb1", "ACTION=add", "SUBSYSTEM=usb")
	first.msgs <- []byte("libudev\x00\xfe\xed")
	first.msgs <- uevent("add@/block/sdb", "ACTION=add", "SUBSYSTEM=block", "DEVTYPE=disk")
	first.msgs <- uevent("add@/block/sdb/sdb1", "ACTION=add", "SUBSYSTEM=block", "DEVTYPE=partition")
	rec.wait(t) // one debounced trigger for the disk and its partition

	first.readErr <- errors.New("no buffer space available")
	rec.wait(t) // catch-up after reopen

	stop()
	select {
	case <-second.done:
	default:
		t.Error("socket not closed after cancel")
	}
	select {
	case <-first.done:
	default:
		t.Error("socket not closed after read error")
	}
	if got := rec.n.Load(); got != 3 {
		t.Errorf("triggers = %d, want 3", got)
	}
}

func TestUeventSource_Unsupported(t *testing.T) {
	opens := 0
	src := fastSource(newUeventSource(func() (io.ReadCloser, error) {
		opens++
		return nil, errors.ErrUnsupported
	}, BlockDeviceUevent), time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	src.Run(ctx, func() { t.Error("unexpected trigger") })
	if opens != 1 {
		t.Errorf("opens = %d, want 1", opens)
	}
}

func TestNewUeventSource_ReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	NewUeventSource(HardwareDeviceUevent).Run(ctx, func() {})
}

func TestUeventFilters(t *testing.T) {
	tests := []struct {
		name         string
		event        lib.Uevent
		wantBlock    bool
		wantHardware bool
	}{
		{"disk added", lib.Uevent{Action: "add", Subsystem: "block", DevType: "disk"}, true, false},
		{"partition removed", lib.Uevent{Action: "remove", Subsystem: "block", DevType: "partition"}, true, false},
		{"disk changed", lib.Uevent{Action: "change", Subsystem: "block"}, false, false},
		{"usb added", lib.Uevent{Action: "add", Subsystem: "usb"}, false, true},
		{"pci removed", lib.Uevent{Action: "remove", Subsystem: "pci"}, false, true},
		{"thunderbolt added", lib.Uevent{Action: "add", Subsystem: "thunderbolt"}, false, true},
		{"usb bind", lib.Uevent{Action: "bind", Subsystem: "usb"}, false, false},
		{"net added", lib.Uevent{Action: "add", Subsystem: "net"}, false, false},
		{"empty", lib.Uevent{}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BlockDeviceUevent(tt.event); got != tt.wantBlock {
				t.Errorf("BlockDeviceUevent = %v, want %v", got, tt.wantBlock)
			}
			if got := HardwareDeviceUevent(tt.event); got != tt.wantHardware {
				t.Errorf("HardwareDeviceUevent = %v, want %v", got, tt.wantHardware)
			}
		})
	}
}
