package clock

import (
	"net/http"
	"strconv"
	"time"
)

// SimulationTimestampHeader is the HTTP header that carries the simulation
// timestamp as a Unix epoch in seconds (int64).
const SimulationTimestampHeader = "X-Simulation-Timestamp"

// SimulationTimestamp returns HTTP middleware that reads the
// X-Simulation-Timestamp header and injects the corresponding time.Time into
// the request context. When the header is absent or unparseable the request
// context is left unchanged and clock.Now(ctx) falls back to time.Now().
func SimulationTimestamp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get(SimulationTimestampHeader); v != "" {
			if unix, err := strconv.ParseInt(v, 10, 64); err == nil {
				ctx := WithSimulationTime(r.Context(), time.Unix(unix, 0))
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}
