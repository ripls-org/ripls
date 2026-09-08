package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PostgreSQLSpecifics implements DatabaseSpecifics.
type PostgreSQLSpecifics struct{}

// sqlTypeInteger is the column type shared by 32-bit numeric kinds and enums.
const sqlTypeInteger = "INTEGER"

func (d PostgreSQLSpecifics) SQLType(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.StringKind:
		return "TEXT"
	case protoreflect.Int32Kind:
		return sqlTypeInteger
	case protoreflect.Int64Kind:
		return "BIGINT"
	case protoreflect.Uint32Kind:
		return sqlTypeInteger
	case protoreflect.Uint64Kind:
		return "BIGINT"
	case protoreflect.FloatKind:
		return "REAL"
	case protoreflect.DoubleKind:
		return "DOUBLE PRECISION"
	case protoreflect.BoolKind:
		return "BOOLEAN"
	case protoreflect.BytesKind:
		return "BYTEA"
	case protoreflect.EnumKind:
		return sqlTypeInteger
	default:
		return "TEXT"
	}
}

func (d PostgreSQLSpecifics) BinaryColumnType() string {
	return "BYTEA"
}

func (d PostgreSQLSpecifics) Placeholder(position int) string {
	return fmt.Sprintf("$%d", position)
}

func (d PostgreSQLSpecifics) OpenDatabase(connectionString string) (*sql.DB, error) {
	return sql.Open("postgres", connectionString)
}

// GetTableColumns returns the set of column names that exist in the given table.
func (d PostgreSQLSpecifics) GetTableColumns(ctx context.Context, db *sql.DB, tableName string) (map[string]bool, error) {
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = $1
	`
	rows, err := db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, err
		}
		columns[columnName] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

// OpenPostgreSQLDatabaseReadOnly opens storage over an existing PostgreSQL
// schema without running any DDL or migration — for least-privilege
// (SELECT-only) connections, e.g. prod content extraction
// (server/cmd/walkthrough-export) via the read-only database role. The normal
// InitializePostgreSQLDatabase path executes CREATE/ALTER/backfill statements
// such a role cannot run. Reads are full-fidelity (binary_proto decode);
// writes through this handle fail at the database when the role is read-only.
func OpenPostgreSQLDatabaseReadOnly(ctx context.Context, connectionString string, typeConfigs []TypeConfig) (*ProtoSQLStorage, error) {
	return openDatabaseNoDDL(ctx, PostgreSQLSpecifics{}, connectionString, typeConfigs)
}

// InitializePostgreSQLDatabase creates a new PostgreSQL storage instance.
// It enables pgvector extension for vector similarity search.
func InitializePostgreSQLDatabase(ctx context.Context, connectionString string, typeConfigs []TypeConfig) (*ProtoSQLStorage, error) {
	driver := PostgreSQLSpecifics{}
	storage, err := initializeDatabase(ctx, driver, connectionString, typeConfigs)
	if err != nil {
		return nil, err
	}

	// Enable pgvector extension for vector similarity search.
	// This is idempotent and only needs to run once per database.
	if err := storage.EnablePgvector(ctx); err != nil {
		storage.Close()
		return nil, err
	}

	// Schema DDL — idempotent, one-time-effective. Adds participant_ids
	// TEXT[] columns + GIN indices on Story and chat_conversation. After
	// the schema is established these are no-ops.
	if err := storage.InitializeStoryArrayColumns(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}
	if err := storage.InitializeChatConversationArrayColumns(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Backfill community.owner_user_id and install the active-row owner CHECK
	// constraint required by the soft-delete work in #1619.
	if err := storage.InitializeCommunityOwnerUserID(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Backfill Gear/Request/Experience.conversation_id from the deprecated
	// CommunityGear/CommunityRequest/CommunityExperience join-table fields
	// (#1332). Idempotent; returns zero updated rows once the backfill is
	// complete on every subsequent startup.
	if err := storage.InitializeCommunityConversationIDMigration(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Verify community_user has no duplicate (community_id, user_id) rows
	// and install the UNIQUE constraint that the soft-delete + rejoin work
	// in #1619 relies on.
	if err := storage.InitializeCommunityUserUniqueness(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Add the snapshot_member_user_ids TEXT[] column on community
	// (#1722 / #1717). Idempotent DDL only.
	if err := storage.InitializeCommunityArrayColumns(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Add the media_ids TEXT[] denormalization columns + GIN indices on
	// User, Gear, Experience, Request, Community, Story, and ChatMessage
	// (#1529). Idempotent DDL only.
	if err := storage.InitializeMediaIdsColumns(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Drop the legacy User.media_id column (#2083 Phase 4b). Idempotent —
	// no-op once the column is gone. Must run AFTER Phase 4a is fully
	// deployed; the helper file itself can be removed in a future cleanup
	// once every environment has run it at least once.
	if err := storage.DropUserMediaIDColumn(context.Background()); err != nil {
		storage.Close()
		return nil, err
	}

	// Register the per-write auto-sync for every denormalized TEXT[]
	// column. Required on every server start regardless of schema
	// state — without it, Insert / Update of these types leaves
	// indexed columns stale and the corresponding read queries
	// silently miss rows. See [ProtoSQLStorage.RegisterArrayColumn].
	storage.RegisterStoryArrayColumns()
	storage.RegisterChatConversationArrayColumns()
	storage.RegisterCommunityArrayColumns()
	storage.RegisterMediaIdsArrayColumns()

	return storage, nil
}
