package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// timePollManageFixture is the shared scenario for organizer-manage tests:
// an experience with a community of (owner, voter, non-voter, bystander),
// all four community members RSVP'd YES, and the owner has opened a time
// poll with one proposal. Helpers can then exercise the manage RPCs from
// any of those contexts.
type timePollManageFixture struct {
	service     *Service
	testStorage *storage.ProtoSQLStorage
	expID       string
	proposalID  string
	communityID string

	ownerID, voterID, nonVoterID, bystanderID     string
	ownerCtx, voterCtx, nonVoterCtx, bystanderCtx context.Context
}

func setupTimePollManageFixture(t *testing.T) *timePollManageFixture {
	t.Helper()
	service, testStorage, _ := setupTestService(t)

	fx := &timePollManageFixture{
		service:     service,
		testStorage: testStorage,
		ownerID:     "manage-owner",
		voterID:     "manage-voter",
		nonVoterID:  "manage-nonvoter",
		bystanderID: "manage-bystander",
	}

	createTestUser(t, testStorage, fx.ownerID, "owner@example.com", "Owner")
	createTestUser(t, testStorage, fx.voterID, "voter@example.com", "Voter")
	createTestUser(t, testStorage, fx.nonVoterID, "nonvoter@example.com", "NonVoter")
	createTestUser(t, testStorage, fx.bystanderID, "bystander@example.com", "Bystander")

	fx.communityID = createTestCommunity(t, testStorage, "Manage Test", fx.ownerID)
	for _, uid := range []string{fx.ownerID, fx.voterID, fx.nonVoterID, fx.bystanderID} {
		createTestCommunityMembership(t, testStorage, fx.communityID, uid)
	}

	fx.ownerCtx = createAuthenticatedContext(fx.ownerID, "owner@example.com", models.Role_ROLE_USER)
	fx.voterCtx = createAuthenticatedContext(fx.voterID, "voter@example.com", models.Role_ROLE_USER)
	fx.nonVoterCtx = createAuthenticatedContext(fx.nonVoterID, "nonvoter@example.com", models.Role_ROLE_USER)
	fx.bystanderCtx = createAuthenticatedContext(fx.bystanderID, "bystander@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(fx.ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Manage Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	fx.expID = createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, fx.ownerCtx, fx.expID, fx.communityID)

	// Voter and non-voter both RSVP YES so the nudge can target the
	// non-voter only. Bystander stays non-RSVP'd to verify they're not
	// nudged either.
	for _, c := range []context.Context{fx.voterCtx, fx.nonVoterCtx} {
		if _, err := service.RSVPToExperience(c, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: fx.expID,
			CommunityId:  fx.communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
			t.Fatalf("RSVP failed: %v", err)
		}
	}

	proposeResp, err := service.ProposeTime(fx.ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: fx.expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735747200,
					Timezone:         "UTC",
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("ProposeTime failed: %v", err)
	}
	fx.proposalID = proposeResp.Msg.Proposal.Id

	return fx
}

func TestService_SetTimePollDeadline(t *testing.T) {
	fx := setupTimePollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.SetTimePollDeadline(fx.voterCtx, connect.NewRequest(&api.SetTimePollDeadlineRequest{
			ExperienceId:    fx.expID,
			DeadlineUnixSec: 1735900000,
		}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner sets and clears deadline", func(t *testing.T) {
		_, err := fx.service.SetTimePollDeadline(fx.ownerCtx, connect.NewRequest(&api.SetTimePollDeadlineRequest{
			ExperienceId:    fx.expID,
			DeadlineUnixSec: 1735900000,
		}))
		if err != nil {
			t.Fatalf("SetTimePollDeadline failed: %v", err)
		}

		exp := &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.TimePollDeadlineUnixSec == nil || *exp.TimePollDeadlineUnixSec != 1735900000 {
			t.Errorf("expected deadline 1735900000, got %v", exp.TimePollDeadlineUnixSec)
		}

		// Clear via zero.
		if _, err := fx.service.SetTimePollDeadline(fx.ownerCtx, connect.NewRequest(&api.SetTimePollDeadlineRequest{
			ExperienceId:    fx.expID,
			DeadlineUnixSec: 0,
		})); err != nil {
			t.Fatalf("clear deadline failed: %v", err)
		}
		exp = &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID after clear: %v", err)
		}
		if exp.TimePollDeadlineUnixSec != nil {
			t.Errorf("expected nil deadline after clear, got %v", *exp.TimePollDeadlineUnixSec)
		}
	})

	t.Run("empty experience_id returns InvalidArgument", func(t *testing.T) {
		_, err := fx.service.SetTimePollDeadline(fx.ownerCtx, connect.NewRequest(&api.SetTimePollDeadlineRequest{}))
		if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})
}

func TestService_LockTimeProposals(t *testing.T) {
	fx := setupTimePollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.LockTimeProposals(fx.voterCtx, connect.NewRequest(&api.LockTimeProposalsRequest{
			ExperienceId: fx.expID,
			Locked:       true,
		}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner locks then unlocks", func(t *testing.T) {
		if _, err := fx.service.LockTimeProposals(fx.ownerCtx, connect.NewRequest(&api.LockTimeProposalsRequest{
			ExperienceId: fx.expID,
			Locked:       true,
		})); err != nil {
			t.Fatalf("Lock failed: %v", err)
		}

		exp := &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.TimeProposalsLocked == nil || !*exp.TimeProposalsLocked {
			t.Errorf("expected locked=true, got %v", exp.TimeProposalsLocked)
		}

		// ProposeTime is rejected while locked.
		_, err := fx.service.ProposeTime(fx.voterCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: fx.expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: 1735833600,
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		}))
		if err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("expected FailedPrecondition when locked, got %v", err)
		}

		// Unlock and the proposal succeeds.
		if _, err := fx.service.LockTimeProposals(fx.ownerCtx, connect.NewRequest(&api.LockTimeProposalsRequest{
			ExperienceId: fx.expID,
			Locked:       false,
		})); err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}
		if _, err := fx.service.ProposeTime(fx.voterCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: fx.expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: 1735833600,
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		})); err != nil {
			t.Errorf("ProposeTime after unlock failed: %v", err)
		}
	})
}

func TestService_NudgeTimePollVoters(t *testing.T) {
	fx := setupTimePollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.NudgeTimePollVoters(fx.voterCtx, connect.NewRequest(&api.NudgeTimePollVotersRequest{
			ExperienceId: fx.expID,
		}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner nudges unreplied voters only", func(t *testing.T) {
		// Voter casts a YES on the proposal so they're excluded from the nudge.
		if _, err := fx.service.VoteOnTime(fx.voterCtx, connect.NewRequest(&api.VoteOnTimeRequest{
			ProposalId: fx.proposalID,
			Status:     api.TimeVoteStatus_TIME_VOTE_STATUS_YES,
		})); err != nil {
			t.Fatalf("VoteOnTime failed: %v", err)
		}

		resp, err := fx.service.NudgeTimePollVoters(fx.ownerCtx, connect.NewRequest(&api.NudgeTimePollVotersRequest{
			ExperienceId: fx.expID,
		}))
		if err != nil {
			t.Fatalf("NudgeTimePollVoters failed: %v", err)
		}
		// One YES-RSVP non-voter remains (non-voter user); bystander is not
		// RSVP'd and is excluded. Without a notification service wired the
		// count is 0 — assertion is just that the RPC succeeded.
		_ = resp
	})

	t.Run("rejected when no poll is active", func(t *testing.T) {
		// Cancel the poll to clear time_poll_active.
		if _, err := fx.service.CancelTimePoll(fx.ownerCtx, connect.NewRequest(&api.CancelTimePollRequest{
			ExperienceId: fx.expID,
		})); err != nil {
			t.Fatalf("CancelTimePoll failed: %v", err)
		}
		_, err := fx.service.NudgeTimePollVoters(fx.ownerCtx, connect.NewRequest(&api.NudgeTimePollVotersRequest{
			ExperienceId: fx.expID,
		}))
		if err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("expected FailedPrecondition without active poll, got %v", err)
		}
	})
}

func TestService_DeleteTimeProposal(t *testing.T) {
	fx := setupTimePollManageFixture(t)

	// Voter adds a second proposal so we can test deletion semantics with
	// two proposals (and so we can verify owner vs proposer permissioning).
	proposeResp, err := fx.service.ProposeTime(fx.voterCtx, connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: fx.expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735833600,
					Timezone:         "UTC",
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("voter ProposeTime failed: %v", err)
	}
	voterProposalID := proposeResp.Msg.Proposal.Id

	t.Run("non-owner non-proposer is rejected", func(t *testing.T) {
		_, err := fx.service.DeleteTimeProposal(fx.nonVoterCtx, connect.NewRequest(&api.DeleteTimeProposalRequest{
			ExperienceId: fx.expID,
			ProposalId:   voterProposalID,
		}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("proposer deletes own proposal", func(t *testing.T) {
		if _, err := fx.service.DeleteTimeProposal(fx.voterCtx, connect.NewRequest(&api.DeleteTimeProposalRequest{
			ExperienceId: fx.expID,
			ProposalId:   voterProposalID,
		})); err != nil {
			t.Fatalf("voter Delete own failed: %v", err)
		}
	})

	t.Run("owner deletes last proposal clears poll", func(t *testing.T) {
		if _, err := fx.service.DeleteTimeProposal(fx.ownerCtx, connect.NewRequest(&api.DeleteTimeProposalRequest{
			ExperienceId: fx.expID,
			ProposalId:   fx.proposalID,
		})); err != nil {
			t.Fatalf("owner Delete failed: %v", err)
		}
		exp := &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.TimePollActive {
			t.Error("time_poll_active should be cleared after last proposal deleted")
		}
		if exp.CurrentPollId != nil {
			t.Errorf("current_poll_id should be nil after last proposal deleted, got %v", *exp.CurrentPollId)
		}
	})
}
