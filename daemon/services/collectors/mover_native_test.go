package collectors

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

func TestMoverNativeStatusAndProgress(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mover")
	if err := os.WriteFile(bin, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	_, idle := lib.ExecCommandOutputWithContext(t.Context(), "false")
	const valid = "TotalSize=20000000\nRemainSize=5000000\nThroughput=2500000\nETA=2\nMoverPid=123\nFile=/private/transfer\n" // pragma: allowlist secret
	for _, tc := range []struct {
		name, version, snapshot  string
		command                  error
		wantActive, wantProgress bool
		source                   string
	}{
		{"active", "7.4.0", valid, nil, true, true, "native"},
		{"idle", "7.4.0", valid, idle, false, false, "native"},
		{"opt out", "7.4.0", "", nil, true, false, "native"},
		{"different PID", "7.4.0", strings.Replace(valid, "Pid=123", "Pid=124", 1), nil, true, false, "native"},
		{"malformed", "7.4.0", strings.Replace(valid, "ETA=2", "ETA=-2", 1), nil, true, false, "native"},
		{"older", "7.3.4", valid, nil, true, false, "legacy"},
		{"unknown", "7.4.0", valid, context.DeadlineExceeded, true, false, "legacy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := &moverChecker{
				version: func() string { return tc.version },
				stat:    func(string) (os.FileInfo, error) { return os.Stat(bin) },
				status:  func(context.Context) error { return tc.command },
				open: func(name string) (io.ReadCloser, error) {
					switch name {
					case constants.VarIni:
						return io.NopCloser(strings.NewReader("shareMoverActive=yes\nshareMoverSchedule=10 4 * * *\n")), nil
					case constants.MoverPID:
						return io.NopCloser(strings.NewReader("123\n")), nil
					case constants.MoverProgress:
						if tc.snapshot != "" {
							return io.NopCloser(strings.NewReader(tc.snapshot)), nil
						}
					}
					return nil, os.ErrNotExist
				},
			}
			hub := domain.NewEventBus(2)
			sub := hub.Sub(constants.TopicMoverUpdate.Name)
			t.Cleanup(func() { hub.Unsub(sub) })
			settings := &SettingsCollector{mover: checker}
			collector := NewMoverCollector(&domain.Context{Hub: hub})
			collector.settings = settings
			collector.Collect(t.Context())
			select {
			case event := <-sub:
				got := event.(*dto.MoverStatus)
				config, err := settings.GetMoverSettings()
				if err != nil || config.Active != tc.wantActive || got.Active != tc.wantActive || got.StatusSource != tc.source {
					t.Fatalf("mover status mismatch: settings %+v, event %+v, err %v", config, got, err)
				}
				wire, err := json.Marshal(got)
				if err != nil || strings.Contains(string(wire), "/private/transfer") {
					t.Fatalf("mover status not safe to publish: %s, %v", wire, err)
				}
				if tc.wantProgress {
					if got.ProgressPercent == nil || *got.ProgressPercent != 75 || got.CurrentThroughputMBs != 2.5 ||
						got.ETASeconds == nil || *got.ETASeconds != 2 || got.ProgressMovedBytes == nil || *got.ProgressMovedBytes != 15000000 {
						t.Fatalf("progress fields were not populated: %s", wire)
					}
				} else if got.ProgressTotalBytes != nil || got.CurrentThroughputMBs != 0 || got.ETASeconds != nil {
					t.Fatalf("unexpected live progress: %s", wire)
				}
			case <-time.After(time.Second):
				t.Fatal("Mover did not publish")
			}
		})
	}
}
