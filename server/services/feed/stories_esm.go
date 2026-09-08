package feed

import (
	"context"
	"time"

	esmlib "go.ripls.org/ripls/server/esm"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// enrichStoriesWithEmbeddedESM is the method-level wrapper that reaches
// into the service's storage handle and delegates to the package-level
// helper. Used by ListStories and ListItemStories.
func (s *Service) enrichStoriesWithEmbeddedESM(
	ctx context.Context,
	stories []*api.StoryPayload,
	viewerID string,
) {
	enrichStoriesWithEmbeddedESM(ctx, s.sqlStorage, stories, viewerID)
}

// enrichStoriesWithEmbeddedESM populates StoryPayload.embedded_esm_prompt
// and the social-proof avatar slice for every concluded-experience story
// in the slice. Stories that do not commemorate a concluded experience,
// or whose experience has no active prompt, are returned untouched.
//
// Uses batched ESM helpers so the total query cost is bounded
// (~3-4 queries) regardless of how many concluded-experience stories
// the input contains.
//
// Failures are logged and stories are returned without enrichment so a
// single broken lookup doesn't blank an entire feed page.
func enrichStoriesWithEmbeddedESM(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	stories []*api.StoryPayload,
	viewerID string,
) {
	if len(stories) == 0 {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "enrichStoriesWithEmbeddedESM",
		"user_id", viewerID,
	)

	experienceIDs := make([]string, 0, len(stories))
	seen := make(map[string]struct{}, len(stories))
	for _, story := range stories {
		if story.StoryType != api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED {
			continue
		}
		if story.ExperienceId == "" {
			continue
		}
		if _, dup := seen[story.ExperienceId]; dup {
			continue
		}
		seen[story.ExperienceId] = struct{}{}
		experienceIDs = append(experienceIDs, story.ExperienceId)
	}
	if len(experienceIDs) == 0 {
		return
	}

	now := time.Now()
	promptByExperience, err := esmlib.EmbeddedPromptsForExperiences(ctx, store, viewerID, experienceIDs, now)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-load embedded ESM prompts",
			"experience_count", len(experienceIDs),
			"error", err,
		)
		return
	}
	if len(promptByExperience) == 0 {
		return
	}

	// Collect (prompt_id → viewer's option_key) for prompts where the
	// viewer has already voted. Only those need a social-proof lookup.
	promptKeys := make(map[string]string, len(promptByExperience))
	for _, prompt := range promptByExperience {
		if prompt.CurrentResponseOptionKey == nil || *prompt.CurrentResponseOptionKey == "" {
			continue
		}
		promptKeys[prompt.PromptId] = *prompt.CurrentResponseOptionKey
	}
	socialProofByPrompt, err := esmlib.SocialProofForPromptsBatch(ctx, store, viewerID, promptKeys)
	if err != nil {
		// Don't bail — without social proof we can still surface the
		// prompt itself, which is the more important payload.
		logger.WarnContext(ctx, "failed to batch-load embedded ESM social proof",
			"prompt_count", len(promptKeys),
			"error", err,
		)
		socialProofByPrompt = nil
	}

	for _, story := range stories {
		if story.StoryType != api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED {
			continue
		}
		if story.ExperienceId == "" {
			continue
		}
		prompt, ok := promptByExperience[story.ExperienceId]
		if !ok || prompt == nil {
			continue
		}
		story.EmbeddedEsmPrompt = prompt

		if prompt.CurrentResponseOptionKey == nil || *prompt.CurrentResponseOptionKey == "" {
			continue
		}
		proof, ok := socialProofByPrompt[prompt.PromptId]
		if !ok {
			continue
		}
		story.EmbeddedEsmSocialProofUsers = toAPIUsersForSocialProof(proof.Users)
		story.EmbeddedEsmSocialProofRemainder = proof.Remainder
	}
}

// enrichFeedStoriesWithEmbeddedESM walks a slice of feed items and
// enriches the StoryPayload of every Story-typed item. Used by GetFeed
// after sort so per-item story enrichment happens in a single pass.
func enrichFeedStoriesWithEmbeddedESM(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	items []*api.FeedItem,
	viewerID string,
) {
	stories := make([]*api.StoryPayload, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		storyPayload, ok := item.Payload.(*api.FeedItem_Story)
		if !ok || storyPayload == nil || storyPayload.Story == nil {
			continue
		}
		stories = append(stories, storyPayload.Story)
	}
	enrichStoriesWithEmbeddedESM(ctx, store, stories, viewerID)
}

func toAPIUsersForSocialProof(users []*models.User) []*api.User {
	out := make([]*api.User, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		apiUser := &api.User{
			Id:   u.Id,
			Name: u.Name,
		}
		if avatar := services.PrimaryAvatarMediaID(u); avatar != "" {
			apiUser.MediaId = avatar
		}
		out = append(out, apiUser)
	}
	return out
}
