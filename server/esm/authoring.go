package esm

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// DefaultPromptWindowSeconds is the default closes_at_unix_sec offset from
// experience.completed_at_unix_sec at prompt-authoring time: 48 hours.
const DefaultPromptWindowSeconds int64 = 48 * 3600

// AttendeeUserIDs returns the user IDs of every ExperienceRSVP row for the
// given experience whose attended status is YES. The recap-story
// materialization in `server/services/experience/` uses this to seed the
// initial recipient set; v2's automated trigger calls the same helper.
//
// provisional-user RSVPs (no real user_id) are skipped — only users who have an
// account can be pre-seeded as recipients.
func AttendeeUserIDs(ctx context.Context, store *storage.ProtoSQLStorage, experienceID string) ([]string, error) {
	rsvps, err := storage.QueryByFields[*models.ExperienceRSVP](store, ctx, map[string]any{
		"experience_id": experienceID,
		"attended":      int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	})
	if err != nil {
		return nil, fmt.Errorf("esm.AttendeeUserIDs: %w", err)
	}
	out := make([]string, 0, len(rsvps))
	for _, r := range rsvps {
		if r.UserId == "" {
			continue
		}
		out = append(out, r.UserId)
	}
	return out, nil
}

// PromptDraft is the structured input that the recap-story materialization
// (and, in v2, the automated trigger) hands to BuildPromptForExperience.
type PromptDraft struct {
	// Optional: when zero, defaults to experience.completed_at_unix_sec
	// + DefaultPromptWindowSeconds.
	ClosesAtUnixSec int64
	Options         []*models.ESMResponseOption
	Recipients      []string
	Question        string
	CreatedByUserID string
}

// BuildPromptForExperience constructs (but does not insert) a StoredESMPrompt
// and one StoredESMResponse row per recipient with consumed_action=UNRESOLVED.
//
// Pure function: no storage writes. The caller (the recap-story
// materialization today, the v2 trigger later) is responsible for the
// actual insert.
//
// Returns an error when the input is invalid (missing question, empty
// option list, empty recipient list) or when the experience cannot be
// loaded from storage (the experience is needed to compute the default
// window and the community_id field on the prompt).
func BuildPromptForExperience(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	experienceID string,
	draft PromptDraft,
	now time.Time,
) (*models.StoredESMPrompt, []*models.StoredESMResponse, error) {
	if err := validateDraft(draft); err != nil {
		return nil, nil, err
	}

	experience := &models.Experience{}
	if err := store.GetByID(ctx, experienceID, experience); err != nil {
		return nil, nil, fmt.Errorf("esm.BuildPromptForExperience: load experience: %w", err)
	}

	closesAt := draft.ClosesAtUnixSec
	if closesAt == 0 {
		base := now.Unix()
		if experience.CompletedAtUnixSec != nil {
			base = *experience.CompletedAtUnixSec
		}
		closesAt = base + DefaultPromptWindowSeconds
	}

	prompt := &models.StoredESMPrompt{
		Id:               uuid.NewString(),
		ExperienceId:     experienceID,
		CommunityId:      communityIDForExperience(ctx, store, experience),
		Question:         draft.Question,
		ResponseOptions:  draft.Options,
		CreatedAtUnixSec: now.Unix(),
		ClosesAtUnixSec:  closesAt,
		CreatedByUserId:  draft.CreatedByUserID,
	}

	// Recipients passed by the recap-story materialization are already
	// restricted to ExperienceRSVP.attended == YES via AttendeeUserIDs, so
	// every pre-seeded row marks the respondent as an attendee. Non-attendee
	// rows are inserted lazily when a non-attendee Story viewer submits a
	// response — see services/esm/responses.go.
	attendeeFlag := true
	responses := make([]*models.StoredESMResponse, 0, len(draft.Recipients))
	for _, userID := range draft.Recipients {
		responses = append(responses, &models.StoredESMResponse{
			Id:                    uuid.NewString(),
			PromptId:              prompt.Id,
			ExperienceId:          experienceID,
			UserId:                userID,
			ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED,
			RespondentWasAttendee: &attendeeFlag,
		})
	}
	return prompt, responses, nil
}

func validateDraft(draft PromptDraft) error {
	if draft.Question == "" {
		return fmt.Errorf("esm.BuildPromptForExperience: question is required")
	}
	if len(draft.Options) < 2 {
		return fmt.Errorf("esm.BuildPromptForExperience: at least 2 response options are required")
	}
	if len(draft.Options) > 4 {
		return fmt.Errorf("esm.BuildPromptForExperience: at most 4 response options are allowed in v1")
	}
	if len(draft.Recipients) == 0 {
		return fmt.Errorf("esm.BuildPromptForExperience: recipient list must not be empty")
	}
	for _, opt := range draft.Options {
		if opt.Key == "" || opt.Label == "" {
			return fmt.Errorf("esm.BuildPromptForExperience: every response option needs a key and a label")
		}
	}
	return nil
}

// communityIDForExperience returns the experience's primary community
// context. The Experience model itself doesn't carry community_id directly
// (it lives on CommunityExperience join rows), so we resolve it best-effort:
// the first CommunityExperience row that references the experience, or
// empty if none. The query is lightweight and only runs once per prompt
// creation.
func communityIDForExperience(ctx context.Context, store *storage.ProtoSQLStorage, experience *models.Experience) string {
	rows, err := storage.QueryByFields[*models.CommunityExperience](store, ctx, map[string]any{
		"experience_id": experience.Id,
	})
	if err != nil || len(rows) == 0 {
		return ""
	}
	return rows[0].CommunityId
}
