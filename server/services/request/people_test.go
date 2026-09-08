package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestGetRequestPeople_NoOfferers verifies that a request with no offers returns only
// the requester with total_count=1.
func TestGetRequestPeople_NoOfferers(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Alice", "alice@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "alice@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	resp, err := service.GetRequestPeople(ctx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestPeople failed: %v", err)
	}

	if resp.Msg.Requester == nil {
		t.Fatal("expected requester to be populated")
	}
	if resp.Msg.Requester.Id != requesterID {
		t.Errorf("expected requester ID %s, got %s", requesterID, resp.Msg.Requester.Id)
	}
	if resp.Msg.Requester.Name != "Alice" {
		t.Errorf("expected requester name 'Alice', got %s", resp.Msg.Requester.Name)
	}
	if len(resp.Msg.Offerers) != 0 {
		t.Errorf("expected 0 offerers, got %d", len(resp.Msg.Offerers))
	}
	if resp.Msg.TotalCount != 1 {
		t.Errorf("expected total_count=1, got %d", resp.Msg.TotalCount)
	}
}

// TestGetRequestPeople_WithOfferers verifies that offerers are assembled and
// total_count equals requester count + offerer count.
func TestGetRequestPeople_WithOfferers(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offerer1ID := setupTestUser(t, testStorage, "Bob", "bob@example.com")
	offerer2ID := setupTestUser(t, testStorage, "Carol", "carol@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offerer1ID, offerer2ID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a drill",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	offerer1Ctx := createAuthenticatedContext(offerer1ID, "bob@example.com", models.Role_ROLE_USER)
	_, err = service.OfferToFulfill(offerer1Ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill (bob) failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	offerer2Ctx := createAuthenticatedContext(offerer2ID, "carol@example.com", models.Role_ROLE_USER)
	_, err = service.OfferToFulfill(offerer2Ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill (carol) failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	resp, err := service.GetRequestPeople(requesterCtx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestPeople failed: %v", err)
	}

	if resp.Msg.Requester == nil || resp.Msg.Requester.Id != requesterID {
		t.Errorf("expected requester ID %s, got %v", requesterID, resp.Msg.Requester)
	}
	if len(resp.Msg.Offerers) != 2 {
		t.Errorf("expected 2 offerers, got %d", len(resp.Msg.Offerers))
	}
	// TotalCount = 1 (requester) + 2 (offerers)
	if resp.Msg.TotalCount != 3 {
		t.Errorf("expected total_count=3, got %d", resp.Msg.TotalCount)
	}

	// Verify both offerer IDs appear.
	offererIDs := make(map[string]bool)
	for _, u := range resp.Msg.Offerers {
		offererIDs[u.Id] = true
	}
	if !offererIDs[offerer1ID] {
		t.Errorf("expected offerer1 (%s) in offerers list", offerer1ID)
	}
	if !offererIDs[offerer2ID] {
		t.Errorf("expected offerer2 (%s) in offerers list", offerer2ID)
	}
}

// TestGetRequestPeople_WithdrawnOfferExcluded verifies that withdrawn offers are not
// counted in the offerers list.
func TestGetRequestPeople_WithdrawnOfferExcluded(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a hammer",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	_, err = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	// Offerer withdraws.
	_, err = service.WithdrawOffer(offererCtx, connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("WithdrawOffer failed: %v", err)
	}

	resp, err := service.GetRequestPeople(requesterCtx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestPeople failed: %v", err)
	}

	if len(resp.Msg.Offerers) != 0 {
		t.Errorf("expected 0 offerers after withdrawal, got %d", len(resp.Msg.Offerers))
	}
	if resp.Msg.TotalCount != 1 {
		t.Errorf("expected total_count=1 after withdrawal, got %d", resp.Msg.TotalCount)
	}
}

// TestGetRequestPeople_WithCommunityFilter verifies that supplying community_id filters
// offers to those associated with that community.
func TestGetRequestPeople_WithCommunityFilter(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	_, err = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	// With matching community filter.
	resp, err := service.GetRequestPeople(requesterCtx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetRequestPeople with community filter failed: %v", err)
	}
	if len(resp.Msg.Offerers) != 1 {
		t.Errorf("expected 1 offerer with matching community filter, got %d", len(resp.Msg.Offerers))
	}

	// With a non-existent community — Phase 4 of #1621 surfaces this as
	// NotFound at the entry-point gate rather than silently returning 0
	// offers, so client bugs (stale or fabricated community IDs) are loud.
	_, err = service.GetRequestPeople(requesterCtx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId:   requestID,
		CommunityId: "non-existent-community",
	}))
	if err == nil {
		t.Fatalf("expected NotFound for non-existent community, got nil")
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("expected NotFound, got %s", got)
	}
}

// TestGetRequestPeople_NotFound verifies that a non-existent request returns NotFound.
func TestGetRequestPeople_NotFound(t *testing.T) {
	service, _, _, _, _ := setupTestServiceWithNotifications(t)

	ctx := createAuthenticatedContext("user-id", "user@example.com", models.Role_ROLE_USER)

	_, err := service.GetRequestPeople(ctx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId: "non-existent-request",
	}))
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

// TestGetRequestPeople_RequiresAuth verifies that an unauthenticated call returns
// an error.
func TestGetRequestPeople_RequiresAuth(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	// Unauthenticated context.
	unauthCtx := context.Background()
	_, err = service.GetRequestPeople(unauthCtx, connect.NewRequest(&api.GetRequestPeopleRequest{
		RequestId: requestID,
	}))
	if err == nil {
		t.Fatal("expected auth error for unauthenticated call, got nil")
	}
}
