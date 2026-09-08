package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestOpenPostgreSQLDatabaseReadOnly proves the no-DDL open path serves
// full-fidelity reads over a schema established by the normal init path —
// the contract server/cmd/walkthrough-export depends on when connecting with a
// SELECT-only role (which cannot run the init path's CREATE/ALTER/backfill
// statements).
func TestOpenPostgreSQLDatabaseReadOnly(t *testing.T) {
	connStr, dbCleanup := SetupTestDatabase(t)
	defer dbCleanup()

	ctx := context.Background()

	// Establish the schema + a row through the normal (DDL-running) path.
	rw, err := InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("init storage: %v", err)
	}
	exp := &models.Experience{
		OwnerId:     "readonly-open-owner",
		Name:        "Readonly Open Fixture",
		Description: "seeded by TestOpenPostgreSQLDatabaseReadOnly",
		// A repeated field, so the read below proves binary_proto decoding
		// (repeated fields have no flattened scalar column).
		MediaIds: []string{"media-a", "media-b"},
	}
	id, err := rw.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if err := rw.Close(); err != nil {
		t.Fatalf("close rw storage: %v", err)
	}

	// Re-open read-only (no DDL) and read the row back both ways.
	ro, err := OpenPostgreSQLDatabaseReadOnly(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()

	got := &models.Experience{}
	if err := ro.GetByID(ctx, id, got); err != nil {
		t.Fatalf("GetByID via read-only handle: %v", err)
	}
	if got.GetName() != exp.GetName() || len(got.GetMediaIds()) != 2 {
		t.Errorf("GetByID = name %q, media %v; want %q, [media-a media-b]",
			got.GetName(), got.GetMediaIds(), exp.GetName())
	}

	byOwner, err := ro.QueryByField(ctx, "owner_id", "readonly-open-owner", &models.Experience{})
	if err != nil {
		t.Fatalf("QueryByField via read-only handle: %v", err)
	}
	if len(byOwner) != 1 {
		t.Fatalf("QueryByField returned %d rows, want 1", len(byOwner))
	}
	if byOwner[0].(*models.Experience).GetId() != id {
		t.Errorf("QueryByField id = %q, want %q", byOwner[0].(*models.Experience).GetId(), id)
	}
}
