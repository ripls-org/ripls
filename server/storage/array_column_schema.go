package storage

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/logging"
)

// InitializeArrayColumn idempotently creates a denormalized TEXT[] column on
// the table for msgType, backfills it from the message's binary_proto via
// extract, and creates a GIN index for efficient WHERE $1 = ANY(col) queries.
//
// Safe to call multiple times — each step is gated on a check (column exists,
// rows backfilled, index exists). If the table doesn't exist yet (test setups
// with limited type registration), the function returns nil without doing work.
//
// extract must agree with the [FieldExtractor] passed to
// [ProtoSQLStorage.RegisterArrayColumn] for the same (msgType, columnName)
// pair, otherwise the backfill and the per-write sync will diverge.
func (s *ProtoSQLStorage) InitializeArrayColumn(
	ctx context.Context,
	msgType proto.Message,
	columnName string,
	extract FieldExtractor,
) error {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return fmt.Errorf("InitializeArrayColumn: %s not registered for storage", typeName)
	}
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedColumn, err := quoteIdent(columnName)
	if err != nil {
		return err
	}
	if extract == nil {
		return fmt.Errorf("InitializeArrayColumn: extract must not be nil")
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "InitializeArrayColumn",
		"table", tableName,
		"column", columnName,
	)

	tableExists, err := s.tableExists(ctx, tableName)
	if err != nil {
		return err
	}
	if !tableExists {
		logger.DebugContext(ctx, "table not present, skipping array-column init")
		return nil
	}

	columnExists, err := s.columnExists(ctx, tableName, columnName)
	if err != nil {
		return err
	}
	if !columnExists {
		logger.InfoContext(ctx, "adding TEXT[] column")
		alter := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s TEXT[]`, quotedTable, quotedColumn)
		if _, err := s.db.ExecContext(ctx, alter); err != nil {
			return fmt.Errorf("add %s.%s column: %w", tableName, columnName, err)
		}
	}

	if err := s.backfillArrayColumn(ctx, msgType, tableName, columnName, extract); err != nil {
		return err
	}

	quotedIndex, err := quoteIdent(sanitizeColumnName(fmt.Sprintf("idx_%s_%s_gin", tableName, columnName)))
	if err != nil {
		return err
	}
	indexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s USING GIN (%s)`,
		quotedIndex, quotedTable, quotedColumn)
	if _, err := s.db.ExecContext(ctx, indexSQL); err != nil {
		return fmt.Errorf("create GIN index on %s.%s: %w", tableName, columnName, err)
	}

	return nil
}

// backfillArrayColumn extracts values from binary_proto via extract and writes
// them to the named TEXT[] column for every row where the column is NULL.
// Skipped rows (scan / unmarshal / update failures) are logged at WARN and
// execution continues so a single bad row doesn't stall startup.
func (s *ProtoSQLStorage) backfillArrayColumn(
	ctx context.Context,
	msgType proto.Message,
	tableName, columnName string,
	extract FieldExtractor,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "backfillArrayColumn",
		"table", tableName,
		"column", columnName,
	)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedColumn, err := quoteIdent(columnName)
	if err != nil {
		return err
	}

	selectSQL := fmt.Sprintf(`SELECT id, binary_proto FROM %s WHERE %s IS NULL`, quotedTable, quotedColumn)
	rows, err := s.db.QueryContext(ctx, selectSQL)
	if err != nil {
		return fmt.Errorf("scan %s for backfill: %w", tableName, err)
	}
	defer rows.Close()

	updateSQL := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE id = $2`, quotedTable, quotedColumn)

	var updateCount int
	for rows.Next() {
		var id string
		var protoBytes []byte
		if err := rows.Scan(&id, &protoBytes); err != nil {
			logger.WarnContext(ctx, "scan row failed, skipping", "error", err)
			continue
		}
		msg := proto.Clone(msgType)
		proto.Reset(msg)
		if err := proto.Unmarshal(protoBytes, msg); err != nil {
			logger.WarnContext(ctx, "unmarshal failed, skipping", "id", id, "error", err)
			continue
		}
		values := extract(msg)
		if _, err := s.db.ExecContext(ctx, updateSQL, pq.Array(values), id); err != nil {
			logger.WarnContext(ctx, "update failed, skipping", "id", id, "error", err)
			continue
		}
		updateCount++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s rows: %w", tableName, err)
	}
	logger.InfoContext(ctx, "backfill complete", "row_count", updateCount)
	return nil
}
