package collectors

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

const (
	stoppedMemoryInspectBudget  = 8
	stoppedMemoryInspectTimeout = 5 * time.Second
	stoppedMemoryCapTTL         = 15 * time.Minute
)

type dockerMemoryCap struct {
	bytes       uint64
	lastSuccess time.Time
	lastAttempt time.Time
}

func (c *DockerCollector) recordDockerMemoryCap(id string, host *container.HostConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cap := c.memoryCaps[id]
	cap.lastAttempt = time.Now()
	if host != nil {
		cap.bytes = uint64(max(host.Memory, 0))
		cap.lastSuccess = cap.lastAttempt
	}
	c.memoryCaps[id] = cap
}

func (c *DockerCollector) inspectStoppedMemoryCaps(ctx context.Context, containers []container.Summary) {
	type candidate struct {
		id          string
		lastAttempt time.Time
	}
	var candidates []candidate
	cutoff := time.Now().Add(-stoppedMemoryCapTTL)
	c.mu.Lock()
	for _, item := range containers {
		cap := c.memoryCaps[item.ID]
		if !strings.EqualFold(string(item.State), "running") && cap.lastSuccess.Before(cutoff) {
			candidates = append(candidates, candidate{item.ID, cap.lastAttempt})
		}
	}
	c.mu.Unlock()
	slices.SortFunc(candidates, func(a, b candidate) int {
		if order := a.lastAttempt.Compare(b.lastAttempt); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})

	// Use one deadline for the extra work so a slow engine cannot spend the
	// whole collection interval timing out individual stopped containers.
	bounded, cancel := context.WithTimeout(ctx, stoppedMemoryInspectTimeout)
	defer cancel()
	for _, item := range candidates[:min(len(candidates), stoppedMemoryInspectBudget)] {
		if bounded.Err() != nil {
			break
		}
		result, err := c.dockerClient.ContainerInspect(bounded, item.id, client.ContainerInspectOptions{})
		if err != nil {
			c.recordDockerMemoryCap(item.id, nil)
			logger.Debug("Docker SDK: stopped container memory configuration unavailable: %v", err)
			continue
		}
		c.recordDockerMemoryCap(item.id, result.Container.HostConfig)
	}
}

func (c *DockerCollector) publishConfiguredMemoryCaps(containers map[string]*dto.ContainerInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, cap := range c.memoryCaps {
		cont, listed := containers[id]
		if !listed {
			delete(c.memoryCaps, id)
		} else if !cap.lastSuccess.IsZero() {
			value := cap.bytes
			cont.ConfiguredMemoryLimit = &value
		}
	}
}
