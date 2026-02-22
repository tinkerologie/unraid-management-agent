package watchdog

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestProbeHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := dto.HealthCheck{
		ID:             "http-ok",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
	}

	result := RunProbe(context.Background(), check)
	if !result.Healthy {
		t.Errorf("expected healthy, got error: %s", result.Error)
	}
}

func TestProbeHTTP_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	check := dto.HealthCheck{
		ID:             "http-500",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    200,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for wrong status code")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestProbeHTTP_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := dto.HealthCheck{
		ID:             "http-timeout",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 1,
		SuccessCode:    200,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for timeout")
	}
}

func TestProbeHTTP_InvalidURL(t *testing.T) {
	check := dto.HealthCheck{
		ID:             "http-bad",
		Type:           dto.HealthCheckHTTP,
		Target:         "http://192.0.2.1:1",
		TimeoutSeconds: 1,
		SuccessCode:    200,
	}
	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for unreachable URL")
	}
}

func TestProbeTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	check := dto.HealthCheck{
		ID:             "tcp-ok",
		Type:           dto.HealthCheckTCP,
		Target:         ln.Addr().String(),
		TimeoutSeconds: 5,
	}

	result := RunProbe(context.Background(), check)
	if !result.Healthy {
		t.Errorf("expected healthy, got error: %s", result.Error)
	}
}

func TestProbeTCP_Failure(t *testing.T) {
	check := dto.HealthCheck{
		ID:             "tcp-fail",
		Type:           dto.HealthCheckTCP,
		Target:         "127.0.0.1:1",
		TimeoutSeconds: 1,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for refused connection")
	}
}

type mockDockerProvider struct {
	containers []dto.ContainerInfo
}

func (m *mockDockerProvider) GetDockerCache() []dto.ContainerInfo {
	return m.containers
}

func TestProbeContainer_Running(t *testing.T) {
	old := dockerProviderInst
	defer func() { dockerProviderInst = old }()

	dockerProviderInst = &mockDockerProvider{
		containers: []dto.ContainerInfo{
			{ID: "abc123", Name: "plex", State: "running"},
		},
	}

	check := dto.HealthCheck{
		ID:             "container-ok",
		Type:           dto.HealthCheckContainer,
		Target:         "plex",
		TimeoutSeconds: 5,
	}

	result := RunProbe(context.Background(), check)
	if !result.Healthy {
		t.Errorf("expected healthy, got error: %s", result.Error)
	}
}

func TestProbeContainer_NotRunning(t *testing.T) {
	old := dockerProviderInst
	defer func() { dockerProviderInst = old }()

	dockerProviderInst = &mockDockerProvider{
		containers: []dto.ContainerInfo{
			{ID: "abc123", Name: "plex", State: "exited"},
		},
	}

	check := dto.HealthCheck{
		ID:             "container-down",
		Type:           dto.HealthCheckContainer,
		Target:         "plex",
		TimeoutSeconds: 5,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for exited container")
	}
}

func TestProbeContainer_NotFound(t *testing.T) {
	old := dockerProviderInst
	defer func() { dockerProviderInst = old }()

	dockerProviderInst = &mockDockerProvider{
		containers: []dto.ContainerInfo{},
	}

	check := dto.HealthCheck{
		ID:             "container-missing",
		Type:           dto.HealthCheckContainer,
		Target:         "nonexistent",
		TimeoutSeconds: 5,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for missing container")
	}
}

func TestProbeContainer_NoProvider(t *testing.T) {
	old := dockerProviderInst
	defer func() { dockerProviderInst = old }()

	dockerProviderInst = nil

	check := dto.HealthCheck{
		ID:   "container-noprov",
		Type: dto.HealthCheckContainer,
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy when no docker provider set")
	}
}

func TestProbeUnknownType(t *testing.T) {
	check := dto.HealthCheck{
		ID:   "unknown",
		Type: dto.HealthCheckType("grpc"),
	}

	result := RunProbe(context.Background(), check)
	if result.Healthy {
		t.Error("expected unhealthy for unknown probe type")
	}
}

func TestProbeHTTP_CustomSuccessCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	check := dto.HealthCheck{
		ID:             "http-202",
		Type:           dto.HealthCheckHTTP,
		Target:         srv.URL,
		TimeoutSeconds: 5,
		SuccessCode:    202,
	}

	result := RunProbe(context.Background(), check)
	if !result.Healthy {
		t.Errorf("expected healthy for 202, got error: %s", result.Error)
	}
}

func TestProbeContainer_MatchByID(t *testing.T) {
	old := dockerProviderInst
	defer func() { dockerProviderInst = old }()

	dockerProviderInst = &mockDockerProvider{
		containers: []dto.ContainerInfo{
			{ID: "abc123def456", Name: "some-container", State: "running"},
		},
	}

	check := dto.HealthCheck{
		ID:     "container-by-id",
		Type:   dto.HealthCheckContainer,
		Target: "abc123def456",
	}

	result := RunProbe(context.Background(), check)
	if !result.Healthy {
		t.Errorf("expected match by container ID, got error: %s", result.Error)
	}
}

func TestProbeTCP_MultipleListeners(t *testing.T) {
	listeners := make([]net.Listener, 3)
	for i := range listeners {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners[i] = ln
		defer ln.Close()
	}

	for i, ln := range listeners {
		t.Run(fmt.Sprintf("listener-%d", i), func(t *testing.T) {
			check := dto.HealthCheck{
				ID:             fmt.Sprintf("tcp-%d", i),
				Type:           dto.HealthCheckTCP,
				Target:         ln.Addr().String(),
				TimeoutSeconds: 2,
			}
			result := RunProbe(context.Background(), check)
			if !result.Healthy {
				t.Errorf("expected healthy for listener %d, got error: %s", i, result.Error)
			}
		})
	}
}
