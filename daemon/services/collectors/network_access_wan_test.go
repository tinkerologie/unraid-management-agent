package collectors

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// wanService starts a fake WAN IP service that answers with status and body
// and counts its hits.
func wanService(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

// networkCfg writes a network.cfg with content (none if content is "") and
// returns its path.
func networkCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "network.cfg")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// discovery returns a discovery wired to the given services and network.cfg.
func discovery(wanLookup bool, cfgPath string, services ...string) *NetworkAccessDiscovery {
	d := NewNetworkAccessDiscovery(wanLookup)
	d.client = http.DefaultClient
	d.wanServices = services
	d.networkCfgPath = cfgPath
	return d
}

// failingDoer fails the test if any HTTP request is made.
type failingDoer struct {
	t     *testing.T
	calls atomic.Int32
}

func (f *failingDoer) Do(req *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	f.t.Errorf("unexpected external request to %s", req.URL)
	return nil, errors.New("external requests are not allowed")
}

func TestWANLookupServiceOrder(t *testing.T) {
	first, firstHits := wanService(t, http.StatusInternalServerError, "")
	second, secondHits := wanService(t, http.StatusOK, "203.0.113.20\n")
	third, thirdHits := wanService(t, http.StatusOK, "203.0.113.30")

	got := discovery(true, networkCfg(t, ""), first.URL, second.URL, third.URL).wanAccessURL()

	if got == nil || got.IPv4 != "http://203.0.113.20" {
		t.Fatalf("wanAccessURL() = %+v, want the second service's IP", got)
	}
	if firstHits.Load() != 1 || secondHits.Load() != 1 || thirdHits.Load() != 0 {
		t.Errorf("hits = %d/%d/%d, want 1/1/0", firstHits.Load(), secondHits.Load(), thirdHits.Load())
	}
}

func TestWANLookupFirstSuccessShortCircuits(t *testing.T) {
	first, firstHits := wanService(t, http.StatusOK, "203.0.113.10")
	second, secondHits := wanService(t, http.StatusOK, "203.0.113.20")

	got := discovery(true, networkCfg(t, ""), first.URL, second.URL).wanAccessURL()

	if got == nil || got.IPv4 != "http://203.0.113.10" {
		t.Fatalf("wanAccessURL() = %+v, want the first service's IP", got)
	}
	if firstHits.Load() != 1 || secondHits.Load() != 0 {
		t.Errorf("hits = %d/%d, want 1/0", firstHits.Load(), secondHits.Load())
	}
}

func TestWANLookupSkipsInvalidBody(t *testing.T) {
	first, _ := wanService(t, http.StatusOK, "<html>not an ip</html>")
	second, _ := wanService(t, http.StatusOK, "203.0.113.20")

	got := discovery(true, networkCfg(t, ""), first.URL, second.URL).wanAccessURL()

	if got == nil || got.IPv4 != "http://203.0.113.20" {
		t.Fatalf("wanAccessURL() = %+v, want the second service's IP", got)
	}
}

func TestWANLookupFallsBackToNetworkCfg(t *testing.T) {
	failing, _ := wanService(t, http.StatusServiceUnavailable, "")

	got := discovery(true, networkCfg(t, "WANIP=\"203.0.113.40\"\n"), failing.URL).wanAccessURL()
	if got == nil || got.IPv4 != "http://203.0.113.40" {
		t.Errorf("with network.cfg: wanAccessURL() = %+v, want the configured WANIP", got)
	}

	if got := discovery(true, networkCfg(t, ""), failing.URL).wanAccessURL(); got != nil {
		t.Errorf("without network.cfg: wanAccessURL() = %+v, want nil", got)
	}
}

func TestWANLookupDisabledMakesNoRequests(t *testing.T) {
	tests := []struct {
		name    string
		cfg     string
		wantURL string
	}{
		{"configured WANIP is used", "WANIP=\"203.0.113.50\"\n", "http://203.0.113.50"},
		{"no WANIP omits the entry", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &failingDoer{t: t}
			d := discovery(false, networkCfg(t, tt.cfg), "https://wan.example.invalid")
			d.client = doer

			got := d.wanAccessURL()
			if tt.wantURL == "" && got != nil {
				t.Errorf("wanAccessURL() = %+v, want nil", got)
			}
			if tt.wantURL != "" && (got == nil || got.IPv4 != tt.wantURL) {
				t.Errorf("wanAccessURL() = %+v, want %s", got, tt.wantURL)
			}
			for _, u := range d.Collect().URLs {
				if tt.wantURL == "" && u.Name == "Remote Access (WAN)" {
					t.Errorf("Collect() included a WAN entry: %+v", u)
				}
			}
			if doer.calls.Load() != 0 {
				t.Errorf("made %d external requests with lookup disabled", doer.calls.Load())
			}
		})
	}
}

func TestWANLookupOnlyOnRequest(t *testing.T) {
	srv, hits := wanService(t, http.StatusOK, "203.0.113.10")

	d := discovery(true, networkCfg(t, ""), srv.URL)
	if hits.Load() != 0 {
		t.Fatalf("construction made %d requests, want 0", hits.Load())
	}
	d.Collect()
	if hits.Load() != 1 {
		t.Errorf("Collect made %d requests, want 1", hits.Load())
	}
}

func TestNewNetworkAccessDiscoveryDefaults(t *testing.T) {
	d := NewNetworkAccessDiscovery(true)
	if !d.WANLookupEnabled() {
		t.Error("WANLookupEnabled() = false, want true")
	}
	if len(d.wanServices) != 3 || d.wanServices[0] != "https://api.ipify.org" {
		t.Errorf("wanServices = %v, want the three default services starting with api.ipify.org", d.wanServices)
	}
	if d.networkCfgPath != "/boot/config/network.cfg" {
		t.Errorf("networkCfgPath = %q, want /boot/config/network.cfg", d.networkCfgPath)
	}
	if NewNetworkAccessDiscovery(false).WANLookupEnabled() {
		t.Error("WANLookupEnabled() = true for a disabled discovery")
	}
}

// stubDoer answers every request with resp and err.
type stubDoer struct {
	resp *http.Response
	err  error
}

func (s stubDoer) Do(*http.Request) (*http.Response, error) { return s.resp, s.err }

// stubBody is a response body whose Read and Close return the given errors.
type stubBody struct {
	data     string
	readErr  error
	closeErr error
	read     bool
}

func (b *stubBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	return copy(p, b.data), io.EOF
}

func (b *stubBody) Close() error { return b.closeErr }

func TestQueryWANServiceFailures(t *testing.T) {
	tests := []struct {
		name    string
		service string
		doer    httpDoer
		want    string
	}{
		{"malformed service URL", "://not a url", stubDoer{err: errors.New("unreachable")}, ""},
		{"client error", "https://wan.example.invalid", stubDoer{err: errors.New("dial failed")}, ""},
		{"body read error", "https://wan.example.invalid",
			stubDoer{resp: &http.Response{StatusCode: http.StatusOK, Body: &stubBody{readErr: errors.New("reset")}}}, ""},
		{"body close error still returns the IP", "https://wan.example.invalid",
			stubDoer{resp: &http.Response{StatusCode: http.StatusOK, Body: &stubBody{data: "203.0.113.60", closeErr: errors.New("close failed")}}},
			"203.0.113.60"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := discovery(true, networkCfg(t, ""), tt.service)
			d.client = tt.doer
			if got := d.queryWANService(tt.service); got != tt.want {
				t.Errorf("queryWANService() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfiguredWANIPRejectsInvalidValue(t *testing.T) {
	d := discovery(false, networkCfg(t, "WANIP=\"not-an-ip\"\nOTHER=1\n"))
	if got := d.configuredWANIP(); got != "" {
		t.Errorf("configuredWANIP() = %q, want empty for an invalid WANIP", got)
	}
}
