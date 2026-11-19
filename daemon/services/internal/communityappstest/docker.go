package communityappstest

import (
	"context"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	digest "github.com/opencontainers/go-digest"
)

const (
	HeimdallReference = "3adc90987a3f0337de838a0069fb7905a3ff217346f733a233af7e30146db071" // pragma: allowlist secret (public fixture listing identifier)
	ApprovedDigest    = digest.Digest("sha256:3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b")
)

// Docker implements the read-only Engine methods needed by preview fixtures.
type Docker struct {
	Containers []container.Summary
	Bridge     network.Network
	Digest     digest.Digest
	Err        error
}

func (*Docker) Close() error { return nil }

func (d *Docker) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: d.Containers}, d.Err
}

func (d *Docker) NetworkInspect(_ context.Context, name string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	bridge := d.Bridge
	if bridge.ID == "" {
		bridge = network.Network{Name: name, ID: strings.Repeat("c", 64), Driver: "bridge", Scope: "local", EnableIPv4: true}
	}
	return client.NetworkInspectResult{Network: network.Inspect{Network: bridge}}, d.Err
}

func (d *Docker) DistributionInspect(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
	var result client.DistributionInspectResult
	result.Descriptor.Digest = d.Digest
	if d.Digest == "" {
		result.Descriptor.Digest = ApprovedDigest
	}
	return result, d.Err
}
