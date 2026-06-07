package collectors

import (
	"bufio"
	"os"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
)

// readFlatCfg parses an Unraid-style flat key="value" config file (e.g.
// docker.cfg, domain.cfg) into a map. Quotes around values and surrounding
// whitespace are stripped; blank and comment lines are skipped.
func readFlatCfg(path string) (map[string]string, error) {
	// #nosec G304 -- callers pass fixed Unraid config constants
	// (docker.cfg/domain.cfg) or test temp files; never user-controlled input.
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read-only; close error is irrelevant

	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// isCfgTruthy reports whether an Unraid cfg value represents an enabled/on state.
func isCfgTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1", "enable", "enabled", "on":
		return true
	default:
		return false
	}
}

// dockerServiceDisabled reports whether Docker is explicitly turned off in
// docker.cfg. See dockerServiceDisabledAt for the decision rules.
func dockerServiceDisabled() bool { return dockerServiceDisabledAt(constants.DockerCfg) }

// dockerServiceDisabledAt reports whether Docker is explicitly turned off in the
// given docker.cfg (DOCKER_ENABLED present and not truthy). It returns false
// when the config is unreadable or the key is absent, so a genuinely
// broken-but-enabled Docker is still reported as unavailable rather than
// silently treated as disabled.
func dockerServiceDisabledAt(path string) bool {
	cfg, err := readFlatCfg(path)
	if err != nil {
		return false
	}
	value, ok := cfg["DOCKER_ENABLED"]
	if !ok {
		return false
	}
	return !isCfgTruthy(value)
}

// vmServiceDisabled reports whether the Unraid VM manager is explicitly turned
// off in domain.cfg. See vmServiceDisabledAt for the decision rules.
func vmServiceDisabled() bool { return vmServiceDisabledAt(constants.DomainCfg) }

// vmServiceDisabledAt reports whether the Unraid VM manager is explicitly turned
// off in the given domain.cfg. An explicit DISABLE flag wins; otherwise a
// SERVICE value that is present but not enable-like means disabled. It returns
// false when the config is unreadable or neither key is present, mirroring
// dockerServiceDisabledAt's conservative default.
func vmServiceDisabledAt(path string) bool {
	cfg, err := readFlatCfg(path)
	if err != nil {
		return false
	}
	if value, ok := cfg["DISABLE"]; ok && isCfgTruthy(value) {
		return true
	}
	if value, ok := cfg["SERVICE"]; ok {
		return !isCfgTruthy(value)
	}
	return false
}
