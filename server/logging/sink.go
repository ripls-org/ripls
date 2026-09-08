package logging

import (
	"context"
	"log/slog"
	"sync"
)

// LogRecord holds a single captured log record with all attributes merged.
type LogRecord struct {
	Attrs   map[string]any
	Message string
	Level   slog.Level
}

// TestSink is an slog.Handler that captures log records for inspection in tests.
// It is safe for concurrent use. Derived sinks created by WithAttrs share the
// same record slice and mutex as the original so all log calls are visible
// regardless of which logger was used. Create with NewTestSink.
type TestSink struct {
	mu       *sync.Mutex
	records  *[]LogRecord
	preAttrs []slog.Attr
	minLevel slog.Level
}

// NewTestSink creates a TestSink that captures records at or above minLevel.
func NewTestSink(minLevel slog.Level) *TestSink {
	return &TestSink{
		mu:       &sync.Mutex{},
		records:  &[]LogRecord{},
		minLevel: minLevel,
	}
}

// NewLogger returns a *Logger backed by this sink, ready to store in a test context
// via logging.WithLogger.
func (s *TestSink) NewLogger() *Logger {
	return &Logger{Logger: slog.New(s)}
}

// Enabled reports whether the handler handles records at the given level.
func (s *TestSink) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.minLevel
}

// Handle captures the log record, merging pre-configured attrs with record attrs.
func (s *TestSink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(s.preAttrs)+r.NumAttrs())
	for _, a := range s.preAttrs {
		attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.records = append(*s.records, LogRecord{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
	})
	return nil
}

// WithAttrs returns a derived TestSink that prepends attrs to all captured records.
// The derived sink shares the same record slice and mutex as the original.
func (s *TestSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(s.preAttrs)+len(attrs))
	copy(combined, s.preAttrs)
	copy(combined[len(s.preAttrs):], attrs)
	return &TestSink{
		mu:       s.mu,
		records:  s.records,
		minLevel: s.minLevel,
		preAttrs: combined,
	}
}

// WithGroup returns s unchanged — TestSink does not model group nesting.
func (s *TestSink) WithGroup(_ string) slog.Handler { return s }

// Records returns a snapshot of all captured log records.
func (s *TestSink) Records() []LogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogRecord, len(*s.records))
	copy(out, *s.records)
	return out
}
