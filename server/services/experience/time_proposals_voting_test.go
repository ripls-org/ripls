package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_VoteOnTime(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)
	user2Ctx := createAuthenticatedContext("user789", "user789@example.com", models.Role_ROLE_USER)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "User One")
	createTestUser(t, testStorage, "user789", "user789@example.com", "User Two")

	// Create community and add users
	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")
	createTestCommunityMembership(t, testStorage, communityID, "user789")

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Vote Test Experience",
		Description: "Test voting functionality",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Users RSVP Yes
	for _, userID := range []string{"user456", "user789"} {
		ctx := createAuthenticatedContext(userID, userID+"@example.com", models.Role_ROLE_USER)
		rsvpReq := connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		})
		_, err = service.RSVPToExperience(ctx, rsvpReq)
		if err != nil {
			t.Fatalf("Failed to RSVP for %s: %v", userID, err)
		}
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

	tests := []struct {
		name   string
		ctx    context.Context
		status api.TimeVoteStatus
		want   error
	}{
		{
			name:   "user can vote YES",
			ctx:    user1Ctx,
			status: api.TimeVoteStatus_TIME_VOTE_STATUS_YES,
			want:   nil,
		},
		{
			name:   "user can vote NO",
			ctx:    user2Ctx,
			status: api.TimeVoteStatus_TIME_VOTE_STATUS_NO,
			want:   nil,
		},
		{
			name:   "user can change vote",
			ctx:    user1Ctx,
			status: api.TimeVoteStatus_TIME_VOTE_STATUS_NO,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voteReq := connect.NewRequest(&api.VoteOnTimeRequest{
				ProposalId: proposalID,
				Status:     tt.status,
			})

			_, err := service.VoteOnTime(tt.ctx, voteReq)
			if (err != nil) != (tt.want != nil) {
				t.Errorf("VoteOnTime() error = %v, wantErr %v", err, tt.want)
			}
		})
	}

	t.Run("votes are persisted and queryable", func(t *testing.T) {
		// Get the experience to verify votes are included
		getReq := connect.NewRequest(&api.GetExperienceRequest{
			Id:          expID,
			CommunityId: communityID,
		})
		getResp, err := service.GetExperience(ownerCtx, getReq)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}

		if len(getResp.Msg.Experience.TimeProposals) == 0 {
			t.Fatal("Expected time proposals in response")
		}

		proposal := getResp.Msg.Experience.TimeProposals[0]
		if len(proposal.Votes) != 2 {
			t.Errorf("Expected 2 votes, got %d", len(proposal.Votes))
		}
	})
}

func TestService_ConfirmTime(t *testing.T) {
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
		Name:        "Confirm Test Experience",
		Description: "Test time confirmation",
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

	// Create multiple time proposals
	proposal1Req := connect.NewRequest(&api.ProposeTimeRequest{
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
	proposal1Resp, err := service.ProposeTime(ownerCtx, proposal1Req)
	if err != nil {
		t.Fatalf("Failed to create proposal 1: %v", err)
	}
	proposal1ID := proposal1Resp.Msg.Proposal.Id

	proposal2Req := connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735833600,
					Timezone:         "UTC",
					DurationMinutes:  90,
				},
			},
		},
	})
	proposal2Resp, err := service.ProposeTime(ownerCtx, proposal2Req)
	if err != nil {
		t.Fatalf("Failed to create proposal 2: %v", err)
	}
	proposal2ID := proposal2Resp.Msg.Proposal.Id

	t.Run("owner can confirm time", func(t *testing.T) {
		confirmReq := connect.NewRequest(&api.ConfirmTimeRequest{
			ExperienceId: expID,
			ProposalId:   proposal1ID,
		})

		_, err := service.ConfirmTime(ownerCtx, confirmReq)
		if err != nil {
			t.Fatalf("ConfirmTime failed: %v", err)
		}

		// Verify proposal is marked as confirmed
		proposals, err := testStorage.QueryByField(ownerCtx, "id", proposal1ID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal: %v", err)
		}
		proposal := proposals[0].(*models.ExperienceTimeProposal)
		if !proposal.IsConfirmed {
			t.Error("Proposal should be marked as confirmed")
		}

		// Verify experience time was updated
		exp := &models.Experience{}
		err = testStorage.GetByID(ownerCtx, expID, exp)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}
		if exp.Time.GetSpecific().UnixTimestampSec != 1735747200 {
			t.Error("Experience time should be updated to confirmed proposal time")
		}
	})

	t.Run("confirming unmarks other proposals", func(t *testing.T) {
		confirmReq := connect.NewRequest(&api.ConfirmTimeRequest{
			ExperienceId: expID,
			ProposalId:   proposal2ID,
		})

		_, err := service.ConfirmTime(ownerCtx, confirmReq)
		if err != nil {
			t.Fatalf("ConfirmTime failed: %v", err)
		}

		// Verify proposal 1 is no longer confirmed
		proposals, err := testStorage.QueryByField(ownerCtx, "id", proposal1ID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal: %v", err)
		}
		proposal := proposals[0].(*models.ExperienceTimeProposal)
		if proposal.IsConfirmed {
			t.Error("Previous proposal should be unmarked as confirmed")
		}

		// Verify proposal 2 is confirmed
		proposals2, err := testStorage.QueryByField(ownerCtx, "id", proposal2ID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal 2: %v", err)
		}
		proposal2 := proposals2[0].(*models.ExperienceTimeProposal)
		if !proposal2.IsConfirmed {
			t.Error("New proposal should be confirmed")
		}
	})

	t.Run("non-owner cannot confirm time", func(t *testing.T) {
		confirmReq := connect.NewRequest(&api.ConfirmTimeRequest{
			ExperienceId: expID,
			ProposalId:   proposal1ID,
		})

		_, err := service.ConfirmTime(userCtx, confirmReq)
		if err == nil {
			t.Fatal("Expected error when non-owner confirms time")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connect.CodeOf(err))
		}
	})
}

// TestService_SaveExperienceClearsStaleConfirmation guards the time-discrepancy
// bug: ConfirmTime locks experience.time to a confirmed proposal, but a later
// direct time edit via SaveExperience must not leave that proposal's IsConfirmed
// flag set — otherwise the client's "When" panel renders the stale proposal time
// instead of the edited experience time.
func TestService_SaveExperienceClearsStaleConfirmation(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	// Create + share an experience, then RSVP and propose a time.
	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Stale Confirmation Experience",
		Description: "Edit time after confirming",
	}))
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)
	if _, err := service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})); err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	const confirmedTimestamp = int64(1735747200) // proposal time we will confirm
	proposalResp, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: confirmedTimestamp,
					Timezone:         "UTC",
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("Failed to propose time: %v", err)
	}
	proposalID := proposalResp.Msg.Proposal.Id

	if _, err := service.ConfirmTime(ownerCtx, connect.NewRequest(&api.ConfirmTimeRequest{
		ExperienceId: expID,
		ProposalId:   proposalID,
	})); err != nil {
		t.Fatalf("ConfirmTime failed: %v", err)
	}

	isConfirmed := func(t *testing.T) bool {
		t.Helper()
		proposals, err := testStorage.QueryByField(ownerCtx, "id", proposalID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query proposal: %v", err)
		}
		return proposals[0].(*models.ExperienceTimeProposal).IsConfirmed
	}

	// Sanity: after ConfirmTime the proposal is confirmed.
	if !isConfirmed(t) {
		t.Fatal("Proposal should be confirmed after ConfirmTime")
	}

	t.Run("re-saving the same time keeps the confirmation", func(t *testing.T) {
		if _, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &expID,
			Time: &api.ExperienceTime{TimeType: &api.ExperienceTime_Specific{Specific: &api.SpecificTime{UnixTimestampSec: confirmedTimestamp, Timezone: "UTC", DurationMinutes: 60}}},
		})); err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}
		if !isConfirmed(t) {
			t.Error("Confirmation should survive re-saving the identical time")
		}
	})

	t.Run("editing the time clears the stale confirmation", func(t *testing.T) {
		const editedTimestamp = int64(1735833600) // a different time
		saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &expID,
			Time: &api.ExperienceTime{TimeType: &api.ExperienceTime_Specific{Specific: &api.SpecificTime{UnixTimestampSec: editedTimestamp, Timezone: "UTC", DurationMinutes: 60}}},
		}))
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		// Experience time reflects the edit ...
		if got := saveResp.Msg.Experience.Time.GetSpecific().UnixTimestampSec; got != editedTimestamp {
			t.Errorf("Experience time = %d, want %d", got, editedTimestamp)
		}
		// ... and the previously-confirmed proposal is no longer confirmed, so the
		// "When" panel won't surface the stale proposal time.
		if isConfirmed(t) {
			t.Error("Confirmation should be cleared after the time is edited away from it")
		}
	})
}
