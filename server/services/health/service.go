package health

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"go.ripls.org/ripls/server/gen/ripls/api"
	healthpkg "go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// Version should be set at build time via ldflags.
var Version = "dev"

// criticalDependencies are the dependency names whose health gates server
// readiness. If any of these is unhealthy, the server cannot serve traffic
// meaningfully and orchestrators (Cloud Run, k8s) should steer traffic away.
// Other dependencies (AI providers, email, geocoding) degrade specific
// features but do not warrant pulling the whole pod from rotation.
var criticalDependencies = map[string]bool{
	"database": true,
	"storage":  true,
}

// Checker is implemented by dependencies that can report their health.
type Checker interface {
	// Name returns the dependency name for reporting (used as fallback if status.Name is empty).
	Name() string
	// CheckHealth verifies the dependency is healthy and returns status information.
	// Returns a slice to allow composite checkers to return multiple leaf statuses.
	CheckHealth(ctx context.Context) ([]*healthpkg.Status, error)
}

// Service implements the HealthService RPC.
type Service struct {
	checkers  []Checker
	startTime time.Time
	logger    *logging.Logger
}

// New creates a new health service.
func New(logger *logging.Logger) *Service {
	return &Service{
		startTime: time.Now(),
		logger:    logger,
	}
}

// Register adds a checker to the health service.
func (s *Service) Register(checker Checker) {
	s.checkers = append(s.checkers, checker)
}

// CheckFunc is a function type that matches the CheckHealth signature.
type CheckFunc func(ctx context.Context) ([]*healthpkg.Status, error)

// funcChecker wraps a CheckFunc into a Checker.
type funcChecker struct {
	name string
	fn   CheckFunc
}

func (f *funcChecker) Name() string { return f.name }

func (f *funcChecker) CheckHealth(ctx context.Context) ([]*healthpkg.Status, error) { return f.fn(ctx) }

// RegisterFunc registers a named check function as a health checker.
func (s *Service) RegisterFunc(name string, fn CheckFunc) {
	s.checkers = append(s.checkers, &funcChecker{name: name, fn: fn})
}

// DefaultExternalTTL is the recommended cache TTL for checkers that probe
// external paid or quota-bound APIs (geocoding, email, AI, GitHub). At 15
// minutes, both environments' uptime-check probes stay far inside every
// provider's free tier (#2809) while a real outage still surfaces within one
// TTL window. Free local checks (database, storage) register uncached so
// /readyz always reflects current state.
const DefaultExternalTTL = 15 * time.Minute

// cachedChecker serves a wrapped checker's results from a TTL cache. Uptime
// checks hit /health every ~100 seconds from multiple prober regions; without
// a cache, every probe re-runs every registered checker, which for paid
// upstreams turns monitoring frequency directly into provider spend (a
// billable geocode per probe, #2809; previously a Pexels quota burn, #929).
// Errors are cached like successes so a failing paid API is not hammered
// either.
type cachedChecker struct {
	inner Checker
	ttl   time.Duration

	// mu also serializes refreshes: when the cache expires, exactly one
	// caller performs the upstream probe while concurrent callers wait
	// and reuse its result.
	mu        sync.Mutex
	fetchedAt time.Time
	statuses  []*healthpkg.Status
	err       error
}

func (c *cachedChecker) Name() string { return c.inner.Name() }

// CheckHealth returns the cached statuses while fresh, otherwise re-probes
// the wrapped checker. Cached copies carry a cached_age_ms detail so the
// /health body is honest about staleness; LatencyMs keeps the real probe's
// measurement.
func (c *cachedChecker) CheckHealth(ctx context.Context) ([]*healthpkg.Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	age := time.Since(c.fetchedAt)
	if !c.fetchedAt.IsZero() && age < c.ttl {
		return cloneStatuses(c.statuses, age), c.err
	}

	statuses, err := c.inner.CheckHealth(ctx)
	c.fetchedAt = time.Now()
	c.statuses = statuses
	c.err = err
	return statuses, err
}

// cloneStatuses copies cached statuses before handing them to a request so
// concurrent health handlers never share (and annotate) the same Metadata
// maps.
func cloneStatuses(statuses []*healthpkg.Status, age time.Duration) []*healthpkg.Status {
	cloned := make([]*healthpkg.Status, len(statuses))
	for i, s := range statuses {
		copied := *s
		copied.Metadata = make(map[string]string, len(s.Metadata)+1)
		for k, v := range s.Metadata {
			copied.Metadata[k] = v
		}
		copied.Metadata["cached_age_ms"] = strconv.FormatInt(age.Milliseconds(), 10)
		cloned[i] = &copied
	}
	return cloned
}

// RegisterFuncCached registers a named check function whose results are
// reused for ttl before the dependency is probed again. Use it for checkers
// whose probe costs money or provider quota; keep free critical checks on
// RegisterFunc so readiness stays current.
func (s *Service) RegisterFuncCached(name string, ttl time.Duration, fn CheckFunc) {
	s.checkers = append(s.checkers, &cachedChecker{
		inner: &funcChecker{name: name, fn: fn},
		ttl:   ttl,
	})
}

// CheckHealth implements the HealthService RPC.
func (s *Service) CheckHealth(
	ctx context.Context,
	req *connect.Request[api.CheckHealthRequest],
) (*connect.Response[api.CheckHealthResponse], error) {
	response := &api.CheckHealthResponse{
		TimestampMs: time.Now().UnixMilli(),
		Version:     Version,
		Healthy:     true,
		UptimeMs:    time.Since(s.startTime).Milliseconds(),
	}

	for _, checker := range s.checkers {
		start := time.Now()
		statuses, err := checker.CheckHealth(ctx)
		if err != nil {
			return nil, err
		}
		fallbackLatency := time.Since(start).Milliseconds()

		for _, healthStatus := range statuses {
			name := healthStatus.Name
			if name == "" {
				name = checker.Name()
			}
			latency := healthStatus.LatencyMs
			if latency == 0 {
				latency = fallbackLatency
			}

			healthy := healthStatus.IsHealthy()
			status := &api.DependencyStatus{
				Name:      name,
				Healthy:   healthy,
				Backend:   healthStatus.Backend,
				Details:   healthStatus.Metadata,
				LatencyMs: latency,
				Error:     healthStatus.Error,
			}
			response.Dependencies = append(response.Dependencies, status)

			if !healthy {
				response.Healthy = false
			}
		}
	}

	// Log failures with full details for alerting
	var failedDependencies []string
	for _, dep := range response.Dependencies {
		if !dep.Healthy {
			failedDependencies = append(failedDependencies, dep.Name)

			// Log each failure with full details for log-based alerting
			logFields := []any{
				"dependency", dep.Name,
				"backend", dep.Backend,
				"error", dep.Error,
				"latency_ms", dep.LatencyMs,
			}
			// Include relevant metadata in log (e.g., bucket name, model)
			for k, v := range dep.Details {
				logFields = append(logFields, k, v)
			}
			s.logger.ErrorContext(ctx, "dependency_health_check_failed", logFields...)
		}
	}

	// Log the overall health check result
	logFields := []any{
		"healthy", response.Healthy,
		"dependency_count", len(response.Dependencies),
	}
	if len(failedDependencies) > 0 {
		logFields = append(logFields, "failed_dependencies", failedDependencies)
	}
	s.logger.InfoContext(ctx, "health_check_complete", logFields...)

	return connect.NewResponse(response), nil
}

// HealthHandler returns a simple GET handler for /health that wraps the RPC.
// Always returns HTTP 200 to indicate the server is reachable. The health status
// is communicated in the response body (healthy: true/false). This allows uptime
// checks to distinguish between "server is down" (connection failure) and
// "server is up but a dependency is failing" (200 with healthy: false).
func (s *Service) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		resp, _ := s.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{}))

		jsonBytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(resp.Msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to marshal health response", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Always return 200 OK - the server is reachable and responding.
		// Health status is in the body. This ensures uptime checks only fail
		// when the server is truly unreachable, not when a dependency is down.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(jsonBytes); err != nil {
			s.logger.ErrorContext(ctx, "failed to write health response", "error", err)
		}
	}
}

// ReadyHandler returns a GET handler for /readyz that reports orchestrator
// readiness. It returns HTTP 503 when any critical dependency (database,
// storage) is unhealthy, signalling Cloud Run / k8s to remove the pod from
// rotation. Returns 200 when all critical dependencies are healthy, even if
// non-critical dependencies (AI providers, email, geocoding) are degraded.
// The full dependency status is included in the response body for debugging.
func (s *Service) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		resp, err := s.CheckHealth(ctx, connect.NewRequest(&api.CheckHealthRequest{}))
		if err != nil {
			s.logger.ErrorContext(ctx, "readiness check failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ready := true
		var failedCritical []string
		for _, dep := range resp.Msg.Dependencies {
			if criticalDependencies[dep.Name] && !dep.Healthy {
				ready = false
				failedCritical = append(failedCritical, dep.Name)
			}
		}

		jsonBytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(resp.Msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to marshal readiness response", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if ready {
			w.WriteHeader(http.StatusOK)
		} else {
			s.logger.WarnContext(ctx, "readiness_check_not_ready", "failed_critical", failedCritical)
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if _, err := w.Write(jsonBytes); err != nil {
			s.logger.ErrorContext(ctx, "failed to write readiness response", "error", err)
		}
	}
}
