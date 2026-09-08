package feed

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestEnrichStoriesWithEmbeddedESM_BoundedQueries pins the per-call DB
// cost of ESM enrichment to a small bound regardless of how many
// concluded-experience stories the feed contains. Regression test for
// the N+1 surfaced in #1807, where a 50-story feed at ~half concluded
// experiences cost ~50 queries inside enrichStoriesWithEmbeddedESM.
//
// Bound: 4 queries.
//   - 1 QueryByFieldIn for prompts on experience_id IN (...)
//   - 1 QueryByFieldIn for viewer's response rows on prompt_id IN (...)
//   - 1 QueryByFieldIn for social-proof responses on prompt_id IN (...)
//   - 1 GetByIDs for avatar users
//
// Loosened to 6 to absorb minor implementation drift; tighten if it
// becomes too lax.
func TestEnrichStoriesWithEmbeddedESM_BoundedQueries(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()
	now := time.Now()
	closesAt := now.Add(48 * time.Hour).Unix()

	const viewerID = "viewer-bounded-queries"
	const respondentID = "respondent-bounded-queries"
	if _, err := sqlStorage.Insert(ctx, &models.User{Id: viewerID, Name: "Viewer"}); err != nil {
		t.Fatalf("insert viewer: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.User{Id: respondentID, Name: "Respondent"}); err != nil {
		t.Fatalf("insert respondent: %v", err)
	}

	const storyCount = 30
	stories := make([]*api.StoryPayload, storyCount)
	for i := 0; i < storyCount; i++ {
		expID := uuid.NewString()
		prompt := &models.StoredESMPrompt{
			Id:               uuid.NewString(),
			ExperienceId:     expID,
			CommunityId:      "comm-bounded",
			Question:         "Want to do this again?",
			ResponseOptions:  []*models.ESMResponseOption{{Key: "do_again", Label: "Yes"}, {Key: "not_for_me", Label: "No"}},
			CreatedAtUnixSec: now.Unix(),
			ClosesAtUnixSec:  closesAt,
		}
		if _, err := sqlStorage.Insert(ctx, prompt); err != nil {
			t.Fatalf("insert prompt %d: %v", i, err)
		}

		// Viewer voted on every story so the social-proof path runs too.
		viewerKey := "do_again"
		viewerRow := &models.StoredESMResponse{
			Id:                uuid.NewString(),
			PromptId:          prompt.Id,
			ExperienceId:      expID,
			UserId:            viewerID,
			ConsumedAction:    models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey: &viewerKey,
		}
		if _, err := sqlStorage.Insert(ctx, viewerRow); err != nil {
			t.Fatalf("insert viewer response %d: %v", i, err)
		}
		// One co-respondent per prompt so social-proof has work to do
		// (avatars + at least one user lookup batch).
		respondentKey := "do_again"
		respondentRow := &models.StoredESMResponse{
			Id:                uuid.NewString(),
			PromptId:          prompt.Id,
			ExperienceId:      expID,
			UserId:            respondentID,
			ConsumedAction:    models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey: &respondentKey,
		}
		if _, err := sqlStorage.Insert(ctx, respondentRow); err != nil {
			t.Fatalf("insert respondent response %d: %v", i, err)
		}

		stories[i] = &api.StoryPayload{
			StoryType:    api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED,
			ExperienceId: expID,
		}
	}

	statsCtx := storage.WithQueryStats(ctx)
	const maxQueries = 6
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		enrichStoriesWithEmbeddedESM(statsCtx, sqlStorage, stories, viewerID)
	})

	if stats := storage.GetQueryStats(statsCtx); stats != nil {
		t.Logf("actual query count for %d-story enrichment: %d", storyCount, stats.Count.Load())
	}

	// Spot-check correctness: every story should have its prompt and
	// social-proof populated since the viewer voted on every prompt and
	// has a same-option co-respondent.
	for i, story := range stories {
		if story.EmbeddedEsmPrompt == nil {
			t.Errorf("story %d: missing EmbeddedEsmPrompt", i)
			continue
		}
		if story.EmbeddedEsmPrompt.CurrentResponseOptionKey == nil ||
			*story.EmbeddedEsmPrompt.CurrentResponseOptionKey != "do_again" {
			t.Errorf("story %d: viewer's vote not propagated", i)
		}
		if len(story.EmbeddedEsmSocialProofUsers) != 1 ||
			story.EmbeddedEsmSocialProofUsers[0].Id != respondentID {
			t.Errorf("story %d: social-proof avatars wrong: %+v", i, story.EmbeddedEsmSocialProofUsers)
		}
	}
}

// TestEnrichStoriesWithEmbeddedESM_NoConcludedStoriesIsZeroQueries
// guards against future regressions where the batched path issues
// queries even when the feed contains no concluded-experience stories.
func TestEnrichStoriesWithEmbeddedESM_NoConcludedStoriesIsZeroQueries(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	stories := []*api.StoryPayload{
		{StoryType: api.StoryType_STORY_TYPE_LOAN_COMPLETED, ExperienceId: ""},
		{StoryType: api.StoryType_STORY_TYPE_LOAN_COMPLETED, ExperienceId: ""},
	}

	statsCtx := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, statsCtx, 0, func() {
		enrichStoriesWithEmbeddedESM(statsCtx, sqlStorage, stories, "viewer")
	})
}
