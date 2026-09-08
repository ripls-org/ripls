package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// seedRequestWithNeeds creates a Request owned by requesterID with the
// given need names, each with 1 slot. Returns the request ID and the
// created need IDs in input order.
func seedRequestWithNeeds(t *testing.T, service *Service, requesterID, communityID string, needNames []string, stockImageryDone chan struct{}) (string, []string) {
	t.Helper()
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	submitResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Yard cleanup",
		Description: "Need help with yard cleanup after the storm",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	services.WaitForStockImagery(t, stockImageryDone)
	shareRequestInto(t, ctx, service, submitResp.Msg.RequestId, communityID)

	if len(needNames) == 0 {
		return requestID, nil
	}

	items := make([]*api.RequestBatchNeedItem, 0, len(needNames))
	for _, name := range needNames {
		items = append(items, &api.RequestBatchNeedItem{Name: name, Slots: 1})
	}
	batchResp, err := service.BatchAddRequestNeeds(ctx, connect.NewRequest(&api.BatchAddRequestNeedsRequest{
		RequestId: requestID,
		Items:     items,
	}))
	if err != nil {
		t.Fatalf("BatchAddRequestNeeds failed: %v", err)
	}

	needIDs := make([]string, 0, len(batchResp.Msg.Needs))
	for _, n := range batchResp.Msg.Needs {
		needIDs = append(needIDs, n.Id)
	}
	return requestID, needIDs
}

func TestService_BatchClaimRequestNeeds_RequesterSelfClaimsAll(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	requestID, needIDs := seedRequestWithNeeds(t, service, requesterID, communityID,
		[]string{"Saw", "Gloves", "Lunch"}, stockImageryDone)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	claims := make([]*api.BatchClaimRequestNeedItem, 0, len(needIDs))
	for _, id := range needIDs {
		claims = append(claims, &api.BatchClaimRequestNeedItem{NeedId: id})
	}

	resp, err := service.BatchClaimRequestNeeds(ctx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims:    claims,
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}

	if got, want := len(resp.Msg.Results), len(needIDs); got != want {
		t.Fatalf("expected %d results, got %d", want, got)
	}

	for i, r := range resp.Msg.Results {
		if r.NeedId != needIDs[i] {
			t.Errorf("result %d: need_id = %q, want %q", i, r.NeedId, needIDs[i])
		}
		if r.ErrorMessage != nil {
			t.Errorf("result %d: unexpected error: %s", i, *r.ErrorMessage)
		}
		if r.Contribution == nil {
			t.Errorf("result %d: contribution missing on success", i)
			continue
		}
		if r.Contribution.Contributor == nil || r.Contribution.Contributor.Id != requesterID {
			t.Errorf("result %d: contributor.id mismatch (want %q)", i, requesterID)
		}
	}

	// Verify slots dropped to zero on each need.
	for _, id := range needIDs {
		stored := &models.PlanningNeed{}
		if err := testStorage.GetByID(context.Background(), id, stored); err != nil {
			t.Fatalf("failed to reload need %s: %v", id, err)
		}
		if stored.SlotsRemaining != 0 {
			t.Errorf("need %s: slots_remaining = %d, want 0", id, stored.SlotsRemaining)
		}
	}
}

func TestService_BatchClaimRequestNeeds_PartialSuccess(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	requestID, needIDs := seedRequestWithNeeds(t, service, requesterID, communityID,
		[]string{"Saw"}, stockImageryDone)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	resp, err := service.BatchClaimRequestNeeds(ctx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims: []*api.BatchClaimRequestNeedItem{
			{NeedId: needIDs[0]},
			{NeedId: "need_does_not_exist"},
		},
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}
	if len(resp.Msg.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Msg.Results))
	}
	if resp.Msg.Results[0].ErrorMessage != nil {
		t.Errorf("result[0]: unexpected error: %s", *resp.Msg.Results[0].ErrorMessage)
	}
	if resp.Msg.Results[0].Contribution == nil {
		t.Errorf("result[0]: missing contribution on success")
	}
	if resp.Msg.Results[1].ErrorMessage == nil {
		t.Errorf("result[1]: expected error for missing need, got nil")
	}
	if resp.Msg.Results[1].Contribution != nil {
		t.Errorf("result[1]: unexpected contribution on failure")
	}
}

func TestService_BatchClaimRequestNeeds_RejectsClaimOnDeletedNeed(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	requestID, needIDs := seedRequestWithNeeds(t, service, requesterID, communityID,
		[]string{"Saw"}, stockImageryDone)

	// Soft-delete the need directly via storage.
	stored := &models.PlanningNeed{}
	if err := testStorage.GetByID(context.Background(), needIDs[0], stored); err != nil {
		t.Fatalf("failed to load need: %v", err)
	}
	stored.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
	if err := testStorage.Update(context.Background(), stored); err != nil {
		t.Fatalf("failed to soft-delete need: %v", err)
	}

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	resp, err := service.BatchClaimRequestNeeds(ctx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims:    []*api.BatchClaimRequestNeedItem{{NeedId: needIDs[0]}},
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}
	if len(resp.Msg.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Msg.Results))
	}
	if resp.Msg.Results[0].ErrorMessage == nil {
		t.Errorf("expected error for deleted need, got nil")
	}
}

func TestService_BatchClaimRequestNeeds_EmptyClaimsReturnsEmptyResponse(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)
	requestID, _ := seedRequestWithNeeds(t, service, requesterID, communityID, nil, stockImageryDone)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	resp, err := service.BatchClaimRequestNeeds(ctx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims:    []*api.BatchClaimRequestNeedItem{},
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}
	if len(resp.Msg.Results) != 0 {
		t.Errorf("expected 0 results for empty claims, got %d", len(resp.Msg.Results))
	}
}

func TestService_BatchClaimRequestNeeds_RejectsForUnknownRequest(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	_, err := service.BatchClaimRequestNeeds(ctx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: "no_such_request",
		Claims:    []*api.BatchClaimRequestNeedItem{{NeedId: "n1"}},
	}))
	if err == nil {
		t.Fatal("expected error for unknown request, got nil")
	}
}
