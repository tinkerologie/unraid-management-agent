package communityappstest

import (
	"context"
	"io"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
)

const ContainerID = "2a0686c9ded09699be089748236184e2b5563269a1759710edf66f8b302d2982" // pragma: allowlist secret (synthetic Docker identifier)
const ImageID = "sha256:1c5d673b707b4d180ac2d4c5c81ca0480424ed82f1b67855acb6720a193ef341"

type RemoveRequest struct {
	ID      string
	Options client.ContainerRemoveOptions
}

type InstallEngine struct {
	Docker
	BeforePull             func(context.Context) error
	BeforeDistribution     func()
	BeforeList             func() error
	BeforeNetwork          func() error
	BeforeCreate           func(client.ContainerCreateOptions) error
	BeforeContainerInspect func(string, *container.InspectResponse) error
	AfterCreate            func() error
	BeforeStart            func() error
	AfterStart             func() error
	BeforeRemove           func() error
	AfterRemove            func() error
	PullMessages           []jsonstream.Message
	PullStreamErr          error
	InspectError           error
	NoDigestProof          bool
	HasDescriptor          bool
	PostStartInspectError  error
	CreateImageID          string
	NoStartEffect          bool
	DropCreateID           bool

	mu          sync.Mutex
	created     map[string]container.InspectResponse
	started     []string
	removed     []RemoveRequest
	createCount int
	lastOptions client.ContainerCreateOptions
	lastInspect string
}

func (d *InstallEngine) ContainerList(ctx context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.BeforeList != nil {
		if err := d.BeforeList(); err != nil {
			return client.ContainerListResult{}, err
		}
	}
	result, err := d.Docker.ContainerList(ctx, opts)
	for _, c := range d.created {
		summary := container.Summary{ID: c.ID, Names: []string{c.Name}, Labels: c.Config.Labels}
		for _, m := range c.HostConfig.Mounts {
			summary.Mounts = append(summary.Mounts, container.MountPoint{Type: mount.TypeBind, Source: m.Source, RW: !m.ReadOnly})
		}
		result.Items = append(result.Items, summary)
	}
	return result, err
}

func (d *InstallEngine) NetworkInspect(ctx context.Context, name string, opts client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	if d.BeforeNetwork != nil {
		if err := d.BeforeNetwork(); err != nil {
			return client.NetworkInspectResult{}, err
		}
	}
	return d.Docker.NetworkInspect(ctx, name, opts)
}

func (d *InstallEngine) DistributionInspect(ctx context.Context, name string, opts client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
	if d.BeforeDistribution != nil {
		d.BeforeDistribution()
	}
	return d.Docker.DistributionInspect(ctx, name, opts)
}

func (d *InstallEngine) ImagePull(ctx context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	if d.BeforePull != nil {
		if err := d.BeforePull(ctx); err != nil {
			return nil, err
		}
	}
	messages := d.PullMessages
	if messages == nil {
		messages = []jsonstream.Message{{Status: "Digest: " + ApprovedDigest.String()}}
	}
	return fakePull{messages: messages, streamErr: d.PullStreamErr}, nil
}

func (d *InstallEngine) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastInspect = ref
	var result client.ImageInspectResult
	result.ID = ImageID
	if d.HasDescriptor {
		result.Descriptor = &ocispec.Descriptor{Digest: ApprovedDigest}
	} else if !d.NoDigestProof {
		result.RepoDigests = []string{"lscr.io/linuxserver/heimdall@" + ApprovedDigest.String()}
	}
	return result, d.InspectError
}

func (d *InstallEngine) ContainerCreate(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.BeforeCreate != nil {
		if err := d.BeforeCreate(opts); err != nil {
			return client.ContainerCreateResult{}, err
		}
	}
	if d.created == nil {
		d.created = make(map[string]container.InspectResponse)
	}
	id := ContainerID
	if d.createCount != 0 {
		id = strings.Repeat("d", 64)
	}
	d.createCount++
	d.lastOptions = opts
	runtimeImage := d.CreateImageID
	if runtimeImage == "" {
		runtimeImage = ImageID
	}
	d.created[id] = container.InspectResponse{
		ID: id, Name: "/" + opts.Name, Image: runtimeImage, HostConfig: opts.HostConfig,
		State:  &container.State{Status: "created", StartedAt: "0001-01-01T00:00:00Z"},
		Config: opts.Config,
	}
	if d.AfterCreate != nil {
		if err := d.AfterCreate(); err != nil {
			return client.ContainerCreateResult{}, err
		}
	}
	if d.DropCreateID {
		id = ""
	}
	return client.ContainerCreateResult{ID: id}, nil
}

func (d *InstallEngine) ContainerInspect(_ context.Context, key string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.started) != 0 && d.PostStartInspectError != nil {
		return client.ContainerInspectResult{}, d.PostStartInspectError
	}
	for id, c := range d.created {
		if id == key || c.Name == "/"+key {
			if c.State != nil {
				state := *c.State
				c.State = &state
			}
			if d.BeforeContainerInspect != nil {
				if err := d.BeforeContainerInspect(key, &c); err != nil {
					return client.ContainerInspectResult{}, err
				}
			}
			return client.ContainerInspectResult{Container: c}, nil
		}
	}
	if d.BeforeContainerInspect != nil {
		if err := d.BeforeContainerInspect(key, nil); err != nil {
			return client.ContainerInspectResult{}, err
		}
	}
	return client.ContainerInspectResult{}, errdefs.ErrNotFound
}

func (d *InstallEngine) ContainerStart(_ context.Context, id string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.started = append(d.started, id)
	if d.BeforeStart != nil {
		if err := d.BeforeStart(); err != nil {
			return client.ContainerStartResult{}, err
		}
	}
	c, found := d.created[id]
	if !found {
		return client.ContainerStartResult{}, errdefs.ErrNotFound
	}
	if !d.NoStartEffect {
		c.State.Status = "exited"
		c.State.StartedAt = time.Unix(1, 0).UTC().Format(time.RFC3339Nano)
	}
	if d.AfterStart != nil {
		return client.ContainerStartResult{}, d.AfterStart()
	}
	return client.ContainerStartResult{}, nil
}

func (d *InstallEngine) ContainerRemove(_ context.Context, id string, opts client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.removed = append(d.removed, RemoveRequest{ID: id, Options: opts})
	if d.BeforeRemove != nil {
		if err := d.BeforeRemove(); err != nil {
			return client.ContainerRemoveResult{}, err
		}
	}
	delete(d.created, id)
	if d.AfterRemove != nil {
		return client.ContainerRemoveResult{}, d.AfterRemove()
	}
	return client.ContainerRemoveResult{}, nil
}

func (d *InstallEngine) Observation() (int, int, []string, client.ContainerCreateOptions, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.createCount, len(d.created), append([]string{}, d.started...), d.lastOptions, d.lastInspect
}

func (d *InstallEngine) RemoveRequests() []RemoveRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]RemoveRequest(nil), d.removed...)
}

func (d *InstallEngine) ReplaceForUpdate(t testing.TB, replacementID string) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.created[ContainerID]
	if !ok {
		t.Fatal("cannot replace missing initial container")
	}
	delete(d.created, ContainerID)
	c.ID = replacementID
	c.Config.Labels[constants.DockerUpdateAttemptLabel] = "fixture updater marker"
	d.created[replacementID] = c
}

type fakePull struct {
	messages  []jsonstream.Message
	streamErr error
}

func (fakePull) Close() error                 { return nil }
func (fakePull) Read([]byte) (int, error)     { return 0, io.EOF }
func (p fakePull) Wait(context.Context) error { return p.streamErr }
func (p fakePull) JSONMessages(context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {
		for _, msg := range p.messages {
			if !yield(msg, nil) {
				return
			}
		}
		if p.streamErr != nil {
			yield(jsonstream.Message{}, p.streamErr)
		}
	}
}
