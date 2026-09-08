// Types, constants, and helper functions shared across the protosql storage layer.

package storage

import (
	"context"
	"database/sql"
	"math"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// SemanticSearchMinSimilarity is the minimum cosine similarity threshold for semantic search results.
// Results below this threshold are filtered out. Based on production testing:
// - Good matches: ~0.55-0.65+ similarity
// - Moderately related: ~0.4-0.5 similarity
// - Unrelated items: ~0.3-0.4 similarity (noise floor)
// A threshold of 0.5 filters out noise while keeping relevant results.
const SemanticSearchMinSimilarity = 0.5

// DatabaseSpecifics defines the interface for database-specific operations.
type DatabaseSpecifics interface {
	// SQLType maps proto types to SQL types.
	SQLType(kind protoreflect.Kind) string
	// BinaryColumnType specifies the column type for storing binary protos.
	BinaryColumnType() string
	// Placeholder returns placeholder characters for query strings.
	Placeholder(position int) string
	// OpenDatabase opens a connection to the underlying database implementation.
	OpenDatabase(connectionString string) (*sql.DB, error)
	// GetTableColumns returns the set of column names that exist in the given table.
	GetTableColumns(ctx context.Context, db *sql.DB, tableName string) (map[string]bool, error)
}

// columnDefinition represents a column definition needed for a field.
type columnDefinition struct {
	name    string
	sqlType string
}

// flattenedField represents a flattened field with its SQL column name and metadata.
type flattenedField struct {
	columnName string
	kind       protoreflect.Kind
	fieldPath  []protoreflect.FieldDescriptor // Path from root to this field
}

// EmbeddingFieldConfig specifies one embedding to generate for a table: which
// fields to embed and, optionally, a variant that lets a table carry more than
// one embedding of the same model in separate columns.
type EmbeddingFieldConfig struct {
	// Fields to concatenate for embedding text (e.g., ["name", "description"])
	Fields []string
	// Variant distinguishes multiple embeddings of the same model on one table.
	// Empty is the default/primary embedding (column "vendor_model", used by
	// semantic search and backward-compatible). A non-empty variant appends
	// "_<variant>" to the column name (e.g. "vendor_model_name" for a name-only
	// embedding used by activity clustering, #2694). Must be a safe SQL
	// identifier.
	Variant string
	// SkipIndex omits the HNSW ANN index for this embedding. Set it for variants
	// that are only read by id (not similarity-searched), so we don't pay index
	// maintenance on every write for a column no query ranks by distance.
	SkipIndex bool
}

// TypeConfig defines the configuration for a storage type.
type TypeConfig struct {
	// The protobuf message type to store
	MessageType proto.Message
	// The table name to use (e.g., "user" for UserStored, "gear" for GearStored)
	TableName string
	// Optional embedding configurations. Empty means no embeddings. A table may
	// declare several (distinct Variant each) to carry multiple embeddings.
	Embeddings []*EmbeddingFieldConfig
}

// QueryOptions configures query behavior.
type QueryOptions struct {
	// IncludeDeleted returns soft-deleted items when true. Default (false) excludes them.
	IncludeDeleted bool
	// Limit caps the number of rows returned. Zero means no limit.
	Limit int
	// ForUpdate appends FOR UPDATE to the SELECT clause, acquiring a row-level
	// write lock for the duration of the enclosing transaction. MUST be used
	// inside a WithTx callback; GetByID returns ErrForUpdateOutsideTransaction
	// otherwise. Combine with IncludeDeleted: true when the caller needs to
	// reload and re-check soft-delete inside the same lock (the tx-guarded
	// async-writer pattern — see WithTx godoc and server/jobs/community_purge.go
	// Safeguard #5 as the canonical example).
	ForUpdate bool
}

// deletedFilterColumn is the flattened column name for the deleted_at timestamp.
// Items with deleted_deleted_at_unix_sec = 0 are not deleted.
const deletedFilterColumn = "deleted_deleted_at_unix_sec"

// SpatialQueryResult contains a proto message and its distance from the query point.
type SpatialQueryResult struct {
	Message        proto.Message
	DistanceMeters float64
	LatitudeDeg    float64
	LongitudeDeg   float64
}

// GearSearchResult represents a gear item from a search query with location data.
type GearSearchResult struct {
	Gear           *models.Gear
	Location       *models.Location
	DistanceMeters float64
	Similarity     float64 // Cosine similarity from semantic search (0-1), 0 for text search
}

// RequestSearchResult contains request data and computed distance from a search query.
type RequestSearchResult struct {
	Request        *models.Request
	Location       *models.Location
	DistanceMeters float64
	Similarity     float64 // Cosine similarity from semantic search (0-1), 0 for text search
}

// UnifiedSearchResult represents a search result with scoring information for any item type.
type UnifiedSearchResult struct {
	ItemType string // "gear", "request", or "experience"
	ID       string

	// Scoring
	CompositeScore     float64 // Composite score (server-computed, default sort order)
	SemanticSimilarity float64 // Vector similarity (0.0-1.0, or 1.0 for text search)
	DistanceMeters     float64 // Geographic distance from user

	// One of these will be populated based on ItemType
	Gear       *models.Gear
	Request    *models.Request
	Experience *models.Experience
	Location   *models.Location

	// Availability from community_gear join (only for gear items)
	Availability int32
}

// computeCompositeScore calculates the overall score from semantic and distance.
func computeCompositeScore(semanticScore, distanceMeters float64) float64 {
	const maxDistanceMeters = 50000.0
	distanceScore := 1.0 - math.Min(distanceMeters/maxDistanceMeters, 1.0)
	return 0.7*semanticScore + 0.3*distanceScore
}

// protoValueToInterface converts a protoreflect.Value to a Go interface{}.
func protoValueToInterface(field protoreflect.FieldDescriptor, value protoreflect.Value) any {
	switch field.Kind() {
	case protoreflect.StringKind:
		return value.String()
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return value.Int()
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return value.Uint()
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return value.Float()
	case protoreflect.BoolKind:
		return value.Bool()
	case protoreflect.BytesKind:
		return value.Bytes()
	case protoreflect.EnumKind:
		// Store the enum number, matching the INTEGER column type. Without
		// this case, enums fell through to value.String() — the decimal
		// number as text — into a TEXT column (#2840).
		return int64(value.Enum())
	default:
		return value.String()
	}
}

// ProtoSQLStorage provides generic SQL storage for arbitrary protobuf message types.
// An explicit list of allowed types is kept to prevent accidental storage of RPC types.
//
// db holds the connection-pool handle used for lifecycle (Close,
// Ping, Stats) and for starting transactions (BeginTx). exec is
// the executor every individual query is routed through — it
// equals db at the top level, but inside a WithTx callback it
// points at the InstrumentedTx so the queries participate in
// the transaction.
type ProtoSQLStorage struct {
	db               SQLDB
	exec             SQLExecutor
	dbSpec           DatabaseSpecifics
	allowedTypes     map[string]string                  // map from message type name to table name
	embeddingConfigs map[string][]*EmbeddingFieldConfig // map from table name to its embedding configs (one per variant)
	embedder         *embedding.Embedder                // embedder for generating embeddings
	embeddingDone    chan error                         // for testing: signals when async embedding completes (nil=success, non-nil=error)
	afterCommit      []func()                           // hooks run after a successful transaction commit

	// arrayColumns lists denormalized TEXT[] columns kept in sync with
	// repeated-field values on every Insert / Update of the proto type.
	// Keyed by proto type full name (e.g. "ripls.models.Gear"). Registered
	// via RegisterArrayColumn at storage construction time.
	arrayColumns map[string][]arrayColumnReg
}
