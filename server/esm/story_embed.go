package esm

import (
	"context"
	"fmt"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// SocialProofMaxAvatars caps the number of co-respondent users surfaced
// inline next to a story's voted state. Excess respondents are reported
// via the remainder count returned alongside the slice.
const SocialProofMaxAvatars = 3

// EmbeddedPromptForStory builds the inline voting payload rendered at the
// bottom of a STORY_TYPE_EXPERIENCE_CONCLUDED story.
//
// Returns nil with no error when no prompt exists for the experience,
// when every prompt is soft-deleted or expired, or when the experience
// itself has no canonical prompt to embed. Selects the first non-deleted,
// non-expired prompt for the experience — v1 ships at most one prompt
// per concluded story.
//
// When the viewer already has a StoredESMResponse row for the prompt,
// the returned payload's CurrentResponseOptionKey is populated so the
// client can render the voted state directly.
func EmbeddedPromptForStory(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	viewerUserID, experienceID string,
	now time.Time,
) (*api.EmbeddedEsmPrompt, error) {
	prompts, err := QueryPromptsByExperience(ctx, store, experienceID)
	if err != nil {
		return nil, fmt.Errorf("esm.EmbeddedPromptForStory: query prompts: %w", err)
	}
	prompt := pickActivePrompt(prompts, now)
	if prompt == nil {
		return nil, nil
	}

	row, err := GetResponseForUser(ctx, store, viewerUserID, prompt.Id)
	if err != nil {
		return nil, fmt.Errorf("esm.EmbeddedPromptForStory: get response: %w", err)
	}

	payload := &api.EmbeddedEsmPrompt{
		PromptId:        prompt.Id,
		Question:        prompt.Question,
		ResponseOptions: toEmbeddedOptions(prompt.ResponseOptions),
		ClosesAtUnixSec: prompt.ClosesAtUnixSec,
	}
	if row != nil &&
		row.ConsumedAction == models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED &&
		row.ResponseOptionKey != nil &&
		*row.ResponseOptionKey != "" {
		key := *row.ResponseOptionKey
		payload.CurrentResponseOptionKey = &key
	}
	return payload, nil
}

// PromptSocialProof carries the avatar slice and overflow remainder for
// a single prompt's social-proof block. Returned by SocialProofForPromptsBatch
// so the feed enrichment path can populate per-story payloads without a
// per-story query.
type PromptSocialProof struct {
	Users     []*models.User
	Remainder int32
}

// EmbeddedPromptsForExperiences is the batched analogue of
// EmbeddedPromptForStory. Given N experience IDs, it loads every prompt
// and the viewer's response rows in two queries total (vs 2*N) and
// returns the inline voting payload for each experience that has an
// active prompt.
//
// Experiences with no active prompt are absent from the returned map.
// Active-prompt selection follows EmbeddedPromptForStory: soft-deleted
// and expired prompts are skipped; the first remaining prompt wins.
//
// CurrentResponseOptionKey is populated on each payload when the viewer
// has a RESPONDED row for the chosen prompt.
func EmbeddedPromptsForExperiences(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	viewerUserID string,
	experienceIDs []string,
	now time.Time,
) (map[string]*api.EmbeddedEsmPrompt, error) {
	if len(experienceIDs) == 0 {
		return nil, nil
	}
	prompts, err := storage.QueryByFieldIn[*models.StoredESMPrompt](store, ctx, "experience_id", experienceIDs)
	if err != nil {
		return nil, fmt.Errorf("esm.EmbeddedPromptsForExperiences: query prompts: %w", err)
	}
	if len(prompts) == 0 {
		return nil, nil
	}

	byExperience := make(map[string][]*models.StoredESMPrompt, len(experienceIDs))
	for _, p := range prompts {
		byExperience[p.ExperienceId] = append(byExperience[p.ExperienceId], p)
	}
	chosen := make(map[string]*models.StoredESMPrompt, len(byExperience))
	chosenIDs := make([]string, 0, len(byExperience))
	for expID, ps := range byExperience {
		active := pickActivePrompt(ps, now)
		if active == nil {
			continue
		}
		chosen[expID] = active
		chosenIDs = append(chosenIDs, active.Id)
	}
	if len(chosen) == 0 {
		return nil, nil
	}

	rows, err := storage.QueryByFieldIn[*models.StoredESMResponse](store, ctx, "prompt_id", chosenIDs)
	if err != nil {
		return nil, fmt.Errorf("esm.EmbeddedPromptsForExperiences: query responses: %w", err)
	}
	viewerByPrompt := make(map[string]*models.StoredESMResponse, len(chosenIDs))
	for _, row := range rows {
		if row.UserId != viewerUserID {
			continue
		}
		viewerByPrompt[row.PromptId] = row
	}

	out := make(map[string]*api.EmbeddedEsmPrompt, len(chosen))
	for expID, prompt := range chosen {
		payload := &api.EmbeddedEsmPrompt{
			PromptId:        prompt.Id,
			Question:        prompt.Question,
			ResponseOptions: toEmbeddedOptions(prompt.ResponseOptions),
			ClosesAtUnixSec: prompt.ClosesAtUnixSec,
		}
		if row, ok := viewerByPrompt[prompt.Id]; ok &&
			row.ConsumedAction == models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED &&
			row.ResponseOptionKey != nil &&
			*row.ResponseOptionKey != "" {
			key := *row.ResponseOptionKey
			payload.CurrentResponseOptionKey = &key
		}
		out[expID] = payload
	}
	return out, nil
}

// SocialProofForPromptsBatch is the batched analogue of SocialProofForStory.
// Given a (prompt_id → viewer's option_key) map, it loads every relevant
// response row in one query and resolves all unique avatar users with one
// GetByIDs, returning the per-prompt avatar slice + remainder.
//
// Prompts whose option_key is empty are skipped (mirrors SocialProofForStory's
// no-vote short-circuit). Prompts with no co-respondents on the viewer's
// chosen option are absent from the returned map.
//
// Privacy contract: surfaces respondents only for the viewer's chosen
// option per prompt. Opposing votes are never disclosed.
func SocialProofForPromptsBatch(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	viewerUserID string,
	promptKeys map[string]string,
) (map[string]PromptSocialProof, error) {
	if len(promptKeys) == 0 {
		return nil, nil
	}
	promptIDs := make([]string, 0, len(promptKeys))
	for id, key := range promptKeys {
		if key == "" {
			continue
		}
		promptIDs = append(promptIDs, id)
	}
	if len(promptIDs) == 0 {
		return nil, nil
	}

	rows, err := storage.QueryByFieldIn[*models.StoredESMResponse](store, ctx, "prompt_id", promptIDs)
	if err != nil {
		return nil, fmt.Errorf("esm.SocialProofForPromptsBatch: query responses: %w", err)
	}

	matchingByPrompt := make(map[string][]string, len(promptIDs))
	for _, row := range rows {
		if row.UserId == viewerUserID {
			continue
		}
		if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED {
			continue
		}
		if row.ResponseOptionKey == nil {
			continue
		}
		wantKey := promptKeys[row.PromptId]
		if wantKey == "" || *row.ResponseOptionKey != wantKey {
			continue
		}
		matchingByPrompt[row.PromptId] = append(matchingByPrompt[row.PromptId], row.UserId)
	}
	if len(matchingByPrompt) == 0 {
		return nil, nil
	}

	type promptAvatars struct {
		ids       []string
		remainder int32
	}
	perPrompt := make(map[string]promptAvatars, len(matchingByPrompt))
	avatarSet := make(map[string]struct{})
	for pid, ids := range matchingByPrompt {
		pa := promptAvatars{}
		if len(ids) > SocialProofMaxAvatars {
			pa.ids = ids[:SocialProofMaxAvatars]
			pa.remainder = int32(len(ids) - SocialProofMaxAvatars)
		} else {
			pa.ids = ids
		}
		perPrompt[pid] = pa
		for _, id := range pa.ids {
			avatarSet[id] = struct{}{}
		}
	}
	avatarIDs := make([]string, 0, len(avatarSet))
	for id := range avatarSet {
		avatarIDs = append(avatarIDs, id)
	}

	usersByID, err := storage.GetByIDs[*models.User](store, ctx, avatarIDs)
	if err != nil {
		return nil, fmt.Errorf("esm.SocialProofForPromptsBatch: load users: %w", err)
	}

	out := make(map[string]PromptSocialProof, len(perPrompt))
	for pid, pa := range perPrompt {
		users := make([]*models.User, 0, len(pa.ids))
		for _, id := range pa.ids {
			if u, ok := usersByID[id]; ok {
				users = append(users, u)
			}
		}
		out[pid] = PromptSocialProof{Users: users, Remainder: pa.remainder}
	}
	return out, nil
}

// SocialProofForStory returns up to SocialProofMaxAvatars users (other
// than the viewer) who responded to the prompt with the given option
// key, plus a remainder count for the overflow.
//
// When the viewer has not yet responded — or selected a different option
// — the caller should pass an empty optionKey and skip this lookup. The
// helper returns no users and a zero remainder when no co-respondents
// exist.
//
// Privacy contract: the helper surfaces respondents only for the viewer's
// chosen option. Opposing votes are never disclosed; the per-user
// attribution exposed here is limited to community-level co-membership
// already implied by the story's audience.
func SocialProofForStory(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	viewerUserID, promptID, optionKey string,
) ([]*models.User, int32, error) {
	if optionKey == "" {
		return nil, 0, nil
	}
	rows, err := QueryResponsesByPrompt(ctx, store, promptID)
	if err != nil {
		return nil, 0, fmt.Errorf("esm.SocialProofForStory: query responses: %w", err)
	}

	matchingUserIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.UserId == viewerUserID {
			continue
		}
		if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED {
			continue
		}
		if row.ResponseOptionKey == nil || *row.ResponseOptionKey != optionKey {
			continue
		}
		matchingUserIDs = append(matchingUserIDs, row.UserId)
	}
	totalMatching := len(matchingUserIDs)
	if totalMatching == 0 {
		return nil, 0, nil
	}

	avatarIDs := matchingUserIDs
	remainder := int32(0)
	if totalMatching > SocialProofMaxAvatars {
		avatarIDs = matchingUserIDs[:SocialProofMaxAvatars]
		remainder = int32(totalMatching - SocialProofMaxAvatars)
	}

	usersByID, err := storage.GetByIDs[*models.User](store, ctx, avatarIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("esm.SocialProofForStory: load users: %w", err)
	}
	users := make([]*models.User, 0, len(avatarIDs))
	for _, id := range avatarIDs {
		if u, ok := usersByID[id]; ok {
			users = append(users, u)
		}
	}
	return users, remainder, nil
}

// pickActivePrompt returns the first non-deleted, non-expired prompt
// from the given slice, or nil when every prompt is unusable.
func pickActivePrompt(prompts []*models.StoredESMPrompt, now time.Time) *models.StoredESMPrompt {
	nowSec := now.Unix()
	for _, p := range prompts {
		if p.Deleted != nil {
			continue
		}
		if p.ClosesAtUnixSec > 0 && p.ClosesAtUnixSec <= nowSec {
			continue
		}
		return p
	}
	return nil
}

func toEmbeddedOptions(stored []*models.ESMResponseOption) []*api.EmbeddedEsmOption {
	out := make([]*api.EmbeddedEsmOption, len(stored))
	for i, o := range stored {
		out[i] = &api.EmbeddedEsmOption{Key: o.Key, Label: o.Label}
	}
	return out
}
