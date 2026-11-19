package communityapps_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

const volumeOverlapSource = "/mnt/cache/application-data/heimdall"

func TestPrepareChecksBindBackedNamedVolumeOverlap(t *testing.T) {
	volumeMount := bindBackedVolumeMount("shared")
	for _, tc := range []struct {
		name       string
		mounted    container.MountPoint
		metadata   volume.Volume
		source     string
		allowShare bool
		wantShare  bool
		wantReads  int32
		want       error
	}{
		{name: "bind requires opt-in", mounted: volumeMount, metadata: bindBackedVolume("shared", volumeOverlapSource, "bind"), wantReads: 1, want: communityapps.ErrSharedAppdata},
		{name: "rbind allows explicit sharing", mounted: volumeMount, metadata: bindBackedVolume("shared", volumeOverlapSource, "rbind,rw"), allowShare: true, wantShare: true, wantReads: 1},
		{name: "distinct path is eligible", mounted: volumeMount, metadata: bindBackedVolume("shared", volumeOverlapSource, "bind"), source: "/mnt/cache/application-data/separate", wantReads: 1},
		{name: "writable parent remains disallowed", mounted: volumeMount, metadata: bindBackedVolume("shared", "/mnt/cache/application-data", "bind"), allowShare: true, wantReads: 1, want: communityapps.ErrExposedMountParent},
		{name: "ordinary local volume", mounted: volumeMount, metadata: volume.Volume{Name: "shared", Driver: "local", Scope: "local"}, wantReads: 1},
		{name: "read-only named volume", mounted: container.MountPoint{Type: mount.TypeVolume, Name: "ignored", RW: false}},
		{name: "non-volume mount", mounted: container.MountPoint{Type: mount.TypeTmpfs, RW: true}},
		{name: "unavailable volume metadata", mounted: volumeMount, wantReads: 1, want: communityapps.ErrDockerState},
		{name: "mount and metadata names differ", mounted: volumeMount, metadata: bindBackedVolume("different", volumeOverlapSource, "bind"), wantReads: 1, want: communityapps.ErrDockerState},
		{name: "mount and metadata drivers differ", mounted: volumeMount, metadata: volume.Volume{Name: "shared", Driver: "other", Scope: "global"}, wantReads: 1, want: communityapps.ErrDockerState},
		{name: "known other volume driver", mounted: container.MountPoint{Type: mount.TypeVolume, Driver: "plugin", Name: "shared", RW: true}, metadata: volume.Volume{Name: "shared", Driver: "plugin", Scope: "global"}, wantReads: 1},
		{name: "malformed metadata scope", mounted: volumeMount, metadata: volume.Volume{Name: "shared", Driver: "local"}, wantReads: 1, want: communityapps.ErrDockerState},
		{name: "bind options without absolute source", mounted: volumeMount, metadata: bindBackedVolume("shared", "relative/path", "bind"), wantReads: 1, want: communityapps.ErrDockerState},
		{name: "malformed mount name", mounted: bindBackedVolumeMount("../elsewhere"), want: communityapps.ErrDockerState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			docker := &namedVolumeEngine{InstallEngine: &communityappstest.InstallEngine{
				Docker: communityappstest.Docker{Containers: []container.Summary{{Mounts: []container.MountPoint{tc.mounted, tc.mounted}}}},
			}, metadata: tc.metadata}
			if tc.metadata.Name == "" {
				docker.inspectErr = errors.New("fixture volume metadata unavailable")
			}
			service := newVolumeService(t, root, docker)
			source := tc.source
			if source == "" {
				source = volumeOverlapSource
			}
			req := communityapps.PrepareRequest{
				Reference: communityappstest.HeimdallReference,
				Paths:     map[string]communityapps.PathOverride{"/config": {Source: source, AllowShared: tc.allowShare}},
			}
			preview, err := service.Prepare(t.Context(), req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Prepare error = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (!preview.Eligible || len(preview.Mounts) != 1 || preview.Mounts[0].Shared != tc.wantShare) {
				t.Fatalf("Prepare eligible = %v, mounts = %+v", preview.Eligible, preview.Mounts)
			}
			if reads := docker.volumeReads.Load(); reads != tc.wantReads {
				t.Fatalf("volume API reads = %v, want %v", reads, tc.wantReads)
			}
			if got := docker.Docker.Containers[0].Mounts[0]; got != tc.mounted {
				t.Fatalf("preparation altered Docker's listed mount: %+v", got)
			}
		})
	}
}

func TestConfirmRechecksNewlyAddedNamedVolumeBinds(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &namedVolumeEngine{
		InstallEngine: &communityappstest.InstallEngine{},
		metadata:      bindBackedVolume("late", volumeOverlapSource, "bind"),
	}
	service := newVolumeService(t, root, docker)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("Prepare eligible = %v, error = %v", preview.Eligible, err)
	}
	docker.showLateVolume.Store(true)
	if _, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrSharedAppdata) {
		t.Fatalf("confirmation did not detect newly overlapping volume: %v", err)
	}
}

func TestInstallRechecksNamedVolumeBeforeStarting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		inspectErr error
		want       *communityapps.Problem
	}{
		{name: "overlap appears", want: communityapps.ErrSharedAppdata},
		{name: "metadata is unreadable", inspectErr: errors.New("volume inspect unavailable"), want: communityapps.ErrDockerState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			docker := &namedVolumeEngine{
				InstallEngine: &communityappstest.InstallEngine{},
				metadata:      bindBackedVolume("late", volumeOverlapSource, "bind"),
				inspectErr:    tc.inspectErr,
			}
			docker.AfterCreate = func() error {
				docker.showLateVolume.Store(true)
				return nil
			}
			service := newVolumeService(t, root, docker)
			preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("Prepare eligible = %v, error = %v", preview.Eligible, err)
			}
			attempt, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatalf("Confirm: %v", err)
			}
			result := waitForAttempt(t, service, attempt.ID)
			if result.State != communityapps.AttemptFailed || result.Problem == nil || result.Problem.Code != tc.want.Code {
				t.Fatalf("pre-start check returned state %v and problem %v, want %v", result.State, result.Problem, tc.want)
			}
			if _, starts, _, _, _ := docker.Observation(); starts != 0 {
				t.Fatalf("installer started a container despite unresolved mount overlap: %v", starts)
			}
		})
	}
}

func TestPrepareFailsClosedIfVolumeInspectorIsUnavailable(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.Docker{Containers: []container.Summary{{Mounts: []container.MountPoint{bindBackedVolumeMount("unknown")}}}}
	service := newPlanningService(t, root, docker, nil)
	t.Cleanup(service.Close)
	if _, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}); !errors.Is(err, communityapps.ErrDockerState) {
		t.Fatalf("unavailable Docker volume inspector returned %v", err)
	}

	// There are no appdata overlaps to resolve when the plan has only ports
	// and environment declarations. No inspection is required for that plan.
	rewriteCatalogString(t, root, "Path", "Variable", `s:4:"Type";`)
	rewriteCatalogString(t, root, "/config", "STORAGE_PATH", `s:6:"Target";`)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
		Reference:   communityappstest.HeimdallReference,
		Environment: map[string]string{"STORAGE_PATH": "fixture"},
	})
	if err != nil || !preview.Eligible || len(preview.Mounts) != 0 {
		t.Fatalf("plan without mounts: eligible = %v, mounts = %+v, issues = %v, error = %v", preview.Eligible, preview.Mounts, preview.Issues, err)
	}
}

type namedVolumeEngine struct {
	*communityappstest.InstallEngine
	metadata       volume.Volume
	inspectErr     error
	volumeReads    atomic.Int32
	showLateVolume atomic.Bool
}

func (e *namedVolumeEngine) ContainerList(ctx context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	result, err := e.InstallEngine.ContainerList(ctx, opts)
	if e.showLateVolume.Load() {
		result.Items = append(slices.Clone(result.Items), container.Summary{Mounts: []container.MountPoint{bindBackedVolumeMount("late")}})
	}
	return result, err
}

func (e *namedVolumeEngine) VolumeInspect(_ context.Context, _ string, _ client.VolumeInspectOptions) (client.VolumeInspectResult, error) {
	e.volumeReads.Add(1)
	return client.VolumeInspectResult{Volume: e.metadata}, e.inspectErr
}

func bindBackedVolumeMount(name string) container.MountPoint {
	return container.MountPoint{Type: mount.TypeVolume, Name: name, Driver: "local", RW: true, Source: "/var/lib/docker/volumes/" + name + "/_data"}
}

func bindBackedVolume(name, source, options string) volume.Volume {
	return volume.Volume{Name: name, Driver: "local", Scope: "local", Options: map[string]string{"type": "none", "device": source, "o": options}}
}

func newVolumeService(t *testing.T, root string, docker *namedVolumeEngine) *communityapps.Service {
	t.Helper()
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot: root,
		OpenDocker: func() (communityapps.PlanningDocker, error) {
			return docker, nil
		},
		OpenInstallDocker: func() (communityapps.InstallDocker, error) {
			return docker, nil
		},
		PortAvailable: func(context.Context, communityapps.PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	return service
}
