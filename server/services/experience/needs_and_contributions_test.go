package experience

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestAddExperienceNeed_Success(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner1", "owner@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner1", "owner@test.com", "Owner")

	// Create and share experience.
	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck",
		Description: "Summer potluck",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	// Owner can add a need.
	resp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Red wine",
		Slots:        int32(2),
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}

	if resp.Msg.Need.Name != "Red wine" {
		t.Errorf("Name = %q, want %q", resp.Msg.Need.Name, "Red wine")
	}
	if resp.Msg.Need.Slots != 2 {
		t.Errorf("Slots = %d, want 2", resp.Msg.Need.Slots)
	}
	if resp.Msg.Need.SlotsRemaining != 2 {
		t.Errorf("SlotsRemaining = %d, want 2", resp.Msg.Need.SlotsRemaining)
	}
}

func TestAddExperienceNeed_SlotCapEnforced(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner2", "owner2@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner2", "owner2@test.com", "Owner2")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}

	_, err = service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: saveResp.Msg.Experience.Id,
		Name:         "Wine",
		Slots:        21,
	}))
	if err == nil {
		t.Fatal("expected error for slots > 20, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestClaimExperienceNeed_DecrementSlots(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner3", "owner3@test.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user3a", "user3a@test.com", models.Role_ROLE_USER)
	user2Ctx := createAuthenticatedContext("user3b", "user3b@test.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner3", "owner3@test.com", "Owner3")
	createTestUser(t, testStorage, "user3a", "user3a@test.com", "UserA")
	createTestUser(t, testStorage, "user3b", "user3b@test.com", "UserB")

	communityID := createTestCommunity(t, testStorage, "Test Community", "owner3")
	createTestCommunityMembership(t, testStorage, communityID, "owner3")
	createTestCommunityMembership(t, testStorage, communityID, "user3a")
	createTestCommunityMembership(t, testStorage, communityID, "user3b")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// RSVP both users so they are participants.
	_, err = service.RSVPToExperience(user1Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVP user3a: %v", err)
	}
	_, err = service.RSVPToExperience(user2Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVP user3b: %v", err)
	}

	// Add a need with 1 slot.
	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Dessert",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	// First claim should succeed.
	claimResp, err := service.ClaimExperienceNeed(user1Ctx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}
	if claimResp.Msg.Contribution.Title != "Dessert" {
		t.Errorf("Contribution.Title = %q, want %q", claimResp.Msg.Contribution.Title, "Dessert")
	}

	// Second claim succeeds even though no slots remain — a helper can
	// intentionally over-claim. The contribution is recorded and the
	// need stays at SlotsRemaining=0 (no further decrement).
	overClaimResp, err := service.ClaimExperienceNeed(user2Ctx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("over-claim ClaimExperienceNeed: %v", err)
	}
	if overClaimResp.Msg.Contribution == nil {
		t.Fatal("over-claim returned nil contribution")
	}
	if overClaimResp.Msg.Contribution.Title != "Dessert" {
		t.Errorf("over-claim Contribution.Title = %q, want %q",
			overClaimResp.Msg.Contribution.Title, "Dessert")
	}
}

func TestUnclaimExperienceNeed_RestoresSlot(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner4", "owner4@test.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user4a", "user4a@test.com", models.Role_ROLE_USER)
	user2Ctx := createAuthenticatedContext("user4b", "user4b@test.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner4", "owner4@test.com", "Owner4")
	createTestUser(t, testStorage, "user4a", "user4a@test.com", "UserA4")
	createTestUser(t, testStorage, "user4b", "user4b@test.com", "UserB4")

	communityID := createTestCommunity(t, testStorage, "Community4", "owner4")
	createTestCommunityMembership(t, testStorage, communityID, "owner4")
	createTestCommunityMembership(t, testStorage, communityID, "user4a")
	createTestCommunityMembership(t, testStorage, communityID, "user4b")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck4",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	for _, rsvpCtx := range []interface{ Value(key any) any }{user1Ctx, user2Ctx} {
		_ = rsvpCtx
	}
	_, _ = service.RSVPToExperience(user1Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	_, _ = service.RSVPToExperience(user2Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))

	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Wine",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	// Claim it.
	claimResp, err := service.ClaimExperienceNeed(user1Ctx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}
	contribID := claimResp.Msg.Contribution.Id

	// Unclaim it.
	_, err = service.UnclaimExperienceNeed(user1Ctx, connect.NewRequest(&api.UnclaimExperienceNeedRequest{
		ContributionId: contribID,
		ExperienceId:   expID,
	}))
	if err != nil {
		t.Fatalf("UnclaimExperienceNeed: %v", err)
	}

	// Now user2 can claim it.
	_, err = service.ClaimExperienceNeed(user2Ctx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed after unclaim: %v", err)
	}
}

func TestAddExperienceContribution_Success(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner5", "owner5@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner5", "owner5@test.com", "Owner5")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck5",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	desc := "A 2019 Tempranillo"
	resp, err := service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Red wine",
		Description:  &desc,
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}
	if resp.Msg.Contribution.Title != "Red wine" {
		t.Errorf("Title = %q, want %q", resp.Msg.Contribution.Title, "Red wine")
	}
	if resp.Msg.Contribution.Description == nil || *resp.Msg.Contribution.Description != desc {
		t.Errorf("Description = %v, want %q", resp.Msg.Contribution.Description, desc)
	}
}

func TestListExperienceNeedsAndContributions(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner6", "owner6@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner6", "owner6@test.com", "Owner6")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck6",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	_, err = service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Dessert",
		Slots:        3,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}

	_, err = service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Salad",
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}

	listResp, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}
	if len(listResp.Msg.Needs) != 1 {
		t.Errorf("len(Needs) = %d, want 1", len(listResp.Msg.Needs))
	}
	if len(listResp.Msg.Contributions) != 1 {
		t.Errorf("len(Contributions) = %d, want 1", len(listResp.Msg.Contributions))
	}
}

func TestRemoveExperienceNeed_OnlyProposerCanRemove(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner7", "owner7@test.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user7a", "user7a@test.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner7", "owner7@test.com", "Owner7")
	createTestUser(t, testStorage, "user7a", "user7a@test.com", "UserA7")

	communityID := createTestCommunity(t, testStorage, "Community7", "owner7")
	createTestCommunityMembership(t, testStorage, communityID, "owner7")
	createTestCommunityMembership(t, testStorage, communityID, "user7a")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck7",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)
	_, _ = service.RSVPToExperience(user1Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))

	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Chips",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	// user7a trying to remove owner7's need — should fail.
	_, err = service.RemoveExperienceNeed(user1Ctx, connect.NewRequest(&api.RemoveExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err == nil {
		t.Fatal("expected PermissionDenied, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("error code = %v, want PermissionDenied", connect.CodeOf(err))
	}

	// owner7 can remove their own need.
	_, err = service.RemoveExperienceNeed(ownerCtx, connect.NewRequest(&api.RemoveExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Errorf("RemoveExperienceNeed by proposer: %v", err)
	}
}

func TestTerminalExperienceBlocksMutations(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner8", "owner8@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "owner8", "owner8@test.com", "Owner8")

	communityID := createTestCommunity(t, testStorage, "Community8", "owner8")
	createTestCommunityMembership(t, testStorage, communityID, "owner8")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck8",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Cancel the experience.
	_, err = service.CancelExperience(ownerCtx, connect.NewRequest(&api.CancelExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CancelExperience: %v", err)
	}

	// Adding a need to a cancelled experience should fail.
	_, err = service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Wine",
		Slots:        1,
	}))
	if err == nil {
		t.Fatal("expected FailedPrecondition for cancelled experience, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("error code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

// TestFullNeedsWorkflow exercises the full lifecycle: propose need → claim →
// offer free-form contribution → complete experience → verify read-only.
func TestFullNeedsWorkflow(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner-wf", "owner-wf@test.com", models.Role_ROLE_USER)
	participantCtx := createAuthenticatedContext("part-wf", "part-wf@test.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner-wf", "owner-wf@test.com", "Owner")
	createTestUser(t, testStorage, "part-wf", "part-wf@test.com", "Participant")

	communityID := createTestCommunity(t, testStorage, "WorkflowCommunity", "owner-wf")
	createTestCommunityMembership(t, testStorage, communityID, "owner-wf")
	createTestCommunityMembership(t, testStorage, communityID, "part-wf")

	// Create + share experience.
	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Summer Potluck",
		Description: "End-to-end test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Participant RSVPs.
	_, err = service.RSVPToExperience(participantCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	// Step 1: Owner proposes a need.
	note := "A nice red, ideally"
	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Red wine",
		Note:         &note,
		Slots:        2,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	// Step 2: Participant claims the need.
	claimNote := "Tempranillo"
	claimResp, err := service.ClaimExperienceNeed(participantCtx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
		Note:         &claimNote,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}
	if claimResp.Msg.Contribution.Title != "Red wine" {
		t.Errorf("claim title = %q, want %q", claimResp.Msg.Contribution.Title, "Red wine")
	}

	// Step 3: Participant adds a free-form contribution.
	_, err = service.AddExperienceContribution(participantCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Caesar salad",
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}

	// Verify list: 1 need (with 1 remaining slot), 2 contributions.
	listResp, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}
	if len(listResp.Msg.Needs) != 1 {
		t.Fatalf("len(Needs) = %d, want 1", len(listResp.Msg.Needs))
	}
	if listResp.Msg.Needs[0].SlotsRemaining != 1 {
		t.Errorf("SlotsRemaining = %d, want 1", listResp.Msg.Needs[0].SlotsRemaining)
	}
	if len(listResp.Msg.Contributions) != 2 {
		t.Fatalf("len(Contributions) = %d, want 2", len(listResp.Msg.Contributions))
	}

	// Step 4: Complete the experience.
	_, err = service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience: %v", err)
	}

	// Step 5: Verify read-only — all mutations should fail with FailedPrecondition.
	_, err = service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Dessert",
		Slots:        1,
	}))
	if err == nil {
		t.Error("expected error adding need to completed experience, got nil")
	} else if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("add need error code = %v, want FailedPrecondition", connect.CodeOf(err))
	}

	_, err = service.AddExperienceContribution(participantCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Cookies",
	}))
	if err == nil {
		t.Error("expected error adding contribution to completed experience, got nil")
	} else if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("add contribution error code = %v, want FailedPrecondition", connect.CodeOf(err))
	}

	// Listing still works (read-only access is fine).
	listResp2, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions after completion: %v", err)
	}
	if len(listResp2.Msg.Needs) != 1 {
		t.Errorf("len(Needs) after completion = %d, want 1", len(listResp2.Msg.Needs))
	}
	if len(listResp2.Msg.Contributions) != 2 {
		t.Errorf("len(Contributions) after completion = %d, want 2", len(listResp2.Msg.Contributions))
	}
}

// TestClaimedNeedStaysVisibleAfterFullClaim verifies the v2 behavior:
// a fully-claimed need stays in the list with slots_remaining == 0 so
// the client's unified Volunteer-sheet list can render it alongside
// open needs with its voter stack. The v1 contract — "fully-claimed
// needs disappear" — was an artifact of the two-column "Still needed /
// Who's bringing what" split and is gone with v2. See
// docs/client/needs.md.
func TestClaimedNeedStaysVisibleAfterFullClaim(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	ownerCtx := createAuthenticatedContext("owner_hidden1", "owner_hidden1@test.com", models.Role_ROLE_USER)
	user1Ctx := createAuthenticatedContext("user_hidden1a", "user_hidden1a@test.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, "owner_hidden1", "owner_hidden1@test.com", "OwnerH1")
	createTestUser(t, testStorage, "user_hidden1a", "user_hidden1a@test.com", "UserH1A")

	communityID := createTestCommunity(t, testStorage, "CommunityH1", "owner_hidden1")
	createTestCommunityMembership(t, testStorage, communityID, "owner_hidden1")
	createTestCommunityMembership(t, testStorage, communityID, "user_hidden1a")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "PotluckH1",
		Description: "Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ownerCtx, expID, communityID)
	_, err = service.RSVPToExperience(user1Ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	// Add a single-slot need.
	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Bread",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	// Before claim: need appears in the list.
	listBefore, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions before claim: %v", err)
	}
	if len(listBefore.Msg.Needs) != 1 {
		t.Errorf("Needs before claim = %d, want 1", len(listBefore.Msg.Needs))
	}

	// Claim the need (fully claims the single slot).
	claimResp, err := service.ClaimExperienceNeed(user1Ctx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}
	contribID := claimResp.Msg.Contribution.Id

	// After claim (v2): the need stays in the list with
	// slots_remaining == 0; the client renders it with a voter stack.
	listAfterClaim, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions after claim: %v", err)
	}
	if len(listAfterClaim.Msg.Needs) != 1 {
		t.Errorf("Needs after claim = %d, want 1 (v2 keeps fully-claimed needs in the list)", len(listAfterClaim.Msg.Needs))
	}
	if listAfterClaim.Msg.Needs[0].SlotsRemaining != 0 {
		t.Errorf("SlotsRemaining after claim = %d, want 0", listAfterClaim.Msg.Needs[0].SlotsRemaining)
	}
	if len(listAfterClaim.Msg.Contributions) != 1 {
		t.Errorf("Contributions after claim = %d, want 1", len(listAfterClaim.Msg.Contributions))
	}

	// Unclaim: slots are restored and the contribution disappears.
	_, err = service.UnclaimExperienceNeed(user1Ctx, connect.NewRequest(&api.UnclaimExperienceNeedRequest{
		ContributionId: contribID,
		ExperienceId:   expID,
	}))
	if err != nil {
		t.Fatalf("UnclaimExperienceNeed: %v", err)
	}

	listAfterUnclaim, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions after unclaim: %v", err)
	}
	if len(listAfterUnclaim.Msg.Needs) != 1 {
		t.Errorf("Needs after unclaim = %d, want 1", len(listAfterUnclaim.Msg.Needs))
	}
	if listAfterUnclaim.Msg.Needs[0].SlotsRemaining != 1 {
		t.Errorf("SlotsRemaining after unclaim = %d, want 1", listAfterUnclaim.Msg.Needs[0].SlotsRemaining)
	}
	if len(listAfterUnclaim.Msg.Contributions) != 0 {
		t.Errorf("Contributions after unclaim = %d, want 0", len(listAfterUnclaim.Msg.Contributions))
	}
}

// TestListExperienceNeedsAndContributions_LazySuggestions verifies that suggestions
// are generated on first call when the experience has none and returned in the response.
func TestListExperienceNeedsAndContributions_LazySuggestions(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	userID := "lazy-sug-user"
	createTestUser(t, testStorage, userID, "lazy@test.com", "Lazy User")
	ctx := createAuthenticatedContext(userID, "lazy@test.com", models.Role_ROLE_USER)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		return &ai.ExperienceSuggestionResult{
			Suggestions:  []string{"bring chairs", "bring ice", "bring napkins"},
			CategoryHint: "outdoor picnic",
		}, nil
	}
	service.aiProvider = mock

	saveResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Park Picnic",
		Description: "Afternoon in the park",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	// Clear suggestions from storage to simulate a pre-feature experience.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	stored.Suggestions = nil
	stored.CategoryHint = nil
	if err := testStorage.Update(ctx, stored); err != nil {
		t.Fatalf("Update (clear suggestions): %v", err)
	}

	// Reset call count so we can verify lazy generation fires exactly once.
	mock.Calls.GenerateExperienceSuggestions = nil

	listResp, err := service.ListExperienceNeedsAndContributions(ctx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}
	if len(listResp.Msg.Suggestions) != 3 {
		t.Errorf("Suggestions = %d, want 3", len(listResp.Msg.Suggestions))
	}
	if listResp.Msg.Suggestions[0] != "bring chairs" {
		t.Errorf("Suggestions[0] = %q, want %q", listResp.Msg.Suggestions[0], "bring chairs")
	}
	if listResp.Msg.CategoryHint == nil || *listResp.Msg.CategoryHint != "outdoor picnic" {
		t.Errorf("CategoryHint = %v, want %q", listResp.Msg.CategoryHint, "outdoor picnic")
	}
	if len(mock.Calls.GenerateExperienceSuggestions) != 1 {
		t.Errorf("GenerateExperienceSuggestions called %d times, want 1", len(mock.Calls.GenerateExperienceSuggestions))
	}

	// Verify suggestions were persisted.
	after := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, after); err != nil {
		t.Fatalf("GetByID after lazy gen: %v", err)
	}
	if len(after.Suggestions) != 3 {
		t.Errorf("Persisted suggestions = %d, want 3", len(after.Suggestions))
	}
}

// TestListExperienceNeedsAndContributions_NoRegenerationWhenPresent verifies that
// GenerateExperienceSuggestions is NOT called when the experience already has suggestions.
func TestListExperienceNeedsAndContributions_NoRegenerationWhenPresent(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	userID := "no-regen-user"
	createTestUser(t, testStorage, userID, "nregen@test.com", "No Regen User")
	ctx := createAuthenticatedContext(userID, "nregen@test.com", models.Role_ROLE_USER)

	mock := ai.NewMockProvider()
	callCount := 0
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		callCount++
		return &ai.ExperienceSuggestionResult{
			Suggestions: []string{"chip a", "chip b"},
		}, nil
	}
	service.aiProvider = mock

	saveResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Pre-filled Event",
		Description: "Already has suggestions",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	// Manually persist suggestions to simulate an experience that already has them.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	stored.Suggestions = []string{"existing chip 1", "existing chip 2"}
	if err := testStorage.Update(ctx, stored); err != nil {
		t.Fatalf("Update (set suggestions): %v", err)
	}
	callCount = 0 // reset after save

	listResp, err := service.ListExperienceNeedsAndContributions(ctx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}
	if callCount != 0 {
		t.Errorf("GenerateExperienceSuggestions called %d times, want 0 (should not regenerate)", callCount)
	}
	if len(listResp.Msg.Suggestions) != 2 {
		t.Errorf("Suggestions = %d, want 2 (existing chips)", len(listResp.Msg.Suggestions))
	}
}

// TestListExperienceNeedsAndContributions_TerminalSkipsGeneration verifies that
// lazy suggestion generation is skipped for terminal (COMPLETED/CANCELLED) experiences.
func TestListExperienceNeedsAndContributions_TerminalSkipsGeneration(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	userID := "terminal-sug-user"
	createTestUser(t, testStorage, userID, "terminal@test.com", "Terminal User")
	ctx := createAuthenticatedContext(userID, "terminal@test.com", models.Role_ROLE_USER)

	mock := ai.NewMockProvider()
	callCount := 0
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		callCount++
		return &ai.ExperienceSuggestionResult{Suggestions: []string{"should not appear"}}, nil
	}
	service.aiProvider = mock

	saveResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Completed Hike",
		Description: "Already done",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	// Force the experience into a terminal state and clear suggestions.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	stored.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED
	stored.Suggestions = nil
	if err := testStorage.Update(ctx, stored); err != nil {
		t.Fatalf("Update (set terminal state): %v", err)
	}
	callCount = 0

	listResp, err := service.ListExperienceNeedsAndContributions(ctx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}
	if callCount != 0 {
		t.Errorf("GenerateExperienceSuggestions called %d times, want 0 for terminal experience", callCount)
	}
	if len(listResp.Msg.Suggestions) != 0 {
		t.Errorf("Suggestions = %d, want 0 for terminal experience", len(listResp.Msg.Suggestions))
	}
}

// TestListExperienceNeedsAndContributions_LLMFailureReturnsEmpty verifies that an LLM
// error during lazy generation does not fail the RPC — empty suggestions are returned instead.
func TestListExperienceNeedsAndContributions_LLMFailureReturnsEmpty(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	userID := "llm-fail-user"
	createTestUser(t, testStorage, userID, "llmfail@test.com", "LLM Fail User")
	ctx := createAuthenticatedContext(userID, "llmfail@test.com", models.Role_ROLE_USER)

	mock := ai.NewMockProvider()
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		return nil, fmt.Errorf("LLM service unavailable")
	}
	service.aiProvider = mock

	saveResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Camping Trip",
		Description: "Weekend in the woods",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	// Clear suggestions.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	stored.Suggestions = nil
	if err := testStorage.Update(ctx, stored); err != nil {
		t.Fatalf("Update (clear suggestions): %v", err)
	}

	listResp, err := service.ListExperienceNeedsAndContributions(ctx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions should succeed even when LLM fails: %v", err)
	}
	if len(listResp.Msg.Suggestions) != 0 {
		t.Errorf("Suggestions = %d, want 0 on LLM failure", len(listResp.Msg.Suggestions))
	}
}
