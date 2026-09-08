package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// serveWithQueryStats runs one request through the QueryStats middleware with
// a handler that records a single fake query, and returns the parsed log line.
func serveWithQueryStats(t *testing.T, path string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := logging.NewLogger(logging.Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	handler := QueryStats(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats := storage.GetQueryStats(r.Context())
		if stats == nil {
			t.Fatal("expected QueryStats in request context")
		}
		stats.Count.Add(1)
		stats.TotalNs.Add(5_000_000) // 5ms
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, path, nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v (raw: %q)", err, buf.String())
	}
	return entry
}

func TestQueryStats_RPCPathIncludesRPCMethod(t *testing.T) {
	entry := serveWithQueryStats(t, "/ripls.api.GearService/SaveGear")

	if entry["rpc_method"] != "GearService/SaveGear" {
		t.Errorf("rpc_method = %v, want GearService/SaveGear", entry["rpc_method"])
	}
	if entry["db_queries"] != float64(1) {
		t.Errorf("db_queries = %v, want 1", entry["db_queries"])
	}
	if entry["db_duration_ms"] != float64(5) {
		t.Errorf("db_duration_ms = %v, want 5", entry["db_duration_ms"])
	}
}

func TestQueryStats_NonRPCPathOmitsRPCMethod(t *testing.T) {
	entry := serveWithQueryStats(t, "/health")

	if _, ok := entry["rpc_method"]; ok {
		t.Errorf("rpc_method should be absent for non-RPC path, got %v", entry["rpc_method"])
	}
	if entry["path"] != "/health" {
		t.Errorf("path = %v, want /health", entry["path"])
	}
}
