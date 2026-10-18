package lib

import (
	"bytes"
	"strings"
)

// Uevent is a kernel device event (add, remove, change, ...) received over
// the NETLINK_KOBJECT_UEVENT socket.
type Uevent struct {
	Action    string
	DevPath   string
	Subsystem string
	DevType   string
	// Env holds every KEY=VALUE pair of the message.
	Env map[string]string
}

// ParseUevent parses a kernel uevent message: an "ACTION@DEVPATH" header
// followed by NUL-separated KEY=VALUE pairs. It returns false for malformed
// messages and for messages re-broadcast by udev (which start with "libudev").
func ParseUevent(b []byte) (Uevent, bool) {
	if bytes.HasPrefix(b, []byte("libudev")) {
		return Uevent{}, false
	}
	fields := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	action, devPath, ok := strings.Cut(fields[0], "@")
	if !ok || action == "" {
		return Uevent{}, false
	}

	e := Uevent{Action: action, DevPath: devPath, Env: make(map[string]string, len(fields)-1)}
	for _, f := range fields[1:] {
		if k, v, ok := strings.Cut(f, "="); ok && k != "" {
			e.Env[k] = v
		}
	}
	if v := e.Env["ACTION"]; v != "" {
		e.Action = v
	}
	if v := e.Env["DEVPATH"]; v != "" {
		e.DevPath = v
	}
	e.Subsystem = e.Env["SUBSYSTEM"]
	e.DevType = e.Env["DEVTYPE"]
	return e, true
}
