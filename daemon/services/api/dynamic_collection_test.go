package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// fakeDynamic is a collector manager in Dynamic mode. Its refresh installs
// the collector's primary topic in the server's cache, like a real run whose
// publication reaches the cache subscription. Concurrent requests for one
// collector share one run until release is closed.
type fakeDynamic struct {
	*mockCollectorManager
	dynamic bool
	demand  *scheduler.Demand
	server  *Server
	stopped bool
	err     error
	// block, when set, keeps refreshes from completing.
	block bool
	// noInstall makes the refresh publish without the data reaching the cache.
	noInstall bool
	// noPublish makes the refresh succeed without publishing anything.
	noPublish bool
	// release, when set, holds runs until it is closed.
	release chan struct{}

	mu       sync.Mutex
	inflight map[string][]chan error
	runs     atomic.Int32
}

func newFakeDynamic() *fakeDynamic {
	return &fakeDynamic{
		mockCollectorManager: newMockCollectorManager(),
		dynamic:              true,
		demand:               scheduler.NewDemand(scheduler.RealClock{}),
		inflight:             map[string][]chan error{},
	}
}

func (f *fakeDynamic) Dynamic() bool             { return f.dynamic }
func (f *fakeDynamic) Demand() *scheduler.Demand { return f.demand }

func (f *fakeDynamic) RequestRefresh(name string) (<-chan error, bool) {
	if f.stopped {
		return nil, false
	}
	ch := make(chan error, 1)
	if f.block {
		return ch, true
	}
	f.mu.Lock()
	waiters := append(f.inflight[name], ch)
	f.inflight[name] = waiters
	first := len(waiters) == 1
	f.mu.Unlock()
	if first {
		f.runs.Add(1)
		go f.run(name)
	}
	return ch, true
}

func (f *fakeDynamic) run(name string) {
	if f.release != nil {
		<-f.release
	}
	p, _ := scheduler.PolicyFor(name)
	if f.err == nil && !f.noPublish {
		// A collector publishes; the cache subscription then installs it.
		f.server.ctx.Hub.Pub(struct{}{}, p.Topics[0])
		if !f.noInstall {
			f.server.markInstalled(p.Topics[0], time.Now())
		}
	}
	f.mu.Lock()
	waiters := f.inflight[name]
	delete(f.inflight, name)
	f.mu.Unlock()
	for _, w := range waiters {
		w <- f.err
	}
}

func newDynamicServer(t *testing.T) (*Server, *fakeDynamic) {
	t.Helper()
	fake := newFakeDynamic()
	s := NewServerWithCollectorManager(&domain.Context{Hub: domain.NewEventBus(10)}, fake)
	fake.server = s
	return s, fake
}

// fakeWSDemand records WSAdd and WSRemove calls.
type fakeWSDemand struct {
	mu      sync.Mutex
	adds    []map[string]bool
	removes []map[string]bool
}

func (d *fakeWSDemand) WSAdd(t map[string]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.adds = append(d.adds, t)
}

func (d *fakeWSDemand) WSRemove(t map[string]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.removes = append(d.removes, t)
}

func (d *fakeWSDemand) counts() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.adds), len(d.removes)
}

// ===== Cache install tracking =====

func TestCacheInstalled(t *testing.T) {
	c := &CacheStore{}
	if gen, at := c.CacheInstalled("system_update"); gen != 0 || !at.IsZero() {
		t.Fatalf("zero state = %d, %v; want 0, zero time", gen, at)
	}
	t1 := time.Now()
	c.markInstalled("system_update", t1)
	c.markInstalled("system_update", t1.Add(time.Second))
	gen, at := c.CacheInstalled("system_update")
	if gen != 2 || !at.Equal(t1.Add(time.Second)) {
		t.Errorf("after two installs = %d, %v; want 2, %v", gen, at, t1.Add(time.Second))
	}
	if gen, _ := c.CacheInstalled("array_status_update"); gen != 0 {
		t.Errorf("other topic gen = %d, want 0", gen)
	}
}

func TestWaitInstalled(t *testing.T) {
	t.Run("returns true once the generation advances", func(t *testing.T) {
		c := &CacheStore{}
		c.markInstalled("system_update", time.Now())
		done := make(chan bool)
		go func() { done <- c.waitInstalled(t.Context(), "system_update", 1) }()
		c.markInstalled("system_update", time.Now())
		select {
		case ok := <-done:
			if !ok {
				t.Error("waitInstalled = false, want true")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("waitInstalled did not return")
		}
	})
	t.Run("returns true at once when already past", func(t *testing.T) {
		c := &CacheStore{}
		c.markInstalled("system_update", time.Now())
		if !c.waitInstalled(t.Context(), "system_update", 0) {
			t.Error("waitInstalled = false, want true")
		}
	})
	t.Run("returns false when ctx ends", func(t *testing.T) {
		c := &CacheStore{}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if c.waitInstalled(ctx, "system_update", 0) {
			t.Error("waitInstalled = true, want false")
		}
	})
}

func TestSubscribeToEvents_MarksInstalledTopic(t *testing.T) {
	hub := domain.NewEventBus(10)
	server := NewServer(&domain.Context{Hub: hub, Version: "test"})
	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	hub.Pub(&dto.ArrayStatus{State: "Started"}, constants.TopicArrayStatusUpdate.Name)
	eventually(t, func() bool {
		gen, _ := server.CacheInstalled(constants.TopicArrayStatusUpdate.Name)
		return gen == 1
	})
	if gen, _ := server.CacheInstalled(constants.TopicSystemUpdate.Name); gen != 0 {
		t.Errorf("system_update gen = %d, want 0", gen)
	}
}

// ===== EnsureFresh =====

func TestEnsureFresh(t *testing.T) {
	t.Run("fresh cache triggers no refresh", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		at := time.Now()
		s.markInstalled(constants.TopicSystemUpdate.Name, at)
		f := s.EnsureFresh(t.Context(), false, "system")
		if fake.runs.Load() != 0 {
			t.Errorf("runs = %d, want 0", fake.runs.Load())
		}
		if f.Stale || !f.UpdatedAt.Equal(at) {
			t.Errorf("freshness = %+v, want fresh at %v", f, at)
		}
	})

	t.Run("stale cache runs one refresh and waits for the install", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		old := time.Now().Add(-time.Hour)
		s.markInstalled(constants.TopicSystemUpdate.Name, old)
		f := s.EnsureFresh(t.Context(), false, "system")
		if fake.runs.Load() != 1 {
			t.Errorf("runs = %d, want 1", fake.runs.Load())
		}
		if f.Stale || !f.UpdatedAt.After(old) {
			t.Errorf("freshness = %+v, want fresh and newer than %v", f, old)
		}
	})

	t.Run("never installed counts as stale", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		f := s.EnsureFresh(t.Context(), false, "docker")
		if fake.runs.Load() != 1 || f.Stale || f.UpdatedAt.IsZero() {
			t.Errorf("runs = %d, freshness = %+v; want 1 run and fresh data", fake.runs.Load(), f)
		}
	})

	t.Run("concurrent stale reads share one refresh", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.release = make(chan struct{})
		var wg sync.WaitGroup
		results := make([]Freshness, 5)
		for i := range results {
			wg.Go(func() { results[i] = s.EnsureFresh(t.Context(), false, "system") })
		}
		eventually(t, func() bool {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			return len(fake.inflight["system"]) == len(results)
		})
		close(fake.release)
		wg.Wait()
		if fake.runs.Load() != 1 {
			t.Errorf("runs = %d, want 1", fake.runs.Load())
		}
		for i, r := range results {
			if r.Stale || r.UpdatedAt.IsZero() {
				t.Errorf("read %d = %+v, want fresh", i, r)
			}
		}
	})

	t.Run("refresh error returns stale cached data", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.err = errors.New("boom")
		old := time.Now().Add(-time.Hour)
		s.markInstalled(constants.TopicSystemUpdate.Name, old)
		f := s.EnsureFresh(t.Context(), false, "system")
		if !f.Stale || !f.UpdatedAt.Equal(old) {
			t.Errorf("freshness = %+v, want stale at %v", f, old)
		}
	})

	t.Run("refresh timeout returns stale cached data", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.block = true
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		if f := s.EnsureFresh(ctx, false, "system"); !f.Stale {
			t.Errorf("freshness = %+v, want stale", f)
		}
	})

	t.Run("data not installed before timeout is stale", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.noInstall = true
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		if f := s.EnsureFresh(ctx, false, "system"); !f.Stale {
			t.Errorf("freshness = %+v, want stale", f)
		}
	})

	t.Run("refresh that publishes nothing returns the cached data at once", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.noPublish = true
		old := time.Now().Add(-time.Hour)
		s.markInstalled(constants.TopicSystemUpdate.Name, old)
		f := s.EnsureFresh(t.Context(), false, "system")
		if fake.runs.Load() != 1 || f.Stale || !f.UpdatedAt.Equal(old) {
			t.Errorf("runs = %d, freshness = %+v; want 1 run, not stale, at %v", fake.runs.Load(), f, old)
		}
	})

	t.Run("a collector:topic source judges freshness on that topic", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		now, old := time.Now(), time.Now().Add(-time.Hour)
		s.markInstalled(constants.TopicZFSPoolsUpdate.Name, old)
		s.markInstalled(constants.TopicZFSDatasetsUpdate.Name, now)
		f := s.EnsureFresh(t.Context(), true, "zfs:"+constants.TopicZFSDatasetsUpdate.Name)
		if fake.runs.Load() != 0 || f.Stale || !f.UpdatedAt.Equal(now) {
			t.Errorf("fresh datasets: runs = %d, freshness = %+v; want no run, fresh at %v", fake.runs.Load(), f, now)
		}
		if !fake.demand.Active("zfs") {
			t.Error("a collector:topic read should record demand for the collector")
		}

		s.markInstalled(constants.TopicZFSDatasetsUpdate.Name, old)
		s.EnsureFresh(t.Context(), false, "zfs:"+constants.TopicZFSDatasetsUpdate.Name)
		if fake.runs.Load() != 1 {
			t.Errorf("stale datasets: runs = %d, want 1", fake.runs.Load())
		}
	})

	t.Run("VM guest data is never refreshed, only aged", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		guest := "vm:" + constants.TopicVMGuestUpdate.Name
		if f := s.EnsureFresh(t.Context(), true, guest); f.Stale {
			t.Errorf("no guest VMs configured: freshness = %+v, want not stale", f)
		}
		s.ctx.VMGuest.VMs = []string{"Home Assistant"}
		if f := s.EnsureFresh(t.Context(), true, guest); !f.Stale {
			t.Errorf("configured guest VMs without data yet: freshness = %+v, want stale", f)
		}
		recent, old := time.Now().Add(-10*time.Second), time.Now().Add(-2*time.Minute)
		s.markInstalled(constants.TopicVMGuestUpdate.Name, recent)
		if f := s.EnsureFresh(t.Context(), true, guest); f.Stale || !f.UpdatedAt.Equal(recent) {
			t.Errorf("recent guest data: freshness = %+v, want fresh at %v", f, recent)
		}
		s.markInstalled(constants.TopicVMGuestUpdate.Name, old)
		if f := s.EnsureFresh(t.Context(), true, guest); !f.Stale || !f.UpdatedAt.Equal(old) {
			t.Errorf("guest data older than two polls: freshness = %+v, want stale at %v", f, old)
		}
		s.ctx.VMGuest.Interval = 120 // two polls = 4 min
		if f := s.EnsureFresh(t.Context(), true, guest); f.Stale {
			t.Errorf("guest data within two configured polls: freshness = %+v, want fresh", f)
		}
		if fake.runs.Load() != 0 {
			t.Errorf("runs = %d, want 0: a VM refresh does not produce guest data", fake.runs.Load())
		}
		if !fake.demand.Active("vm") {
			t.Error("a guest read should record demand for the vm collector")
		}
	})

	t.Run("stopped collector is not stale", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		fake.stopped = true
		if f := s.EnsureFresh(t.Context(), false, "system"); f.Stale {
			t.Errorf("freshness = %+v, want not stale", f)
		}
	})

	t.Run("collectors without refresh on read and unknown names are skipped", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		f := s.EnsureFresh(t.Context(), false, "hardware", "no-such-collector")
		if fake.runs.Load() != 0 || f.Stale || !f.UpdatedAt.IsZero() {
			t.Errorf("runs = %d, freshness = %+v; want no run", fake.runs.Load(), f)
		}
	})

	t.Run("reports the oldest install time", func(t *testing.T) {
		s, _ := newDynamicServer(t)
		older, newer := time.Now().Add(-time.Second), time.Now()
		s.markInstalled(constants.TopicSystemUpdate.Name, newer)
		s.markInstalled(constants.TopicArrayStatusUpdate.Name, older)
		if f := s.EnsureFresh(t.Context(), false, "system", "array"); !f.UpdatedAt.Equal(older) {
			t.Errorf("UpdatedAt = %v, want %v", f.UpdatedAt, older)
		}
	})

	t.Run("records demand only when asked", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		s.markInstalled(constants.TopicSystemUpdate.Name, time.Now())
		s.markInstalled(constants.TopicArrayStatusUpdate.Name, time.Now())
		s.EnsureFresh(t.Context(), false, "system")
		if fake.demand.Active("system") {
			t.Error("recordDemand=false marked system active")
		}
		s.EnsureFresh(t.Context(), true, "array")
		if !fake.demand.Active("array") {
			t.Error("recordDemand=true did not mark array active")
		}
	})

	t.Run("manual mode never refreshes", func(t *testing.T) {
		s, _ := setupTestServer()
		if s.dynamic != nil {
			t.Fatal("manual server has a dynamic manager")
		}
		if f := s.EnsureFresh(t.Context(), true, "system"); f.Stale || !f.UpdatedAt.IsZero() {
			t.Errorf("freshness = %+v, want empty", f)
		}
	})
}

// ===== Freshness middleware =====

func TestFreshnessMiddleware(t *testing.T) {
	t.Run("sets the cache headers on a mapped GET route", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		at := time.Now().Add(-time.Hour)
		s.markInstalled(constants.TopicSystemUpdate.Name, at)
		fake.err = errors.New("boom")
		rr := httptest.NewRecorder()
		s.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
		if got := rr.Header().Get("X-Cache-Updated-At"); got != at.UTC().Format(time.RFC3339) {
			t.Errorf("X-Cache-Updated-At = %q, want %q", got, at.UTC().Format(time.RFC3339))
		}
		if got := rr.Header().Get("X-Cache-Stale"); got != "true" {
			t.Errorf("X-Cache-Stale = %q, want true", got)
		}
		if !fake.demand.Active("system") {
			t.Error("REST read did not record demand")
		}
	})

	t.Run("fresh data sets no stale header", func(t *testing.T) {
		s, _ := newDynamicServer(t)
		s.markInstalled(constants.TopicSystemUpdate.Name, time.Now())
		rr := httptest.NewRecorder()
		s.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
		if rr.Header().Get("X-Cache-Stale") != "" || rr.Header().Get("X-Cache-Updated-At") == "" {
			t.Errorf("headers = %v, want Updated-At and no Stale", rr.Header())
		}
	})

	t.Run("metrics scrape does not record demand", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		rr := httptest.NewRecorder()
		s.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if fake.demand.Active("system") {
			t.Error("/metrics recorded demand")
		}
		if fake.runs.Load() == 0 {
			t.Error("/metrics with stale data ran no refresh")
		}
	})

	t.Run("unmapped GET and non-GET routes pass through", func(t *testing.T) {
		s, fake := newDynamicServer(t)
		for _, req := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/api/v1/collectors/status", nil),
			httptest.NewRequest(http.MethodPost, "/api/v1/collectors/system/enable", nil),
		} {
			rr := httptest.NewRecorder()
			s.router.ServeHTTP(rr, req)
			if rr.Header().Get("X-Cache-Updated-At") != "" || rr.Header().Get("X-Cache-Stale") != "" {
				t.Errorf("%s %s set cache headers", req.Method, req.URL.Path)
			}
		}
		if fake.runs.Load() != 0 {
			t.Errorf("runs = %d, want 0", fake.runs.Load())
		}
	})

	t.Run("request without a route passes through", func(t *testing.T) {
		s, _ := newDynamicServer(t)
		called := false
		h := s.freshnessMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
		if !called {
			t.Error("next handler not called")
		}
	})
}

// TestRouteCollectorsCoverGETRoutes makes every GET route either map to the
// collectors it serves or be listed as serving none.
func TestRouteCollectorsCoverGETRoutes(t *testing.T) {
	s, _ := setupTestServer()
	routes := map[string]bool{}
	err := s.router.Walk(func(r *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, err := r.GetPathTemplate()
		if err != nil {
			return nil
		}
		routes[tmpl] = true
		methods, _ := r.GetMethods()
		for _, m := range methods {
			if m != http.MethodGet {
				continue
			}
			_, mapped := routeCollectors[tmpl]
			if mapped == routesWithoutCollectors[tmpl] {
				t.Errorf("GET %s must be in exactly one of routeCollectors and routesWithoutCollectors", tmpl)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for tmpl, names := range routeCollectors {
		if !routes[tmpl] {
			t.Errorf("routeCollectors has unknown route %s", tmpl)
		}
		for _, src := range names {
			if err := checkSource(src); err != nil {
				t.Errorf("route %s: %v", tmpl, err)
			}
		}
	}
	for tmpl := range routesWithoutCollectors {
		if !routes[tmpl] {
			t.Errorf("routesWithoutCollectors has unknown route %s", tmpl)
		}
	}
}

// ===== Server wiring =====

func TestNewServerWithCollectorManager_DynamicWiring(t *testing.T) {
	fake := newFakeDynamic()
	s := NewServerWithCollectorManager(&domain.Context{}, fake)
	if s.dynamic == nil || s.wsHub.demand == nil {
		t.Error("dynamic manager not wired")
	}

	fake = newFakeDynamic()
	fake.dynamic = false
	s = NewServerWithCollectorManager(&domain.Context{}, fake)
	if s.dynamic != nil || s.wsHub.demand != nil {
		t.Error("manual-mode manager wired as dynamic")
	}

	s = NewServerWithCollectorManager(&domain.Context{}, newMockCollectorManager())
	if s.dynamic != nil || s.wsHub.demand != nil {
		t.Error("manager without the dynamic contract wired as dynamic")
	}
}

// ===== WebSocket demand tracking =====

func runHubWithDemand(t *testing.T) (*WSHub, *fakeWSDemand, context.CancelFunc, chan struct{}) {
	t.Helper()
	hub := NewWSHub()
	d := &fakeWSDemand{}
	hub.demand = d
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return hub, d, cancel, done
}

func TestWSDemand_RegisterAndUnregister(t *testing.T) {
	hub, d, _, _ := runHubWithDemand(t)
	c := &WSClient{hub: hub, send: make(chan dto.WSEvent, 1)}
	hub.register <- c
	eventually(t, func() bool { a, _ := d.counts(); return a == 1 })
	if d.adds[0] != nil {
		t.Errorf("new client filter = %v, want nil (all topics)", d.adds[0])
	}
	hub.unregister <- c
	eventually(t, func() bool { _, r := d.counts(); return r == 1 })
	// A second untrack is a no-op.
	c.untrack()
	if _, r := d.counts(); r != 1 {
		t.Errorf("removes = %d, want 1", r)
	}
}

func TestWSDemand_SlowClientEvicted(t *testing.T) {
	hub, d, _, _ := runHubWithDemand(t)
	c := &WSClient{hub: hub, send: make(chan dto.WSEvent)} // unbuffered: always full
	hub.register <- c
	eventually(t, func() bool { a, _ := d.counts(); return a == 1 })
	hub.Broadcast("system_update", "x")
	eventually(t, func() bool { _, r := d.counts(); return r == 1 })
}

func TestWSDemand_HubShutdown(t *testing.T) {
	hub, d, cancel, done := runHubWithDemand(t)
	c := &WSClient{hub: hub, send: make(chan dto.WSEvent, 1)}
	hub.register <- c
	eventually(t, func() bool { a, _ := d.counts(); return a == 1 })
	cancel()
	<-done
	if _, r := d.counts(); r != 1 {
		t.Errorf("removes after shutdown = %d, want 1", r)
	}
}

func TestWSDemand_SetTopics(t *testing.T) {
	d := &fakeWSDemand{}
	hub := NewWSHub()
	hub.demand = d
	c := &WSClient{hub: hub}

	c.setTopics([]string{"system_update"})
	if a, r := d.counts(); a != 0 || r != 0 {
		t.Fatalf("untracked client recorded %d adds, %d removes", a, r)
	}

	c.track()
	c.track() // already tracked: no second add
	c.setTopics([]string{"array_status_update"})
	if a, r := d.counts(); a != 2 || r != 1 {
		t.Fatalf("adds, removes = %d, %d; want 2, 1", a, r)
	}
	if !d.removes[0]["system_update"] || !d.adds[1]["array_status_update"] {
		t.Errorf("removed %v, added %v", d.removes[0], d.adds[1])
	}
	c.setTopics(nil)
	if d.adds[2] != nil {
		t.Errorf("reset filter recorded %v, want nil", d.adds[2])
	}
}

func TestWSDemand_NoRecorder(t *testing.T) {
	c := &WSClient{hub: NewWSHub()}
	c.track()
	c.setTopics([]string{"system_update"})
	c.untrack()
	if c.tracked {
		t.Error("client tracked without a demand recorder")
	}
	(&WSClient{}).track() // no hub
}

// ===== Collector handlers =====

func TestHandleCollectorInterval_DynamicModeConflict(t *testing.T) {
	server, mock := setupTestServerWithCollectorManager()
	mock.intervalErr = domain.ErrDynamicCollectionMode
	body, _ := json.Marshal(dto.CollectorIntervalRequest{Interval: 30})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/collectors/system/interval", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorsStatus_LegacyFallbackMode(t *testing.T) {
	for _, mode := range []string{domain.CollectionModeManual, domain.CollectionModeDynamic} {
		t.Run(mode, func(t *testing.T) {
			server, _ := setupTestServer()
			server.ctx.CollectionMode = mode
			server.ctx.Intervals.System = 15 // one running collector
			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/collectors/status", nil))
			var status dto.CollectorsStatusResponse
			if err := json.NewDecoder(rr.Body).Decode(&status); err != nil {
				t.Fatal(err)
			}
			if status.Mode != mode {
				t.Errorf("mode = %q, want %q", status.Mode, mode)
			}
			running := 0
			for _, c := range status.Collectors {
				if c.Status == "running" {
					running++
				}
				if c.Mode != mode {
					t.Errorf("%s mode = %q, want %q", c.Name, c.Mode, mode)
				}
				// Only Manual mode knows the schedule: running collectors report it.
				wantEff, wantReason := 0, ""
				if mode == domain.CollectionModeManual && c.Status == "running" {
					wantEff, wantReason = c.Interval, "manual"
				}
				if c.EffectiveInterval != wantEff || c.Reason != wantReason {
					t.Errorf("%s effective/reason = %d/%q, want %d/%q", c.Name, c.EffectiveInterval, c.Reason, wantEff, wantReason)
				}
			}
			if running != 1 {
				t.Errorf("running collectors = %d, want 1", running)
			}
		})
	}
}

// routesWithoutCollectors are the GET routes that serve no cached collector
// data, so reads neither refresh nor record demand.
var routesWithoutCollectors = map[string]bool{
	"/api/v1/health":                      true,
	"/api/v1/diagnostics/self-test":       true,
	"/api/v1/capabilities":                true,
	"/api/v1/diagnostics/bundle":          true,
	"/api/v1/docker/port-conflicts":       true,
	"/api/v1/docker/{id}/check-update":    true,
	"/api/v1/docker/{id}/size":            true,
	"/api/v1/docker/{id}/logs":            true,
	"/api/v1/system/flash":                true,
	"/api/v1/network/access-urls":         true,
	"/api/v1/vm/{name}/snapshots":         true,
	"/api/v1/array/parity-check/history":  true,
	"/api/v1/array/parity-check/schedule": true,
	"/api/v1/shares/{name}/config":        true,
	"/api/v1/network/{interface}/config":  true,
	"/api/v1/settings/system":             true,
	"/api/v1/settings/docker":             true,
	"/api/v1/settings/vm":                 true,
	"/api/v1/settings/disks":              true,
	"/api/v1/settings/disk-thresholds":    true,
	"/api/v1/settings/mover":              true,
	"/api/v1/settings/services":           true,
	"/api/v1/settings/network-services":   true,
	"/api/v1/plugins":                     true,
	"/api/v1/user-scripts":                true,
	"/api/v1/logs":                        true,
	"/api/v1/logs/{filename}":             true,
	"/api/v1/collectors/status":           true,
	"/api/v1/collectors/{name}":           true,
	"/api/v1/mqtt/status":                 true,
	"/api/v1/services":                    true,
	"/api/v1/processes":                   true,
	"/api/v1/processes/io":                true,
	"/api/v1/healthchecks":                true,
	"/api/v1/healthchecks/status":         true,
	"/api/v1/healthchecks/history":        true,
	"/api/v1/healthchecks/{id}":           true,
	"/api/v1/metrics/history":             true,
	"/api/v1/alerts/templates":            true,
	"/api/v1/alerts/rules":                true,
	"/api/v1/alerts/rules/{id}":           true,
	"/api/v1/alerts/status":               true,
	"/api/v1/alerts/history":              true,
	"/api/v1/alerts/firing":               true,
	"/api/v1/agent/sessions":              true,
	"/api/v1/agent/sessions/{id}":         true,
	"/api/v1/agent/memory":                true,
	"/api/v1/fans/sensors":                true,
	"/api/v1/mcp/tool-policy":             true,
	"/api/v1/ws":                          true,
}

// checkSource reports an EnsureFresh source whose collector has no policy or
// does not publish the named topic.
func checkSource(src string) error {
	name, topic, _ := strings.Cut(src, ":")
	p, ok := scheduler.PolicyFor(name)
	if !ok {
		return fmt.Errorf("collector %q has no policy", name)
	}
	if topic != "" && !slices.Contains(p.Topics, topic) {
		return fmt.Errorf("collector %s does not publish %s", name, topic)
	}
	return nil
}

func TestPublishCountWithoutHub(t *testing.T) {
	for _, s := range []*Server{{}, {ctx: &domain.Context{}}} {
		if got := s.publishCount(constants.TopicSystemUpdate.Name); got != 0 {
			t.Errorf("publishCount = %d, want 0 without an event bus", got)
		}
	}
}
