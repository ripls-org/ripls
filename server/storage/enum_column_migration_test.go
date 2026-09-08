package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// columnDataType reads a column's type from information_schema. The empty
// string means the column does not exist.
func columnDataType(t *testing.T, store *ProtoSQLStorage, table, column string) string {
	t.Helper()
	var dataType string
	err := store.db.QueryRowContext(context.Background(), `
		SELECT COALESCE(
			(SELECT data_type FROM information_schema.columns
			 WHERE table_name = $1 AND column_name = $2), '')
	`, table, column).Scan(&dataType)
	if err != nil {
		t.Fatalf("look up type of %s.%s: %v", table, column, err)
	}
	return dataType
}

// A fresh schema must create enum flat columns as INTEGER, and integer-literal
// comparisons against them must work — the exact query shape that failed with
// 42883 when the columns were TEXT (#2840).
func TestEnumColumns_FreshSchemaIsInteger(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	for _, tc := range []struct{ table, column string }{
		{"experience_rsvp", "intention"},
		{"experience_rsvp", "attended"},
		{"gear", "state"},
		{"experience", "state"},
		{"scheduled_notification", "experience_purpose"},
	} {
		if got := columnDataType(t, store, tc.table, tc.column); got != "integer" {
			t.Errorf("%s.%s: data_type = %q, want integer", tc.table, tc.column, got)
		}
	}

	rsvp := &models.ExperienceRSVP{
		ExperienceId: "exp-1",
		UserId:       "user-1",
		CommunityId:  "comm-1",
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
	}
	if _, err := store.Insert(ctx, rsvp); err != nil {
		t.Fatalf("insert RSVP: %v", err)
	}

	var count int
	err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM "experience_rsvp" WHERE intention = 1
	`).Scan(&count)
	if err != nil {
		t.Fatalf("integer-literal query against enum column: %v", err)
	}
	if count != 1 {
		t.Errorf("rows with intention = 1: got %d, want 1", count)
	}
}

// A database whose enum columns predate #2840 — TEXT holding decimal digit
// strings — must be converted to INTEGER on the next schema initialization,
// preserving values and NULLs.
func TestEnumColumns_LegacyTextColumnsMigrated(t *testing.T) {
	connStr, dbCleanup := SetupTestDatabase(t)
	defer dbCleanup()
	ctx := context.Background()

	store, err := InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	// Recreate the pre-#2840 shape: a TEXT enum column holding digit strings.
	if _, err := store.db.ExecContext(ctx,
		`ALTER TABLE "gear" ALTER COLUMN state TYPE TEXT USING state::text`); err != nil {
		t.Fatalf("setup: downgrade column to TEXT: %v", err)
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO "gear" (id, state, binary_proto)
		VALUES
			('gear-digits', '2', ''::bytea),
			('gear-null',   NULL, ''::bytea),
			('gear-empty',  '',   ''::bytea)
	`)
	if err != nil {
		t.Fatalf("setup: seed legacy rows: %v", err)
	}
	store.Close()

	// Re-initialization must ratchet the column back to INTEGER.
	store, err = InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("second init (migration): %v", err)
	}
	defer store.Close()

	if got := columnDataType(t, store, "gear", "state"); got != "integer" {
		t.Errorf("after migration, gear.state: data_type = %q, want integer", got)
	}

	rows, err := store.db.QueryContext(ctx, `SELECT id, state FROM "gear" ORDER BY id`)
	if err != nil {
		t.Fatalf("read migrated rows: %v", err)
	}
	defer rows.Close()
	got := map[string]*int{}
	for rows.Next() {
		var id string
		var state *int
		if err := rows.Scan(&id, &state); err != nil {
			t.Fatalf("scan migrated row: %v", err)
		}
		got[id] = state
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated rows: %v", err)
	}

	if v := got["gear-digits"]; v == nil || *v != 2 {
		t.Errorf("gear-digits state: got %v, want 2", v)
	}
	if v := got["gear-null"]; v != nil {
		t.Errorf("gear-null state: got %v, want NULL", *v)
	}
	if v := got["gear-empty"]; v != nil {
		t.Errorf("gear-empty state: got %v, want NULL", *v)
	}
}

// A database from the #2829 migration window — INTEGER intention_enum /
// attended_enum next to legacy TEXT intention/attended holding string values —
// must come out of initialization with the enum data renamed into the clean
// column names and the legacy TEXT columns gone (#2832).
func TestEnumColumns_RSVPEnumColumnsRenamed(t *testing.T) {
	connStr, dbCleanup := SetupTestDatabase(t)
	defer dbCleanup()
	ctx := context.Background()

	store, err := InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("first init: %v", err)
	}

	// Recreate the pre-#2832 shape.
	for _, stmt := range []string{
		`ALTER TABLE "experience_rsvp" RENAME COLUMN intention TO intention_enum`,
		`ALTER TABLE "experience_rsvp" RENAME COLUMN attended TO attended_enum`,
		`ALTER TABLE "experience_rsvp" ADD COLUMN intention TEXT`,
		`ALTER TABLE "experience_rsvp" ADD COLUMN attended TEXT`,
	} {
		if _, err := store.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("setup: recreate migration-window schema: %v", err)
		}
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO "experience_rsvp"
			(id, experience_id, user_id, community_id, intention, attended, intention_enum, attended_enum, binary_proto)
		VALUES
			('rsvp-yes', 'exp-1', 'user-1', 'comm-1', 'YES', 'UNKNOWN', 1, 1, ''::bytea),
			('rsvp-no',  'exp-1', 'user-2', 'comm-1', 'NO',  '',        3, 0, ''::bytea)
	`)
	if err != nil {
		t.Fatalf("setup: seed migration-window rows: %v", err)
	}
	store.Close()

	// Re-initialization must drop the TEXT columns and rename the enum data
	// into their place. Before the rename migration existed, this init failed
	// outright: the enum ratchet tried to cast 'YES' to integer.
	store, err = InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("second init (rename migration): %v", err)
	}
	defer store.Close()

	for _, column := range []string{"intention", "attended"} {
		if got := columnDataType(t, store, "experience_rsvp", column); got != "integer" {
			t.Errorf("after rename, experience_rsvp.%s: data_type = %q, want integer", column, got)
		}
	}
	for _, column := range []string{"intention_enum", "attended_enum"} {
		if got := columnDataType(t, store, "experience_rsvp", column); got != "" {
			t.Errorf("after rename, experience_rsvp.%s still exists with type %q", column, got)
		}
	}

	// The enum data survived the rename and reads back through the proto.
	got := &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, "rsvp-no", got); err != nil {
		t.Fatalf("get renamed RSVP: %v", err)
	}
	var intention int
	if err := store.db.QueryRowContext(ctx,
		`SELECT intention FROM "experience_rsvp" WHERE id = 'rsvp-no'`).Scan(&intention); err != nil {
		t.Fatalf("read renamed column: %v", err)
	}
	if intention != int(models.RSVPIntention_RSVP_INTENTION_NO) {
		t.Errorf("rsvp-no intention column: got %d, want %d", intention, models.RSVPIntention_RSVP_INTENTION_NO)
	}

	// A third initialization is a no-op — the rename must be idempotent.
	store3, err := InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		t.Fatalf("third init (idempotency): %v", err)
	}
	store3.Close()
}
