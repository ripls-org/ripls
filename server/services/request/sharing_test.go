package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestShareRequestToCommunity_Success verifies that a request can be shared with
// a new community, a CommunityRequest junction row is created, and a
// REQUEST_CREATED event is recorded.
//
// Sharing emits a REQUEST_CREATED event which does not trigger push
// notifications, so there is no notification channel signal to wait for.
func TestShareRequestToCommunity_Success(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requesterID)
	community2ID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Create request in community1.
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	// Share with community2.
	shareRequestInto(t, ctx, service, requestID, community2ID)

	// Verify a CommunityRequest junction row was created for community2.
	rows, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id":   requestID,
		"community_id": community2ID,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("failed to query CommunityRequest: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 CommunityRequest row for community2, got %d", len(rows))
	}

	// Verify a REQUEST_CREATED event was emitted for community2.
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": community2ID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 REQUEST_CREATED event for community2, got %d", len(events))
	}
}

// TestShareRequestToCommunity_AlreadyShared verifies that sharing with a
// community the request is already in is idempotent — no duplicate
// CommunityRequest row is created. CommunityService.ShareItem relies on this:
// re-sharing is a success, not an AlreadyExists.
func TestShareRequestToCommunity_AlreadyShared(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	// Share with the same community the request is already in.
	shareRequestInto(t, ctx, service, requestID, communityID)

	// There should still be exactly one CommunityRequest row.
	rows, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id":   requestID,
		"community_id": communityID,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("failed to query CommunityRequest: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 CommunityRequest row (idempotent), got %d", len(rows))
	}
}

// TestVerifyRequestViewer covers the view-access check gating link-only
// ShareItem calls (#2630): the creator and any member of a community the
// request is shared with pass (and get the creator's ID back); everyone else
// is denied.
func TestVerifyRequestViewer(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	ctx := context.Background()

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	memberID := setupTestUser(t, testStorage, "Member", "member@example.com")
	strangerID := setupTestUser(t, testStorage, "Stranger", "stranger@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, memberID)

	requestID, err := testStorage.Insert(ctx, &models.Request{RequesterId: requesterID, Title: "Need a ladder"})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: requestID}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}

	t.Run("creator passes and gets creator id", func(t *testing.T) {
		ownerID, err := service.VerifyRequestViewer(ctx, requestID, requesterID)
		if err != nil || ownerID != requesterID {
			t.Errorf("got (%q, %v), want (%q, nil)", ownerID, err, requesterID)
		}
	})

	t.Run("shared-community member passes and gets creator id", func(t *testing.T) {
		ownerID, err := service.VerifyRequestViewer(ctx, requestID, memberID)
		if err != nil || ownerID != requesterID {
			t.Errorf("got (%q, %v), want (%q, nil)", ownerID, err, requesterID)
		}
	})

	t.Run("non-member is denied", func(t *testing.T) {
		_, err := service.VerifyRequestViewer(ctx, requestID, strangerID)
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("missing request is NotFound", func(t *testing.T) {
		_, err := service.VerifyRequestViewer(ctx, "does-not-exist", requesterID)
		if err == nil || connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})
}

// TestVerifyRequestOwner covers the owner check CommunityService.ShareItem
// applies through the request ItemSharer before any audience mutation runs:
// only the request creator may share or unshare it.
//
// Membership of each target community is checked by ShareItem itself
// (RequireMemberOfActiveCommunity per share_to_community_ids entry), not here —
// see TestShareItem_MemberOnly in the community package.
func TestVerifyRequestOwner(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	otherID := setupTestUser(t, testStorage, "Other", "other@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requesterID, otherID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	t.Run("creator passes", func(t *testing.T) {
		if err := service.VerifyRequestOwner(requesterCtx, requestID, requesterID); err != nil {
			t.Errorf("expected creator to pass, got %v", err)
		}
	})

	t.Run("non-owner is denied", func(t *testing.T) {
		err := service.VerifyRequestOwner(requesterCtx, requestID, otherID)
		if err == nil {
			t.Fatal("expected PermissionDenied error for non-owner, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("missing request is NotFound", func(t *testing.T) {
		err := service.VerifyRequestOwner(requesterCtx, "does-not-exist", requesterID)
		if err == nil || connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})
}

// TestUnshareRequestFromCommunity_Success verifies that a request can be removed
// from a community when it is shared with more than one community.
func TestUnshareRequestFromCommunity_Success(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requesterID)
	community2ID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Create request in community1, then share with community2.
	// Sharing emits REQUEST_CREATED which does not trigger notifications.
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	shareRequestInto(t, ctx, service, requestID, community2ID)

	// Unshare from community2.
	if err := unshareRequestFrom(t, ctx, service, requestID, community2ID); err != nil {
		t.Fatalf("unshare from community2 failed: %v", err)
	}

	// The CommunityRequest for community2 should now be archived.
	rows, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id":   requestID,
		"community_id": community2ID,
		"archived":     true,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("failed to query archived CommunityRequest: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 archived CommunityRequest for community2, got %d", len(rows))
	}

	// The request should still be live in community1.
	liveRows, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"request_id":   requestID,
		"community_id": community1ID,
		"archived":     false,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("failed to query live CommunityRequest: %v", err)
	}
	if len(liveRows) != 1 {
		t.Errorf("expected 1 live CommunityRequest for community1, got %d", len(liveRows))
	}
}

// TestUnshareRequestFromCommunity_LastCommunity verifies that unsharing from the
// last community returns FailedPrecondition. This guard is what makes the
// request unshare path diverge from gear's and the experience's, and it lives in
// the core so CommunityService.UnshareItem inherits it.
func TestUnshareRequestFromCommunity_LastCommunity(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Submit without an explicit community_id: the request lives ONLY in its
	// per-item community (#2492), which is therefore the last (and only) one.
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId
	itemCommunityID := createResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("expected SubmitRequest to return ItemCommunityId")
	}

	err = unshareRequestFrom(t, ctx, service, requestID, itemCommunityID)
	if err == nil {
		t.Fatal("expected FailedPrecondition error when unsharing from last community, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected CodeFailedPrecondition, got %v", connect.CodeOf(err))
	}
}

// TestUnshareRequestFromCommunity_NonOwner verifies that a non-owner cannot
// unshare a request. Unlike the share path, the request unshare core keeps its
// own owner check, so this is asserted here rather than on VerifyRequestOwner.
func TestUnshareRequestFromCommunity_NonOwner(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	otherID := setupTestUser(t, testStorage, "Other", "other@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requesterID, otherID)
	community2ID := setupCommunityWithMembers(t, testStorage, requesterID, otherID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	shareRequestInto(t, requesterCtx, service, requestID, community2ID)

	// Other user tries to unshare.
	otherCtx := createAuthenticatedContext(otherID, "other@example.com", models.Role_ROLE_USER)
	err = unshareRequestFrom(t, otherCtx, service, requestID, community2ID)
	if err == nil {
		t.Fatal("expected PermissionDenied for non-owner unshare, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
}

// TestUnshareRequestFromCommunity_NotSharedWithCommunity verifies that unsharing
// from a community the request was never shared with returns NotFound.
func TestUnshareRequestFromCommunity_NotSharedWithCommunity(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requesterID)
	community2ID := setupCommunityWithMembers(t, testStorage, requesterID)
	community3ID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Share with community1 and community2, but not community3.
	// Sharing emits REQUEST_CREATED which does not trigger notifications.
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	shareRequestInto(t, ctx, service, requestID, community2ID)

	// Attempt to unshare from community3 (never shared).
	err = unshareRequestFrom(t, ctx, service, requestID, community3ID)
	if err == nil {
		t.Fatal("expected NotFound for unsharing from community that was never shared, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}
