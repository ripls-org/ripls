package esm

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	esmlib "go.ripls.org/ripls/server/esm"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// requireESMRespondentOrStoryViewer verifies that the calling user is
// allowed to submit a response to the prompt, accommodating both
// pre-seeded attendee rows (the #1580 path) and lazy non-attendee rows
// inserted on first vote (the Story-embedded path).
//
// On success the caller receives a StoredESMResponse row that has been
// either retrieved (attendee with an UNRESOLVED row), retrieved as
// already-RESPONDED for a vote-change submission, or constructed and
// inserted in place (non-attendee community member who can see the
// Story). The handler is responsible for mutating the row's
// consumed_action / response_option_key fields and persisting via
// UpdateResponse.
//
// Authorization rules in priority order:
//   - Prompt must exist, not be soft-deleted, not be expired.
//   - If the caller has a row for this prompt (UNRESOLVED or RESPONDED)
//     they may submit/change their response.
//   - Otherwise, the caller must be a member of the prompt's community
//     so that the Story is in their feed; a fresh row is inserted with
//     respondent_was_attendee = false.
func requireESMRespondentOrStoryViewer(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, promptID string,
	now time.Time,
) (*models.StoredESMResponse, *models.StoredESMPrompt, error) {
	prompt, err := esmlib.GetPrompt(ctx, store, promptID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("prompt not found"))
	}
	if prompt.Deleted != nil {
		return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("prompt has been retracted"))
	}
	if prompt.ClosesAtUnixSec <= now.Unix() {
		return nil, nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "esm_prompt_expired", "this prompt has expired", nil)
	}

	row, err := esmlib.GetResponseForUser(ctx, store, userID, promptID)
	if err != nil {
		return nil, nil, connecterr.Internal(ctx, "requireESMRespondentOrStoryViewer.GetResponseForUser", err)
	}
	if row != nil {
		return row, prompt, nil
	}

	if prompt.CommunityId == "" {
		return nil, nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not a recipient of this prompt"))
	}
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, store, prompt.CommunityId, userID); err != nil {
		return nil, nil, err
	}

	attendee := false
	row = &models.StoredESMResponse{
		Id:                    uuid.NewString(),
		PromptId:              prompt.Id,
		ExperienceId:          prompt.ExperienceId,
		UserId:                userID,
		ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED,
		RespondentWasAttendee: &attendee,
	}
	if err := esmlib.InsertResponseRows(ctx, store, []*models.StoredESMResponse{row}); err != nil {
		return nil, nil, connecterr.Internal(ctx, "requireESMRespondentOrStoryViewer.InsertResponseRows", err)
	}
	return row, prompt, nil
}
