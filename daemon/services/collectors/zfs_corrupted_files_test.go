package collectors

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// zpoolStatusNoErrors is the tail of `zpool status -v` for a healthy pool.
const zpoolStatusNoErrors = `  pool: tank
 state: ONLINE
  scan: scrub repaired 0B in 00:00:02 with 0 errors on Sun Oct  4 05:00:03 2026
config:

	NAME        STATE     READ WRITE CKSUM
	tank        ONLINE       0     0     0
	  sdb1      ONLINE       0     0     0

errors: No known data errors
`

// TestParsePoolStatusCorruptedFiles checks that the paths listed under
// "errors: Permanent errors have been detected in the following files:" are
// collected. OpenZFS print_error_log() (cmd/zpool/zpool_main.c) prints that
// header, a blank line, then each path as "%7s %s" (8 spaces of indent), in
// the forms produced by zpool_obj_to_path(): a path under the mountpoint,
// "<metadata>:<0x..>", "dataset:<0x..>", "dataset:/path" for an unmounted
// dataset and "<0x..>:<0x..>" for a dataset that no longer exists.
func TestParsePoolStatusCorruptedFiles(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "zpool", "status-v-permanent-errors.txt"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "permanent errors with all path forms",
			output: string(fixture),
			want: []string{
				"/mnt/tank/media/movie: part 1.mkv",
				"<metadata>:<0x1a>",
				"tank/appdata:<0x2f1>",
				"tank/backups:/2026/09/db.dump",
				"<0x5f>:<0x3>",
			},
		},
		{
			name: "list ends at the next pool header",
			output: string(fixture) + "\n" +
				"  pool: other\n" +
				" state: ONLINE\n",
			want: []string{
				"/mnt/tank/media/movie: part 1.mkv",
				"<metadata>:<0x1a>",
				"tank/appdata:<0x2f1>",
				"tank/backups:/2026/09/db.dump",
				"<0x5f>:<0x3>",
			},
		},
		{
			name:   "no known data errors",
			output: zpoolStatusNoErrors,
			want:   []string{},
		},
		{
			// Only printed without -v; the file list is unknown, so it stays nil.
			name:   "error count without a file list",
			output: zpoolStatusNoErrors[:len(zpoolStatusNoErrors)-len("No known data errors\n")] + "2 data errors, use '-v' for a list\n",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ZFSCollector{execOutput: func(command string, args ...string) (string, error) {
				if command != constants.ZpoolBin || !reflect.DeepEqual(args, []string{"status", "-v", "tank"}) {
					t.Errorf("unexpected command %s %v", command, args)
				}
				return tt.output, nil
			}}
			pool := &dto.ZFSPool{Name: "tank"}

			if err := c.parsePoolStatus(pool); err != nil {
				t.Fatalf("parsePoolStatus() error = %v", err)
			}
			if !reflect.DeepEqual(pool.CorruptedFiles, tt.want) {
				t.Errorf("CorruptedFiles = %q, want %q", pool.CorruptedFiles, tt.want)
			}
			// The error list must not disturb the rest of the status parse.
			if pool.State != "ONLINE" || len(pool.VDEVs) != 1 {
				t.Errorf("State = %q, VDEVs = %d; want ONLINE, 1", pool.State, len(pool.VDEVs))
			}
		})
	}
}

// TestZFSPoolCorruptedFilesJSON checks that corrupted_files is always present
// in the JSON that the REST API, websocket, MCP and MQTT send: [] for a healthy
// pool, and null only when `zpool status` could not be read.
func TestZFSPoolCorruptedFilesJSON(t *testing.T) {
	c := &ZFSCollector{execOutput: func(string, ...string) (string, error) {
		return zpoolStatusNoErrors, nil
	}}
	healthy := dto.ZFSPool{Name: "tank"}
	if err := c.parsePoolStatus(&healthy); err != nil {
		t.Fatalf("parsePoolStatus() error = %v", err)
	}

	c.execOutput = func(string, ...string) (string, error) {
		return "", errors.New("zpool status failed")
	}
	failed := dto.ZFSPool{Name: "tank"}
	if err := c.parsePoolStatus(&failed); err == nil {
		t.Fatal("parsePoolStatus() error = nil, want the command error")
	}

	for _, tt := range []struct {
		pool dto.ZFSPool
		want string
	}{
		{healthy, `"corrupted_files":[]`},
		{failed, `"corrupted_files":null`},
	} {
		data, err := json.Marshal(tt.pool)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if !strings.Contains(string(data), tt.want) {
			t.Errorf("JSON %s does not contain %s", data, tt.want)
		}
	}
}
