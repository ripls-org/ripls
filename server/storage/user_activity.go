package storage

import (
	"context"
	"fmt"
	"time"
)

// This file owns the user_active_day telemetry table (#2665): one row
// per (user, UTC day), upserted by the activitystamp middleware and read
// by the ops activity digest for true active-user counts. day_utc is
// only the upsert dedup key; window queries range over the first/last
// seen timestamps so the digest's local-timezone windows never depend on
// how the day key was bucketed.

// UserActiveSpan is one user's activity span within a single UTC day.
type UserActiveSpan struct {
	UserID           string
	FirstSeenUnixSec int64
	LastSeenUnixSec  int64
}

// maxUserActiveSpanRows bounds FindUserActiveSpansInWindow. Rows are
// (user × day) so even a year-long window over thousands of users stays
// far below this; the LIMIT is an unbounded-query safety net, not an
// expected truncation point.
const maxUserActiveSpanRows = 500_000

// RecordUserActivity upserts the (user, UTC day) stamp for one observed
// authenticated request, widening the day's seen-span. Callers
// rate-limit invocations (the middleware stamps at most once per user
// per interval), so this is a low-frequency write.
func (s *ProtoSQLStorage) RecordUserActivity(ctx context.Context, userID string, atUnixSec int64) error {
	if userID == "" {
		return fmt.Errorf("RecordUserActivity: userID must not be empty")
	}
	dayUTC := time.Unix(atUnixSec, 0).UTC().Format("2006-01-02")
	query := fmt.Sprintf(`
		INSERT INTO user_active_day (user_id, day_utc, first_seen_unix_sec, last_seen_unix_sec)
		VALUES (%s, %s, %s, %s)
		ON CONFLICT (user_id, day_utc) DO UPDATE SET
		  first_seen_unix_sec = LEAST(user_active_day.first_seen_unix_sec, EXCLUDED.first_seen_unix_sec),
		  last_seen_unix_sec = GREATEST(user_active_day.last_seen_unix_sec, EXCLUDED.last_seen_unix_sec)
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(3), s.dbSpec.Placeholder(4))
	if _, err := s.exec.ExecContext(ctx, query, userID, dayUTC, atUnixSec, atUnixSec); err != nil {
		return fmt.Errorf("RecordUserActivity: %w", err)
	}
	return nil
}

// FindUserActiveSpansInWindow returns the activity spans overlapping the
// half-open window [startUnixSec, endUnixSec). Callers derive distinct
// active-user counts and per-day breakdowns from the spans in Go.
func (s *ProtoSQLStorage) FindUserActiveSpansInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]UserActiveSpan, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("FindUserActiveSpansInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT user_id, first_seen_unix_sec, last_seen_unix_sec
		FROM user_active_day
		WHERE last_seen_unix_sec >= %s
		  AND first_seen_unix_sec < %s
		LIMIT %s
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(3))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec, maxUserActiveSpanRows)
	if err != nil {
		return nil, fmt.Errorf("FindUserActiveSpansInWindow: %w", err)
	}
	defer rows.Close()

	var out []UserActiveSpan
	for rows.Next() {
		var span UserActiveSpan
		if err := rows.Scan(&span.UserID, &span.FirstSeenUnixSec, &span.LastSeenUnixSec); err != nil {
			return nil, fmt.Errorf("FindUserActiveSpansInWindow scan: %w", err)
		}
		out = append(out, span)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindUserActiveSpansInWindow iterate: %w", err)
	}
	return out, nil
}

// EarliestUserActivityUnixSec returns the first_seen timestamp of the
// oldest stamp row, or 0 when no activity has been recorded yet. The
// digest uses it to decide whether the stamp table covers a window (the
// table only accrues from the feature's deploy forward).
func (s *ProtoSQLStorage) EarliestUserActivityUnixSec(ctx context.Context) (int64, error) {
	var earliest *int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MIN(first_seen_unix_sec) FROM user_active_day`,
	).Scan(&earliest); err != nil {
		return 0, fmt.Errorf("EarliestUserActivityUnixSec: %w", err)
	}
	if earliest == nil {
		return 0, nil
	}
	return *earliest, nil
}

// PruneUserActivityBefore deletes stamp rows whose last activity is
// older than beforeUnixSec, returning the number of rows removed. The
// weekly digest job calls it to keep roughly a year of history.
func (s *ProtoSQLStorage) PruneUserActivityBefore(ctx context.Context, beforeUnixSec int64) (int64, error) {
	res, err := s.exec.ExecContext(ctx, fmt.Sprintf(
		`DELETE FROM user_active_day WHERE last_seen_unix_sec < %s`, s.dbSpec.Placeholder(1),
	), beforeUnixSec)
	if err != nil {
		return 0, fmt.Errorf("PruneUserActivityBefore: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("PruneUserActivityBefore rows affected: %w", err)
	}
	return n, nil
}
