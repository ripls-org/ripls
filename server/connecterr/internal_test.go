package connecterr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/logging"
)

// captureLogger returns a Logger that writes JSON records to buf, plus a
// context that carries it.
func captureLogger(t *testing.T) (*bytes.Buffer, context.Context) {
	t.Helper()
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	base := slog.New(handler)
	logger := &logging.Logger{Logger: base}
	ctx := logging.WithLogger(context.Background(), logger)
	return buf, ctx
}

// decodeRecords parses one JSON object per line from buf.
func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func TestInternalReturnsCodeInternal(t *testing.T) {
	_, ctx := captureLogger(t)
	err := Internal(ctx, "SaveGear", errors.New("pq: duplicate key"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Errorf("CodeOf = %v, want %v", got, connect.CodeInternal)
	}
}

func TestInternalPublicMessageIsGeneric(t *testing.T) {
	_, ctx := captureLogger(t)
	rawMessages := []string{
		"pq: duplicate key value violates unique constraint \"users_email_key\"",
		"json: cannot unmarshal string into field Foo of type int",
		"some.vendor/sdk: 500 internal server error: detail leak <user@example.com>",
	}
	for _, raw := range rawMessages {
		err := Internal(ctx, "op", errors.New(raw))
		if err.Error() == "" {
			t.Errorf("empty Error() for raw=%q", raw)
		}
		if strings.Contains(err.Error(), raw) {
			t.Errorf("public Error() leaked raw text %q: %q", raw, err.Error())
		}
		// Connect prefixes the public message with "internal: ", so check
		// the inner contents via Unwrap.
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) {
			t.Fatalf("expected *connect.Error, got %T", err)
		}
		if got, want := connectErr.Message(), "internal server error"; got != want {
			t.Errorf("Message() = %q, want %q", got, want)
		}
	}
}

func TestInternalLogsErrorWithFields(t *testing.T) {
	buf, ctx := captureLogger(t)
	cause := errors.New("storage decode failed")
	_ = Internal(ctx, "GetGear", cause, "gear_id", "g_42", "request_id", "r_1")
	recs := decodeRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 log record, got %d: %v", len(recs), recs)
	}
	rec := recs[0]
	if rec["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", rec["level"])
	}
	if rec["msg"] != "GetGear" {
		t.Errorf("msg = %v, want GetGear", rec["msg"])
	}
	if rec["error"] != cause.Error() {
		t.Errorf("error = %v, want %q", rec["error"], cause.Error())
	}
	if rec["gear_id"] != "g_42" {
		t.Errorf("gear_id = %v, want g_42", rec["gear_id"])
	}
	if rec["request_id"] != "r_1" {
		t.Errorf("request_id = %v, want r_1", rec["request_id"])
	}
}

func TestInternalNilErrorLogsWarnAndReturnsCodeInternal(t *testing.T) {
	buf, ctx := captureLogger(t)
	err := Internal(ctx, "BadCall", nil)
	if err == nil {
		t.Fatal("expected non-nil return even with nil err")
	}
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Errorf("CodeOf = %v, want %v", got, connect.CodeInternal)
	}
	recs := decodeRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 log record, got %d: %v", len(recs), recs)
	}
	rec := recs[0]
	if rec["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", rec["level"])
	}
	if rec["operation"] != "BadCall" {
		t.Errorf("operation = %v, want BadCall", rec["operation"])
	}
	if _, ok := rec["stack"].(string); !ok {
		t.Errorf("expected stack field of type string, got %T (%v)", rec["stack"], rec["stack"])
	}
}

func TestInternalDoesNotPanicOnNilError(t *testing.T) {
	_, ctx := captureLogger(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Internal panicked on nil err: %v", r)
		}
	}()
	_ = Internal(ctx, "op", nil)
}

func TestInternalRequestContextFieldsPropagate(t *testing.T) {
	buf, ctx := captureLogger(t)
	ctx = logging.WithRequestID(ctx, "req-123")
	ctx = logging.WithUserID(ctx, "user-9")
	ctx = logging.WithOperation(ctx, "SaveGear")
	_ = Internal(ctx, "SaveGear", errors.New("boom"))
	recs := decodeRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("expected 1 log record, got %d: %v", len(recs), recs)
	}
	rec := recs[0]
	if rec["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", rec["request_id"])
	}
	if rec["user_id"] != "user-9" {
		t.Errorf("user_id = %v, want user-9", rec["user_id"])
	}
	if rec["operation"] != "SaveGear" {
		t.Errorf("operation = %v, want SaveGear", rec["operation"])
	}
}
