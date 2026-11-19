package communityapps_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestPrepareViewsUnsupportedEffectiveFieldsTogether(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	entry := testCatalogEntry()
	entry["Config"] = []any{
		publisherField("Variable", "DUPLICATED", "", "1", "", false),
		publisherField("Variable", "DUPLICATED", "", "2", "", false),
		publisherField("Path", "/var/run/docker.sock", "ro", "/mnt/cache/application-data", "", false),
		publisherField("Path", "relative", "rw", "/mnt/cache/application-data", "", false),
		publisherField("Path", "/missing-required", "ro", "", "", true),
		publisherField("Path", "/empty-optional", "rw", "", "", false),
		publisherField("Path", "/implicit-mode", "", "/mnt/cache/application-data", "", false),
		publisherField("Variable", "VALID_OPTION", "", "|second", "second", false),
		publisherField("Variable", "NOT-SHELL-COMPATIBLE", "", "one", "", false),
		publisherField("Variable", "MISSING_REQUIRED", "", "", "", true),
		publisherField("Port", "080", "tcp", "8080", "", true),
	}
	req := prepareFixtureRequest(t, root, entry)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || preview.Eligible || preview.PlanID != "" {
		t.Fatalf("unsupported template returned %+v, %v", preview, err)
	}
	for _, issue := range []string{
		"duplicate_template_setting", "docker_socket_mount", "unsafe_mount_target", "missing_required_path",
		"invalid_environment_name", "missing_required_environment", "unsupported_template_port",
	} {
		if !slices.Contains(preview.Issues, issue) {
			t.Errorf("preview issues omit %q: %v", issue, preview.Issues)
		}
	}
	if len(preview.Mounts) != 1 || preview.Mounts[0].Target != "/implicit-mode" || preview.Mounts[0].Mode != "rw" {
		t.Fatalf("valid mount should retain the native default mode: %+v", preview.Mounts)
	}
	if !slices.Contains(preview.Environment, communityapps.EnvironmentField{Name: "VALID_OPTION", Set: true}) {
		t.Fatalf("saved dropdown selection was not honored: %+v", preview.Environment)
	}
}

func TestPrepareRejectsInvalidPlanInputsAndRegistryState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any, *communityapps.PrepareRequest)
		want   error
		issue  string
	}{
		{name: "unsafe native name", change: func(_ map[string]any, r *communityapps.PrepareRequest) { r.Name = "another/container" }, want: communityapps.ErrInvalidPlanRequest},
		{name: "syntactically unsupported network", change: func(_ map[string]any, r *communityapps.PrepareRequest) { r.Network = "container:other" }, issue: "unsupported_network"},
		{name: "invalid bind address", change: func(_ map[string]any, r *communityapps.PrepareRequest) {
			r.Ports = map[string]communityapps.PortOverride{"80/tcp": {HostPort: 8080, BindAddress: "255.255.255.255"}}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "undeclared path", change: func(_ map[string]any, r *communityapps.PrepareRequest) {
			r.Paths = map[string]communityapps.PathOverride{"/undeclared": {Source: "/mnt/cache/test"}}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "large override count", change: func(_ map[string]any, r *communityapps.PrepareRequest) {
			r.Environment = make(map[string]string)
			for i := range 257 {
				r.Environment[fmt.Sprintf("CONFIG_%d", i)] = "one"
			}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "total override byte limit", change: func(entry map[string]any, r *communityapps.PrepareRequest) {
			fields := make([]any, 0, 17)
			r.Environment = make(map[string]string)
			for i := range 17 {
				name := fmt.Sprintf("SETTING_%d", i)
				fields = append(fields, publisherField("Variable", name, "", "", "", false))
				r.Environment[name] = strings.Repeat("x", 4096)
			}
			entry["Config"] = fields
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "publisher settings too large", change: func(entry map[string]any, _ *communityapps.PrepareRequest) {
			entry["Config"] = []any{publisherField("Variable", "LARGE", "", strings.Repeat("x", 129<<10), "", false)}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "image reference too large", change: func(entry map[string]any, _ *communityapps.PrepareRequest) {
			entry["Repository"] = strings.Repeat("a", 513)
		}, want: communityapps.ErrImageDigest},
		{name: "invalid image reference", change: func(entry map[string]any, _ *communityapps.PrepareRequest) {
			entry["Repository"] = "https://registry.example.invalid/image"
		}, want: communityapps.ErrImageDigest},
		{name: "registry digest disagrees with configured image pin", change: func(entry map[string]any, _ *communityapps.PrepareRequest) {
			entry["Repository"] = "example/sample@sha256:" + strings.Repeat("3", 64)
		}, want: communityapps.ErrImageDigest},
		{name: "ineligible privileged template with ordinary config", change: func(entry map[string]any, _ *communityapps.PrepareRequest) {
			entry["Privileged"] = true
		}, issue: "privileged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			entry := testCatalogEntry()
			entry["Config"] = []any{publisherField("Port", "80", "tcp", "8080", "", false)}
			request := communityapps.PrepareRequest{}
			tc.change(entry, &request)
			request.Reference = prepareFixtureRequest(t, root, entry).Reference
			service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
			t.Cleanup(service.Close)
			preview, err := service.Prepare(t.Context(), request)
			if tc.want != nil {
				if !errors.Is(err, tc.want) || preview.PlanID != "" {
					t.Fatalf("invalid preparation = %+v, %v; want %v", preview, err, tc.want)
				}
				return
			}
			if err != nil || preview.Eligible || preview.PlanID != "" || !slices.Contains(preview.Issues, tc.issue) {
				t.Fatalf("ineligible preparation = %+v, %v; want issue %q", preview, err, tc.issue)
			}
		})
	}
}

func TestPrepareFailsWhenContainerStateCannotBeRead(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot: root,
		OpenDocker: func() (communityapps.PlanningDocker, error) {
			return &unavailableContainerList{}, nil
		},
	})
	t.Cleanup(service.Close)
	if _, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}); !errors.Is(err, communityapps.ErrDockerState) {
		t.Fatalf("unavailable Docker state returned %v", err)
	}
}

type unavailableContainerList struct {
	communityappstest.Docker
}

func (unavailableContainerList) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, errors.New("engine unavailable")
}

func TestPrepareAfterCloseStopsPendingApproval(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	service.Close()
	if _, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}); !errors.Is(err, communityapps.ErrInstallStopped) {
		t.Fatalf("closed service prepared an approval: %v", err)
	}
}

func TestPendingApprovalExpiresOnTimer(t *testing.T) {
	// Prepare uses an injected clock and an independent timer. Free up plan
	// capacity through the timer without advancing the injected clock.
	root := communityappstest.CopyHeimdallHost(t)
	synctest.Test(t, func(t *testing.T) {
		now := time.Now
		initial := now()
		service := newPlanningService(t, root, &communityappstest.Docker{}, func() time.Time { return initial })
		t.Cleanup(service.Close)
		req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
		preview, err := service.Prepare(t.Context(), req)
		if err != nil || preview.ExpiresAt == nil {
			t.Fatalf("prepare: %+v, %v", preview, err)
		}
		time.Sleep(time.Until(*preview.ExpiresAt))
		synctest.Wait()
		if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrPlanExpired) {
			t.Fatalf("elapsed pending approval remained valid: %v", err)
		}
	})
}

func TestDefaultInstallDockerVerifiesUnixSocket(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	t.Setenv("DOCKER_CERT_PATH", "")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_API_VERSION", "")
	t.Run("invalid engine configuration", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "invalid-engine-address")
		service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{HostRoot: root})
		t.Cleanup(service.Close)
		req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
		if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrDockerState) {
			t.Fatalf("invalid Docker environment returned %v", err)
		}
	})
	t.Run("confirmed worker checks local engine", func(t *testing.T) {
		requests := make(chan string, 2)
		usePlanUnixEngine(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/_ping" {
				w.Header().Set("API-Version", "1.56")
				return
			}
			select {
			case requests <- r.URL.Path:
			default:
			}
			http.Error(w, `{"message":"unavailable"}`, http.StatusServiceUnavailable)
		}))
		docker := &communityappstest.Docker{}
		service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
			HostRoot: root,
			OpenDocker: func() (communityapps.PlanningDocker, error) {
				return docker, nil
			},
			PortAvailable: func(context.Context, communityapps.PortBinding) error { return nil },
		})
		t.Cleanup(service.Close)
		preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
		if err != nil || !preview.Eligible {
			t.Fatalf("prepare: %+v, %v", preview, err)
		}
		if _, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken); err != nil {
			t.Fatal(err)
		}
		select {
		case path := <-requests:
			if !strings.HasPrefix(path, "/v1.56/") {
				t.Fatalf("unexpected local Docker request %q", path)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("worker did not verify its local Docker Engine")
		}
	})
}

func usePlanUnixEngine(t *testing.T, handler http.Handler) {
	t.Helper()
	dir, err := os.MkdirTemp("", "uma-plan-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "docker.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	t.Setenv("DOCKER_HOST", "unix://"+socket)
}

func publisherField(kind, target, mode, defaultValue, value string, required bool) map[string]any {
	return map[string]any{
		"@attributes": map[string]any{"Type": kind, "Target": target, "Mode": mode, "Default": defaultValue, "Required": required},
		"value":       value,
	}
}

func prepareFixtureRequest(t *testing.T, root string, entry map[string]any) communityapps.PrepareRequest {
	t.Helper()
	writeTestCatalog(t, root, entry)
	result, err := communityapps.NewCatalog(os.DirFS(root)).Search(t.Context(), "")
	if err != nil || len(result.Listings) != 1 {
		t.Fatalf("temporary publisher fixture = %+v, %v", result, err)
	}
	return communityapps.PrepareRequest{Reference: result.Listings[0].Reference}
}

func TestPreviewProbesIPv6AndRejectsBusyUDP(t *testing.T) {
	for _, protocol := range []string{"tcp", "udp"} {
		t.Run(protocol, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			entry := testCatalogEntry()
			entry["Config"] = []any{publisherField("Port", "8080", protocol, "", "", false)}
			req := prepareFixtureRequest(t, root, entry)
			var port uint16
			switch protocol {
			case "udp":
				busy, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp6", "[::1]:0")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = busy.Close() })
				port = busy.LocalAddr().(*net.UDPAddr).AddrPort().Port()
			case "tcp":
				free, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp6", "[::1]:0")
				if err != nil {
					t.Fatal(err)
				}
				port = free.Addr().(*net.TCPAddr).AddrPort().Port()
				if err := free.Close(); err != nil {
					t.Fatal(err)
				}
			}
			req.Ports = map[string]communityapps.PortOverride{"8080/" + protocol: {HostPort: port, BindAddress: "::1"}}
			engine := &communityappstest.Docker{Bridge: network.Network{
				Name: "bridge", ID: strings.Repeat("b", 64), Driver: "bridge", Scope: "local", EnableIPv6: true,
			}}
			service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
				HostRoot: root,
				OpenDocker: func() (communityapps.PlanningDocker, error) {
					return engine, nil
				},
			})
			t.Cleanup(service.Close)
			preview, err := service.Prepare(t.Context(), req)
			if protocol == "udp" {
				if !errors.Is(err, communityapps.ErrPortConflict) {
					t.Fatalf("occupied UDP6 port returned %v", err)
				}
			} else if err != nil || !preview.Eligible || preview.Ports[0].BindAddress != "::1" {
				t.Fatalf("free TCP6 port returned %+v, %v", preview, err)
			}
		})
	}
}

func TestPrepareRejectsInvalidDefaultBridgeBinding(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.Docker{Bridge: network.Network{
		Name: "bridge", ID: strings.Repeat("b", 64), Driver: "bridge", Scope: "local", EnableIPv4: true,
		Options: map[string]string{"com.docker.network.bridge.host_binding_ipv4": "invalid"},
	}}
	service := newPlanningService(t, root, docker, nil)
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || preview.Eligible || !slices.Contains(preview.Issues, "unsupported_network") {
		t.Fatalf("invalid bridge bind default returned %+v, %v", preview, err)
	}
}
