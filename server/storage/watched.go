package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// WatchStorage provides watched-item database operations.
type WatchStorage struct {
	storage *ProtoSQLStorage
}

// NewWatchStorage creates a new watch storage instance.
func NewWatchStorage(storage *ProtoSQLStorage) *WatchStorage {
	return &WatchStorage{storage: storage}
}

// UpsertWatch creates a watch entry or reactivates a dismissed one.
// If the watch already exists and is active, this is a no-op.
// If the watch was dismissed, it clears dismissed_at and marks unread.
func (ws *WatchStorage) UpsertWatch(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) error {
	now := time.Now().Unix()

	// Try to find an existing watch for this (user, item_type, item_id).
	existing, err := ws.getWatch(ctx, userID, itemType, itemID)
	if err != nil {
		return fmt.Errorf("failed to check existing watch: %w", err)
	}

	if existing != nil {
		// Watch exists. If dismissed, reactivate it.
		if existing.DismissedAtUnixSec != nil {
			existing.DismissedAtUnixSec = nil
			existing.IsRead = false
			existing.UpdatedAtUnixSec = now
			if err := ws.storage.Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to reactivate watch: %w", err)
			}
		}
		return nil
	}

	// Create new watch entry.
	watch := &models.WatchedItem{
		Id:               uuid.New().String(),
		UserId:           userID,
		ItemType:         itemType,
		ItemId:           itemID,
		IsRead:           false,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	if _, err := ws.storage.Insert(ctx, watch); err != nil {
		return fmt.Errorf("failed to insert watch: %w", err)
	}
	return nil
}

// MarkRead sets is_read = true for a specific watch entry.
func (ws *WatchStorage) MarkRead(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) error {
	existing, err := ws.getWatch(ctx, userID, itemType, itemID)
	if err != nil {
		return fmt.Errorf("failed to get watch: %w", err)
	}
	if existing == nil || existing.DismissedAtUnixSec != nil {
		return nil // No active watch to mark read.
	}
	if existing.IsRead {
		return nil // Already read.
	}
	existing.IsRead = true
	if err := ws.storage.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to mark watch read: %w", err)
	}
	return nil
}

// MarkUnreadForWatchers marks all active watchers of an item as unread,
// except the specified user (typically the actor who triggered the event).
// Each watch is fetched and updated individually so that binary_proto stays
// consistent with the scalar columns.
func (ws *WatchStorage) MarkUnreadForWatchers(ctx context.Context, itemType models.WatchedItemType, itemID, exceptUserID string) error {
	rows, err := ws.storage.db.QueryContext(ctx, `
		SELECT binary_proto FROM "watched_item"
		WHERE item_type = $1 AND item_id = $2
		  AND user_id != $3
		  AND (dismissed_at_unix_sec IS NULL OR dismissed_at_unix_sec = 0)
		  AND is_read = true
	`, int32(itemType), itemID, exceptUserID)
	if err != nil {
		return fmt.Errorf("failed to query watchers: %w", err)
	}
	defer rows.Close()

	var watches []*models.WatchedItem
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return fmt.Errorf("failed to scan watcher row: %w", err)
		}
		watch := &models.WatchedItem{}
		if err := proto.Unmarshal(data, watch); err != nil {
			return fmt.Errorf("failed to unmarshal watch: %w", err)
		}
		watches = append(watches, watch)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate watcher rows: %w", err)
	}

	now := time.Now().Unix()
	for _, watch := range watches {
		watch.IsRead = false
		watch.UpdatedAtUnixSec = now
		if err := ws.storage.Update(ctx, watch); err != nil {
			return fmt.Errorf("failed to mark watcher unread (user=%s): %w", watch.UserId, err)
		}
	}
	return nil
}

// DismissWatch soft-deletes a watch by setting dismissed_at.
func (ws *WatchStorage) DismissWatch(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) error {
	existing, err := ws.getWatch(ctx, userID, itemType, itemID)
	if err != nil {
		return fmt.Errorf("failed to get watch: %w", err)
	}
	if existing == nil || existing.DismissedAtUnixSec != nil {
		return nil // Already dismissed or doesn't exist.
	}
	now := time.Now().Unix()
	existing.DismissedAtUnixSec = &now
	if err := ws.storage.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to dismiss watch: %w", err)
	}
	return nil
}

// DismissAllForItem dismisses all active watches for an item (used when item
// reaches terminal state).
func (ws *WatchStorage) DismissAllForItem(ctx context.Context, itemType models.WatchedItemType, itemID string) error {
	now := time.Now().Unix()
	_, err := ws.storage.db.ExecContext(ctx, `
		UPDATE "watched_item"
		SET dismissed_at_unix_sec = $1
		WHERE item_type = $2 AND item_id = $3
		  AND (dismissed_at_unix_sec IS NULL OR dismissed_at_unix_sec = 0)
	`, now, int32(itemType), itemID)
	if err != nil {
		return fmt.Errorf("failed to dismiss all watches for item: %w", err)
	}
	return nil
}

// GetAllWatches returns all watch records for a user, including dismissed ones.
// Used to distinguish "no record" (new item) from "dismissed" (user opted out).
func (ws *WatchStorage) GetAllWatches(ctx context.Context, userID string) ([]*models.WatchedItem, error) {
	rows, err := ws.storage.db.QueryContext(ctx, `
		SELECT binary_proto FROM "watched_item"
		WHERE user_id = $1
		ORDER BY updated_at_unix_sec DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query all watches: %w", err)
	}
	defer rows.Close()

	var watches []*models.WatchedItem
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan watch row: %w", err)
		}
		watch := &models.WatchedItem{}
		if err := proto.Unmarshal(data, watch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal watch: %w", err)
		}
		watches = append(watches, watch)
	}
	return watches, rows.Err()
}

// GetActiveWatches returns all non-dismissed watches for a user.
func (ws *WatchStorage) GetActiveWatches(ctx context.Context, userID string) ([]*models.WatchedItem, error) {
	rows, err := ws.storage.db.QueryContext(ctx, `
		SELECT binary_proto FROM "watched_item"
		WHERE user_id = $1
		  AND (dismissed_at_unix_sec IS NULL OR dismissed_at_unix_sec = 0)
		ORDER BY updated_at_unix_sec DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active watches: %w", err)
	}
	defer rows.Close()

	var watches []*models.WatchedItem
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan watch row: %w", err)
		}
		watch := &models.WatchedItem{}
		if err := proto.Unmarshal(data, watch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal watch: %w", err)
		}
		watches = append(watches, watch)
	}
	return watches, rows.Err()
}

// GetUnreadCount returns the count of unread, non-dismissed watches for a user.
func (ws *WatchStorage) GetUnreadCount(ctx context.Context, userID string) (int32, error) {
	var count int32
	err := ws.storage.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM "watched_item"
		WHERE user_id = $1
		  AND is_read = false
		  AND (dismissed_at_unix_sec IS NULL OR dismissed_at_unix_sec = 0)
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread watches: %w", err)
	}
	return count, nil
}

// IsWatched checks if a specific item has an active (non-dismissed) watch entry
// for a user, regardless of read state.
func (ws *WatchStorage) IsWatched(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) (bool, error) {
	watch, err := ws.getWatch(ctx, userID, itemType, itemID)
	if err != nil {
		return false, err
	}
	if watch == nil || watch.DismissedAtUnixSec != nil {
		return false, nil
	}
	return true, nil
}

// IsWatchedUnread checks if a specific item is watched and unread for a user.
func (ws *WatchStorage) IsWatchedUnread(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) (bool, error) {
	watch, err := ws.getWatch(ctx, userID, itemType, itemID)
	if err != nil {
		return false, err
	}
	if watch == nil || watch.DismissedAtUnixSec != nil {
		return false, nil
	}
	return !watch.IsRead, nil
}

// getWatch retrieves a watch entry by its unique key (user_id, item_type, item_id).
func (ws *WatchStorage) getWatch(ctx context.Context, userID string, itemType models.WatchedItemType, itemID string) (*models.WatchedItem, error) {
	rows, err := ws.storage.db.QueryContext(ctx, `
		SELECT binary_proto FROM "watched_item"
		WHERE user_id = $1 AND item_type = $2 AND item_id = $3
		LIMIT 1
	`, userID, int32(itemType), itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to query watch: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		// Next() returns false both for "no rows" and for an iteration error, so
		// rows.Err() has to be consulted before reporting "no watch". Without it
		// a transient DB failure was indistinguishable from an unwatched item —
		// and callers read a nil watch as "not watched", which drives unread and
		// notification state.
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate watch: %w", err)
		}
		return nil, nil
	}
	var data []byte
	if err := rows.Scan(&data); err != nil {
		return nil, fmt.Errorf("failed to scan watch: %w", err)
	}
	watch := &models.WatchedItem{}
	if err := proto.Unmarshal(data, watch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal watch: %w", err)
	}
	return watch, nil
}
