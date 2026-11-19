package communityapps_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestConfirmationRejectsOversizedReceiptAndKeepsPriorAttemptsReadable(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	engine := &communityappstest.InstallEngine{}
	service := newInstallService(t, root, engine)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	normal, err := service.Prepare(t.Context(), req)
	if err != nil || !normal.Eligible {
		t.Fatalf("normal preview: %+v, %v", normal.Issues, err)
	}
	first, err := service.Confirm(t.Context(), normal.PlanID, normal.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	if got := waitForAttempt(t, service, first.ID); got.State != communityapps.AttemptSucceeded {
		t.Fatalf("normal attempt: %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(root, communityappstest.CachePath))
	if err != nil {
		t.Fatal(err)
	}
	const count = 245
	source := "/mnt/cache/application-data/" + strings.Repeat("&", 190)
	var fields strings.Builder
	for i := range count {
		target := fmt.Sprintf("/data-%03d", i)
		fmt.Fprintf(&fields, `i:%d;a:1:{s:11:"@attributes";a:4:{s:6:"Target";s:%d:"%s";s:4:"Type";s:4:"Path";s:4:"Mode";s:2:"ro";s:7:"Default";s:%d:"%s";}}`, i+7, len(target), target, len(source), source)
	}
	const config = `s:6:"Config";a:7:{`
	cache := strings.Replace(string(data), config, fmt.Sprintf(`s:6:"Config";a:%d:{%s`, count+7, fields.String()), 1)
	communityappstest.WriteFile(t, root, communityappstest.CachePath, cache)
	req.Name = "oversized"
	req.Ports = map[string]communityapps.PortOverride{"80/tcp": {HostPort: 18080}, "443/tcp": {HostPort: 18443}}
	large, err := service.Prepare(t.Context(), req)
	if err != nil || !large.Eligible {
		t.Fatalf("oversized fixture preview: %+v, %v", large.Issues, err)
	}
	if _, err := service.Confirm(t.Context(), large.PlanID, large.ApprovalToken); !errors.Is(err, communityapps.ErrInstallPlanTooLarge) {
		t.Fatalf("confirmation accepted a receipt larger than it can reload: %v", err)
	}
	service.Close()
	restarted := newInstallService(t, root, engine)
	if got, err := restarted.GetAttempt(first.ID); err != nil || got.State != communityapps.AttemptSucceeded {
		t.Fatalf("previously accepted attempt no longer readable: %+v, %v", got, err)
	}
	if creates, _, _, _, _ := engine.Observation(); creates != 1 {
		t.Fatalf("oversized receipt reached Docker create: %d calls", creates)
	}
}

func TestDefaultPlanningDockerRefusesRemoteEngine(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	var requests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "fixture must not be called", http.StatusBadRequest)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("DOCKER_HOST", remote.URL)
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{HostRoot: root})
	t.Cleanup(service.Close)
	_, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if !errors.Is(err, communityapps.ErrDockerState) || requests.Load() != 0 {
		t.Fatalf("default preparation used the remote Docker host: %v, %d requests", err, requests.Load())
	}
}
