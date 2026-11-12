package controllers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

func TestDockerReplacementReadsConfiguredMACsFromLocalEngine(t *testing.T) {
	root := t.TempDir()
	id := strings.Repeat("a", 64)
	writeDockerMACFixture(t, root, id, fmt.Sprintf(`{
		"ID":%q,
		"NetworkSettings":{"Networks":{
			"primary":{"DesiredMacAddress":"02:76:00:00:00:01"},
			"secondary":{"DesiredMacAddress":"02:76:00:00:00:02"},
			"dynamic":{"MacAddress":"ea:84:00:2c:cf:11","DesiredMacAddress":""}
		}}}`, id))
	socketDir, err := os.MkdirTemp("", "uma-mac-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "engine.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/v1.56") {
		case "/_ping":
			w.Header().Set("API-Version", "1.56")
		case "/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"DockerRootDir": root})
		default:
			http.NotFound(w, r)
		}
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", "unix://"+socket)

	controller := NewDockerController()
	t.Cleanup(func() { _ = controller.Close() })
	if err := controller.initClient(); err != nil {
		t.Fatal(err)
	}
	original := container.InspectResponse{
		ID:         id,
		HostConfig: &container.HostConfig{NetworkMode: "primary"},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{
			"primary": {}, "secondary": {}, "dynamic": {MacAddress: network.HardwareAddr{0xea, 0x84, 0, 0x2c, 0xcf, 0x11}},
		}},
	}
	addresses, err := controller.configuredDockerMACs(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	replacement := dockerReplacementNetworks(original.NetworkSettings, "fixture", addresses)
	if len(addresses) != 2 ||
		replacement.EndpointsConfig["primary"].MacAddress.String() != "02:76:00:00:00:01" ||
		replacement.EndpointsConfig["secondary"].MacAddress.String() != "02:76:00:00:00:02" ||
		len(replacement.EndpointsConfig["dynamic"].MacAddress) != 0 {
		t.Fatalf("incorrect configured MACs: %v", addresses)
	}

	server.Close()
	if _, err := controller.configuredDockerMACs(t.Context(), original); err == nil || !strings.Contains(err.Error(), "find local Docker state directory") {
		t.Fatalf("expected Docker state discovery failure: %v", err)
	}
}

func TestDockerConfiguredMACsRejectInvalidLocalState(t *testing.T) {
	id := strings.Repeat("b", 64)
	root := t.TempDir()
	for _, tc := range []struct{ root, id, reason string }{
		{"relative", id, "root directory"},
		{root, strings.Repeat("/", 64), "container ID"},
		{filepath.Join(root, "absent"), id, "open local Docker"},
		{root, id, "stat Docker container"},
	} {
		if _, err := readDockerConfiguredMACs(tc.root, tc.id); err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Fatalf("expected %q, got %v", tc.reason, err)
		}
	}
	for _, tc := range []struct{ input, reason string }{
		{"{", "decode Docker container state"},
		{`{"ID":"another-container"}`, "does not match"},
		{strings.Repeat(" ", dockerContainerStateLimit+1), "bounded regular file"},
	} {
		root := t.TempDir()
		writeDockerMACFixture(t, root, id, tc.input)
		if _, err := readDockerConfiguredMACs(root, id); err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Fatalf("expected %q, got %v", tc.reason, err)
		}
	}
}

func writeDockerMACFixture(t *testing.T, root, id, contents string) {
	t.Helper()
	dir := filepath.Join(root, "containers", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.v2.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
