// Package recovery supplies interrupted CA install fixtures for transport tests.
package recovery

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/containerd/errdefs"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

// InterruptedStart leaves a real durable start intent and native template, but
// makes Docker inspection unavailable to the original worker. The returned
// service shares the flash and Engine state as if the daemon had restarted.
func InterruptedStart(t *testing.T, environment string, started bool) (*communityapps.Service, communityapps.Attempt) {
	t.Helper()
	root := communityappstest.CopyHeimdallHost(t)
	startIssued := make(chan struct{})
	docker := &communityappstest.InstallEngine{
		PostStartInspectError: errors.New(environment),
		BeforeRemove:          func() error { return errors.New(environment) },
		BeforeStart: func() error {
			close(startIssued)
			if !started {
				return errdefs.ErrInvalidArgument.WithMessage(environment)
			}
			return nil
		},
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
	communityappstest.Require(t, err == nil && preview.Eligible, "fixture prepare returned %v (%+v)", err, preview.Issues)
	attempt, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	communityappstest.Require(t, err == nil, "confirm fixture install: %v", err)
	startCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err = waitForStartRequest(startCtx, startIssued)
	communityappstest.Require(t, err == nil, "wait for fixture Docker start: %v", err)
	before.Close()
	pending, err := before.GetAttempt(attempt.ID)
	communityappstest.Require(t, err == nil && pending.Phase == "starting_container", "fixture start intent was not retained: %+v, %v", pending, err)
	docker.PostStartInspectError = nil
	return newService(), attempt
}

func waitForStartRequest(ctx context.Context, startIssued <-chan struct{}) error {
	select {
	case <-startIssued:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
