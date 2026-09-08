// Storage helpers for the scheduled_notification table that the
// reconciler/dispatcher pair (#625) needs above the generic CRUD
// surface.
//
// The lifecycle is delete-on-fire — every row that exists is one the
// dispatcher should still fire. The dispatcher claims via atomic
// DELETE: exactly one runner's DELETE affects the row. Record-of-send
// lives in structured logs, not in the table.
//
// One subtle invariant: UpdateScheduledNotificationFireAt re-encodes
// binary_proto in lockstep with the flat fire_at_unix_sec column so
// the reconciler (which reads via binary_proto) doesn't see a stale
// fire_at after a policy-driven update.

package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// scheduledNotificationTableName resolves the registered, quoted table name
// or returns an error. A missing registration is a programmer error
// caught at startup.
func (s *ProtoSQLStorage) scheduledNotificationTableName() (string, error) {
	return s.quotedTableFor("ripls.models.ScheduledNotification")
}

// InsertScheduledNotificationIfAbsent inserts row using INSERT … ON CONFLICT
// DO NOTHING and reports whether the row was actually written. A conflict
// (inserted=false, nil error) means another reconciler replica already holds
// a row for this uniqueness key (#2458) — the caller treats this as Unchanged,
// not as an error, so alert noise stays clean under multi-replica deploys.
//
// The id field on row is set to the minted UUID even on inserted=false (the
// attempted id is useful for forensic log correlation).
func (s *ProtoSQLStorage) InsertScheduledNotificationIfAbsent(
	ctx context.Context, row *models.ScheduledNotification,
) (inserted bool, id string, err error) {
	table, err := s.scheduledNotificationTableName()
	if err != nil {
		return false, "", err
	}

	// Mint id before extracting field values so binary_proto includes it.
	if row.Id == "" {
		row.Id = uuid.New().String()
	}
	id = row.Id

	msgReflect := row.ProtoReflect()
	columns, values := extractFieldValues(msgReflect)

	blob, err := proto.Marshal(row)
	if err != nil {
		return false, id, fmt.Errorf("InsertScheduledNotificationIfAbsent marshal: %w", err)
	}
	columns = append(columns, "binary_proto")
	values = append(values, blob)

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = s.dbSpec.Placeholder(i + 1)
	}

	quotedColumns, err := quoteIdents(columns)
	if err != nil {
		return false, id, err
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING`,
		table,
		strings.Join(quotedColumns, ", "),
		strings.Join(placeholders, ", "),
	)
	result, err := s.db.ExecContext(ctx, query, values...)
	if err != nil {
		return false, id, fmt.Errorf("InsertScheduledNotificationIfAbsent: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, id, fmt.Errorf("InsertScheduledNotificationIfAbsent rows affected: %w", err)
	}
	return rowsAffected > 0, id, nil
}

// ClaimAndDeleteScheduledNotification atomically removes a row by id
// and reports whether this caller is the one that consumed it.
// Returns true exactly once across concurrent dispatcher runners —
// losing runners get false and skip the row.
func (s *ProtoSQLStorage) ClaimAndDeleteScheduledNotification(
	ctx context.Context, id string,
) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("ClaimAndDeleteScheduledNotification: id is required")
	}
	table, err := s.scheduledNotificationTableName()
	if err != nil {
		return false, err
	}

	query := fmt.Sprintf(
		`DELETE FROM %s WHERE id = %s`,
		table,
		s.dbSpec.Placeholder(1),
	)
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("ClaimAndDeleteScheduledNotification: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimAndDeleteScheduledNotification rows affected: %w", err)
	}
	return rows > 0, nil
}

// UpdateScheduledNotificationFireAt rewrites fire_at_unix_sec on a
// row in place, keeping binary_proto in sync. Used by the reconciler
// when a policy change (different offset) or an entity edit (anchor
// time shift) produces a new desired fire_at for an existing row.
// Returns true if a row matched; returns false if the row no longer
// exists (e.g., a concurrent dispatcher claimed and deleted it).
//
// The caller passes the previously-loaded row; this function mutates
// row.FireAtUnixSec and row.UpdatedAtUnixSec before persisting so the
// in-memory object stays consistent with what was written.
func (s *ProtoSQLStorage) UpdateScheduledNotificationFireAt(
	ctx context.Context, row *models.ScheduledNotification, fireAtUnixSec, updatedAtUnixSec int64,
) (bool, error) {
	if row == nil || row.Id == "" {
		return false, fmt.Errorf("UpdateScheduledNotificationFireAt: row.Id is required")
	}
	table, err := s.scheduledNotificationTableName()
	if err != nil {
		return false, err
	}

	row.FireAtUnixSec = fireAtUnixSec
	row.UpdatedAtUnixSec = updatedAtUnixSec
	blob, err := proto.Marshal(row)
	if err != nil {
		return false, fmt.Errorf("UpdateScheduledNotificationFireAt marshal: %w", err)
	}

	query := fmt.Sprintf(
		`UPDATE %s
		 SET fire_at_unix_sec = %s,
		     updated_at_unix_sec = %s,
		     binary_proto = %s
		 WHERE id = %s`,
		table,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(3),
		s.dbSpec.Placeholder(4),
	)
	result, err := s.db.ExecContext(ctx, query, fireAtUnixSec, updatedAtUnixSec, blob, row.Id)
	if err != nil {
		return false, fmt.Errorf("UpdateScheduledNotificationFireAt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdateScheduledNotificationFireAt rows affected: %w", err)
	}
	return rows > 0, nil
}

// FindExperienceScheduledNotifications returns every row whose
// `item` oneof variant is experience. Used by the experience-reminder
// reconciler. The discriminator column is `experience_experience_id`:
// protosql flattens oneof message variants by their variant name
// (no `item_` prefix because the oneof group itself isn't a field),
// so the inner `experience_id` becomes `experience_experience_id`.
// protosql writes that column only when the experience variant is
// set; rows with another variant leave it NULL.
//
// No LIMIT: the reconciler needs the full set to make correct
// decisions. The total is bounded by in-flight experiences × slots ×
// attendees, which stays small at Ripls scale.
func (s *ProtoSQLStorage) FindExperienceScheduledNotifications(
	ctx context.Context,
) ([]*models.ScheduledNotification, error) {
	return s.findScheduledNotificationsByItemColumn(ctx, "experience_experience_id")
}

// FindLoanScheduledNotifications returns every row whose `item`
// oneof variant is loan. Used by the loan-return-reminder reconciler.
func (s *ProtoSQLStorage) FindLoanScheduledNotifications(
	ctx context.Context,
) ([]*models.ScheduledNotification, error) {
	return s.findScheduledNotificationsByItemColumn(ctx, "loan_transfer_id")
}

// FindRequestScheduledNotifications returns every row whose `item`
// oneof variant is request. Used by the request-followup-prompt
// reconciler. The discriminator column is `request_request_id`:
// protosql flattens oneof message variants by their variant name
// (no `item_` prefix), so the inner `request_id` becomes
// `request_request_id`.
func (s *ProtoSQLStorage) FindRequestScheduledNotifications(
	ctx context.Context,
) ([]*models.ScheduledNotification, error) {
	return s.findScheduledNotificationsByItemColumn(ctx, "request_request_id")
}

// findScheduledNotificationsByItemColumn returns every row that has
// the given discriminator column set. protosql writes oneof-variant
// columns only when the variant is set, so checking IS NOT NULL on a
// required field inside the variant identifies rows of that type.
func (s *ProtoSQLStorage) findScheduledNotificationsByItemColumn(
	ctx context.Context, discriminatorColumn string,
) ([]*models.ScheduledNotification, error) {
	table, err := s.scheduledNotificationTableName()
	if err != nil {
		return nil, err
	}

	// discriminatorColumn names the oneof-variant column to probe. Every caller
	// passes a literal; validate anyway so the invariant does not depend on that
	// staying true.
	quotedDiscriminator, err := quoteIdent(discriminatorColumn)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE %s IS NOT NULL AND %s != ''`,
		table, quotedDiscriminator, quotedDiscriminator,
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("findScheduledNotificationsByItemColumn: %w", err)
	}
	defer rows.Close()

	var out []*models.ScheduledNotification
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("findScheduledNotificationsByItemColumn scan: %w", err)
		}
		row := &models.ScheduledNotification{}
		if err := proto.Unmarshal(blob, row); err != nil {
			return nil, fmt.Errorf("findScheduledNotificationsByItemColumn unmarshal: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("findScheduledNotificationsByItemColumn iterate: %w", err)
	}
	return out, nil
}

// FindDueScheduledNotifications returns rows whose fire_at_unix_sec
// has elapsed, bounded by limit so a long backlog doesn't produce a
// runaway dispatcher batch. Ordered oldest-first so the queue drains
// FIFO. Powers the dispatcher's "what's due" pull.
func (s *ProtoSQLStorage) FindDueScheduledNotifications(
	ctx context.Context, nowUnixSec int64, limit int,
) ([]*models.ScheduledNotification, error) {
	table, err := s.scheduledNotificationTableName()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	query := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE fire_at_unix_sec <= %s
		 ORDER BY fire_at_unix_sec ASC
		 LIMIT %s`,
		table,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	rows, err := s.db.QueryContext(ctx, query, nowUnixSec, limit)
	if err != nil {
		return nil, fmt.Errorf("FindDueScheduledNotifications: %w", err)
	}
	defer rows.Close()

	var out []*models.ScheduledNotification
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindDueScheduledNotifications scan: %w", err)
		}
		row := &models.ScheduledNotification{}
		if err := proto.Unmarshal(blob, row); err != nil {
			return nil, fmt.Errorf("FindDueScheduledNotifications unmarshal: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindDueScheduledNotifications iterate: %w", err)
	}
	return out, nil
}
