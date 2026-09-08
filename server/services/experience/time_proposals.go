package experience

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// ProposeTime creates a new time proposal for an experience.
func (s *Service) ProposeTime(
	ctx context.Context,
	req *connect.Request[api.ProposeTimeRequest],
) (*connect.Response[api.ProposeTimeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.Time != nil {
		if err := validateExperienceTimeTimezone(req.Msg.Time); err != nil {
			return nil, err
		}
	}

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "ProposeTime")
	if err != nil {
		return nil, err
	}

	// Check if experience is active or joined
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting time proposals (current state: %s)", expStored.State))
	}

	// Any eligible participant (owner or YES/MAYBE RSVP) may propose times.
	if err := s.requireParticipant(ctx, authInfo.UserID, expStored); err != nil {
		return nil, err
	}
	logger = logger.With("actor_is_owner", expStored.OwnerId == authInfo.UserID)

	// Reject new proposals once the organizer has locked the list. Existing
	// proposals remain votable; the lock is cleared when a fresh poll opens
	// or the time is confirmed.
	if expStored.TimePollActive &&
		expStored.TimeProposalsLocked != nil &&
		*expStored.TimeProposalsLocked {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("time poll is locked; no new proposals can be added"))
	}

	// Resolve which poll this proposal belongs to. If a poll is currently
	// running, reuse its poll_id. Otherwise we're starting a fresh poll —
	// mint a new poll_id so historical proposals from previous polls stay
	// grouped under their own ids and the client can show per-poll history.
	isFirstPollOption := !expStored.TimePollActive
	pollID := ""
	if isFirstPollOption {
		pollID = uuid.New().String()
		expStored.CurrentPollId = proto.String(pollID)
		expStored.TimePollCompleted = proto.Bool(false)
		// Fresh poll resets the per-poll modal state and seeds a default
		// reply-by deadline. The organizer can override via
		// SetTimePollDeadline. Mirrors ProposeLocation.
		expStored.TimePollDeadlineUnixSec = proto.Int64(defaultTimePollDeadline(clock.UnixSec(ctx)))
		expStored.TimeProposalsLocked = nil
	} else if expStored.CurrentPollId != nil {
		pollID = *expStored.CurrentPollId
	}

	// Create the time proposal stamped with the poll_id.
	proposal, err := s.createTimeProposal(ctx, req.Msg.ExperienceId, authInfo.UserID, req.Msg.Time, pollID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create time proposal", "error", err)
		return nil, err
	}

	logger.InfoContext(ctx, "time proposal created", "proposal_id", proposal.Id, "poll_id", pollID)

	// Activate the time poll flag if not already set.
	// Emit the "asking the group" system message only on the first poll option —
	// subsequent options added to an already-active poll produce no message.
	if isFirstPollOption {
		expStored.TimePollActive = true
		if err := s.storage.Update(ctx, expStored); err != nil {
			logger.WarnContext(ctx, "failed to set time_poll_active", "error", err)
			// Non-fatal: proposal was created, poll activation is best-effort.
		}

		if s.systemMessageWriter != nil && expStored.ConversationId != "" {
			displayName := s.getUserDisplayName(ctx, authInfo.UserID)
			if err := s.systemMessageWriter.InsertLocalized(
				ctx,
				expStored.ConversationId,
				authInfo.UserID,
				models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED,
				chat.TimePollOpenedMessage(displayName),
				chat.SystemMessageInsertOptions{PollID: pollID},
			); err != nil {
				logger.ErrorContext(ctx, "failed to write poll-opened system message",
					"conversation_id", expStored.ConversationId,
					"poll_id", pollID,
					"error", err)
			}
		}
	}

	// Build API response with enriched user data
	apiProposal, err := s.buildTimeProposal(ctx, proposal)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build API time proposal", "error", err)
		return nil, err
	}

	return connect.NewResponse(&api.ProposeTimeResponse{
		Proposal: apiProposal,
	}), nil
}

// VoteOnTime records a user's vote on a time proposal.
func (s *Service) VoteOnTime(
	ctx context.Context,
	req *connect.Request[api.VoteOnTimeRequest],
) (*connect.Response[api.VoteOnTimeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"proposal_id", req.Msg.ProposalId,
		"status", req.Msg.Status.String(),
	)

	// Fetch the proposal
	proposalStored := &models.ExperienceTimeProposal{}
	err = s.storage.GetByID(ctx, req.Msg.ProposalId, proposalStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get time proposal", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	expStored, err := s.fetchExperienceForRead(ctx, proposalStored.ExperienceId, logger.Logger, "VoteOnTime")
	if err != nil {
		return nil, err
	}

	// Check if experience is active or joined
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting votes (current state: %s)", expStored.State))
	}

	// Anyone who can see the experience can vote on times (no RSVP required)

	// Check for existing vote
	voteQueryFields := map[string]any{
		"proposal_id": req.Msg.ProposalId,
		"user_id":     authInfo.UserID,
	}
	existingVotes, err := s.storage.QueryByFields(ctx, voteQueryFields, &models.TimeVote{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing vote", "error", err)
		return nil, connecterr.Internal(ctx, "VoteOnTime", err)
	}

	now := clock.UnixSec(ctx)

	if len(existingVotes) > 0 {
		// Update existing vote
		vote := existingVotes[0].(*models.TimeVote)
		vote.Status = models.TimeVoteStatus(req.Msg.Status)
		vote.VotedAtUnixSec = now

		err = s.storage.Update(ctx, vote)
		if err != nil {
			logger.ErrorContext(ctx, "failed to update vote", "error", err)
			return nil, connecterr.Internal(ctx, "VoteOnTime", err)
		}
	} else {
		// Create new vote
		vote := &models.TimeVote{
			ProposalId:     req.Msg.ProposalId,
			UserId:         authInfo.UserID,
			Status:         models.TimeVoteStatus(req.Msg.Status),
			VotedAtUnixSec: now,
		}

		_, err = s.storage.Insert(ctx, vote)
		if err != nil {
			logger.ErrorContext(ctx, "failed to create vote", "error", err)
			return nil, connecterr.Internal(ctx, "VoteOnTime", err)
		}
	}

	logger.InfoContext(ctx, "vote recorded")

	return connect.NewResponse(&api.VoteOnTimeResponse{}), nil
}

// ConfirmTime confirms a time proposal as the final time for the experience.
func (s *Service) ConfirmTime(
	ctx context.Context,
	req *connect.Request[api.ConfirmTimeRequest],
) (*connect.Response[api.ConfirmTimeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"proposal_id", req.Msg.ProposalId,
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "ConfirmTime")
	if err != nil {
		return nil, err
	}

	// Check if user is the owner
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can confirm times"))
	}

	// Check if experience is active or joined
	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting time confirmations (current state: %s)", expStored.State))
	}

	// Fetch the proposal
	proposalStored := &models.ExperienceTimeProposal{}
	err = s.storage.GetByID(ctx, req.Msg.ProposalId, proposalStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get time proposal", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Verify proposal belongs to this experience
	if proposalStored.ExperienceId != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("proposal does not belong to this experience"))
	}

	// Mark proposal as confirmed
	proposalStored.IsConfirmed = true
	err = s.storage.Update(ctx, proposalStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update proposal", "error", err)
		return nil, connecterr.Internal(ctx, "ConfirmTime", err)
	}

	// Update experience time to match the confirmed proposal and clear the poll flag.
	// If a poll was active, mark it as completed so the client can distinguish
	// poll proposals from direct time-set proposals.
	if expStored.TimePollActive {
		expStored.TimePollCompleted = proto.Bool(true)
	}
	expStored.Time = proposalStored.Time
	expStored.TimePollActive = false
	// Clear the per-poll modal state — Lock + Deadline only apply while a
	// poll is active. Mirrors the equivalent ConfirmLocation behavior.
	expStored.TimePollDeadlineUnixSec = nil
	expStored.TimeProposalsLocked = nil
	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update experience time", "error", err)
		return nil, connecterr.Internal(ctx, "ConfirmTime", err)
	}

	// Unconfirm all other proposals for this experience.
	unconfirmStart := time.Now()
	allProposals, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceTimeProposal{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query other proposals", "error", err)
	} else {
		for _, msg := range allProposals {
			proposal := msg.(*models.ExperienceTimeProposal)
			if proposal.Id != req.Msg.ProposalId && proposal.IsConfirmed {
				proposal.IsConfirmed = false
				if err := s.storage.Update(ctx, proposal); err != nil {
					logger.WarnContext(ctx, "failed to unconfirm other proposal",
						"other_proposal_id", proposal.Id,
						"error", err)
				}
			}
		}
	}
	logger.DebugContext(ctx, "unconfirmed other proposals", "duration_ms", time.Since(unconfirmStart).Milliseconds())

	logger.InfoContext(ctx, "time confirmed")

	// Emit DETAIL_CHANGED system message to the experience conversation.
	if s.systemMessageWriter != nil && expStored.ConversationId != "" {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED,
			chat.TimeConfirmedMessage(displayName, experienceTimeParam(expStored.Time)),
		); err != nil {
			logger.WarnContext(ctx, "failed to write time-confirmed system message",
				"conversation_id", expStored.ConversationId,
				"error", err)
		}
	}

	return connect.NewResponse(&api.ConfirmTimeResponse{}), nil
}

// UnlockTime unlocks a previously confirmed time for the experience.
func (s *Service) UnlockTime(
	ctx context.Context,
	req *connect.Request[api.UnlockTimeRequest],
) (*connect.Response[api.UnlockTimeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UnlockTime",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	// Validate experience_id is non-empty
	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "UnlockTime")
	if err != nil {
		return nil, err
	}

	// Check if user is the owner
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can unlock times"))
	}

	// Query all proposals for this experience
	allProposals, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceTimeProposal{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query proposals", "error", err)
		return nil, connecterr.Internal(ctx, "UnlockTime", err)
	}

	// Unconfirm all confirmed proposals
	unlockedCount := 0
	for _, msg := range allProposals {
		proposal := msg.(*models.ExperienceTimeProposal)
		if proposal.IsConfirmed {
			proposal.IsConfirmed = false
			if err := s.storage.Update(ctx, proposal); err != nil {
				logger.ErrorContext(ctx, "failed to unlock proposal",
					"proposal_id", proposal.Id,
					"error", err)
				return nil, connecterr.Internal(ctx, "UnlockTime", err)
			}
			unlockedCount++
		}
	}

	logger.InfoContext(ctx, "time unlocked", "unlocked_count", unlockedCount)

	return connect.NewResponse(&api.UnlockTimeResponse{}), nil
}

// buildTimeProposal builds an API TimeProposal from a stored ExperienceTimeProposal with enriched user data.
func (s *Service) buildTimeProposal(ctx context.Context, proposal *models.ExperienceTimeProposal) (*api.TimeProposal, error) {
	logger := logging.LoggerWithContext(ctx).With("proposal_id", proposal.Id)

	// Get proposer details
	proposer, err := services.FetchAPIUser(ctx, s.storage, proposal.ProposedByUserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get proposer user", "error", err)
		return nil, connecterr.Internal(ctx, "buildTimeProposal", fmt.Errorf("failed to load proposer information"))
	}

	// Get votes for this proposal
	voteMessages, err := s.storage.QueryByField(ctx, "proposal_id", proposal.Id, &models.TimeVote{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query votes", "error", err)
		return nil, connecterr.Internal(ctx, "buildTimeProposal", err)
	}

	// Build votes with user details
	votes := make([]*api.TimeVote, 0, len(voteMessages))
	for _, msg := range voteMessages {
		vote := msg.(*models.TimeVote)

		// Fetch user details
		user, err := services.FetchAPIUser(ctx, s.storage, vote.UserId)
		if err != nil {
			logger.WarnContext(ctx, "failed to get user for vote", "user_id", vote.UserId, "error", err)
			continue // Skip this vote rather than failing the whole request
		}

		votes = append(votes, &api.TimeVote{
			User:   user,
			Status: api.TimeVoteStatus(vote.Status),
		})
	}

	return &api.TimeProposal{
		Id:                proposal.Id,
		ProposedBy:        proposer,
		Time:              services.ConvertTimeModelsToAPI(proposal.Time),
		Votes:             votes,
		IsConfirmed:       proposal.IsConfirmed,
		ProposedAtUnixSec: proposal.ProposedAtUnixSec,
		PollId:            proposal.PollId,
	}, nil
}

// CancelTimePoll ends the active time poll for an experience.
//
// Clears time_poll_active, marks the poll as completed, and emits a system
// message. Proposals and their votes are preserved so anyone can still open
// the read-only results view after the poll ends. Only the experience owner
// can end a poll.
func (s *Service) CancelTimePoll(
	ctx context.Context,
	req *connect.Request[api.CancelTimePollRequest],
) (*connect.Response[api.CancelTimePollResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CancelTimePoll",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "CancelTimePoll")
	if err != nil {
		return nil, err
	}

	// Only the owner can end a poll
	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can cancel a time poll"))
	}

	// Clear the time_poll_active flag and mark poll as completed.
	// Per-poll modal state (lock, deadline) only applies while the poll is
	// active — clear both so a fresh poll starts from defaults.
	expStored.TimePollActive = false
	expStored.TimePollCompleted = proto.Bool(true)
	expStored.TimePollDeadlineUnixSec = nil
	expStored.TimeProposalsLocked = nil
	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to clear time_poll_active", "error", err)
		return nil, connecterr.Internal(ctx, "CancelTimePoll", err)
	}

	logger.InfoContext(ctx, "time poll ended")

	// Emit TIME_POLL_CANCELLED system message to the experience conversation.
	if s.systemMessageWriter != nil && expStored.ConversationId != "" {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_POLL_CANCELLED,
			chat.TimePollCancelledMessage(displayName),
		); err != nil {
			logger.WarnContext(ctx, "failed to write TIME_POLL_CANCELLED system message",
				"conversation_id", expStored.ConversationId,
				"error", err)
		}
	}

	return connect.NewResponse(&api.CancelTimePollResponse{}), nil
}

// DeleteTimeProposal removes a single proposal (and its votes) from a time
// poll. Owner can delete any proposal; participants can delete only their
// own. If the deleted proposal was the last one in the active poll, the
// poll is also cleared so the experience returns to a fresh no-poll state.
// timeProposalSpec binds the shared deletion logic to the time proposal's
// proto types and experience fields.
var timeProposalSpec = proposalSpec{
	logIDField: "time_proposal_id",
	kind:       "time",

	newProposal: func() proto.Message { return &models.ExperienceTimeProposal{} },
	proposalFields: func(m proto.Message) (string, string, *string) {
		p := m.(*models.ExperienceTimeProposal)
		return p.ExperienceId, p.ProposedByUserId, p.PollId
	},
	newVote: func() proto.Message { return &models.TimeVote{} },
	voteID:  func(m proto.Message) string { return m.(*models.TimeVote).Id },

	pollActive:    func(e *models.Experience) bool { return e.TimePollActive },
	currentPollID: func(e *models.Experience) *string { return e.CurrentPollId },
	clearPoll: func(e *models.Experience) {
		e.TimePollActive = false
		e.TimePollCompleted = nil
		e.CurrentPollId = nil
		e.TimePollDeadlineUnixSec = nil
		e.TimeProposalsLocked = nil
	},
}

func (s *Service) DeleteTimeProposal(
	ctx context.Context,
	req *connect.Request[api.DeleteTimeProposalRequest],
) (*connect.Response[api.DeleteTimeProposalResponse], error) {
	if err := s.deletePollProposal(ctx, req.Msg.ExperienceId, req.Msg.ProposalId,
		"DeleteTimeProposal", timeProposalSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.DeleteTimeProposalResponse{}), nil
}

// buildTimeProposalsFromMaps assembles TimeProposals for one experience from pre-fetched maps.
// No database calls are made; all dependency data is read from the provided maps.
// Votes whose proposer or voter is absent from userMap are skipped with a warn log.
// Proposals are returned in the same order they appear in proposalsByExp.
func buildTimeProposalsFromMaps(
	ctx context.Context,
	experienceID string,
	proposalsByExp map[string][]*models.ExperienceTimeProposal,
	votesByProposal map[string][]*models.TimeVote,
	userMap map[string]*api.User,
	logger *logging.Logger,
) []*api.TimeProposal {
	rawProposals := proposalsByExp[experienceID]
	proposals := make([]*api.TimeProposal, 0, len(rawProposals))
	for _, proposal := range rawProposals {
		proposer := userMap[proposal.ProposedByUserId]
		if proposer == nil {
			logger.WarnContext(ctx, "proposer not found in user map, skipping proposal",
				"proposal_id", proposal.Id, "user_id", proposal.ProposedByUserId)
			continue
		}

		rawVotes := votesByProposal[proposal.Id]
		votes := make([]*api.TimeVote, 0, len(rawVotes))
		for _, vote := range rawVotes {
			voter := userMap[vote.UserId]
			if voter == nil {
				logger.WarnContext(ctx, "voter not found in user map, skipping vote",
					"proposal_id", proposal.Id, "user_id", vote.UserId)
				continue
			}
			votes = append(votes, &api.TimeVote{
				User:   voter,
				Status: api.TimeVoteStatus(vote.Status),
			})
		}

		proposals = append(proposals, &api.TimeProposal{
			Id:                proposal.Id,
			ProposedBy:        proposer,
			Time:              services.ConvertTimeModelsToAPI(proposal.Time),
			Votes:             votes,
			IsConfirmed:       proposal.IsConfirmed,
			ProposedAtUnixSec: proposal.ProposedAtUnixSec,
			PollId:            proposal.PollId,
		})
	}
	return proposals
}

// defaultTimePollDeadline returns the reply-by deadline (unix seconds) to
// seed on the first proposal of a fresh poll: 24 hours from now. Mirrors
// defaultLocationPollDeadline.
func defaultTimePollDeadline(now int64) int64 {
	const dayInSeconds = int64(24 * time.Hour / time.Second)
	return now + dayInSeconds
}

// createTimeProposal creates a new time proposal record in storage.
// Returns the created proposal with its ID set. [pollID] tags the proposal as
// belonging to a particular poll run; pass an empty string to leave it unset
// (e.g. for legacy callers / tests that don't care about poll grouping).
func (s *Service) createTimeProposal(ctx context.Context, experienceID, proposedByUserID string, apiTime *api.ExperienceTime, pollID string) (*models.ExperienceTimeProposal, error) {
	now := clock.UnixSec(ctx)
	proposal := &models.ExperienceTimeProposal{
		ExperienceId:      experienceID,
		ProposedByUserId:  proposedByUserID,
		Time:              convertTimeAPIToModels(apiTime),
		ProposedAtUnixSec: now,
		IsConfirmed:       false,
	}
	if pollID != "" {
		proposal.PollId = proto.String(pollID)
	}

	proposalID, err := s.storage.Insert(ctx, proposal)
	if err != nil {
		return nil, connecterr.Internal(ctx, "createTimeProposal", err, "detail",

			// Set the ID on the proposal object
			"failed to insert time proposal")
	}

	proposal.Id = proposalID

	return proposal, nil
}
