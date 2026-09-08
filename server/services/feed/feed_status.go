package feed

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GetFeedStatus returns whether each of the user's communities has unseen feed items.
// It is a lightweight alternative to GetFeed that avoids generating full feed payloads.
func (s *Service) GetFeedStatus(
	ctx context.Context,
	req *connect.Request[api.GetFeedStatusRequest],
) (*connect.Response[api.GetFeedStatusResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "GetFeedStatus",
	)
	logger.DebugContext(ctx, "checking feed status for user communities")

	// Find all communities the user belongs to (1 query).
	memberships, err := storage.QueryByField[*models.CommunityUser](s.sqlStorage, ctx, "user_id", authInfo.UserID)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetFeedStatus", err, "detail", "failed to list memberships")
	}

	communityIDs := make([]string, 0, len(memberships))
	for _, m := range memberships {
		communityIDs = append(communityIDs, m.CommunityId)
	}

	if len(communityIDs) == 0 {
		return connect.NewResponse(&api.GetFeedStatusResponse{
			CommunityIdToHasNew: make(map[string]bool),
		}), nil
	}

	// Filter out soft-deleted communities. GetByIDs excludes them by default,
	// so the returned map contains only active communities.
	activeCommunityMap, err := storage.GetByIDs[*models.Community](s.sqlStorage, ctx, communityIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetFeedStatus", err, "detail", "failed to fetch communities")
	}
	if len(activeCommunityMap) < len(communityIDs) {
		filtered := make([]string, 0, len(activeCommunityMap))
		for _, cid := range communityIDs {
			if _, ok := activeCommunityMap[cid]; ok {
				filtered = append(filtered, cid)
			}
		}
		communityIDs = filtered
	}

	now := time.Now().Unix()

	// For each community, check if any feed items have activity newer than
	// the user's last_seen_at. We check community events as a proxy for
	// feed item activity.
	result := make(map[string]bool, len(communityIDs))
	newCount := 0
	for _, communityID := range communityIDs {
		hasNew, checkErr := s.communityHasNewFeedItems(ctx, authInfo.UserID, communityID, now)
		if checkErr != nil {
			logger.WarnContext(ctx, "failed to check feed status for community",
				"community_id", communityID, "error", checkErr)
			result[communityID] = false
			continue
		}
		result[communityID] = hasNew
		if hasNew {
			newCount++
		}
	}

	logger.DebugContext(ctx, "feed status computed",
		"communities_checked", len(communityIDs),
		"communities_with_new", newCount,
	)

	return connect.NewResponse(&api.GetFeedStatusResponse{
		CommunityIdToHasNew: result,
	}), nil
}

// communityHasNewFeedItems reports whether the community has any visible feed
// items with activity the user has not yet seen.
//
// The check mirrors GetFeed's deduplication logic: only the most-recent event
// per item key is considered, matching exactly which event IDs MarkFeedItemsViewed
// is called with as the user scrolls.  Checking older superseded events
// (e.g. the original COMMUNITY_CREATED event when newer activity exists) causes
// a permanently-stuck "New" badge because the user can never view a card that
// GetFeed no longer shows.
func (s *Service) communityHasNewFeedItems(ctx context.Context, userID, communityID string, now int64) (bool, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"community_id", communityID,
		"user_id", userID,
		"operation", "communityHasNewFeedItems",
	)

	// Get all events for this community.
	events, err := storage.QueryByField[*models.CommunityEvent](s.sqlStorage, ctx, "community_id", communityID)
	if err != nil {
		return false, fmt.Errorf("query events: %w", err)
	}
	if len(events) == 0 {
		logger.DebugContext(ctx, "no events found for community")
		return false, nil
	}

	// Apply the same deduplication as GetFeed: only the most-recent event per
	// item key is checked, since that is the only event the user can ever view.
	deduped := deduplicateEventsByItemKey(events)

	logger.DebugContext(ctx, "filtered events for freshness check",
		"total_events", len(events),
		"visible_items", len(deduped),
	)

	if len(deduped) == 0 {
		return false, nil
	}

	// Collect champion IDs to fetch view records, and extract entity IDs to
	// resolve conversation activity (chat messages count as new activity).
	championIDs := make([]string, 0, len(deduped))
	gearIDs := make([]string, 0)
	requestIDs := make([]string, 0)
	experienceIDs := make([]string, 0)
	for _, g := range deduped {
		e := g.latestEvent
		championIDs = append(championIDs, e.Id)
		if e.GearId != "" {
			gearIDs = append(gearIDs, e.GearId)
		} else if rid := e.GetRequestId(); rid != "" {
			requestIDs = append(requestIDs, rid)
		} else if xid := e.GetExperienceId(); xid != "" {
			experienceIDs = append(experienceIDs, xid)
		}
	}

	viewRecords, err := s.feedStorage.GetFeedItemViews(ctx, userID, communityID, championIDs)
	if err != nil {
		return false, fmt.Errorf("get view records: %w", err)
	}

	// Resolve conversation IDs for gear, requests, and experiences so we can
	// check whether a chat message was posted after the user last viewed the item.
	gearConvIDMap := make(map[string]string) // gear_id -> conversation_id
	if len(gearIDs) > 0 {
		// conversation_id is now on the Gear model; batch fetch gears.
		gearModelMap, gErr := storage.GetByIDs[*models.Gear](s.sqlStorage, ctx, gearIDs)
		if gErr != nil {
			logger.WarnContext(ctx, "failed to fetch gears for badge check", "error", gErr)
		} else {
			for _, g := range gearModelMap {
				if g.ConversationId != "" {
					gearConvIDMap[g.Id] = g.ConversationId
				}
			}
		}
		// Fall back to ChatConversation by gear_id for any unresolved gear.
		unresolvedGearIDs := make([]string, 0)
		for _, gID := range gearIDs {
			if _, ok := gearConvIDMap[gID]; !ok {
				unresolvedGearIDs = append(unresolvedGearIDs, gID)
			}
		}
		if len(unresolvedGearIDs) > 0 {
			gearConvs, gcErr := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_gear_id", unresolvedGearIDs)
			if gcErr != nil {
				logger.WarnContext(ctx, "failed to fetch gear conversations by gear_id for badge check", "error", gcErr)
			} else {
				for _, conv := range gearConvs {
					if gID := conv.GetTopic().GetGearId(); gID != "" {
						gearConvIDMap[gID] = conv.Id
					}
				}
			}
		}
	}

	reqConvIDMap := make(map[string]string) // request_id -> conversation_id
	if len(requestIDs) > 0 {
		convs, rcErr := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_request_id", requestIDs)
		if rcErr != nil {
			logger.WarnContext(ctx, "failed to fetch request conversations for badge check", "error", rcErr)
		} else {
			for _, conv := range convs {
				reqConvIDMap[conv.GetTopic().GetRequestId()] = conv.Id
			}
		}
	}

	expConvIDMap := make(map[string]string) // experience_id -> conversation_id
	if len(experienceIDs) > 0 {
		convs, ecErr := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_experience_id", experienceIDs)
		if ecErr != nil {
			logger.WarnContext(ctx, "failed to fetch experience conversations for badge check", "error", ecErr)
		} else {
			for _, conv := range convs {
				expConvIDMap[conv.GetTopic().GetExperienceId()] = conv.Id
			}
		}
	}

	// Batch-fetch latest non-viewer message timestamps per conversation.
	allConvIDs := make([]string, 0, len(gearConvIDMap)+len(reqConvIDMap)+len(expConvIDMap))
	for _, cid := range gearConvIDMap {
		allConvIDs = append(allConvIDs, cid)
	}
	for _, cid := range reqConvIDMap {
		allConvIDs = append(allConvIDs, cid)
	}
	for _, cid := range expConvIDMap {
		allConvIDs = append(allConvIDs, cid)
	}
	latestMsgAt := make(map[string]int64)
	if len(allConvIDs) > 0 {
		var msgErr error
		latestMsgAt, msgErr = s.feedStorage.GetLatestNonViewerMessageAtBatch(ctx, allConvIDs, userID)
		if msgErr != nil {
			logger.WarnContext(ctx, "failed to fetch message timestamps for badge check", "error", msgErr)
		}
	}

	// convActivityAt returns the latest chat message timestamp for an event's conversation.
	convActivityAt := func(event *models.CommunityEvent) int64 {
		var convID string
		switch {
		case event.GearId != "":
			convID = gearConvIDMap[event.GearId]
		default:
			if rid := event.GetRequestId(); rid != "" {
				convID = reqConvIDMap[rid]
			} else if xid := event.GetExperienceId(); xid != "" {
				convID = expConvIDMap[xid]
			}
		}
		return latestMsgAt[convID]
	}

	for _, g := range deduped {
		event := g.latestEvent
		msgAt := convActivityAt(event)
		lastActivityAt := event.OccurredAtUnixSec
		if msgAt > lastActivityAt {
			lastActivityAt = msgAt
		}

		view := viewRecords[event.Id]

		if isItemUnread(view, lastActivityAt, now) {
			logger.DebugContext(ctx, "unread item found",
				"event_id", event.Id,
				"event_type", event.EventType.String(),
				"last_activity_at_unix_sec", lastActivityAt,
			)
			return true, nil
		}
	}

	logger.DebugContext(ctx, "all visible items are seen", "community_id", communityID)
	return false, nil
}
