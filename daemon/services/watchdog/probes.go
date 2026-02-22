package watchdog

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ProbeResult holds the outcome of a single probe execution.
type ProbeResult struct {
	Healthy bool
	Error   string
}

// RunProbe executes the appropriate probe based on the health check type.
func RunProbe(ctx context.Context, check dto.HealthCheck) ProbeResult {
	timeout := time.Duration(check.TimeoutSeconds) * time.Second

	switch check.Type {
	case dto.HealthCheckHTTP:
		return probeHTTP(ctx, check.Target, check.SuccessCode, timeout)
	case dto.HealthCheckTCP:
		return probeTCP(ctx, check.Target, timeout)
	case dto.HealthCheckContainer:
		return probeContainer(ctx, check.Target)
	default:
		return ProbeResult{Healthy: false, Error: fmt.Sprintf("unknown probe type: %s", check.Type)}
	}
}

// probeHTTP performs an HTTP GET and checks the response status code.
func probeHTTP(ctx context.Context, url string, expectedCode int, timeout time.Duration) ProbeResult {
	if expectedCode == 0 {
		expectedCode = DefaultSuccessCode
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{Healthy: false, Error: fmt.Sprintf("creating request: %s", err)}
	}

	resp, err := client.Do(req) //nolint:gosec //#nosec G704 -- Target URL is user-configured health check endpoint
	if err != nil {
		return ProbeResult{Healthy: false, Error: fmt.Sprintf("HTTP request failed: %s", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != expectedCode {
		return ProbeResult{
			Healthy: false,
			Error:   fmt.Sprintf("expected status %d, got %d", expectedCode, resp.StatusCode),
		}
	}

	return ProbeResult{Healthy: true}
}

// probeTCP attempts a TCP connection to host:port.
func probeTCP(_ context.Context, target string, timeout time.Duration) ProbeResult {
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return ProbeResult{Healthy: false, Error: fmt.Sprintf("TCP connect failed: %s", err)}
	}
	_ = conn.Close()
	return ProbeResult{Healthy: true}
}

// probeContainer checks if a Docker container is running using the Docker SDK.
func probeContainer(ctx context.Context, containerID string) ProbeResult {
	dockerProvider := getDockerProvider()
	if dockerProvider == nil {
		return ProbeResult{Healthy: false, Error: "Docker provider not available"}
	}

	containers := dockerProvider.GetDockerCache()
	if containers == nil {
		return ProbeResult{Healthy: false, Error: "Docker cache not available"}
	}

	for _, c := range containers {
		if c.ID == containerID || c.Name == containerID {
			if c.State == "running" {
				return ProbeResult{Healthy: true}
			}
			return ProbeResult{
				Healthy: false,
				Error:   fmt.Sprintf("container %s is %s (expected running)", containerID, c.State),
			}
		}
	}

	return ProbeResult{Healthy: false, Error: fmt.Sprintf("container %s not found", containerID)}
}

// DockerCacheProvider provides access to the Docker container cache.
type DockerCacheProvider interface {
	GetDockerCache() []dto.ContainerInfo
}

var dockerProviderInst DockerCacheProvider

// SetDockerProvider sets the Docker cache provider (called during initialization).
func SetDockerProvider(p DockerCacheProvider) {
	logger.Debug("Watchdog: Docker cache provider set")
	dockerProviderInst = p
}

func getDockerProvider() DockerCacheProvider {
	return dockerProviderInst
}
