package storage

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

const defaultChatMessageLimit = 200

// ListByConversation returns ChatMessage rows for a conversation ordered by
// sent_at_unix_sec (ascending when asc=true, descending when asc=false), with
// id ASC as a deterministic tiebreaker. Soft-deleted rows are excluded.
// If limit <= 0 a default cap of 200 is applied per CLAUDE.md "always LIMIT
// unbounded queries".
func ListByConversation(ctx context.Context, store *ProtoSQLStorage, conversationID string, asc bool, limit int) ([]*models.ChatMessage, error) {
	if limit <= 0 || limit > defaultChatMessageLimit {
		limit = defaultChatMessageLimit
	}

	direction := "ASC"
	if !asc {
		direction = "DESC"
	}

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "chat_message"
		WHERE conversation_id = %s
		  AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)
		ORDER BY sent_at_unix_sec %s, id ASC
		LIMIT %s
	`, store.dbSpec.Placeholder(1), direction, store.dbSpec.Placeholder(2))

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListByConversation",
		"conversation_id", conversationID,
		"asc", asc,
		"limit", limit,
	)
	logger.DebugContext(ctx, "querying chat messages")

	rows, err := store.db.QueryContext(ctx, query, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query chat messages for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	var messages []*models.ChatMessage
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan chat message: %w", err)
		}

		msg := &models.ChatMessage{}
		if err := proto.Unmarshal(binaryProto, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal chat message: %w", err)
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chat messages: %w", err)
	}

	return messages, nil
}
