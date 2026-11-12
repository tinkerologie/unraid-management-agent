package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestUpdateContainerRetainsAndRestoresOriginal(t *testing.T) {
	for _, tc := range []struct {
		name         string
		running      bool
		failure      string
		wantOriginal bool
		wantRetained bool
	}{
		{name: "running", running: true},
		{name: "stopped"},
		{name: "stop fails", running: true, failure: "stop", wantOriginal: true},
		{name: "rename fails", running: true, failure: "rename", wantOriginal: true},
		{name: "create fails", running: true, failure: "create", wantOriginal: true},
		{name: "create fails when stopped", failure: "create", wantOriginal: true},
		{name: "ambiguous create", running: true, failure: "ambiguous", wantOriginal: true},
		{name: "start fails", running: true, failure: "start", wantOriginal: true},
		{name: "unrelated name conflict", running: true, failure: "conflict", wantRetained: true},
		{name: "cleanup fails", running: true, failure: "cleanup", wantRetained: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &updateTestEngine{originalName: "fixture", originalRunning: tc.running, failure: tc.failure}
			server := httptest.NewServer(engine)
			t.Cleanup(server.Close)
			t.Setenv("DOCKER_HOST", server.URL)
			controller := NewDockerController()
			result, err := controller.UpdateContainer("fixture", true)
			engine.mu.Lock()
			defer engine.mu.Unlock()
			switch {
			case tc.wantOriginal:
				if err == nil || !strings.Contains(err.Error(), "original restored") || engine.originalName != "fixture" ||
					engine.originalRunning != tc.running || engine.replacement || engine.originalRemoved {
					t.Fatalf("original was not restored: err=%v name=%q running=%v replacement=%v removed=%v",
						err, engine.originalName, engine.originalRunning, engine.replacement, engine.originalRemoved)
				}
			case tc.failure == "conflict":
				if err == nil || !strings.Contains(err.Error(), "retained") || !engine.replacement || engine.originalRemoved {
					t.Fatalf("name conflict harmed an unrelated container: err=%v replacement=%v removed=%v",
						err, engine.replacement, engine.originalRemoved)
				}
			default:
				if err != nil || result == nil || !result.Updated || !result.Recreated || !engine.replacement ||
					engine.replacementRunning != tc.running || engine.originalRemoved == tc.wantRetained {
					t.Fatalf("update result=%+v, err=%v, replacement=%v running=%v original removed=%v",
						result, err, engine.replacement, engine.replacementRunning, engine.originalRemoved)
				}
				if tc.wantRetained && !strings.Contains(result.Message, engine.originalName) {
					t.Fatalf("cleanup warning omits retained container %q: %s", engine.originalName, result.Message)
				}
				engine.assertCreateConfig(t)
			}
		})
	}
}

type updateTestEngine struct {
	mu                 sync.Mutex
	originalName       string
	originalRunning    bool
	originalRemoved    bool
	replacement        bool
	replacementRunning bool
	failure            string
	created            map[string]any
}

const (
	updateOriginalID    = "aaaaaaaaaaaa0001"
	updateReplacementID = "bbbbbbbbbbbb0002"
)

func (e *updateTestEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/v1.56")
	switch {
	case path == "/_ping":
		w.Header().Set("API-Version", "1.56")
	case path == "/images/create":
		if e.failure == "pull" {
			_, _ = fmt.Fprintln(w, `{"errorDetail":{"message":"registry pull failed"},"error":"registry pull failed"}`)
			return
		}
		_, _ = fmt.Fprintln(w, `{"status":"pulled"}`)
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		_, _ = fmt.Fprint(w, `{"Id":"sha256:new"}`)
	case path == "/containers/create":
		e.create(w, r)
	case strings.HasSuffix(path, "/json"):
		e.inspect(w, strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json"))
	case path == "/containers/"+updateOriginalID+"/stop":
		e.originalRunning = false
		if e.failure == "stop" {
			writeUpdateTestError(w, 500)
		}
	case path == "/containers/"+updateOriginalID+"/rename":
		name := r.URL.Query().Get("name")
		if e.failure == "rename" || (name == "fixture" && e.replacement) {
			writeUpdateTestError(w, 409)
			return
		}
		e.originalName = name
	case path == "/containers/"+updateOriginalID+"/start":
		e.originalRunning = true
	case path == "/containers/"+updateReplacementID+"/start":
		if e.failure == "start" {
			writeUpdateTestError(w, 500)
			return
		}
		e.replacementRunning = true
	case r.Method == http.MethodDelete && path == "/containers/"+updateOriginalID:
		if e.failure == "cleanup" {
			writeUpdateTestError(w, 500)
			return
		}
		e.originalRemoved = true
	case r.Method == http.MethodDelete && path == "/containers/"+updateReplacementID:
		e.replacement = false
	default:
		writeUpdateTestError(w, 404)
	}
}

func (e *updateTestEngine) create(w http.ResponseWriter, r *http.Request) {
	if err := json.NewDecoder(r.Body).Decode(&e.created); err != nil {
		writeUpdateTestError(w, 400)
		return
	}
	switch e.failure {
	case "create":
		writeUpdateTestError(w, 500)
	case "ambiguous", "conflict":
		e.replacement = true
		writeUpdateTestError(w, 409)
	default:
		e.replacement = true
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"Id":%q}`, updateReplacementID)
	}
}

func (e *updateTestEngine) inspect(w http.ResponseWriter, ref string) {
	if !e.originalRemoved && (ref == updateOriginalID || ref == e.originalName) {
		_, _ = fmt.Fprintf(w, `{
			"Id":%q,"Name":%q,"Image":"sha256:old","Config":{"Image":"fixture:latest"},
			"State":{"Running":%t},"HostConfig":{"Memory":268435456,"MemorySwap":536870912,"NetworkMode":"static"},
			"Mounts":[{"Type":"volume","Name":"existing-data","Destination":"/data","RW":true}],
			"NetworkSettings":{"Networks":{
				"static":{"IPAMConfig":{"IPv4Address":"192.0.2.5","IPv6Address":"2001:db8::5"},"Aliases":["static-alias","aaaaaaaaaaaa"],"EndpointID":"old-static"},
				"dynamic":{"IPAddress":"198.51.100.2","Aliases":["dynamic-alias"],"EndpointID":"old-dynamic"}
			}}}`, updateOriginalID, "/"+e.originalName, e.originalRunning)
		return
	}
	if e.replacement && (ref == "fixture" || ref == updateReplacementID) {
		labels := any(nil)
		if e.failure != "conflict" {
			labels = e.created["Labels"]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id": updateReplacementID, "Config": map[string]any{"Labels": labels},
		})
		return
	}
	writeUpdateTestError(w, 404)
}

func writeUpdateTestError(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
	_, _ = fmt.Fprint(w, `{"message":"fixture failure"}`)
}

func (e *updateTestEngine) assertCreateConfig(t *testing.T) {
	t.Helper()
	host, _ := e.created["HostConfig"].(map[string]any)
	if host["Memory"] != float64(268435456) || host["MemorySwap"] != float64(536870912) {
		t.Fatalf("memory limits were not preserved: %v", host)
	}
	mounts, _ := host["Mounts"].([]any)
	if len(mounts) != 1 {
		t.Fatalf("existing volume mount is missing: %v", mounts)
	}
	volume, _ := mounts[0].(map[string]any)
	if volume["Source"] != "existing-data" || volume["Target"] != "/data" {
		t.Fatalf("unexpected volume mount: %v", volume)
	}
	networking, _ := e.created["NetworkingConfig"].(map[string]any)
	endpoints, _ := networking["EndpointsConfig"].(map[string]any)
	static, _ := endpoints["static"].(map[string]any)
	dynamic, _ := endpoints["dynamic"].(map[string]any)
	ipam, _ := static["IPAMConfig"].(map[string]any)
	aliases, _ := static["Aliases"].([]any)
	if len(endpoints) != 2 || ipam["IPv4Address"] != "192.0.2.5" || ipam["IPv6Address"] != "2001:db8::5" ||
		len(aliases) != 1 || aliases[0] != "static-alias" || static["EndpointID"] != "" ||
		dynamic["IPAMConfig"] != nil || dynamic["IPAddress"] != "" || dynamic["EndpointID"] != "" {
		t.Fatalf("unexpected endpoint config: %v", endpoints)
	}
}

func TestUpdateContainerAbortsOnRegistryStreamError(t *testing.T) {
	engine := &updateTestEngine{originalName: "fixture", originalRunning: true, failure: "pull"}
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", server.URL)

	_, err := NewDockerController().UpdateContainer("fixture", true)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "registry pull failed") || !engine.originalRunning ||
		engine.originalName != "fixture" || engine.originalRemoved || engine.created != nil {
		t.Fatalf("failed pull reached recreation: err=%v engine=%+v", err, engine)
	}
}
