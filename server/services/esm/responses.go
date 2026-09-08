package esm

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	esmlib "go.ripls.org/ripls/server/esm"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// RespondToESMPrompt records a recipient's response or dismissal of an ESM
// prompt. The recipient must be a targeted user with an UNRESOLVED row;
// the prompt must not be expired or soft-deleted.
func (s *Service) RespondToESMPrompt(
	ctx context.Context,
	req *connect.Request[api.RespondToESMPromptRequest],
) (*connect.Response[api.RespondToESMPromptResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.PromptId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("prompt_id is required"))
	}

	if !req.Msg.Dismissed && (req.Msg.ResponseOptionKey == nil || *req.Msg.ResponseOptionKey == "") {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("response_option_key is required when not dismissed"))
	}
	if req.Msg.Dismissed && req.Msg.ResponseOptionKey != nil && *req.Msg.ResponseOptionKey != "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("response_option_key and dismissed are mutually exclusive"))
	}

	row, prompt, err := requireESMRespondentOrStoryViewer(ctx, s.storage, authInfo.UserID, req.Msg.PromptId, time.Now())
	if err != nil {
		return nil, err
	}

	if !req.Msg.Dismissed {
		if !optionKeyExists(prompt, *req.Msg.ResponseOptionKey) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("response_option_key %q is not offered by this prompt", *req.Msg.ResponseOptionKey))
		}
	}

	attendee := row.RespondentWasAttendee != nil && *row.RespondentWasAttendee
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RespondToESMPrompt",
		"prompt_id", req.Msg.PromptId,
		"user_id", authInfo.UserID,
		"dismissed", req.Msg.Dismissed,
		"respondent_was_attendee", attendee,
	)

	if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED && req.Msg.Dismissed {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("response already recorded; dismiss is not available after voting"))
	}

	now := time.Now().Unix()
	row.ConsumedAtUnixSec = &now
	if req.Msg.Dismissed {
		row.ConsumedAction = models.ESMConsumedAction_ESM_CONSUMED_ACTION_DISMISSED
	} else {
		row.ConsumedAction = models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED
		row.ResponseOptionKey = req.Msg.ResponseOptionKey
	}

	if err := esmlib.UpdateResponse(ctx, s.storage, row); err != nil {
		return nil, connecterr.Internal(ctx, "RespondToESMPrompt.UpdateResponse", err)
	}

	logger.InfoContext(ctx, "ESM response recorded")
	return connect.NewResponse(&api.RespondToESMPromptResponse{}), nil
}

func optionKeyExists(prompt *models.StoredESMPrompt, key string) bool {
	for _, opt := range prompt.ResponseOptions {
		if opt.Key == key {
			return true
		}
	}
	return false
}
