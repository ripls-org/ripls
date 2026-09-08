package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// createTestGearShareLink inserts a gear-flavored ShareLink (typed
// target = gear_id) and returns it.
func createTestGearShareLink(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID, gearID string) *models.ShareLink {
	t.Helper()
	link := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		CreatedAtUnixSec: time.Now().Unix(),
		Target:           &models.ShareLink_GearId{GearId: gearID},
	}
	if _, err := sqlStorage.Insert(ctx, link); err != nil {
		t.Fatalf("Failed to insert gear share link: %v", err)
	}
	return link
}

// createTestTransferShareLink inserts a transfer-flavored ShareLink
// (typed target = transfer_id) and returns it.
func createTestTransferShareLink(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID, transferID string) *models.ShareLink {
	t.Helper()
	link := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		CreatedAtUnixSec: time.Now().Unix(),
		Target:           &models.ShareLink_TransferId{TransferId: transferID},
	}
	if _, err := sqlStorage.Insert(ctx, link); err != nil {
		t.Fatalf("Failed to insert transfer share link: %v", err)
	}
	return link
}

// createTestTransfer inserts a transfer for (gear, owner) and returns it.
func createTestTransfer(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, gearID, ownerID string) *models.Transfer {
	t.Helper()
	transfer := &models.Transfer{
		Id:                   uuid.New().String(),
		GearId:               gearID,
		OwnerId:              ownerID,
		State:                models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		LatestRequestUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(ctx, transfer); err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}
	return transfer
}

// gearPageTestSetup is the base fixture for gear-landing tests.
type gearPageTestSetup struct {
	svc        *Service
	sqlStorage *storage.ProtoSQLStorage
	owner      *models.User
	community  *models.Community
	gear       *models.Gear
	shareLink  *models.ShareLink
}

// setupGearLoanTest builds a loan fixture: an owner (Dana), a community,
// an available gear shared FOR_LOAN, and a gear-flavored ShareLink.
func setupGearLoanTest(t *testing.T) gearPageTestSetup {
	t.Helper()
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed-gear-image.jpg"

	owner := createTestUser(t, ctx, sqlStorage, "Dana Lopez")
	community := createTestCommunity(t, ctx, sqlStorage, owner.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, owner.Id)
	media := createTestMedia(t, ctx, sqlStorage, owner.Id)
	gear := createTestGear(t, ctx, sqlStorage, owner.Id, "Cordless Drill", []string{media.Id}, models.GearState_GEAR_STATE_AVAILABLE)
	createTestCommunityGear(t, ctx, sqlStorage, community.Id, gear.Id, models.Availability_AVAILABILITY_FOR_LOAN)
	shareLink := createTestGearShareLink(t, ctx, sqlStorage, community.Id, owner.Id, gear.Id)

	return gearPageTestSetup{svc, sqlStorage, owner, community, gear, shareLink}
}

func TestHandleGearLanding_Loan(t *testing.T) {
	s := setupGearLoanTest(t)

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=30" {
		t.Errorf("Expected Cache-Control 'public, max-age=30', got %q", got)
	}

	body := w.Body.String()
	assertContains(t, body, "Cordless Drill")
	assertContains(t, body, "Dana is lending") // first name only — no "Lopez"
	assertNotContains(t, body, "Lopez")
	assertContains(t, body, "Ask to borrow")
	assertContains(t, body, "/item/"+s.gear.Id+"?intent=interest")
	assertContains(t, body, "code="+s.shareLink.ShortCode)
	// Hero image (gear has media) threaded into the CTA via &img=.
	assertContains(t, body, "https://storage.example.com/signed-gear-image.jpg")
}

func TestHandleGearLanding_Giveaway(t *testing.T) {
	s := setupGearLoanTest(t)
	ctx := context.Background()

	// Re-point the share link's community-gear row to a giveaway.
	owner := createTestUser(t, ctx, s.sqlStorage, "Sam Park")
	gear := createTestGear(t, ctx, s.sqlStorage, owner.Id, "Free Couch", nil, models.GearState_GEAR_STATE_AVAILABLE)
	createTestCommunityGear(t, ctx, s.sqlStorage, s.community.Id, gear.Id, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	link := createTestGearShareLink(t, ctx, s.sqlStorage, s.community.Id, owner.Id, gear.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+link.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "Free Couch")
	assertContains(t, body, "Sam is giving this away")
	assertContains(t, body, "I want this")
	assertContains(t, body, "/item/"+gear.Id+"?intent=interest")
}

// TestHandleGearLanding_TransferResolvesToGear proves a transfer-flavored
// share link renders the gear landing for the gear behind the transfer.
func TestHandleGearLanding_TransferResolvesToGear(t *testing.T) {
	s := setupGearLoanTest(t)
	ctx := context.Background()

	transfer := createTestTransfer(t, ctx, s.sqlStorage, s.gear.Id, s.owner.Id)
	link := createTestTransferShareLink(t, ctx, s.sqlStorage, s.community.Id, s.owner.Id, transfer.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+link.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}
	body := w.Body.String()
	assertContains(t, body, "Cordless Drill")
	assertContains(t, body, "/item/"+s.gear.Id+"?intent=interest")
}

func TestHandleGearLanding_GivenAwayUnavailable(t *testing.T) {
	s := setupGearLoanTest(t)
	ctx := context.Background()

	gear := createTestGear(t, ctx, s.sqlStorage, s.owner.Id, "Already Gone", nil, models.GearState_GEAR_STATE_GIVEN_AWAY)
	link := createTestGearShareLink(t, ctx, s.sqlStorage, s.community.Id, s.owner.Id, gear.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+link.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "no longer available")
	assertNotContains(t, body, "Already Gone")
	assertNotContains(t, body, "Ask to borrow")
}

func TestHandleGearLanding_AtCapacity(t *testing.T) {
	s := setupGearLoanTest(t)
	ctx := context.Background()

	// Fill the community to the cap (owner already counts as one).
	for i := range communitylib.MaxCommunityMembers - 1 {
		filler := createTestUser(t, ctx, s.sqlStorage, "Filler"+string(rune('A'+i)))
		createTestMembership(t, ctx, s.sqlStorage, s.community.Id, filler.Id)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "This community is full")
	assertNotContains(t, body, "Ask to borrow")
}

func TestHandleGearLanding_DeletedCommunity(t *testing.T) {
	s := setupGearLoanTest(t)
	ctx := context.Background()

	s.community.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	if err := s.sqlStorage.Update(ctx, s.community); err != nil {
		t.Fatalf("Failed to soft-delete community: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	assertContains(t, w.Body.String(), "no longer available")
}
