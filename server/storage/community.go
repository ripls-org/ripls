package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// GetCommunityWithMembership fetches a Community and the caller's CommunityUser
// row in a single round-trip via LEFT JOIN. Membership is nil if the user is
// not a member. The community's Deleted field is preserved verbatim — callers
// decide whether to treat soft-deleted communities as not-found, restorable,
// or otherwise; this method is policy-free.
//
// Returns ErrRecordNotFound only when the community itself does not exist.
// Soft-deleted communities still return successfully so that auth helpers can
// distinguish "deleted, caller is a member" (FailedPrecondition for restore
// flow) from "deleted, caller is not a member" (NotFound, no information leak).
func (s *ProtoSQLStorage) GetCommunityWithMembership(
	ctx context.Context, communityID, userID string,
) (*models.Community, *models.CommunityUser, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, nil, err
	}
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, nil, err
	}

	query := fmt.Sprintf(
		`SELECT c.binary_proto, cu.binary_proto FROM %s c LEFT JOIN %s cu ON cu.community_id = c.id AND cu.user_id = %s WHERE c.id = %s`,
		communityTable, cuTable,
		s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(1),
	)

	var communityProto, membershipProto []byte
	err = s.db.QueryRowContext(ctx, query, communityID, userID).Scan(&communityProto, &membershipProto)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("community %s: %w", communityID, ErrRecordNotFound)
		}
		return nil, nil, fmt.Errorf("GetCommunityWithMembership query: %w", err)
	}

	community := &models.Community{}
	if err := proto.Unmarshal(communityProto, community); err != nil {
		return nil, nil, fmt.Errorf("unmarshal community: %w", err)
	}

	var membership *models.CommunityUser
	if len(membershipProto) > 0 {
		membership = &models.CommunityUser{}
		if err := proto.Unmarshal(membershipProto, membership); err != nil {
			return nil, nil, fmt.Errorf("unmarshal community_user: %w", err)
		}
	}

	return community, membership, nil
}

// CommunityWithMembership pairs a Community with the caller's CommunityUser
// row (nil when the caller is not a member). Returned by
// GetCommunitiesWithMembership for batch lookups.
type CommunityWithMembership struct {
	Community  *models.Community
	Membership *models.CommunityUser
}

// GetCommunitiesWithMembership is the batch variant of
// GetCommunityWithMembership. For N community IDs, it issues a SINGLE
// JOIN query that returns each existing community paired with the caller's
// membership row (nil when not a member). This collapses an O(N) round-trip
// loop down to O(1) for multi-community surfaces (search, feed, list-all-my).
//
// The community's Deleted field is preserved verbatim — policy stays in the
// auth layer.
//
// Communities that do not exist are simply absent from the returned map.
// Callers infer "missing" by key absence, no error is raised. Empty input
// returns an empty map and no error.
func (s *ProtoSQLStorage) GetCommunitiesWithMembership(
	ctx context.Context, communityIDs []string, userID string,
) (map[string]CommunityWithMembership, error) {
	if len(communityIDs) == 0 {
		return map[string]CommunityWithMembership{}, nil
	}

	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, err
	}
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}

	result := make(map[string]CommunityWithMembership, len(communityIDs))

	// Chunk to stay below the Postgres parameter limit (mirrors GetByIDs).
	for start := 0; start < len(communityIDs); start += maxBatchGetSize {
		end := start + maxBatchGetSize
		if end > len(communityIDs) {
			end = len(communityIDs)
		}
		chunk := communityIDs[start:end]

		// One placeholder per community ID, plus one for the userID at position 1.
		// Args layout: $1 = userID, $2..$N+1 = community IDs.
		args := make([]any, 0, len(chunk)+1)
		args = append(args, userID)
		placeholders := make([]string, len(chunk))
		for i, cid := range chunk {
			placeholders[i] = s.dbSpec.Placeholder(i + 2)
			args = append(args, cid)
		}

		query := fmt.Sprintf(
			`SELECT c.binary_proto, cu.binary_proto FROM %s c LEFT JOIN %s cu ON cu.community_id = c.id AND cu.user_id = %s WHERE c.id IN (%s)`,
			communityTable, cuTable,
			s.dbSpec.Placeholder(1),
			strings.Join(placeholders, ", "),
		)

		// Closure so `defer rows.Close()` is scoped to this chunk rather than the
		// whole batch — see protosql_batch.go's BatchGetByIDs for the same shape.
		if err := func() error {
			rows, err := s.db.QueryContext(ctx, query, args...)
			if err != nil {
				return fmt.Errorf("GetCommunitiesWithMembership query: %w", err)
			}
			defer rows.Close()

			for rows.Next() {
				var communityProto, membershipProto []byte
				if err := rows.Scan(&communityProto, &membershipProto); err != nil {
					return fmt.Errorf("scan: %w", err)
				}
				community := &models.Community{}
				if err := proto.Unmarshal(communityProto, community); err != nil {
					return fmt.Errorf("unmarshal community: %w", err)
				}
				pair := CommunityWithMembership{Community: community}
				if len(membershipProto) > 0 {
					m := &models.CommunityUser{}
					if err := proto.Unmarshal(membershipProto, m); err != nil {
						return fmt.Errorf("unmarshal community_user: %w", err)
					}
					pair.Membership = m
				}
				result[community.Id] = pair
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("rows iterate: %w", err)
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// ClaimCommunityRestore atomically clears the soft-delete marker on
// the community row, returning true exactly once across concurrent
// callers. The race-claim is intentionally narrow — it touches only
// the flat deleted_* columns and not binary_proto. The caller MUST
// follow up with a full storage.Update to resync binary_proto, the
// owner_user_id field, and the cleared deleted_snapshot. Until that
// follow-up lands, GetByID returns a Community whose binary_proto
// still says deleted; the read filter no longer hides the row.
//
// Mirrors the DecrementFieldIfPositive pattern in protosql.go: a
// conditional UPDATE plus RowsAffected() inspection. See the
// RestoreCommunity service handler for the full write-then-resync
// sequence and the crash-window recovery branch.
func (s *ProtoSQLStorage) ClaimCommunityRestore(ctx context.Context, communityID string) (bool, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return false, err
	}

	// Also clears purge_reminder_sent_at_unix_sec and
	// snapshot_member_user_ids so a delete → restore → delete cycle
	// gets a fresh reminder window AND a fresh eligible-restorer
	// snapshot. Both flat columns are the source of truth for their
	// respective queries — clearing the proto layer alone (which
	// the subsequent storage.Update does) would leave the columns
	// stuck because optional-message-field updates skip nested
	// columns when the parent message is nil and repeated-field
	// updates are skipped entirely. See extractFieldValuesRecursive
	// in protosql_schema.go.
	query := fmt.Sprintf(
		`UPDATE %s SET deleted_deleted_by_user_id = NULL, deleted_deleted_at_unix_sec = 0, purge_reminder_sent_at_unix_sec = 0, snapshot_member_user_ids = NULL WHERE id = %s AND deleted_deleted_by_user_id IS NOT NULL`,
		communityTable, s.dbSpec.Placeholder(1),
	)
	result, err := s.db.ExecContext(ctx, query, communityID)
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityRestore: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityRestore rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// ProbeCommunityFlatDeleted reads the deleted_deleted_by_user_id
// flat column directly, bypassing the proto layer. Used by
// RestoreCommunity's crash-window recovery branch to distinguish
// "race lost" (flat column says active because someone else
// restored) from "stuck mid-restore" (flat column says active but
// binary_proto still has Deleted set from a prior crashed
// attempt).
func (s *ProtoSQLStorage) ProbeCommunityFlatDeleted(ctx context.Context, communityID string) (deletedByUserID string, found bool, err error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return "", false, err
	}
	query := fmt.Sprintf(
		`SELECT COALESCE(deleted_deleted_by_user_id, '') FROM %s WHERE id = %s`,
		communityTable, s.dbSpec.Placeholder(1),
	)
	row := s.db.QueryRowContext(ctx, query, communityID)
	if err := row.Scan(&deletedByUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("ProbeCommunityFlatDeleted: %w", err)
	}
	return deletedByUserID, true, nil
}

// ClaimCommunityOwnerHandoff atomically transfers ownership of a
// community from fromUserID to toUserID, returning true exactly once
// across concurrent callers. The conditional UPDATE succeeds only
// when (a) the current owner is still fromUserID, (b) the row is not
// soft-deleted, and (c) toUserID has an active CommunityUser row in
// the community. If any predicate fails the UPDATE matches no row
// and the function returns false — the caller distinguishes the
// reasons (ownership_changed vs candidate_not_member) with a follow-up
// read.
//
// Like ClaimCommunityRestore, this only touches the flat
// owner_user_id column. The caller MUST follow up with a full
// storage.Update to resync binary_proto. Until that resync lands,
// reads via GetByID see the old owner in the proto layer; reads via
// the flat column see the new owner. The community_owner_required
// CHECK constraint stays satisfied because we transition from one
// non-empty owner to another. See LeaveCommunity's leaveAsOwner
// branch for the full sequence.
func (s *ProtoSQLStorage) ClaimCommunityOwnerHandoff(ctx context.Context, communityID, fromUserID, toUserID string) (bool, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return false, err
	}
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return false, err
	}

	// Args layout: $1 = toUserID (new owner), $2 = communityID,
	// $3 = fromUserID (current owner).
	query := fmt.Sprintf(
		`UPDATE %s SET owner_user_id = %s
		 WHERE id = %s
		   AND owner_user_id = %s
		   AND (deleted_deleted_by_user_id IS NULL OR deleted_deleted_by_user_id = '')
		   AND EXISTS (
		     SELECT 1 FROM %s
		     WHERE community_id = %s
		       AND user_id = %s
		       AND (deleted_deleted_by_user_id IS NULL OR deleted_deleted_by_user_id = '')
		   )`,
		communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(3),
		cuTable,
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(1),
	)
	result, err := s.db.ExecContext(ctx, query, toUserID, communityID, fromUserID)
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityOwnerHandoff: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityOwnerHandoff rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// ClaimCommunityUserRejoin atomically un-soft-deletes the caller's
// CommunityUser row in a community, returning true exactly once
// across concurrent callers. Mirrors the ClaimCommunityRestore /
// ClaimCommunityOwnerHandoff pattern: a conditional UPDATE plus
// RowsAffected() inspection, with the window check pushed into SQL
// so a slow handler can't reopen the window accidentally.
//
// The UPDATE succeeds iff (a) the row exists, (b) it is currently
// soft-deleted (deleted_deleted_by_user_id IS NOT NULL), AND (c)
// the soft-delete is within rejoinWindowSeconds of nowUnixSec. The
// boundary is inclusive — a row whose deleted_at is exactly
// rejoinWindowSeconds ago still qualifies.
//
// Like the other claim helpers, this only touches the flat
// deleted_* columns. The caller MUST follow up with a full
// storage.Update to resync binary_proto. Until that follow-up
// lands, GetByID returns a CommunityUser whose binary_proto still
// says deleted; the read filter no longer hides the row.
//
// On loss, callers should disambiguate via
// ProbeCommunityUserDeletedAt: zero deleted_at means the row is
// already active (race-lost or stale UI); a deleted_at older than
// the window means the row exists but cannot be rejoined; a
// not-found means there's no membership at all.
func (s *ProtoSQLStorage) ClaimCommunityUserRejoin(
	ctx context.Context, communityID, userID string,
	nowUnixSec, rejoinWindowSeconds int64,
) (bool, error) {
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return false, err
	}

	// Args layout: $1 = communityID, $2 = userID,
	// $3 = nowUnixSec - rejoinWindowSeconds (the floor below which
	// the window has expired).
	floor := nowUnixSec - rejoinWindowSeconds
	query := fmt.Sprintf(
		`UPDATE %s
		 SET deleted_deleted_by_user_id = NULL, deleted_deleted_at_unix_sec = 0
		 WHERE community_id = %s
		   AND user_id = %s
		   AND deleted_deleted_by_user_id IS NOT NULL
		   AND deleted_deleted_at_unix_sec >= %s`,
		cuTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(3),
	)
	result, err := s.db.ExecContext(ctx, query, communityID, userID, floor)
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityUserRejoin: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityUserRejoin rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// ProbeCommunityUserDeletedAt reads the flat
// deleted_deleted_at_unix_sec column for a (community_id, user_id)
// pair, bypassing the proto layer. Used by RejoinCommunity to
// distinguish race-loss outcomes:
//
//   - found=false: no CommunityUser row exists for this pair → caller
//     was never a member or their row has been hard-deleted by the
//     daily purge job.
//   - found=true, deletedAtUnixSec=0: row exists and is currently
//     active → either a concurrent rejoin already won, or the caller
//     was already a member (stale UI).
//   - found=true, deletedAtUnixSec>0: row exists, soft-deleted at
//     that timestamp → caller can compare against the window and
//     report rejoin_window_expired vs already_member as appropriate.
func (s *ProtoSQLStorage) ProbeCommunityUserDeletedAt(
	ctx context.Context, communityID, userID string,
) (deletedAtUnixSec int64, found bool, err error) {
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return 0, false, err
	}
	query := fmt.Sprintf(
		`SELECT COALESCE(deleted_deleted_at_unix_sec, 0) FROM %s WHERE community_id = %s AND user_id = %s`,
		cuTable, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2),
	)
	row := s.db.QueryRowContext(ctx, query, communityID, userID)
	if err := row.Scan(&deletedAtUnixSec); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("ProbeCommunityUserDeletedAt: %w", err)
	}
	return deletedAtUnixSec, true, nil
}

// ClaimCommunityPurgeReminder atomically marks a community as
// having received its day-before-purge reminder, returning true
// exactly once across concurrent job runs. Mirrors the
// ClaimCommunityRestore / ClaimCommunityOwnerHandoff /
// ClaimCommunityUserRejoin pattern: a conditional UPDATE plus
// RowsAffected() inspection.
//
// The UPDATE succeeds iff (a) the community is currently
// soft-deleted (deleted_deleted_by_user_id IS NOT NULL), AND (b)
// the purge_reminder_sent_at_unix_sec column is currently 0 or
// NULL (set-once semantics).
//
// Like the other claim helpers, this only touches the flat
// purge_reminder_sent_at_unix_sec column. The job does NOT
// follow up with a storage.Update to resync binary_proto — the
// flat column is the source of truth for the daily query, and a
// future RestoreCommunity will rewrite both layers in lockstep
// (Restore clears PurgeReminderSentAtUnixSec on the in-memory
// proto and the subsequent storage.Update propagates that to the
// flat column).
func (s *ProtoSQLStorage) ClaimCommunityPurgeReminder(
	ctx context.Context, communityID string, nowUnixSec int64,
) (bool, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return false, err
	}

	// Args layout: $1 = nowUnixSec, $2 = communityID.
	query := fmt.Sprintf(
		`UPDATE %s
		 SET purge_reminder_sent_at_unix_sec = %s
		 WHERE id = %s
		   AND deleted_deleted_by_user_id IS NOT NULL
		   AND (purge_reminder_sent_at_unix_sec IS NULL OR purge_reminder_sent_at_unix_sec = 0)`,
		communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	result, err := s.db.ExecContext(ctx, query, nowUnixSec, communityID)
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityPurgeReminder: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimCommunityPurgeReminder rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

// FindCommunitiesNeedingPurgeReminder returns soft-deleted
// communities whose deleted_at is at least leadSeconds in the
// past AND whose purge_reminder_sent_at_unix_sec is still unset.
// Bounded by limit to cap per-run batch size.
//
// This is the rare query that opts INTO deleted rows — it
// queries the flat columns directly rather than going through
// QueryByField (which strips deleted rows by default). The
// returned communities have their full binary_proto unmarshalled
// so the caller can read DeletedSnapshot.MemberUserIds and build
// the recipient list.
func (s *ProtoSQLStorage) FindCommunitiesNeedingPurgeReminder(
	ctx context.Context, nowUnixSec, leadSeconds int64, limit int,
) ([]*models.Community, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	floor := nowUnixSec - leadSeconds
	query := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE deleted_deleted_at_unix_sec > 0
		   AND deleted_deleted_at_unix_sec <= %s
		   AND (purge_reminder_sent_at_unix_sec IS NULL OR purge_reminder_sent_at_unix_sec = 0)
		 ORDER BY deleted_deleted_at_unix_sec ASC
		 LIMIT %s`,
		communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	rows, err := s.db.QueryContext(ctx, query, floor, limit)
	if err != nil {
		return nil, fmt.Errorf("FindCommunitiesNeedingPurgeReminder: %w", err)
	}
	defer rows.Close()

	var out []*models.Community
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindCommunitiesNeedingPurgeReminder scan: %w", err)
		}
		c := &models.Community{}
		if err := proto.Unmarshal(blob, c); err != nil {
			return nil, fmt.Errorf("FindCommunitiesNeedingPurgeReminder unmarshal: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindCommunitiesNeedingPurgeReminder iterate: %w", err)
	}
	return out, nil
}

// FindExpiredCommunities returns soft-deleted communities whose
// deleted_at is older than purgeWindowSeconds. Powers #1620's
// daily hard-delete job.
//
// Like FindCommunitiesNeedingPurgeReminder, this query opts INTO
// soft-deleted rows by reading the flat columns directly. Bounded
// by limit. Ordered oldest-first so a backlog drains FIFO.
func (s *ProtoSQLStorage) FindExpiredCommunities(
	ctx context.Context, nowUnixSec, purgeWindowSeconds int64, limit int,
) ([]*models.Community, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	floor := nowUnixSec - purgeWindowSeconds
	query := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE deleted_deleted_at_unix_sec > 0
		   AND deleted_deleted_at_unix_sec < %s
		 ORDER BY deleted_deleted_at_unix_sec ASC
		 LIMIT %s`,
		communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	rows, err := s.exec.QueryContext(ctx, query, floor, limit)
	if err != nil {
		return nil, fmt.Errorf("FindExpiredCommunities: %w", err)
	}
	defer rows.Close()

	var out []*models.Community
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindExpiredCommunities scan: %w", err)
		}
		c := &models.Community{}
		if err := proto.Unmarshal(blob, c); err != nil {
			return nil, fmt.Errorf("FindExpiredCommunities unmarshal: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindExpiredCommunities iterate: %w", err)
	}
	return out, nil
}

// FindExpiredCommunityUserIDs returns the IDs of soft-deleted
// CommunityUser rows whose deleted_at is older than
// rejoinWindowSeconds. Powers #1620's per-user rejoin-window
// purge: a soft-deleted membership row whose 30-day rejoin
// window has expired is hard-deleted by the daily job.
//
// Returns IDs only because the caller bulk-deletes via
// DeleteByFieldIn; no need to round-trip the full proto. Bounded
// by limit. Ordered oldest-first.
func (s *ProtoSQLStorage) FindExpiredCommunityUserIDs(
	ctx context.Context, nowUnixSec, rejoinWindowSeconds int64, limit int,
) ([]string, error) {
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	floor := nowUnixSec - rejoinWindowSeconds
	query := fmt.Sprintf(
		`SELECT id FROM %s
		 WHERE deleted_deleted_at_unix_sec > 0
		   AND deleted_deleted_at_unix_sec < %s
		 ORDER BY deleted_deleted_at_unix_sec ASC
		 LIMIT %s`,
		cuTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	rows, err := s.exec.QueryContext(ctx, query, floor, limit)
	if err != nil {
		return nil, fmt.Errorf("FindExpiredCommunityUserIDs: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("FindExpiredCommunityUserIDs scan: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindExpiredCommunityUserIDs iterate: %w", err)
	}
	return out, nil
}

// CountCommunities returns the total number of community rows
// (active + soft-deleted) in the database. Powers #1620's
// percent-of-total threshold gate so the daily job's
// abort-if-too-many threshold scales with database size.
func (s *ProtoSQLStorage) CountCommunities(ctx context.Context) (int, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, communityTable)
	var count int
	if err := s.exec.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountCommunities: %w", err)
	}
	return count, nil
}

// FindCommunitiesEligibleForRestore returns soft-deleted
// communities whose snapshot_member_user_ids array contains
// userID — i.e., the set the caller is eligible to restore per
// docs/community_delete_and_leave.md §2.5. Bounded by limit.
//
// This is the rare query that opts INTO deleted rows — it
// queries the flat columns directly. Returns full
// *models.Community so the service handler can build the API
// response items including deleted_at and deleted_by_user_id
// from community.GetDeleted().
func (s *ProtoSQLStorage) FindCommunitiesEligibleForRestore(
	ctx context.Context, userID string, limit int,
) ([]*models.Community, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}
	query := fmt.Sprintf(
		`SELECT binary_proto FROM %s
		 WHERE deleted_deleted_at_unix_sec > 0
		   AND snapshot_member_user_ids @> %s
		 ORDER BY deleted_deleted_at_unix_sec ASC
		 LIMIT %s`,
		communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
	)
	rows, err := s.db.QueryContext(ctx, query, pq.Array([]string{userID}), limit)
	if err != nil {
		return nil, fmt.Errorf("FindCommunitiesEligibleForRestore: %w", err)
	}
	defer rows.Close()

	var out []*models.Community
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindCommunitiesEligibleForRestore scan: %w", err)
		}
		c := &models.Community{}
		if err := proto.Unmarshal(blob, c); err != nil {
			return nil, fmt.Errorf("FindCommunitiesEligibleForRestore unmarshal: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindCommunitiesEligibleForRestore iterate: %w", err)
	}
	return out, nil
}

// RejoinableCommunity pairs a soft-deleted CommunityUser row with
// its (still-active) parent Community for the §2.6 rejoin
// listing surface (#1721 / RPC #1723). Mirrors the
// CommunityWithMembership shape used by
// GetCommunityWithMembership.
type RejoinableCommunity struct {
	Community  *models.Community
	Membership *models.CommunityUser
}

// FindRejoinableCommunitiesForUser returns the active communities
// the caller can rejoin without a fresh invite — i.e., the caller
// has a soft-deleted CommunityUser row less than windowSeconds
// old AND the community itself is still active. Enforces the
// §2.7 cross-cut: per-user rejoin window does not override a
// community's deleted state. Bounded by limit.
//
// Inclusive boundary semantics: a row with
// deleted_at_unix_sec >= nowUnixSec - windowSeconds qualifies.
// Matches ClaimCommunityUserRejoin's inclusive boundary so the
// listing surface and the action surface agree.
//
// Single JOIN query against community_user ⋈ community.
func (s *ProtoSQLStorage) FindRejoinableCommunitiesForUser(
	ctx context.Context, userID string, nowUnixSec, windowSeconds int64, limit int,
) ([]RejoinableCommunity, error) {
	communityTable, err := s.quotedTableFor("ripls.models.Community")
	if err != nil {
		return nil, err
	}
	cuTable, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	floor := nowUnixSec - windowSeconds
	// Args: $1 = userID, $2 = floor (deleted_at lower bound), $3 = limit.
	query := fmt.Sprintf(
		`SELECT cu.binary_proto, c.binary_proto
		 FROM %s cu
		 JOIN %s c ON c.id = cu.community_id
		 WHERE cu.user_id = %s
		   AND cu.deleted_deleted_by_user_id IS NOT NULL
		   AND cu.deleted_deleted_at_unix_sec >= %s
		   AND (c.deleted_deleted_by_user_id IS NULL OR c.deleted_deleted_by_user_id = '')
		 ORDER BY cu.deleted_deleted_at_unix_sec DESC
		 LIMIT %s`,
		cuTable, communityTable,
		s.dbSpec.Placeholder(1),
		s.dbSpec.Placeholder(2),
		s.dbSpec.Placeholder(3),
	)
	rows, err := s.db.QueryContext(ctx, query, userID, floor, limit)
	if err != nil {
		return nil, fmt.Errorf("FindRejoinableCommunitiesForUser: %w", err)
	}
	defer rows.Close()

	var out []RejoinableCommunity
	for rows.Next() {
		var cuProto, cProto []byte
		if err := rows.Scan(&cuProto, &cProto); err != nil {
			return nil, fmt.Errorf("FindRejoinableCommunitiesForUser scan: %w", err)
		}
		cu := &models.CommunityUser{}
		if err := proto.Unmarshal(cuProto, cu); err != nil {
			return nil, fmt.Errorf("FindRejoinableCommunitiesForUser unmarshal community_user: %w", err)
		}
		c := &models.Community{}
		if err := proto.Unmarshal(cProto, c); err != nil {
			return nil, fmt.Errorf("FindRejoinableCommunitiesForUser unmarshal community: %w", err)
		}
		out = append(out, RejoinableCommunity{Community: c, Membership: cu})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindRejoinableCommunitiesForUser iterate: %w", err)
	}
	return out, nil
}
