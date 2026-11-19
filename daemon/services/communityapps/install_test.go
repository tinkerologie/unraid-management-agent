package communityapps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	digest "github.com/opencontainers/go-digest"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestConfirmRequiresExactPlanApproval(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %+v, %v", preview, err)
	}
	if _, err := service.Confirm(t.Context(), preview.PlanID, "true"); !errors.Is(err, communityapps.ErrPlanApproval) {
		t.Fatalf("generic approval returned %v", err)
	}
	if _, err := service.Confirm(t.Context(), "unknown", preview.ApprovalToken); !errors.Is(err, communityapps.ErrPlanExpired) {
		t.Fatalf("unknown plan returned %v", err)
	}
}

// newInstallService creates the shared application service against a fake
// Engine while exercising the host catalog, mounts and durable XML writes.
func newInstallService(t testing.TB, root string, docker *communityappstest.InstallEngine) *communityapps.Service {
	t.Helper()
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (communityapps.PlanningDocker, error) { return docker, nil },
		OpenInstallDocker: func() (communityapps.InstallDocker, error) { return docker, nil },
		PortAvailable:     func(context.Context, communityapps.PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	return service
}

// waitForAttempt returns an install result when the worker has settled.
func waitForAttempt(t testing.TB, service *communityapps.Service, id string) communityapps.Attempt {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		result, err := service.GetAttempt(id)
		if err != nil {
			t.Fatal(err)
		}
		if result.State == communityapps.AttemptSucceeded || result.State == communityapps.AttemptFailed {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("install did not settle: %+v", result)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestInstallReceiptSurvivesDisconnectAndStartsExactNativeContainer(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	pullStarted := make(chan struct{})
	unblock := make(chan struct{})
	docker := &communityappstest.InstallEngine{BeforePull: func(ctx context.Context) error {
		close(pullStarted)
		select {
		case <-unblock:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	service := newInstallService(t, root, docker)
	request := communityapps.PrepareRequest{
		Reference:   communityappstest.HeimdallReference,
		Name:        "exact-install",
		Environment: map[string]string{"PUID": "native-private-'&>$", "ALLOW_INTERNAL_REQUESTS": ""},
		Ports:       map[string]communityapps.PortOverride{"80/tcp": {HostPort: 18080, BindAddress: "127.0.0.1"}},
	}
	preview, err := service.Prepare(t.Context(), request)
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	first, err := service.Confirm(ctx, preview.PlanID, preview.ApprovalToken)
	if err != nil || first.State != communityapps.AttemptPending || first.ID == "" {
		t.Fatalf("confirm returned %+v, %v", first, err)
	}
	disconnect()
	select {
	case <-pullStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("receipt was accepted but worker did not start")
	}

	// Read using a fresh service while the original request is disconnected.
	reconnected := newInstallService(t, root, docker)
	current, err := reconnected.GetAttempt(first.ID)
	if err != nil || current.State != communityapps.AttemptRunning || current.Phase != "pulling_image" {
		t.Fatalf("durable status was %+v, %v", current, err)
	}
	for range 4 {
		again, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
		if again.ID != first.ID || err != nil {
			t.Fatalf("replayed confirmation was %+v, %v", again, err)
		}
	}
	other := strings.Repeat("A", 26)
	if _, err := service.Confirm(t.Context(), preview.PlanID, other); !errors.Is(err, communityapps.ErrPlanApproval) {
		t.Fatalf("wrong replay token returned %v", err)
	}

	close(unblock)
	attempt := waitForAttempt(t, service, first.ID)
	if attempt.State != communityapps.AttemptSucceeded || attempt.ContainerID != communityappstest.ContainerID ||
		attempt.Configuration.Name != request.Name || attempt.Configuration.Listing.Reference != request.Reference {
		t.Fatalf("install failed: %+v", attempt)
	}
	restarted := newInstallService(t, root, docker)
	again, err := restarted.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil || !reflect.DeepEqual(again, attempt) {
		t.Fatalf("confirmation after restart returned %+v, %v", again, err)
	}
	created, present, started, opts, canonical := docker.Observation()
	if created != 1 || present != 1 || !slices.Equal(started, []string{communityappstest.ContainerID}) ||
		opts.Config.Image != "lscr.io/linuxserver/heimdall" || canonical != opts.Config.Image+"@"+preview.Digest.String() ||
		!slices.Contains(opts.Config.Env, "PUID="+request.Environment["PUID"]) || !slices.Contains(opts.Config.Env, "ALLOW_INTERNAL_REQUESTS=") ||
		opts.HostConfig.PidsLimit == nil || *opts.HostConfig.PidsLimit != 2048 ||
		opts.NetworkingConfig.EndpointsConfig["bridge"].NetworkID != preview.Network.ID ||
		len(opts.HostConfig.Mounts) != 1 || opts.HostConfig.Mounts[0].Source != preview.Mounts[0].Source {
		t.Fatal("worker did not create and start the exact reviewed container")
	}
	receiptPath := filepath.Join(root, "boot/config/plugins/unraid-management-agent/community-app-attempts", preview.PlanID+".json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil || bytes.Contains(receipt, []byte("native-private")) || bytes.Contains(receipt, []byte(preview.ApprovalToken)) {
		t.Fatalf("receipt was unavailable or exposed a sensitive value: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, preview.Mounts[0].Source)); err != nil {
		t.Fatalf("approved appdata was not created: %v", err)
	}
	native := readNativeInstall(t, root, request.Name)
	if native.Name != request.Name || native.Repository != opts.Config.Image || native.Network != "bridge" ||
		native.Privileged || native.Version != "2" || native.TemplateURL != "https://raw.githubusercontent.com/linuxserver/templates/main/unraid/heimdall.xml" {
		t.Fatalf("invalid native template identity: %s, %s, %s", native.Name, native.Repository, native.TemplateURL)
	}
	for target, value := range map[string]string{
		"80": "127.0.0.1:18080", "/config": preview.Mounts[0].Source,
		"PUID": request.Environment["PUID"], "ALLOW_INTERNAL_REQUESTS": "",
	} {
		if !slices.ContainsFunc(native.Fields, func(field nativeInstallField) bool {
			return field.Target == target && field.Value == value && field.Default == ""
		}) {
			t.Fatalf("native setting %s differs from approved configuration", target)
		}
	}
}

type nativeInstallField struct {
	Target  string `xml:"Target,attr"`
	Default string `xml:"Default,attr"`
	Value   string `xml:",chardata"`
}

type nativeInstall struct {
	Version     string `xml:"version,attr"`
	Name        string
	Repository  string
	Network     string
	Privileged  bool
	TemplateURL string
	Fields      []nativeInstallField `xml:"Config"`
}

func readNativeInstall(t *testing.T, root, name string) nativeInstall {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "boot/config/plugins/dockerMan/templates-user", "my-"+name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	var result nativeInstall
	if err := xml.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstallFailurePreservesAppdataAndOnlyCleansOwnedResources(t *testing.T) {
	const private = "private-install-failure-sentinel"
	const templateDir = "boot/config/plugins/dockerMan/templates-user"
	const runtime = "runtime-attempt"
	pathForTemplate := func(root string) string { return filepath.Join(root, templateDir, "my-"+runtime+".xml") }
	failure := errors.New(private)
	for _, tc := range []struct {
		name                   string
		configure              func(*testing.T, string, *communityappstest.InstallEngine)
		wantProblem            string
		wantCleanup            []string
		wantCreate             int
		wantRemaining          int
		wantStart              int
		wantSentinel           bool
		wantBlockedTemplateDir bool
	}{
		{
			name: "digest changes before pull",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				var resolutions int
				d.BeforeDistribution = func() {
					resolutions++
					if resolutions > 1 {
						d.Digest = digest.Digest("sha256:" + strings.Repeat("e", 64))
					}
				}
			},
			wantProblem: "image_digest_changed",
		},
		{
			name: "registry digest becomes unavailable",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				var resolutions int
				d.BeforeDistribution = func() {
					resolutions++
					if resolutions > 1 {
						d.Digest = "sha256:invalid"
					}
				}
			},
			wantProblem: "image_digest_unavailable",
		},
		{
			name: "registry pull fails before returning a stream",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforePull = func(context.Context) error { return failure }
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "host changes native defaults during pull",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.BeforePull = func(context.Context) error {
					return os.WriteFile(filepath.Join(root, "boot/config/docker.cfg"), []byte("DOCKER_PID_LIMIT=3072\n"), 0o600)
				}
			},
			wantProblem: "plan_changed",
		},
		{
			name: "mounted path changes during final approval check",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				var lists int
				d.BeforeList = func() error {
					lists++
					if lists == 4 {
						return os.WriteFile(filepath.Join(root, "proc/self/mountinfo"), []byte("23 0 8:4 / /mnt/cache rw - btrfs /dev/sdc1 rw\n"), 0o600)
					}
					return nil
				}
			},
			wantProblem: "plan_changed",
		},
		{
			name: "storage topology becomes unavailable during final approval check",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				var lists int
				d.BeforeList = func() error {
					lists++
					if lists == 4 {
						return os.Remove(filepath.Join(root, "proc/self/mountinfo"))
					}
					return nil
				}
			},
			wantProblem: "storage_unavailable",
		},
		{
			name: "digest drift during pull despite matching image ID",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.PullMessages = []jsonstream.Message{{Status: "Digest: sha256:" + strings.Repeat("a", 64)}}
			},
			wantProblem: "image_digest_changed",
		},
		{
			name: "no digest progress",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.PullMessages = []jsonstream.Message{{ID: "layer", Status: "Download complete"}}
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "failure after pull digest",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.PullStreamErr = failure
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "engine pull response error after digest",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.PullMessages = []jsonstream.Message{
					{Status: "Digest: " + communityappstest.ApprovedDigest.String()},
					{Error: &jsonstream.Error{Message: private}},
				}
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "expected canonical image unavailable",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.InspectError = failure
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "canonical digest proof missing",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.NoDigestProof = true
			},
			wantProblem: "image_pull_failed",
		},
		{
			name: "tag changes between canonical inspect and stopped create",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.CreateImageID = "sha256:" + strings.Repeat("e", 64)
			},
			wantProblem: "image_digest_changed", wantCreate: 1,
		},
		{
			name: "Docker name allocated by another caller",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeCreate = func(client.ContainerCreateOptions) error {
					return errdefs.ErrConflict.WithMessage("The container name is already in use: " + private)
				}
			},
			wantProblem: "name_conflict",
		},
		{
			name: "Docker rejected create before allocating a container",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeCreate = func(client.ContainerCreateOptions) error {
					return errdefs.ErrInvalidArgument.WithMessage("invalid Docker options: " + private)
				}
			},
			wantProblem: "container_create_failed",
		},
		{
			name: "Docker create returned failure after persisting container",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error { return failure }
			},
			wantProblem: "container_create_failed", wantCreate: 1,
		},
		{
			name: "Docker omitted container ID after creating resource",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.DropCreateID = true
			},
			wantProblem: "container_create_failed", wantCreate: 1,
		},
		{
			name: "replacement container copying owner label survives lost create response",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeCreate = func(opts client.ContainerCreateOptions) error {
					opts.Config.Labels[constants.DockerUpdateAttemptLabel] = "fixture replacement"
					return nil
				}
				d.AfterCreate = func() error { return failure }
			},
			wantProblem: "container_create_failed", wantCreate: 1, wantRemaining: 1, wantCleanup: []string{"container_identity_unverified"},
		},
		{
			name: "native template staging directory is unavailable",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					dir := filepath.Join(root, templateDir)
					if err := os.Rename(dir, dir+"-keep"); err != nil {
						return err
					}
					return os.WriteFile(dir, []byte("keep unrelated file"), 0o600)
				}
			},
			wantProblem: "native_template_failed", wantCreate: 1,
			wantCleanup: []string{"remove_template", "remove_staged_template"}, wantBlockedTemplateDir: true,
		},
		{
			name: "existing native template created after Docker create",
			configure: func(t *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					return os.WriteFile(pathForTemplate(root), []byte("<Container><Name>native-sentinel</Name></Container>"), 0o600)
				}
			},
			wantProblem: "name_conflict", wantCreate: 1, wantSentinel: true,
		},
		{
			name: "staged XML cannot publish over an existing destination",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					return os.Mkdir(pathForTemplate(root), 0o700)
				}
			},
			wantProblem: "name_conflict", wantCreate: 1, wantSentinel: true,
		},
		{
			name: "bridge is replaced after container creation",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					d.Bridge = network.Network{Name: "bridge", ID: strings.Repeat("e", 64), Scope: "local", Driver: "bridge", EnableIPv4: true}
					return nil
				}
			},
			wantProblem: "plan_changed", wantCreate: 1,
		},
		{
			name: "bridge cannot be verified before start",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					d.BeforeNetwork = func() error { return failure }
					return nil
				}
			},
			wantProblem: "docker_unavailable", wantCreate: 1,
		},
		{
			name: "storage is unavailable immediately before start",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					return os.Remove(filepath.Join(root, "proc/self/mountinfo"))
				}
			},
			wantProblem: "storage_unavailable", wantCreate: 1,
		},
		{
			name: "host PID limit changes immediately before start",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					return os.WriteFile(filepath.Join(root, "boot/config/docker.cfg"), []byte("DOCKER_PID_LIMIT=3072\n"), 0o600)
				}
			},
			wantProblem: "plan_changed", wantCreate: 1,
		},
		{
			name: "approved appdata bind is remounted before start",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					return os.WriteFile(filepath.Join(root, "proc/self/mountinfo"), []byte("23 0 8:4 / /mnt/cache rw - btrfs /dev/sdc1 rw\n"), 0o600)
				}
			},
			wantProblem: "plan_changed", wantCreate: 1,
		},
		{
			name: "other port reservations cannot be checked before start",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					d.BeforeList = func() error { return failure }
					return nil
				}
			},
			wantProblem: "docker_unavailable", wantCreate: 1,
		},
		{
			name: "other container obtains port before start",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.AfterCreate = func() error {
					d.Containers = []container.Summary{{Names: []string{"/unrelated"}, Ports: []container.PortSummary{{PublicPort: 80, Type: "tcp"}}}}
					return nil
				}
			},
			wantProblem: "port_conflict", wantCreate: 1,
		},
		{
			name: "Docker port allocation races with start",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeStart = func() error {
					return errdefs.ErrInvalidArgument.WithMessage("bind: address already in use: " + private)
				}
			},
			wantProblem: "port_conflict", wantCreate: 1, wantStart: 1,
		},
		{
			name: "Docker start failed and native XML was replaced",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.BeforeStart = func() error {
					if err := os.WriteFile(pathForTemplate(root), []byte("<Container><Name>edited</Name></Container>"), 0o600); err != nil {
						return err
					}
					return errdefs.ErrInvalidArgument.WithMessage(failure.Error())
				}
			},
			wantProblem: "container_start_failed", wantCreate: 1, wantStart: 1, wantSentinel: true,
		},
		{
			name: "Docker start failed after native XML edit that keeps ownership marker",
			configure: func(_ *testing.T, root string, d *communityappstest.InstallEngine) {
				d.BeforeStart = func() error {
					filename := pathForTemplate(root)
					if data, err := os.ReadFile(filename); err == nil {
						if err := os.WriteFile(filename, bytes.Replace(data, []byte("<Shell>sh</Shell>"), []byte("<Shell>bash</Shell>"), 1), 0o600); err != nil {
							return err
						}
					}
					return errdefs.ErrInvalidArgument.WithMessage(failure.Error())
				}
			},
			wantProblem: "container_start_failed", wantCreate: 1, wantStart: 1, wantSentinel: true, wantCleanup: []string{"remove_template"},
		},
		{
			name: "Docker removal returned an error after removing the container",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeStart = func() error { return errdefs.ErrInvalidArgument.WithMessage(failure.Error()) }
				d.AfterRemove = func() error { return failure }
			},
			wantProblem: "container_start_failed", wantCreate: 1, wantStart: 1,
		},
		{
			name: "Docker start and cleanup both failed",
			configure: func(_ *testing.T, _ string, d *communityappstest.InstallEngine) {
				d.BeforeStart = func() error { return errdefs.ErrInvalidArgument.WithMessage(failure.Error()) }
				d.BeforeRemove = func() error { return failure }
			},
			wantProblem: "container_start_failed", wantCreate: 1, wantStart: 1, wantRemaining: 1, wantCleanup: []string{"remove_container"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			sentinel := "mnt/cache/application-data/" + runtime + "/keep.txt"
			communityappstest.WriteFile(t, root, sentinel, "keep appdata")
			communityappstest.WriteFile(t, root, templateDir+"/unrelated.xml", "<Container><Name>unrelated</Name></Container>")
			docker := &communityappstest.InstallEngine{}
			tc.configure(t, root, docker)
			service := newInstallService(t, root, docker)
			preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
				Reference: communityappstest.HeimdallReference, Name: runtime, Environment: map[string]string{"PUID": private},
			})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare: %+v, %v", preview, err)
			}
			claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			attempt := waitForAttempt(t, service, claim.ID)
			if attempt.State != communityapps.AttemptFailed || attempt.Problem == nil || attempt.Problem.Code != tc.wantProblem ||
				!slices.Equal(attempt.CleanupFailures, tc.wantCleanup) {
				t.Fatalf("unexpected attempt: %+v", attempt)
			}
			if data, err := json.Marshal(attempt); err != nil || bytes.Contains(data, []byte(private)) {
				t.Fatalf("attempt metadata disclosed environment or daemon diagnostics: %v", err)
			}
			created, present, started, _, _ := docker.Observation()
			if created != tc.wantCreate || present != tc.wantRemaining || len(started) != tc.wantStart {
				t.Fatalf("unexpected Docker state: created %d, present %d, started %d", created, present, len(started))
			}
			if tc.wantBlockedTemplateDir {
				dir := filepath.Join(root, templateDir)
				if content, err := os.ReadFile(dir); err != nil || string(content) != "keep unrelated file" {
					t.Fatalf("blocked native directory sentinel was lost: %v", err)
				}
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(dir+"-keep", dir); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(pathForTemplate(root)); tc.wantSentinel && err != nil || !tc.wantSentinel && !os.IsNotExist(err) {
				t.Fatalf("unexpected resulting template: %v", err)
			}
			if paths, err := filepath.Glob(filepath.Join(root, templateDir, ".uma-ca-*")); err != nil || len(paths) != 0 {
				t.Fatalf("owned staging file remains: %v, %v", paths, err)
			}
			if data, err := os.ReadFile(filepath.Join(root, sentinel)); err != nil || string(data) != "keep appdata" {
				t.Fatalf("appdata was lost: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, templateDir, "unrelated.xml")); err != nil {
				t.Fatalf("unrelated template was lost: %v", err)
			}
			retry, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil || !reflect.DeepEqual(retry, attempt) {
				t.Fatalf("failure was not idempotent: %+v, %v", retry, err)
			}
		})
	}
}

func TestConcurrentConfirmsClaimOnceAndOneListingSupportsTwoInstalls(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.InstallEngine{AfterStart: func() error {
		return errors.New("lost Docker start response")
	}}
	service := newInstallService(t, root, docker)
	var prior string
	for idx, name := range []string{"first-instance", "second-instance"} {
		preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
			Reference: communityappstest.HeimdallReference,
			Name:      name,
			Ports: map[string]communityapps.PortOverride{
				"80/tcp":  {HostPort: uint16(18080 + idx)},
				"443/tcp": {HostPort: uint16(18443 + idx)},
			},
		})
		if err != nil || !preview.Eligible {
			t.Fatalf("prepare: %+v, %v", preview, err)
		}
		type confirmation struct {
			attempt communityapps.Attempt
			err     error
		}
		confirmations := make(chan confirmation, 8)
		for range cap(confirmations) {
			go func() {
				attempt, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
				confirmations <- confirmation{attempt, err}
			}()
		}
		first := <-confirmations
		if first.err != nil || first.attempt.ID == prior || first.attempt.ID == "" {
			t.Fatalf("unexpected claim: %+v", first)
		}
		for range cap(confirmations) - 1 {
			next := <-confirmations
			if next.attempt.ID != first.attempt.ID || next.err != nil {
				t.Fatalf("different concurrent claim: %+v", next)
			}
		}
		result := waitForAttempt(t, service, first.attempt.ID)
		if result.State != communityapps.AttemptSucceeded || readNativeInstall(t, root, name).Name != name {
			t.Fatalf("install failed: %+v", result)
		}
		prior = result.ID
	}
	created, present, started, _, _ := docker.Observation()
	if created != 2 || present != 2 || len(started) != 2 {
		t.Fatalf("expected two distinct successful installs, got %d created, %d present, %d started", created, present, len(started))
	}
}

func TestConfirmRejectsStalePlansAndUnavailableDurableStorage(t *testing.T) {
	const statusID = "AAAAAAAAAAAAAAAAAAAAAAAAAA"
	const attemptDir = "boot/config/plugins/unraid-management-agent/community-app-attempts"
	for _, tt := range []struct {
		name     string
		setup    func(*testing.T, string, *communityapps.Service, communityapps.Preview)
		expected *communityapps.Problem
	}{
		{name: "onboarding withdrawn", expected: communityapps.ErrCAOnboarding, setup: func(t *testing.T, root string, _ *communityapps.Service, _ communityapps.Preview) {
			if err := os.Remove(filepath.Join(root, settingsDirectory, "accepted")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "material host setting changed", expected: communityapps.ErrPlanChanged, setup: func(t *testing.T, root string, _ *communityapps.Service, _ communityapps.Preview) {
			communityappstest.WriteFile(t, root, "boot/config/docker.cfg", "DOCKER_PID_LIMIT=3096\n")
		}},
		{name: "lifecycle closed", expected: communityapps.ErrInstallStopped, setup: func(_ *testing.T, _ string, service *communityapps.Service, _ communityapps.Preview) {
			service.Close()
		}},
		{name: "durable store is not a directory", expected: communityapps.ErrAttemptStorage, setup: func(t *testing.T, root string, service *communityapps.Service, _ communityapps.Preview) {
			if _, err := service.GetAttempt(statusID); !errors.Is(err, communityapps.ErrAttemptNotFound) {
				t.Fatalf("unexpected status error: %v", err)
			}
			communityappstest.WriteFile(t, root, attemptDir, "store blocked")
		}},
		{name: "durable store scan fails", expected: communityapps.ErrAttemptStorage, setup: func(t *testing.T, root string, _ *communityapps.Service, _ communityapps.Preview) {
			communityappstest.WriteFile(t, root, attemptDir, "store blocked")
		}},
		{name: "existing damaged receipt fails closed", expected: communityapps.ErrAttemptStorage, setup: func(t *testing.T, root string, service *communityapps.Service, preview communityapps.Preview) {
			communityappstest.WriteFile(t, root, attemptDir+"/.receipt-orphan", "interrupted temporary write")
			communityappstest.WriteFile(t, root, attemptDir+"/"+preview.PlanID+".json", "{")
			if _, err := service.GetAttempt(statusID); !errors.Is(err, communityapps.ErrAttemptStorage) {
				t.Fatalf("unexpected damaged status error: %v", err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			docker := &communityappstest.InstallEngine{}
			service := newInstallService(t, root, docker)
			preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare returned %+v, %v", preview, err)
			}
			tt.setup(t, root, service, preview)
			if _, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, tt.expected) {
				t.Fatalf("confirmation returned %v, expected %v", err, tt.expected)
			}
			if created, _, _, _, _ := docker.Observation(); created != 0 {
				t.Fatal("failed confirmation mutated Docker")
			}
		})
	}
}

func TestInstallLeavesUnverifiableStartForReconciliation(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.InstallEngine{
		HasDescriptor:         true,
		PostStartInspectError: errors.New("temporarily unavailable"),
	}
	service := newInstallService(t, root, docker)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		_, count, starts, _, _ := docker.Observation()
		if count == 1 && len(starts) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Docker start was never issued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	service.Close()
	state, err := service.GetAttempt(claim.ID)
	if err != nil || state.State != communityapps.AttemptRunning || state.Phase != "starting_container" {
		t.Fatalf("ambiguous start was treated as terminal: %+v, %v", state, err)
	}
	if _, present, _, _, _ := docker.Observation(); present != 1 || readNativeInstall(t, root, "heimdall").Name != "heimdall" {
		t.Fatal("a temporary inspection error deleted installed resources")
	}
}

func TestInstallWorkerDoesNotApplyPlansAfterServiceShutdown(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	docker := &communityappstest.InstallEngine{BeforePull: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	service := newInstallService(t, root, docker)
	var accepted []string
	for i := range 17 {
		preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
			Reference: communityappstest.HeimdallReference, Name: fmt.Sprintf("queued-%d", i),
		})
		if err != nil || !preview.Eligible {
			t.Fatalf("prepare returned %+v, %v", preview, err)
		}
		claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
		if i == 16 {
			if !errors.Is(err, communityapps.ErrInstallCapacity) {
				t.Fatalf("expected capacity reached, got %v", err)
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, claim.ID)
	}
	service.Close()
	for _, id := range accepted {
		attempt, err := service.GetAttempt(id)
		if err != nil || attempt.State != communityapps.AttemptFailed {
			t.Fatalf("shutdown attempt returned %+v, %v", attempt, err)
		}
	}
	if created, _, _, _, _ := docker.Observation(); created != 0 {
		t.Fatal("shutdown allowed a queued worker to mutate Docker")
	}
}

func TestInstallFailsSafelyWhenWorkerCannotOpenEngine(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (communityapps.PlanningDocker, error) { return &communityappstest.Docker{}, nil },
		OpenInstallDocker: func() (communityapps.InstallDocker, error) { return nil, errors.New("private engine configuration") },
		PortAvailable:     func(context.Context, communityapps.PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	attempt := waitForAttempt(t, service, claim.ID)
	if attempt.State != communityapps.AttemptFailed || attempt.Problem == nil || attempt.Problem.Code != "docker_unavailable" {
		t.Fatalf("unavailable worker returned %+v", attempt)
	}
}

func TestInstallStopsBeforeCreatingContainerWhenReceiptCannotAdvance(t *testing.T) {
	const attemptDirectory = "boot/config/plugins/unraid-management-agent/community-app-attempts"
	root := communityappstest.CopyHeimdallHost(t)
	path := filepath.Join(root, attemptDirectory)
	hold := path + "-test-hold"
	changed := make(chan error, 1)
	docker := &communityappstest.InstallEngine{BeforePull: func(context.Context) error {
		if err := os.Rename(path, hold); err != nil {
			changed <- err
			return err
		}
		err := os.WriteFile(path, []byte("simulate unavailable attempt directory"), 0o600)
		changed <- err
		return err
	}}
	service := newInstallService(t, root, docker)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker never began an image pull")
	}
	service.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(hold, path); err != nil {
		t.Fatal(err)
	}
	attempt, err := service.GetAttempt(claim.ID)
	if err != nil || attempt.State != communityapps.AttemptRunning || attempt.Phase != "pulling_image" {
		t.Fatalf("last durable receipt was lost: %+v, %v", attempt, err)
	}
	if created, present, _, _, _ := docker.Observation(); created != 0 || present != 0 {
		t.Fatal("Docker container created before progress was durable")
	}
}

func TestConfirmRechecksExpiryAndCancellationAtClaimBoundary(t *testing.T) {
	for _, mode := range []string{"expires", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			now := time.Now()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			docker := &communityappstest.InstallEngine{}
			var confirming bool
			service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
				HostRoot:          root,
				Now:               func() time.Time { return now },
				OpenDocker:        func() (communityapps.PlanningDocker, error) { return docker, nil },
				OpenInstallDocker: func() (communityapps.InstallDocker, error) { return docker, nil },
				PortAvailable: func(context.Context, communityapps.PortBinding) error {
					if confirming {
						if mode == "expires" {
							now = now.Add(16 * time.Minute)
						} else {
							cancel()
						}
					}
					return nil
				},
			})
			t.Cleanup(service.Close)
			preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare returned %+v, %v", preview, err)
			}
			confirming = true
			want := communityapps.ErrPlanExpired
			if mode == "canceled" {
				want = communityapps.ErrInstallStopped
			}
			if _, err := service.Confirm(ctx, preview.PlanID, preview.ApprovalToken); !errors.Is(err, want) {
				t.Fatalf("claim boundary returned %v, expected %v", err, want)
			}
		})
	}
}

func TestInstallFailsSafelyWhenPortBecomesBusyWhilePublishingTemplate(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	blocked := false
	docker := &communityappstest.InstallEngine{
		AfterCreate: func() error {
			blocked = true
			return nil
		},
	}
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (communityapps.PlanningDocker, error) { return docker, nil },
		OpenInstallDocker: func() (communityapps.InstallDocker, error) { return docker, nil },
		PortAvailable: func(context.Context, communityapps.PortBinding) error {
			if blocked {
				return errors.New("socket is busy")
			}
			return nil
		},
	})
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	attempt := waitForAttempt(t, service, claim.ID)
	if attempt.State != communityapps.AttemptFailed || attempt.Problem == nil || attempt.Problem.Code != "port_conflict" {
		t.Fatalf("unexpected attempt: %+v", attempt)
	}
	if _, present, starts, _, _ := docker.Observation(); present != 0 || len(starts) != 0 {
		t.Fatal("Docker used a port that became busy")
	}
}

func TestInstallPreservesPinnedRepositoryAndSafeNativeFieldNames(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	catalogPath := filepath.Join(root, communityappstest.CachePath)
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	image := "lscr.io/linuxserver/heimdall@" + communityappstest.ApprovedDigest.String()
	fixture := strings.ReplaceAll(string(data), `s:10:"Repository";s:28:"lscr.io/linuxserver/heimdall";`,
		fmt.Sprintf("s:10:\"Repository\";s:%d:%q;", len(image), image))
	// The native WebGUI puts a field's Name into markup. A name that
	// requires quote escaping is safely replaced with its declared target.
	fixture = strings.ReplaceAll(fixture, `s:4:"Name";s:4:"PUID";`, `s:4:"Name";s:9:"User "ID"";`)
	communityappstest.WriteFile(t, root, communityappstest.CachePath, fixture)
	docker := &communityappstest.InstallEngine{}
	service := newInstallService(t, root, docker)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible || preview.Image != image {
		t.Fatalf("pinned image was not retained: %+v, %v", preview, err)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	attempt := waitForAttempt(t, service, claim.ID)
	if attempt.State != communityapps.AttemptSucceeded || attempt.Configuration.Image != image {
		t.Fatalf("pinned install failed: %+v", attempt)
	}
	data, err = os.ReadFile(filepath.Join(root, "boot/config/plugins/dockerMan/templates-user/my-heimdall.xml"))
	if err != nil || !bytes.Contains(data, []byte(`Name="PUID" Target="PUID"`)) {
		t.Fatalf("native template field name was not safe: %v", err)
	}
	_, _, _, opts, _ := docker.Observation()
	if opts.Config.Image != image || readNativeInstall(t, root, "heimdall").Repository != image {
		t.Fatal("the exact pinned update reference was lost")
	}
}

func TestInstallCleanupVerifiesQuarantinedTemplate(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprintf("foreign quarantine %v", edited), func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			dir := filepath.Join(root, "boot/config/plugins/dockerMan/templates-user")
			target := filepath.Join(dir, "my-heimdall.xml")
			var quarantine string
			docker := &communityappstest.InstallEngine{
				BeforeStart: func() error {
					var r receiptCheckpoint
					if err := captureReceipt(root, &r); err != nil {
						return err
					}
					quarantine = filepath.Join(dir, ".uma-ca-"+r.Attempt.ID+".published-cleanup")
					if !edited {
						if err := os.Rename(target, quarantine); err != nil {
							return err
						}
					}
					writer := target
					if edited {
						writer = quarantine
					}
					if err := os.WriteFile(writer, []byte("unrelated native record sentinel"), 0o600); err != nil {
						return err
					}
					return errdefs.ErrInvalidArgument.WithMessage("Docker rejected the start")
				},
			}
			service := newInstallService(t, root, docker)
			preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare returned %+v, %v", preview, err)
			}
			claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			outcome := waitForAttempt(t, service, claim.ID)
			if outcome.State != communityapps.AttemptFailed {
				t.Fatalf("start did not fail: %+v", outcome)
			}
			wantPath := target
			if edited {
				wantPath = quarantine
				if !slices.Contains(outcome.CleanupFailures, "remove_template") {
					t.Fatalf("unresolved cleanup was not reported: %+v", outcome)
				}
			} else if len(outcome.CleanupFailures) != 0 {
				t.Fatalf("owned quarantine did not finish cleanup: %+v", outcome)
			}
			if contents, err := os.ReadFile(wantPath); err != nil || string(contents) != "unrelated native record sentinel" {
				t.Fatalf("unrelated native file was lost: %v", err)
			}
		})
	}
}
