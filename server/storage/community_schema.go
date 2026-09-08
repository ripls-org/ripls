package storage

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// InitializeCommunityOwnerUserID backfills owner_user_id from creator_id
// for any community row where it is unset, then installs a CHECK
// constraint that enforces the single-owner invariant on active rows
// (a row is either soft-deleted or has a non-empty owner_user_id).
//
// Idempotent: existence checks gate every write, so the helper is safe
// to run on every startup. If the community table is not registered
// (e.g. test runs that do not include the Community type), the helper
// returns nil without touching the database.
//
// See docs/community_delete_and_leave.md §6.1 for the design rationale
// and #1644 for the migration plan.
func (s *ProtoSQLStorage) InitializeCommunityOwnerUserID(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With("operation", "InitializeCommunityOwnerUserID")

	tableExists, err := s.tableExists(ctx, "community")
	if err != nil {
		return fmt.Errorf("failed to check if community table exists: %w", err)
	}
	if !tableExists {
		logger.DebugContext(ctx, "community table not present, skipping owner_user_id initialization")
		return nil
	}

	columnExists, err := s.columnExists(ctx, "community", "owner_user_id")
	if err != nil {
		return fmt.Errorf("failed to check if owner_user_id column exists: %w", err)
	}
	if !columnExists {
		logger.DebugContext(ctx, "owner_user_id column not present, skipping owner_user_id initialization")
		return nil
	}

	if err := s.backfillCommunityOwnerUserID(ctx); err != nil {
		return fmt.Errorf("failed to backfill community.owner_user_id: %w", err)
	}

	if err := s.addCommunityOwnerRequiredConstraint(ctx); err != nil {
		return fmt.Errorf("failed to add community_owner_required constraint: %w", err)
	}

	logger.InfoContext(ctx, "community owner_user_id initialization complete")
	return nil
}

// backfillCommunityOwnerUserID copies creator_id into owner_user_id for
// every community row where owner_user_id is NULL or empty. Reads the
// row's binary_proto so the storage.Update call rewrites both the flat
// column and the serialized proto in lockstep.
func (s *ProtoSQLStorage) backfillCommunityOwnerUserID(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With("operation", "backfillCommunityOwnerUserID")
	start := time.Now()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, binary_proto
		FROM "community"
		WHERE owner_user_id IS NULL OR owner_user_id = ''
		LIMIT 10000
	`)
	if err != nil {
		return fmt.Errorf("failed to query communities: %w", err)
	}
	defer rows.Close()

	type pending struct {
		id          string
		binaryProto []byte
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.binaryProto); err != nil {
			return fmt.Errorf("failed to scan community row: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating communities: %w", err)
	}

	updated := 0
	for _, p := range todo {
		community := &models.Community{}
		if err := proto.Unmarshal(p.binaryProto, community); err != nil {
			logger.WarnContext(ctx, "failed to unmarshal community, skipping",
				"community_id", p.id, "error", err)
			continue
		}
		if community.OwnerUserId != "" {
			continue
		}
		if community.CreatorId == "" {
			logger.WarnContext(ctx, "community has no creator_id, skipping",
				"community_id", p.id)
			continue
		}
		community.OwnerUserId = community.CreatorId
		if err := s.Update(ctx, community); err != nil {
			logger.WarnContext(ctx, "failed to update community owner_user_id, skipping",
				"community_id", p.id, "error", err)
			continue
		}
		updated++
	}

	logger.InfoContext(ctx, "backfilled community owner_user_id",
		"community_count", updated,
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}

// addCommunityOwnerRequiredConstraint installs the
// community_owner_required CHECK constraint if it does not already
// exist. The constraint requires that every row is either soft-deleted
// (deleted_deleted_by_user_id populated) or has a non-empty
// owner_user_id.
func (s *ProtoSQLStorage) addCommunityOwnerRequiredConstraint(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With("operation", "addCommunityOwnerRequiredConstraint")

	exists, err := s.constraintExists(ctx, "community", "community_owner_required")
	if err != nil {
		return fmt.Errorf("failed to check community_owner_required existence: %w", err)
	}
	if exists {
		logger.DebugContext(ctx, "community_owner_required already present, skipping")
		return nil
	}

	logger.InfoContext(ctx, "adding community_owner_required CHECK constraint")
	_, err = s.db.ExecContext(ctx, `
		ALTER TABLE "community"
		ADD CONSTRAINT community_owner_required
		CHECK (
			(deleted_deleted_by_user_id IS NOT NULL AND deleted_deleted_by_user_id <> '')
			OR
			(owner_user_id IS NOT NULL AND owner_user_id <> '')
		)
	`)
	if err != nil {
		return fmt.Errorf("ALTER TABLE community ADD CONSTRAINT community_owner_required: %w", err)
	}
	return nil
}

// InitializeCommunityUserUniqueness verifies that no duplicate
// (community_id, user_id) rows exist in community_user, then installs a
// UNIQUE constraint on the pair. Failure on duplicates is fatal —
// duplicates indicate a data-integrity issue that needs human attention,
// not silent merging.
func (s *ProtoSQLStorage) InitializeCommunityUserUniqueness(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With("operation", "InitializeCommunityUserUniqueness")

	tableExists, err := s.tableExists(ctx, "community_user")
	if err != nil {
		return fmt.Errorf("failed to check if community_user table exists: %w", err)
	}
	if !tableExists {
		logger.DebugContext(ctx, "community_user table not present, skipping uniqueness initialization")
		return nil
	}

	if err := s.assertNoCommunityUserDuplicates(ctx, logger); err != nil {
		return err
	}

	if err := s.addCommunityUserUniqueConstraint(ctx, logger); err != nil {
		return fmt.Errorf("failed to add community_user_unique constraint: %w", err)
	}

	logger.InfoContext(ctx, "community_user uniqueness initialization complete")
	return nil
}

// assertNoCommunityUserDuplicates returns an error if any
// (community_id, user_id) pair appears more than once. Reports up to
// 10 offenders in the error so an operator can locate the bad data.
func (s *ProtoSQLStorage) assertNoCommunityUserDuplicates(ctx context.Context, logger *logging.Logger) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT community_id, user_id, COUNT(*)
		FROM "community_user"
		GROUP BY community_id, user_id
		HAVING COUNT(*) > 1
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("failed to query community_user duplicates: %w", err)
	}
	defer rows.Close()

	var offenders []string
	for rows.Next() {
		var communityID, userID string
		var count int
		if err := rows.Scan(&communityID, &userID, &count); err != nil {
			return fmt.Errorf("failed to scan duplicate row: %w", err)
		}
		offenders = append(offenders, fmt.Sprintf("(%s, %s) x %d", communityID, userID, count))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating duplicates: %w", err)
	}
	if len(offenders) > 0 {
		logger.ErrorContext(ctx, "duplicate community_user rows detected; manual cleanup required before constraint can be added",
			"offenders", offenders)
		return fmt.Errorf("community_user has duplicate (community_id, user_id) rows: %v", offenders)
	}
	return nil
}

// addCommunityUserUniqueConstraint installs the community_user_unique
// constraint if it does not already exist.
func (s *ProtoSQLStorage) addCommunityUserUniqueConstraint(ctx context.Context, logger *logging.Logger) error {
	exists, err := s.constraintExists(ctx, "community_user", "community_user_unique")
	if err != nil {
		return fmt.Errorf("failed to check community_user_unique existence: %w", err)
	}
	if exists {
		logger.DebugContext(ctx, "community_user_unique already present, skipping")
		return nil
	}

	logger.InfoContext(ctx, "adding community_user_unique constraint")
	_, err = s.db.ExecContext(ctx, `
		ALTER TABLE "community_user"
		ADD CONSTRAINT community_user_unique
		UNIQUE (community_id, user_id)
	`)
	if err != nil {
		return fmt.Errorf("ALTER TABLE community_user ADD CONSTRAINT community_user_unique: %w", err)
	}
	return nil
}

// tableExists checks information_schema for a table by its lowercase
// name. PostgreSQL stores unquoted identifiers in lowercase.
func (s *ProtoSQLStorage) tableExists(ctx context.Context, tableName string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

// columnExists checks information_schema for a column by its
// (lowercase) table and column name.
func (s *ProtoSQLStorage) columnExists(ctx context.Context, tableName, columnName string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)
	`, tableName, columnName).Scan(&exists)
	return exists, err
}

// constraintExists checks pg_constraint for a constraint by name on a
// specific table. Looking up by both name and table avoids collisions
// across tables that happen to use the same constraint name.
func (s *ProtoSQLStorage) constraintExists(ctx context.Context, tableName, constraintName string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint
			WHERE conname = $1
			  AND conrelid = ('"' || $2 || '"')::regclass
		)
	`, constraintName, tableName).Scan(&exists)
	return exists, err
}

// communitySnapshotMemberIds extracts the eligible-restorer set from a
// Community's DeletedSnapshot. Used as the [FieldExtractor] for the
// snapshot_member_user_ids TEXT[] denormalization column. Live rows
// (no DeletedSnapshot) return nil and the column ends up NULL —
// functionally equivalent to the legacy bespoke pattern, where the
// backfill filtered on deleted_at and live rows were never written.
func communitySnapshotMemberIds(msg proto.Message) []string {
	c, ok := msg.(*models.Community)
	if !ok || c == nil {
		return nil
	}
	return c.GetDeletedSnapshot().GetMemberUserIds()
}

// InitializeCommunityArrayColumns idempotently runs the DDL for every
// denormalized TEXT[] column on the Community type — currently just
// snapshot_member_user_ids, which powers FindCommunitiesEligibleForRestore
// (#1722 / #1717). One-time-effective per database; subsequent calls
// are no-ops gated on column / index existence.
//
// DDL only — does NOT register the per-write sync. Pair every startup
// call with [ProtoSQLStorage.RegisterCommunityArrayColumns] (which must
// run every process start regardless of schema state).
func (s *ProtoSQLStorage) InitializeCommunityArrayColumns(ctx context.Context) error {
	if !s.isTypeRegistered(&models.Community{}) {
		return nil
	}
	if err := s.InitializeArrayColumn(
		ctx, &models.Community{}, "snapshot_member_user_ids", communitySnapshotMemberIds,
	); err != nil {
		return fmt.Errorf("initialize community.snapshot_member_user_ids: %w", err)
	}
	return nil
}

// RegisterCommunityArrayColumns registers the per-write auto-sync for
// every Community TEXT[] denormalization column. Required on every
// server start: without it, Insert / Update of a Community will not
// populate the indexed columns and the restore-list lookup silently
// breaks.
//
// Safe no-op when Community is not registered for storage.
func (s *ProtoSQLStorage) RegisterCommunityArrayColumns() {
	if !s.isTypeRegistered(&models.Community{}) {
		return
	}
	s.RegisterArrayColumn(&models.Community{}, "snapshot_member_user_ids", communitySnapshotMemberIds)
}

// InitializeCommunityConversationIDMigration backfills conversation_id onto
// the canonical parent entity (Gear, Request, Experience) for any join-table
// row where the deprecated CommunityGear/CommunityRequest/CommunityExperience
// conversation_id is non-empty but the parent's field is still empty.
//
// Idempotent: processes up to 10 000 rows per join table per pass; a parent
// whose field is already non-empty is left alone. Safe to run on every
// startup — once the backfill is complete all passes return zero updated rows
// and cost only a bounded index scan.
//
// See #1332 for the migration plan and InitializeCommunityOwnerUserID in this
// file for the reference shape.
func (s *ProtoSQLStorage) InitializeCommunityConversationIDMigration(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With("operation", "InitializeCommunityConversationIDMigration")
	start := time.Now()

	gearUpdated, err := s.backfillConversationID(ctx, logger, gearConversationIDBackfill)
	if err != nil {
		return fmt.Errorf("backfill gear conversation_id: %w", err)
	}
	requestUpdated, err := s.backfillConversationID(ctx, logger, requestConversationIDBackfill)
	if err != nil {
		return fmt.Errorf("backfill request conversation_id: %w", err)
	}
	expUpdated, err := s.backfillConversationID(ctx, logger, experienceConversationIDBackfill)
	if err != nil {
		return fmt.Errorf("backfill experience conversation_id: %w", err)
	}

	totalUpdated := gearUpdated + requestUpdated + expUpdated
	logger.InfoContext(ctx, "community conversation_id migration complete",
		"gear_updated", gearUpdated,
		"request_updated", requestUpdated,
		"experience_updated", expUpdated,
		"total_updated", totalUpdated,
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}

// conversationIDBackfill describes one parent/pivot pair for
// backfillConversationID. The three concrete backfills differ only in these
// values — they used to be three 71-line copies, the largest in-file
// duplication in the repo (#2816).
type conversationIDBackfill struct {
	// pivotTable is the community_* join table carrying the flat column.
	pivotTable string
	// idColumn is the parent's ID column on the pivot table ("gear_id").
	idColumn string
	// kind names the parent in log lines ("gear", "request", "experience").
	kind string
	// newParent returns an empty parent message to load into.
	newParent func() proto.Message
	// conversationID reads the parent's current conversation_id.
	conversationID func(proto.Message) string
	// setConversationID writes it.
	setConversationID func(proto.Message, string)
}

// backfillConversationID copies the conversation_id flat column from a
// community_* pivot table to the parent's conversation_id, for rows where the
// parent field is empty. Reads the flat SQL column directly (via
// columnExists/raw SQL) rather than through the generated proto struct, since
// the corresponding proto field is reserved and no longer generates a Go
// accessor.
//
// Skips silently when the table or column is absent, so it is safe against a
// database that never carried the flat column.
func (s *ProtoSQLStorage) backfillConversationID(
	ctx context.Context, logger *logging.Logger, spec conversationIDBackfill,
) (int, error) {
	tableExists, err := s.tableExists(ctx, spec.pivotTable)
	if err != nil {
		return 0, fmt.Errorf("check %s table: %w", spec.pivotTable, err)
	}
	if !tableExists {
		logger.DebugContext(ctx, "pivot table not present, skipping", "table", spec.pivotTable)
		return 0, nil
	}
	colExists, err := s.columnExists(ctx, spec.pivotTable, "conversation_id")
	if err != nil {
		return 0, fmt.Errorf("check %s.conversation_id column: %w", spec.pivotTable, err)
	}
	if !colExists {
		logger.DebugContext(ctx, "conversation_id column not present, backfill complete",
			"table", spec.pivotTable)
		return 0, nil
	}

	// Both identifiers come from the closed set of conversationIDBackfill values
	// below, never from a caller — but they still go through quoteIdent, which
	// is the only sanctioned way to put an identifier in a query (#2795). This
	// used to use %q, which applies Go string quoting rather than PostgreSQL's.
	quotedIDColumn, err := quoteIdent(spec.idColumn)
	if err != nil {
		return 0, fmt.Errorf("quote %s.%s: %w", spec.pivotTable, spec.idColumn, err)
	}
	quotedTable, err := quoteIdent(spec.pivotTable)
	if err != nil {
		return 0, fmt.Errorf("quote %s: %w", spec.pivotTable, err)
	}
	query := fmt.Sprintf(`
		SELECT %s, conversation_id
		FROM %s
		WHERE conversation_id IS NOT NULL AND conversation_id != ''
		LIMIT 10000
	`, quotedIDColumn, quotedTable) // sql-fragment-allow: both identifiers quoted via quoteIdent; no values interpolated

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("query %s: %w", spec.pivotTable, err)
	}
	defer rows.Close()

	type pending struct{ parentID, convID string }
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.parentID, &p.convID); err != nil {
			return 0, fmt.Errorf("scan %s row: %w", spec.pivotTable, err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate %s: %w", spec.pivotTable, err)
	}

	updated := 0
	// Deduplicate by parent ID — there may be several pivot rows per parent.
	seen := make(map[string]bool, len(todo))
	for _, p := range todo {
		if seen[p.parentID] {
			continue
		}
		seen[p.parentID] = true

		parent := spec.newParent()
		if err := s.GetByID(ctx, p.parentID, parent); err != nil {
			logger.WarnContext(ctx, "parent not found, skipping",
				"kind", spec.kind, "parent_id", p.parentID, "error", err)
			continue
		}
		if spec.conversationID(parent) != "" {
			continue
		}
		spec.setConversationID(parent, p.convID)
		if err := s.Update(ctx, parent); err != nil {
			logger.WarnContext(ctx, "failed to update conversation_id, skipping",
				"kind", spec.kind, "parent_id", p.parentID, "error", err)
			continue
		}
		updated++
	}
	return updated, nil
}

var (
	gearConversationIDBackfill = conversationIDBackfill{
		pivotTable: "community_gear", idColumn: "gear_id", kind: "gear",
		newParent:         func() proto.Message { return &models.Gear{} },
		conversationID:    func(m proto.Message) string { return m.(*models.Gear).ConversationId },
		setConversationID: func(m proto.Message, id string) { m.(*models.Gear).ConversationId = id },
	}
	requestConversationIDBackfill = conversationIDBackfill{
		pivotTable: "community_request", idColumn: "request_id", kind: "request",
		newParent:         func() proto.Message { return &models.Request{} },
		conversationID:    func(m proto.Message) string { return m.(*models.Request).ConversationId },
		setConversationID: func(m proto.Message, id string) { m.(*models.Request).ConversationId = id },
	}
	experienceConversationIDBackfill = conversationIDBackfill{
		pivotTable: "community_experience", idColumn: "experience_id", kind: "experience",
		newParent:         func() proto.Message { return &models.Experience{} },
		conversationID:    func(m proto.Message) string { return m.(*models.Experience).ConversationId },
		setConversationID: func(m proto.Message, id string) { m.(*models.Experience).ConversationId = id },
	}
)
