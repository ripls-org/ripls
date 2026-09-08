package portfolio

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// WatchItem manually adds an item to the authenticated user's watch list.
// If the item was previously dismissed, it is reactivated.
func (s *Service) WatchItem(
	ctx context.Context,
	req *connect.Request[api.WatchItemRequest],
) (*connect.Response[api.WatchItemResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "WatchItem",
		"user_id", authInfo.UserID,
		"item_type", req.Msg.ItemType.String(),
		"item_id", req.Msg.ItemId,
	)

	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	watchType := dailyItemTypeToWatchType(req.Msg.ItemType)
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported item type: %s", req.Msg.ItemType))
	}

	if err := s.watchStorage.UpsertWatch(ctx, authInfo.UserID, watchType, req.Msg.ItemId); err != nil {
		logger.ErrorContext(ctx, "failed to watch item", "error", err)
		return nil, connecterr.Internal(ctx, "WatchItem", err)
	}

	logger.DebugContext(ctx, "item watched")
	return connect.NewResponse(&api.WatchItemResponse{}), nil
}

// DismissInboxItem soft-deletes the watch entry for the authenticated user,
// removing the item from their inbox. The item reappears if new activity occurs.
func (s *Service) DismissInboxItem(
	ctx context.Context,
	req *connect.Request[api.DismissInboxItemRequest],
) (*connect.Response[api.DismissInboxItemResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DismissInboxItem",
		"user_id", authInfo.UserID,
		"item_type", req.Msg.ItemType.String(),
		"item_id", req.Msg.ItemId,
	)

	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	watchType := dailyItemTypeToWatchType(req.Msg.ItemType)
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported item type: %s", req.Msg.ItemType))
	}

	// Decisions ("Not now" on a Needs-you card, #2435) have no prior watch
	// entry — DismissWatch no-ops without a row, so create one first. The
	// entry exists solely to carry the dismissed state.
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_DECISION {
		if err := s.watchStorage.UpsertWatch(ctx, authInfo.UserID, watchType, req.Msg.ItemId); err != nil {
			logger.ErrorContext(ctx, "failed to create decision watch entry for dismissal", "error", err)
			return nil, connecterr.Internal(ctx, "DismissInboxItem", err)
		}
	}

	if err := s.watchStorage.DismissWatch(ctx, authInfo.UserID, watchType, req.Msg.ItemId); err != nil {
		logger.ErrorContext(ctx, "failed to dismiss inbox item", "error", err)
		return nil, connecterr.Internal(ctx, "DismissInboxItem", err)
	}

	logger.DebugContext(ctx, "inbox item dismissed")
	return connect.NewResponse(&api.DismissInboxItemResponse{}), nil
}

// IsWatched returns whether the authenticated user has an active watch entry
// for the specified item.
func (s *Service) IsWatched(
	ctx context.Context,
	req *connect.Request[api.IsWatchedRequest],
) (*connect.Response[api.IsWatchedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	watchType := dailyItemTypeToWatchType(req.Msg.ItemType)
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported item type: %s", req.Msg.ItemType))
	}

	watched, err := s.watchStorage.IsWatched(ctx, authInfo.UserID, watchType, req.Msg.ItemId)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to check watch state",
			"operation", "IsWatched",
			"user_id", authInfo.UserID,
			"item_id", req.Msg.ItemId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "IsWatched", err)
	}

	return connect.NewResponse(&api.IsWatchedResponse{IsWatched: watched}), nil
}

// MarkInboxItemRead marks a watched item as read for the authenticated user.
func (s *Service) MarkInboxItemRead(
	ctx context.Context,
	req *connect.Request[api.MarkInboxItemReadRequest],
) (*connect.Response[api.MarkInboxItemReadResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "MarkInboxItemRead",
		"user_id", authInfo.UserID,
		"item_type", req.Msg.ItemType.String(),
		"item_id", req.Msg.ItemId,
	)

	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	watchType := dailyItemTypeToWatchType(req.Msg.ItemType)
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported item type: %s", req.Msg.ItemType))
	}

	// Decisions ("Say thanks" on a Needs-you card, #2435) have no prior
	// watch entry — MarkRead no-ops without a row, so create one first. A
	// read DECISION watch is the durable "resolved" marker that keeps the
	// decision out of the queue for good.
	if watchType == models.WatchedItemType_WATCHED_ITEM_TYPE_DECISION {
		if err := s.watchStorage.UpsertWatch(ctx, authInfo.UserID, watchType, req.Msg.ItemId); err != nil {
			logger.ErrorContext(ctx, "failed to create decision watch entry before mark-read", "error", err)
			return nil, connecterr.Internal(ctx, "MarkInboxItemRead", err)
		}
	}

	if err := s.watchStorage.MarkRead(ctx, authInfo.UserID, watchType, req.Msg.ItemId); err != nil {
		logger.ErrorContext(ctx, "failed to mark inbox item read", "error", err)
		return nil, connecterr.Internal(ctx, "MarkInboxItemRead", err)
	}

	logger.DebugContext(ctx, "marked inbox item read")
	return connect.NewResponse(&api.MarkInboxItemReadResponse{}), nil
}

// dailyItemTypeToWatchType converts an API DailyItemType to a storage WatchedItemType.
func dailyItemTypeToWatchType(t api.DailyItemType) models.WatchedItemType {
	switch t {
	case api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER, api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR
	case api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE
	case api.DailyItemType_DAILY_ITEM_TYPE_REQUEST:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST
	case api.DailyItemType_DAILY_ITEM_TYPE_DECISION:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_DECISION
	default:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED
	}
}
