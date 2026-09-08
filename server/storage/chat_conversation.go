package storage

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ChatConversationStorage provides chat-conversation-specific database
// operations — primarily ListByParticipant, which uses the indexed
// participant_ids TEXT[] denormalization column. The column is kept in
// sync with conversation.ParticipantIds inside the proto write
// transaction (see [ProtoSQLStorage.RegisterArrayColumn] and
// InitializeChatConversationArrayColumns); binary_proto remains
// authoritative.
type ChatConversationStorage struct {
	storage *ProtoSQLStorage
}

// NewChatConversationStorage creates a new ChatConversationStorage instance.
func NewChatConversationStorage(storage *ProtoSQLStorage) *ChatConversationStorage {
	return &ChatConversationStorage{storage: storage}
}

// Insert creates a new chat conversation. The participant_ids TEXT[]
// denormalization column is auto-synced from conversation.ParticipantIds
// inside the same transaction as the proto write.
func (cs *ChatConversationStorage) Insert(ctx context.Context, conversation *models.ChatConversation) (string, error) {
	return cs.storage.Insert(ctx, conversation)
}

// Update persists changes to an existing chat conversation. The
// participant_ids TEXT[] denormalization column is auto-synced from
// conversation.ParticipantIds inside the same transaction.
func (cs *ChatConversationStorage) Update(ctx context.Context, conversation *models.ChatConversation) error {
	return cs.storage.Update(ctx, conversation)
}

// GetByID retrieves a chat conversation by its ID.
func (cs *ChatConversationStorage) GetByID(ctx context.Context, id string) (*models.ChatConversation, error) {
	conversation := &models.ChatConversation{}
	if err := cs.storage.GetByID(ctx, id, conversation); err != nil {
		return nil, fmt.Errorf("failed to get chat conversation: %w", err)
	}
	return conversation, nil
}

// maxChatConversationLimit caps ListByParticipant. It is both the default (for
// a caller that passes no limit) and the ceiling (for one that passes an
// absurd one), satisfying docs/server/conventions.md § SQL Efficiency rule 3.
const maxChatConversationLimit = 200

// ListByParticipant returns all non-deleted chat conversations where userID is a participant,
// ordered by last_message_at_unix_sec descending.
// A limit <= 0 or above maxChatConversationLimit is clamped to
// maxChatConversationLimit.
func (cs *ChatConversationStorage) ListByParticipant(ctx context.Context, userID string, limit int) ([]*models.ChatConversation, error) {
	if limit <= 0 || limit > maxChatConversationLimit {
		limit = maxChatConversationLimit
	}

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "chat_conversation"
		WHERE %s = ANY(participant_ids)
		AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0
		ORDER BY last_message_at_unix_sec DESC
		LIMIT %s
	`, cs.storage.dbSpec.Placeholder(1), cs.storage.dbSpec.Placeholder(2))

	rows, err := cs.storage.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query chat conversations by participant: %w", err)
	}
	defer rows.Close()

	var conversations []*models.ChatConversation
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan chat conversation: %w", err)
		}

		conversation := &models.ChatConversation{}
		if err := proto.Unmarshal(binaryProto, conversation); err != nil {
			return nil, fmt.Errorf("failed to unmarshal chat conversation: %w", err)
		}

		conversations = append(conversations, conversation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chat conversations: %w", err)
	}

	return conversations, nil
}
