package storage

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// FindEventsForUserSince returns community events across every community the
// user belongs to that occurred strictly after sinceUnixSec, oldest first,
// bounded by limit.
//
// One JOIN replaces what the per-community realtime layer did with N round
// trips: the caller's membership is resolved inside the query, so a portfolio
// of 50 communities costs the same single query as a portfolio of 3 (#2867).
// This exists because the obvious composition — QueryByField per community, or
// QueryByFieldIn over the whole list — has no time predicate and no bound, so
// it loads the complete event history of every community into memory before
// discarding almost all of it.
//
// Membership is filtered to live rows, matching the default soft-delete
// behavior of QueryByField: a user who left a community stops receiving its
// events. Communities that are themselves soft-deleted are deliberately NOT
// filtered out — community_user rows survive a community delete (they are the
// restore list; see lifecycle.go DeleteCommunity, whose cascade list omits
// community_user), and the COMMUNITY_DELETED event is exactly what a client
// that was offline during the delete needs in order to navigate away.
//
// A caller that receives `limit` rows should assume older events were dropped.
func (s *ProtoSQLStorage) FindEventsForUserSince(
	ctx context.Context, userID string, sinceUnixSec int64, limit int,
) ([]*models.CommunityEvent, error) {
	if userID == "" {
		return nil, fmt.Errorf("FindEventsForUserSince: userID is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("FindEventsForUserSince: limit must be positive, got %d", limit)
	}

	eventTable, err := s.quotedTableFor("ripls.models.CommunityEvent")
	if err != nil {
		return nil, err
	}
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}

	// Args: $1 = userID, $2 = sinceUnixSec (exclusive), $3 = limit.
	//
	// Ordered ASC so a client can advance its high-water mark as it walks the
	// slice, and so a truncated page is the OLDEST events rather than a
	// arbitrary window — the client's next poll then continues from where this
	// one stopped instead of skipping the gap.
	query := fmt.Sprintf(
		`SELECT e.binary_proto
		 FROM %s e
		 JOIN %s cu ON cu.community_id = e.community_id
		 WHERE cu.user_id = %s
		   AND (cu.deleted_deleted_by_user_id IS NULL OR cu.deleted_deleted_by_user_id = '')
		   AND e.occurred_at_unix_sec > %s
		 ORDER BY e.occurred_at_unix_sec ASC, e.id ASC
		 LIMIT %s`,
		eventTable, cuTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(3),
	)

	rows, err := s.db.QueryContext(ctx, query, userID, sinceUnixSec, limit)
	if err != nil {
		return nil, fmt.Errorf("FindEventsForUserSince: %w", err)
	}
	defer rows.Close()

	var out []*models.CommunityEvent
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindEventsForUserSince scan: %w", err)
		}
		event := &models.CommunityEvent{}
		if err := proto.Unmarshal(blob, event); err != nil {
			return nil, fmt.Errorf("FindEventsForUserSince unmarshal: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindEventsForUserSince iterate: %w", err)
	}
	return out, nil
}
