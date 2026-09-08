package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ChatActivityRow is one community's chat activity for a digest window.
type ChatActivityRow struct {
	CommunityID               string
	MessageCount              int
	DistinctSenderCount       int
	DistinctConversationCount int
}

// ChatTopicCounts is the global breakdown of user chat messages in a
// digest window, grouped by the kind of entity the conversation is
// scoped to. Fields correspond to the ConversationTopic oneof.
type ChatTopicCounts struct {
	Transfer   int // about a specific loan or giveaway
	Request    int // about a specific request
	Experience int // about a specific event
	Gear       int // about a specific gear item (perpetual)
	Community  int // community-wide perpetual conversation
}

// Total returns the sum across all topic types.
func (c ChatTopicCounts) Total() int {
	return c.Transfer + c.Request + c.Experience + c.Gear + c.Community
}

// ActivityDigestCadence keys the row in activity_digest_state. The
// daily and weekly digests use separate rows so they can claim
// independently.
type ActivityDigestCadence string

const (
	// CadenceDaily is the row owned by the daily digest job.
	CadenceDaily ActivityDigestCadence = "daily"
	// CadenceWeekly is the row owned by the weekly digest job.
	CadenceWeekly ActivityDigestCadence = "weekly"
)

// FindEventsInWindow returns CommunityEvent rows whose occurred_at falls
// in the half-open window [startUnixSec, endUnixSec), ordered ASC by
// occurred_at. CommunityEvent has no soft-delete primitive — every row
// in the table is a real event — so no deleted-filter clause is needed
// or possible. UNDONE event types are real rows; the digest builder
// filters them out at format time.
func (s *ProtoSQLStorage) FindEventsInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]*models.CommunityEvent, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("FindEventsInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "community_event"
		WHERE occurred_at_unix_sec >= %s
		  AND occurred_at_unix_sec < %s
		ORDER BY occurred_at_unix_sec ASC, id ASC
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec)
	if err != nil {
		return nil, fmt.Errorf("FindEventsInWindow: %w", err)
	}
	defer rows.Close()

	var out []*models.CommunityEvent
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindEventsInWindow scan: %w", err)
		}
		ev := &models.CommunityEvent{}
		if err := proto.Unmarshal(blob, ev); err != nil {
			return nil, fmt.Errorf("FindEventsInWindow unmarshal: %w", err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindEventsInWindow iterate: %w", err)
	}
	return out, nil
}

// CountUserMessagesByCommunity returns per-community chat activity in
// the half-open window [startUnixSec, endUnixSec). It joins chat_message
// to chat_conversation to recover community_id, filters out system
// messages (user_message_sender_id present) and soft-deleted rows, and
// aggregates message count, distinct sender count, and distinct
// conversation count per community.
func (s *ProtoSQLStorage) CountUserMessagesByCommunity(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]ChatActivityRow, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("CountUserMessagesByCommunity: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT c.community_id,
		       COUNT(*) AS message_count,
		       COUNT(DISTINCT m.user_message_sender_id) AS distinct_senders,
		       COUNT(DISTINCT m.conversation_id) AS distinct_conversations
		FROM "chat_message" m
		JOIN "chat_conversation" c ON c.id = m.conversation_id
		WHERE m.sent_at_unix_sec >= %s
		  AND m.sent_at_unix_sec < %s
		  AND m.user_message_sender_id IS NOT NULL
		  AND m.user_message_sender_id <> ''
		  AND (m.deleted_deleted_at_unix_sec = 0 OR m.deleted_deleted_at_unix_sec IS NULL)
		GROUP BY c.community_id
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec)
	if err != nil {
		return nil, fmt.Errorf("CountUserMessagesByCommunity: %w", err)
	}
	defer rows.Close()

	var out []ChatActivityRow
	for rows.Next() {
		var r ChatActivityRow
		if err := rows.Scan(&r.CommunityID, &r.MessageCount, &r.DistinctSenderCount, &r.DistinctConversationCount); err != nil {
			return nil, fmt.Errorf("CountUserMessagesByCommunity scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("CountUserMessagesByCommunity iterate: %w", err)
	}
	return out, nil
}

// Conversation topic_type values, as stored on the conversation row. The
// digest counts messages per topic, so these must match what the chat layer
// writes.
const (
	topicTransfer   = "transfer"
	topicRequest    = "request"
	topicExperience = "experience"
	topicGear       = "gear"
	topicCommunity  = "community"
)

// CountUserMessagesByTopicType returns the count of user-authored chat
// messages in the half-open window [startUnixSec, endUnixSec) grouped
// by the kind of entity the conversation's topic points at
// (transfer / request / experience / gear / community). System
// messages and soft-deleted rows are excluded.
func (s *ProtoSQLStorage) CountUserMessagesByTopicType(
	ctx context.Context, startUnixSec, endUnixSec int64,
) (ChatTopicCounts, error) {
	var out ChatTopicCounts
	if endUnixSec <= startUnixSec {
		return out, fmt.Errorf("CountUserMessagesByTopicType: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT
		  CASE
		    WHEN c.topic_transfer_id IS NOT NULL AND c.topic_transfer_id <> '' THEN 'transfer'
		    WHEN c.topic_request_id IS NOT NULL AND c.topic_request_id <> '' THEN 'request'
		    WHEN c.topic_experience_id IS NOT NULL AND c.topic_experience_id <> '' THEN 'experience'
		    WHEN c.topic_gear_id IS NOT NULL AND c.topic_gear_id <> '' THEN 'gear'
		    WHEN c.topic_community_id IS NOT NULL AND c.topic_community_id <> '' THEN 'community'
		    ELSE 'other'
		  END AS topic_type,
		  COUNT(*) AS msg_count
		FROM "chat_message" m
		JOIN "chat_conversation" c ON c.id = m.conversation_id
		WHERE m.sent_at_unix_sec >= %s
		  AND m.sent_at_unix_sec < %s
		  AND m.user_message_sender_id IS NOT NULL
		  AND m.user_message_sender_id <> ''
		  AND (m.deleted_deleted_at_unix_sec = 0 OR m.deleted_deleted_at_unix_sec IS NULL)
		GROUP BY topic_type
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec)
	if err != nil {
		return out, fmt.Errorf("CountUserMessagesByTopicType: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var topic string
		var n int
		if err := rows.Scan(&topic, &n); err != nil {
			return out, fmt.Errorf("CountUserMessagesByTopicType scan: %w", err)
		}
		switch topic {
		case topicTransfer:
			out.Transfer = n
		case topicRequest:
			out.Request = n
		case topicExperience:
			out.Experience = n
		case topicGear:
			out.Gear = n
		case topicCommunity:
			out.Community = n
		}
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("CountUserMessagesByTopicType iterate: %w", err)
	}
	return out, nil
}

// FindUsersCreatedInWindow returns User rows whose created_at falls in
// the half-open window [startUnixSec, endUnixSec). Soft-deleted users
// are excluded.
func (s *ProtoSQLStorage) FindUsersCreatedInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]*models.User, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("FindUsersCreatedInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "user"
		WHERE created_at >= %s
		  AND created_at < %s
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		ORDER BY created_at ASC, id ASC
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec)
	if err != nil {
		return nil, fmt.Errorf("FindUsersCreatedInWindow: %w", err)
	}
	defer rows.Close()

	var out []*models.User
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindUsersCreatedInWindow scan: %w", err)
		}
		u := &models.User{}
		if err := proto.Unmarshal(blob, u); err != nil {
			return nil, fmt.Errorf("FindUsersCreatedInWindow unmarshal: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindUsersCreatedInWindow iterate: %w", err)
	}
	return out, nil
}

// CountDistinctInteractiveSignInsInWindow returns the number of
// distinct user IDs that completed an interactive sign-in (password /
// one-time-code / identity-provider login, or registration) in the
// half-open window [startUnixSec, endUnixSec), excluding any user_ids
// the caller passes in excludeUserIDs. Pass the new-signup user IDs
// there so a brand-new account's registration token does not also count
// as a "returning" sign-in. Rows minted by token rotation (or rows
// predating origin stamping) are never counted — that distinction is
// the whole point of RefreshTokenOrigin (#2665).
func (s *ProtoSQLStorage) CountDistinctInteractiveSignInsInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64, excludeUserIDs []string,
) (int, error) {
	if endUnixSec <= startUnixSec {
		return 0, fmt.Errorf("CountDistinctInteractiveSignInsInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}

	args := []any{startUnixSec, endUnixSec, int(models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)}
	excludeClause := ""
	if len(excludeUserIDs) > 0 {
		placeholders := make([]string, 0, len(excludeUserIDs))
		for i, id := range excludeUserIDs {
			placeholders = append(placeholders, s.dbSpec.Placeholder(i+4))
			args = append(args, id)
		}
		excludeClause = " AND user_id NOT IN (" + strings.Join(placeholders, ", ") + ")"
	}
	query := fmt.Sprintf(`
		SELECT COUNT(DISTINCT user_id)
		FROM "refresh_token"
		WHERE created_at_unix_sec >= %s
		  AND created_at_unix_sec < %s
		  AND origin = %s%s
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(3), excludeClause)

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountDistinctInteractiveSignInsInWindow: %w", err)
	}
	return count, nil
}

// maxDistinctChatSenders bounds FindDistinctChatSenderIDsInWindow — a
// safety LIMIT far above any plausible distinct-sender population in a
// digest window, not an expected truncation point.
const maxDistinctChatSenders = 100_000

// FindDistinctChatSenderIDsInWindow returns the distinct user IDs that
// sent a chat message in the half-open window [startUnixSec,
// endUnixSec). System messages and soft-deleted rows are excluded. The
// digest unions these with community-event actors for its
// active-contributor count.
func (s *ProtoSQLStorage) FindDistinctChatSenderIDsInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]string, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("FindDistinctChatSenderIDsInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT DISTINCT user_message_sender_id
		FROM "chat_message"
		WHERE sent_at_unix_sec >= %s
		  AND sent_at_unix_sec < %s
		  AND user_message_sender_id IS NOT NULL
		  AND user_message_sender_id <> ''
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		LIMIT %s
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(3))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec, maxDistinctChatSenders)
	if err != nil {
		return nil, fmt.Errorf("FindDistinctChatSenderIDsInWindow: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("FindDistinctChatSenderIDsInWindow scan: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindDistinctChatSenderIDsInWindow iterate: %w", err)
	}
	return out, nil
}

// CountUsersCreatedInWindow returns the number of non-deleted users
// created in the half-open window [startUnixSec, endUnixSec). The slim
// counterpart of FindUsersCreatedInWindow for prior-window deltas that
// never render the rows.
func (s *ProtoSQLStorage) CountUsersCreatedInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) (int, error) {
	if endUnixSec <= startUnixSec {
		return 0, fmt.Errorf("CountUsersCreatedInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM "user"
		WHERE created_at >= %s
		  AND created_at < %s
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	var count int
	if err := s.db.QueryRowContext(ctx, query, startUnixSec, endUnixSec).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountUsersCreatedInWindow: %w", err)
	}
	return count, nil
}

// CountMembersByCommunity returns the current active-member count for
// each of the given community IDs in one GROUP BY query. Soft-deleted
// membership rows (members who left) are excluded. Communities with no
// active members are absent from the map.
func (s *ProtoSQLStorage) CountMembersByCommunity(
	ctx context.Context, communityIDs []string,
) (map[string]int, error) {
	out := map[string]int{}
	if len(communityIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(communityIDs))
	args := make([]any, 0, len(communityIDs))
	for i, id := range communityIDs {
		placeholders = append(placeholders, s.dbSpec.Placeholder(i+1))
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT community_id, COUNT(*)
		FROM "community_user"
		WHERE community_id IN (%s)
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		GROUP BY community_id
	`, strings.Join(placeholders, ", "))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("CountMembersByCommunity: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("CountMembersByCommunity scan: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("CountMembersByCommunity iterate: %w", err)
	}
	return out, nil
}

// FindEarliestCommunityForUsers returns, per user ID, the community_id
// of that user's earliest active membership — the "primary" community a
// digest reader most wants to see next to a new signup. Users with no
// active membership are absent from the map.
func (s *ProtoSQLStorage) FindEarliestCommunityForUsers(
	ctx context.Context, userIDs []string,
) (map[string]string, error) {
	out := map[string]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(userIDs))
	args := make([]any, 0, len(userIDs))
	for i, id := range userIDs {
		placeholders = append(placeholders, s.dbSpec.Placeholder(i+1))
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT DISTINCT ON (user_id) user_id, community_id
		FROM "community_user"
		WHERE user_id IN (%s)
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		ORDER BY user_id, created_at_unix_sec ASC, id ASC
	`, strings.Join(placeholders, ", "))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("FindEarliestCommunityForUsers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, communityID string
		if err := rows.Scan(&userID, &communityID); err != nil {
			return nil, fmt.Errorf("FindEarliestCommunityForUsers scan: %w", err)
		}
		out[userID] = communityID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindEarliestCommunityForUsers iterate: %w", err)
	}
	return out, nil
}

// FindWaitlistSignupsInWindow returns WaitlistEntry rows whose
// created_at_unix_sec falls in the half-open window
// [startUnixSec, endUnixSec).
func (s *ProtoSQLStorage) FindWaitlistSignupsInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) ([]*models.WaitlistEntry, error) {
	if endUnixSec <= startUnixSec {
		return nil, fmt.Errorf("FindWaitlistSignupsInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "waitlist_entry"
		WHERE created_at_unix_sec >= %s
		  AND created_at_unix_sec < %s
		ORDER BY created_at_unix_sec ASC, id ASC
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	rows, err := s.db.QueryContext(ctx, query, startUnixSec, endUnixSec)
	if err != nil {
		return nil, fmt.Errorf("FindWaitlistSignupsInWindow: %w", err)
	}
	defer rows.Close()

	var out []*models.WaitlistEntry
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("FindWaitlistSignupsInWindow scan: %w", err)
		}
		e := &models.WaitlistEntry{}
		if err := proto.Unmarshal(blob, e); err != nil {
			return nil, fmt.Errorf("FindWaitlistSignupsInWindow unmarshal: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FindWaitlistSignupsInWindow iterate: %w", err)
	}
	return out, nil
}

// CountPasswordResetsInWindow returns the number of PendingPasswordReset
// rows created in the half-open window [startUnixSec, endUnixSec).
func (s *ProtoSQLStorage) CountPasswordResetsInWindow(
	ctx context.Context, startUnixSec, endUnixSec int64,
) (int, error) {
	if endUnixSec <= startUnixSec {
		return 0, fmt.Errorf("CountPasswordResetsInWindow: end (%d) must exceed start (%d)", endUnixSec, startUnixSec)
	}
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM "pending_password_reset"
		WHERE created_at_unix_sec >= %s
		  AND created_at_unix_sec < %s
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2))
	var count int
	if err := s.db.QueryRowContext(ctx, query, startUnixSec, endUnixSec).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountPasswordResetsInWindow: %w", err)
	}
	return count, nil
}

// ClaimActivityDigestSend attempts to claim today's send for the given
// cadence (daily / weekly) and runKey. The runKey is a monotonically
// increasing string ("YYYY-MM-DD" for daily, "YYYY-Www" for weekly).
// Returns (true, nil) when the caller owns the send, (false, nil) when
// another instance got there first, and (false, err) on database
// errors. The conditional UPDATE guarantees at-most-once delivery
// across concurrent server instances.
func (s *ProtoSQLStorage) ClaimActivityDigestSend(
	ctx context.Context, cadence ActivityDigestCadence, runKey string,
) (bool, error) {
	if runKey == "" {
		return false, errors.New("ClaimActivityDigestSend: runKey must not be empty")
	}
	if cadence == "" {
		return false, errors.New("ClaimActivityDigestSend: cadence must not be empty")
	}
	query := fmt.Sprintf(`
		UPDATE activity_digest_state
		SET last_sent_date = %s
		WHERE id = %s
		  AND last_sent_date < %s
	`, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2), s.dbSpec.Placeholder(3))
	res, err := s.exec.ExecContext(ctx, query, runKey, string(cadence), runKey)
	if err != nil {
		return false, fmt.Errorf("ClaimActivityDigestSend update: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ClaimActivityDigestSend rows affected: %w", err)
	}
	return rows == 1, nil
}

// GetActivityDigestLastSentKey returns the last successfully claimed
// runKey for the given cadence, or empty string when no digest has
// shipped yet. Exposed so tests can assert on state without poking
// the table.
func (s *ProtoSQLStorage) GetActivityDigestLastSentKey(ctx context.Context, cadence ActivityDigestCadence) (string, error) {
	var last string
	err := s.db.QueryRowContext(ctx,
		`SELECT last_sent_date FROM activity_digest_state WHERE id = $1`,
		string(cadence),
	).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetActivityDigestLastSentKey: %w", err)
	}
	return last, nil
}
