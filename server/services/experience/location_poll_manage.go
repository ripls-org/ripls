// Package experience: organizer "Manage" actions on the active location poll.
//
// Implements the three RPCs invoked from the propose modal's deadline strip
// and the vote modal's Manage menu:
//
//   - SetLocationPollDeadline records (or clears) a reply-by deadline.
//   - LockLocationProposals freezes the list of proposals.
//   - NudgeLocationPollVoters dispatches a reminder push to participants
//     who have not voted on the active poll.
//
// All three reject non-owners and require an active location poll. Nudge may
// return nudged_count=0 (not an error) when everyone has already replied.
//
// The bodies live in poll_manage.go, shared with the time poll; this file holds
// the RPC signatures and locationPollSpec.
// lifecycle (docs/client/polls.md). Their substantive Manage logic is shared
// in poll_manage.go; what is left here is the binding layer — three RPC
// wrappers that cannot share a body because each takes a different proto
// request/response type, plus a voter lookup whose only variance is the proto
// message type. Genericizing that needs four accessor closures to save 25
// lines, a worse trade. dupl cannot tell parallel declarations from copied
// logic; this pair was 305 duplicated lines before the extraction.
//
//nolint:dupl // The location and time polls run a deliberate parity
package experience

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// locationPollSpec binds the shared Manage implementation to the location poll's
// proto fields and copy.
var locationPollSpec = pollSpec{
	kind:           "location",
	nudgeEventType: "EXPERIENCE_LOCATION_NUDGE",

	isActive: func(e *models.Experience) bool { return e.LocationPollActive },
	setDeadline: func(e *models.Experience, deadline *int64) {
		e.LocationPollDeadlineUnixSec = deadline
	},
	setLocked: func(e *models.Experience, locked bool) {
		if locked {
			e.LocationProposalsLocked = proto.Bool(true)
			return
		}
		e.LocationProposalsLocked = nil
	},
	currentPollID: func(e *models.Experience) *string { return e.CurrentLocationPollId },
	votersOnPoll:  locationVotersOnPoll,
}

// SetLocationPollDeadline records the reply-by deadline displayed on the
// propose / vote modals. Passing deadline_unix_sec=0 clears the deadline.
func (s *Service) SetLocationPollDeadline(
	ctx context.Context,
	req *connect.Request[api.SetLocationPollDeadlineRequest],
) (*connect.Response[api.SetLocationPollDeadlineResponse], error) {
	if err := s.setPollDeadline(ctx, req.Msg.ExperienceId, req.Msg.DeadlineUnixSec,
		"SetLocationPollDeadline", locationPollSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.SetLocationPollDeadlineResponse{}), nil
}

// LockLocationProposals freezes the list of proposals on the active poll.
// While locked, ProposeLocation returns FailedPrecondition.
func (s *Service) LockLocationProposals(
	ctx context.Context,
	req *connect.Request[api.LockLocationProposalsRequest],
) (*connect.Response[api.LockLocationProposalsResponse], error) {
	if err := s.lockPollProposals(ctx, req.Msg.ExperienceId, req.Msg.Locked,
		"LockLocationProposals", locationPollSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.LockLocationProposalsResponse{}), nil
}

// NudgeLocationPollVoters dispatches a reminder push to each Yes/Maybe RSVP
// who has not voted on any proposal in the active poll. The owner is
// excluded from the recipient set. Returns the count of users notified
// (zero is allowed and is not an error).
func (s *Service) NudgeLocationPollVoters(
	ctx context.Context,
	req *connect.Request[api.NudgeLocationPollVotersRequest],
) (*connect.Response[api.NudgeLocationPollVotersResponse], error) {
	nudged, err := s.nudgePollVoters(ctx, req.Msg.ExperienceId,
		"NudgeLocationPollVoters", locationPollSpec)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.NudgeLocationPollVotersResponse{
		NudgedCount: int32(nudged),
	}), nil
}

// locationVotersOnPoll returns the user IDs who replied to the given location
// poll — anyone who cast a YES vote on a proposal or recorded FLEXIBLE
// ("any spot works").
func locationVotersOnPoll(
	ctx context.Context, s *Service, exp *models.Experience, pollID string,
) (map[string]struct{}, error) {
	voters := make(map[string]struct{})

	proposalMessages, err := s.storage.QueryByField(ctx, "experience_id", exp.Id, &models.ExperienceLocationProposal{})
	if err != nil {
		return voters, err
	}
	for _, msg := range proposalMessages {
		proposal := msg.(*models.ExperienceLocationProposal)
		if proposal.PollId == nil || *proposal.PollId != pollID {
			continue
		}
		voteMessages, err := s.storage.QueryByField(ctx, "proposal_id", proposal.Id, &models.LocationVote{})
		if err != nil {
			return voters, err
		}
		for _, vmsg := range voteMessages {
			vote := vmsg.(*models.LocationVote)
			if vote.Status == models.LocationVoteStatus_LOCATION_VOTE_STATUS_YES ||
				vote.Status == models.LocationVoteStatus_LOCATION_VOTE_STATUS_FLEXIBLE {
				voters[vote.UserId] = struct{}{}
			}
		}
	}
	return voters, nil
}
