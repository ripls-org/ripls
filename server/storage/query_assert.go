// Test helpers for asserting database query counts, used to detect N+1 regressions.

package storage

import (
	"context"
	"testing"
)

// AssertMaxQueries runs fn and fails the test if more than max database queries are executed.
// The context must have been created with WithQueryStats.
func AssertMaxQueries(t *testing.T, ctx context.Context, hi int, fn func()) {
	t.Helper()
	stats := GetQueryStats(ctx)
	if stats == nil {
		t.Fatal("AssertMaxQueries requires a context created with WithQueryStats")
	}
	before := stats.Count.Load()
	fn()
	after := stats.Count.Load()
	count := int(after - before)
	if count > hi {
		t.Errorf("expected at most %d queries, got %d", hi, count)
	}
}
