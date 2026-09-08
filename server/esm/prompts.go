package esm

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// GetPrompt retrieves a single StoredESMPrompt by ID. Returns the storage
// layer's not-found error when no prompt exists.
func GetPrompt(ctx context.Context, store *storage.ProtoSQLStorage, promptID string) (*models.StoredESMPrompt, error) {
	prompt := &models.StoredESMPrompt{}
	if err := store.GetByID(ctx, promptID, prompt); err != nil {
		return nil, fmt.Errorf("esm.GetPrompt: %w", err)
	}
	return prompt, nil
}

// QueryPromptsByExperience returns all non-deleted prompts authored against
// the given experience.
func QueryPromptsByExperience(ctx context.Context, store *storage.ProtoSQLStorage, experienceID string) ([]*models.StoredESMPrompt, error) {
	prompts, err := storage.QueryByFields[*models.StoredESMPrompt](store, ctx, map[string]any{
		"experience_id": experienceID,
	})
	if err != nil {
		return nil, fmt.Errorf("esm.QueryPromptsByExperience: %w", err)
	}
	return prompts, nil
}

// InsertPrompt persists a new StoredESMPrompt record.
func InsertPrompt(ctx context.Context, store *storage.ProtoSQLStorage, prompt *models.StoredESMPrompt) error {
	if _, err := store.Insert(ctx, prompt); err != nil {
		return fmt.Errorf("esm.InsertPrompt: %w", err)
	}
	return nil
}
