package storage

import (
	"context"
	"fmt"

	"github.com/lib/pq"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldExtractor returns the values that should be denormalized into a
// TEXT[] column for the given message. Implementations are pure — they
// read msg and nothing else.
//
// For a top-level repeated-string proto field, use [TopLevelRepeatedString].
// For nested fields (e.g. inside a oneof), supply a typed extractor.
type FieldExtractor func(msg proto.Message) []string

// arrayColumnReg pairs a TEXT[] column on a row with the extractor that
// produces the values to write into it from the row's proto.
type arrayColumnReg struct {
	columnName string
	extract    FieldExtractor
}

// RegisterArrayColumn declares that the named TEXT[] column on the table
// for msgType denormalizes the values returned by extract. After registration,
// [ProtoSQLStorage.Insert] and [ProtoSQLStorage.Update] keep the column in
// sync with the proto atomically (via [ProtoSQLStorage.WithTx]) on every
// write of msgType.
//
// Call once per (msgType, columnName) at storage construction time, before
// any writes of msgType. Use [InitializeArrayColumn] at startup to add the
// column + GIN index — the two steps are independent so registration can
// reuse columns created elsewhere.
//
// binary_proto remains the source of truth; the denormalized column exists
// purely to make WHERE $1 = ANY(col) queries efficient via a GIN index.
func (s *ProtoSQLStorage) RegisterArrayColumn(msgType proto.Message, columnName string, extract FieldExtractor) {
	if extract == nil {
		panic("RegisterArrayColumn: extract must not be nil")
	}
	typeName := string(msgType.ProtoReflect().Descriptor().FullName())
	if s.arrayColumns == nil {
		s.arrayColumns = make(map[string][]arrayColumnReg)
	}
	s.arrayColumns[typeName] = append(s.arrayColumns[typeName], arrayColumnReg{
		columnName: columnName,
		extract:    extract,
	})
}

// TopLevelRepeatedString returns a [FieldExtractor] for a repeated-string
// proto field at the top level of the message. The returned extractor is
// safe to call on any message — it returns nil for messages whose type
// doesn't have the field, so registrations are robust to schema drift.
//
// Panics at construction time (not at call time) if fieldName doesn't
// resolve to a repeated string on msgType — catches typos at startup.
func TopLevelRepeatedString(msgType proto.Message, fieldName protoreflect.Name) FieldExtractor {
	descriptor := msgType.ProtoReflect().Descriptor()
	field := descriptor.Fields().ByName(fieldName)
	if field == nil {
		panic(fmt.Sprintf("TopLevelRepeatedString: %s has no field %q",
			descriptor.FullName(), fieldName))
	}
	if !field.IsList() || field.Kind() != protoreflect.StringKind {
		panic(fmt.Sprintf("TopLevelRepeatedString: %s.%s must be a repeated string",
			descriptor.FullName(), fieldName))
	}
	fieldNum := field.Number()
	return func(msg proto.Message) []string {
		if msg == nil {
			return nil
		}
		reflect := msg.ProtoReflect()
		f := reflect.Descriptor().Fields().ByNumber(fieldNum)
		if f == nil {
			return nil
		}
		list := reflect.Get(f).List()
		if list.Len() == 0 {
			return nil
		}
		out := make([]string, list.Len())
		for i := 0; i < list.Len(); i++ {
			out[i] = list.Get(i).String()
		}
		return out
	}
}

// inTransaction reports whether s is scoped to an active transaction.
// True when s.exec is the [*InstrumentedTx] returned by [ProtoSQLStorage.WithTx];
// false at the top level (where s.exec is [*InstrumentedDB]).
func (s *ProtoSQLStorage) inTransaction() bool {
	_, ok := s.exec.(*InstrumentedTx)
	return ok
}

// hasArrayColumns reports whether msg has any registered array-column
// denormalizations. Used by [ProtoSQLStorage.Insert] and
// [ProtoSQLStorage.Update] to decide whether the write needs to be wrapped
// in a transaction.
func (s *ProtoSQLStorage) hasArrayColumns(msg proto.Message) bool {
	typeName := string(msg.ProtoReflect().Descriptor().FullName())
	return len(s.arrayColumns[typeName]) > 0
}

// isTypeRegistered reports whether msg's proto type has a table mapping
// in s.allowedTypes. Used by the per-table Initialize / Register helpers
// to skip types not in the current TypeConfig list (test setups with a
// narrow type registration would otherwise fail startup on an
// allowedTypes lookup).
func (s *ProtoSQLStorage) isTypeRegistered(msg proto.Message) bool {
	typeName := string(msg.ProtoReflect().Descriptor().FullName())
	_, ok := s.allowedTypes[typeName]
	return ok
}

// syncArrayColumns writes every registered TEXT[] column for the row with
// id == id from the values returned by each registration's extractor.
// Routes through s.exec so it participates in any active transaction.
func (s *ProtoSQLStorage) syncArrayColumns(ctx context.Context, msg proto.Message, id string) error {
	typeName := string(msg.ProtoReflect().Descriptor().FullName())
	regs := s.arrayColumns[typeName]
	if len(regs) == 0 {
		return nil
	}
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return fmt.Errorf("syncArrayColumns: type %s not registered for storage", typeName)
	}
	for _, reg := range regs {
		values := reg.extract(msg)
		if err := s.updateArrayColumn(ctx, tableName, reg.columnName, id, values); err != nil {
			return err
		}
	}
	return nil
}

// updateArrayColumn writes values to the named TEXT[] column on tableName
// where id matches. Routes through s.exec so it participates in any
// active transaction.
func (s *ProtoSQLStorage) updateArrayColumn(ctx context.Context, tableName, columnName, id string, values []string) error {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return err
	}
	quotedColumn, err := quoteIdent(columnName)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`UPDATE %s SET %s = %s WHERE id = %s`,
		quotedTable, quotedColumn, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	if _, err := s.exec.ExecContext(ctx, query, pq.Array(values), id); err != nil {
		return fmt.Errorf("update %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

// QueryByArrayContains retrieves proto messages where the named TEXT[]
// denormalization column contains value (WHERE $1 = ANY(col)). The column
// must have been previously initialized via [InitializeArrayColumn] so
// the GIN index is in place; without it the query degrades to a
// sequential scan.
//
// By default, soft-deleted rows are excluded. Pass
// [QueryOptions]{IncludeDeleted: true} to include them.
func (s *ProtoSQLStorage) QueryByArrayContains(
	ctx context.Context,
	columnName string,
	value string,
	msgType proto.Message,
	opts ...QueryOptions,
) ([]proto.Message, error) {
	quotedColumn, err := quoteIdent(columnName)
	if err != nil {
		return nil, err
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
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT binary_proto FROM %s WHERE %s = ANY(%s)%s`,
		quotedTable, s.dbSpec.Placeholder(1), quotedColumn, buildDeletedFilter(descriptor, opt))

	rows, err := s.exec.QueryContext(ctx, query, value)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s by array column %s: %w", tableName, columnName, err)
	}
	defer rows.Close()

	var messages []proto.Message
	for rows.Next() {
		var protoData []byte
		if err := rows.Scan(&protoData); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
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
