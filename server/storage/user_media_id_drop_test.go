package storage

import (
	"context"
	"testing"
)

func TestDropUserMediaIdColumn_DropsColumn(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Simulate a pre-Phase-4a schema by manually re-adding the legacy
	// media_id column (the current proto doesn't declare it, so test
	// storage init doesn't create it).
	if _, err := store.db.ExecContext(ctx, `ALTER TABLE "user" ADD COLUMN media_id TEXT`); err != nil {
		t.Fatalf("setup: add legacy media_id column: %v", err)
	}
	exists, err := store.columnExists(ctx, "user", "media_id")
	if err != nil {
		t.Fatalf("setup: columnExists check: %v", err)
	}
	if !exists {
		t.Fatal("setup: media_id column should exist after ALTER TABLE ADD")
	}

	if err := store.DropUserMediaIDColumn(ctx); err != nil {
		t.Fatalf("DropUserMediaIDColumn: %v", err)
	}

	exists, err = store.columnExists(ctx, "user", "media_id")
	if err != nil {
		t.Fatalf("post-drop columnExists check: %v", err)
	}
	if exists {
		t.Error("media_id column should be gone after DropUserMediaIDColumn")
	}
}

func TestDropUserMediaIdColumn_NoopWhenAbsent(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Phase 4a state: column doesn't exist. Drop must succeed silently.
	if err := store.DropUserMediaIDColumn(ctx); err != nil {
		t.Fatalf("first call on absent column: %v", err)
	}
	// And again — idempotent across repeated startups.
	if err := store.DropUserMediaIDColumn(ctx); err != nil {
		t.Fatalf("second call (idempotency): %v", err)
	}
}

func TestDropUserMediaIdColumn_DropsDependentIndex(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Recreate the column + the partial index that Phase 2 installed,
	// then verify the index is gone after the column drop. Postgres
	// auto-drops indexes whose columns are dropped — this test pins
	// that contract for the deprecation.
	if _, err := store.db.ExecContext(ctx, `ALTER TABLE "user" ADD COLUMN media_id TEXT`); err != nil {
		t.Fatalf("setup: add media_id column: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`CREATE INDEX idx_user_media_id ON "user" (media_id) WHERE media_id <> ''`); err != nil {
		t.Fatalf("setup: create partial index: %v", err)
	}

	if err := store.DropUserMediaIDColumn(ctx); err != nil {
		t.Fatalf("DropUserMediaIDColumn: %v", err)
	}

	var indexCount int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'user' AND indexname = 'idx_user_media_id'`,
	).Scan(&indexCount); err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	if indexCount != 0 {
		t.Errorf("idx_user_media_id should be auto-dropped with the column, but it still exists")
	}
}
