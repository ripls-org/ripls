package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	healthpkg "go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// mockChecker is a test implementation of Checker.
type mockChecker struct {
	name     string
	statuses []*healthpkg.Status
	err      error
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) CheckHealth(ctx context.Context) ([]*healthpkg.Status, error) {
	return m.statuses, m.err
}

func TestCheckHealth_AllHealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	// Register healthy checkers
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql", Metadata: map[string]string{"host": "localhost"}}},
	})
	svc.Register(&mockChecker{
		name:     "storage",
		statuses: []*healthpkg.Status{{Name: "storage", Backend: "gcs", Metadata: map[string]string{"bucket": "test-bucket"}}},
	})

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if !resp.Msg.Healthy {
		t.Error("expected healthy=true when all checkers healthy")
	}

	if len(resp.Msg.Dependencies) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(resp.Msg.Dependencies))
	}

	// Verify database dependency
	db := resp.Msg.Dependencies[0]
	if db.Name != "database" {
		t.Errorf("expected name=database, got %s", db.Name)
	}
	if !db.Healthy {
		t.Error("expected database to be healthy")
	}
	if db.Backend != "postgresql" {
		t.Errorf("expected backend=postgresql, got %s", db.Backend)
	}
	if db.Details["host"] != "localhost" {
		t.Errorf("expected details host=localhost, got %s", db.Details["host"])
	}

	// Verify storage dependency
	storage := resp.Msg.Dependencies[1]
	if storage.Name != "storage" {
		t.Errorf("expected name=storage, got %s", storage.Name)
	}
	if !storage.Healthy {
		t.Error("expected storage to be healthy")
	}
	if storage.Backend != "gcs" {
		t.Errorf("expected backend=gcs, got %s", storage.Backend)
	}
}

func TestCheckHealth_OneUnhealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql"}},
	})
	svc.Register(&mockChecker{
		name:     "email",
		statuses: []*healthpkg.Status{{Name: "email", Backend: "mailgun", Error: "connection refused"}},
	})

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if resp.Msg.Healthy {
		t.Error("expected healthy=false when one checker fails")
	}

	// Find the email dependency
	var emailDep *api.DependencyStatus
	for _, dep := range resp.Msg.Dependencies {
		if dep.Name == "email" {
			emailDep = dep
			break
		}
	}

	if emailDep == nil {
		t.Fatal("expected to find email dependency")
	}
	if emailDep.Healthy {
		t.Error("expected email to be unhealthy")
	}
	if emailDep.Error != "connection refused" {
		t.Errorf("expected error message, got %s", emailDep.Error)
	}
	if emailDep.Backend != "mailgun" {
		t.Errorf("expected backend=mailgun, got %s", emailDep.Backend)
	}
}

func TestCheckHealth_NoCheckers(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if !resp.Msg.Healthy {
		t.Error("expected healthy=true with no checkers")
	}
	if len(resp.Msg.Dependencies) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(resp.Msg.Dependencies))
	}
	if resp.Msg.Version != "dev" {
		t.Errorf("expected version=dev, got %s", resp.Msg.Version)
	}
}

func TestCheckHealth_IncludesUptime(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if resp.Msg.UptimeMs < 0 {
		t.Error("expected non-negative uptime")
	}
	if resp.Msg.TimestampMs == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestHealthHandler_Healthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql"}},
	})

	handler := svc.HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type application/json, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql", Error: "connection failed"}},
	})

	handler := svc.HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Always returns 200 to indicate server is reachable.
	// Health status is in the response body (healthy: false).
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 (server reachable), got %d", rec.Code)
	}
}

func TestReadyHandler_AllCriticalHealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql"}},
	})
	svc.Register(&mockChecker{
		name:     "storage",
		statuses: []*healthpkg.Status{{Name: "storage", Backend: "gcs"}},
	})

	handler := svc.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected content-type application/json, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestReadyHandler_DatabaseUnhealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql", Error: "connection refused"}},
	})
	svc.Register(&mockChecker{
		name:     "storage",
		statuses: []*healthpkg.Status{{Name: "storage", Backend: "gcs"}},
	})

	handler := svc.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when database fails, got %d", rec.Code)
	}
}

func TestReadyHandler_StorageUnhealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql"}},
	})
	svc.Register(&mockChecker{
		name:     "storage",
		statuses: []*healthpkg.Status{{Name: "storage", Backend: "gcs", Error: "bucket not found"}},
	})

	handler := svc.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when storage fails, got %d", rec.Code)
	}
}

func TestReadyHandler_NonCriticalUnhealthy(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)
	svc.Register(&mockChecker{
		name:     "database",
		statuses: []*healthpkg.Status{{Name: "database", Backend: "postgresql"}},
	})
	svc.Register(&mockChecker{
		name:     "storage",
		statuses: []*healthpkg.Status{{Name: "storage", Backend: "gcs"}},
	})
	// AI provider failure should NOT mark the pod as not-ready - other
	// requests can still be served.
	svc.Register(&mockChecker{
		name:     "ai",
		statuses: []*healthpkg.Status{{Name: "ai", Backend: "gemini", Error: "rate limited"}},
	})
	svc.Register(&mockChecker{
		name:     "email",
		statuses: []*healthpkg.Status{{Name: "email", Backend: "mailgun", Error: "smtp timeout"}},
	})

	handler := svc.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 when only non-critical deps fail, got %d", rec.Code)
	}
}

func TestReadyHandler_NoCheckers(t *testing.T) {
	// With no checkers registered, the server has nothing to gate on and is
	// considered ready. This matches CheckHealth's behaviour.
	logger := logging.Default()
	svc := New(logger)

	handler := svc.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with no checkers, got %d", rec.Code)
	}
}

func TestCheckHealth_IndividualLatencies(t *testing.T) {
	logger := logging.Default()
	svc := New(logger)

	// Register composite checker that returns multiple statuses with individual latencies
	svc.Register(&mockChecker{
		name: "ai",
		statuses: []*healthpkg.Status{
			{Name: "ai", Backend: "anthropic", LatencyMs: 100},
			{Name: "ai", Backend: "gemini", LatencyMs: 200},
			{Name: "ai", Backend: "openai", LatencyMs: 150},
		},
	})

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}

	if len(resp.Msg.Dependencies) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(resp.Msg.Dependencies))
	}

	// Verify each dependency has its own latency
	expectedLatencies := map[string]int64{
		"anthropic": 100,
		"gemini":    200,
		"openai":    150,
	}

	for _, dep := range resp.Msg.Dependencies {
		expected, ok := expectedLatencies[dep.Backend]
		if !ok {
			t.Errorf("unexpected backend: %s", dep.Backend)
			continue
		}
		if dep.LatencyMs != expected {
			t.Errorf("expected %s latency=%d, got %d", dep.Backend, expected, dep.LatencyMs)
		}
		if dep.Name != "ai" {
			t.Errorf("expected name=ai for backend %s, got %s", dep.Backend, dep.Name)
		}
	}
}
