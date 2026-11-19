package communityappstest

import (
	"errors"
	"io"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestInstallEngineReportsInspectionFailureForCreatedContainer(t *testing.T) {
	inspectErr := errors.New("inspect unavailable after create")
	docker := &InstallEngine{
		BeforeContainerInspect: func(_ string, c *container.InspectResponse) error {
			if c != nil {
				return inspectErr
			}
			return nil
		},
	}
	created, err := docker.ContainerCreate(t.Context(), client.ContainerCreateOptions{
		Name: "fixture-app", Config: &container.Config{}, HostConfig: &container.HostConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docker.ContainerInspect(t.Context(), created.ID, client.ContainerInspectOptions{}); !errors.Is(err, inspectErr) {
		t.Fatalf("fixture inspection returned %v, want %v", err, inspectErr)
	}
}

func TestInstallEngineRejectsStartingAMissingContainer(t *testing.T) {
	docker := &InstallEngine{}
	if _, err := docker.ContainerStart(t.Context(), ContainerID, client.ContainerStartOptions{}); !errdefs.IsNotFound(err) {
		t.Fatalf("starting a missing fixture container returned %v", err)
	}
}

func TestInstallEnginePullSurfacesStreamFailureToWaitingCaller(t *testing.T) {
	pullErr := errors.New("pull stream disconnected")
	docker := &InstallEngine{PullStreamErr: pullErr}
	stream, err := docker.ImagePull(t.Context(), "fixture-app:latest", client.ImagePullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	if n, err := stream.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("fixture stream raw read returned (%d, %v)", n, err)
	}
	if err := stream.Wait(t.Context()); !errors.Is(err, pullErr) {
		t.Fatalf("fixture stream wait returned %v, want %v", err, pullErr)
	}
}
