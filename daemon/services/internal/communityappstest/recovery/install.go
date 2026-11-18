// Package recovery supplies interrupted CA install fixtures for transport tests.
package recovery

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

// InterruptedStart leaves a real durable start intent and native template, but
// makes Docker inspection unavailable to the original worker. The returned
// service shares the flash and Engine state as if the daemon had restarted.
func InterruptedStart(t *testing.T, environment string, started bool) (*communityapps.Service, communityapps.Attempt) {
	t.Helper()
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.InstallEngine{
		PostStartInspectError: errors.New(environment),
		NoStartEffect:         !started,
		BeforeRemove:          func() error { return errors.New(environment) },
	}
	newService := func() *communityapps.Service {
		service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
			HostRoot:          root,
			OpenDocker:        func() (communityapps.PlanningDocker, error) { return docker, nil },
			OpenInstallDocker: func() (communityapps.InstallDocker, error) { return docker, nil },
			PortAvailable:     func(context.Context, communityapps.PortBinding) error { return nil },
		})
		t.Cleanup(service.Close)
		return service
	}
	before := newService()
	preview, err := before.Prepare(t.Context(), communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference, Environment: map[string]string{"PUID": environment},
	})
	if err != nil || !preview.Eligible {
		t.Fatalf("fixture prepare returned %v (%+v)", err, preview.Issues)
	}
	attempt, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, _, starts, _, _ := docker.Observation(); len(starts) != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture Docker start was never issued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	before.Close()
	pending, err := before.GetAttempt(attempt.ID)
	if err != nil || pending.Phase != "starting_container" {
		t.Fatalf("fixture start intent was not retained: %+v, %v", pending, err)
	}
	docker.PostStartInspectError = nil
	return newService(), attempt
}
