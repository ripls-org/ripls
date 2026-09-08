package conversation

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// IsItemDone checks if the associated item (transfer or request) is in a terminal state.
// Used by both chat and portfolio services to exclude completed/cancelled items.
func IsItemDone(ctx context.Context, store *storage.ProtoSQLStorage, conv *models.ChatConversation) bool {
	logger := logging.LoggerWithContext(ctx).With("conversation_id", conv.Id)

	switch topic := conv.GetTopic().GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		transfer := &models.Transfer{}
		if err := store.GetByID(ctx, topic.TransferId, transfer); err != nil {
			logger.Error("failed to get transfer for archive check", "transfer_id", topic.TransferId, "error", err)
			return false
		}
		return transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED ||
			transfer.State == models.TransferState_TRANSFER_STATE_CANCELLED
	case *models.ConversationTopic_RequestId:
		request := &models.Request{}
		if err := store.GetByID(ctx, topic.RequestId, request); err != nil {
			logger.Error("failed to get request for archive check", "request_id", topic.RequestId, "error", err)
			return false
		}
		return request.State == models.RequestState_REQUEST_STATE_FULFILLED ||
			request.State == models.RequestState_REQUEST_STATE_CANCELLED
	default:
		return false
	}
}

// IsItemDoneFromMaps checks terminal state using pre-fetched transfer and request maps,
// avoiding a per-conversation database query. Both maps are keyed by their respective IDs
// and are obtained via storage.GetByIDs before iterating over conversations.
func IsItemDoneFromMaps(
	conv *models.ChatConversation,
	transfers map[string]proto.Message,
	requests map[string]proto.Message,
) bool {
	switch topic := conv.GetTopic().GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		msg, ok := transfers[topic.TransferId]
		if !ok {
			return false
		}
		transfer := msg.(*models.Transfer)
		return transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED ||
			transfer.State == models.TransferState_TRANSFER_STATE_CANCELLED
	case *models.ConversationTopic_RequestId:
		msg, ok := requests[topic.RequestId]
		if !ok {
			return false
		}
		request := msg.(*models.Request)
		return request.State == models.RequestState_REQUEST_STATE_FULFILLED ||
			request.State == models.RequestState_REQUEST_STATE_CANCELLED
	default:
		return false
	}
}
