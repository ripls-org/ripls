package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// migrateEnumColumnsToInteger converts legacy TEXT enum flat columns to
// INTEGER for one registered message type.
//
// Before #2840, SQLType had no EnumKind case, so every enum field's flat
// column was created as TEXT and populated with the decimal enum number as a
// string ("1"). Parameterized queries worked by accident — lib/pq sends
// text-format parameters and Postgres infers their type from context — but
// integer-literal comparisons failed (42883) and ordering was lexicographic.
//
// Runs during schema initialization alongside missing-column addition and is
// idempotent: once a column reports a non-text type it is skipped, so on a
// converted (or freshly created) database this costs one information_schema
// lookup per enum column. The USING clause maps the stored digit strings to
// their numbers and preserves NULL for rows that predate the column.
func migrateEnumColumnsToInteger(ctx context.Context, db *sql.DB, descriptor protoreflect.MessageDescriptor, tableName string) error {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	for _, field := range flattenFields(descriptor, "") {
		if field.kind != protoreflect.EnumKind {
			continue
		}
		var dataType string
		err := db.QueryRowContext(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		`, tableName, field.columnName).Scan(&dataType)
		if errors.Is(err, sql.ErrNoRows) {
			// Column not created yet; the add-missing-columns pass creates it
			// with the correct INTEGER type.
			continue
		}
		if err != nil {
			return fmt.Errorf("look up type of %s.%s: %w", tableName, field.columnName, err)
		}
		if dataType != "text" {
			continue
		}
		quotedColumn, err := quoteIdent(field.columnName)
		if err != nil {
			return err
		}
		alterSQL := fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE INTEGER USING NULLIF(%s, '')::integer`,
			quotedTable, quotedColumn, quotedColumn,
		)
		if _, err := db.ExecContext(ctx, alterSQL); err != nil {
			return fmt.Errorf("convert %s.%s to INTEGER: %w", tableName, field.columnName, err)
		}
	}
	return nil
}
