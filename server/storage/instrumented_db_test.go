package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestQueryStats(t *testing.T) {
	t.Run("WithQueryStats attaches stats to context", func(t *testing.T) {
		ctx := WithQueryStats(context.Background())
		stats := GetQueryStats(ctx)
		if stats == nil {
			t.Fatal("expected non-nil QueryStats")
		}
		if stats.Count.Load() != 0 {
			t.Errorf("expected count 0, got %d", stats.Count.Load())
		}
	})

	t.Run("GetQueryStats returns nil without WithQueryStats", func(t *testing.T) {
		stats := GetQueryStats(context.Background())
		if stats != nil {
			t.Fatal("expected nil QueryStats from bare context")
		}
	})

	t.Run("recordQuery increments count and duration", func(t *testing.T) {
		ctx := WithQueryStats(context.Background())
		recordQuery(ctx, "SELECT 1", 10*time.Millisecond)
		recordQuery(ctx, "SELECT 2", 20*time.Millisecond)

		stats := GetQueryStats(ctx)
		if stats.Count.Load() != 2 {
			t.Errorf("expected count 2, got %d", stats.Count.Load())
		}
		if stats.TotalDuration() != 30*time.Millisecond {
			t.Errorf("expected 30ms total, got %v", stats.TotalDuration())
		}
	})

	t.Run("recordQuery is safe without stats in context", func(t *testing.T) {
		// Should not panic
		recordQuery(context.Background(), "SELECT 1", 5*time.Millisecond)
	})
}

// captureSlowQueryLog swaps the default slog logger for a JSON capture,
// runs fn, and returns the raw log output.
func captureSlowQueryLog(t *testing.T, fn func()) []byte {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	fn()
	return buf.Bytes()
}

func TestRecordQuery_SlowQueryLogIncludesQueryKind(t *testing.T) {
	out := captureSlowQueryLog(t, func() {
		recordQuery(context.Background(), "UPDATE gear SET name = $1", slowQueryThreshold)
	})

	var entry map[string]any
	if err := json.Unmarshal(out, &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v (raw: %q)", err, out)
	}
	if entry["msg"] != "slow query" {
		t.Errorf("msg = %v, want \"slow query\"", entry["msg"])
	}
	if entry["query_kind"] != "update" {
		t.Errorf("query_kind = %v, want update", entry["query_kind"])
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected duration_ms to be present")
	}
}

func TestRecordQuery_FastQueryDoesNotLog(t *testing.T) {
	out := captureSlowQueryLog(t, func() {
		recordQuery(context.Background(), "SELECT 1", slowQueryThreshold/2)
	})
	if len(out) != 0 {
		t.Errorf("expected no log output for fast query, got %q", out)
	}
}

func TestQueryKind(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"SELECT * FROM gear", "select"},
		{"select * from gear", "select"},
		{"  \n  SELECT 1", "select"},
		{"INSERT INTO gear ...", "insert"},
		{"UPDATE gear SET ...", "update"},
		{"DELETE FROM gear", "delete"},
		{"WITH cte AS (...) SELECT", "other"},
		{"", "other"},
		{"foo", "other"},
	}
	for _, c := range cases {
		if got := queryKind(c.query); got != c.want {
			t.Errorf("queryKind(%q) = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestAssertMaxQueries(t *testing.T) {
	t.Run("passes when under limit", func(t *testing.T) {
		ctx := WithQueryStats(context.Background())
		AssertMaxQueries(t, ctx, 5, func() {
			recordQuery(ctx, "SELECT 1", time.Millisecond)
			recordQuery(ctx, "SELECT 1", time.Millisecond)
		})
	})

	t.Run("passes at exact limit", func(t *testing.T) {
		ctx := WithQueryStats(context.Background())
		AssertMaxQueries(t, ctx, 2, func() {
			recordQuery(ctx, "SELECT 1", time.Millisecond)
			recordQuery(ctx, "SELECT 1", time.Millisecond)
		})
	})
}

func TestInstrumentedDB_CountsRealQueries(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := WithQueryStats(context.Background())

	// Gear has the media_ids TEXT[] denormalization registered, so a
	// successful Insert runs both the INSERT and a follow-up UPDATE
	// inside one transaction — 2 queries from Insert. Plus the GetByID
	// SELECT = 3 queries total.
	gear := &models.Gear{Name: "Instrumented Test"}
	id, err := s.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got := &models.Gear{}
	if err := s.GetByID(ctx, id, got); err != nil {
		t.Fatalf("get: %v", err)
	}

	stats := GetQueryStats(ctx)
	if stats.Count.Load() != 3 {
		t.Errorf("expected 3 queries (Insert+media_ids sync + GetByID), got %d", stats.Count.Load())
	}
	if stats.TotalDuration() <= 0 {
		t.Error("expected positive total duration")
	}
}
