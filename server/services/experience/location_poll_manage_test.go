package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// locationPollManageFixture is the shared scenario for organizer-manage
// tests: an experience with a community of (owner, voter, non-voter,
// bystander), the voter and non-voter both RSVP'd YES, and the owner has
// opened a location poll with one proposal. Mirrors timePollManageFixture
// so the two flows' manage RPCs get equivalent coverage.
type locationPollManageFixture struct {
	service     *Service
	testStorage *storage.ProtoSQLStorage
	expID       string
	proposalID  string
	communityID string

	ownerID, voterID, nonVoterID, bystanderID     string
	ownerCtx, voterCtx, nonVoterCtx, bystanderCtx context.Context
}

func setupLocationPollManageFixture(t *testing.T) *locationPollManageFixture {
	t.Helper()
	service, testStorage, _ := setupTestService(t)

	fx := &locationPollManageFixture{
		service:     service,
		testStorage: testStorage,
		ownerID:     "loc-manage-owner",
		voterID:     "loc-manage-voter",
		nonVoterID:  "loc-manage-nonvoter",
		bystanderID: "loc-manage-bystander",
	}

	createTestUser(t, testStorage, fx.ownerID, "owner@example.com", "Owner")
	createTestUser(t, testStorage, fx.voterID, "voter@example.com", "Voter")
	createTestUser(t, testStorage, fx.nonVoterID, "nonvoter@example.com", "NonVoter")
	createTestUser(t, testStorage, fx.bystanderID, "bystander@example.com", "Bystander")

	fx.communityID = createTestCommunity(t, testStorage, "Loc Manage Test", fx.ownerID)
	for _, uid := range []string{fx.ownerID, fx.voterID, fx.nonVoterID, fx.bystanderID} {
		createTestCommunityMembership(t, testStorage, fx.communityID, uid)
	}

	fx.ownerCtx = createAuthenticatedContext(fx.ownerID, "owner@example.com", models.Role_ROLE_USER)
	fx.voterCtx = createAuthenticatedContext(fx.voterID, "voter@example.com", models.Role_ROLE_USER)
	fx.nonVoterCtx = createAuthenticatedContext(fx.nonVoterID, "nonvoter@example.com", models.Role_ROLE_USER)
	fx.bystanderCtx = createAuthenticatedContext(fx.bystanderID, "bystander@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(fx.ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Loc Manage Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	fx.expID = createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, fx.ownerCtx, fx.expID, fx.communityID)

	for _, c := range []context.Context{fx.voterCtx, fx.nonVoterCtx} {
		if _, err := service.RSVPToExperience(c, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: fx.expID,
			CommunityId:  fx.communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})); err != nil {
			t.Fatalf("RSVP failed: %v", err)
		}
	}

	proposeResp, err := service.ProposeLocation(fx.ownerCtx,
		makeLocationProposal(fx.expID, "Owner Spot", 37.0, -122.0))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}
	fx.proposalID = proposeResp.Msg.Proposal.Id

	return fx
}

func TestService_SetLocationPollDeadline(t *testing.T) {
	fx := setupLocationPollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.SetLocationPollDeadline(fx.voterCtx,
			connect.NewRequest(&api.SetLocationPollDeadlineRequest{
				ExperienceId:    fx.expID,
				DeadlineUnixSec: 1735900000,
			}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner sets and clears deadline", func(t *testing.T) {
		_, err := fx.service.SetLocationPollDeadline(fx.ownerCtx,
			connect.NewRequest(&api.SetLocationPollDeadlineRequest{
				ExperienceId:    fx.expID,
				DeadlineUnixSec: 1735900000,
			}))
		if err != nil {
			t.Fatalf("SetLocationPollDeadline failed: %v", err)
		}

		exp := &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.LocationPollDeadlineUnixSec == nil ||
			*exp.LocationPollDeadlineUnixSec != 1735900000 {
			t.Errorf("expected deadline 1735900000, got %v",
				exp.LocationPollDeadlineUnixSec)
		}

		// Clear via zero.
		if _, err := fx.service.SetLocationPollDeadline(fx.ownerCtx,
			connect.NewRequest(&api.SetLocationPollDeadlineRequest{
				ExperienceId:    fx.expID,
				DeadlineUnixSec: 0,
			})); err != nil {
			t.Fatalf("clear deadline failed: %v", err)
		}
		exp = &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID after clear: %v", err)
		}
		if exp.LocationPollDeadlineUnixSec != nil {
			t.Errorf("expected nil deadline after clear, got %v",
				*exp.LocationPollDeadlineUnixSec)
		}
	})

	t.Run("empty experience_id returns InvalidArgument", func(t *testing.T) {
		_, err := fx.service.SetLocationPollDeadline(fx.ownerCtx,
			connect.NewRequest(&api.SetLocationPollDeadlineRequest{}))
		if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})
}

func TestService_LockLocationProposals(t *testing.T) {
	fx := setupLocationPollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.LockLocationProposals(fx.voterCtx,
			connect.NewRequest(&api.LockLocationProposalsRequest{
				ExperienceId: fx.expID,
				Locked:       true,
			}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner locks then unlocks; ProposeLocation rejected while locked",
		func(t *testing.T) {
			if _, err := fx.service.LockLocationProposals(fx.ownerCtx,
				connect.NewRequest(&api.LockLocationProposalsRequest{
					ExperienceId: fx.expID,
					Locked:       true,
				})); err != nil {
				t.Fatalf("Lock failed: %v", err)
			}

			exp := &models.Experience{}
			if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if exp.LocationProposalsLocked == nil || !*exp.LocationProposalsLocked {
				t.Errorf("expected locked=true, got %v", exp.LocationProposalsLocked)
			}

			// ProposeLocation is rejected while locked.
			_, err := fx.service.ProposeLocation(fx.voterCtx,
				makeLocationProposal(fx.expID, "Locked Out", 37.1, -122.1))
			if err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
				t.Fatalf("expected FailedPrecondition when locked, got %v", err)
			}

			// Unlock and the proposal succeeds.
			if _, err := fx.service.LockLocationProposals(fx.ownerCtx,
				connect.NewRequest(&api.LockLocationProposalsRequest{
					ExperienceId: fx.expID,
					Locked:       false,
				})); err != nil {
				t.Fatalf("Unlock failed: %v", err)
			}
			if _, err := fx.service.ProposeLocation(fx.voterCtx,
				makeLocationProposal(fx.expID, "Unlocked", 37.2, -122.2)); err != nil {
				t.Errorf("ProposeLocation after unlock failed: %v", err)
			}
		})
}

func TestService_NudgeLocationPollVoters(t *testing.T) {
	fx := setupLocationPollManageFixture(t)

	t.Run("non-owner is rejected", func(t *testing.T) {
		_, err := fx.service.NudgeLocationPollVoters(fx.voterCtx,
			connect.NewRequest(&api.NudgeLocationPollVotersRequest{
				ExperienceId: fx.expID,
			}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("owner nudges unreplied voters only", func(t *testing.T) {
		// Voter casts a YES so they're excluded from the nudge.
		if _, err := fx.service.VoteOnLocation(fx.voterCtx,
			connect.NewRequest(&api.VoteOnLocationRequest{
				ProposalId: fx.proposalID,
				Status:     api.LocationVoteStatus_LOCATION_VOTE_STATUS_YES,
			})); err != nil {
			t.Fatalf("VoteOnLocation failed: %v", err)
		}

		// Same-package test, so wire the recording double directly; the
		// service takes its notifier through the constructor.
		mockNotify := notifications.NewMockService()
		prev := fx.service.notificationService
		fx.service.notificationService = mockNotify
		t.Cleanup(func() { fx.service.notificationService = prev })

		resp, err := fx.service.NudgeLocationPollVoters(fx.ownerCtx,
			connect.NewRequest(&api.NudgeLocationPollVotersRequest{
				ExperienceId: fx.expID,
			}))
		if err != nil {
			t.Fatalf("NudgeLocationPollVoters failed: %v", err)
		}

		// The non-voter RSVPed but never voted on the poll, so they are the
		// one candidate. The voter voted YES and the owner is excluded.
		//
		// This assertion is the point of the test. It previously only
		// checked that the RPC returned without error, which passed for
		// months while the candidate switch compared rsvp.Intention (which
		// stores "YES") against "RSVP_INTENTION_YES" and therefore matched
		// nobody — the nudge silently reached zero people.
		if resp.Msg.NudgedCount != 1 {
			t.Errorf("NudgedCount = %d, want 1", resp.Msg.NudgedCount)
		}
		calls := mockNotify.GetCalls()
		if len(calls) != 1 {
			t.Fatalf("notification calls = %d, want 1", len(calls))
		}
		if calls[0].UserID != fx.nonVoterID {
			t.Errorf("nudged %q, want the non-voter %q", calls[0].UserID, fx.nonVoterID)
		}
	})

	t.Run("rejected when no poll is active", func(t *testing.T) {
		if _, err := fx.service.CancelLocationPoll(fx.ownerCtx,
			connect.NewRequest(&api.CancelLocationPollRequest{
				ExperienceId: fx.expID,
			})); err != nil {
			t.Fatalf("CancelLocationPoll failed: %v", err)
		}
		_, err := fx.service.NudgeLocationPollVoters(fx.ownerCtx,
			connect.NewRequest(&api.NudgeLocationPollVotersRequest{
				ExperienceId: fx.expID,
			}))
		if err == nil || connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("expected FailedPrecondition without active poll, got %v", err)
		}
	})
}

// TestService_FlexibleVoteCountsAsReplied verifies that a FLEXIBLE
// ("any spot works") vote marks the voter as having replied to the poll, so
// the nudge set excludes them just as it does an explicit YES voter.
func TestService_FlexibleVoteCountsAsReplied(t *testing.T) {
	fx := setupLocationPollManageFixture(t)

	if _, err := fx.service.VoteOnLocation(fx.voterCtx,
		connect.NewRequest(&api.VoteOnLocationRequest{
			ProposalId: fx.proposalID,
			Status:     api.LocationVoteStatus_LOCATION_VOTE_STATUS_FLEXIBLE,
		})); err != nil {
		t.Fatalf("VoteOnLocation (flexible) failed: %v", err)
	}

	expStored := &models.Experience{}
	if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, expStored); err != nil {
		t.Fatalf("GetByID experience failed: %v", err)
	}

	if expStored.CurrentLocationPollId == nil {
		t.Fatal("expected an active location poll")
	}
	voters, err := locationVotersOnPoll(fx.ownerCtx, fx.service, expStored, *expStored.CurrentLocationPollId)
	if err != nil {
		t.Fatalf("locationVotersOnPoll failed: %v", err)
	}
	if _, ok := voters[fx.voterID]; !ok {
		t.Fatalf("expected flexible voter %q to count as replied, got %v", fx.voterID, voters)
	}
}

func TestService_DeleteLocationProposal(t *testing.T) {
	fx := setupLocationPollManageFixture(t)

	// Voter adds a second proposal so we can test deletion semantics with
	// two proposals (and verify owner vs proposer permissioning).
	proposeResp, err := fx.service.ProposeLocation(fx.voterCtx,
		makeLocationProposal(fx.expID, "Voter Spot", 37.5, -122.5))
	if err != nil {
		t.Fatalf("voter ProposeLocation failed: %v", err)
	}
	voterProposalID := proposeResp.Msg.Proposal.Id

	t.Run("non-owner non-proposer is rejected", func(t *testing.T) {
		_, err := fx.service.DeleteLocationProposal(fx.nonVoterCtx,
			connect.NewRequest(&api.DeleteLocationProposalRequest{
				ExperienceId: fx.expID,
				ProposalId:   voterProposalID,
			}))
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("proposer deletes own proposal", func(t *testing.T) {
		if _, err := fx.service.DeleteLocationProposal(fx.voterCtx,
			connect.NewRequest(&api.DeleteLocationProposalRequest{
				ExperienceId: fx.expID,
				ProposalId:   voterProposalID,
			})); err != nil {
			t.Fatalf("voter Delete own failed: %v", err)
		}
	})

	t.Run("owner deletes last proposal clears poll", func(t *testing.T) {
		if _, err := fx.service.DeleteLocationProposal(fx.ownerCtx,
			connect.NewRequest(&api.DeleteLocationProposalRequest{
				ExperienceId: fx.expID,
				ProposalId:   fx.proposalID,
			})); err != nil {
			t.Fatalf("owner Delete failed: %v", err)
		}
		exp := &models.Experience{}
		if err := fx.testStorage.GetByID(fx.ownerCtx, fx.expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.LocationPollActive {
			t.Error("location_poll_active should be cleared after last proposal deleted")
		}
		if exp.CurrentLocationPollId != nil {
			t.Errorf("current_location_poll_id should be nil, got %v",
				*exp.CurrentLocationPollId)
		}
	})
}
