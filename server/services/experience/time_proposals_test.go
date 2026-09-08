package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestDefaultTimePollDeadline mirrors TestDefaultLocationPollDeadline: the
// helper that seeds the 24-hour reply-by default must produce now + 24h.
func TestDefaultTimePollDeadline(t *testing.T) {
	const now int64 = 1_700_000_000
	const day int64 = 24 * 60 * 60
	want := now + day
	if got := defaultTimePollDeadline(now); got != want {
		t.Errorf("defaultTimePollDeadline(%d) = %d, want %d", now, got, want)
	}
}

// TestService_ProposeTime_SeedsDeadline checks that the first ProposeTime
// of a fresh poll persists a 24h-from-now deadline on the experience —
// matching the location flow. Subsequent proposals in the same poll must
// not overwrite the seeded value (so an owner who manually changed the
// deadline isn't reset when a participant adds another option).
func TestService_ProposeTime_SeedsDeadline(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	communityID := createTestCommunity(t, testStorage, "Deadline Test", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Deadline Seed Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	t.Run("first ProposeTime seeds a non-nil deadline", func(t *testing.T) {
		if _, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
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
		})); err != nil {
			t.Fatalf("ProposeTime failed: %v", err)
		}
		exp := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if exp.TimePollDeadlineUnixSec == nil || *exp.TimePollDeadlineUnixSec == 0 {
			t.Errorf("expected deadline to be seeded, got %v", exp.TimePollDeadlineUnixSec)
		}
		// The seeded value should be roughly 24h in the future. Use a
		// generous window to absorb clock skew between when the server
		// stamped it and when we read it back.
		const day = int64(24 * 60 * 60)
		approxNow := *exp.TimePollDeadlineUnixSec - day
		if approxNow <= 0 {
			t.Errorf("seeded deadline isn't ~now+24h: got %d", *exp.TimePollDeadlineUnixSec)
		}
	})

	t.Run("subsequent ProposeTime does not overwrite a manually-set deadline",
		func(t *testing.T) {
			// Owner manually overrides the deadline to a known value.
			const manualDeadline = int64(1_900_000_000)
			if _, err := service.SetTimePollDeadline(ownerCtx,
				connect.NewRequest(&api.SetTimePollDeadlineRequest{
					ExperienceId:    expID,
					DeadlineUnixSec: manualDeadline,
				})); err != nil {
				t.Fatalf("SetTimePollDeadline failed: %v", err)
			}

			// Owner adds another option to the same active poll.
			if _, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
				ExperienceId: expID,
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
				t.Fatalf("second ProposeTime failed: %v", err)
			}

			exp := &models.Experience{}
			if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if exp.TimePollDeadlineUnixSec == nil ||
				*exp.TimePollDeadlineUnixSec != manualDeadline {
				t.Errorf("expected manual deadline %d preserved, got %v",
					manualDeadline, exp.TimePollDeadlineUnixSec)
			}
		})
}

func TestService_ProposeTime(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	yesUserCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)
	maybeUserCtx := createAuthenticatedContext("user789", "user789@example.com", models.Role_ROLE_USER)
	noRsvpUserCtx := createAuthenticatedContext("user999", "user999@example.com", models.Role_ROLE_USER)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Yes User")
	createTestUser(t, testStorage, "user789", "user789@example.com", "Maybe User")
	createTestUser(t, testStorage, "user999", "user999@example.com", "No RSVP User")

	// Create community and add users
	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")
	createTestCommunityMembership(t, testStorage, communityID, "user789")
	createTestCommunityMembership(t, testStorage, communityID, "user999")

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Time Proposal Test",
		Description: "Test time proposal functionality",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share to community
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// YES user RSVPs
	_, err = service.RSVPToExperience(yesUserCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("Failed to RSVP YES: %v", err)
	}

	// MAYBE user RSVPs
	_, err = service.RSVPToExperience(maybeUserCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
	}))
	if err != nil {
		t.Fatalf("Failed to RSVP MAYBE: %v", err)
	}

	makeProposal := func(ts int64) *connect.Request[api.ProposeTimeRequest] {
		return connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: ts,
						Timezone:         "America/Los_Angeles",
						DurationMinutes:  60,
					},
				},
			},
		})
	}

	t.Run("owner can propose time", func(t *testing.T) {
		resp, err := service.ProposeTime(ownerCtx, makeProposal(1735747200))
		if err != nil {
			t.Fatalf("ProposeTime failed: %v", err)
		}
		if resp.Msg.Proposal == nil {
			t.Fatal("Expected proposal in response")
		}
		if resp.Msg.Proposal.ProposedBy.Id != "owner123" {
			t.Errorf("Expected proposer owner123, got %s", resp.Msg.Proposal.ProposedBy.Id)
		}
		if resp.Msg.Proposal.IsConfirmed {
			t.Error("New proposal should not be confirmed")
		}
	})

	t.Run("owner first proposal sets time_poll_active", func(t *testing.T) {
		exp := &models.Experience{}
		if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}
		if !exp.TimePollActive {
			t.Error("time_poll_active should be true after owner proposes time")
		}
	})

	t.Run("YES RSVP user can propose time into live poll", func(t *testing.T) {
		resp, err := service.ProposeTime(yesUserCtx, makeProposal(1735833600))
		if err != nil {
			t.Fatalf("ProposeTime for YES RSVP user failed: %v", err)
		}
		if resp.Msg.Proposal.ProposedBy.Id != "user456" {
			t.Errorf("Expected proposer user456, got %s", resp.Msg.Proposal.ProposedBy.Id)
		}
	})

	t.Run("MAYBE RSVP user can propose time", func(t *testing.T) {
		resp, err := service.ProposeTime(maybeUserCtx, makeProposal(1735920000))
		if err != nil {
			t.Fatalf("ProposeTime for MAYBE RSVP user failed: %v", err)
		}
		if resp.Msg.Proposal.ProposedBy.Id != "user789" {
			t.Errorf("Expected proposer user789, got %s", resp.Msg.Proposal.ProposedBy.Id)
		}
	})

	t.Run("community member with no RSVP is rejected", func(t *testing.T) {
		_, err := service.ProposeTime(noRsvpUserCtx, makeProposal(1736006400))
		if err == nil {
			t.Fatal("Expected error for user with no RSVP")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})
}

func TestService_ProposeTime_NonOwnerStartsPoll(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Non-owner Start Poll Test",
	}))
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	_, err = service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	// Non-owner starts the first proposal (no poll was active before).
	resp, err := service.ProposeTime(userCtx, connect.NewRequest(&api.ProposeTimeRequest{
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
	}))
	if err != nil {
		t.Fatalf("Non-owner ProposeTime failed: %v", err)
	}
	if resp.Msg.Proposal.ProposedBy.Id != "user456" {
		t.Errorf("Expected proposer user456, got %s", resp.Msg.Proposal.ProposedBy.Id)
	}

	// First proposal by non-owner must activate the poll.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if !exp.TimePollActive {
		t.Error("time_poll_active should be true after non-owner's first proposal")
	}
}

func TestService_ProposeTime_TerminalState(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	userCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Test User")

	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Terminal State Test",
	}))
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	_, err = service.RSVPToExperience(userCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("Failed to RSVP: %v", err)
	}

	// Force-cancel the experience to put it in terminal state.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	exp.State = models.ExperienceState_EXPERIENCE_STATE_CANCELLED
	if err := testStorage.Update(ownerCtx, exp); err != nil {
		t.Fatalf("Failed to cancel experience: %v", err)
	}

	makeProposal := func(ctx context.Context) error {
		_, err := service.ProposeTime(ctx, connect.NewRequest(&api.ProposeTimeRequest{
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
		}))
		return err
	}

	t.Run("owner rejected in terminal state", func(t *testing.T) {
		err := makeProposal(ownerCtx)
		if err == nil {
			t.Fatal("Expected error for terminal-state experience")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition, got %v", connect.CodeOf(err))
		}
	})

	t.Run("YES RSVP user rejected in terminal state", func(t *testing.T) {
		err := makeProposal(userCtx)
		if err == nil {
			t.Fatal("Expected error for terminal-state experience")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition, got %v", connect.CodeOf(err))
		}
	})
}
