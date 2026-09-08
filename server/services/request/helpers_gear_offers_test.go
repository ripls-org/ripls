package request

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertGearOffer inserts the storage rows a gear-backed offer (#2702) leaves
// behind — gear, an origin-linked transfer, and a linked contribution — and
// returns the transfer id.
func insertGearOffer(t *testing.T, st *storage.ProtoSQLStorage, requestID, communityID, helperID, askerID, gearName string, seq int64) string {
	t.Helper()
	ctx := context.Background()

	gearID, err := st.Insert(ctx, &models.Gear{
		Name:     gearName,
		OwnerId:  helperID,
		State:    models.GearState_GEAR_STATE_AVAILABLE,
		MediaIds: []string{"media-" + gearName},
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	transferID, err := st.Insert(ctx, &models.Transfer{
		GearId:               gearID,
		OwnerId:              helperID,
		RecipientId:          askerID,
		TransferType:         models.TransferType_TRANSFER_TYPE_LOAN,
		State:                models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:          communityID,
		Origin:               &models.Transfer_OriginRequestId{OriginRequestId: requestID},
		LatestRequestUnixSec: seq,
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	contribution := &models.PlanningContribution{
		ContributorId: helperID,
		Title:         gearName,
		Scope:         &models.PlanningContribution_RequestId{RequestId: requestID},
		GearId:        &gearID,
		TransferId:    &transferID,
	}
	if _, err := st.Insert(ctx, contribution); err != nil {
		t.Fatalf("insert contribution: %v", err)
	}
	return transferID
}

// setupRequestWithCommunity inserts a request by askerID shared into a new
// community whose members are askerID plus the given helpers.
func setupRequestWithCommunity(t *testing.T, st *storage.ProtoSQLStorage, askerID string, helperIDs ...string) (requestID, communityID string) {
	t.Helper()
	ctx := context.Background()
	communityID = setupCommunityWithMembers(t, st, askerID, helperIDs...)
	requestID, err := st.Insert(ctx, &models.Request{
		RequesterId: askerID,
		Title:       "Looking for a lawn mower",
		State:       models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
	})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := st.Insert(ctx, &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
		Archived:    false,
	}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}
	return requestID, communityID
}

func TestGetRequest_CarriesGearOffers(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	askerID := setupTestUser(t, testStorage, "June", "june-offers@example.com")
	helperID := setupTestUser(t, testStorage, "Theo", "theo-offers@example.com")
	requestID, communityID := setupRequestWithCommunity(t, testStorage, askerID, helperID)
	transferID := insertGearOffer(t, testStorage, requestID, communityID, helperID, askerID, "Lawn mower", 100)

	ctx := createAuthenticatedContext(askerID, "june-offers@example.com", models.Role_ROLE_USER)
	resp, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}

	offers := resp.Msg.Request.GearOffers
	if len(offers) != 1 {
		t.Fatalf("expected 1 gear offer, got %d", len(offers))
	}
	offer := offers[0]
	if offer.TransferId != transferID {
		t.Errorf("expected transfer id %s, got %s", transferID, offer.TransferId)
	}
	if offer.TransferType != api.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("expected LOAN, got %s", offer.TransferType)
	}
	if offer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("expected RECIPIENT_SELECTED, got %s", offer.State)
	}
	if offer.GearName != "Lawn mower" {
		t.Errorf("expected gear name 'Lawn mower', got %q", offer.GearName)
	}
	if offer.GearMediaId != "media-Lawn mower" {
		t.Errorf("expected gear media id, got %q", offer.GearMediaId)
	}
	if offer.Helper == nil || offer.Helper.Id != helperID {
		t.Errorf("expected helper %s, got %+v", helperID, offer.Helper)
	}
	if offer.GetContributionId() == "" {
		t.Error("expected contribution_id on the offer")
	}
}

func TestGetRequest_NoGearOffers_EmptyList(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	askerID := setupTestUser(t, testStorage, "June", "june-nooffers@example.com")
	requestID, _ := setupRequestWithCommunity(t, testStorage, askerID)

	ctx := createAuthenticatedContext(askerID, "june-nooffers@example.com", models.Role_ROLE_USER)
	resp, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if len(resp.Msg.Request.GearOffers) != 0 {
		t.Errorf("expected no gear offers, got %d", len(resp.Msg.Request.GearOffers))
	}
}

func TestGetRequest_GearOffersAreBatched_NoN1(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	askerID := setupTestUser(t, testStorage, "June", "june-batch@example.com")
	helperIDs := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		helperIDs = append(helperIDs, setupTestUser(t, testStorage, fmt.Sprintf("Helper%d", i), fmt.Sprintf("helper%d-batch@example.com", i)))
	}
	requestID, communityID := setupRequestWithCommunity(t, testStorage, askerID, helperIDs...)
	for i, helperID := range helperIDs {
		insertGearOffer(t, testStorage, requestID, communityID, helperID, askerID, fmt.Sprintf("Mower %d", i), int64(100+i))
	}

	ctx := createAuthenticatedContext(askerID, "june-batch@example.com", models.Role_ROLE_USER)
	statsCtx := storage.WithQueryStats(ctx)
	var (
		resp *connect.Response[api.GetRequestResponse]
		err  error
	)
	// GetRequest issues a bounded number of queries regardless of how many
	// gear offers exist — the offer enrichment adds exactly four (transfers,
	// gear, users, contributions) on top of the base read.
	storage.AssertMaxQueries(t, statsCtx, 20, func() {
		resp, err = service.GetRequest(statsCtx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	})
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	offers := resp.Msg.Request.GearOffers
	if len(offers) != len(helperIDs) {
		t.Fatalf("expected %d gear offers, got %d", len(helperIDs), len(offers))
	}
	// Newest first by latest_request_unix_sec.
	if offers[0].GearName != "Mower 7" {
		t.Errorf("expected newest offer first, got %q", offers[0].GearName)
	}
}
