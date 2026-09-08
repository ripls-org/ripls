// Core CRUD operations for protobuf-to-SQL storage: Insert, GetByID, ListAll,
// QueryByField, QueryByFields, Update, Delete, and related utilities.

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.ripls.org/ripls/server/health"
)

// Insert stores a proto message in its corresponding table.
func (s *ProtoSQLStorage) Insert(ctx context.Context, msg proto.Message) (string, error) {
	// If msg has registered array-column denormalizations and we're not
	// already in a transaction, wrap the insert + column sync in WithTx so
	// the row and its indexed array columns commit atomically. Recursing
	// into Insert inside the tx is intentional — tx.inTransaction() is then
	// true so we fall through to the body below on the second entry.
	if s.hasArrayColumns(msg) && !s.inTransaction() {
		var id string
		err := s.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
			var ierr error
			id, ierr = tx.Insert(ctx, msg)
			return ierr
		})
		return id, err
	}

	msgReflect := msg.ProtoReflect()
	descriptor := msgReflect.Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return "", fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	fields := descriptor.Fields()

	// Extract field names and values using reflection
	columns, values := extractFieldValues(msgReflect)

	// Generate ID if first field is 'id' and empty
	if fields.Len() > 0 && string(fields.Get(0).Name()) == "id" && values[0] == "" {
		newID := uuid.New().String()
		values[0] = newID
		// Update the message with the generated ID
		msgReflect.Set(fields.Get(0), protoreflect.ValueOfString(newID))
	}

	// Serialize the proto message for binary_proto column
	protoData, err := proto.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal proto: %w", err)
	}

	columns = append(columns, "binary_proto")
	values = append(values, protoData)

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return "", err
	}
	quotedColumns, err := quoteIdents(columns)
	if err != nil {
		return "", err
	}

	// Build and execute INSERT query
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quotedTable,
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err = s.exec.ExecContext(ctx, query, values...)
	if err != nil {
		return "", fmt.Errorf("failed to insert into %s: %w", tableName, err)
	}

	// Get the ID (first field value as string)
	id := ""
	if len(values) > 0 {
		id = fmt.Sprintf("%v", values[0])
	}

	// Sync any registered TEXT[] denormalization columns from msg. Routes
	// through s.exec so it participates in the surrounding transaction
	// when the outer Insert wrapped us in one.
	if err := s.syncArrayColumns(ctx, msg, id); err != nil {
		return "", fmt.Errorf("sync array columns on insert: %w", err)
	}

	// Trigger async embedding generation for every configured variant.
	s.queueEmbeddingGeneration(ctx, tableName, msg, id)

	return id, nil
}

// GetByID retrieves a proto message by its ID from the binary_proto column.
// By default, soft-deleted items are excluded (returns "record not found").
// Pass QueryOptions{IncludeDeleted: true} to include deleted items.
func (s *ProtoSQLStorage) GetByID(ctx context.Context, id string, msg proto.Message, opts ...QueryOptions) error {
	descriptor := msg.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Build query with optional deleted filter
	var opt *QueryOptions
	if len(opts) > 0 {
		opt = &opts[0]
	}

	if opt != nil && opt.ForUpdate && !s.inTransaction() {
		return fmt.Errorf("GetByID on %s: %w", tableName, ErrForUpdateOutsideTransaction)
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("SELECT binary_proto FROM %s WHERE id = %s%s",
		quotedTable, s.dbSpec.Placeholder(1), buildDeletedFilter(descriptor, opt))
	if opt != nil && opt.ForUpdate {
		// sql-fragment-allow: FOR UPDATE clause is a fixed constant, no caller input
		query += " FOR UPDATE"
	}

	row := s.exec.QueryRowContext(ctx, query, id)

	var protoData []byte
	if err := row.Scan(&protoData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)
		}
		return fmt.Errorf("failed to scan from %s: %w", tableName, err)
	}

	// Deserialize into the provided message
	if err := proto.Unmarshal(protoData, msg); err != nil {
		return fmt.Errorf("failed to unmarshal proto: %w", err)
	}

	return nil
}

// ListAll retrieves all proto messages of a given type.
// The msgType parameter should be an empty instance of the desired message type.
// Optional QueryOptions can specify a Limit to cap the number of rows returned.
func (s *ProtoSQLStorage) ListAll(ctx context.Context, msgType proto.Message, opts ...QueryOptions) ([]proto.Message, error) {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT binary_proto FROM %s", quotedTable)

	// ListAll deliberately has no default cap: several callers
	// (server/impact_metrics/dataset.go, server/services/location/places.go)
	// need every row, and silently truncating them would skew derived metrics
	// with no error raised anywhere. A caller that wants a bound passes one.
	var args []any
	if len(opts) > 0 && opts[0].Limit > 0 {
		query += " LIMIT " + s.dbSpec.Placeholder(1)
		args = append(args, opts[0].Limit)
	}

	rows, err := s.exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query all from %s: %w", tableName, err)
	}
	defer rows.Close()

	var messages []proto.Message
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a new instance of the message type
		msg := proto.Clone(msgType)
		proto.Reset(msg)

		if err := proto.Unmarshal(protoData, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proto: %w", err)
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// HasAnyUsers checks if any users exist in the database.
// Returns true if at least one user exists, false otherwise.
func (s *ProtoSQLStorage) HasAnyUsers(ctx context.Context) (bool, error) {
	tableName, ok := s.allowedTypes["ripls.models.User"]
	if !ok {
		return false, fmt.Errorf("user type is not registered for storage")
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return false, err
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s LIMIT 1", quotedTable)

	var count int64
	err = s.exec.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check for users: %w", err)
	}

	return count > 0, nil
}

// QueryByField retrieves proto messages by a field value.
// The msgType parameter should be an empty instance of the desired message type.
// By default, soft-deleted items are excluded. Pass QueryOptions{IncludeDeleted: true} to include them.
func (s *ProtoSQLStorage) QueryByField(ctx context.Context, fieldName string, value any, msgType proto.Message, opts ...QueryOptions) ([]proto.Message, error) {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Build query with optional deleted filter
	var opt *QueryOptions
	if len(opts) > 0 {
		opt = &opts[0]
	}
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT binary_proto FROM %s WHERE %s = %s%s",
		quotedTable, quotedField, s.dbSpec.Placeholder(1), buildDeletedFilter(descriptor, opt))

	rows, err := s.exec.QueryContext(ctx, query, value)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s by %s: %w", tableName, fieldName, err)
	}
	defer rows.Close()

	var messages []proto.Message
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a new instance of the message type
		msg := proto.Clone(msgType)
		proto.Reset(msg)

		if err := proto.Unmarshal(protoData, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proto: %w", err)
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// QueryByFields retrieves proto messages matching multiple field values.
// The msgType parameter should be an empty instance of the desired message type.
// fieldValues is a map of field names to their values.
// By default, soft-deleted items are excluded. Pass QueryOptions{IncludeDeleted: true} to include them.
func (s *ProtoSQLStorage) QueryByFields(ctx context.Context, fieldValues map[string]any, msgType proto.Message, opts ...QueryOptions) ([]proto.Message, error) {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	if len(fieldValues) == 0 {
		return nil, fmt.Errorf("at least one field must be specified")
	}

	// Build WHERE clause with AND conditions
	var whereClauses []string
	var values []any
	placeholderIdx := 1
	for fieldName, value := range fieldValues {
		quotedField, err := quoteIdent(fieldName)
		if err != nil {
			return nil, err
		}
		whereClauses = append(whereClauses, fmt.Sprintf("%s = %s", quotedField, s.dbSpec.Placeholder(placeholderIdx)))
		values = append(values, value)
		placeholderIdx++
	}

	// Build query with optional deleted filter
	var opt *QueryOptions
	if len(opts) > 0 {
		opt = &opts[0]
	}
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT binary_proto FROM %s WHERE %s%s",
		quotedTable, strings.Join(whereClauses, " AND "), buildDeletedFilter(descriptor, opt))

	rows, err := s.exec.QueryContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s by fields: %w", tableName, err)
	}
	defer rows.Close()

	var messages []proto.Message
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a new instance of the message type
		msg := proto.Clone(msgType)
		proto.Reset(msg)

		if err := proto.Unmarshal(protoData, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal proto: %w", err)
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// Update updates an existing proto message in its corresponding table.
func (s *ProtoSQLStorage) Update(ctx context.Context, msg proto.Message) error {
	_, err := s.update(ctx, msg, false)
	return err
}

// UpdateIfNotDeleted writes msg to its table by id, but only when the stored
// row is not soft-deleted. It returns true when the row was written and false
// when the write was skipped because the row was concurrently soft-deleted (or
// no longer exists).
//
// This guard covers only the soft-delete race: a concurrent soft-delete between
// the caller's read and this write is made a no-op. It does NOT prevent other
// concurrent field changes (e.g. a concurrent CancelRequest flipping State)
// from being clobbered by a stale snapshot. For callers that also need to guard
// concurrent field changes, use the tx-guarded pattern instead:
// WithTx + GetByID(QueryOptions{ForUpdate: true, IncludeDeleted: true}), copy
// only the owned fields onto the fresh snapshot, then plain tx.Update.
// See WithTx godoc → "Locked reload" and #2900.
func (s *ProtoSQLStorage) UpdateIfNotDeleted(ctx context.Context, msg proto.Message) (bool, error) {
	return s.update(ctx, msg, true)
}

// update writes msg to its table by id. When guardNotDeleted is true the WHERE
// clause additionally requires the row to be live, and a zero-row result is
// reported as (false, nil) rather than ErrRecordNotFound. The bool return
// reports whether a row was written.
func (s *ProtoSQLStorage) update(ctx context.Context, msg proto.Message, guardNotDeleted bool) (bool, error) {
	// See Insert for the rationale: registered array-column denormalizations
	// require atomicity with the underlying row write.
	if s.hasArrayColumns(msg) && !s.inTransaction() {
		var applied bool
		err := s.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
			var e error
			applied, e = tx.update(ctx, msg, guardNotDeleted)
			return e
		})
		return applied, err
	}

	msgReflect := msg.ProtoReflect()
	descriptor := msgReflect.Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return false, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	fields := descriptor.Fields()
	if fields.Len() == 0 {
		return false, fmt.Errorf("message type %s has no fields", typeName)
	}

	// First field must be 'id' and must be set
	idField := fields.Get(0)
	if string(idField.Name()) != "id" {
		return false, fmt.Errorf("first field must be 'id' for updates")
	}

	id := msgReflect.Get(idField).String()
	if id == "" {
		return false, fmt.Errorf("id field cannot be empty for update")
	}

	// Extract field names and values using reflection
	columns, values := extractFieldValues(msgReflect)

	// Serialize the proto message for binary_proto column
	protoData, err := proto.Marshal(msg)
	if err != nil {
		return false, fmt.Errorf("failed to marshal proto: %w", err)
	}

	columns = append(columns, "binary_proto")
	values = append(values, protoData)

	// Build SET clause (skip the id field as it's in WHERE clause). Columns are
	// quoted here to match the WHERE clause below; the two used to disagree,
	// which worked only because proto field names are lowercase snake_case that
	// PostgreSQL folds identically whether quoted or not.
	quotedColumns, err := quoteIdents(columns)
	if err != nil {
		return false, err
	}
	var setClauses []string
	var setValues []any
	for i := 1; i < len(columns); i++ {
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", quotedColumns[i], s.dbSpec.Placeholder(i)))
		setValues = append(setValues, values[i])
	}

	// Add id value for WHERE clause
	setValues = append(setValues, id)

	// Build the WHERE clause. When guarding against resurrection, require the
	// stored row to be live so a concurrent soft-delete makes this a no-op.
	// The predicate is a constant (no extra bind values).
	whereClause := fmt.Sprintf("id = %s", s.dbSpec.Placeholder(len(setValues)))
	if guardNotDeleted && hasDeletedField(descriptor) {
		whereClause += fmt.Sprintf(" AND (%s = 0 OR %s IS NULL)", deletedFilterColumn, deletedFilterColumn)
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return false, err
	}

	// Build and execute UPDATE query
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s",
		quotedTable,
		strings.Join(setClauses, ", "),
		whereClause,
	)

	result, err := s.exec.ExecContext(ctx, query, setValues...)
	if err != nil {
		return false, fmt.Errorf("failed to update %s: %w", tableName, err)
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		if guardNotDeleted {
			// The row was concurrently soft-deleted (or never existed). Skip the
			// write rather than resurrecting it, and don't sync denormalizations
			// or regenerate embeddings for a row we did not touch.
			return false, nil
		}
		return false, fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)
	}

	// Sync any registered TEXT[] denormalization columns from msg. Routes
	// through s.exec so it participates in the surrounding transaction
	// when the outer Update wrapped us in one.
	if err := s.syncArrayColumns(ctx, msg, id); err != nil {
		return false, fmt.Errorf("sync array columns on update: %w", err)
	}

	// Trigger async embedding regeneration for every configured variant.
	s.queueEmbeddingGeneration(ctx, tableName, msg, id)

	return true, nil
}

// Delete removes a record from the database by its ID.
func (s *ProtoSQLStorage) Delete(ctx context.Context, msg proto.Message) error {
	msgReflect := msg.ProtoReflect()
	descriptor := msgReflect.Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Get the ID field
	fields := descriptor.Fields()
	if fields.Len() == 0 {
		return fmt.Errorf("message has no fields")
	}

	idField := fields.Get(0)
	if string(idField.Name()) != "id" {
		return fmt.Errorf("first field is not named 'id'")
	}

	id := msgReflect.Get(idField).String()
	if id == "" {
		return fmt.Errorf("id field is empty")
	}

	// Build DELETE query
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = %s`, quotedTable, s.dbSpec.Placeholder(1))

	// Execute the delete
	result, err := s.exec.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete from database: %w", err)
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("record not found with id %s: %w", id, ErrRecordNotFound)
	}

	return nil
}

// DeleteByField deletes all records from the given table where fieldName matches value.
// Returns the number of rows deleted.
func (s *ProtoSQLStorage) DeleteByField(ctx context.Context, tableName, fieldName string, value any) (int64, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return 0, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`DELETE FROM %s WHERE %s = %s`, quotedTable, quotedField, s.dbSpec.Placeholder(1))
	result, err := s.exec.ExecContext(ctx, query, value)
	if err != nil {
		return 0, fmt.Errorf("failed to delete from %s by %s: %w", tableName, fieldName, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rows, nil
}

// DeleteByFieldIn deletes all records from the given table where fieldName is in the provided values.
// Returns the number of rows deleted. If values is empty, returns 0 without executing a query.
func (s *ProtoSQLStorage) DeleteByFieldIn(ctx context.Context, tableName, fieldName string, values []string) (int64, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return 0, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
		args[i] = v
	}
	query := fmt.Sprintf(`DELETE FROM %s WHERE %s IN (%s)`,
		quotedTable, quotedField, strings.Join(placeholders, ", "))
	result, err := s.exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete from %s by %s IN: %w", tableName, fieldName, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rows, nil
}

// QueryDistinctField returns all distinct non-empty values of the given field in the given table.
func (s *ProtoSQLStorage) QueryDistinctField(ctx context.Context, tableName, fieldName string) ([]string, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL AND %s != ''`,
		quotedField, quotedTable, quotedField, quotedField)
	rows, err := s.exec.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query distinct %s from %s: %w", fieldName, tableName, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("failed to scan distinct value: %w", err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating distinct values: %w", err)
	}
	return values, nil
}

// MaxInt64FieldByGroup returns the maximum int64 value of valueField
// grouped by groupField, restricted to rows whose groupField is in the
// supplied groupValues. Powers "latest activity per parent" lookups in
// a single round-trip so callers can avoid an N+1 over parent IDs.
//
// Missing parent IDs (no matching rows) are absent from the result map
// — callers decide how to render that absence (e.g. sort to the end).
func (s *ProtoSQLStorage) MaxInt64FieldByGroup(
	ctx context.Context,
	tableName string,
	groupField string,
	valueField string,
	groupValues []string,
) (map[string]int64, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedGroup, err := quoteIdent(groupField)
	if err != nil {
		return nil, err
	}
	quotedValue, err := quoteIdent(valueField)
	if err != nil {
		return nil, err
	}
	if len(groupValues) == 0 {
		return map[string]int64{}, nil
	}
	placeholders := make([]string, len(groupValues))
	args := make([]any, len(groupValues))
	for i, v := range groupValues {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
		args[i] = v
	}
	query := fmt.Sprintf(
		`SELECT %s, MAX(%s) FROM %s WHERE %s IN (%s) GROUP BY %s`,
		quotedGroup, quotedValue, quotedTable, quotedGroup,
		strings.Join(placeholders, ", "), quotedGroup,
	)
	rows, err := s.exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate max %s by %s from %s: %w",
			valueField, groupField, tableName, err)
	}
	defer rows.Close()

	out := make(map[string]int64, len(groupValues))
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan max-by-group row: %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating max-by-group rows: %w", err)
	}
	return out, nil
}

// CountByField returns the number of rows in the given table matching a field value.
func (s *ProtoSQLStorage) CountByField(ctx context.Context, tableName, fieldName, value string) (int64, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return 0, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = %s`, quotedTable, quotedField, s.dbSpec.Placeholder(1))
	var count int64
	if err := s.exec.QueryRowContext(ctx, query, value).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count %s in %s: %w", fieldName, tableName, err)
	}
	return count, nil
}

// DecrementFieldIfPositive atomically decrements an integer column for the row
// with the given ID, but only when the current value is greater than zero.
// Returns (true, nil) when the decrement succeeded, (false, nil) when the field
// was already zero (no update performed), or (false, err) on database error.
// The caller is responsible for supplying a registered table name.
func (s *ProtoSQLStorage) DecrementFieldIfPositive(ctx context.Context, tableName, id, fieldName string) (bool, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return false, err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return false, err
	}
	query := fmt.Sprintf(
		`UPDATE %s SET %s = %s - 1 WHERE id = %s AND %s > 0`,
		quotedTable,
		quotedField, quotedField,
		s.dbSpec.Placeholder(1),
		quotedField,
	)
	result, err := s.exec.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to decrement %s on %s: %w", fieldName, tableName, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return rows > 0, nil
}

// IncrementField atomically increments an integer column for the row with the
// given ID. The caller is responsible for supplying a registered table name.
func (s *ProtoSQLStorage) IncrementField(ctx context.Context, tableName, id, fieldName string) error {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		`UPDATE %s SET %s = %s + 1 WHERE id = %s`,
		quotedTable,
		quotedField, quotedField,
		s.dbSpec.Placeholder(1),
	)
	_, err = s.exec.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment %s on %s: %w", fieldName, tableName, err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *ProtoSQLStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// PoolStats returns a snapshot of the connection pool statistics, for the
// periodic statslog pool-stats log line that feeds the db_pool_* log-based
// metrics (#1613).
func (s *ProtoSQLStorage) PoolStats() sql.DBStats {
	return s.db.Stats()
}

// CheckHealth verifies the database connection is healthy by pinging it.
func (s *ProtoSQLStorage) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	start := time.Now()

	// Get pool stats for metadata
	stats := s.db.Stats()
	status := &health.Status{
		Name:    "database",
		Backend: "postgresql",
		Metadata: map[string]string{
			"open_connections": fmt.Sprintf("%d", stats.OpenConnections),
			"in_use":           fmt.Sprintf("%d", stats.InUse),
			"idle":             fmt.Sprintf("%d", stats.Idle),
			"wait_count":       fmt.Sprintf("%d", stats.WaitCount),
			"wait_duration_ms": fmt.Sprintf("%d", stats.WaitDuration.Milliseconds()),
		},
	}

	err := s.db.PingContext(ctx)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	}

	return []*health.Status{status}, nil
}
