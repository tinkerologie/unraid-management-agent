package dto

import (
	"reflect"
	"strings"
	"testing"
)

// TestAlertEnvSourceTags guards that every AlertEnv field, including future
// ones, names the collectors that feed it, so Dynamic collection mode keeps
// alert data fresh.
func TestAlertEnvSourceTags(t *testing.T) {
	collectors := map[string]bool{}
	for _, name := range []string{
		"system", "array", "disk", "docker", "vm",
		"ups", "nut", "gpu", "shares", "network",
		"hardware", "zfs", "notification", "registration", "unassigned",
		"fancontrol", "tuning", "docker_update", "docker_networks", "plugin_update",
		"os_update", "mover", "storage_topology", "nic",
	} {
		collectors[name] = true
	}

	typ := reflect.TypeFor[AlertEnv]()
	for i := range typ.NumField() {
		f := typ.Field(i)
		t.Run(f.Name, func(t *testing.T) {
			src, ok := f.Tag.Lookup("source")
			if !ok || src == "" {
				t.Fatalf("AlertEnv.%s has no source tag", f.Name)
			}
			if src == "-" {
				return
			}
			for c := range strings.SplitSeq(src, ",") {
				if !collectors[c] {
					t.Errorf("AlertEnv.%s source %q is not a collector name", f.Name, c)
				}
			}
		})
	}
}
