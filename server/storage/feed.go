package storage

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// FeedStorage provides feed-specific storage operations.
type FeedStorage struct {
	storage *ProtoSQLStorage
}

// NewFeedStorage creates a new feed storage instance.
func NewFeedStorage(storage *ProtoSQLStorage) *FeedStorage {
	return &FeedStorage{storage: storage}
}

// GetFeedItemViews retrieves view records for multiple feed items for a user in a community.
// Returns a map of feed_item_id -> FeedItemView for efficient lookup.
func (fs *FeedStorage) GetFeedItemViews(ctx context.Context, userID, communityID string, itemIDs []string) (map[string]*models.FeedItemView, error) {
	if len(itemIDs) == 0 {
		return make(map[string]*models.FeedItemView), nil
	}

	// Build query with IN clause for batch fetching
	placeholders := make([]string, len(itemIDs))
	args := []interface{}{userID, communityID}
	for i, itemID := range itemIDs {
		placeholders[i] = fs.storage.dbSpec.Placeholder(i + 3) // +3 because userID and communityID are first
		args = append(args, itemID)
	}

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "FeedItemView"
		WHERE user_id = %s
		AND community_id = %s
		AND feed_item_id IN (%s)
	`, fs.storage.dbSpec.Placeholder(1), fs.storage.dbSpec.Placeholder(2),
		strings.Join(placeholders, ","))

	rows, err := fs.storage.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query feed item views: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*models.FeedItemView)
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan feed item view: %w", err)
		}

		view := &models.FeedItemView{}
		if err := proto.Unmarshal(binaryProto, view); err != nil {
			return nil, fmt.Errorf("failed to unmarshal feed item view: %w", err)
		}

		result[view.FeedItemId] = view
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating feed item views: %w", err)
	}

	return result, nil
}

// IncrementViewCount increments the view count for a feed item, creating a new record if needed.
func (fs *FeedStorage) IncrementViewCount(ctx context.Context, userID, communityID, itemID, itemType string) error {
	now := clock.UnixSec(ctx)

	// Try to get existing view record
	existingViews, err := fs.GetFeedItemViews(ctx, userID, communityID, []string{itemID})
	if err != nil {
		return fmt.Errorf("failed to get existing view: %w", err)
	}

	if existingView, exists := existingViews[itemID]; exists {
		// Update existing record.
		existingView.ViewCount++
		existingView.LastViewedAtUnixSec = now
		existingView.LastSeenAtUnixSec = now
		existingView.ViewsSinceLastActivity++

		if err := fs.storage.Update(ctx, existingView); err != nil {
			return fmt.Errorf("failed to update view count: %w", err)
		}
	} else {
		// Create new record.
		newView := &models.FeedItemView{
			UserId:                 userID,
			CommunityId:            communityID,
			FeedItemId:             itemID,
			ItemType:               itemType,
			ViewCount:              1,
			FirstViewedAtUnixSec:   now,
			LastViewedAtUnixSec:    now,
			LastSeenAtUnixSec:      now,
			ViewsSinceLastActivity: 1,
		}

		if _, err := fs.storage.Insert(ctx, newView); err != nil {
			return fmt.Errorf("failed to insert view record: %w", err)
		}
	}

	return nil
}

// IncrementViewCountBatch increments view counts for multiple feed items in
// fewer queries than calling IncrementViewCount per item. It fetches all
// existing views in one batch query, then inserts new records with InsertBatch
// and updates existing records individually.
//
// lastActivityAtByID maps each feed item ID to its most recent activity
// timestamp. When provided, views_since_last_activity is reset to 0 for items
// where last_activity_at > last_seen_at (new activity since last seen), or
// incremented otherwise. Pass nil to skip activity-aware tracking.
func (fs *FeedStorage) IncrementViewCountBatch(ctx context.Context, userID, communityID string, itemIDs []string, itemType string, lastActivityAtByID map[string]int64) error {
	if len(itemIDs) == 0 {
		return nil
	}

	now := clock.UnixSec(ctx)

	// Batch fetch all existing view records (1 query).
	existingViews, err := fs.GetFeedItemViews(ctx, userID, communityID, itemIDs)
	if err != nil {
		return fmt.Errorf("batch get existing views: %w", err)
	}

	// Separate into new vs existing items.
	var newViews []proto.Message
	for _, itemID := range itemIDs {
		lastActivityAt := int64(0)
		if lastActivityAtByID != nil {
			lastActivityAt = lastActivityAtByID[itemID]
		}

		if existingView, exists := existingViews[itemID]; exists {
			// Capture old timestamp before overwriting so the activity check below
			// compares against the value the user last saw, not the current time.
			oldLastSeen := existingView.LastSeenAtUnixSec

			// Update existing record.
			existingView.ViewCount++
			existingView.LastViewedAtUnixSec = now
			existingView.LastSeenAtUnixSec = now

			// Reset views_since_last_activity if there's new activity since last seen.
			if lastActivityAt > oldLastSeen {
				existingView.ViewsSinceLastActivity = 1
			} else {
				existingView.ViewsSinceLastActivity++
			}

			if err := fs.storage.Update(ctx, existingView); err != nil {
				return fmt.Errorf("update view count for %s: %w", itemID, err)
			}
		} else {
			newViews = append(newViews, &models.FeedItemView{
				UserId:                 userID,
				CommunityId:            communityID,
				FeedItemId:             itemID,
				ItemType:               itemType,
				ViewCount:              1,
				FirstViewedAtUnixSec:   now,
				LastViewedAtUnixSec:    now,
				LastSeenAtUnixSec:      now,
				ViewsSinceLastActivity: 1,
			})
		}
	}

	// Batch insert all new view records (1 query).
	if len(newViews) > 0 {
		if _, err := fs.storage.InsertBatch(ctx, newViews); err != nil {
			return fmt.Errorf("batch insert new views: %w", err)
		}
	}

	return nil
}

// GetViewCountsForItems returns view counts for specific items.
// Returns a map of feed_item_id -> view_count.
func (fs *FeedStorage) GetViewCountsForItems(ctx context.Context, userID, communityID string, itemIDs []string) (map[string]int32, error) {
	views, err := fs.GetFeedItemViews(ctx, userID, communityID, itemIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[string]int32, len(itemIDs))
	for itemID, view := range views {
		result[itemID] = view.ViewCount
	}

	// Fill in zeros for items with no views
	for _, itemID := range itemIDs {
		if _, exists := result[itemID]; !exists {
			result[itemID] = 0
		}
	}

	return result, nil
}

// GetLatestNonViewerMessageAtBatch returns the most recent message timestamp per
// conversation for messages NOT authored by viewerUserID. Only user-authored
// messages with a sender different from the viewer are considered; system messages
// (empty sender) count as activity for everyone.
//
// Returns a map of conversation_id -> unix timestamp (seconds). Conversations with
// no qualifying messages are absent from the map.
func (fs *FeedStorage) GetLatestNonViewerMessageAtBatch(ctx context.Context, conversationIDs []string, viewerUserID string) (map[string]int64, error) {
	if len(conversationIDs) == 0 {
		return make(map[string]int64), nil
	}

	placeholders := make([]string, len(conversationIDs))
	args := []interface{}{viewerUserID}
	for i, id := range conversationIDs {
		placeholders[i] = fs.storage.dbSpec.Placeholder(i + 2) // +2 because viewerUserID is $1
		args = append(args, id)
	}

	// user_message_sender_id is NULL for system messages; NULL IS DISTINCT FROM
	// viewerUserID so system messages are included as activity for everyone.
	query := fmt.Sprintf(`
		SELECT conversation_id, MAX(sent_at_unix_sec)
		FROM "chat_message"
		WHERE conversation_id IN (%s)
		  AND (user_message_sender_id IS NULL
		       OR user_message_sender_id = ''
		       OR user_message_sender_id IS DISTINCT FROM %s)
		GROUP BY conversation_id
	`, strings.Join(placeholders, ","), fs.storage.dbSpec.Placeholder(1))

	rows, err := fs.storage.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get latest non-viewer message timestamps: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64, len(conversationIDs))
	for rows.Next() {
		var convID string
		var lastAt int64
		if err := rows.Scan(&convID, &lastAt); err != nil {
			return nil, fmt.Errorf("scan conversation timestamp: %w", err)
		}
		result[convID] = lastAt
	}
	return result, rows.Err()
}
