package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

type recordingCollector struct {
	open    func(context.Context) error
	refresh func(context.Context) error
	close   func()
}

func (c recordingCollector) Open(ctx context.Context) error    { return c.open(ctx) }
func (c recordingCollector) Refresh(ctx context.Context) error { return c.refresh(ctx) }
func (c recordingCollector) Close()                            { c.close() }

func TestCollectorReplacementWaitsForPreviousGeneration(t *testing.T) {
	for _, mode := range []string{domain.CollectionModeManual, domain.CollectionModeDynamic} {
		t.Run(mode, func(t *testing.T) {
			app := createTestContext()
			app.CollectionMode = mode
			var wg sync.WaitGroup
			cm := NewCollectorManager(app, &wg)
			cm.eventSources = nil
			entered, cancelled := make(chan struct{}), make(chan struct{})
			closing, releaseClose := make(chan struct{}), make(chan struct{})
			releaseRefresh := make(chan struct{})
			created := make(chan *mockCollector, 2)
			release := sync.OnceFunc(func() { close(releaseClose) })
			unblock := sync.OnceFunc(func() { close(releaseRefresh) })
			t.Cleanup(func() {
				unblock()
				release()
				cm.StopAll()
				wg.Wait()
			})
			first := recordingCollector{
				open: func(context.Context) error { return nil },
				refresh: func(ctx context.Context) error {
					close(entered)
					<-ctx.Done()
					close(cancelled)
					<-releaseRefresh
					return ctx.Err()
				},
				close: func() {
					close(closing)
					<-releaseClose
				},
			}
			factory := func(*domain.Context) Collector {
				select {
				case <-entered:
					next := newMockCollector()
					created <- next
					return next
				default:
					return first
				}
			}
			cm.Register("docker", factory, 30, false)
			enable := func() {
				t.Helper()
				if err := cm.EnableCollector("docker"); err != nil {
					t.Fatal(err)
				}
			}
			enable()
			waitChan(t, entered, "first refresh")
			if mode == domain.CollectionModeManual {
				if err := cm.UpdateInterval("docker", 120); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := cm.DisableCollector("docker"); err != nil {
					t.Fatal(err)
				}
				enable()
			}
			waitChan(t, cancelled, "cancellation")
			for range 2 {
				if err := cm.DisableCollector("docker"); err != nil {
					t.Fatal(err)
				}
				if st := mustStatus(t, cm, "docker"); st.Enabled || st.Status != "stopped" {
					t.Fatalf("disabled status = %+v", st)
				}
				enable()
			}
			assertNoReplacement := func() {
				t.Helper()
				select {
				case <-created:
					t.Fatal("replacement constructed before previous generation exited")
				default:
				}
			}
			assertNoReplacement()
			unblock()
			waitChan(t, closing, "close")
			assertNoReplacement()
			release()
			next := waitChan(t, created, "replacement generation")
			next.waitStarted(t)
			if mode == domain.CollectionModeManual && mustStatus(t, cm, "docker").EffectiveInterval != 120 {
				t.Fatal("replacement lost the latest interval")
			}
		})
	}
}

func TestCollectorShutdownReportsPendingCleanup(t *testing.T) {
	var wg sync.WaitGroup
	cm := NewCollectorManager(createTestContext(), &wg)
	opened := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(func() {
		unblock()
		cm.StopAll()
		wg.Wait()
	})
	cm.Register("test", func(*domain.Context) Collector {
		return recordingCollector{
			open:    func(context.Context) error { close(opened); return nil },
			refresh: func(context.Context) error { return nil },
			close:   func() { <-release },
		}
	}, 30, false)
	cm.StartAll()
	waitChan(t, opened, "Open")
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	if got := cm.Shutdown(expired); len(got) != 1 || got[0] != "test" {
		t.Fatalf("expected outstanding cleanup, got %v", got)
	}
	if err := cm.EnableCollector("test"); !errors.Is(err, errRunnerStopped) {
		t.Fatalf("EnableCollector after shutdown: %v", err)
	}
	if got := cm.StartAll(); got != 0 {
		t.Fatalf("StartAll after shutdown reported %d", got)
	}
	unblock()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if got := cm.Shutdown(ctx); len(got) != 0 {
		t.Fatalf("cleanup did not complete: %v", got)
	}
}

func TestCollectorOpenFailureCleansUpBothModes(t *testing.T) {
	for _, mode := range []string{domain.CollectionModeManual, domain.CollectionModeDynamic} {
		for _, panics := range []bool{false, true} {
			label := "error"
			if panics {
				label = "panic"
			}
			t.Run(mode+"/"+label, func(t *testing.T) {
				app := createTestContext()
				app.CollectionMode = mode
				var wg sync.WaitGroup
				cm := NewCollectorManager(app, &wg)
				closed := make(chan struct{})
				refreshes, closes := 0, 0
				cm.Register("docker", func(*domain.Context) Collector {
					return recordingCollector{
						open: func(context.Context) error {
							if panics {
								panic("open fault")
							}
							return errors.New("open fault")
						},
						refresh: func(context.Context) error { refreshes++; return nil },
						close:   func() { closes++; close(closed) },
					}
				}, 10, false)
				cm.StartAll()
				waitChan(t, closed, "cleanup after failed Open")
				wg.Wait()
				if refreshes != 0 || closes != 1 || mustStatus(t, cm, "docker").Status != "stopped" {
					t.Fatalf("refreshes=%d closes=%d status=%+v", refreshes, closes, mustStatus(t, cm, "docker"))
				}
				if err := cm.DisableCollector("docker"); err != nil {
					t.Fatal(err)
				}
				if st := mustStatus(t, cm, "docker"); st.Enabled || cm.StartAll() != 0 {
					t.Fatalf("disabled collector remains eligible for restart: %+v", st)
				}

			})
		}
	}
}

func TestManualCollectorStaggerCadenceAndPanicRecovery(t *testing.T) {
	var wg sync.WaitGroup
	cm := NewCollectorManager(createTestContext(), &wg)
	clock := scheduler.NewFakeClock(runnerEpoch)
	cm.clock = clock
	runs := make(chan time.Time, 4)
	cm.Register("nic", func(*domain.Context) Collector {
		count := 0
		return recordingCollector{
			open: func(context.Context) error { return nil },
			refresh: func(context.Context) error {
				count++
				runs <- clock.Now()
				if count == 1 {
					panic("refresh fault")
				}
				return nil
			},
			close: func() {},
		}
	}, 15, false)
	t.Cleanup(func() { cm.StopAll(); wg.Wait() })
	cm.StartAll()
	eventually(t, "startup timer", func() bool {
		p := clock.Pending()
		return len(p) == 1 && p[0] == 10*time.Second
	})
	clock.Advance(9 * time.Second)
	select {
	case <-runs:
		t.Fatal("refresh ran before the stagger elapsed")
	default:
	}
	clock.Advance(time.Second)
	if got := waitChan(t, runs, "initial refresh"); !got.Equal(runnerEpoch.Add(10 * time.Second)) {
		t.Fatal(got)
	}
	eventually(t, "poll timer", func() bool {
		p := clock.Pending()
		return len(p) == 1 && p[0] == 15*time.Second
	})
	clock.Advance(15 * time.Second)
	if got := waitChan(t, runs, "poll after a refresh panic"); !got.Equal(runnerEpoch.Add(25 * time.Second)) {
		t.Fatal(got)
	}
}

func TestManualCollectorEventKeepsCadence(t *testing.T) {
	clock := scheduler.NewFakeClock(runnerEpoch)
	coll := newFakeDynamic()
	r := newCollectorRunner("notification", scheduler.Policy{}, coll, clock)
	fire := make(chan func(), 1)
	sourceStopped := make(chan struct{})
	r.sources = []eventSource{eventSourceFunc(func(ctx context.Context, trigger func()) {
		fire <- trigger
		<-ctx.Done()
		close(sourceStopped)
	})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.runManual(ctx, 20*time.Second)
	}()
	t.Cleanup(func() { cancel(); <-done })
	trigger := waitChan(t, fire, "source start")
	coll.waitCall(t)
	eventually(t, "poll timer", func() bool {
		pending := clock.Pending()
		return len(pending) == 1 && pending[0] == 20*time.Second
	})
	clock.Advance(7 * time.Second)
	trigger()
	coll.waitCall(t)
	if pending := clock.Pending(); len(pending) != 1 || pending[0] != 13*time.Second {
		t.Fatalf("manual event moved the periodic deadline: %v", pending)
	}
	clock.Advance(13 * time.Second)
	coll.waitCall(t)
	cancel()
	waitChan(t, done, "collector shutdown")
	waitChan(t, sourceStopped, "source shutdown")
}

func TestCollectorDisableDuringInitialStagger(t *testing.T) {
	var wg sync.WaitGroup
	cm := NewCollectorManager(createTestContext(), &wg)
	clock := scheduler.NewFakeClock(runnerEpoch)
	cm.clock = clock
	closed := make(chan struct{})
	runs := make(chan struct{}, 1)
	cm.Register("nic", func(*domain.Context) Collector {
		return recordingCollector{
			open:    func(context.Context) error { return nil },
			refresh: func(context.Context) error { runs <- struct{}{}; return nil },
			close:   func() { close(closed) },
		}
	}, 10, false)
	t.Cleanup(func() { cm.StopAll(); wg.Wait() })
	cm.StartAll()
	eventually(t, "startup timer", func() bool { return len(clock.Pending()) == 1 })
	// A new registration cannot replace a generation while it owns the slot.
	cm.Register("nic", func(*domain.Context) Collector { return newMockCollector() }, 30, false)
	if st := mustStatus(t, cm, "nic"); st.Interval != 10 || st.Status != "running" {
		t.Fatalf("active registration replaced: %+v", st)
	}
	if err := cm.DisableCollector("nic"); err != nil {
		t.Fatal(err)
	}
	waitChan(t, closed, "cleanup during stagger")
	select {
	case <-runs:
		t.Fatal("refresh ran during the stagger")
	default:
	}
}

func TestCollectorCancellationDuringOpen(t *testing.T) {
	var wg sync.WaitGroup
	cm := NewCollectorManager(createTestContext(), &wg)
	opening, closed := make(chan struct{}), make(chan struct{})
	refreshes := 0
	cm.Register("test", func(*domain.Context) Collector {
		return recordingCollector{
			open: func(ctx context.Context) error {
				close(opening)
				<-ctx.Done()
				return nil
			},
			refresh: func(context.Context) error { refreshes++; return nil },
			close:   func() { close(closed) },
		}
	}, 30, false)
	t.Cleanup(func() { cm.StopAll(); wg.Wait() })
	cm.StartAll()
	waitChan(t, opening, "Open")
	if err := cm.DisableCollector("test"); err != nil {
		t.Fatal(err)
	}
	waitChan(t, closed, "Close after cancelled Open")
	if refreshes != 0 {
		t.Fatalf("refresh ran after cancellation: %d", refreshes)
	}
}

func TestRunnerCancelledBeforeOpenFailsPendingRequest(t *testing.T) {
	coll := newFakeDynamic()
	r := newCollectorRunner("system", scheduler.Policy{}, coll, scheduler.RealClock{})
	result := r.RequestRefresh()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r.run(ctx)
	if got := waitChan(t, result, "cancelled request"); !errors.Is(got, errRunnerStopped) {
		t.Fatalf("pending request: %v", got)
	}
}

func TestStorageTopologyRetainsStalledCommandAcrossRestart(t *testing.T) {
	var wg sync.WaitGroup
	app := createTestContext()
	cm := NewCollectorManager(app, &wg)
	clock := scheduler.NewFakeClock(runnerEpoch)
	cm.clock = clock
	commandStarted := make(chan struct{})
	commandReturned := make(chan struct{})
	reaped := make(chan struct{})
	release := sync.OnceFunc(func() { close(reaped) })
	replacement := make(chan struct{}, 1)
	sysRoot := t.TempDir()
	t.Cleanup(func() { release(); cm.StopAll(); wg.Wait() })
	factory := func(ctx *domain.Context) Collector {
		select {
		case <-commandStarted:
			replacement <- struct{}{}
			return newMockCollector()
		default:
		}
		c := collectors.NewStorageTopologyCollector(ctx)
		c.SysfsRoot = sysRoot
		c.StorcliCandidates = []string{"/synthetic/storcli"}
		c.IsExecutable = func(path string) bool { return path == c.StorcliCandidates[0] }
		c.RunFn = func(ctx context.Context, _ time.Duration, _ string, _ ...string) (string, <-chan struct{}, error) {
			close(commandStarted)
			<-ctx.Done()
			close(commandReturned)
			return "", reaped, ctx.Err()
		}
		return c
	}
	cm.Register("storage_topology", factory, 30, false)
	cm.StartAll()
	eventually(t, "storage stagger", func() bool { return len(clock.Pending()) == 1 })
	clock.Advance(20 * time.Second)
	waitChan(t, commandStarted, "hardware command")
	if err := cm.UpdateInterval("storage_topology", 120); err != nil {
		t.Fatal(err)
	}
	waitChan(t, commandReturned, "cancelled command returned")
	select {
	case <-replacement:
		t.Fatal("replacement started while hardware command was still alive")
	case <-time.After(25 * time.Millisecond):
	}
	release()
	waitChan(t, replacement, "replacement after command exit")
}

func TestManualArrayReportsActualCollectionTime(t *testing.T) {
	var wg sync.WaitGroup
	cm := NewCollectorManager(createTestContext(), &wg)
	clock := scheduler.NewFakeClock(runnerEpoch)
	cm.clock = clock
	runs := make(chan struct{}, 2)
	cm.Register("array", func(*domain.Context) Collector {
		return collectOnce(func() { runs <- struct{}{} })
	}, 86400, false)
	t.Cleanup(func() { cm.StopAll(); wg.Wait() })
	cm.StartAll()
	waitChan(t, runs, "initial array collection")
	eventually(t, "Manual array poll timer", func() bool {
		pending := clock.Pending()
		return len(pending) == 1 && pending[0] == 24*time.Hour
	})
	clock.Advance(24 * time.Hour)
	waitChan(t, runs, "next array collection")
	got := mustStatus(t, cm, "array")
	if got.LastRun == nil || !got.LastRun.Equal(runnerEpoch.Add(24*time.Hour)) || got.EffectiveInterval != 86400 {
		t.Fatalf("Manual array status shows the wrong collection time or interval: %+v", got)
	}
}
