package story_subscriber

import (
	"context"
	"fmt"
	"time"

	esmlib "go.ripls.org/ripls/server/esm"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// storyEmbeddedPromptQuestion is the canonical v1 question rendered at the
// bottom of post-experience stories.
const storyEmbeddedPromptQuestion = "Want to join if they do something like this again?"

func storyEmbeddedPromptOptions() []*models.ESMResponseOption {
	return []*models.ESMResponseOption{
		{Key: "do_again", Label: "Yes"},
		{Key: "maybe", Label: "Maybe"},
		{Key: "not_for_me", Label: "Not for me"},
	}
}

// materializeESMPromptForStory creates the StoredESMPrompt and pre-seeded
// StoredESMResponse rows for the (experience, community) pair so the story
// renders with an embedded voting block on first paint. Idempotent: returns
// nil without rewriting state when a prompt already exists for the pair.
func materializeESMPromptForStory(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	experience *models.Experience,
	communityID, storyID string,
	now time.Time,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "materializeESMPromptForStory",
		"experience_id", experience.Id,
		"community_id", communityID,
		"story_id", storyID,
	)

	existing, err := esmlib.QueryPromptsByExperience(ctx, store, experience.Id)
	if err != nil {
		return fmt.Errorf("query existing prompts: %w", err)
	}
	for _, p := range existing {
		if p.CommunityId == communityID && p.Deleted == nil {
			logger.DebugContext(ctx, "ESM prompt already exists for experience+community, skipping")
			return nil
		}
	}

	attendeeIDs, err := esmlib.AttendeeUserIDs(ctx, store, experience.Id)
	if err != nil {
		return fmt.Errorf("query attendee user IDs: %w", err)
	}
	if len(attendeeIDs) == 0 {
		logger.DebugContext(ctx, "no attendees for experience, skipping ESM prompt materialization")
		return nil
	}

	draft := esmlib.PromptDraft{
		Question:        storyEmbeddedPromptQuestion,
		Options:         storyEmbeddedPromptOptions(),
		Recipients:      attendeeIDs,
		CreatedByUserID: experience.OwnerId,
	}
	prompt, responses, err := esmlib.BuildPromptForExperience(ctx, store, experience.Id, draft, now)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}
	if err := esmlib.InsertPrompt(ctx, store, prompt); err != nil {
		return fmt.Errorf("insert prompt: %w", err)
	}
	if err := esmlib.InsertResponseRows(ctx, store, responses); err != nil {
		return fmt.Errorf("insert response rows: %w", err)
	}

	logger.InfoContext(ctx, "ESM prompt materialized for story",
		"esm_prompt_id", prompt.Id,
		"recipient_count", len(responses),
	)
	return nil
}
