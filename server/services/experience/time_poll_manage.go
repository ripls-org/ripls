// Package experience: organizer "Manage" actions on the active time poll.
//
// Implements the three RPCs invoked from the vote modal's Manage menu:
//
//   - SetTimePollDeadline records (or clears) a reply-by deadline.
//   - LockTimeProposals freezes the list of proposals.
//   - NudgeTimePollVoters dispatches a reminder push to participants
//     who have not voted on the active poll.
//
// All three reject non-owners and require an active time poll. Nudge may return
// nudged_count=0 (not an error) when everyone has already replied.
//
// The bodies live in poll_manage.go, shared with the location poll; this file
// holds the RPC signatures and timePollSpec.
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

// timePollSpec binds the shared Manage implementation to the time poll's proto
// fields and copy.
var timePollSpec = pollSpec{
	kind:           "time",
	nudgeEventType: "EXPERIENCE_TIME_NUDGE",

	isActive: func(e *models.Experience) bool { return e.TimePollActive },
	setDeadline: func(e *models.Experience, deadline *int64) {
		e.TimePollDeadlineUnixSec = deadline
	},
	setLocked: func(e *models.Experience, locked bool) {
		if locked {
			e.TimeProposalsLocked = proto.Bool(true)
			return
		}
		e.TimeProposalsLocked = nil
	},
	currentPollID: func(e *models.Experience) *string { return e.CurrentPollId },
	votersOnPoll:  timeVotersOnPoll,
}

// SetTimePollDeadline records the reply-by deadline displayed on the propose /
// vote modals. Passing deadline_unix_sec=0 clears the deadline.
func (s *Service) SetTimePollDeadline(
	ctx context.Context,
	req *connect.Request[api.SetTimePollDeadlineRequest],
) (*connect.Response[api.SetTimePollDeadlineResponse], error) {
	if err := s.setPollDeadline(ctx, req.Msg.ExperienceId, req.Msg.DeadlineUnixSec,
		"SetTimePollDeadline", timePollSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.SetTimePollDeadlineResponse{}), nil
}

// LockTimeProposals freezes the list of proposals on the active poll.
// While locked, ProposeTime returns FailedPrecondition.
func (s *Service) LockTimeProposals(
	ctx context.Context,
	req *connect.Request[api.LockTimeProposalsRequest],
) (*connect.Response[api.LockTimeProposalsResponse], error) {
	if err := s.lockPollProposals(ctx, req.Msg.ExperienceId, req.Msg.Locked,
		"LockTimeProposals", timePollSpec); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.LockTimeProposalsResponse{}), nil
}

// NudgeTimePollVoters dispatches a reminder push to each Yes/Maybe RSVP who has
// not voted on any proposal in the active poll. The owner is excluded from the
// recipient set. Returns the count of users notified (zero is allowed and is not
// an error).
func (s *Service) NudgeTimePollVoters(
	ctx context.Context,
	req *connect.Request[api.NudgeTimePollVotersRequest],
) (*connect.Response[api.NudgeTimePollVotersResponse], error) {
	nudged, err := s.nudgePollVoters(ctx, req.Msg.ExperienceId,
		"NudgeTimePollVoters", timePollSpec)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.NudgeTimePollVotersResponse{
		NudgedCount: int32(nudged),
	}), nil
}

// timeVotersOnPoll returns the user IDs who replied to the given time poll —
// anyone who cast a YES vote on a proposal or recorded FLEXIBLE
// ("any time works").
func timeVotersOnPoll(
	ctx context.Context, s *Service, exp *models.Experience, pollID string,
) (map[string]struct{}, error) {
	voters := make(map[string]struct{})

	proposalMessages, err := s.storage.QueryByField(ctx, "experience_id", exp.Id, &models.ExperienceTimeProposal{})
	if err != nil {
		return voters, err
	}
	for _, msg := range proposalMessages {
		proposal := msg.(*models.ExperienceTimeProposal)
		if proposal.PollId == nil || *proposal.PollId != pollID {
			continue
		}
		voteMessages, err := s.storage.QueryByField(ctx, "proposal_id", proposal.Id, &models.TimeVote{})
		if err != nil {
			return voters, err
		}
		for _, vmsg := range voteMessages {
			vote := vmsg.(*models.TimeVote)
			if vote.Status == models.TimeVoteStatus_TIME_VOTE_STATUS_YES ||
				vote.Status == models.TimeVoteStatus_TIME_VOTE_STATUS_FLEXIBLE {
				voters[vote.UserId] = struct{}{}
			}
		}
	}
	return voters, nil
}
