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

// ProposeLocation creates a new location proposal for an experience.
//
// The proposal carries either a saved Location reference (location_id) or an
// inline GeocodedLocation (drop-pin / unsaved search result). When called and
// no poll is currently running, it mints a fresh poll_id and flips
// location_poll_active to true. Subsequent proposals during the same poll
// reuse the existing poll_id and do not emit a duplicate "poll opened" system
// message.
func (s *Service) ProposeLocation(
	ctx context.Context,
	req *connect.Request[api.ProposeLocationRequest],
) (*connect.Response[api.ProposeLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ProposeLocation",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}
	if req.Msg.Location == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("location is required"))
	}
	if req.Msg.Location.LocationId == "" && req.Msg.Location.Geocoded == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("location must specify either location_id or geocoded"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience for location proposal", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting location proposals (current state: %s)", expStored.State))
	}

	// Any authenticated user can propose a spot — a viewer's eventual
	// attendance may be predicated on the location choice, so RSVP status
	// is not a prerequisite. Other location-poll actions (vote, confirm,
	// lock) keep their participant / owner gates.
	logger = logger.With("actor_is_owner", expStored.OwnerId == authInfo.UserID)

	// Reject new proposals once the organizer has locked the list. Existing
	// proposals remain votable; the lock is cleared when a fresh poll opens
	// or the location is confirmed.
	if expStored.LocationPollActive &&
		expStored.LocationProposalsLocked != nil &&
		*expStored.LocationProposalsLocked {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("location poll is locked; no new proposals can be added"))
	}

	// Resolve which poll this proposal belongs to. Fresh poll → new poll_id;
	// existing poll → reuse current_location_poll_id.
	isFirstPollOption := !expStored.LocationPollActive
	pollID := ""
	if isFirstPollOption {
		pollID = uuid.New().String()
		expStored.CurrentLocationPollId = proto.String(pollID)
		expStored.LocationPollCompleted = proto.Bool(false)
		// Fresh poll resets the per-poll modal state and seeds a default
		// reply-by deadline. The organizer can override via
		// SetLocationPollDeadline.
		expStored.LocationPollDeadlineUnixSec = proto.Int64(defaultLocationPollDeadline(clock.UnixSec(ctx)))
		expStored.LocationProposalsLocked = nil
	} else if expStored.CurrentLocationPollId != nil {
		pollID = *expStored.CurrentLocationPollId
	}

	proposal, err := s.createLocationProposal(ctx, req.Msg.ExperienceId, authInfo.UserID, req.Msg.Location, pollID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create location proposal", "error", err)
		return nil, err
	}

	logger.InfoContext(ctx, "location proposal created",
		"location_proposal_id", proposal.Id,
		"location_poll_id", pollID)

	// Activate the location poll flag on the first option of a fresh poll
	// and emit a single "poll opened" system message. Sequential client-side
	// submission pairs with this guarantee.
	if isFirstPollOption {
		expStored.LocationPollActive = true
		if err := s.storage.Update(ctx, expStored); err != nil {
			logger.WarnContext(ctx, "failed to set location_poll_active", "error", err)
		}

		if s.systemMessageWriter != nil && expStored.ConversationId != "" {
			displayName := s.getUserDisplayName(ctx, authInfo.UserID)
			if err := s.systemMessageWriter.InsertLocalized(
				ctx,
				expStored.ConversationId,
				authInfo.UserID,
				models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOCATION_PROPOSED,
				chat.LocationPollOpenedMessage(displayName),
				chat.SystemMessageInsertOptions{PollID: pollID},
			); err != nil {
				logger.ErrorContext(ctx, "failed to write location-poll-opened system message",
					"conversation_id", expStored.ConversationId,
					"location_poll_id", pollID,
					"error", err)
			}
		}
	}

	apiProposal, err := s.buildLocationProposal(ctx, proposal)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build API location proposal", "error", err)
		return nil, err
	}

	return connect.NewResponse(&api.ProposeLocationResponse{
		Proposal: apiProposal,
	}), nil
}

// VoteOnLocation records a user's vote on a location proposal.
func (s *Service) VoteOnLocation(
	ctx context.Context,
	req *connect.Request[api.VoteOnLocationRequest],
) (*connect.Response[api.VoteOnLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "VoteOnLocation",
		"user_id", authInfo.UserID,
		"location_proposal_id", req.Msg.ProposalId,
		"status", req.Msg.Status.String(),
	)

	if req.Msg.ProposalId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("proposal_id is required"))
	}

	proposalStored := &models.ExperienceLocationProposal{}
	if err := s.storage.GetByID(ctx, req.Msg.ProposalId, proposalStored); err != nil {
		logger.ErrorContext(ctx, "failed to get location proposal", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, proposalStored.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting votes (current state: %s)", expStored.State))
	}

	voteQueryFields := map[string]any{
		"proposal_id": req.Msg.ProposalId,
		"user_id":     authInfo.UserID,
	}
	existingVotes, err := s.storage.QueryByFields(ctx, voteQueryFields, &models.LocationVote{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing vote", "error", err)
		return nil, connecterr.Internal(ctx, "VoteOnLocation", err)
	}

	now := clock.UnixSec(ctx)

	if len(existingVotes) > 0 {
		vote := existingVotes[0].(*models.LocationVote)
		vote.Status = models.LocationVoteStatus(req.Msg.Status)
		vote.VotedAtUnixSec = now
		if err := s.storage.Update(ctx, vote); err != nil {
			logger.ErrorContext(ctx, "failed to update vote", "error", err)
			return nil, connecterr.Internal(ctx, "VoteOnLocation", err)
		}
	} else {
		vote := &models.LocationVote{
			ProposalId:     req.Msg.ProposalId,
			UserId:         authInfo.UserID,
			Status:         models.LocationVoteStatus(req.Msg.Status),
			VotedAtUnixSec: now,
		}
		if _, err := s.storage.Insert(ctx, vote); err != nil {
			logger.ErrorContext(ctx, "failed to create vote", "error", err)
			return nil, connecterr.Internal(ctx, "VoteOnLocation", err)
		}
	}

	logger.InfoContext(ctx, "location vote recorded")

	return connect.NewResponse(&api.VoteOnLocationResponse{}), nil
}

// ConfirmLocation confirms a location proposal as the final location for the
// experience.
//
// If the winning proposal carries an inline geocoded location (drop-pin /
// unsaved search result), it is materialized into a stored Location row and
// the experience.location_id is updated to point at the new row. If the
// proposal already references a saved Location, the experience is simply
// pointed at it. Only the experience owner can confirm.
func (s *Service) ConfirmLocation(
	ctx context.Context,
	req *connect.Request[api.ConfirmLocationRequest],
) (*connect.Response[api.ConfirmLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ConfirmLocation",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"location_proposal_id", req.Msg.ProposalId,
	)

	if req.Msg.ExperienceId == "" || req.Msg.ProposalId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id and proposal_id are required"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can confirm locations"))
	}

	if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
		expStored.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is not accepting location confirmations (current state: %s)", expStored.State))
	}

	proposalStored := &models.ExperienceLocationProposal{}
	if err := s.storage.GetByID(ctx, req.Msg.ProposalId, proposalStored); err != nil {
		logger.ErrorContext(ctx, "failed to get location proposal", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if proposalStored.ExperienceId != req.Msg.ExperienceId {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("proposal does not belong to this experience"))
	}

	// Resolve location_id: either reuse the saved reference, or materialize
	// the inline geocoded location into a stored Location row.
	locationID := proposalStored.LocationId
	if locationID == "" {
		if proposalStored.GeocodedLocation == nil {
			return nil, connecterr.Internal(ctx, "ConfirmLocation",
				fmt.Errorf("proposal %s has neither location_id nor geocoded_location", proposalStored.Id),
				"location_proposal_id", proposalStored.Id)
		}
		// Materialize the geocoded proposal as a stored Location.
		loc := proto.Clone(proposalStored.GeocodedLocation).(*models.Location)
		loc.Id = ""
		now := time.Now().Unix()
		loc.CreatedAtUnixSec = now
		loc.UpdatedAtUnixSec = now
		id, err := s.storage.Insert(ctx, loc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to materialize proposed geocoded location", "error", err)
			return nil, connecterr.Internal(ctx, "ConfirmLocation", err)
		}
		locationID = id
		// Update the proposal so future reads carry the canonical location_id.
		proposalStored.LocationId = id
	}

	proposalStored.IsConfirmed = true
	if err := s.storage.Update(ctx, proposalStored); err != nil {
		logger.ErrorContext(ctx, "failed to update proposal", "error", err)
		return nil, connecterr.Internal(ctx, "ConfirmLocation", err)
	}

	// Mark the poll as completed and clear the active flag.
	if expStored.LocationPollActive {
		expStored.LocationPollCompleted = proto.Bool(true)
	}
	expStored.LocationId = locationID
	expStored.LocationPollActive = false
	expStored.LocationPollDeadlineUnixSec = nil
	expStored.LocationProposalsLocked = nil
	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to update experience location", "error", err)
		return nil, connecterr.Internal(ctx, "ConfirmLocation", err)
	}

	// Unconfirm all other proposals for this experience.
	allProposals, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceLocationProposal{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query other proposals", "error", err)
	} else {
		for _, msg := range allProposals {
			other := msg.(*models.ExperienceLocationProposal)
			if other.Id != req.Msg.ProposalId && other.IsConfirmed {
				other.IsConfirmed = false
				if err := s.storage.Update(ctx, other); err != nil {
					logger.WarnContext(ctx, "failed to unconfirm other proposal",
						"other_location_proposal_id", other.Id,
						"error", err)
				}
			}
		}
	}

	logger.InfoContext(ctx, "location confirmed", "location_id", locationID)

	// Emit a DETAIL_CHANGED system message with the confirmed location name.
	if s.systemMessageWriter != nil && expStored.ConversationId != "" {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		msg := chat.DetailChangedLocationUnnamedMessage(displayName)
		if locName, ok := s.locationDisplayName(ctx, locationID); ok {
			msg = chat.DetailChangedLocationMessage(displayName, locName)
		}
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED,
			msg,
		); err != nil {
			logger.WarnContext(ctx, "failed to write location-confirmed system message",
				"conversation_id", expStored.ConversationId,
				"error", err)
		}
	}

	return connect.NewResponse(&api.ConfirmLocationResponse{
		LocationId: locationID,
	}), nil
}

// UnlockLocation unlocks a previously confirmed location.
func (s *Service) UnlockLocation(
	ctx context.Context,
	req *connect.Request[api.UnlockLocationRequest],
) (*connect.Response[api.UnlockLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UnlockLocation",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can unlock locations"))
	}

	allProposals, err := s.storage.QueryByField(ctx, "experience_id", req.Msg.ExperienceId, &models.ExperienceLocationProposal{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query proposals", "error", err)
		return nil, connecterr.Internal(ctx, "UnlockLocation", err)
	}

	unlockedCount := 0
	for _, msg := range allProposals {
		proposal := msg.(*models.ExperienceLocationProposal)
		if proposal.IsConfirmed {
			proposal.IsConfirmed = false
			if err := s.storage.Update(ctx, proposal); err != nil {
				logger.ErrorContext(ctx, "failed to unlock proposal",
					"location_proposal_id", proposal.Id,
					"error", err)
				return nil, connecterr.Internal(ctx, "UnlockLocation", err)
			}
			unlockedCount++
		}
	}

	logger.InfoContext(ctx, "location unlocked", "unlocked_count", unlockedCount)

	return connect.NewResponse(&api.UnlockLocationResponse{}), nil
}

// CancelLocationPoll ends the active location poll for an experience.
//
// Clears location_poll_active, marks the poll as completed, and emits a
// system message. Proposals and their votes are preserved so anyone can
// still open the read-only results view after the poll ends. Only the
// experience owner can end a poll.
func (s *Service) CancelLocationPoll(
	ctx context.Context,
	req *connect.Request[api.CancelLocationPollRequest],
) (*connect.Response[api.CancelLocationPollResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CancelLocationPoll",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can cancel a location poll"))
	}

	// Cancel returns the experience to a fresh "no poll" state. Clearing
	// completed + current_poll_id matters for the client dispatcher: with
	// completed=true, tapping the still-TBD location row would route to
	// the "owner picks a winner from these proposals" confirm picker — the
	// opposite of what cancel implies. Proposals themselves are preserved
	// so any future history view can still surface them.
	expStored.LocationPollActive = false
	expStored.LocationPollCompleted = nil
	expStored.CurrentLocationPollId = nil
	expStored.LocationPollDeadlineUnixSec = nil
	expStored.LocationProposalsLocked = nil
	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to clear location_poll_active", "error", err)
		return nil, connecterr.Internal(ctx, "CancelLocationPoll", err)
	}

	logger.InfoContext(ctx, "location poll ended")

	if s.systemMessageWriter != nil && expStored.ConversationId != "" {
		displayName := s.getUserDisplayName(ctx, authInfo.UserID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			expStored.ConversationId,
			authInfo.UserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOCATION_POLL_CANCELLED,
			chat.LocationPollCancelledMessage(displayName),
		); err != nil {
			logger.WarnContext(ctx, "failed to write LOCATION_POLL_CANCELLED system message",
				"conversation_id", expStored.ConversationId,
				"error", err)
		}
	}

	return connect.NewResponse(&api.CancelLocationPollResponse{}), nil
}

// DeleteLocationProposal removes a single proposal (and its votes) from a
// location poll. Owner can delete any proposal; participants can delete
// only their own. If the deleted proposal was the last one in the poll,
// the poll is also cleared (matches the no-proposals state).
// locationProposalSpec binds the shared deletion logic to the location proposal's
// proto types and experience fields.
var locationProposalSpec = proposalSpec{
	logIDField: "location_proposal_id",
	kind:       "location",

	newProposal: func() proto.Message { return &models.ExperienceLocationProposal{} },
	proposalFields: func(m proto.Message) (string, string, *string) {
		p := m.(*models.ExperienceLocationProposal)
		return p.ExperienceId, p.ProposedByUserId, p.PollId
	},
	newVote: func() proto.Message { return &models.LocationVote{} },
	voteID:  func(m proto.Message) string { return m.(*models.LocationVote).Id },

	pollActive:    func(e *models.Experience) bool { return e.LocationPollActive },
	currentPollID: func(e *models.Experience) *string { return e.CurrentLocationPollId },
	clearPoll: func(e *models.Experience) {
		e.LocationPollActive = false
		e.LocationPollCompleted = nil
		e.CurrentLocationPollId = nil
		e.LocationPollDeadlineUnixSec = nil
		e.LocationProposalsLocked = nil
	},
}

func (s *Service) DeleteLocationProposal(
	ctx context.Context,
	req *connect.Request[api.DeleteLocationProposalRequest],
) (*connect.Response[api.DeleteLocationProposalResponse], error) {
	if err := s.deletePollProposal(ctx, req.Msg.ExperienceId, req.Msg.ProposalId,
		"DeleteLocationProposal", locationProposalSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.DeleteLocationProposalResponse{}), nil
}

// defaultLocationPollDeadline returns the reply-by deadline (unix seconds) to
// seed on the first proposal of a fresh poll: 24 hours from now.
func defaultLocationPollDeadline(now int64) int64 {
	const dayInSeconds = int64(24 * time.Hour / time.Second)
	return now + dayInSeconds
}

// createLocationProposal creates a new location-proposal record in storage.
//
// One of [apiLoc.LocationId] / [apiLoc.Geocoded] must be set. [pollID] tags
// the proposal as belonging to a particular poll run; an empty string leaves
// it unset.
func (s *Service) createLocationProposal(
	ctx context.Context,
	experienceID, proposedByUserID string,
	apiLoc *api.ProposedLocation,
	pollID string,
) (*models.ExperienceLocationProposal, error) {
	now := clock.UnixSec(ctx)
	proposal := &models.ExperienceLocationProposal{
		ExperienceId:      experienceID,
		ProposedByUserId:  proposedByUserID,
		ProposedAtUnixSec: now,
		IsConfirmed:       false,
	}
	if apiLoc.LocationId != "" {
		proposal.LocationId = apiLoc.LocationId
	} else if apiLoc.Geocoded != nil {
		proposal.GeocodedLocation = geocodedAPIToLocationModel(apiLoc.Geocoded)
	}
	if pollID != "" {
		proposal.PollId = proto.String(pollID)
	}

	proposalID, err := s.storage.Insert(ctx, proposal)
	if err != nil {
		return nil, connecterr.Internal(ctx, "createLocationProposal", err)
	}
	proposal.Id = proposalID
	return proposal, nil
}

// buildLocationProposal builds an API LocationProposal from a stored
// ExperienceLocationProposal with enriched user data.
func (s *Service) buildLocationProposal(ctx context.Context, proposal *models.ExperienceLocationProposal) (*api.LocationProposal, error) {
	logger := logging.LoggerWithContext(ctx).With("location_proposal_id", proposal.Id)

	proposer, err := services.FetchAPIUser(ctx, s.storage, proposal.ProposedByUserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get proposer user", "error", err)
		return nil, connecterr.Internal(ctx, "buildLocationProposal", fmt.Errorf("failed to load proposer information"))
	}

	voteMessages, err := s.storage.QueryByField(ctx, "proposal_id", proposal.Id, &models.LocationVote{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query votes", "error", err)
		return nil, connecterr.Internal(ctx, "buildLocationProposal", err)
	}

	votes := make([]*api.LocationVote, 0, len(voteMessages))
	for _, msg := range voteMessages {
		vote := msg.(*models.LocationVote)
		user, err := services.FetchAPIUser(ctx, s.storage, vote.UserId)
		if err != nil {
			logger.WarnContext(ctx, "failed to get user for vote", "user_id", vote.UserId, "error", err)
			continue
		}
		votes = append(votes, &api.LocationVote{
			User:   user,
			Status: api.LocationVoteStatus(vote.Status),
		})
	}

	return &api.LocationProposal{
		Id:                proposal.Id,
		ProposedBy:        proposer,
		Location:          proposedLocationModelToAPI(proposal),
		Votes:             votes,
		IsConfirmed:       proposal.IsConfirmed,
		ProposedAtUnixSec: proposal.ProposedAtUnixSec,
		PollId:            proposal.PollId,
	}, nil
}

// buildLocationProposalsFromMaps assembles LocationProposals for one
// experience from pre-fetched maps, mirroring buildTimeProposalsFromMaps.
// Proposals are returned in the same order they appear in proposalsByExp.
func buildLocationProposalsFromMaps(
	ctx context.Context,
	experienceID string,
	proposalsByExp map[string][]*models.ExperienceLocationProposal,
	votesByProposal map[string][]*models.LocationVote,
	userMap map[string]*api.User,
	logger *logging.Logger,
) []*api.LocationProposal {
	rawProposals := proposalsByExp[experienceID]
	proposals := make([]*api.LocationProposal, 0, len(rawProposals))
	for _, proposal := range rawProposals {
		proposer := userMap[proposal.ProposedByUserId]
		if proposer == nil {
			logger.WarnContext(ctx, "proposer not found in user map, skipping location proposal",
				"location_proposal_id", proposal.Id, "user_id", proposal.ProposedByUserId)
			continue
		}

		rawVotes := votesByProposal[proposal.Id]
		votes := make([]*api.LocationVote, 0, len(rawVotes))
		for _, vote := range rawVotes {
			voter := userMap[vote.UserId]
			if voter == nil {
				logger.WarnContext(ctx, "voter not found in user map, skipping location vote",
					"location_proposal_id", proposal.Id, "user_id", vote.UserId)
				continue
			}
			votes = append(votes, &api.LocationVote{
				User:   voter,
				Status: api.LocationVoteStatus(vote.Status),
			})
		}

		proposals = append(proposals, &api.LocationProposal{
			Id:                proposal.Id,
			ProposedBy:        proposer,
			Location:          proposedLocationModelToAPI(proposal),
			Votes:             votes,
			IsConfirmed:       proposal.IsConfirmed,
			ProposedAtUnixSec: proposal.ProposedAtUnixSec,
			PollId:            proposal.PollId,
		})
	}
	return proposals
}

// proposedLocationModelToAPI converts a stored proposal into the API's
// ProposedLocation oneof, preferring the saved location_id when both are set
// (which should be impossible by construction but is defensive).
func proposedLocationModelToAPI(p *models.ExperienceLocationProposal) *api.ProposedLocation {
	if p.LocationId != "" {
		return &api.ProposedLocation{LocationId: p.LocationId}
	}
	if p.GeocodedLocation != nil {
		return &api.ProposedLocation{Geocoded: locationModelToGeocodedAPI(p.GeocodedLocation)}
	}
	return &api.ProposedLocation{}
}

// geocodedAPIToLocationModel converts an API GeocodedLocation into a storage
// Location proto. The returned model carries no ID; the caller is responsible
// for inserting it if it needs to become a stored row.
func geocodedAPIToLocationModel(g *api.GeocodedLocation) *models.Location {
	if g == nil {
		return nil
	}
	loc := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  g.LatitudeDeg,
			LongitudeDeg: g.LongitudeDeg,
		},
		Address: &models.Address{
			RegionCode:   g.RegionCode,
			PostalCode:   g.PostalCode,
			Locality:     g.Locality,
			AddressLines: g.AddressLines,
		},
	}
	if g.Name != "" {
		name := g.Name
		loc.Name = &name
	}
	if g.ExternalPlaceId != "" {
		id := g.ExternalPlaceId
		loc.ExternalPlaceId = &id
	}
	if g.ExternalPlaceProvider != "" {
		provider := g.ExternalPlaceProvider
		loc.ExternalPlaceProvider = &provider
	}
	if g.Neighborhood != "" {
		n := g.Neighborhood
		loc.Address.Neighborhood = &n
	}
	if g.County != "" {
		c := g.County
		loc.Address.County = &c
	}
	if g.AdministrativeArea != "" {
		a := g.AdministrativeArea
		loc.Address.AdministrativeArea = &a
	}
	return loc
}

// locationModelToGeocodedAPI converts a stored Location into an API
// GeocodedLocation for inclusion in a proposal that hasn't been materialized
// into a saved row yet.
func locationModelToGeocodedAPI(loc *models.Location) *api.GeocodedLocation {
	if loc == nil {
		return nil
	}
	out := &api.GeocodedLocation{}
	if loc.Geolocation != nil {
		out.LatitudeDeg = loc.Geolocation.LatitudeDeg
		out.LongitudeDeg = loc.Geolocation.LongitudeDeg
	}
	if loc.Address != nil {
		out.RegionCode = loc.Address.RegionCode
		out.PostalCode = loc.Address.PostalCode
		out.Locality = loc.Address.Locality
		out.AddressLines = loc.Address.AddressLines
		if loc.Address.Neighborhood != nil {
			out.Neighborhood = *loc.Address.Neighborhood
		}
		if loc.Address.County != nil {
			out.County = *loc.Address.County
		}
		if loc.Address.AdministrativeArea != nil {
			out.AdministrativeArea = *loc.Address.AdministrativeArea
		}
	}
	if loc.Name != nil {
		out.Name = *loc.Name
	}
	if loc.ExternalPlaceId != nil {
		out.ExternalPlaceId = *loc.ExternalPlaceId
	}
	if loc.ExternalPlaceProvider != nil {
		out.ExternalPlaceProvider = *loc.ExternalPlaceProvider
	}
	return out
}

// locationDisplayName returns a short human-readable name for a stored
// Location, used in chat system messages, falling back to locality + region
// when no explicit name is set. ok is false when the location has no
// displayable name (missing id, fetch failure, or no name/locality) — the
// caller emits the unnamed message variant so the client renders a localized
// line instead of a server-invented placeholder.
func (s *Service) locationDisplayName(ctx context.Context, locationID string) (name string, ok bool) {
	if locationID == "" {
		return "", false
	}
	loc := &models.Location{}
	if err := s.storage.GetByID(ctx, locationID, loc); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get location for display name",
			"location_id", locationID, "error", err)
		return "", false
	}
	if loc.GetName() != "" {
		return loc.GetName(), true
	}
	if loc.Address != nil && loc.Address.Locality != "" {
		if loc.Address.RegionCode != "" {
			return fmt.Sprintf("%s, %s", loc.Address.Locality, loc.Address.RegionCode), true
		}
		return loc.Address.Locality, true
	}
	return "", false
}
