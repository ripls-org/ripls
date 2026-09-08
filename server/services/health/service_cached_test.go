package health

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	healthpkg "go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// countingCheck returns a CheckFunc that counts invocations and returns the
// given statuses.
func countingCheck(calls *atomic.Int64, statuses []*healthpkg.Status) CheckFunc {
	return func(ctx context.Context) ([]*healthpkg.Status, error) {
		calls.Add(1)
		return statuses, nil
	}
}

func TestRegisterFuncCached_ReusesResultWithinTTL(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	var calls atomic.Int64
	svc.RegisterFuncCached("google_maps", time.Hour, countingCheck(&calls, []*healthpkg.Status{
		{Name: "location", Backend: "google_maps", LatencyMs: 42, Metadata: map[string]string{"probe": "geocode"}},
	}))

	first, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	second, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 upstream probe, got %d", got)
	}

	// The fresh result is not stale-stamped; the cached one is, and it keeps
	// the real probe's latency and metadata.
	if _, ok := first.Msg.Dependencies[0].Details["cached_age_ms"]; ok {
		t.Error("fresh result should not carry cached_age_ms")
	}
	cached := second.Msg.Dependencies[0]
	if _, ok := cached.Details["cached_age_ms"]; !ok {
		t.Error("cached result should carry cached_age_ms")
	}
	if cached.LatencyMs != 42 {
		t.Errorf("cached result should keep probe latency 42, got %d", cached.LatencyMs)
	}
	if cached.Details["probe"] != "geocode" {
		t.Errorf("cached result should keep metadata, got %v", cached.Details)
	}
	if !cached.Healthy {
		t.Error("cached result should stay healthy")
	}
}

func TestRegisterFuncCached_RefreshesAfterTTL(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	var calls atomic.Int64
	svc.RegisterFuncCached("google_maps", 10*time.Millisecond, countingCheck(&calls, []*healthpkg.Status{
		{Name: "location", Backend: "google_maps"},
	}))

	ctx := context.Background()
	if _, err := svc.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{})); err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	//nolint:forbidigo // The assertion IS that a real TTL expires: cachedChecker
	// compares fetchedAt against time.Now() (service.go:121) and the health
	// service has no injectable clock, so wall-clock time genuinely has to pass.
	// 20ms against a 10ms TTL. Clock injection here would be the better fix and
	// would let this sleep go.
	time.Sleep(20 * time.Millisecond)
	if _, err := svc.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{})); err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("expected a fresh probe after TTL expiry, got %d probes", got)
	}
}

func TestRegisterFuncCached_CachesFailures(t *testing.T) {
	// A failing paid upstream must not be re-probed on every request — the
	// error result is cached for the TTL like a success, and still reported
	// unhealthy so alerting sees it on every probe.
	logger := logging.Default()
	svc := New(logger)

	var calls atomic.Int64
	svc.RegisterFuncCached("google_maps", time.Hour, countingCheck(&calls, []*healthpkg.Status{
		{Name: "location", Backend: "google_maps", Error: "status 403"},
	}))

	ctx := context.Background()
	for range 3 {
		resp, err := svc.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{}))
		if err != nil {
			t.Fatalf("CheckHealth returned error: %v", err)
		}
		if resp.Msg.Healthy {
			t.Error("expected overall healthy=false while dependency fails")
		}
		if resp.Msg.Dependencies[0].Error != "status 403" {
			t.Errorf("expected cached error to surface, got %q", resp.Msg.Dependencies[0].Error)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("expected the failing upstream probed once, got %d", got)
	}
}

func TestRegisterFuncCached_SingleProbeUnderConcurrency(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	// The probe blocks on `release` instead of sleeping. A fixed sleep would be
	// racing the scheduler: if the 5ms probe finished before the 8th goroutine
	// reached CheckHealth, that goroutine would start a second probe and the
	// assertion below would see calls == 2. Holding the probe open until every
	// caller is in flight removes the wall-clock dependency entirely (#1364).
	var calls atomic.Int64
	release := make(chan struct{})
	slowCheck := func(ctx context.Context) ([]*healthpkg.Status, error) {
		calls.Add(1)
		<-release
		return []*healthpkg.Status{{Name: "location", Backend: "google_maps"}}, nil
	}
	svc.RegisterFuncCached("google_maps", time.Hour, slowCheck)

	var wg sync.WaitGroup
	var started sync.WaitGroup
	started.Add(8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			if _, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{})); err != nil {
				t.Errorf("CheckHealth returned error: %v", err)
			}
		}()
	}
	// All 8 callers are running; the in-flight probe is parked on `release`, so
	// they are queued behind the single-flight barrier rather than starting
	// probes of their own.
	started.Wait()
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("expected concurrent probes to share one upstream call, got %d", got)
	}
}

func TestRegisterFuncCached_CachedMetadataIsIsolated(t *testing.T) {
	// Each request gets its own Metadata copy: a consumer mutating one
	// response must not leak into the cache or later responses.
	var calls atomic.Int64
	checker := &cachedChecker{
		inner: &funcChecker{name: "google_maps", fn: countingCheck(&calls, []*healthpkg.Status{
			{Name: "location", Backend: "google_maps", Metadata: map[string]string{"probe": "geocode"}},
		})},
		ttl: time.Hour,
	}

	ctx := context.Background()
	if _, err := checker.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	hit1, err := checker.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	hit1[0].Metadata["probe"] = "tampered"

	hit2, err := checker.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	if hit2[0].Metadata["probe"] != "geocode" {
		t.Errorf("cache metadata leaked across requests: %v", hit2[0].Metadata)
	}
}

func TestRegisterFunc_RemainsUncached(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	var calls atomic.Int64
	svc.RegisterFunc("database", countingCheck(&calls, []*healthpkg.Status{
		{Name: "database", Backend: "postgresql"},
	}))

	ctx := context.Background()
	for range 3 {
		if _, err := svc.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{})); err != nil {
			t.Fatalf("CheckHealth returned error: %v", err)
		}
	}

	if got := calls.Load(); got != 3 {
		t.Errorf("expected uncached checker probed every time, got %d probes", got)
	}
}
