package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

func TestService_UnlockTime(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	// Create community and add users
	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Unlock Test Experience",
		Description: "Test time unlocking",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// User RSVPs Yes
	rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(userCtx, rsvpReq)
	if err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	// Create a time proposal
	proposeReq := connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735747200,
					Timezone:         "UTC",
					DurationMinutes:  60,
				},
			},
		},
	})
	proposeResp, err := service.ProposeTime(ownerCtx, proposeReq)
	if err != nil {
		t.Fatalf("Failed to create proposal: %v", err)
	}
	proposalID := proposeResp.Msg.Proposal.Id

	// Confirm the time proposal
	confirmReq := connect.NewRequest(&api.ConfirmTimeRequest{
		ExperienceId: expID,
		ProposalId:   proposalID,
	})
	_, err = service.ConfirmTime(ownerCtx, confirmReq)
	if err != nil {
		t.Fatalf("Failed to confirm time: %v", err)
	}

	// Verify proposal is confirmed
	proposals, err := testStorage.QueryByField(ownerCtx, "id", proposalID, &models.ExperienceTimeProposal{})
	if err != nil {
		t.Fatalf("Failed to query proposal: %v", err)
	}
	proposal := proposals[0].(*models.ExperienceTimeProposal)
	if !proposal.IsConfirmed {
		t.Fatal("Proposal should be confirmed before unlock test")
	}

	t.Run("owner can unlock confirmed time", func(t *testing.T) {
		unlockReq := connect.NewRequest(&api.UnlockTimeRequest{
			ExperienceId: expID,
		})

		_, err := service.UnlockTime(ownerCtx, unlockReq)
		if err != nil {
			t.Fatalf("UnlockTime failed: %v", err)
		}

		// Verify proposal is no longer confirmed
		proposals, err := testStorage.QueryByField(ownerCtx, "id", proposalID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal: %v", err)
		}
		proposal := proposals[0].(*models.ExperienceTimeProposal)
		if proposal.IsConfirmed {
			t.Error("Proposal should be unlocked (is_confirmed = false)")
		}
	})

	t.Run("unlock when no proposals are confirmed is no-op", func(t *testing.T) {
		unlockReq := connect.NewRequest(&api.UnlockTimeRequest{
			ExperienceId: expID,
		})

		_, err := service.UnlockTime(ownerCtx, unlockReq)
		if err != nil {
			t.Fatalf("UnlockTime failed: %v", err)
		}

		// Verify no error when unlocking already-unlocked time
		proposals, err := testStorage.QueryByField(ownerCtx, "id", proposalID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal: %v", err)
		}
		proposal := proposals[0].(*models.ExperienceTimeProposal)
		if proposal.IsConfirmed {
			t.Error("Proposal should still be unlocked")
		}
	})

	t.Run("non-owner cannot unlock time", func(t *testing.T) {
		unlockReq := connect.NewRequest(&api.UnlockTimeRequest{
			ExperienceId: expID,
		})

		_, err := service.UnlockTime(userCtx, unlockReq)
		if err == nil {
			t.Fatal("Expected error when non-owner unlocks time")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("unlock non-existent experience returns NotFound", func(t *testing.T) {
		unlockReq := connect.NewRequest(&api.UnlockTimeRequest{
			ExperienceId: "nonexistent-exp-id",
		})

		_, err := service.UnlockTime(ownerCtx, unlockReq)
		if err == nil {
			t.Fatal("Expected error when unlocking non-existent experience")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("unlock with empty experience_id returns InvalidArgument", func(t *testing.T) {
		unlockReq := connect.NewRequest(&api.UnlockTimeRequest{
			ExperienceId: "",
		})

		_, err := service.UnlockTime(ownerCtx, unlockReq)
		if err == nil {
			t.Fatal("Expected error when experience_id is empty")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connect.CodeOf(err))
		}
	})
}

func TestService_CancelTimePoll(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	// Create community and add users
	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Cancel Poll Test Experience",
		Description: "Test poll cancellation",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Owner proposes two times to start a poll
	for _, ts := range []int64{1735747200, 1735833600} {
		proposeReq := connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: ts,
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		})
		if _, err := service.ProposeTime(ownerCtx, proposeReq); err != nil {
			t.Fatalf("Failed to propose time: %v", err)
		}
	}

	// Verify poll is active
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if !exp.TimePollActive {
		t.Fatal("time_poll_active should be true before cancel")
	}

	t.Run("non-owner cannot cancel poll", func(t *testing.T) {
		cancelReq := connect.NewRequest(&api.CancelTimePollRequest{
			ExperienceId: expID,
		})
		_, err := service.CancelTimePoll(userCtx, cancelReq)
		if err == nil {
			t.Fatal("Expected error when non-owner cancels poll")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("owner can end poll and proposals are preserved", func(t *testing.T) {
		cancelReq := connect.NewRequest(&api.CancelTimePollRequest{
			ExperienceId: expID,
		})
		_, err := service.CancelTimePoll(ownerCtx, cancelReq)
		if err != nil {
			t.Fatalf("CancelTimePoll failed: %v", err)
		}

		// Verify time_poll_active is cleared and the poll is marked completed.
		exp := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}
		if exp.TimePollActive {
			t.Error("time_poll_active should be false after end")
		}
		if exp.TimePollCompleted == nil || !*exp.TimePollCompleted {
			t.Error("time_poll_completed should be true after end")
		}

		// Proposals must be preserved so the read-only results view still has
		// data to show after the poll ends.
		proposals, err := testStorage.QueryByField(ownerCtx, "experience_id", expID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposals: %v", err)
		}
		if len(proposals) != 2 {
			t.Errorf("Expected 2 proposals preserved after end, got %d", len(proposals))
		}
	})

	t.Run("cancel with empty experience_id returns InvalidArgument", func(t *testing.T) {
		cancelReq := connect.NewRequest(&api.CancelTimePollRequest{
			ExperienceId: "",
		})
		_, err := service.CancelTimePoll(ownerCtx, cancelReq)
		if err == nil {
			t.Fatal("Expected error when experience_id is empty")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connect.CodeOf(err))
		}
	})
}

// TestBuildTimeProposalsFromMaps tests the maps-based time-proposal assembly helper.
func TestBuildTimeProposalsFromMaps(t *testing.T) {
	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)

	userAlice := &api.User{Id: "alice", Name: "Alice"}
	userBob := &api.User{Id: "bob", Name: "Bob"}
	userMap := map[string]*api.User{"alice": userAlice, "bob": userBob}

	tbdTime := &models.ExperienceTime{TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}}}

	t.Run("no proposals", func(t *testing.T) {
		result := buildTimeProposalsFromMaps(ctx, "exp1",
			map[string][]*models.ExperienceTimeProposal{},
			map[string][]*models.TimeVote{},
			userMap, logger)
		if len(result) != 0 {
			t.Errorf("expected 0 proposals, got %d", len(result))
		}
	})

	t.Run("proposals with no votes", func(t *testing.T) {
		proposals := map[string][]*models.ExperienceTimeProposal{
			"exp1": {
				{Id: "p1", ExperienceId: "exp1", ProposedByUserId: "alice", Time: tbdTime, ProposedAtUnixSec: 100},
			},
		}
		result := buildTimeProposalsFromMaps(ctx, "exp1", proposals,
			map[string][]*models.TimeVote{}, userMap, logger)
		if len(result) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(result))
		}
		if result[0].Id != "p1" {
			t.Errorf("expected proposal ID p1, got %s", result[0].Id)
		}
		if len(result[0].Votes) != 0 {
			t.Errorf("expected 0 votes, got %d", len(result[0].Votes))
		}
		if result[0].ProposedBy.Id != "alice" {
			t.Errorf("expected proposer alice, got %s", result[0].ProposedBy.Id)
		}
	})

	t.Run("votes with missing user are skipped", func(t *testing.T) {
		proposals := map[string][]*models.ExperienceTimeProposal{
			"exp1": {
				{Id: "p1", ExperienceId: "exp1", ProposedByUserId: "alice", Time: tbdTime},
			},
		}
		votes := map[string][]*models.TimeVote{
			"p1": {
				{ProposalId: "p1", UserId: "unknown-user", Status: models.TimeVoteStatus_TIME_VOTE_STATUS_YES},
				{ProposalId: "p1", UserId: "bob", Status: models.TimeVoteStatus_TIME_VOTE_STATUS_YES},
			},
		}
		result := buildTimeProposalsFromMaps(ctx, "exp1", proposals, votes, userMap, logger)
		if len(result) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(result))
		}
		// Only bob's vote survives; unknown-user is skipped.
		if len(result[0].Votes) != 1 {
			t.Errorf("expected 1 vote (unknown skipped), got %d", len(result[0].Votes))
		}
		if result[0].Votes[0].User.Id != "bob" {
			t.Errorf("expected voter bob, got %s", result[0].Votes[0].User.Id)
		}
	})

	t.Run("confirmed proposal", func(t *testing.T) {
		proposals := map[string][]*models.ExperienceTimeProposal{
			"exp1": {
				{Id: "p2", ExperienceId: "exp1", ProposedByUserId: "alice", Time: tbdTime, IsConfirmed: true},
			},
		}
		result := buildTimeProposalsFromMaps(ctx, "exp1", proposals,
			map[string][]*models.TimeVote{}, userMap, logger)
		if len(result) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(result))
		}
		if !result[0].IsConfirmed {
			t.Error("expected IsConfirmed=true")
		}
	})

	t.Run("proposal with missing proposer is skipped", func(t *testing.T) {
		proposals := map[string][]*models.ExperienceTimeProposal{
			"exp1": {
				{Id: "p-missing", ExperienceId: "exp1", ProposedByUserId: "ghost", Time: tbdTime},
				{Id: "p-ok", ExperienceId: "exp1", ProposedByUserId: "alice", Time: tbdTime},
			},
		}
		result := buildTimeProposalsFromMaps(ctx, "exp1", proposals,
			map[string][]*models.TimeVote{}, userMap, logger)
		if len(result) != 1 {
			t.Fatalf("expected 1 proposal (ghost skipped), got %d", len(result))
		}
		if result[0].Id != "p-ok" {
			t.Errorf("expected proposal p-ok, got %s", result[0].Id)
		}
	})

	t.Run("poll_id preserved", func(t *testing.T) {
		pollID := "poll-abc"
		proposals := map[string][]*models.ExperienceTimeProposal{
			"exp1": {
				{Id: "p3", ExperienceId: "exp1", ProposedByUserId: "alice", Time: tbdTime, PollId: proto.String(pollID)},
			},
		}
		result := buildTimeProposalsFromMaps(ctx, "exp1", proposals,
			map[string][]*models.TimeVote{}, userMap, logger)
		if len(result) != 1 {
			t.Fatalf("expected 1 proposal, got %d", len(result))
		}
		if result[0].PollId == nil || *result[0].PollId != pollID {
			t.Errorf("expected PollId=%q, got %v", pollID, result[0].PollId)
		}
	})
}
