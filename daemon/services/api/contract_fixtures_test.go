package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

const contractSentinel = "2000-01-01T00:00:00Z"
const contractBearer = "synthetic-contract-test"

var contractTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type contractCase struct {
	ID               string   `json:"id"`
	Transport        string   `json:"transport"`
	Method           string   `json:"method,omitempty"`
	Path             string   `json:"path,omitempty"`
	Status           int      `json:"status,omitempty"`
	Event            string   `json:"event,omitempty"`
	Envelope         string   `json:"envelope,omitempty"`
	Model            string   `json:"model"`
	Case             string   `json:"case"`
	Semantics        []string `json:"semantics"`
	Origin           string   `json:"origin"`
	Fixture          string   `json:"fixture"`
	SHA256           string   `json:"sha256"`
	NormalizedFields []string `json:"normalized_fields,omitempty"`
}

type contractCapture struct {
	contractCase
	configure func(*Server)
	publish   func(*domain.EventBus)
}

func TestProducerContractFixtures(t *testing.T) {
	cases := producerContractCases()
	slices.SortFunc(cases, func(a, b contractCapture) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	manifest := struct {
		SchemaRevision    int            `json:"schema_revision"`
		Generator         string         `json:"generator"`
		GeneratorRevision int            `json:"generator_revision"`
		BasePath          string         `json:"base_path"`
		FixtureEncoding   string         `json:"fixture_encoding"`
		TimestampSentinel string         `json:"timestamp_sentinel"`
		Cases             []contractCase `json:"cases"`
	}{1, "TestProducerContractFixtures", 1, "/api/v1", "JSON, two-space indentation, trailing LF", contractSentinel, nil}

	files := make(map[string][]byte, len(cases)+1)
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			c.Fixture = c.ID + ".json"
			if _, exists := files[c.Fixture]; exists {
				t.Fatalf("duplicate fixture ID %q", c.ID)
			}
			s := newContractServer(t)
			if c.configure != nil {
				c.configure(s)
			}
			var raw []byte
			if c.publish != nil {
				raw = captureContractEvent(t, s, c)
			} else {
				raw = captureContractResponse(t, s, c)
			}
			var formatted bytes.Buffer
			if err := json.Indent(&formatted, bytes.TrimSpace(raw), "", "  "); err != nil {
				t.Fatalf("format producer JSON: %v", err)
			}
			formatted.WriteByte('\n')
			files[c.Fixture] = formatted.Bytes()
			c.SHA256 = fmt.Sprintf("%x", sha256.Sum256(formatted.Bytes()))
			manifest.Cases = append(manifest.Cases, c.contractCase)
		})
	}
	if t.Failed() {
		return
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = append(raw, '\n')
	checkContractFiles(t, files)
}

func newContractServer(t *testing.T) *Server {
	t.Helper()
	s := NewServer(&domain.Context{
		Hub:    domain.NewEventBus(32),
		Config: domain.Config{APIToken: contractBearer, MetricsDisabled: true},
	})
	t.Cleanup(s.Stop)
	return s
}

func captureContractResponse(t *testing.T, s *Server, c contractCapture) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(c.Method, c.Path, nil)
	req.Header.Set("Authorization", "Bearer "+contractBearer)
	start := time.Now()
	s.GetRouter().ServeHTTP(w, req)
	if w.Code != c.Status {
		t.Fatalf("%s: HTTP status %d, want %d", c.Path, w.Code, c.Status)
	}
	raw := w.Body.Bytes()
	if len(c.NormalizedFields) != 0 {
		return normalizeContractTimestamp(t, raw, start, time.Now())
	}
	return raw
}

func normalizeContractTimestamp(t *testing.T, raw []byte, start, end time.Time) []byte {
	t.Helper()
	var body struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	// The JSON representation has no monotonic time component.
	if body.Timestamp.Before(start) || body.Timestamp.After(end) {
		t.Fatalf("producer timestamp %v outside request window [%v, %v]", body.Timestamp, start, end)
	}
	field := []byte(fmt.Sprintf(`"timestamp":%q`, body.Timestamp.Format(time.RFC3339Nano)))
	if bytes.Count(raw, field) != 1 {
		t.Fatal("expected one exact timestamp field to normalize")
	}
	return bytes.Replace(raw, field, []byte(`"timestamp":"`+contractSentinel+`"`), 1)
}

func captureContractEvent(t *testing.T, s *Server, c contractCapture) []byte {
	t.Helper()
	s.wsHub.now = func() time.Time { return contractTime }
	s.StartSubscriptions()
	select {
	case <-s.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("subscriptions did not become ready")
	}
	server := httptest.NewServer(s.GetRouter())
	t.Cleanup(server.Close)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/ws",
		http.Header{"Authorization": {"Bearer " + contractBearer}})
	if response != nil {
		defer func() { _ = response.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close()
		waitForContractClients(t, s, 0)
	}()
	waitForContractClients(t, s, 1)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	c.publish(s.ctx.Hub)
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Event != c.Event {
		t.Fatalf("expected %q envelope, got %q (%v)", c.Event, envelope.Event, err)
	}
	return raw
}

func waitForContractClients(t *testing.T, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.wsHub.ClientCount() != want {
		if time.Now().After(deadline) {
			t.Fatalf("WebSocket hub did not reach %d clients", want)
		}
		time.Sleep(time.Millisecond)
	}
}

func checkContractFiles(t *testing.T, files map[string][]byte) {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "contracts", "v1")
	update := os.Getenv("UPDATE_CONTRACT_FIXTURES") == "1"
	if update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read producer fixtures: %v. Run make contract-fixtures for intentional changes.", err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			t.Errorf("unexpected non-regular entry %s", entry.Name())
			continue
		}
		if _, expected := files[entry.Name()]; !expected {
			t.Errorf("unexpected fixture %s; remove it if the case was intentionally retired", entry.Name())
		}
	}
	if t.Failed() {
		return
	}
	for name, expected := range files {
		path := filepath.Join(dir, name)
		if update {
			if err := os.WriteFile(path, expected, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Errorf("%s differs or is missing (%v). Run make contract-fixtures for intentional changes.", name, err)
		}
	}
}
