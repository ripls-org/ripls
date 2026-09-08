// Batch storage operations: GetByIDs, QueryByFieldIn, and InsertBatch.

package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// maxBatchGetSize is the maximum number of IDs per batch query to stay within
// PostgreSQL's parameter limit.
const maxBatchGetSize = 500

// maxBatchInsertSize is the maximum number of rows per batch INSERT.
const maxBatchInsertSize = 100

// GetByIDs retrieves multiple proto messages by their IDs in a single query.
// Returns a map of id → message for easy lookup. Missing IDs are silently omitted.
// By default, soft-deleted items are excluded.
func (s *ProtoSQLStorage) GetByIDs(ctx context.Context, ids []string, msgType proto.Message, opts ...QueryOptions) (map[string]proto.Message, error) {
	if len(ids) == 0 {
		return map[string]proto.Message{}, nil
	}

	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	var opt *QueryOptions
	if len(opts) > 0 {
		opt = &opts[0]
	}
	deletedFilter := buildDeletedFilter(descriptor, opt)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}

	result := make(map[string]proto.Message, len(ids))

	// Process in chunks to stay within PostgreSQL parameter limits
	for start := 0; start < len(ids); start += maxBatchGetSize {
		end := start + maxBatchGetSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = s.dbSpec.Placeholder(i + 1)
			args[i] = id
		}

		query := fmt.Sprintf(`SELECT id, binary_proto FROM %s WHERE id IN (%s)%s`,
			quotedTable, strings.Join(placeholders, ", "), deletedFilter)

		// Closure so `defer rows.Close()` fires at the end of THIS chunk. A
		// function-scoped defer would hold every chunk's result set open until
		// the whole batch finished; hand-placing Close on each error path (what
		// this used to do) leaks the moment someone adds a return.
		if err := func() error {
			rows, err := s.db.QueryContext(ctx, query, args...)
			if err != nil {
				return fmt.Errorf("failed to batch get from %s: %w", tableName, err)
			}
			defer rows.Close()

			for rows.Next() {
				var id string
				var protoData []byte
				if err := rows.Scan(&id, &protoData); err != nil {
					return fmt.Errorf("failed to scan row: %w", err)
				}

				msg := proto.Clone(msgType)
				proto.Reset(msg)
				if err := proto.Unmarshal(protoData, msg); err != nil {
					return fmt.Errorf("failed to unmarshal proto: %w", err)
				}
				result[id] = msg
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("error iterating rows: %w", err)
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// QueryByFieldIn retrieves proto messages where the given field matches any of the provided values.
// By default, soft-deleted items are excluded.
func (s *ProtoSQLStorage) QueryByFieldIn(ctx context.Context, fieldName string, values []string, msgType proto.Message, opts ...QueryOptions) ([]proto.Message, error) {
	quotedField, err := quoteIdent(fieldName)
	if err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, nil
	}

	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	var opt *QueryOptions
	if len(opts) > 0 {
		opt = &opts[0]
	}
	deletedFilter := buildDeletedFilter(descriptor, opt)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}

	var messages []proto.Message

	// Process in chunks to stay within PostgreSQL parameter limits
	for start := 0; start < len(values); start += maxBatchGetSize {
		end := start + maxBatchGetSize
		if end > len(values) {
			end = len(values)
		}
		chunk := values[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, v := range chunk {
			placeholders[i] = s.dbSpec.Placeholder(i + 1)
			args[i] = v
		}

		query := fmt.Sprintf(`SELECT binary_proto FROM %s WHERE %s IN (%s)%s`,
			quotedTable, quotedField, strings.Join(placeholders, ", "), deletedFilter)

		// Closure so `defer rows.Close()` is scoped to this chunk — see the
		// equivalent comment in BatchGetByIDs above.
		if err := func() error {
			rows, err := s.db.QueryContext(ctx, query, args...)
			if err != nil {
				return fmt.Errorf("failed to query %s by %s IN: %w", tableName, fieldName, err)
			}
			defer rows.Close()

			for rows.Next() {
				var protoData []byte
				if err := rows.Scan(&protoData); err != nil {
					return fmt.Errorf("failed to scan row: %w", err)
				}

				msg := proto.Clone(msgType)
				proto.Reset(msg)
				if err := proto.Unmarshal(protoData, msg); err != nil {
					return fmt.Errorf("failed to unmarshal proto: %w", err)
				}
				messages = append(messages, msg)
			}
			return rows.Err()
		}(); err != nil {
			return nil, err
		}
	}

	return messages, nil
}

// InsertBatch inserts multiple proto messages of the same type in a single multi-row INSERT.
// All messages must be the same protobuf type. Returns the slice of generated/existing IDs.
func (s *ProtoSQLStorage) InsertBatch(ctx context.Context, msgs []proto.Message) ([]string, error) {
	if len(msgs) == 0 {
		return nil, nil
	}

	// Determine type from first message
	first := msgs[0]
	descriptor := first.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Verify all messages are the same type
	for i := 1; i < len(msgs); i++ {
		otherType := string(msgs[i].ProtoReflect().Descriptor().FullName())
		if otherType != typeName {
			return nil, fmt.Errorf("mixed types in batch: %s and %s", typeName, otherType)
		}
	}

	allIDs := make([]string, 0, len(msgs))

	// Process in chunks
	for start := 0; start < len(msgs); start += maxBatchInsertSize {
		end := start + maxBatchInsertSize
		if end > len(msgs) {
			end = len(msgs)
		}
		chunk := msgs[start:end]

		ids, err := s.insertBatchChunk(ctx, chunk, tableName, descriptor)
		if err != nil {
			return nil, err
		}
		allIDs = append(allIDs, ids...)
	}

	return allIDs, nil
}

// insertBatchChunk inserts a single chunk of messages using a multi-row INSERT.
func (s *ProtoSQLStorage) insertBatchChunk(ctx context.Context, msgs []proto.Message, tableName string, descriptor protoreflect.MessageDescriptor) ([]string, error) {
	fields := descriptor.Fields()
	hasIDField := fields.Len() > 0 && string(fields.Get(0).Name()) == "id"

	// Extract columns from first message to determine shape
	sampleColumns, _ := extractFieldValues(msgs[0].ProtoReflect())
	sampleColumns = append(sampleColumns, "binary_proto")
	colCount := len(sampleColumns)

	var allValues []any
	ids := make([]string, len(msgs))

	for i, msg := range msgs {
		msgReflect := msg.ProtoReflect()
		columns, values := extractFieldValues(msgReflect)

		// Generate ID if needed
		if hasIDField && fmt.Sprintf("%v", values[0]) == "" {
			newID := uuid.New().String()
			values[0] = newID
			msgReflect.Set(fields.Get(0), protoreflect.ValueOfString(newID))
		}

		ids[i] = fmt.Sprintf("%v", values[0])

		// Serialize proto
		protoData, err := proto.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal proto at index %d: %w", i, err)
		}

		_ = columns // columns match sampleColumns
		values = append(values, protoData)
		allValues = append(allValues, values...)
	}

	// Build multi-row VALUES clause
	var rowPlaceholders []string
	for row := 0; row < len(msgs); row++ {
		var colPlaceholders []string
		for col := 0; col < colCount; col++ {
			paramIdx := row*colCount + col + 1
			colPlaceholders = append(colPlaceholders, s.dbSpec.Placeholder(paramIdx))
		}
		rowPlaceholders = append(rowPlaceholders, "("+strings.Join(colPlaceholders, ", ")+")")
	}

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedColumns, err := quoteIdents(sampleColumns)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`INSERT INTO %s (%s) VALUES %s`,
		quotedTable,
		strings.Join(quotedColumns, ", "),
		strings.Join(rowPlaceholders, ", "),
	)

	_, err = s.db.ExecContext(ctx, query, allValues...)
	if err != nil {
		return nil, fmt.Errorf("failed to batch insert into %s: %w", tableName, err)
	}

	return ids, nil
}
