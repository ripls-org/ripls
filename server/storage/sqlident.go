// SQL identifier quoting — the single sanctioned way to put a table or column
// name into a query string. See docs/server/conventions.md § SQL Safety.

package storage

import (
	"fmt"

	"github.com/lib/pq"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// quoteIdent validates name as a SQL identifier and returns it quoted for
// PostgreSQL.
//
// This is the only approved way to interpolate an identifier into a query.
// Everything else — values of any kind — must be bound through
// [DatabaseSpecifics.Placeholder]. `server/cmd/check-sql` (npm run lint:go:sql)
// fails the build on any other shape.
//
// Do not reach for fmt's %q instead: %q applies Go string quoting, which
// escapes an embedded double quote as \" and mangles backslashes, whereas
// PostgreSQL requires the quote doubled ("") and treats backslashes literally.
// The two agree only on identifiers that need no escaping at all, which is
// exactly the set validateIdentifier already restricts us to — so %q looks
// correct right up until the day the validation is skipped.
func quoteIdent(name string) (string, error) {
	if err := validateIdentifier(name); err != nil {
		return "", err
	}
	return pq.QuoteIdentifier(name), nil
}

// quoteIdents applies [quoteIdent] to every name, returning the first error.
// Used for column lists in INSERT and UPDATE statements.
func quoteIdents(names []string) ([]string, error) {
	quoted := make([]string, len(names))
	for i, name := range names {
		q, err := quoteIdent(name)
		if err != nil {
			return nil, err
		}
		quoted[i] = q
	}
	return quoted, nil
}

// quotedTableFor looks up the table registered for a proto type name and
// returns it quoted, ready to interpolate.
//
// The lookup and the quoting belong together: `s.allowedTypes[...]` is the
// closed set that makes a table name trustworthy, and quoteIdent is what puts
// it in the query. Splitting them is what let the two drift apart at ~20 call
// sites before #2795.
func (s *ProtoSQLStorage) quotedTableFor(typeName string) (string, error) {
	table, ok := s.allowedTypes[typeName]
	if !ok {
		return "", fmt.Errorf("message type %s is not registered for storage", typeName)
	}
	return quoteIdent(table)
}

// qualify returns alias.column with the column quoted — e.g. `e."name"`. The
// alias must be a short literal chosen by the caller at the call site, never a
// variable, so it is not validated here; check-sql rejects a non-constant
// alias.
func qualify(alias, column string) (string, error) {
	q, err := quoteIdent(column)
	if err != nil {
		return "", err
	}
	return alias + "." + q, nil
}

// validateSchemaIdentifiers checks every identifier that the storage layer
// derives from proto descriptors — table names and flattened column names —
// before any of them reaches a query.
//
// This runs once, at startup, from initializeDatabase. Doing it here rather
// than per query is what lets the hot paths treat a quoteIdent error as
// impossible-but-handled instead of panicking inside a request: a proto
// descriptor that yields an invalid SQL identifier is a build-time mistake, and
// the server should refuse to start rather than fail one RPC at a time.
func validateSchemaIdentifiers(typeConfigs []TypeConfig) error {
	for _, config := range typeConfigs {
		if err := validateIdentifier(config.TableName); err != nil {
			return fmt.Errorf("storage type %T has an unusable table name: %w",
				config.MessageType, err)
		}

		descriptor := config.MessageType.ProtoReflect().Descriptor()
		for _, field := range flattenFields(descriptor, "") {
			if err := validateIdentifier(field.columnName); err != nil {
				return fmt.Errorf("table %q: proto field yields an unusable column name: %w",
					config.TableName, err)
			}
		}

		for _, embedding := range config.Embeddings {
			for _, f := range embedding.Fields {
				// Fields entries are proto field paths, which may be dotted for
				// nested messages; embeddingTextColumn maps one to the flattened
				// column that actually holds it.
				if err := validateIdentifier(embeddingTextColumn(f)); err != nil {
					return fmt.Errorf("table %q: embedding config names an unusable column: %w",
						config.TableName, err)
				}
			}
		}
	}
	return nil
}

// validateMessageColumns checks the flattened column names of a single
// descriptor. Used by the array-column and embedding registration paths, which
// accept proto types outside the startup typeConfigs sweep.
func validateMessageColumns(descriptor protoreflect.MessageDescriptor) error {
	for _, field := range flattenFields(descriptor, "") {
		if err := validateIdentifier(field.columnName); err != nil {
			return fmt.Errorf("message %s: %w", descriptor.FullName(), err)
		}
	}
	return nil
}
