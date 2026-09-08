package feed

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the FeedService RPC interface.
type Service struct {
	sqlStorage           *storage.ProtoSQLStorage
	feedStorage          *storage.FeedStorage
	storyStorage         *storage.StoryStorage
	aiProvider           ai.Provider
	stockImageryProvider mediapkg.StockImageryProvider
	bucketStorage        storage.BucketStorage
}

// New creates a new feed service.
func New(sqlStorage *storage.ProtoSQLStorage) *Service {
	return &Service{
		sqlStorage:   sqlStorage,
		feedStorage:  storage.NewFeedStorage(sqlStorage),
		storyStorage: storage.NewStoryStorage(sqlStorage),
	}
}

// SetAIProvider sets the AI provider for nudge content generation.
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// Ensure Service implements FeedServiceHandler.
var _ apiconnect.FeedServiceHandler = (*Service)(nil)

// resolveCommunityIDs returns the list of community IDs to query.
func resolveCommunityIDs(req *api.GetFeedRequest) []string {
	return req.CommunityIds
}

// GetFeed returns a personalized feed for one or more communities.
func (s *Service) GetFeed(
	ctx context.Context,
	req *connect.Request[api.GetFeedRequest],
) (*connect.Response[api.GetFeedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	communityIDs := resolveCommunityIDs(req.Msg)
	if len(communityIDs) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("at least one community_id or community_ids entry is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_ids", strings.Join(communityIDs, ","),
	)

	logger.Info("requesting feed")

	// Single batched JOIN: collapses the per-id round-trip loop down to one
	// query for all communities. Drops communities that are missing,
	// soft-deleted, or that the caller does not belong to — multi-community
	// surfaces degrade gracefully rather than fail the whole request.
	activeCommunityIDs, communities, err := auth.FilterActiveMemberCommunities(ctx, s.sqlStorage, communityIDs, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Set default page size.
	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}

	// Generate feed items across all communities, deduplicating by entity key
	// so an item shared with multiple communities appears exactly once. When
	// an item appears in multiple communities, keep the one with the earliest
	// OccurredAtUnixSec (i.e., the first time it was shared).
	var feedItems []*api.FeedItem
	seenEntityKeys := make(map[string]int) // entity key -> index in feedItems
	for _, cid := range activeCommunityIDs {
		items, genErr := s.generateFeedItems(ctx, authInfo.UserID, cid, communities[cid])
		if genErr != nil {
			logger.Error("failed to generate feed items", "error", genErr, "community_id", cid)
			return nil, connecterr.Internal(ctx, "GetFeed", genErr)
		}
		for _, item := range items {
			entityKey := feedItemEntityKey(item)
			if idx, exists := seenEntityKeys[entityKey]; !exists {
				seenEntityKeys[entityKey] = len(feedItems)
				feedItems = append(feedItems, item)
			} else if item.OccurredAtUnixSec < feedItems[idx].OccurredAtUnixSec {
				// Replace with the earlier occurrence so the item sorts by
				// its original sharing time, not a later cross-post.
				feedItems[idx] = item
			}
		}
	}

	// Sort chronologically by event creation time, newest first. Unread and
	// action-required flags are still populated for client-side badge display
	// but do not affect sort order.
	sort.Slice(feedItems, func(i, j int) bool {
		return feedItems[i].OccurredAtUnixSec > feedItems[j].OccurredAtUnixSec
	})

	// Populate the inline ESM voting block on every concluded-experience
	// story payload so the client can render the voting / voted state
	// without an extra fetch.
	enrichFeedStoriesWithEmbeddedESM(ctx, s.sqlStorage, feedItems, authInfo.UserID)

	// Append the terminator card (first page only). It requires the stock
	// imagery provider, so skip entirely if that is not configured.
	//
	// The terminator is extracted before pagination so it always appears on
	// the first page regardless of page size. appendTerminator always adds it
	// as the very last element.
	var terminator *api.FeedItem
	if req.Msg.PageToken == "" && s.stockImageryProvider != nil && len(activeCommunityIDs) > 0 {
		feedItems = s.appendTerminator(ctx, authInfo.UserID, feedItems, logger)
		if len(feedItems) > 0 {
			last := feedItems[len(feedItems)-1]
			if last.ItemType == api.FeedItemType_FEED_ITEM_TYPE_NUDGE {
				terminator = last
				feedItems = feedItems[:len(feedItems)-1]
			}
		}
	}

	// Apply offset-based pagination using page_token as a string-encoded offset.
	startIdx := 0
	if req.Msg.PageToken != "" {
		if offset, parseErr := strconv.Atoi(req.Msg.PageToken); parseErr == nil && offset > 0 {
			startIdx = offset
		}
	}
	if startIdx > len(feedItems) {
		startIdx = len(feedItems)
	}

	endIdx := startIdx + int(pageSize)
	if endIdx > len(feedItems) {
		endIdx = len(feedItems)
	}

	pagedItems := feedItems[startIdx:endIdx]
	nextPageToken := ""
	if endIdx < len(feedItems) {
		nextPageToken = strconv.Itoa(endIdx)
	}

	// Re-attach the terminator to the first page. It must always be the last item
	// shown so the user sees "you're all caught up" after scrolling through content.
	if startIdx == 0 && terminator != nil {
		pagedItems = append(pagedItems, terminator)
	}

	// A sort-order investigation once dumped the first page to a fixed path in
	// shared /tmp on every request. Removed: it leaked the caller's user ID,
	// community IDs, gear names and request titles to a world-readable file that
	// any local process could also pre-create as a symlink. Use
	// logger.DebugContext for this — structured logging is masked and level-gated,
	// a debug file in an RPC handler is neither.

	return connect.NewResponse(&api.GetFeedResponse{
		Items:         pagedItems,
		NextPageToken: nextPageToken,
	}), nil
}

// generateFeedItems creates feed items from community events and stories.
//
// Three stages: collect the entity IDs off the events, batch-load everything
// the cards need from those IDs, then turn events and stories into items. Only
// the events, the users and the view records are load-bearing enough to fail
// the request — every other fetch degrades (#2816).
func (s *Service) generateFeedItems(ctx context.Context, userID, communityID string, community *models.Community) ([]*api.FeedItem, error) {
	logger := logging.LoggerWithContext(ctx)

	events, err := storage.QueryByField[*models.CommunityEvent](s.sqlStorage, ctx, "community_id", communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query community events: %w", err)
	}
	stories, err := s.storyStorage.ListByCommunity(ctx, communityID, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to query stories: %w", err)
	}

	ids := collectFeedEntityIDs(events, stories)
	fc, err := s.loadFeedLookups(ctx, logger, userID, communityID, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to batch fetch users for feed: %w", err)
	}
	convs := s.loadFeedConversations(ctx, logger, userID, ids, fc.gearMap, fc.communityGearMap)

	// slices.Concat, not append(eventIDs, ...): appending onto eventIDs would
	// write into its backing array whenever it has spare capacity, silently
	// corrupting it for any later reader.
	viewRecords, err := s.feedStorage.GetFeedItemViews(ctx, userID, communityID,
		slices.Concat(ids.eventIDs, ids.storyIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to get view records: %w", err)
	}

	expiryViews, expiryHours := feedExpiry(community)
	now := time.Now().Unix()

	feedItems := s.eventFeedItems(ctx, events, userID, fc, convs, viewRecords, expiryViews, expiryHours, now)
	return append(feedItems,
		s.storyFeedItems(ctx, stories, fc, viewRecords, expiryViews, expiryHours, now)...), nil
}

// eventFeedItems deduplicates the events by item and renders the survivors,
// dropping anything the viewer has already seen through.
func (s *Service) eventFeedItems(
	ctx context.Context,
	events []*models.CommunityEvent,
	userID string,
	fc *feedLookups,
	convs *feedConversations,
	viewRecords map[string]*models.FeedItemView,
	expiryViews, expiryHours int32,
	now int64,
) []*api.FeedItem {
	var feedItems []*api.FeedItem
	for _, group := range deduplicateEventsByItemKey(events) {
		event := group.latestEvent

		lastActivityAt := event.OccurredAtUnixSec
		if convActivity := convs.activityAt(event); convActivity > lastActivityAt {
			lastActivityAt = convActivity
		}

		view := viewRecords[event.Id]
		// Live opportunities — an event still ahead, a request still wanted, a
		// giveaway still on offer, a transfer mid-handoff — stay in the feed
		// until they resolve, even after being seen.  The archived check in
		// eventToFeedItem already filters out terminal items, so live items
		// need only skip the staleness expiry.  Everything else must be both
		// fresh (seen-based) and within the horizon (absolute age).
		if !isLiveOpportunity(event, fc, now) &&
			(!isItemFresh(view, lastActivityAt, now, expiryViews, expiryHours) ||
				!isWithinHorizon(lastActivityAt, now)) {
			continue
		}

		feedItem := s.eventToFeedItem(ctx, event, userID, fc)
		if feedItem == nil {
			continue
		}
		feedItem.LastActivityAtUnixSec = lastActivityAt
		feedItem.IsActionRequired = isActionRequired(event, userID, fc)
		feedItem.IsUnread = isItemUnread(view, lastActivityAt, now)
		if feedItem.IsUnread {
			feedItem.UnreadAtUnixSec = lastActivityAt
		}
		feedItems = append(feedItems, feedItem)
	}
	return feedItems
}

// storyFeedItems renders the stories that are still fresh and within the
// horizon. A story commemorates something that already happened, so it is never
// a live opportunity and always ages out.
func (s *Service) storyFeedItems(
	ctx context.Context,
	stories []*models.Story,
	fc *feedLookups,
	viewRecords map[string]*models.FeedItemView,
	expiryViews, expiryHours int32,
	now int64,
) []*api.FeedItem {
	var feedItems []*api.FeedItem
	for _, story := range stories {
		view := viewRecords[story.Id]
		if !isItemFresh(view, story.CreatedAtUnixSec, now, expiryViews, expiryHours) ||
			!isWithinHorizon(story.CreatedAtUnixSec, now) {
			continue
		}
		feedItem := s.createStoryItem(ctx, story, fc)
		if feedItem == nil {
			continue
		}
		feedItem.LastActivityAtUnixSec = story.CreatedAtUnixSec
		feedItem.IsUnread = isItemUnread(view, story.CreatedAtUnixSec, now)
		if feedItem.IsUnread {
			feedItem.UnreadAtUnixSec = story.CreatedAtUnixSec
		}
		feedItems = append(feedItems, feedItem)
	}
	return feedItems
}

// MarkFeedItemsViewed marks feed items as viewed, incrementing their view counts.
func (s *Service) MarkFeedItemsViewed(
	ctx context.Context,
	req *connect.Request[api.MarkFeedItemsViewedRequest],
) (*connect.Response[api.MarkFeedItemsViewedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
		"item_count", len(req.Msg.ItemIds),
	)

	logger.Info("marking feed items as viewed")

	// Verify community is active and caller is a member (single round-trip).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.sqlStorage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Batch increment view counts for all items.
	if err := s.feedStorage.IncrementViewCountBatch(ctx, authInfo.UserID, req.Msg.CommunityId, req.Msg.ItemIds, "event", nil); err != nil {
		logger.Error("failed to batch increment view counts", "error", err)
		return nil, connecterr.Internal(ctx, "MarkFeedItemsViewed", err)
	}

	return connect.NewResponse(&api.MarkFeedItemsViewedResponse{}), nil
}
