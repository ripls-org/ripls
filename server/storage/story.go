package storage

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// StoryStorage provides story-specific database operations.
type StoryStorage struct {
	storage *ProtoSQLStorage
}

// NewStoryStorage creates a new story storage instance.
func NewStoryStorage(storage *ProtoSQLStorage) *StoryStorage {
	return &StoryStorage{storage: storage}
}

// Insert creates a new story in the database. The participant_ids
// TEXT[] denormalization column is auto-synced from story.ParticipantIds
// inside the same transaction as the proto write — see
// [ProtoSQLStorage.RegisterArrayColumn] and InitializeStoryArrayColumns.
func (ss *StoryStorage) Insert(ctx context.Context, story *models.Story) (string, error) {
	return ss.storage.Insert(ctx, story)
}

// GetByID retrieves a story by its ID.
func (ss *StoryStorage) GetByID(ctx context.Context, id string) (*models.Story, error) {
	story := &models.Story{}
	if err := ss.storage.GetByID(ctx, id, story); err != nil {
		return nil, fmt.Errorf("failed to get story: %w", err)
	}
	return story, nil
}

// Story list bounds. A caller that passes no limit gets defaultStoryLimit;
// one that passes an absurd limit gets maxStoryLimit. Before #2795 only the
// floor existed, so `limit` was effectively unbounded from the caller's side —
// which docs/server/conventions.md § SQL Efficiency rule 3 forbids.
const (
	defaultStoryLimit = 50
	maxStoryLimit     = 500
)

// clampStoryLimit applies the default and the ceiling.
func clampStoryLimit(limit int) int {
	if limit <= 0 {
		return defaultStoryLimit
	}
	if limit > maxStoryLimit {
		return maxStoryLimit
	}
	return limit
}

// ListByCommunity retrieves all stories for a community, ordered by creation time (newest first).
func (ss *StoryStorage) ListByCommunity(ctx context.Context, communityID string, limit int) ([]*models.Story, error) {
	limit = clampStoryLimit(limit)

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "Story"
		WHERE community_id = %s
		AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0
		ORDER BY created_at_unix_sec DESC
		LIMIT %s
	`, ss.storage.dbSpec.Placeholder(1), ss.storage.dbSpec.Placeholder(2))

	rows, err := ss.storage.db.QueryContext(ctx, query, communityID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query stories: %w", err)
	}
	defer rows.Close()

	var stories []*models.Story
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan story: %w", err)
		}

		story := &models.Story{}
		if err := proto.Unmarshal(binaryProto, story); err != nil {
			return nil, fmt.Errorf("failed to unmarshal story: %w", err)
		}

		stories = append(stories, story)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stories: %w", err)
	}

	return stories, nil
}

// ListByUser retrieves all stories where the user is a participant, ordered by creation time (newest first).
func (ss *StoryStorage) ListByUser(ctx context.Context, userID string, limit int) ([]*models.Story, error) {
	limit = clampStoryLimit(limit)

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "Story"
		WHERE %s = ANY(participant_ids)
		AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0
		ORDER BY created_at_unix_sec DESC
		LIMIT %s
	`, ss.storage.dbSpec.Placeholder(1), ss.storage.dbSpec.Placeholder(2))

	rows, err := ss.storage.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query user stories: %w", err)
	}
	defer rows.Close()

	var stories []*models.Story
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan story: %w", err)
		}

		story := &models.Story{}
		if err := proto.Unmarshal(binaryProto, story); err != nil {
			return nil, fmt.Errorf("failed to unmarshal story: %w", err)
		}

		stories = append(stories, story)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stories: %w", err)
	}

	return stories, nil
}

// ListByItem retrieves all stories for a specific item (gear, request, or experience), ordered by creation time (newest first).
// itemID can match any of: gear_id, experience_id, or request_id.
func (ss *StoryStorage) ListByItem(ctx context.Context, itemID string, limit int) ([]*models.Story, error) {
	limit = clampStoryLimit(limit)

	query := fmt.Sprintf(`
		SELECT binary_proto
		FROM "Story"
		WHERE (
			gear_id = %s
			OR experience_id = %s
			OR request_id = %s
		)
		AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0
		ORDER BY created_at_unix_sec DESC
		LIMIT %s
	`, ss.storage.dbSpec.Placeholder(1),
		ss.storage.dbSpec.Placeholder(2),
		ss.storage.dbSpec.Placeholder(3),
		ss.storage.dbSpec.Placeholder(4))

	rows, err := ss.storage.db.QueryContext(ctx, query, itemID, itemID, itemID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query item stories: %w", err)
	}
	defer rows.Close()

	var stories []*models.Story
	for rows.Next() {
		var binaryProto []byte
		if err := rows.Scan(&binaryProto); err != nil {
			return nil, fmt.Errorf("failed to scan story: %w", err)
		}

		story := &models.Story{}
		if err := proto.Unmarshal(binaryProto, story); err != nil {
			return nil, fmt.Errorf("failed to unmarshal story: %w", err)
		}

		stories = append(stories, story)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stories: %w", err)
	}

	return stories, nil
}

// Exists checks if a story already exists for a given combination of community, story type, and related entity.
// This is used for deduplication to prevent creating multiple stories for the same event.
// relatedEntityID can be one of: gear_id, loan_id, experience_id, or request_id.
func (ss *StoryStorage) Exists(ctx context.Context, communityID, storyType, relatedEntityID string) (bool, error) {
	// Query for existing story with matching community, type, and any of the related entity IDs
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM "Story"
		WHERE community_id = %s
		AND story_type = %s
		AND (
			gear_id = %s
			OR loan_id = %s
			OR experience_id = %s
			OR request_id = %s
		)
		AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0
	`, ss.storage.dbSpec.Placeholder(1),
		ss.storage.dbSpec.Placeholder(2),
		ss.storage.dbSpec.Placeholder(3),
		ss.storage.dbSpec.Placeholder(4),
		ss.storage.dbSpec.Placeholder(5),
		ss.storage.dbSpec.Placeholder(6))

	var count int
	err := ss.storage.db.QueryRowContext(ctx, query,
		communityID,
		storyType,
		relatedEntityID,
		relatedEntityID,
		relatedEntityID,
		relatedEntityID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check story existence: %w", err)
	}

	return count > 0, nil
}
