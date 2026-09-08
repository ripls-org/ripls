// Package experience: the shared implementation behind the organizer "Manage"
// actions on an active poll.
//
// The location and time polls run parallel lifecycles (the parity contract in
// docs/client/polls.md), and their three Manage RPCs were literal copies of one
// another — 305 of 318 lines identical, differing only in which proto field they
// read and what the nudge says. That is the pair `dupl` flags first. The bodies
// live here once, parameterized by pollSpec; location_poll_manage.go and
// time_poll_manage.go keep only the RPC signatures and their spec.
//
// Adding a third poll kind should mean writing one pollSpec, not a third copy.
package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/rsvpstate"
)

// pollSpec is everything that differs between the location poll and the time
// poll. Everything else about Manage is identical between them.
type pollSpec struct {
	// kind names the poll in error messages and log lines ("location", "time").
	kind string
	// nudgeEventType is the CommunityEventPayload event type for the nudge. It
	// selects the poll's copy in the shared catalog — title, message and CTA —
	// so no copy lives on this struct.
	nudgeEventType string

	// isActive reports whether this poll is currently open on the experience.
	isActive func(*models.Experience) bool
	// setDeadline writes (or clears, on nil) the reply-by deadline.
	setDeadline func(*models.Experience, *int64)
	// setLocked freezes or unfreezes the proposal list.
	setLocked func(*models.Experience, bool)
	// currentPollID returns the active poll's ID, or nil when none is active.
	currentPollID func(*models.Experience) *string
	// votersOnPoll returns the user IDs who have replied to the given poll.
	votersOnPoll func(context.Context, *Service, *models.Experience, string) (map[string]struct{}, error)
}

// loadOwnedActivePoll performs the preamble all three Manage RPCs share: require
// auth, validate the experience ID, load the experience, and require that the
// caller owns it and that this poll is active.
//
// Returns the loaded experience and a logger already carrying the standard
// fields. `operation` is the RPC name so the log line and any Internal error
// attribute to the caller rather than to this shared helper.
func (s *Service) loadOwnedActivePoll(
	ctx context.Context, experienceID, operation string, spec pollSpec, extra ...any,
) (*models.Experience, *logging.Logger, error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, nil, err
	}

	fields := append([]any{
		"operation", operation,
		"user_id", authInfo.UserID,
		"experience_id", experienceID,
	}, extra...)
	logger := logging.LoggerWithContext(ctx).With(fields...)

	if experienceID == "" {
		return nil, logger, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, logger, connect.NewError(connect.CodeNotFound, err)
	}

	if expStored.OwnerId != authInfo.UserID {
		return nil, logger, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner can manage the %s poll", spec.kind))
	}

	if !spec.isActive(expStored) {
		return nil, logger, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no active %s poll", spec.kind))
	}

	return expStored, logger, nil
}

// setPollDeadline records the reply-by deadline shown on the propose / vote
// modals. A deadline <= 0 clears it.
func (s *Service) setPollDeadline(
	ctx context.Context, experienceID string, deadlineUnixSec int64, operation string, spec pollSpec,
) error {
	expStored, logger, err := s.loadOwnedActivePoll(
		ctx, experienceID, operation, spec, "deadline_unix_sec", deadlineUnixSec,
	)
	if err != nil {
		return err
	}

	if deadlineUnixSec <= 0 {
		spec.setDeadline(expStored, nil)
	} else {
		spec.setDeadline(expStored, proto.Int64(deadlineUnixSec))
	}

	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to update experience deadline", "error", err)
		return connecterr.Internal(ctx, operation, err)
	}

	logger.InfoContext(ctx, "poll deadline updated", "poll_kind", spec.kind)
	return nil
}

// lockPollProposals freezes the proposal list on the active poll. While locked,
// the corresponding Propose RPC returns FailedPrecondition.
func (s *Service) lockPollProposals(
	ctx context.Context, experienceID string, locked bool, operation string, spec pollSpec,
) error {
	expStored, logger, err := s.loadOwnedActivePoll(
		ctx, experienceID, operation, spec, "locked", locked,
	)
	if err != nil {
		return err
	}

	spec.setLocked(expStored, locked)

	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to update lock state", "error", err)
		return connecterr.Internal(ctx, operation, err)
	}

	logger.InfoContext(ctx, "proposal lock state updated", "poll_kind", spec.kind)
	return nil
}

// nudgePollVoters pushes a reminder to every Yes/Maybe RSVP who has not replied
// to the active poll, excluding the owner. Returns how many were notified; zero
// is a valid outcome, not an error.
func (s *Service) nudgePollVoters(
	ctx context.Context, experienceID, operation string, spec pollSpec,
) (int, error) {
	expStored, logger, err := s.loadOwnedActivePoll(ctx, experienceID, operation, spec)
	if err != nil {
		return 0, err
	}

	candidates, err := s.pollNudgeCandidates(ctx, experienceID, expStored, operation, logger, spec)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		logger.InfoContext(ctx, "no unreplied voters to nudge", "poll_kind", spec.kind)
		return 0, nil
	}

	// Drop the nudge entirely if every community the experience is shared with
	// has been soft-deleted. Mirrors the active-community gate the
	// community_subscriber applies to bus-driven pushes.
	if !s.experienceHasActiveCommunity(ctx, experienceID, logger) {
		logger.InfoContext(ctx, "no active community for nudge; suppressing pushes")
		return 0, nil
	}

	notification := pollNudgeNotification(ctx, experienceID, expStored.Name, spec)

	nudged := 0
	if s.notificationService != nil {
		for userID := range candidates {
			if err := s.notificationService.NotifyUser(ctx, userID, notification); err != nil {
				logger.WarnContext(ctx, "failed to nudge voter",
					"recipient_user_id", userID, "error", err)
				continue
			}
			nudged++
		}
	}

	logger.InfoContext(ctx, "poll nudge dispatched",
		"poll_kind", spec.kind, "nudged_count", nudged, "candidate_count", len(candidates))
	return nudged, nil
}

// pollNudgeCandidates returns the Yes/Maybe RSVPs who have not yet replied to
// the active poll, excluding the owner.
func (s *Service) pollNudgeCandidates(
	ctx context.Context, experienceID string, expStored *models.Experience,
	operation string, logger *logging.Logger, spec pollSpec,
) (map[string]struct{}, error) {
	// Every Yes/Maybe RSVP — the people the modal counts as "invited" to the poll.
	rsvpMessages, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query rsvps for nudge", "error", err)
		return nil, connecterr.Internal(ctx, operation, err)
	}

	candidates := make(map[string]struct{}, len(rsvpMessages))
	for _, msg := range rsvpMessages {
		rsvp := msg.(*models.ExperienceRSVP)
		if rsvp.Deleted != nil {
			continue
		}
		if rsvp.UserId == "" || rsvp.UserId == expStored.OwnerId {
			continue
		}
		if rsvpstate.IsGoing(rsvp.GetIntention()) {
			candidates[rsvp.UserId] = struct{}{}
		}
	}

	if len(candidates) == 0 {
		return candidates, nil
	}

	// Subtract anyone who has already replied. A failure here is non-fatal:
	// nudging someone twice beats nudging nobody.
	pollID := spec.currentPollID(expStored)
	if pollID == nil {
		return candidates, nil
	}
	voters, err := spec.votersOnPoll(ctx, s, expStored, *pollID)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve current voters; nudging full RSVP set", "error", err)
		return candidates, nil
	}
	for uid := range voters {
		delete(candidates, uid)
	}
	return candidates, nil
}

// experienceHasActiveCommunity reports whether the experience is still shared
// with at least one non-archived, non-deleted, active community. A query failure
// is treated as "yes" so a transient storage error does not silently swallow the
// nudge.
func (s *Service) experienceHasActiveCommunity(
	ctx context.Context, experienceID string, logger *logging.Logger,
) bool {
	links, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load community-experience links for nudge", "error", err)
		return true
	}
	for _, msg := range links {
		ce := msg.(*models.CommunityExperience)
		if ce.Archived || ce.Deleted != nil {
			continue
		}
		if community.IsActive(ctx, s.storage, ce.CommunityId) {
			return true
		}
	}
	return false
}

// pollNudgeNotification builds the reminder. Copy comes from the poll's kind in
// the shared catalog, so push, SMS and email render one sentence that names the
// plan — the old push body ("The organizer is waiting on your pick.") named
// nothing, which left an off-app recipient with no way to tell which plan wanted
// them (#2896).
//
// The experience name must reach the payload, not just the rendered body: the
// off-app senders ignore Title/Body and re-render from these fields.
func pollNudgeNotification(ctx context.Context, experienceID, experienceName string, spec pollSpec) *models.Notification {
	return notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		EventType:      spec.nudgeEventType,
		ExperienceName: experienceName,
		ExperienceId:   proto.String(experienceID),
	})
}

// --- Proposal deletion -------------------------------------------------------
//
// DeleteLocationProposal and DeleteTimeProposal were the second pair `dupl`
// flagged: 108 identical lines of permission, cascade-delete and empty-poll
// cleanup logic. That logic is security-relevant (who may delete whose
// proposal), so having one copy matters more than the line count — a fix to the
// permission rule previously had to be made twice or silently diverge.

// proposalSpec is the deletion-side counterpart to pollSpec: the proto types and
// experience fields that differ between the location and time proposals.
type proposalSpec struct {
	// logIDField names the proposal ID in log lines ("location_proposal_id").
	logIDField string
	// kind names the proposal in error and log messages ("location", "time").
	kind string

	// newProposal returns an empty proposal message to load into.
	newProposal func() proto.Message
	// proposalFields reads the experience ID, proposer ID and poll ID off a
	// loaded proposal.
	proposalFields func(proto.Message) (experienceID, proposedByUserID string, pollID *string)
	// newVote returns an empty vote message for the votes cascade.
	newVote func() proto.Message
	// voteID reads a vote's ID, for the failure log line.
	voteID func(proto.Message) string

	// pollActive reports whether this poll is open.
	pollActive func(*models.Experience) bool
	// currentPollID returns the active poll's ID, or nil.
	currentPollID func(*models.Experience) *string
	// clearPoll resets every field of this poll, used when the last surviving
	// proposal is deleted.
	clearPoll func(*models.Experience)
}

// deletePollProposal removes a proposal and its votes, then clears the poll if
// that was the last surviving option. The owner may delete any proposal; a
// non-owner may delete only their own, and must still be a participant.
func (s *Service) deletePollProposal(
	ctx context.Context, experienceID, proposalID, operation string, spec proposalSpec,
) error {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", operation,
		"user_id", authInfo.UserID,
		"experience_id", experienceID,
		spec.logIDField, proposalID,
	)

	if experienceID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"))
	}
	if proposalID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("proposal_id is required"))
	}

	proposalStored := spec.newProposal()
	if err := s.storage.GetByID(ctx, proposalID, proposalStored); err != nil {
		logger.ErrorContext(ctx, "failed to get proposal", "poll_kind", spec.kind, "error", err)
		return connect.NewError(connect.CodeNotFound, err)
	}
	proposalExpID, proposedBy, _ := spec.proposalFields(proposalStored)
	if proposalExpID != experienceID {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("proposal does not belong to this experience"))
	}

	expStored := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, expStored); err != nil {
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return connect.NewError(connect.CodeNotFound, err)
	}

	// Owner can delete any proposal. Non-owners can only delete their own.
	isOwner := expStored.OwnerId == authInfo.UserID
	if !isOwner && proposedBy != authInfo.UserID {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the experience owner or the proposer can delete this proposal"))
	}
	// Non-owners still need to be participants to act on the poll.
	if !isOwner {
		if err := s.requireParticipant(ctx, authInfo.UserID, expStored); err != nil {
			return err
		}
	}

	// Delete any votes cast on this proposal before removing the proposal
	// itself, so we don't leave orphan vote rows.
	voteMessages, err := s.storage.QueryByField(ctx, "proposal_id", proposalID, spec.newVote())
	if err != nil {
		logger.WarnContext(ctx, "failed to query votes for proposal", "error", err)
	} else {
		for _, vm := range voteMessages {
			if err := s.storage.Delete(ctx, vm); err != nil {
				logger.WarnContext(ctx, "failed to delete vote",
					"vote_id", spec.voteID(vm), "error", err)
			}
		}
	}

	if err := s.storage.Delete(ctx, proposalStored); err != nil {
		logger.ErrorContext(ctx, "failed to delete proposal", "poll_kind", spec.kind, "error", err)
		return connecterr.Internal(ctx, operation, err)
	}

	s.clearPollIfEmpty(ctx, experienceID, expStored, logger, spec)

	logger.InfoContext(ctx, "proposal deleted", "poll_kind", spec.kind)
	return nil
}

// clearPollIfEmpty resets the poll when deleting a proposal left the active poll
// with no surviving options, so the experience returns to a fresh state.
func (s *Service) clearPollIfEmpty(
	ctx context.Context, experienceID string, expStored *models.Experience,
	logger *logging.Logger, spec proposalSpec,
) {
	if !spec.pollActive(expStored) {
		return
	}
	remaining, err := s.storage.QueryByField(ctx, "experience_id", experienceID, spec.newProposal())
	if err != nil {
		logger.WarnContext(ctx, "failed to query remaining proposals", "error", err)
		return
	}

	activePollID := ""
	if id := spec.currentPollID(expStored); id != nil {
		activePollID = *id
	}
	surviving := 0
	for _, m := range remaining {
		_, _, pollID := spec.proposalFields(m)
		if activePollID == "" || (pollID != nil && *pollID == activePollID) {
			surviving++
		}
	}
	if surviving > 0 {
		return
	}

	spec.clearPoll(expStored)
	if err := s.storage.Update(ctx, expStored); err != nil {
		logger.WarnContext(ctx, "failed to clear empty poll", "error", err)
	}
}
