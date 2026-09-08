package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	storagepkg "go.ripls.org/ripls/server/storage"
)

func TestDefaultLocationPollDeadline(t *testing.T) {
	const now int64 = 1_700_000_000
	const day int64 = 24 * 60 * 60
	want := now + day
	if got := defaultLocationPollDeadline(now); got != want {
		t.Errorf("defaultLocationPollDeadline(%d) = %d, want %d", now, got, want)
	}
}

// makeLocationProposal builds a ProposeLocationRequest that carries a fresh
// geocoded proposal (drop-pin path).
func makeLocationProposal(expID, name string, lat, lng float64) *connect.Request[api.ProposeLocationRequest] {
	return connect.NewRequest(&api.ProposeLocationRequest{
		ExperienceId: expID,
		Location: &api.ProposedLocation{
			Geocoded: &api.GeocodedLocation{
				Name:         name,
				LatitudeDeg:  lat,
				LongitudeDeg: lng,
				Locality:     "Test City",
				RegionCode:   "CA",
			},
		},
	})
}

// setupLocationPollExperience creates an experience shared with a community,
// with the owner and one YES RSVP user already in place. Returns the
// experience id and community id.
func setupLocationPollExperience(t *testing.T, service *Service, testStorage *storagepkg.ProtoSQLStorage) (string, string) {
	t.Helper()

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	yesUserCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "Yes User")

	communityID := createTestCommunity(t, testStorage, "Loc Poll Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user456")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Location Poll Test",
	}))
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	_, err = service.RSVPToExperience(yesUserCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("Failed to RSVP YES: %v", err)
	}

	return expID, communityID
}

func TestService_ProposeLocation_OwnerOpensPoll(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	resp, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.7749, -122.4194))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
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

	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if !exp.LocationPollActive {
		t.Error("location_poll_active should be true after first proposal")
	}
	if exp.CurrentLocationPollId == nil || *exp.CurrentLocationPollId == "" {
		t.Error("current_location_poll_id should be set after first proposal")
	}
	if exp.LocationPollDeadlineUnixSec == nil {
		t.Error("location_poll_deadline_unix_sec should be seeded on first proposal")
	}
}

func TestService_ProposeLocation_YesUserCanPropose(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	yesUserCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	// Owner opens the poll first.
	if _, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0)); err != nil {
		t.Fatalf("Owner ProposeLocation failed: %v", err)
	}

	// YES RSVP user adds an option.
	resp, err := service.ProposeLocation(yesUserCtx, makeLocationProposal(expID, "Beach", 36.9, -122.1))
	if err != nil {
		t.Fatalf("YES user ProposeLocation failed: %v", err)
	}
	if resp.Msg.Proposal.ProposedBy.Id != "user456" {
		t.Errorf("Expected proposer user456, got %s", resp.Msg.Proposal.ProposedBy.Id)
	}

	// Both proposals should share the same poll_id.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if exp.CurrentLocationPollId == nil {
		t.Fatal("current_location_poll_id missing")
	}
	if resp.Msg.Proposal.PollId == nil || *resp.Msg.Proposal.PollId != *exp.CurrentLocationPollId {
		t.Errorf("Second proposal poll_id should match current_location_poll_id, got %v vs %s",
			resp.Msg.Proposal.PollId, *exp.CurrentLocationPollId)
	}
}

// TestService_ProposeLocation_NonParticipantCanPropose pins the behaviour
// that any authenticated user can add a candidate spot to a poll, not just
// the owner and YES/MAYBE RSVPs. A viewer's eventual attendance may be
// predicated on the location choice, so RSVP status is not a prerequisite
// for proposing.
func TestService_ProposeLocation_NonParticipantCanPropose(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	strangerCtx := createAuthenticatedContext("stranger789", "stranger@example.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "stranger789", "stranger@example.com", "Stranger User")

	expID, communityID := setupLocationPollExperience(t, service, testStorage)
	// The stranger is a community member but has not RSVPed — neither YES,
	// MAYBE, nor NO. They should still be able to propose a spot.
	createTestCommunityMembership(t, testStorage, communityID, "stranger789")

	// Owner opens the poll first so the stranger's call lands as an
	// addition rather than the first option.
	if _, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0)); err != nil {
		t.Fatalf("Owner ProposeLocation failed: %v", err)
	}

	resp, err := service.ProposeLocation(strangerCtx, makeLocationProposal(expID, "Cafe", 36.8, -122.2))
	if err != nil {
		t.Fatalf("Non-participant ProposeLocation should succeed, got: %v", err)
	}
	if resp.Msg.Proposal == nil {
		t.Fatal("Expected proposal in response")
	}
	if resp.Msg.Proposal.ProposedBy.Id != "stranger789" {
		t.Errorf("Expected proposer stranger789, got %s", resp.Msg.Proposal.ProposedBy.Id)
	}

	// The stranger's proposal should share the open poll's id, not start
	// a fresh one.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if exp.CurrentLocationPollId == nil {
		t.Fatal("current_location_poll_id missing after stranger proposal")
	}
	if resp.Msg.Proposal.PollId == nil ||
		*resp.Msg.Proposal.PollId != *exp.CurrentLocationPollId {
		t.Errorf("Stranger proposal poll_id should match current poll, got %v vs %s",
			resp.Msg.Proposal.PollId, *exp.CurrentLocationPollId)
	}
}

func TestService_ProposeLocation_RequiresLocationField(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	// Empty ProposedLocation (no location_id, no geocoded) should fail validation.
	_, err := service.ProposeLocation(ownerCtx, connect.NewRequest(&api.ProposeLocationRequest{
		ExperienceId: expID,
		Location:     &api.ProposedLocation{},
	}))
	if err == nil {
		t.Fatal("Expected validation error for empty ProposedLocation")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestService_VoteOnLocation(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	yesUserCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	proposalResp, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}
	proposalID := proposalResp.Msg.Proposal.Id

	// YES user votes YES.
	if _, err := service.VoteOnLocation(yesUserCtx, connect.NewRequest(&api.VoteOnLocationRequest{
		ProposalId: proposalID,
		Status:     api.LocationVoteStatus_LOCATION_VOTE_STATUS_YES,
	})); err != nil {
		t.Fatalf("VoteOnLocation failed: %v", err)
	}

	// Re-voting toggles to UNSPECIFIED (clears vote).
	if _, err := service.VoteOnLocation(yesUserCtx, connect.NewRequest(&api.VoteOnLocationRequest{
		ProposalId: proposalID,
		Status:     api.LocationVoteStatus_LOCATION_VOTE_STATUS_UNSPECIFIED,
	})); err != nil {
		t.Fatalf("Second VoteOnLocation failed: %v", err)
	}
}

func TestService_ConfirmLocation_GeocodedPath(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	proposalResp, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}
	proposalID := proposalResp.Msg.Proposal.Id

	if _, err := service.ConfirmLocation(ownerCtx, connect.NewRequest(&api.ConfirmLocationRequest{
		ExperienceId: expID,
		ProposalId:   proposalID,
	})); err != nil {
		t.Fatalf("ConfirmLocation failed: %v", err)
	}

	// Confirmation should materialize the geocoded proposal into a saved Location
	// and point the experience at it.
	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if exp.LocationId == "" {
		t.Fatal("experience.location_id should be set after confirm")
	}
	if exp.LocationPollActive {
		t.Error("location_poll_active should be cleared after confirm")
	}
	if exp.LocationPollCompleted == nil || !*exp.LocationPollCompleted {
		t.Error("location_poll_completed should be true after confirm")
	}

	// The saved Location row should exist.
	loc := &models.Location{}
	if err := testStorage.GetByID(ownerCtx, exp.LocationId, loc); err != nil {
		t.Fatalf("Materialized location not found: %v", err)
	}
	if loc.Name == nil || *loc.Name != "Park" {
		t.Errorf("Expected location name 'Park', got %v", loc.Name)
	}
}

func TestService_ConfirmLocation_OwnerOnly(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)
	yesUserCtx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	proposalResp, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}
	proposalID := proposalResp.Msg.Proposal.Id

	_, err = service.ConfirmLocation(yesUserCtx, connect.NewRequest(&api.ConfirmLocationRequest{
		ExperienceId: expID,
		ProposalId:   proposalID,
	}))
	if err == nil {
		t.Fatal("Expected PermissionDenied for non-owner confirm")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied, got %v", connect.CodeOf(err))
	}
}

func TestService_CancelLocationPoll(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	if _, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0)); err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}

	if _, err := service.CancelLocationPoll(ownerCtx, connect.NewRequest(&api.CancelLocationPollRequest{
		ExperienceId: expID,
	})); err != nil {
		t.Fatalf("CancelLocationPoll failed: %v", err)
	}

	exp := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, exp); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if exp.LocationPollActive {
		t.Error("location_poll_active should be cleared after cancel")
	}
	// Cancel returns the experience to a fresh "no poll" state so the
	// client dispatcher routes a subsequent location-row tap to the
	// propose modal (start over) rather than the confirm-picker.
	if exp.LocationPollCompleted != nil {
		t.Error("location_poll_completed should be cleared after cancel")
	}
	if exp.CurrentLocationPollId != nil {
		t.Error("current_location_poll_id should be cleared after cancel")
	}

	// Proposals should be preserved (not deleted) so the read-only results
	// view can still render the poll history.
	proposals, err := service.storage.QueryByField(ownerCtx, "experience_id", expID, &models.ExperienceLocationProposal{})
	if err != nil {
		t.Fatalf("Failed to query proposals: %v", err)
	}
	if len(proposals) == 0 {
		t.Error("Proposals should be preserved after cancel")
	}
}

func TestService_UnlockLocation(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	expID, _ := setupLocationPollExperience(t, service, testStorage)

	proposalResp, err := service.ProposeLocation(ownerCtx, makeLocationProposal(expID, "Park", 37.0, -122.0))
	if err != nil {
		t.Fatalf("ProposeLocation failed: %v", err)
	}
	proposalID := proposalResp.Msg.Proposal.Id

	if _, err := service.ConfirmLocation(ownerCtx, connect.NewRequest(&api.ConfirmLocationRequest{
		ExperienceId: expID,
		ProposalId:   proposalID,
	})); err != nil {
		t.Fatalf("ConfirmLocation failed: %v", err)
	}

	if _, err := service.UnlockLocation(ownerCtx, connect.NewRequest(&api.UnlockLocationRequest{
		ExperienceId: expID,
	})); err != nil {
		t.Fatalf("UnlockLocation failed: %v", err)
	}

	// Confirmed proposal should be un-confirmed after unlock.
	proposal := &models.ExperienceLocationProposal{}
	if err := testStorage.GetByID(ownerCtx, proposalID, proposal); err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}
	if proposal.IsConfirmed {
		t.Error("Proposal should be un-confirmed after UnlockLocation")
	}
}
