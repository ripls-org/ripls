package storage

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// Hard-delete cascade helpers used by the daily community purge
// job (#1620). Each helper physically removes rows scoped to a
// community ID and returns the row count. Unlike the soft-delete
// counterparts in cascade_delete.go, these calls bypass the
// DeletedMetadata convention — once a row is hard-deleted it is
// irrecoverable.
//
// Helpers are designed to be reusable for #517 (GDPR account
// deletion); the user-deletion path will call a parallel set
// scoped by user ID.
//
// Order-of-operations note: the purge job calls these in
// child-before-parent order so that, even without a wrapping
// transaction (the storage layer does not yet expose one), a
// mid-cascade failure leaves orphan join rows pointing at a
// still-present community parent — recoverable on the next
// daily tick rather than requiring manual cleanup.

// HardDeleteCommunityUserByCommunityID physically removes every
// community_user row scoped to communityID. Returns the number
// of rows deleted.
func HardDeleteCommunityUserByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_user", communityID)
}

// HardDeleteCommunityGearByCommunityID physically removes every
// community_gear row scoped to communityID.
func HardDeleteCommunityGearByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_gear", communityID)
}

// HardDeleteCommunityRequestByCommunityID physically removes
// every community_request row scoped to communityID.
func HardDeleteCommunityRequestByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_request", communityID)
}

// HardDeleteCommunityExperienceByCommunityID physically removes
// every community_experience row scoped to communityID.
func HardDeleteCommunityExperienceByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_experience", communityID)
}

// HardDeleteCommunityNotificationPreferencesByCommunityID
// physically removes every community_notification_preferences row
// scoped to communityID.
func HardDeleteCommunityNotificationPreferencesByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_notification_preferences", communityID)
}

// HardDeleteCommunityInvitationLinkByCommunityID physically
// removes every community_invitation_link row scoped to communityID.
func HardDeleteCommunityInvitationLinkByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_invitation_link", communityID)
}

// HardDeleteCommunityEventByCommunityID physically removes every
// community_event row scoped to communityID.
func HardDeleteCommunityEventByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_event", communityID)
}

// HardDeleteCommunityRegionByCommunityID physically removes every
// community_region row scoped to communityID.
func HardDeleteCommunityRegionByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "community_region", communityID)
}

// HardDeleteStoriesByCommunityID physically removes every Story
// row scoped to communityID.
func HardDeleteStoriesByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "Story", communityID)
}

// HardDeleteStoredNudgesByCommunityID physically removes every
// stored_nudge row scoped to communityID.
func HardDeleteStoredNudgesByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "stored_nudge", communityID)
}

// HardDeleteFeedItemViewsByCommunityID physically removes every
// FeedItemView row scoped to communityID.
func HardDeleteFeedItemViewsByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "FeedItemView", communityID)
}

// HardDeleteChatMessagesByCommunityID physically removes every
// chat_message row whose conversation belongs to communityID.
// ChatMessage has no community_id column, so this helper first
// queries chat_conversation for the matching conversation IDs
// and then bulk-deletes by conversation_id IN (...).
//
// Caller is expected to invoke
// HardDeleteChatConversationsByCommunityID *after* this helper
// (child-before-parent order).
func HardDeleteChatMessagesByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HardDeleteChatMessagesByCommunityID",
		"community_id", communityID,
	)

	conversations, err := storage.QueryByField(ctx, "community_id", communityID, &models.ChatConversation{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query conversations", "error", err)
		return 0, fmt.Errorf("query conversations for community %s: %w", communityID, err)
	}
	if len(conversations) == 0 {
		return 0, nil
	}

	convIDs := make([]string, 0, len(conversations))
	for _, c := range conversations {
		convIDs = append(convIDs, c.(*models.ChatConversation).Id)
	}

	rows, err := storage.DeleteByFieldIn(ctx, "chat_message", "conversation_id", convIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to hard-delete chat messages", "error", err, "conversation_count", len(convIDs))
		return 0, fmt.Errorf("hard-delete chat_message for community %s: %w", communityID, err)
	}
	return rows, nil
}

// HardDeleteChatConversationsByCommunityID physically removes
// every chat_conversation row scoped to communityID.
//
// HardDeleteChatMessagesByCommunityID must run *before* this
// helper, otherwise the messages will dangle.
func HardDeleteChatConversationsByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByCommunityID(ctx, storage, "chat_conversation", communityID)
}

// HardDeleteCommunityByID physically removes the single community
// row identified by communityID. This is the last step of the
// cascade — the parent row goes after every child has been
// removed.
func HardDeleteCommunityByID(
	ctx context.Context, storage *ProtoSQLStorage, communityID string,
) (int64, error) {
	return hardDeleteByField(ctx, storage, "community", "id", communityID)
}

// hardDeleteByCommunityID is the common DELETE-WHERE-community_id
// shape shared by most cascade helpers. Returns rowsAffected.
func hardDeleteByCommunityID(
	ctx context.Context, storage *ProtoSQLStorage, table, communityID string,
) (int64, error) {
	return hardDeleteByField(ctx, storage, table, "community_id", communityID)
}

// hardDeleteByField is the lowest-level helper, separated so
// HardDeleteCommunityByID (which keys on `id`, not `community_id`)
// can share the same logging shape.
func hardDeleteByField(
	ctx context.Context, storage *ProtoSQLStorage, table, field, value string,
) (int64, error) {
	rows, err := storage.DeleteByField(ctx, table, field, value)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "hard-delete failed",
			"operation", "hardDeleteByField",
			"table", table,
			"field", field,
			"value", value,
			"error", err,
		)
		return 0, fmt.Errorf("hard-delete %s by %s=%s: %w", table, field, value, err)
	}
	return rows, nil
}
