package collectors

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// validateRequiredKeys returns (ok, reason) for a parsed INI map. ok=false with a
// human-readable reason when any required key is missing/empty.
func validateRequiredKeys(parsed map[string]string, keys ...string) (bool, string) {
	missing := platform.MissingKeys(parsed, keys...)
	if len(missing) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("missing expected keys: %v", missing)
}
