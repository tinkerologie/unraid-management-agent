package collectors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

const (
	// ueventDebounce coalesces the burst of events a single hotplug produces
	// (a disk and its partitions, a USB device and its interfaces).
	ueventDebounce = 500 * time.Millisecond
	// ueventBufferSize fits the largest uevent message (UEVENT_BUFFER_SIZE is 2 KiB).
	ueventBufferSize = 16 << 10
)

// hardwareUeventSubsystems are the bus subsystems whose hotplug changes the
// hardware inventory.
var hardwareUeventSubsystems = []string{"usb", "pci", "thunderbolt"}

// BlockDeviceUevent matches block devices being added or removed.
func BlockDeviceUevent(e lib.Uevent) bool {
	return e.Subsystem == "block" && isAddOrRemove(e.Action)
}

// HardwareDeviceUevent matches USB, PCI and Thunderbolt devices being added
// or removed.
func HardwareDeviceUevent(e lib.Uevent) bool {
	return slices.Contains(hardwareUeventSubsystems, e.Subsystem) && isAddOrRemove(e.Action)
}

// PSUDeviceUevent notices USB hotplug and hwmon registration/removal. Sensor
// reads still validate exact identity; an unrelated event only wakes inventory.
func PSUDeviceUevent(e lib.Uevent) bool {
	return (e.Subsystem == "usb" || e.Subsystem == "hwmon") && isAddOrRemove(e.Action)
}

// isAddOrRemove reports a device add or remove action.
func isAddOrRemove(action string) bool {
	return action == "add" || action == "remove"
}

// NewUeventSource returns an EventSource that reports kernel uevents accepted
// by match. On systems without kernel uevents, Run just waits for ctx.
func NewUeventSource(match func(lib.Uevent) bool) EventSource {
	return newUeventSource(lib.OpenUevents, match)
}

// newUeventSource builds the uevent source on an injectable socket opener.
func newUeventSource(open func() (io.ReadCloser, error), match func(lib.Uevent) bool) *sessionSource {
	return newSessionSource("Uevent source", ueventDebounce, func(ctx context.Context, connected, changed func()) error {
		conn, err := open()
		if errors.Is(err, errors.ErrUnsupported) {
			logger.Debug("Uevent source: kernel uevents not supported on this OS")
			<-ctx.Done()
			return ctx.Err()
		}
		if err != nil {
			return fmt.Errorf("opening uevent socket: %w", err)
		}
		// Closing the socket unblocks Read once ctx is cancelled.
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer func() {
			if stop() {
				_ = conn.Close()
			}
		}()
		connected()

		buf := make([]byte, ueventBufferSize)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return fmt.Errorf("reading uevent: %w", err)
			}
			if e, ok := lib.ParseUevent(buf[:n]); ok && match(e) {
				changed()
			}
		}
	})
}
