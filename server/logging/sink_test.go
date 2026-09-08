package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

func TestTestSink_CapturesInfo(t *testing.T) {
	sink := NewTestSink(slog.LevelInfo)
	logger := sink.NewLogger()

	logger.InfoContext(context.Background(), "hello", "key", "value")

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", r.Level)
	}
	if r.Message != "hello" {
		t.Errorf("message = %q, want %q", r.Message, "hello")
	}
	if r.Attrs["key"] != "value" {
		t.Errorf("attrs[key] = %v, want %q", r.Attrs["key"], "value")
	}
}

func TestTestSink_FiltersBelowMinLevel(t *testing.T) {
	sink := NewTestSink(slog.LevelInfo)
	logger := sink.NewLogger()

	logger.DebugContext(context.Background(), "debug line")
	if len(sink.Records()) != 0 {
		t.Error("expected DEBUG record to be filtered, got non-empty records")
	}

	logger.InfoContext(context.Background(), "info line")
	if len(sink.Records()) != 1 {
		t.Errorf("expected 1 record after INFO, got %d", len(sink.Records()))
	}
}

func TestTestSink_WithAttrsPreservesFields(t *testing.T) {
	sink := NewTestSink(slog.LevelInfo)
	logger := sink.NewLogger().With("op", "TestOp", "event_id", "evt-1")

	logger.InfoContext(context.Background(), "msg", "outcome", "sent")

	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Attrs["op"] != "TestOp" {
		t.Errorf("attrs[op] = %v, want %q", r.Attrs["op"], "TestOp")
	}
	if r.Attrs["event_id"] != "evt-1" {
		t.Errorf("attrs[event_id] = %v, want %q", r.Attrs["event_id"], "evt-1")
	}
	if r.Attrs["outcome"] != "sent" {
		t.Errorf("attrs[outcome] = %v, want %q", r.Attrs["outcome"], "sent")
	}
}

func TestTestSink_DerivedSinksShareRecords(t *testing.T) {
	// Records from a WithAttrs-derived logger must appear in the original sink.
	sink := NewTestSink(slog.LevelInfo)
	base := sink.NewLogger()
	derived := base.With("scope", "derived")

	derived.InfoContext(context.Background(), "from derived")
	base.InfoContext(context.Background(), "from base")

	records := sink.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records (one from each logger), got %d", len(records))
	}
}

func TestTestSink_ConcurrentSafe(t *testing.T) {
	sink := NewTestSink(slog.LevelInfo)
	logger := sink.NewLogger()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			logger.InfoContext(context.Background(), "concurrent", "i", i)
		}(i)
	}
	wg.Wait()

	if got := len(sink.Records()); got != goroutines {
		t.Errorf("expected %d records, got %d", goroutines, got)
	}
}
