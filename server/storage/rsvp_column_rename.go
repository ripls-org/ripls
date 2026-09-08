package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// migrateRSVPEnumColumnRename finishes the #2829 RSVP enum migration's
// storage side. #2832 removed the legacy string fields and renamed the proto
// fields intention_enum/attended_enum to intention/attended (numbers 11/12
// unchanged), so the flat columns must follow: drop the legacy TEXT column
// left over from the string era and rename the INTEGER enum column into its
// place, keeping its data — no re-backfill.
//
// Must run before the generic per-type schema pass, and that ordering is
// load-bearing: left alone, the generic pass would treat the legacy TEXT
// intention column as the flat column for the renamed enum field, and
// migrateEnumColumnsToInteger would then fail the boot trying to cast its
// "YES" values to integers.
//
// Idempotent: once renamed, the *_enum columns are gone and every step
// no-ops. A fresh database never has them and skips entirely.
func migrateRSVPEnumColumnRename(ctx context.Context, db *sql.DB) error {
	steps := []struct {
		enumColumn string
		stmts      []string
	}{
		{"intention_enum", []string{
			`ALTER TABLE "experience_rsvp" DROP COLUMN IF EXISTS intention`,
			`ALTER TABLE "experience_rsvp" RENAME COLUMN intention_enum TO intention`,
		}},
		{"attended_enum", []string{
			`ALTER TABLE "experience_rsvp" DROP COLUMN IF EXISTS attended`,
			`ALTER TABLE "experience_rsvp" RENAME COLUMN attended_enum TO attended`,
		}},
	}
	for _, step := range steps {
		var exists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_name = 'experience_rsvp' AND column_name = $1
			)
		`, step.enumColumn).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check experience_rsvp.%s: %w", step.enumColumn, err)
		}
		if !exists {
			continue
		}
		for _, stmt := range step.stmts {
			// sql-fragment-allow: fixed DDL literals from the steps table above, no caller input
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("rename experience_rsvp.%s: %w", step.enumColumn, err)
			}
		}
	}
	return nil
}
