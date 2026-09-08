package storage

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// InitializeChatConversationArrayColumns idempotently runs the DDL for
// every denormalized TEXT[] column on the ChatConversation type —
// currently just participant_ids, which powers
// ChatConversationStorage.ListByParticipant (the inbox query).
// One-time-effective per database.
//
// DDL only — does NOT register the per-write sync. Pair every startup
// call with [ProtoSQLStorage.RegisterChatConversationArrayColumns]
// (which must run every process start regardless of schema state).
func (s *ProtoSQLStorage) InitializeChatConversationArrayColumns(ctx context.Context) error {
	if !s.isTypeRegistered(&models.ChatConversation{}) {
		return nil
	}
	extractor := TopLevelRepeatedString(&models.ChatConversation{}, "participant_ids")
	if err := s.InitializeArrayColumn(ctx, &models.ChatConversation{}, "participant_ids", extractor); err != nil {
		return fmt.Errorf("initialize chat_conversation.participant_ids: %w", err)
	}
	return nil
}

// RegisterChatConversationArrayColumns registers the per-write
// auto-sync for every ChatConversation TEXT[] denormalization column.
// Required on every server start: without it, Insert / Update of a
// ChatConversation leaves participant_ids stale and ListByParticipant
// (the inbox query) silently misses rows.
//
// Safe no-op when ChatConversation is not registered for storage.
func (s *ProtoSQLStorage) RegisterChatConversationArrayColumns() {
	if !s.isTypeRegistered(&models.ChatConversation{}) {
		return
	}
	s.RegisterArrayColumn(&models.ChatConversation{}, "participant_ids", TopLevelRepeatedString(&models.ChatConversation{}, "participant_ids"))
}
