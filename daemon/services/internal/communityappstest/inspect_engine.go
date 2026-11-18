package communityappstest

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// UseDockerInspectEngine points the Docker SDK at an isolated local Engine fixture.
func UseDockerInspectEngine(t *testing.T, containers map[string]string) {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.56")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1.56")
		if strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json") {
			ref := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
			if result, ok := containers[ref]; ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(result))
				return
			}
		}
		http.Error(w, `{"message":"No such container"}`, http.StatusNotFound)
	})
	socketDir, err := os.MkdirTemp("", "uma-detail-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "docker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	engine := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = engine.Serve(listener) }()
	t.Cleanup(func() { _ = engine.Close() })
	t.Setenv("DOCKER_HOST", "unix://"+socket)
}
