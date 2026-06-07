package lib

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DiskTempsPath is the default Unraid disks.ini location.
const DiskTempsPath = "/boot/config/disks.ini"

// DiskTemp is the temperature and spin state of a single Unraid disk,
// parsed from disks.ini. It never wakes a drive: Unraid writes "*" for a
// spun-down disk, which this maps to SpunDown=true.
type DiskTemp struct {
	ID       string  // disks.ini section name: "disk1", "cache", "parity"
	Device   string  // "sdb"
	TempC    float64 // 0 when unavailable
	SpunDown bool    // true when no usable temperature is available (Unraid "*"/empty, missing temp key, or unparsable value)
}

// ReadDiskTemps parses the default disks.ini (/boot/config/disks.ini).
func ReadDiskTemps() (map[string]DiskTemp, error) {
	return ReadDiskTempsFromFile(DiskTempsPath)
}

// ReadDiskTempsFromFile parses the given disks.ini and returns temps keyed by
// disk ID (the section-header name). The returned map is always non-nil.
func ReadDiskTempsFromFile(path string) (map[string]DiskTemp, error) {
	result := make(map[string]DiskTemp)

	// #nosec G304 -- callers are either ReadDiskTemps (fixed const) or tests; never user-controlled input.
	file, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("open disks.ini: %w", err)
	}
	defer func() { _ = file.Close() }()

	var cur *DiskTemp
	flush := func() {
		if cur != nil && cur.ID != "" {
			result[cur.ID] = *cur
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			id := strings.TrimPrefix(line, "[")
			id = strings.TrimSuffix(id, "]")
			id = strings.Trim(id, `"`)
			cur = &DiskTemp{ID: id, SpunDown: true} // assume no reading until a valid temp parses
			continue
		}

		if cur == nil || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "device":
			cur.Device = val
		case "temp":
			// Unraid writes "*" (or empty) for a spun-down disk — do NOT wake it.
			if t, perr := strconv.ParseFloat(val, 64); perr == nil {
				cur.TempC = t
				cur.SpunDown = false
			}
			// "*", "", or unparsable leaves SpunDown=true (set at section start).
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("scan disks.ini: %w", err)
	}
	return result, nil
}
