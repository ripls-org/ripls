package emailsuppression

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestSuppressAndIsSuppressed(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	if supp, err := IsSuppressed(ctx, st, "a@example.com"); err != nil || supp {
		t.Fatalf("expected not suppressed initially, got supp=%v err=%v", supp, err)
	}

	// Suppress with mixed case; lookups normalize, so a lowercase check matches.
	if err := Suppress(ctx, st, "A@Example.com"); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if supp, err := IsSuppressed(ctx, st, "a@example.com"); err != nil || !supp {
		t.Fatalf("expected suppressed after Suppress, got supp=%v err=%v", supp, err)
	}

	// Idempotent: a second Suppress writes no duplicate row.
	if err := Suppress(ctx, st, "a@example.com"); err != nil {
		t.Fatalf("Suppress (idempotent): %v", err)
	}
	rows, err := storage.QueryByField[*models.EmailSuppression](st, ctx, "email", "a@example.com")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected exactly 1 suppression row, got %d", len(rows))
	}

	// A different, unsuppressed address is unaffected.
	if supp, err := IsSuppressed(ctx, st, "other@example.com"); err != nil || supp {
		t.Errorf("unrelated address should not be suppressed, got supp=%v err=%v", supp, err)
	}
}

func TestSuppressEmptyEmailIsNoop(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	if err := Suppress(ctx, st, ""); err != nil {
		t.Fatalf("Suppress(empty): %v", err)
	}
	if supp, err := IsSuppressed(ctx, st, ""); err != nil || supp {
		t.Errorf("empty email should never be suppressed, got supp=%v err=%v", supp, err)
	}
}
