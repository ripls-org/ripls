// Schema management: table creation, column evolution, field flattening, and default storage types.

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// maxColumnNameLen is the maximum PostgreSQL identifier length (NAMEDATALEN - 1).
const maxColumnNameLen = 63

// sanitizeColumnName ensures a column name fits within PostgreSQL's 63-byte
// identifier limit. If the name exceeds the limit, it keeps a prefix of the
// original name and appends a short hash of the full name for uniqueness.
func sanitizeColumnName(name string) string {
	if len(name) <= maxColumnNameLen {
		return name
	}
	// Use 8 hex chars from SHA-256 of the full name for uniqueness.
	hash := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(hash[:4]) // 8 hex chars
	// Truncate prefix to leave room for the hash suffix.
	prefixLen := maxColumnNameLen - len(suffix)
	return name[:prefixLen] + suffix
}

// flattenFields recursively flattens nested message fields into a list of SQL-queryable fields.
// prefix is used to build column names like "geolocation_latitude_deg" for nested fields.
func flattenFields(descriptor protoreflect.MessageDescriptor, prefix string) []flattenedField {
	var flattened []flattenedField
	fields := descriptor.Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Skip repeated fields - they can't be mapped to simple SQL columns
		if field.Cardinality() == protoreflect.Repeated {
			continue
		}

		fieldName := string(field.Name())
		columnName := fieldName
		if prefix != "" {
			columnName = prefix + "_" + fieldName
		}

		// If it's a nested message, recurse
		if field.Kind() == protoreflect.MessageKind {
			nestedFields := flattenFields(field.Message(), columnName)
			flattened = append(flattened, nestedFields...)
		} else {
			// It's a scalar field, add it to the flattened list
			flattened = append(flattened, flattenedField{
				columnName: sanitizeColumnName(columnName),
				kind:       field.Kind(),
				fieldPath:  []protoreflect.FieldDescriptor{field},
			})
		}
	}

	return flattened
}

// detectGeospatialFields checks if a message has latitude_deg and longitude_deg fields
// (either directly or in flattened nested messages).
func detectGeospatialFields(descriptor protoreflect.MessageDescriptor) (hasLatLon bool, latField, lonField string) {
	flattened := flattenFields(descriptor, "")
	var foundLat, foundLon bool

	for _, field := range flattened {
		if field.columnName == "latitude_deg" && (field.kind == protoreflect.DoubleKind || field.kind == protoreflect.FloatKind) {
			foundLat = true
			latField = field.columnName
		}
		if field.columnName == "longitude_deg" && (field.kind == protoreflect.DoubleKind || field.kind == protoreflect.FloatKind) {
			foundLon = true
			lonField = field.columnName
		}
		// Also check for geolocation_latitude_deg pattern
		if field.columnName == "geolocation_latitude_deg" && (field.kind == protoreflect.DoubleKind || field.kind == protoreflect.FloatKind) {
			foundLat = true
			latField = field.columnName
		}
		if field.columnName == "geolocation_longitude_deg" && (field.kind == protoreflect.DoubleKind || field.kind == protoreflect.FloatKind) {
			foundLon = true
			lonField = field.columnName
		}
	}

	return foundLat && foundLon, latField, lonField
}

// generateGeospatialIndexSQL creates index statements for latitude and longitude columns.
func generateGeospatialIndexSQL(tableName, latField, lonField string) ([]string, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	var statements []string
	for _, field := range []string{latField, lonField} {
		quotedField, err := quoteIdent(field)
		if err != nil {
			return nil, err
		}
		// The index name is derived from two already-validated identifiers, so
		// it is a safe identifier by construction — but it can exceed
		// PostgreSQL's 63-byte limit, so run it through the same sanitizer the
		// column names use before quoting.
		quotedIndex, err := quoteIdent(sanitizeColumnName("idx_" + tableName + "_" + field))
		if err != nil {
			return nil, err
		}
		statements = append(statements,
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (%s);`, quotedIndex, quotedTable, quotedField))
	}
	return statements, nil
}

// generateCreateTableSQL generates a CREATE TABLE statement for a proto message with flattened nested fields.
func generateCreateTableSQL(dbSpec DatabaseSpecifics, descriptor protoreflect.MessageDescriptor, tableName string) (string, error) {
	var columns []string

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return "", err
	}

	// Flatten all fields (including nested messages)
	flattened := flattenFields(descriptor, "")

	// Add columns for each flattened field
	for i, field := range flattened {
		quotedColumn, err := quoteIdent(field.columnName)
		if err != nil {
			return "", err
		}
		// SQLType returns one of a fixed set of dialect keywords, never
		// caller-controlled text.
		sqlType := dbSpec.SQLType(field.kind) // sql-fragment-allow: dialect type keyword from DatabaseSpecifics, closed set

		// Make the first field (usually 'id') the primary key
		if i == 0 && field.columnName == "id" {
			columns = append(columns, fmt.Sprintf("    %s %s PRIMARY KEY", quotedColumn, sqlType))
		} else {
			columns = append(columns, fmt.Sprintf("    %s %s", quotedColumn, sqlType))
		}
	}

	// Add the serialized proto column
	columns = append(columns, fmt.Sprintf("    binary_proto %s NOT NULL", dbSpec.BinaryColumnType())) // sql-fragment-allow: dialect type keyword from DatabaseSpecifics, closed set

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
%s
);`, quotedTable, strings.Join(columns, ",\n")), nil
}

// getExpectedColumns returns the list of column definitions expected for a proto message.
// Uses field flattening to handle nested messages.
func getExpectedColumns(dbSpec DatabaseSpecifics, descriptor protoreflect.MessageDescriptor) []columnDefinition {
	var columns []columnDefinition

	// Flatten all fields (including nested messages)
	flattened := flattenFields(descriptor, "")

	// Convert flattened fields to column definitions
	for _, field := range flattened {
		columns = append(columns, columnDefinition{
			name:    field.columnName,
			sqlType: dbSpec.SQLType(field.kind),
		})
	}

	// Add the binary_proto column
	columns = append(columns, columnDefinition{name: "binary_proto", sqlType: dbSpec.BinaryColumnType() + " NOT NULL"})

	return columns
}

// getMissingColumns compares expected columns with existing columns and returns the missing ones.
func getMissingColumns(expectedColumns []columnDefinition, existingColumns map[string]bool) []columnDefinition {
	var missing []columnDefinition
	for _, col := range expectedColumns {
		if !existingColumns[col.name] {
			missing = append(missing, col)
		}
	}
	return missing
}

// generateAlterTableAddColumnSQL generates an ALTER TABLE statement to add a column.
func generateAlterTableAddColumnSQL(tableName string, column columnDefinition) (string, error) {
	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return "", err
	}
	quotedColumn, err := quoteIdent(column.name)
	if err != nil {
		return "", err
	}
	// column.sqlType comes from DatabaseSpecifics.SQLType / BinaryColumnType —
	// a closed set of dialect keywords, never caller-controlled text.
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, quotedTable, quotedColumn, column.sqlType), nil // sql-fragment-allow: dialect type keyword from DatabaseSpecifics, closed set
}

// extractFieldValuesRecursive recursively extracts flattened field names and values from a proto message.
func extractFieldValuesRecursive(msg protoreflect.Message, prefix string) ([]string, []any) {
	descriptor := msg.Descriptor()
	fields := descriptor.Fields()

	var fieldNames []string
	var values []any

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		// Skip repeated fields - they can't be mapped to simple SQL columns
		if field.Cardinality() == protoreflect.Repeated {
			continue
		}

		fieldName := string(field.Name())
		columnName := fieldName
		if prefix != "" {
			columnName = prefix + "_" + fieldName
		}

		fieldValue := msg.Get(field)

		// If it's a nested message, recurse
		if field.Kind() == protoreflect.MessageKind {
			// Only process valid (set) messages
			if fieldValue.Message().IsValid() {
				nestedNames, nestedValues := extractFieldValuesRecursive(fieldValue.Message(), columnName)
				fieldNames = append(fieldNames, nestedNames...)
				values = append(values, nestedValues...)
			}
			// Skip invalid/unset message fields (e.g., unset oneof fields)
		} else {
			// It's a scalar field
			fieldNames = append(fieldNames, sanitizeColumnName(columnName))
			values = append(values, protoValueToInterface(field, fieldValue))
		}
	}

	return fieldNames, values
}

// extractFieldValues extracts field names and values from a proto message (wrapper for recursive version).
func extractFieldValues(msg protoreflect.Message) ([]string, []any) {
	return extractFieldValuesRecursive(msg, "")
}

// hasDeletedField checks if a message descriptor has the DeletedMetadata field.
func hasDeletedField(descriptor protoreflect.MessageDescriptor) bool {
	fields := descriptor.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if string(field.Name()) == "deleted" && field.Kind() == protoreflect.MessageKind {
			return true
		}
	}
	return false
}

// buildDeletedFilter returns a SQL WHERE clause fragment to filter out deleted items.
// Returns empty string if:
// - opts.IncludeDeleted is true
// - the message type doesn't have a 'deleted' field.
func buildDeletedFilter(descriptor protoreflect.MessageDescriptor, opts *QueryOptions) string {
	if opts != nil && opts.IncludeDeleted {
		return ""
	}
	if !hasDeletedField(descriptor) {
		return ""
	}
	return fmt.Sprintf(" AND (%s = 0 OR %s IS NULL)", deletedFilterColumn, deletedFilterColumn)
}

// DefaultStorageTypes returns the default set of storage type configurations.
// ExperienceNameEmbeddingVariant is the embedding variant that embeds an
// experience's name only (no description). It powers open-day activity
// clustering (#2674/#2694); semantic search uses the default name+description
// embedding instead.
const ExperienceNameEmbeddingVariant = "name"

func DefaultStorageTypes() []TypeConfig {
	return []TypeConfig{
		{MessageType: &models.ChatConversation{}, TableName: "chat_conversation"},
		{MessageType: &models.ChatMessage{}, TableName: "chat_message"},
		{MessageType: &models.Community{}, TableName: "community"},
		{MessageType: &models.CommunityEvent{}, TableName: "community_event"},
		{MessageType: &models.CommunityExperience{}, TableName: "community_experience"},
		{MessageType: &models.CommunityGear{}, TableName: "community_gear"},
		{MessageType: &models.CommunityInvitationLink{}, TableName: "community_invitation_link"},
		{MessageType: &models.CommunityNotificationPreferences{}, TableName: "community_notification_preferences"},
		{MessageType: &models.CommunityRegion{}, TableName: "community_region"},
		{MessageType: &models.CommunityPurgeAudit{}, TableName: "community_purge_audit"},
		{MessageType: &models.CommunityRequest{}, TableName: "community_request"},
		{MessageType: &models.CommunityUser{}, TableName: "community_user"},
		{
			MessageType: &models.Experience{},
			TableName:   "experience",
			Embeddings: []*EmbeddingFieldConfig{
				// Default: name+description, powers semantic search.
				{Fields: []string{"name", "description"}},
				// Name-only, powers open-day activity clustering (#2674/#2694).
				// Read by id only (no ANN search), so skip the HNSW index.
				{Fields: []string{"name"}, Variant: ExperienceNameEmbeddingVariant, SkipIndex: true},
			},
		},
		{MessageType: &models.PlanningNeed{}, TableName: "planning_need"},
		{MessageType: &models.PlanningContribution{}, TableName: "planning_contribution"},
		{MessageType: &models.ExperienceRSVP{}, TableName: "experience_rsvp"},
		{MessageType: &models.ExperienceTimeProposal{}, TableName: "experience_time_proposal"},
		{MessageType: &models.TimeVote{}, TableName: "time_vote"},
		{MessageType: &models.ExperienceLocationProposal{}, TableName: "experience_location_proposal"},
		{MessageType: &models.LocationVote{}, TableName: "location_vote"},
		{MessageType: &models.FeedItemView{}, TableName: "FeedItemView"},
		{
			MessageType: &models.Gear{},
			TableName:   "gear",
			Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"name", "description"}}},
		},
		{MessageType: &models.Transfer{}, TableName: "transfer"},
		{
			MessageType: &models.Request{},
			TableName:   "request",
			Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"title", "description"}}},
		},
		{MessageType: &models.RequestOffer{}, TableName: "request_offer"},
		{MessageType: &models.ScheduledNotification{}, TableName: "scheduled_notification"},
		{MessageType: &models.ShareLink{}, TableName: "share_link"},
		{MessageType: &models.Location{}, TableName: "location"},
		{MessageType: &models.UserLocation{}, TableName: "user_location"},
		{MessageType: &models.Region{}, TableName: "region"},
		{MessageType: &models.Media{}, TableName: "media"},
		{
			MessageType: &models.StockImage{},
			TableName:   "stock_image",
			Embeddings:  []*EmbeddingFieldConfig{{Fields: []string{"provider_image.description", "provider_image.alt_description"}}},
		},
		{MessageType: &models.ProvisionalUser{}, TableName: "provisional_user"},
		{MessageType: &models.InviteConsent{}, TableName: "invite_consent"},
		{MessageType: &models.EmailSuppression{}, TableName: "email_suppression"},
		{MessageType: &models.SmsOptOut{}, TableName: "sms_opt_out"},
		{MessageType: &models.SuppressedKnownFor{}, TableName: "suppressed_known_for"},
		{MessageType: &models.StoredNudge{}, TableName: "stored_nudge"},
		{MessageType: &models.StoredESMPrompt{}, TableName: "stored_esm_prompt"},
		{MessageType: &models.StoredESMResponse{}, TableName: "stored_esm_response"},
		{MessageType: &models.Story{}, TableName: "Story"},
		{MessageType: &models.PendingEmailCode{}, TableName: "pending_email_code"},
		{MessageType: &models.PendingPasswordReset{}, TableName: "pending_password_reset"},
		{MessageType: &models.RefreshToken{}, TableName: "refresh_token"},
		{MessageType: &models.User{}, TableName: "user"},
		{MessageType: &models.UserDevice{}, TableName: "user_device"},
		{MessageType: &models.UserNotificationPreferences{}, TableName: "user_notification_preferences"},
		{MessageType: &models.UserWeeklyGoals{}, TableName: "user_weekly_goals"},
		{MessageType: &models.WaitlistEntry{}, TableName: "waitlist_entry"},
		{MessageType: &models.WatchedItem{}, TableName: "watched_item"},
	}
}

// initializeDatabase returns a general SQL storage object wrapping the specific database implementation.
func initializeDatabase(ctx context.Context, dbSpec DatabaseSpecifics, connectionString string, typeConfigs []TypeConfig) (*ProtoSQLStorage, error) {
	// Validate that type configs are provided
	if typeConfigs == nil {
		return nil, fmt.Errorf("typeConfigs cannot be nil; use DefaultStorageTypes() or provide custom configuration")
	}

	// Validate every descriptor-derived identifier once, before any of them can
	// reach a query. This is what lets the per-query paths handle a quoteIdent
	// error as impossible-but-handled rather than panicking mid-request.
	if err := validateSchemaIdentifiers(typeConfigs); err != nil {
		return nil, fmt.Errorf("storage schema has an unusable SQL identifier: %w", err)
	}

	// Open database using the specific driver
	db, err := dbSpec.OpenDatabase(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool to prevent exhausting database connections.
	// Cloud SQL instances have limited connections (e.g., 25-100 depending on instance size).
	// These settings ensure we stay well under those limits while maintaining good performance.
	db.SetMaxOpenConns(10)                 // Maximum connections to avoid exhausting Cloud SQL limits
	db.SetMaxIdleConns(5)                  // Keep some connections ready for reuse
	db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections before Cloud SQL idle timeout (10 min)
	db.SetConnMaxIdleTime(2 * time.Minute) // Close idle connections to free up resources

	// Finish the #2832 RSVP flat-column rename before the generic schema
	// pass — see migrateRSVPEnumColumnRename for why the ordering matters.
	if err := migrateRSVPEnumColumnRename(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to rename RSVP enum columns: %w", err)
	}

	// Build allowed types map and embedding configs, initialize schema for each message type
	allowedTypes := make(map[string]string)
	embeddingConfigs := make(map[string][]*EmbeddingFieldConfig)
	for _, config := range typeConfigs {
		descriptor := config.MessageType.ProtoReflect().Descriptor()
		typeName := string(descriptor.FullName())
		allowedTypes[typeName] = config.TableName
		if len(config.Embeddings) > 0 {
			embeddingConfigs[config.TableName] = config.Embeddings
		}

		// First, try to create the table
		createTableSQL, err := generateCreateTableSQL(dbSpec, descriptor, config.TableName)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to build CREATE TABLE for %s: %w", config.TableName, err)
		}
		_, err = db.ExecContext(ctx, createTableSQL)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create table %s: %w", config.TableName, err)
		}

		// Check if the table has all required columns and add any that are missing (schema evolution)
		existingColumns, err := dbSpec.GetTableColumns(ctx, db, config.TableName)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to get columns for table %s: %w", config.TableName, err)
		}

		expectedColumns := getExpectedColumns(dbSpec, descriptor)
		missingColumns := getMissingColumns(expectedColumns, existingColumns)

		// Add any missing columns
		for _, col := range missingColumns {
			alterSQL, err := generateAlterTableAddColumnSQL(config.TableName, col)
			if err != nil {
				db.Close()
				return nil, fmt.Errorf("failed to build ALTER TABLE for %s.%s: %w", config.TableName, col.name, err)
			}
			_, err = db.ExecContext(ctx, alterSQL)
			if err != nil {
				db.Close()
				return nil, fmt.Errorf("failed to add column %s to table %s: %w", col.name, config.TableName, err)
			}
		}

		// Convert any legacy TEXT enum flat column to INTEGER (#2840). Runs
		// before queries can touch the table; a no-op once converted.
		if err := migrateEnumColumnsToInteger(ctx, db, descriptor, config.TableName); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to migrate enum columns for table %s: %w", config.TableName, err)
		}

		// Create geospatial indexes if the message has latitude_deg and longitude_deg fields
		if hasLatLon, latField, lonField := detectGeospatialFields(descriptor); hasLatLon {
			indexStatements, err := generateGeospatialIndexSQL(config.TableName, latField, lonField)
			if err != nil {
				db.Close()
				return nil, fmt.Errorf("failed to build geospatial index SQL for %s: %w", config.TableName, err)
			}
			for _, indexSQL := range indexStatements {
				_, err = db.ExecContext(ctx, indexSQL)
				if err != nil {
					db.Close()
					return nil, fmt.Errorf("failed to create geospatial index on %s: %w", config.TableName, err)
				}
			}
		}
	}

	// Pre-index cleanup: reap duplicate scheduled_notification rows with a
	// future fire_at before the unique indexes below are created. Without
	// this, CREATE UNIQUE INDEX IF NOT EXISTS fails on first deploy if prod
	// already has duplicates from the multi-replica race (#2458). Scoped to
	// fire_at > now so the dispatcher's ownership of past-due rows is
	// respected. Each statement is idempotent — a no-op once there are no
	// more duplicates.
	preIndexCleanupStatements := []string{
		`DELETE FROM "scheduled_notification" a USING "scheduled_notification" b
		 WHERE a.id > b.id
		   AND a.recipient_user_id = b.recipient_user_id
		   AND a.experience_experience_id = b.experience_experience_id
		   AND a.experience_purpose = b.experience_purpose
		   AND a.experience_offset_seconds_from_anchor = b.experience_offset_seconds_from_anchor
		   AND a.experience_experience_id IS NOT NULL
		   AND a.experience_experience_id <> ''
		   AND a.fire_at_unix_sec > EXTRACT(EPOCH FROM NOW())::bigint`,
		`DELETE FROM "scheduled_notification" a USING "scheduled_notification" b
		 WHERE a.id > b.id
		   AND a.recipient_user_id = b.recipient_user_id
		   AND a.loan_transfer_id = b.loan_transfer_id
		   AND a.loan_purpose = b.loan_purpose
		   AND a.loan_offset_seconds_from_anchor = b.loan_offset_seconds_from_anchor
		   AND a.loan_transfer_id IS NOT NULL
		   AND a.loan_transfer_id <> ''
		   AND a.fire_at_unix_sec > EXTRACT(EPOCH FROM NOW())::bigint`,
		`DELETE FROM "scheduled_notification" a USING "scheduled_notification" b
		 WHERE a.id > b.id
		   AND a.recipient_user_id = b.recipient_user_id
		   AND a.request_request_id = b.request_request_id
		   AND a.request_purpose = b.request_purpose
		   AND a.request_offset_seconds_from_anchor = b.request_offset_seconds_from_anchor
		   AND a.request_request_id IS NOT NULL
		   AND a.request_request_id <> ''
		   AND a.fire_at_unix_sec > EXTRACT(EPOCH FROM NOW())::bigint`,
	}
	for _, cleanupSQL := range preIndexCleanupStatements {
		if _, err := db.ExecContext(ctx, cleanupSQL); err != nil {
			logging.Default().Warn("pre-index dedup failed (non-fatal)", "error", err)
		}
	}

	// Create indexes for frequently filtered columns.
	indexStatements := []string{
		// Story table: community_id is the primary list filter.
		`CREATE INDEX IF NOT EXISTS idx_story_community ON "story" (community_id)`,
		// Daily purge job (#1620) WHERE clauses. Partial indexes
		// keyed on the flattened deleted-metadata column so the
		// daily candidate scan reads only soft-deleted rows.
		`CREATE INDEX IF NOT EXISTS idx_community_deleted_at ON "community" (deleted_deleted_at_unix_sec) WHERE deleted_deleted_at_unix_sec > 0`,
		`CREATE INDEX IF NOT EXISTS idx_community_user_deleted_at ON "community_user" (deleted_deleted_at_unix_sec) WHERE deleted_deleted_at_unix_sec > 0`,
		// FeedItemView is community-scoped per #1620 cascade.
		`CREATE INDEX IF NOT EXISTS idx_feeditemview_community ON "FeedItemView" (community_id)`,
		// Powers the "latest activity per community" GROUP BY in
		// ListCommunities, which sorts the Workshop carousel by most
		// recent CommunityEvent (#1898).
		`CREATE INDEX IF NOT EXISTS idx_community_event_community_occurred ON "community_event" (community_id, occurred_at_unix_sec DESC)`,
		// Serves the batched request_id IN (…) lookup in buildAPIRequests (#1806).
		// Partial index because request_id is empty on every non-request event.
		`CREATE INDEX IF NOT EXISTS idx_community_event_request_id ON "community_event" (request_id) WHERE request_id <> ''`,
		// Scheduled-notification reconciler & dispatcher (#625). Rows
		// are hard-deleted on dispatch (record-of-send lives in
		// structured logs) so every row is "due to fire" until it
		// isn't. Two access patterns: the dispatcher pulls due rows
		// by fire_at, and the per-user inspection query lists what a
		// given recipient will receive. Per-item-type reconciler
		// queries are predicated on the oneof discriminator columns
		// (e.g. item_experience_experience_id IS NOT NULL) and are
		// fast enough without a dedicated index at current scale.
		`CREATE INDEX IF NOT EXISTS idx_scheduled_notification_due ON "scheduled_notification" (fire_at_unix_sec)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_notification_user ON "scheduled_notification" (recipient_user_id)`,
		// Enforce the reconciler's uniqueness key (framework.go:UniquenessKey)
		// at the DB level so concurrent reconciler replicas cannot insert
		// duplicate rows for the same (recipient, anchor, purpose, offset)
		// tuple regardless of timing or replica count (#2458). One partial
		// unique index per item oneof variant, matching the partial-index
		// pattern used for share_link.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_notification_unique_experience
		   ON "scheduled_notification" (recipient_user_id, experience_experience_id, experience_purpose, experience_offset_seconds_from_anchor)
		   WHERE experience_experience_id IS NOT NULL AND experience_experience_id <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_notification_unique_loan
		   ON "scheduled_notification" (recipient_user_id, loan_transfer_id, loan_purpose, loan_offset_seconds_from_anchor)
		   WHERE loan_transfer_id IS NOT NULL AND loan_transfer_id <> ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_notification_unique_request
		   ON "scheduled_notification" (recipient_user_id, request_request_id, request_purpose, request_offset_seconds_from_anchor)
		   WHERE request_request_id IS NOT NULL AND request_request_id <> ''`,
		// Loan-return reconciler (#625) scans active loans by their
		// derived return date. Partial because only loans set this
		// column; giveaways and pre-pickup transfers leave it NULL.
		`CREATE INDEX IF NOT EXISTS idx_transfer_expected_return ON "transfer" (expected_return_unix_sec) WHERE expected_return_unix_sec IS NOT NULL`,
		// Daily activity digest (#1924) scans these tables in a
		// fixed 24h window. Each index keeps the cross-table scan
		// cheap as the underlying tables grow.
		`CREATE INDEX IF NOT EXISTS idx_community_event_occurred ON "community_event" (occurred_at_unix_sec DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_message_sent ON "chat_message" (sent_at_unix_sec DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_user_created_at ON "user" (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_token_created ON "refresh_token" (created_at_unix_sec DESC, user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_waitlist_entry_created ON "waitlist_entry" (created_at_unix_sec DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pending_password_reset_created ON "pending_password_reset" (created_at_unix_sec DESC)`,
		// ShareLink (#2048): short_code is the hot-path resolver for
		// /go/{code}; community_id powers per-community listing.
		// Each oneof target variant gets a partial index for "find the
		// canonical share link for entity X" lookups, plus a partial
		// unique constraint enforcing one link per (inviter, target).
		// Partial-index pattern follows community_event.request_id.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_short_code ON "share_link" (short_code)`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_community ON "share_link" (community_id)`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_community_invite ON "share_link" (community_invite_id) WHERE community_invite_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_gear ON "share_link" (gear_id) WHERE gear_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_transfer ON "share_link" (transfer_id) WHERE transfer_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_request ON "share_link" (request_id) WHERE request_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_share_link_experience ON "share_link" (experience_id) WHERE experience_id <> ''`,
		// The unique-per-(inviter, target) constraints apply only to
		// active rows — revoked or soft-deleted rows must be allowed
		// to coexist with a fresh active link, otherwise revoke +
		// re-create cycles would fail.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_community_invite ON "share_link" (inviter_id, community_invite_id) WHERE community_invite_id <> '' AND is_revoked = false AND (deleted_deleted_at_unix_sec IS NULL OR deleted_deleted_at_unix_sec = 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_gear ON "share_link" (inviter_id, gear_id) WHERE gear_id <> '' AND is_revoked = false AND (deleted_deleted_at_unix_sec IS NULL OR deleted_deleted_at_unix_sec = 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_transfer ON "share_link" (inviter_id, transfer_id) WHERE transfer_id <> '' AND is_revoked = false AND (deleted_deleted_at_unix_sec IS NULL OR deleted_deleted_at_unix_sec = 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_request ON "share_link" (inviter_id, request_id) WHERE request_id <> '' AND is_revoked = false AND (deleted_deleted_at_unix_sec IS NULL OR deleted_deleted_at_unix_sec = 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_experience ON "share_link" (inviter_id, experience_id) WHERE experience_id <> '' AND is_revoked = false AND (deleted_deleted_at_unix_sec IS NULL OR deleted_deleted_at_unix_sec = 0)`,
		// One text-message preference row per number (#2492), looked up
		// before every platform text send and on each STOP/START webhook.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sms_opt_out_phone ON "sms_opt_out" (phone_number)`,
	}

	// Singleton state table for the daily activity digest claim
	// primitive (#1924). One row keyed by a fixed sentinel; the
	// ClaimActivityDigestSend conditional UPDATE moves last_sent_date
	// forward so only one Cloud Run instance sends per local day.
	activityDigestStateTable := `CREATE TABLE IF NOT EXISTS activity_digest_state (
		id TEXT PRIMARY KEY,
		last_sent_date TEXT NOT NULL DEFAULT ''
	)`
	if _, err := db.ExecContext(ctx, activityDigestStateTable); err != nil {
		return nil, fmt.Errorf("failed to create activity_digest_state table: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO activity_digest_state (id, last_sent_date) VALUES ('daily', '') ON CONFLICT (id) DO NOTHING`,
	); err != nil {
		return nil, fmt.Errorf("failed to seed activity_digest_state daily row: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO activity_digest_state (id, last_sent_date) VALUES ('weekly', '') ON CONFLICT (id) DO NOTHING`,
	); err != nil {
		return nil, fmt.Errorf("failed to seed activity_digest_state weekly row: %w", err)
	}
	// Migration: an earlier revision wrote id='singleton' for the daily
	// claim. If that row exists and 'daily' is still empty, copy its
	// last_sent_date forward so a deploy doesn't double-send today.
	if _, err := db.ExecContext(ctx,
		`UPDATE activity_digest_state SET last_sent_date = (SELECT last_sent_date FROM activity_digest_state WHERE id = 'singleton')
		 WHERE id = 'daily' AND last_sent_date = ''
		   AND EXISTS (SELECT 1 FROM activity_digest_state WHERE id = 'singleton' AND last_sent_date <> '')`,
	); err != nil {
		return nil, fmt.Errorf("failed to migrate activity_digest_state singleton -> daily: %w", err)
	}

	// Per-user daily activity stamps (#2665): one row per (user, UTC day)
	// upserted by the activitystamp middleware, giving the ops digest a
	// true active-users count. Plain telemetry table, not a domain model
	// — day_utc is only the upsert dedup key; window queries range over
	// the first/last-seen timestamps.
	userActiveDayTable := `CREATE TABLE IF NOT EXISTS user_active_day (
		user_id TEXT NOT NULL,
		day_utc TEXT NOT NULL,
		first_seen_unix_sec BIGINT NOT NULL,
		last_seen_unix_sec BIGINT NOT NULL,
		PRIMARY KEY (user_id, day_utc)
	)`
	if _, err := db.ExecContext(ctx, userActiveDayTable); err != nil {
		return nil, fmt.Errorf("failed to create user_active_day table: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_user_active_day_last_seen ON user_active_day (last_seen_unix_sec)`,
	); err != nil {
		return nil, fmt.Errorf("failed to create user_active_day index: %w", err)
	}
	for _, indexSQL := range indexStatements {
		if _, err := db.ExecContext(ctx, indexSQL); err != nil {
			// Non-fatal: table may not exist yet or column may be named differently.
			// Log and continue rather than failing startup.
			logging.Default().Warn("failed to create index (non-fatal)", "sql", indexSQL, "error", err)
		}
	}

	instrumented := NewInstrumentedDB(db)
	return &ProtoSQLStorage{
		db:               instrumented,
		exec:             instrumented,
		dbSpec:           dbSpec,
		allowedTypes:     allowedTypes,
		embeddingConfigs: embeddingConfigs,
		arrayColumns:     make(map[string][]arrayColumnReg),
	}, nil
}

// openDatabaseNoDDL constructs a ProtoSQLStorage over an EXISTING schema
// without executing a single DDL or migration statement. For least-privilege
// (e.g. SELECT-only) connections: the initializeDatabase path runs
// CREATE/ALTER/backfill statements that such a role cannot execute —
// PostgreSQL checks table ownership on ALTER TABLE before deciding the
// sub-command is a no-op. Reads (GetByID / Query*) decode the binary_proto
// column, so skipping schema evolution and array-column registration loses
// nothing for read-only consumers; writes through this handle fail at the
// database when the role lacks privileges.
func openDatabaseNoDDL(ctx context.Context, dbSpec DatabaseSpecifics, connectionString string, typeConfigs []TypeConfig) (*ProtoSQLStorage, error) {
	if typeConfigs == nil {
		return nil, fmt.Errorf("typeConfigs cannot be nil; use DefaultStorageTypes() or provide custom configuration")
	}

	db, err := dbSpec.OpenDatabase(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Same pool discipline as initializeDatabase (Cloud SQL connection limits).
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	allowedTypes := make(map[string]string)
	embeddingConfigs := make(map[string][]*EmbeddingFieldConfig)
	for _, config := range typeConfigs {
		descriptor := config.MessageType.ProtoReflect().Descriptor()
		allowedTypes[string(descriptor.FullName())] = config.TableName
		if len(config.Embeddings) > 0 {
			embeddingConfigs[config.TableName] = config.Embeddings
		}
	}

	instrumented := NewInstrumentedDB(db)
	return &ProtoSQLStorage{
		db:               instrumented,
		exec:             instrumented,
		dbSpec:           dbSpec,
		allowedTypes:     allowedTypes,
		embeddingConfigs: embeddingConfigs,
		arrayColumns:     make(map[string][]arrayColumnReg),
	}, nil
}
