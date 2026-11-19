package communityapps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestPrepareUsesNativeAppdataDefaultsAndRedactsEnvironment(t *testing.T) {
	ctx := t.Context()
	root := communityappstest.CopyHeimdallHost(t)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	preview, err := service.Prepare(ctx, communityapps.PrepareRequest{
		Reference:   communityappstest.HeimdallReference,
		Name:        "second-heimdall",
		Environment: map[string]string{"PUID": "secret-preview-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Eligible || preview.PlanID == "" || preview.ApprovalToken == "" || preview.Digest != communityappstest.ApprovedDigest {
		t.Fatalf("expected an approvable image and plan: %+v", preview)
	}
	if len(preview.Mounts) != 1 || preview.Mounts[0].Source != "/mnt/cache/application-data/second-heimdall" || !preview.Mounts[0].CreateDirectory {
		t.Fatalf("expected native appdata directory: %+v", preview.Mounts)
	}
	if len(preview.Ports) != 2 || preview.Ports[0].BindAddress != "0.0.0.0" || preview.Ports[0].HostPort != 80 {
		t.Fatalf("expected effective fixed bindings: %+v", preview.Ports)
	}
	payload, err := json.Marshal(preview)
	if err != nil ||
		!strings.Contains(string(payload), `"name":"PUID","set":true`) ||
		strings.Contains(string(payload), "secret-preview-value") ||
		strings.Contains(string(payload), "fixture-default") {
		t.Fatalf("unexpected redacted preview: %s, %v", payload, err)
	}
	if _, err := os.Stat(filepath.Join(root, preview.Mounts[0].Source)); !os.IsNotExist(err) {
		t.Fatalf("prepare must leave the filesystem unchanged: %v", err)
	}
}

func newPlanningService(t *testing.T, root string, docker *communityappstest.Docker, clock func() time.Time) *communityapps.Service {
	t.Helper()
	return communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:      root,
		OpenDocker:    func() (communityapps.PlanningDocker, error) { return docker, nil },
		PortAvailable: func(context.Context, communityapps.PortBinding) error { return nil },
		Now:           clock,
	})
}

func TestPrepareReportsConflictsAndUnsafeSettings(t *testing.T) {
	const templateDir = "boot/config/plugins/dockerMan/templates-user/"
	for _, tc := range []struct {
		name   string
		change func(*testing.T, string, *communityappstest.Docker, *communityapps.PrepareRequest)
		reason string
		want   error
	}{
		{name: "incomplete onboarding directs operator to Apps", change: func(t *testing.T, root string, _ *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			if err := os.Remove(filepath.Join(root, settingsDirectory, "accepted")); err != nil {
				t.Fatal(err)
			}
		}, want: communityapps.ErrCAOnboarding},
		{name: "stopped Docker names conflict", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			docker.Containers = []container.Summary{{Names: []string{"/HEIMDALL"}}}
		}, want: communityapps.ErrNameConflict},
		{name: "case-insensitive native filename conflicts", change: func(t *testing.T, root string, _ *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, templateDir+"my-HEIMDALL.xml", "<Container><Name>old</Name></Container>")
		}, want: communityapps.ErrNameConflict},
		{name: "retained template name conflicts independently of filename", change: func(t *testing.T, root string, _ *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, templateDir+"archive/old.xml", "<Container><Name>heimdall</Name></Container>")
		}, want: communityapps.ErrNameConflict},
		{name: "existing Docker publication overlaps wildcard", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			docker.Containers = []container.Summary{{Ports: []container.PortSummary{{IP: netip.MustParseAddr("127.0.0.1"), PublicPort: 80, Type: "tcp"}}}}
		}, want: communityapps.ErrPortConflict},
		{name: "duplicate fixed bindings conflict", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Ports = map[string]communityapps.PortOverride{"443/tcp": {HostPort: 80}}
		}, want: communityapps.ErrPortConflict},
		{name: "nonbridge network is unsupported", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Network = "lan"
			docker.Bridge = network.Network{Name: "lan", ID: strings.Repeat("c", 64), Driver: "macvlan", Scope: "local"}
		}, reason: "unsupported_network"},
		{name: "routed bridge is unsupported for published ports", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			docker.Bridge = network.Network{Name: "bridge", ID: strings.Repeat("c", 64), Driver: "bridge", Scope: "local", EnableIPv4: true,
				Options: map[string]string{"com.docker.network.bridge.gateway_mode_ipv4": "routed"}}
		}, reason: "unsupported_network"},
		{name: "undeclared environment key is rejected", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Environment = map[string]string{"UNDECLARED": "secret-dont-reflect"}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "typed fixed host port required", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Ports = map[string]communityapps.PortOverride{"80/tcp": {}}
		}, want: communityapps.ErrInvalidPlanRequest},
		{name: "native entity decoding would change supplied environment", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Environment = map[string]string{"PUID": "secret-dont-reflect&#65;"}
		}, reason: "unsupported_environment_value"},
		{name: "unmounted path is ineligible", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Paths = map[string]communityapps.PathOverride{"/config": {Source: "/mnt/unmounted/config"}}
		}, reason: "unsafe_mount_source"},
		{name: "escaped symlink is ineligible", change: func(t *testing.T, root string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			if err := os.Symlink(root, filepath.Join(root, "mnt/cache/outside")); err != nil {
				t.Fatal(err)
			}
			request.Paths = map[string]communityapps.PathOverride{"/config": {Source: "/mnt/cache/outside/config"}}
		}, reason: "unsafe_mount_source"},
		{name: "effective Docker socket is ineligible", change: func(_ *testing.T, _ string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			request.Paths = map[string]communityapps.PathOverride{"/config": {Source: "/var/run/docker.sock"}}
		}, reason: "docker_socket_mount"},
		{name: "a bind cannot include a foreign child mount", change: func(t *testing.T, root string, _ *communityappstest.Docker, request *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "proc/self/mountinfo", "1 0 8:1 / /mnt/cache rw - btrfs /dev/sdb1 rw\n2 1 0:24 / /mnt/cache/application-data/foreign rw - tmpfs tmpfs rw\n")
			request.Paths = map[string]communityapps.PathOverride{"/config": {Source: "/mnt/cache/application-data"}}
		}, reason: "nested_mount"},
		{name: "unavailable digest fails closed", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			docker.Digest = "sha256:secret-dont-reflect"
		}, want: communityapps.ErrImageDigest},
		{name: "Docker errors are redacted", change: func(_ *testing.T, _ string, docker *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			docker.Err = errors.New("daemon error: secret-dont-reflect")
		}, want: communityapps.ErrDockerState},
		{name: "mount topology must be verifiable", change: func(t *testing.T, root string, _ *communityappstest.Docker, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "proc/self/mountinfo", "malformed")
		}, want: communityapps.ErrStorageState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			docker := &communityappstest.Docker{}
			request := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
			tc.change(t, root, docker, &request)
			preview, err := newPlanningService(t, root, docker, nil).Prepare(t.Context(), request)
			serialized, _ := json.Marshal(communityapps.ProblemForError(err))
			if !errors.Is(err, tc.want) || tc.reason != "" && !slices.Contains(preview.Issues, tc.reason) ||
				preview.Eligible || preview.PlanID != "" || preview.ApprovalToken != "" ||
				strings.Contains(string(serialized), "secret-dont-reflect") {
				t.Fatalf("expected failure %v or issue %q, got preview %+v and %v", tc.want, tc.reason, preview, err)
			}
		})
	}
}

func TestPrepareRequiresExplicitMountSharingAcrossUserAndPoolAliases(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	communityappstest.WriteFile(t, root, "proc/self/mountinfo", "1 0 8:1 / /mnt/cache rw - zfs cache rw\n2 1 0:15 / /mnt/cache/application-data rw - zfs cache/application-data rw\n3 0 0:10 / /mnt/user rw - fuse.shfs shfs rw\n")
	communityappstest.WriteFile(t, root, "boot/config/plugins/dockerMan/templates-user/my-old.xml",
		`<Container><Name>old</Name><Config Type="Path" Mode="rw">/mnt/user/application-data/heimdall</Config></Container>`)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrSharedAppdata) {
		t.Fatalf("unapproved shared path returned %v", err)
	}
	const explicit = "/mnt/cache/application-data/heimdall"
	req.Paths = map[string]communityapps.PathOverride{"/config": {Source: explicit, AllowShared: true}}
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || !preview.Eligible || len(preview.Mounts) != 1 || !preview.Mounts[0].Shared || preview.Mounts[0].Source != explicit {
		t.Fatalf("explicit sharing returned %+v, %v", preview, err)
	}
}

func TestPendingPreviewIsBoundToImmutableSettingsAndExpires(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	now := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)
	docker := &communityappstest.Docker{}
	service := newPlanningService(t, root, docker, func() time.Time { return now })
	req := communityapps.PrepareRequest{
		Reference:   communityappstest.HeimdallReference,
		Environment: map[string]string{"PUID": "99", "UMASK": ""},
	}
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || !preview.Eligible {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if got := preview.ExpiresAt.Sub(now); got != 15*time.Minute {
		t.Fatalf("expiry window = %v, want 15 minutes", got)
	}
	if !slices.Contains(preview.Environment, communityapps.EnvironmentField{Name: "UMASK", Set: false}) {
		t.Fatalf("explicit empty value did not clear a template default: %+v", preview.Environment)
	}
	if err := service.ValidateApproval(t.Context(), preview.PlanID, "true"); !errors.Is(err, communityapps.ErrPlanApproval) {
		t.Fatalf("generic confirmation passed exact approval: %v", err)
	}
	req.Environment["PUID"] = "100"
	preview.Mounts[0].Source = "/tmp/replacement"
	*preview.ExpiresAt = now
	if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); err != nil {
		t.Fatalf("caller mutation altered stored preview: %v", err)
	}
	docker.Digest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrPlanChanged) {
		t.Fatalf("digest drift returned %v", err)
	}
	now = now.Add(15 * time.Minute)
	if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrPlanExpired) {
		t.Fatalf("elapsed plan returned %v", err)
	}
}

func TestPrepareRejectsUnsupportedEffectivePublisherDefaults(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	runtimeCatalog, err := os.ReadFile("testdata/runtime-2026.07.21.serialized.txt")
	if err != nil {
		t.Fatal(err)
	}
	communityappstest.WriteFile(t, root, communityappstest.CachePath, string(runtimeCatalog))
	catalog := communityapps.NewCatalog(os.DirFS(root))
	results, err := catalog.Search(t.Context(), "modern")
	if err != nil || len(results.Listings) != 1 {
		t.Fatalf("fixture search: %+v, %v", results, err)
	}
	preview, err := newPlanningService(t, root, &communityappstest.Docker{}, nil).Prepare(t.Context(), communityapps.PrepareRequest{
		Reference: results.Listings[0].Reference, Name: "modern",
	})
	if err != nil || preview.Eligible || !preview.Privileged || preview.ApprovalToken != "" {
		t.Fatalf("unsafe plan became eligible: %+v, %v", preview, err)
	}
	for _, reason := range []string{"privileged", "device_passthrough", "docker_socket_mount"} {
		if !slices.Contains(preview.Issues, reason) {
			t.Fatalf("missing effective issue %q from %v", reason, preview.Issues)
		}
	}
}

func TestPrepareHonorsBridgeAddressDefaultsAndBoundPendingPlans(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	now := time.Now()
	docker := &communityappstest.Docker{Bridge: network.Network{
		Name: "custom-bridge", ID: strings.Repeat("c", 64), Driver: "bridge", Scope: "local", EnableIPv6: true,
		IPAM:    network.IPAM{Config: []network.IPAMConfig{{Subnet: netip.MustParsePrefix("172.18.0.0/16")}}},
		Options: map[string]string{"com.docker.network.bridge.host_binding_ipv4": "127.0.0.1"},
	}}
	service := newPlanningService(t, root, docker, func() time.Time { return now })
	req := communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference,
		Network:   "custom-bridge",
		Ports: map[string]communityapps.PortOverride{
			"80/tcp":  {HostPort: 18080},
			"443/tcp": {HostPort: 18443, BindAddress: "::1"},
		},
	}
	for range 64 {
		p, err := service.Prepare(t.Context(), req)
		if err != nil || !p.Eligible || !p.Network.IPv4 || !p.Network.IPv6 ||
			p.Ports[0].BindAddress != "127.0.0.1" || p.Ports[1].BindAddress != "::1" {
			t.Fatalf("effective bridge bindings = %+v, %v", p, err)
		}
	}
	if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrPlanCapacity) {
		t.Fatalf("capacity returned %v", err)
	}
	now = now.Add(15 * time.Minute)
	if _, err := service.Prepare(t.Context(), req); err != nil {
		t.Fatalf("expired previews did not release capacity: %v", err)
	}
}

func TestPrepareProbesHostPortsAtReadOnlyBoundary(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	address := listener.Addr().(*net.TCPAddr).AddrPort()
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:   root,
		OpenDocker: func() (communityapps.PlanningDocker, error) { return &communityappstest.Docker{}, nil },
	})
	req := communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference,
		Ports: map[string]communityapps.PortOverride{
			"80/tcp":  {HostPort: address.Port(), BindAddress: "127.0.0.1"},
			"443/tcp": {HostPort: address.Port(), BindAddress: "127.0.0.2"},
		},
	}
	if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrPortConflict) {
		t.Fatalf("occupied host socket returned %v", err)
	}
}

func TestPrepareResolvesNativelyDeclaredIPAndHostPaths(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	rewriteCatalogString(t, root, "80", "127.0.0.7:18080", `s:7:"Default";`)
	rewriteCatalogString(t, root, "/config", "/unraid", "")
	communityappstest.WriteFile(t, root, "boot/config/docker.cfg",
		"DOCKER_APP_UNRAID_PATH=\"/mnt/cache/linked/\"\nDOCKER_PID_LIMIT=\"4096\"\n")
	if err := os.Symlink("application-data", filepath.Join(root, "mnt/cache/linked")); err != nil {
		t.Fatal(err)
	}
	req := communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference,
		Ports:     map[string]communityapps.PortOverride{"80/tcp": {HostPort: 18081}},
	}
	preview, err := newPlanningService(t, root, &communityappstest.Docker{}, nil).Prepare(t.Context(), req)
	if err != nil || !preview.Eligible || preview.PIDsLimit != 4096 || len(preview.Mounts) != 1 ||
		preview.Mounts[0].Source != "/mnt/cache/application-data" || preview.Mounts[0].CreateDirectory ||
		preview.Ports[0].BindAddress != "127.0.0.7" || preview.Ports[0].HostPort != 18081 {
		t.Fatalf("expected reviewed host settings, got %+v, %v", preview, err)
	}
}

func TestPrepareProbesFreeUDPAndKeepsTransportFamiliesSeparate(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	rewriteCatalogString(t, root, "tcp", "udp", `s:4:"Mode";`)
	var listeners []net.PacketConn
	var ports []uint16
	for range 2 {
		listener, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		listeners = append(listeners, listener)
		ports = append(ports, listener.LocalAddr().(*net.UDPAddr).AddrPort().Port())
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}
	docker := &communityappstest.Docker{Containers: []container.Summary{{Ports: []container.PortSummary{
		{IP: netip.IPv6Unspecified(), PublicPort: ports[0], Type: "udp"},
	}}}}
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:   root,
		OpenDocker: func() (communityapps.PlanningDocker, error) { return docker, nil },
	})
	req := communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference,
		Ports: map[string]communityapps.PortOverride{
			"80/udp":  {HostPort: ports[0], BindAddress: "127.0.0.1"},
			"443/udp": {HostPort: ports[1], BindAddress: "127.0.0.1"},
		},
	}
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || !preview.Eligible || preview.Ports[0].Protocol != "udp" {
		t.Fatalf("free UDP IPv4 despite existing IPv6 bind returned %+v, %v", preview, err)
	}
}

func TestApprovalTracksSelectedTemplateRevision(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("initial plan: %+v, %v", preview, err)
	}
	rewriteCatalogString(t, root, "fixture-default", "changed-private-default", "")
	if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrPlanChanged) {
		t.Fatalf("material default changed approval: %v", err)
	}
	if err := os.Remove(filepath.Join(root, settingsDirectory, "accepted")); err != nil {
		t.Fatal(err)
	}
	if err := service.ValidateApproval(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, communityapps.ErrCAOnboarding) {
		t.Fatalf("approval ignored updated first-run policy: %v", err)
	}
}

func rewriteCatalogString(t *testing.T, root, from, to, key string) {
	t.Helper()
	filename := filepath.Join(root, communityappstest.CachePath)
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	source := key + fmt.Sprintf("s:%d:\"%s\";", len(from), from)
	replacement := key + fmt.Sprintf("s:%d:\"%s\";", len(to), to)
	if !bytes.Contains(data, []byte(source)) {
		t.Fatalf("fixture string %q not found", source)
	}
	writeFixtureFile(t, root, cacheFile, bytes.ReplaceAll(data, []byte(source), []byte(replacement)))
}

func TestPrepareRejectsUnrepresentablePortsAndHostState(t *testing.T) {
	for _, tc := range []struct {
		name, port, cfg, issue string
		want                   error
	}{
		{name: "malformed IPv6 binding", port: "[::1", issue: "unsupported_port_binding"},
		{name: "unassigned host port", port: "0", issue: "fixed_port_required"},
		{name: "non-IP binding", port: "hostname:18080", issue: "unsupported_port_binding"},
		{name: "invalid PID limit", cfg: "DOCKER_PID_LIMIT=none\n", want: communityapps.ErrStorageState},
		{name: "malformed host settings", cfg: "[", want: communityapps.ErrStorageState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			if tc.port != "" {
				rewriteCatalogString(t, root, "80", tc.port, `s:7:"Default";`)
			}
			if tc.cfg != "" {
				communityappstest.WriteFile(t, root, "boot/config/docker.cfg", tc.cfg)
			}
			result, err := newPlanningService(t, root, &communityappstest.Docker{}, nil).Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if !errors.Is(err, tc.want) || tc.issue != "" && !slices.Contains(result.Issues, tc.issue) || result.Eligible {
				t.Fatalf("unexpected settings result %+v, %v", result, err)
			}
		})
	}
}

func TestPrepareRebasesPublisherAppdataAcrossRealMountedStorage(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	rewriteCatalogString(t, root, "/config", "/data", "")
	rewriteCatalogString(t, root, "", "/mnt/user/appdata/heimdall", `s:7:"Default";`)
	communityappstest.WriteFile(t, root, "boot/config/docker.cfg",
		"DOCKER_APP_CONFIG_PATH=\"/mnt/disks/external-drive/appdata/\"\n")
	communityappstest.WriteFile(t, root, "proc/self/mountinfo", "1 0 8:1 / /mnt/disks/external-drive rw - xfs /dev/sdb1 rw\n")
	communityappstest.WriteFile(t, root, "mnt/disks/external-drive/appdata/existing-data", "keep")
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || !preview.Eligible || preview.Mounts[0].Source != "/mnt/disks/external-drive/appdata/heimdall" {
		t.Fatalf("expected mounted default rewrite, got %+v %v", preview, err)
	}
	docker := &communityappstest.Docker{Containers: []container.Summary{{Mounts: []container.MountPoint{
		{Type: mount.TypeBind, RW: true, Source: "/mnt/disks/external-drive/appdata"},
	}}}}
	req.Paths = map[string]communityapps.PathOverride{"/data": {Source: "/mnt/disks/external-drive/appdata/existing-data", AllowShared: true}}
	preview, err = newPlanningService(t, root, docker, nil).Prepare(t.Context(), req)
	if !errors.Is(err, communityapps.ErrExposedMountParent) || preview.Eligible {
		t.Fatalf("expected the writable parent bind to block approval, got %+v %v", preview, err)
	}
}

func TestPrepareChecksLegacySavedTemplateAndUnavailableTemplateState(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	const retained = "boot/config/plugins/dockerMan/templates-user/old.xml"
	communityappstest.WriteFile(t, root, retained,
		"<Container><Name>old</Name><Data><Volume><HostDir>/mnt/cache/application-data</HostDir><Mode>rw</Mode></Volume></Data></Container>")
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrExposedMountParent) {
		t.Fatalf("overlapping retained legacy template returned %v", err)
	}
	communityappstest.WriteFile(t, root, retained, "<Malformed")
	if _, err := service.Prepare(t.Context(), req); !errors.Is(err, communityapps.ErrStorageState) {
		t.Fatalf("unverifiable retained template returned %v", err)
	}
}

func TestPrepareResolvesDropdownDefaultsAndBlocksUnsupportedMountModes(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	rewriteCatalogString(t, root, "fixture-default", "99|100", `s:7:"Default";`)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || !preview.Eligible {
		t.Fatalf("native dropdown was ineligible: %+v, %v", preview, err)
	}
	rewriteCatalogString(t, root, "rw", "rw,shared", `s:4:"Mode";`)
	preview, err = service.Prepare(t.Context(), req)
	if err != nil || preview.Eligible || !slices.Contains(preview.Issues, "unsupported_mount_mode") {
		t.Fatalf("unsupported propagation received %+v, %v", preview, err)
	}
}
