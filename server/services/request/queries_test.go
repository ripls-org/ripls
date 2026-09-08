package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// assertConnectCode is a helper for asserting a connect error code.
func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a connect error with code %v, got nil", want)
	}
	if connect.CodeOf(err) != want {
		t.Errorf("expected connect code %v, got %v (err: %v)", want, connect.CodeOf(err), err)
	}
}

func TestService_GetRequest(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a ladder",
	})
	createResp, err := service.SubmitRequest(ctx, createReq)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId

	// Get the request
	getReq := connect.NewRequest(&api.GetRequestRequest{
		RequestId: requestID,
	})

	getResp, err := service.GetRequest(ctx, getReq)
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}

	if getResp.Msg.Request.Id != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, getResp.Msg.Request.Id)
	}

	if getResp.Msg.Request.Description != "Looking for a ladder" {
		t.Errorf("Expected description 'Looking for a ladder', got %s", getResp.Msg.Request.Description)
	}

	if getResp.Msg.Request.Requester.Id != requesterID {
		t.Errorf("Expected requester ID %s, got %s", requesterID, getResp.Msg.Request.Requester.Id)
	}

	if getResp.Msg.Request.Requester.Name != "Requester" {
		t.Errorf("Expected requester name 'Requester', got %s", getResp.Msg.Request.Requester.Name)
	}
}

// TestGetRequest_InvitedIndividuals verifies that people individually invited to
// a request's own ad-hoc origin community surface in InvitedIndividuals for the
// "Shared with" roster, excluding the requester. A request has no RSVP, so every
// invited member other than the requester is listed (#2492).
func TestGetRequest_InvitedIndividuals(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	aliceID := setupTestUser(t, testStorage, "Alice", "alice@example.com")
	bobID := setupTestUser(t, testStorage, "Bob", "bob@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	// SubmitRequest provisions the request's per-item ad-hoc origin community and
	// shares the request into it, so it is the sole shared community here.
	resp1, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest (pre-invite) failed: %v", err)
	}
	if len(resp1.Msg.Request.SharedCommunities) != 1 {
		t.Fatalf("expected exactly one (origin) shared community, got %d", len(resp1.Msg.Request.SharedCommunities))
	}
	originID := resp1.Msg.Request.SharedCommunities[0].CommunityId

	// Directly invite alice + bob into the origin community.
	for _, uid := range []string{aliceID, bobID} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{
			CommunityId: originID, UserId: uid, InviterId: requesterID,
		}); err != nil {
			t.Fatalf("failed to add membership for %s: %v", uid, err)
		}
	}

	resp2, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest (post-invite) failed: %v", err)
	}
	got := make(map[string]bool, len(resp2.Msg.Request.InvitedIndividuals))
	for _, u := range resp2.Msg.Request.InvitedIndividuals {
		got[u.Id] = true
	}
	if got[requesterID] {
		t.Errorf("requester must not appear in InvitedIndividuals")
	}
	if !got[aliceID] || !got[bobID] || len(got) != 2 {
		t.Errorf("InvitedIndividuals = %v, want exactly alice (%s) + bob (%s)", got, aliceID, bobID)
	}
}

func TestService_ListRequests(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	// Create multiple requests
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	for i := 0; i < 3; i++ {
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Description: "Test request",
		})
		resp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
	}

	// List requests
	listReq := connect.NewRequest(&api.ListRequestsRequest{
		CommunityId: communityID,
	})

	listResp, err := service.ListRequests(ctx, listReq)
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}

	if len(listResp.Msg.Requests) != 3 {
		t.Errorf("Expected 3 requests, got %d", len(listResp.Msg.Requests))
	}
}

func TestService_ListMyRequests(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requester1ID := setupTestUser(t, testStorage, "Requester1", "requester1@example.com")
	requester2ID := setupTestUser(t, testStorage, "Requester2", "requester2@example.com")
	community1ID := setupCommunityWithMembers(t, testStorage, requester1ID, requester2ID)
	community2ID := setupCommunityWithMembers(t, testStorage, requester1ID, requester2ID)

	// Create requests for requester1 in both communities
	ctx1 := createAuthenticatedContext(requester1ID, "requester1@example.com", models.Role_ROLE_USER)
	for i := 0; i < 2; i++ {
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Description: "Test request in community 1",
		})
		resp, err := service.SubmitRequest(ctx1, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		shareRequestInto(t, ctx1, service, resp.Msg.RequestId, community1ID)
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
	}

	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Test request in community 2",
	})
	resp, err := service.SubmitRequest(ctx1, createReq)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx1, service, resp.Msg.RequestId, community2ID)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	// Create a request for requester2
	ctx2 := createAuthenticatedContext(requester2ID, "requester2@example.com", models.Role_ROLE_USER)
	createReq2 := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Requester2's request",
	})
	resp2, err := service.SubmitRequest(ctx2, createReq2)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx2, service, resp2.Msg.RequestId, community1ID)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	// List all requests by requester1
	listAllReq := connect.NewRequest(&api.ListMyRequestsRequest{})
	listAllResp, err := service.ListMyRequests(ctx1, listAllReq)
	if err != nil {
		t.Fatalf("ListMyRequests failed: %v", err)
	}

	if len(listAllResp.Msg.Requests) != 3 {
		t.Errorf("Expected 3 requests for requester1, got %d", len(listAllResp.Msg.Requests))
	}

	// List requests by requester1 filtered by community1
	listFilteredReq := connect.NewRequest(&api.ListMyRequestsRequest{
		CommunityId: community1ID,
	})
	listFilteredResp, err := service.ListMyRequests(ctx1, listFilteredReq)
	if err != nil {
		t.Fatalf("ListMyRequests with filter failed: %v", err)
	}

	if len(listFilteredResp.Msg.Requests) != 2 {
		t.Errorf("Expected 2 requests for requester1 in community1, got %d", len(listFilteredResp.Msg.Requests))
	}
}

func TestService_ListMyRequests_NoN1(t *testing.T) {
	// Seed 30+ requests and assert that ListMyRequests uses ≤12 DB queries
	// regardless of inbox size, proving the N+1 regression is CI-blocked.
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "RequesterN1", "rn1@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "rn1@example.com", models.Role_ROLE_USER)

	const numRequests = 30
	for i := 0; i < numRequests; i++ {
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Description: "N+1 test request",
			MediaIds:    []string{"media-n1-static"}, // skip async stock imagery fetch
		})
		resp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest %d failed: %v", i, err)
		}
		shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)
	}

	// Insert a chat message into each request's conversation so that the
	// conversation-enrichment path (BatchEnrichWithCommentPreviewAndCounts)
	// is exercised with real data.
	allReqs, err := storage.QueryByFields[*models.Request](testStorage, context.Background(), map[string]any{
		"requester_id": requesterID,
	})
	if err != nil {
		t.Fatalf("failed to fetch inserted requests: %v", err)
	}
	for _, req := range allReqs {
		if req.ConversationId == "" {
			continue
		}
		msg := &models.ChatMessage{
			ConversationId: req.ConversationId,
			SentAtUnixSec:  1000,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: requesterID,
					Text:     "hello",
				},
			},
			ParticipantIdToIsRead: map[string]bool{requesterID: true},
		}
		if _, insertErr := testStorage.Insert(context.Background(), msg); insertErr != nil {
			t.Fatalf("failed to insert chat message: %v", insertErr)
		}
	}

	listReq := connect.NewRequest(&api.ListMyRequestsRequest{})
	statsCtx := storage.WithQueryStats(ctx)
	var listResp *connect.Response[api.ListMyRequestsResponse]
	storage.AssertMaxQueries(t, statsCtx, 12, func() {
		listResp, err = service.ListMyRequests(statsCtx, listReq)
	})
	if err != nil {
		t.Fatalf("ListMyRequests failed: %v", err)
	}
	if len(listResp.Msg.Requests) != numRequests {
		t.Errorf("expected %d requests, got %d", numRequests, len(listResp.Msg.Requests))
	}
}

func TestService_ListRequests_NoN1(t *testing.T) {
	// Mirrors TestService_ListMyRequests_NoN1 for the community-scoped list handler.
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "RequesterN1R", "rn1r@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "rn1r@example.com", models.Role_ROLE_USER)

	const numRequests = 30
	for i := 0; i < numRequests; i++ {
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Description: "N+1 test request for ListRequests",
			MediaIds:    []string{"media-n1-static"},
		})
		resp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest %d failed: %v", i, err)
		}
		shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)
	}

	listReq := connect.NewRequest(&api.ListRequestsRequest{CommunityId: communityID})
	statsCtx := storage.WithQueryStats(ctx)
	var (
		listResp *connect.Response[api.ListRequestsResponse]
		err      error
	)
	storage.AssertMaxQueries(t, statsCtx, 12, func() {
		listResp, err = service.ListRequests(statsCtx, listReq)
	})
	if err != nil {
		t.Fatalf("ListRequests failed: %v", err)
	}
	if len(listResp.Msg.Requests) != numRequests {
		t.Errorf("expected %d requests, got %d", numRequests, len(listResp.Msg.Requests))
	}
}

func TestService_ListMyRequests_BatchedShapeMatchesSingular(t *testing.T) {
	// Verify that the batched list path produces identical field values to the
	// singular GetRequest path for the same requests. SharedCommunities is
	// excluded because GetRequest populates it via populateRequestSharedCommunities
	// but ListMyRequests does not.
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "RequesterMatch", "rm@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "rm@example.com", models.Role_ROLE_USER)

	const numRequests = 3
	requestIDs := make([]string, 0, numRequests)
	for i := 0; i < numRequests; i++ {
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Description: "Match test request",
			MediaIds:    []string{"media-match-static"},
		})
		resp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest %d failed: %v", i, err)
		}
		shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)
		requestIDs = append(requestIDs, resp.Msg.RequestId)
	}

	// Insert a chat message into each conversation so MessageCount is non-zero.
	allReqs, err := storage.QueryByFields[*models.Request](testStorage, context.Background(), map[string]any{
		"requester_id": requesterID,
	})
	if err != nil {
		t.Fatalf("failed to fetch requests: %v", err)
	}
	for _, req := range allReqs {
		if req.ConversationId == "" {
			continue
		}
		msg := &models.ChatMessage{
			ConversationId: req.ConversationId,
			SentAtUnixSec:  1000,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: requesterID,
					Text:     "hello",
				},
			},
			ParticipantIdToIsRead: map[string]bool{requesterID: true},
		}
		if _, insertErr := testStorage.Insert(context.Background(), msg); insertErr != nil {
			t.Fatalf("failed to insert chat message: %v", insertErr)
		}
	}

	// Collect ListMyRequests results indexed by request ID.
	listResp, err := service.ListMyRequests(ctx, connect.NewRequest(&api.ListMyRequestsRequest{}))
	if err != nil {
		t.Fatalf("ListMyRequests failed: %v", err)
	}
	listByID := make(map[string]*api.Request, len(listResp.Msg.Requests))
	for _, r := range listResp.Msg.Requests {
		listByID[r.Id] = r
	}

	// Compare field-by-field with GetRequest for each request.
	for _, reqID := range requestIDs {
		getResp, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: reqID}))
		if err != nil {
			t.Fatalf("GetRequest(%s) failed: %v", reqID, err)
		}
		single := getResp.Msg.Request
		batched := listByID[reqID]
		if batched == nil {
			t.Errorf("request %s not found in ListMyRequests response", reqID)
			continue
		}

		if batched.Id != single.Id {
			t.Errorf("Id mismatch for %s: list=%q get=%q", reqID, batched.Id, single.Id)
		}
		if batched.Description != single.Description {
			t.Errorf("Description mismatch for %s: list=%q get=%q", reqID, batched.Description, single.Description)
		}
		if batched.State != single.State {
			t.Errorf("State mismatch for %s: list=%v get=%v", reqID, batched.State, single.State)
		}
		if batched.Requester == nil || single.Requester == nil {
			t.Errorf("nil Requester for %s: list=%v get=%v", reqID, batched.Requester, single.Requester)
		} else if batched.Requester.Id != single.Requester.Id {
			t.Errorf("Requester.Id mismatch for %s: list=%q get=%q", reqID, batched.Requester.Id, single.Requester.Id)
		}
		if batched.MessageCount != single.MessageCount {
			t.Errorf("MessageCount mismatch for %s: list=%d get=%d", reqID, batched.MessageCount, single.MessageCount)
		}
		if batched.CommunityId != single.CommunityId {
			t.Errorf("CommunityId mismatch for %s: list=%q get=%q", reqID, batched.CommunityId, single.CommunityId)
		}
		// ConversationId is a pointer; both should be non-nil after request creation.
		if (batched.ConversationId == nil) != (single.ConversationId == nil) {
			t.Errorf("ConversationId nil mismatch for %s: list=%v get=%v", reqID, batched.ConversationId, single.ConversationId)
		} else if batched.ConversationId != nil && *batched.ConversationId != *single.ConversationId {
			t.Errorf("ConversationId mismatch for %s: list=%q get=%q", reqID, *batched.ConversationId, *single.ConversationId)
		}
	}
}

// TestGetRequest_AccessControl verifies the community-scoped access gate on GetRequest.
func TestGetRequest_AccessControl(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	memberID := setupTestUser(t, testStorage, "Member", "member@example.com")
	strangerID := setupTestUser(t, testStorage, "Stranger", "stranger@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, memberID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a tent for the weekend",
		MediaIds:    []string{"media-static"}, // explicit media skips async stock imagery fetch
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	t.Run("requester can read own request", func(t *testing.T) {
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		resp, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		if err != nil {
			t.Fatalf("GetRequest failed: %v", err)
		}
		if resp.Msg.Request.Id != requestID {
			t.Errorf("expected request ID %s, got %s", requestID, resp.Msg.Request.Id)
		}
	})

	t.Run("community member can read request", func(t *testing.T) {
		ctx := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)
		resp, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		if err != nil {
			t.Fatalf("GetRequest failed for member: %v", err)
		}
		if resp.Msg.Request.Id != requestID {
			t.Errorf("expected request ID %s, got %s", requestID, resp.Msg.Request.Id)
		}
	})

	t.Run("stranger is denied", func(t *testing.T) {
		ctx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)
		_, err := service.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for stranger, got nil")
		}
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})
}

// TestListRequestNeedsAndContributions_AccessControl verifies the access gate.
func TestListRequestNeedsAndContributions_AccessControl(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "NACRequester", "nac-req@example.com")
	strangerID := setupTestUser(t, testStorage, "NACStranger", "nac-stranger@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "nac-req@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
		MediaIds:    []string{"media-static"}, // explicit media skips async stock imagery fetch
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	t.Run("requester can list needs and contributions", func(t *testing.T) {
		ctx := createAuthenticatedContext(requesterID, "nac-req@example.com", models.Role_ROLE_USER)
		_, err := service.ListRequestNeedsAndContributions(ctx, connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
		if err != nil {
			t.Fatalf("ListRequestNeedsAndContributions failed: %v", err)
		}
	})

	t.Run("stranger is denied", func(t *testing.T) {
		ctx := createAuthenticatedContext(strangerID, "nac-stranger@example.com", models.Role_ROLE_USER)
		_, err := service.ListRequestNeedsAndContributions(ctx, connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for stranger, got nil")
		}
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})
}
