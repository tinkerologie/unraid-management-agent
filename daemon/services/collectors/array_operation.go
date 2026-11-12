package collectors

import (
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
)

const arrayOperationUnknown = "unknown"

// ArrayOperationStatus reports the native array operation without probing
// devices. An empty value means idle, and unknown means the native state
// cannot distinguish idle from maintenance.
func ArrayOperationStatus(section *ini.Section) string {
	status, _ := arrayOperation(section)
	return status
}

func arrayOperation(section *ini.Section) (status string, position uint64) {
	native := section.Key("mdResync").String()
	active, activeErr := strconv.ParseUint(native, 10, 64)
	pos, posErr := strconv.ParseUint(section.Key("mdResyncPos").String(), 10, 64)
	if posErr != nil {
		pos = 0
	}
	if activeErr != nil || active == 0 {
		if posErr != nil || (pos == 0 && native != "" && activeErr != nil) {
			return arrayOperationUnknown, 0
		}
		if pos == 0 {
			return "", 0
		}
		// Older snapshots may have no mdResync. Fall back to the
		// sampling delta only for an operation with a nonzero position.
		delta, _ := strconv.ParseUint(section.Key("mdResyncDt").String(), 10, 64)
		if activeErr == nil || delta == 0 {
			return "paused", pos
		}
	}

	action := section.Key("mdResyncAction").String()
	if action == "" {
		action = section.Key("sbSyncAction").String()
	}
	action = strings.ToLower(action)
	switch {
	case strings.Contains(action, "clear"):
		return "clearing", pos
	case strings.Contains(action, "recon"):
		return "reconstructing", pos
	default:
		return "running", pos
	}
}
