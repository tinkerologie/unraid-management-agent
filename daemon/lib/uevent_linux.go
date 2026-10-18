//go:build linux

package lib

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ueventKernelGroup is the netlink multicast group of kernel uevents.
const ueventKernelGroup = 1

// The socket calls OpenUevents makes; tests replace them to inject failures.
var (
	ueventSocket = unix.Socket
	ueventBind   = unix.Bind
)

// OpenUevents opens a netlink socket subscribed to kernel uevents. Each Read
// returns one raw message for ParseUevent. Close unblocks a pending Read.
func OpenUevents() (io.ReadCloser, error) {
	// Non-blocking so os.File uses the runtime poller and Close wakes Read.
	fd, err := ueventSocket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, fmt.Errorf("creating uevent socket: %w", err)
	}
	if err := ueventBind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: ueventKernelGroup}); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("binding uevent socket: %w", err)
	}
	return os.NewFile(uintptr(fd), "uevent"), nil
}
