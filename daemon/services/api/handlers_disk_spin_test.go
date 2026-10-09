package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

type spinCall struct {
	diskID string
	up     bool
}

// stubDiskSpin replaces the emhttpd call on the server.
func stubDiskSpin(server *Server, err error) *[]spinCall {
	calls := &[]spinCall{}
	server.diskSpinFn = func(diskID string, up bool) error {
		*calls = append(*calls, spinCall{diskID: diskID, up: up})
		return err
	}
	return calls
}

func serveDiskSpin(t *testing.T, server *Server, path string) (int, dto.Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)
	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	return rr.Code, resp
}

func diskSpinTestServer() *Server {
	server, _ := setupTestServer()
	disks := []dto.DiskInfo{
		{ID: "disk1", Device: "sdb", Name: "Disk 1"},
		{ID: "parity", Device: "sdc", Name: "Parity"},
	}
	server.disksCache.Store(&disks)
	return server
}

func TestHandleDiskSpin_UpAndDownByID(t *testing.T) {
	server := diskSpinTestServer()
	calls := stubDiskSpin(server, nil)

	code, resp := serveDiskSpin(t, server, "/api/v1/disks/disk1/spinup")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("spinup: got %d %+v", code, resp)
	}
	code, resp = serveDiskSpin(t, server, "/api/v1/disks/parity/spindown")
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("spindown: got %d %+v", code, resp)
	}

	want := []spinCall{{diskID: "disk1", up: true}, {diskID: "parity", up: false}}
	if len(*calls) != len(want) || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Errorf("controller calls = %+v, want %+v", *calls, want)
	}
}

func TestHandleDiskSpin_ResolvesDeviceToDiskID(t *testing.T) {
	server := diskSpinTestServer()
	calls := stubDiskSpin(server, nil)

	code, _ := serveDiskSpin(t, server, "/api/v1/disks/sdb/spindown")

	if code != http.StatusOK {
		t.Fatalf("got status %d", code)
	}
	if len(*calls) != 1 || (*calls)[0].diskID != "disk1" {
		t.Errorf("expected the emhttpd id disk1, got %+v", *calls)
	}
}

func TestHandleDiskSpin_UnknownDiskIsNotFound(t *testing.T) {
	server := diskSpinTestServer()
	calls := stubDiskSpin(server, nil)

	code, resp := serveDiskSpin(t, server, "/api/v1/disks/disk99/spinup")

	if code != http.StatusNotFound || resp.Success {
		t.Fatalf("got %d %+v, want 404", code, resp)
	}
	if len(*calls) != 0 {
		t.Errorf("controller must not be called for unknown disks, got %+v", *calls)
	}
}

func TestHandleDiskSpin_ControllerError(t *testing.T) {
	server := diskSpinTestServer()
	stubDiskSpin(server, errors.New("emhttpd unavailable"))

	code, resp := serveDiskSpin(t, server, "/api/v1/disks/disk1/spindown")

	if code != http.StatusInternalServerError || resp.Success {
		t.Fatalf("got %d %+v, want 500", code, resp)
	}
}

func TestDiskSpinDefault_FailsOffUnraid(t *testing.T) {
	// Without the emhttpd socket or /proc/mdcmd the real controller must
	// report an error rather than pretend the disk was spun.
	server, _ := setupTestServer()
	if err := server.spinDisk("disk1", true); err == nil {
		t.Error("spin up: expected an error outside Unraid")
	}
	if err := server.spinDisk("disk1", false); err == nil {
		t.Error("spin down: expected an error outside Unraid")
	}
}
