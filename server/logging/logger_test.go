package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Cloud Logging format uses "message" not "msg"
	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["message"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", logEntry["key"])
	}
	// Cloud Logging format uses "severity" not "level"
	if logEntry["severity"] != "INFO" {
		t.Errorf("Expected severity 'INFO', got %v", logEntry["severity"])
	}
	// Ensure old field names are not present
	if _, exists := logEntry["msg"]; exists {
		t.Error("Old field 'msg' should not be present in Cloud Logging format")
	}
	if _, exists := logEntry["level"]; exists {
		t.Error("Old field 'level' should not be present in Cloud Logging format")
	}
}

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

func TestNewLogger_TextFormat_EmojiLevels(t *testing.T) {
	tests := []struct {
		level         string
		logFunc       func(*Logger)
		expectedEmoji string
	}{
		{"debug", func(l *Logger) { l.Debug("debug msg") }, "🐛"},
		{"debug", func(l *Logger) { l.Info("info msg") }, "ℹ️"},
		{"debug", func(l *Logger) { l.Warn("warn msg") }, "⚠️"},
		{"debug", func(l *Logger) { l.Error("error msg") }, "❌"},
	}

	for _, tc := range tests {
		var buf bytes.Buffer
		logger := NewLogger(Options{
			Level:  tc.level,
			Format: "text",
			Output: &buf,
		})

		tc.logFunc(logger)
		output := buf.String()

		if !strings.Contains(output, tc.expectedEmoji) {
			t.Errorf("Expected output to contain emoji %s, got: %s", tc.expectedEmoji, output)
		}
	}
}

func TestNewLogger_TextFormat_WithSourceLocation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:     "info",
		Format:    "text",
		Output:    &buf,
		AddSource: true,
	})

	logger.Info("test message")

	output := buf.String()
	// Should contain source file reference in brackets
	if !strings.Contains(output, "[logger_test.go:") {
		t.Errorf("Expected output to contain source location [logger_test.go:...], got: %s", output)
	}
}

func TestNewLogger_DefaultValues(t *testing.T) {
	var buf bytes.Buffer
	// Empty options should use defaults
	logger := NewLogger(Options{
		Output: &buf,
	})

	logger.Info("test")

	// Default format should be JSON
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Default format should be JSON, but got: %s", buf.String())
	}

	// Default level should be info (so info messages should appear)
	// Cloud Logging format uses "message" not "msg"
	if logEntry["message"] != "test" {
		t.Error("Info message should be logged with default level")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"invalid", slog.LevelInfo}, // Default to info
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			level := parseLevel(tc.input)
			if level != tc.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.input, level, tc.expected)
			}
		})
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "warn",
		Format: "json",
		Output: &buf,
	})

	// Info should be filtered out
	logger.Info("info message")
	if buf.Len() > 0 {
		t.Errorf("Info message should have been filtered, got: %s", buf.String())
	}

	// Warn should pass through
	logger.Warn("warn message")
	if buf.Len() == 0 {
		t.Error("Warn message should have been logged")
	}
}

func TestLogLevelFiltering_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "debug",
		Format: "json",
		Output: &buf,
	})

	// Debug should pass through when level is debug
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("Debug message should have been logged with debug level")
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	childLogger := logger.With("request_id", "req-123")
	childLogger.Info("test")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["request_id"] != "req-123" {
		t.Errorf("Expected request_id 'req-123', got %v", logEntry["request_id"])
	}
}

func TestContextFunctions_RequestID(t *testing.T) {
	ctx := context.Background()

	// Test WithRequestID and RequestIDFromContext
	ctx = WithRequestID(ctx, "req-456")
	if id := RequestIDFromContext(ctx); id != "req-456" {
		t.Errorf("Expected request_id 'req-456', got %q", id)
	}

	// Test empty context returns empty string
	emptyCtx := context.Background()
	if id := RequestIDFromContext(emptyCtx); id != "" {
		t.Errorf("Expected empty request_id, got %q", id)
	}
}

func TestContextFunctions_UserID(t *testing.T) {
	ctx := context.Background()

	// Test WithUserID and UserIDFromContext
	ctx = WithUserID(ctx, "user-789")
	if id := UserIDFromContext(ctx); id != "user-789" {
		t.Errorf("Expected user_id 'user-789', got %q", id)
	}

	// Test empty context returns empty string
	emptyCtx := context.Background()
	if id := UserIDFromContext(emptyCtx); id != "" {
		t.Errorf("Expected empty user_id, got %q", id)
	}
}

func TestContextFunctions_Operation(t *testing.T) {
	ctx := context.Background()

	// Test WithOperation and OperationFromContext
	ctx = WithOperation(ctx, "SaveGear")
	if op := OperationFromContext(ctx); op != "SaveGear" {
		t.Errorf("Expected operation 'SaveGear', got %q", op)
	}

	// Test empty context returns empty string
	emptyCtx := context.Background()
	if op := OperationFromContext(emptyCtx); op != "" {
		t.Errorf("Expected empty operation, got %q", op)
	}
}

func TestWithLogger_FromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	ctx := WithLogger(context.Background(), logger)
	retrieved := FromContext(ctx)

	retrieved.Info("from context")

	if buf.Len() == 0 {
		t.Error("Logger from context should have written to buffer")
	}
}

func TestFromContext_ReturnsDefault(t *testing.T) {
	// FromContext should return default logger when no logger in context
	ctx := context.Background()
	logger := FromContext(ctx)

	if logger == nil {
		t.Error("FromContext should return default logger, got nil")
	}
}

func TestLoggerWithContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	ctx := context.Background()
	ctx = WithLogger(ctx, logger)
	ctx = WithRequestID(ctx, "req-abc")
	ctx = WithUserID(ctx, "user-xyz")
	ctx = WithOperation(ctx, "TestOperation")

	contextLogger := LoggerWithContext(ctx)
	contextLogger.Info("with context")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["request_id"] != "req-abc" {
		t.Errorf("Expected request_id 'req-abc', got %v", logEntry["request_id"])
	}
	if logEntry["user_id"] != "user-xyz" {
		t.Errorf("Expected user_id 'user-xyz', got %v", logEntry["user_id"])
	}
	if logEntry["operation"] != "TestOperation" {
		t.Errorf("Expected operation 'TestOperation', got %v", logEntry["operation"])
	}
}

func TestLoggerWithContext_PartialContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Only request_id, no user_id or operation
	ctx := context.Background()
	ctx = WithLogger(ctx, logger)
	ctx = WithRequestID(ctx, "req-only")

	contextLogger := LoggerWithContext(ctx)
	contextLogger.Info("partial context")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["request_id"] != "req-only" {
		t.Errorf("Expected request_id 'req-only', got %v", logEntry["request_id"])
	}
	if _, exists := logEntry["user_id"]; exists {
		t.Error("user_id should not be present when not in context")
	}
	if _, exists := logEntry["operation"]; exists {
		t.Error("operation should not be present when not in context")
	}
}

func TestSetDefault(t *testing.T) {
	// Save original default
	original := defaultLogger

	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	SetDefault(logger)
	retrieved := Default()

	if retrieved != logger {
		t.Error("Default should return the logger set by SetDefault")
	}

	// Restore original
	defaultLogger = original
}

// TestCloudLoggingFormat verifies that JSON logs use Cloud Logging-compatible field names.
// Cloud Logging expects "severity" (not "level"), "message" (not "msg"),
// and "WARNING" (not "WARN") per the LogEntry specification.
// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry
func TestCloudLoggingFormat(t *testing.T) {
	tests := []struct {
		name             string
		logFunc          func(*Logger)
		expectedSeverity string
	}{
		{
			name:             "DEBUG level",
			logFunc:          func(l *Logger) { l.Debug("debug message") },
			expectedSeverity: "DEBUG",
		},
		{
			name:             "INFO level",
			logFunc:          func(l *Logger) { l.Info("info message") },
			expectedSeverity: "INFO",
		},
		{
			name:             "WARN level converts to WARNING",
			logFunc:          func(l *Logger) { l.Warn("warn message") },
			expectedSeverity: "WARNING",
		},
		{
			name:             "ERROR level",
			logFunc:          func(l *Logger) { l.Error("error message") },
			expectedSeverity: "ERROR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(Options{
				Level:  "debug", // Enable all levels
				Format: "json",
				Output: &buf,
			})

			tc.logFunc(logger)

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse JSON log output: %v", err)
			}

			// Verify Cloud Logging field names
			if _, exists := logEntry["severity"]; !exists {
				t.Error("Expected 'severity' field to be present")
			}
			if _, exists := logEntry["message"]; !exists {
				t.Error("Expected 'message' field to be present")
			}

			// Verify old field names are not present
			if _, exists := logEntry["level"]; exists {
				t.Error("Old field 'level' should not be present in Cloud Logging format")
			}
			if _, exists := logEntry["msg"]; exists {
				t.Error("Old field 'msg' should not be present in Cloud Logging format")
			}

			// Verify severity value
			if severity, ok := logEntry["severity"].(string); !ok {
				t.Errorf("Expected severity to be a string, got %T", logEntry["severity"])
			} else if severity != tc.expectedSeverity {
				t.Errorf("Expected severity %q, got %q", tc.expectedSeverity, severity)
			}
		})
	}
}

// TestCloudLoggingFormat_WithAttributes verifies that custom attributes are preserved
// and not renamed during the Cloud Logging field transformation.
func TestCloudLoggingFormat_WithAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	logger.Info("test message",
		"request_id", "req-123",
		"user_id", "user-456",
		"operation", "TestOp",
		"duration_ms", 42,
	)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify Cloud Logging fields
	if logEntry["severity"] != "INFO" {
		t.Errorf("Expected severity 'INFO', got %v", logEntry["severity"])
	}
	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["message"])
	}

	// Verify custom attributes are preserved
	if logEntry["request_id"] != "req-123" {
		t.Errorf("Expected request_id 'req-123', got %v", logEntry["request_id"])
	}
	if logEntry["user_id"] != "user-456" {
		t.Errorf("Expected user_id 'user-456', got %v", logEntry["user_id"])
	}
	if logEntry["operation"] != "TestOp" {
		t.Errorf("Expected operation 'TestOp', got %v", logEntry["operation"])
	}
	// JSON numbers are float64
	if duration, ok := logEntry["duration_ms"].(float64); !ok || duration != 42 {
		t.Errorf("Expected duration_ms 42, got %v", logEntry["duration_ms"])
	}
}

// signalingBuf wraps bytes.Buffer and closes a channel on the first Write so
// callers can wait for the write to occur before reading the buffer.
type signalingBuf struct {
	bytes.Buffer
	written chan struct{}
	once    sync.Once
}

func newSignalingBuf() *signalingBuf {
	return &signalingBuf{written: make(chan struct{})}
}

func (s *signalingBuf) Write(p []byte) (int, error) {
	n, err := s.Buffer.Write(p)
	s.once.Do(func() { close(s.written) })
	return n, err
}

// TestGoSafe verifies that GoSafe recovers panics without crashing, logs the
// goroutine name and panic value, and still runs non-panicking functions normally.
func TestGoSafe(t *testing.T) {
	t.Run("recovers panic and logs goroutine name", func(t *testing.T) {
		sig := newSignalingBuf()
		logger := NewLogger(Options{Level: "debug", Format: "json", Output: sig})

		ctx := WithLogger(context.Background(), logger)

		GoSafe(ctx, "test-goroutine", func() {
			panic("test panic value")
		})
		select {
		case <-sig.written:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for panic log")
		}

		output := sig.String()
		var logEntry map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
			t.Fatalf("Expected JSON log entry, got: %s", output)
		}
		if logEntry["goroutine"] != "test-goroutine" {
			t.Errorf("Expected goroutine 'test-goroutine', got %v", logEntry["goroutine"])
		}
		if logEntry["panic"] != "test panic value" {
			t.Errorf("Expected panic 'test panic value', got %v", logEntry["panic"])
		}
		if logEntry["severity"] != "ERROR" {
			t.Errorf("Expected severity 'ERROR', got %v", logEntry["severity"])
		}
		if _, ok := logEntry["stack"]; !ok {
			t.Error("Expected stack field in log entry")
		}
	})

	t.Run("runs non-panicking function normally", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewLogger(Options{Level: "debug", Format: "json", Output: &buf})

		ctx := WithLogger(context.Background(), logger)

		var wg sync.WaitGroup
		wg.Add(1)
		ran := false
		GoSafe(ctx, "safe-goroutine", func() {
			defer wg.Done()
			ran = true
		})
		wg.Wait()

		if !ran {
			t.Error("Expected GoSafe to run the function")
		}
		if buf.Len() > 0 {
			t.Errorf("Expected no log output for non-panicking function, got: %s", buf.String())
		}
	})

	t.Run("includes context fields in panic log", func(t *testing.T) {
		sig := newSignalingBuf()
		logger := NewLogger(Options{Level: "debug", Format: "json", Output: sig})

		ctx := WithLogger(context.Background(), logger)
		ctx = WithUserID(ctx, "user-123")
		ctx = WithRequestID(ctx, "req-456")

		GoSafe(ctx, "ctx-goroutine", func() {
			panic("ctx panic")
		})
		select {
		case <-sig.written:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for panic log")
		}

		var logEntry map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(sig.String())), &logEntry); err != nil {
			t.Fatalf("Expected JSON log entry: %v", err)
		}
		if logEntry["user_id"] != "user-123" {
			t.Errorf("Expected user_id 'user-123', got %v", logEntry["user_id"])
		}
		if logEntry["request_id"] != "req-456" {
			t.Errorf("Expected request_id 'req-456', got %v", logEntry["request_id"])
		}
	})
}

// TestCloudLoggingFormat_TextFormatUnaffected verifies that text format logs
// are not affected by Cloud Logging transformations (only JSON is transformed).
func TestCloudLoggingFormat_TextFormatUnaffected(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	logger.Warn("warning message")

	output := buf.String()
	// Text format should still work normally with emoji
	if !strings.Contains(output, "⚠️") {
		t.Errorf("Expected text format to contain warning emoji, got: %s", output)
	}
	if !strings.Contains(output, "warning message") {
		t.Errorf("Expected text format to contain message, got: %s", output)
	}
}

// TestLoggerWithContextNoDuplicateRequestID documents the slog key-collision
// bug that prompted issue #1548, and validates that the fix (using
// target_request_id for entity IDs) eliminates the collision.
//
// slog's JSON handler emits all attributes verbatim, including duplicates.
// Cloud Logging concatenates duplicate-key values, producing 72-char IDs.
// The correct fix is to use distinct keys: request_id for the HTTP
// correlation ID and target_request_id for a models.Request entity ID.
func TestLoggerWithContextNoDuplicateRequestID(t *testing.T) {
	t.Run("OldPattern_ProducesDuplicateKeys", func(t *testing.T) {
		// Demonstrates the original bug: using "request_id" as an entity-level
		// attribute key on a logger that already carries the HTTP correlation
		// request_id from LoggerWithContext produces two "request_id" keys in
		// the JSON output.
		var buf bytes.Buffer
		logger := NewLogger(Options{Level: "info", Format: "json", Output: &buf})
		ctx := WithLogger(context.Background(), logger)
		ctx = WithRequestID(ctx, "http-correlation-id")

		// Simulate the old buggy pattern.
		LoggerWithContext(ctx).With("request_id", "entity-id").Info("test")

		raw := buf.String()
		count := strings.Count(raw, `"request_id"`)
		if count != 2 {
			t.Errorf("expected 2 duplicate 'request_id' keys in raw JSON (documenting slog collision bug), got %d; output: %s", count, raw)
		}
	})

	t.Run("FixedPattern_NoCollision", func(t *testing.T) {
		// Validates the fix: using target_request_id for the entity ID
		// leaves request_id unambiguous as the HTTP correlation ID.
		var buf bytes.Buffer
		logger := NewLogger(Options{Level: "info", Format: "json", Output: &buf})
		ctx := WithLogger(context.Background(), logger)
		ctx = WithRequestID(ctx, "http-correlation-id")

		// Correct pattern: use target_request_id for the models.Request entity.
		LoggerWithContext(ctx).With("target_request_id", "entity-id").Info("test")

		raw := buf.String()
		if count := strings.Count(raw, `"request_id"`); count != 1 {
			t.Errorf("expected exactly 1 'request_id' key in JSON, got %d; output: %s", count, raw)
		}
		if count := strings.Count(raw, `"target_request_id"`); count != 1 {
			t.Errorf("expected exactly 1 'target_request_id' key in JSON, got %d; output: %s", count, raw)
		}

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if logEntry["request_id"] != "http-correlation-id" {
			t.Errorf("request_id should be the HTTP correlation ID, got %v", logEntry["request_id"])
		}
		if logEntry["target_request_id"] != "entity-id" {
			t.Errorf("target_request_id should be the entity ID, got %v", logEntry["target_request_id"])
		}
	})
}
