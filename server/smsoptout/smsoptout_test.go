package smsoptout

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestOptOutOptInCycle(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	if out, err := IsOptedOut(ctx, st, "+15551234567"); err != nil || out {
		t.Fatalf("expected not opted out initially, got out=%v err=%v", out, err)
	}

	// STOP via a loosely-formatted number; lookups normalize, so the E.164
	// form matches.
	if err := RecordOptOut(ctx, st, "(555) 123-4567"); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}
	if out, err := IsOptedOut(ctx, st, "+15551234567"); err != nil || !out {
		t.Fatalf("expected opted out after STOP, got out=%v err=%v", out, err)
	}

	// Exactly one row, with an opt-out timestamp set.
	rows, err := storage.QueryByField[*models.SmsOptOut](st, ctx, "phone_number", "+15551234567")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].OptedOutAtUnixSec == 0 {
		t.Errorf("expected opted_out_at to be set")
	}

	// START re-enables sends, reusing the same row.
	if err := RecordOptIn(ctx, st, "+15551234567"); err != nil {
		t.Fatalf("RecordOptIn: %v", err)
	}
	if out, err := IsOptedOut(ctx, st, "+15551234567"); err != nil || out {
		t.Fatalf("expected opted back in after START, got out=%v err=%v", out, err)
	}
	rows, _ = storage.QueryByField[*models.SmsOptOut](st, ctx, "phone_number", "+15551234567")
	if len(rows) != 1 {
		t.Errorf("START must reuse the row, got %d rows", len(rows))
	}
}

func TestOptOutIdempotent(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := RecordOptOut(ctx, st, "+15559990000"); err != nil {
			t.Fatalf("RecordOptOut #%d: %v", i, err)
		}
	}
	rows, err := storage.QueryByField[*models.SmsOptOut](st, ctx, "phone_number", "+15559990000")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("repeated STOP must not duplicate rows, got %d", len(rows))
	}
}

func TestInvalidNumberIsNoop(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	if err := RecordOptOut(ctx, st, "not-a-number"); err != nil {
		t.Fatalf("RecordOptOut(invalid): %v", err)
	}
	if out, err := IsOptedOut(ctx, st, "not-a-number"); err != nil || out {
		t.Errorf("invalid number should never be opted out, got out=%v err=%v", out, err)
	}
}
