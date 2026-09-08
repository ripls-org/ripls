package esm

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// GetResponseForUser retrieves the StoredESMResponse row for a given
// (user, prompt) pair. Returns nil with no error when no row exists,
// indicating the user was not targeted by the prompt.
func GetResponseForUser(ctx context.Context, store *storage.ProtoSQLStorage, userID, promptID string) (*models.StoredESMResponse, error) {
	rows, err := storage.QueryByFields[*models.StoredESMResponse](store, ctx, map[string]any{
		"user_id":   userID,
		"prompt_id": promptID,
	})
	if err != nil {
		return nil, fmt.Errorf("esm.GetResponseForUser: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// QueryUnresolvedForUser returns all unresolved (still-pending) ESM response
// rows for a user. Used by the feed assembly to determine which ESM prompts
// to surface in the feed.
func QueryUnresolvedForUser(ctx context.Context, store *storage.ProtoSQLStorage, userID string) ([]*models.StoredESMResponse, error) {
	rows, err := storage.QueryByFields[*models.StoredESMResponse](store, ctx, map[string]any{
		"user_id":         userID,
		"consumed_action": int32(models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED),
	})
	if err != nil {
		return nil, fmt.Errorf("esm.QueryUnresolvedForUser: %w", err)
	}
	return rows, nil
}

// QueryResponsesByPrompt returns all response rows for a prompt, regardless
// of consumed_action. Used by the aggregation path.
func QueryResponsesByPrompt(ctx context.Context, store *storage.ProtoSQLStorage, promptID string) ([]*models.StoredESMResponse, error) {
	rows, err := storage.QueryByFields[*models.StoredESMResponse](store, ctx, map[string]any{
		"prompt_id": promptID,
	})
	if err != nil {
		return nil, fmt.Errorf("esm.QueryResponsesByPrompt: %w", err)
	}
	return rows, nil
}

// InsertResponseRows inserts one or more response rows in a single batched
// INSERT. Used at prompt-authoring time to seed UNRESOLVED rows for each
// targeted recipient.
func InsertResponseRows(ctx context.Context, store *storage.ProtoSQLStorage, rows []*models.StoredESMResponse) error {
	if len(rows) == 0 {
		return nil
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		return fmt.Errorf("esm.InsertResponseRows: %w", err)
	}
	return nil
}

// UpdateResponse persists changes to a StoredESMResponse (e.g., flipping
// consumed_action from UNRESOLVED to RESPONDED or DISMISSED).
func UpdateResponse(ctx context.Context, store *storage.ProtoSQLStorage, row *models.StoredESMResponse) error {
	if err := store.Update(ctx, row); err != nil {
		return fmt.Errorf("esm.UpdateResponse: %w", err)
	}
	return nil
}
