package esm

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func makePromptForStory(t *testing.T, store *storage.ProtoSQLStorage, ctx context.Context, experienceID, communityID string, closesAt int64) *models.StoredESMPrompt {
	t.Helper()
	prompt := &models.StoredESMPrompt{
		Id:               uuid.NewString(),
		ExperienceId:     experienceID,
		CommunityId:      communityID,
		Question:         "Want to do this again?",
		ResponseOptions:  []*models.ESMResponseOption{{Key: "do_again", Label: "Yes"}, {Key: "not_for_me", Label: "No"}},
		CreatedAtUnixSec: time.Now().Unix(),
		ClosesAtUnixSec:  closesAt,
	}
	if err := InsertPrompt(ctx, store, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	return prompt
}

func makeResponseForStory(t *testing.T, store *storage.ProtoSQLStorage, ctx context.Context, prompt *models.StoredESMPrompt, userID, optKey string, action models.ESMConsumedAction, attendee bool) {
	t.Helper()
	att := attendee
	row := &models.StoredESMResponse{
		Id:                    uuid.NewString(),
		PromptId:              prompt.Id,
		ExperienceId:          prompt.ExperienceId,
		UserId:                userID,
		ConsumedAction:        action,
		RespondentWasAttendee: &att,
	}
	if optKey != "" {
		key := optKey
		row.ResponseOptionKey = &key
	}
	if err := InsertResponseRows(ctx, store, []*models.StoredESMResponse{row}); err != nil {
		t.Fatalf("insert response: %v", err)
	}
}

func makeUserForStory(t *testing.T, store *storage.ProtoSQLStorage, ctx context.Context, id, name string) {
	t.Helper()
	if _, err := store.Insert(ctx, &models.User{Id: id, Name: name}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestEmbeddedPromptForStory_NoPrompt(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	got, err := EmbeddedPromptForStory(ctx, store, "viewer1", "exp1", time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing prompt, got %+v", got)
	}
}

func TestEmbeddedPromptForStory_ExpiredPrompt(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	makePromptForStory(t, store, ctx, "exp1", "comm1", time.Now().Add(-time.Hour).Unix())

	got, err := EmbeddedPromptForStory(ctx, store, "viewer1", "exp1", time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for expired prompt, got %+v", got)
	}
}

func TestEmbeddedPromptForStory_UnvotedViewer(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	makePromptForStory(t, store, ctx, "exp1", "comm1", time.Now().Add(48*time.Hour).Unix())

	got, err := EmbeddedPromptForStory(ctx, store, "viewer1", "exp1", time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prompt for active prompt + unvoted viewer")
	}
	if got.CurrentResponseOptionKey != nil {
		t.Errorf("expected no current_response_option_key, got %v", got.CurrentResponseOptionKey)
	}
	if got.Question == "" {
		t.Error("expected Question to be populated")
	}
	if len(got.ResponseOptions) != 2 {
		t.Errorf("expected 2 options, got %d", len(got.ResponseOptions))
	}
}

func TestEmbeddedPromptForStory_ViewerAlreadyVoted(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	prompt := makePromptForStory(t, store, ctx, "exp1", "comm1", time.Now().Add(48*time.Hour).Unix())
	makeResponseForStory(t, store, ctx, prompt, "viewer1", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)

	got, err := EmbeddedPromptForStory(ctx, store, "viewer1", "exp1", time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prompt")
	}
	if got.CurrentResponseOptionKey == nil || *got.CurrentResponseOptionKey != "do_again" {
		t.Errorf("expected current_response_option_key=do_again, got %v", got.CurrentResponseOptionKey)
	}
}

func TestSocialProofForStory_FiltersToSameOptionAndExcludesViewer(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	prompt := makePromptForStory(t, store, ctx, "exp1", "comm1", time.Now().Add(48*time.Hour).Unix())

	makeUserForStory(t, store, ctx, "u1", "Alice Aaa")
	makeUserForStory(t, store, ctx, "u2", "Bob Bbb")
	makeUserForStory(t, store, ctx, "u3", "Carol Ccc")
	makeUserForStory(t, store, ctx, "viewer", "Viewer V")

	makeResponseForStory(t, store, ctx, prompt, "viewer", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	makeResponseForStory(t, store, ctx, prompt, "u1", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	makeResponseForStory(t, store, ctx, prompt, "u2", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, false)
	makeResponseForStory(t, store, ctx, prompt, "u3", "not_for_me", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)

	users, remainder, err := SocialProofForStory(ctx, store, "viewer", prompt.Id, "do_again")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if remainder != 0 {
		t.Errorf("expected remainder=0, got %d", remainder)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 social-proof users, got %d", len(users))
	}
	ids := map[string]bool{}
	for _, u := range users {
		ids[u.Id] = true
	}
	if !ids["u1"] || !ids["u2"] {
		t.Errorf("expected u1 and u2 in result, got %+v", ids)
	}
	if ids["u3"] || ids["viewer"] {
		t.Errorf("u3 (different option) and viewer should be excluded, got %+v", ids)
	}
}

func TestSocialProofForStory_RemainderOverflow(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	prompt := makePromptForStory(t, store, ctx, "exp1", "comm1", time.Now().Add(48*time.Hour).Unix())

	for i := 0; i < SocialProofMaxAvatars+2; i++ {
		uid := uuid.NewString()
		makeUserForStory(t, store, ctx, uid, "User "+uid[:4])
		makeResponseForStory(t, store, ctx, prompt, uid, "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	}

	users, remainder, err := SocialProofForStory(ctx, store, "viewer", prompt.Id, "do_again")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(users) != SocialProofMaxAvatars {
		t.Errorf("expected %d users, got %d", SocialProofMaxAvatars, len(users))
	}
	if remainder != 2 {
		t.Errorf("expected remainder=2, got %d", remainder)
	}
}

func TestEmbeddedPromptsForExperiences_BatchedAcrossExperiences(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Now()

	// Three experiences: one with active prompt + viewer voted, one with
	// active prompt + viewer not voted, one with only an expired prompt.
	prompt1 := makePromptForStory(t, store, ctx, "exp1", "comm1", now.Add(48*time.Hour).Unix())
	makeResponseForStory(t, store, ctx, prompt1, "viewer", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)

	makePromptForStory(t, store, ctx, "exp2", "comm1", now.Add(48*time.Hour).Unix())

	makePromptForStory(t, store, ctx, "exp3", "comm1", now.Add(-time.Hour).Unix())

	// A fourth experience id with no prompts at all.
	got, err := EmbeddedPromptsForExperiences(ctx, store, "viewer", []string{"exp1", "exp2", "exp3", "exp4"}, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 active prompts, got %d", len(got))
	}
	if got["exp1"] == nil {
		t.Fatal("expected payload for exp1")
	}
	if got["exp1"].CurrentResponseOptionKey == nil || *got["exp1"].CurrentResponseOptionKey != "do_again" {
		t.Errorf("expected exp1 viewer vote propagated, got %v", got["exp1"].CurrentResponseOptionKey)
	}
	if got["exp2"] == nil {
		t.Fatal("expected payload for exp2")
	}
	if got["exp2"].CurrentResponseOptionKey != nil {
		t.Errorf("exp2 should have no current option, got %v", got["exp2"].CurrentResponseOptionKey)
	}
	if _, ok := got["exp3"]; ok {
		t.Error("exp3 has only an expired prompt; it must be absent from the result")
	}
}

func TestEmbeddedPromptsForExperiences_EmptyInput(t *testing.T) {
	store := setupTestStorage(t)
	got, err := EmbeddedPromptsForExperiences(context.Background(), store, "viewer", nil, time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map for empty input, got %+v", got)
	}
}

func TestSocialProofForPromptsBatch_BatchedAcrossPrompts(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Now()

	prompt1 := makePromptForStory(t, store, ctx, "exp1", "comm1", now.Add(48*time.Hour).Unix())
	prompt2 := makePromptForStory(t, store, ctx, "exp2", "comm1", now.Add(48*time.Hour).Unix())

	makeUserForStory(t, store, ctx, "u1", "Alice")
	makeUserForStory(t, store, ctx, "u2", "Bob")
	makeUserForStory(t, store, ctx, "u3", "Carol")
	makeUserForStory(t, store, ctx, "viewer", "Viewer")

	// Prompt 1: viewer + u1 voted do_again, u2 voted not_for_me.
	makeResponseForStory(t, store, ctx, prompt1, "viewer", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	makeResponseForStory(t, store, ctx, prompt1, "u1", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	makeResponseForStory(t, store, ctx, prompt1, "u2", "not_for_me", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)

	// Prompt 2: viewer + u3 voted do_again.
	makeResponseForStory(t, store, ctx, prompt2, "viewer", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	makeResponseForStory(t, store, ctx, prompt2, "u3", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)

	got, err := SocialProofForPromptsBatch(ctx, store, "viewer", map[string]string{
		prompt1.Id: "do_again",
		prompt2.Id: "do_again",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected social proof for 2 prompts, got %d", len(got))
	}
	p1 := got[prompt1.Id]
	if len(p1.Users) != 1 || p1.Users[0].Id != "u1" {
		t.Errorf("expected prompt1 to surface u1 only, got %+v", p1.Users)
	}
	if p1.Remainder != 0 {
		t.Errorf("expected prompt1 remainder=0, got %d", p1.Remainder)
	}
	p2 := got[prompt2.Id]
	if len(p2.Users) != 1 || p2.Users[0].Id != "u3" {
		t.Errorf("expected prompt2 to surface u3 only, got %+v", p2.Users)
	}
}

func TestSocialProofForPromptsBatch_RemainderAndDedupedUsers(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Now()

	prompt := makePromptForStory(t, store, ctx, "exp1", "comm1", now.Add(48*time.Hour).Unix())
	for i := 0; i < SocialProofMaxAvatars+2; i++ {
		uid := uuid.NewString()
		makeUserForStory(t, store, ctx, uid, "U"+uid[:4])
		makeResponseForStory(t, store, ctx, prompt, uid, "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
	}

	got, err := SocialProofForPromptsBatch(ctx, store, "viewer", map[string]string{
		prompt.Id: "do_again",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	pp, ok := got[prompt.Id]
	if !ok {
		t.Fatal("expected entry for prompt")
	}
	if len(pp.Users) != SocialProofMaxAvatars {
		t.Errorf("expected %d avatars, got %d", SocialProofMaxAvatars, len(pp.Users))
	}
	if pp.Remainder != 2 {
		t.Errorf("expected remainder=2, got %d", pp.Remainder)
	}
}

func TestSocialProofForPromptsBatch_EmptyOptionKeySkipped(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	got, err := SocialProofForPromptsBatch(ctx, store, "viewer", map[string]string{
		"prompt-with-no-vote": "",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map when every input has empty option_key, got %+v", got)
	}
}

func TestEmbeddedPromptsForExperiences_QueryCount(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Now()

	// Seed 30 experiences each with an active prompt; viewer has voted on
	// half of them, so the social-proof batch is also exercised.
	const n = 30
	experienceIDs := make([]string, n)
	promptKeys := make(map[string]string, n)
	for i := 0; i < n; i++ {
		expID := uuid.NewString()
		experienceIDs[i] = expID
		prompt := makePromptForStory(t, store, ctx, expID, "comm1", now.Add(48*time.Hour).Unix())
		if i%2 == 0 {
			makeResponseForStory(t, store, ctx, prompt, "viewer", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, true)
			promptKeys[prompt.Id] = "do_again"
		}
	}

	statsCtx := storage.WithQueryStats(ctx)
	// Two queries: prompts IN (...), then viewer responses IN (chosen prompt ids).
	storage.AssertMaxQueries(t, statsCtx, 2, func() {
		out, err := EmbeddedPromptsForExperiences(statsCtx, store, "viewer", experienceIDs, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(out) != n {
			t.Errorf("expected %d prompts in result, got %d", n, len(out))
		}
	})

	statsCtx2 := storage.WithQueryStats(ctx)
	// Two queries: responses IN (...), then GetByIDs for avatar users (no
	// avatars exist here so the user lookup may return early).
	storage.AssertMaxQueries(t, statsCtx2, 2, func() {
		if _, err := SocialProofForPromptsBatch(statsCtx2, store, "viewer", promptKeys); err != nil {
			t.Fatalf("err: %v", err)
		}
	})
}

func TestAggregateAttendeesOnly_FiltersOutNonAttendees(t *testing.T) {
	rows := []*models.StoredESMResponse{
		{
			PromptId:              "p1",
			UserId:                "u1",
			ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey:     ptr("do_again"),
			RespondentWasAttendee: ptr(true),
		},
		{
			PromptId:              "p1",
			UserId:                "u2",
			ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey:     ptr("do_again"),
			RespondentWasAttendee: ptr(false),
		},
		{
			PromptId:              "p1",
			UserId:                "u3",
			ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey:     ptr("do_again"),
			RespondentWasAttendee: nil,
		},
	}
	signals := AggregateAttendeesOnly(rows, 12345)
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].RespondentCount != 1 {
		t.Errorf("expected RespondentCount=1 (attendee-only), got %d", signals[0].RespondentCount)
	}
}
