package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

func TestService_DeleteRequest(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	otherUserID := setupTestUser(t, testStorage, "Other User", "other@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, otherUserID)

	t.Run("successful deletion", func(t *testing.T) {
		// Create a request
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Power Drill",
			Description: "Looking for a power drill for weekend project",
		})
		createResp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
		requestID := createResp.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

		// Delete the request
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: requestID,
		})

		_, err = service.DeleteRequest(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteRequest failed: %v", err)
		}

		// Verify request is no longer retrievable via GetByID (soft delete filtering)
		request := &models.Request{}
		err = testStorage.GetByID(context.Background(), requestID, request)
		if err == nil {
			t.Error("Expected error when getting deleted request, but got none")
		}

		// Verify request still exists with IncludeDeleted option
		err = testStorage.GetByID(context.Background(), requestID, request, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted request with IncludeDeleted: %v", err)
		}

		if request.Deleted == nil {
			t.Error("Expected deleted metadata to be present")
		}
		if request.Deleted.DeletedByUserId != requesterID {
			t.Errorf("Expected deleted_by_user_id %s, got %s", requesterID, request.Deleted.DeletedByUserId)
		}
		if request.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})

	t.Run("non-owner cannot delete", func(t *testing.T) {
		// Create a request as requester
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Another Request",
			Description: "Test request",
		})
		createResp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
		requestID := createResp.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

		// Try to delete as other user
		ctx = createAuthenticatedContext(otherUserID, "other@example.com", models.Role_ROLE_USER)
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: requestID,
		})

		_, err = service.DeleteRequest(ctx, deleteReq)
		if err == nil {
			t.Error("Expected error when non-owner tries to delete, but got none")
		}
		// Verify it's a permission error
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected permission_denied error, got %v", err)
		}
	})

	t.Run("delete non-existent request", func(t *testing.T) {
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: "non-existent-id",
		})

		_, err := service.DeleteRequest(ctx, deleteReq)
		if err == nil {
			t.Error("Expected error when deleting non-existent request, but got none")
		}
		// Verify it's a not found error
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("Expected not_found error, got %v", err)
		}
	})

	t.Run("deletion cascades to conversation", func(t *testing.T) {
		// Create a request
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Request with Conversation",
			Description: "Looking for something",
		})
		createResp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
		requestID := createResp.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

		// Get the Request to verify conversation was created
		requestStored := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, requestStored); err != nil {
			t.Fatalf("Failed to get Request: %v", err)
		}
		if requestStored.ConversationId == "" {
			t.Fatal("Expected request to have a conversation_id")
		}
		conversationID := requestStored.ConversationId

		// Verify conversation exists
		conversation := &models.ChatConversation{}
		err = testStorage.GetByID(context.Background(), conversationID, conversation)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		// Delete the request
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: requestID,
		})
		_, err = service.DeleteRequest(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteRequest failed: %v", err)
		}

		// Verify conversation is no longer retrievable via GetByID (soft delete filtering)
		err = testStorage.GetByID(context.Background(), conversationID, conversation)
		if err == nil {
			t.Error("Expected error when getting deleted conversation, but got none")
		}

		// Verify conversation still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(context.Background(), conversationID, conversation, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted conversation with IncludeDeleted: %v", err)
		}

		if conversation.Deleted == nil {
			t.Error("Expected conversation to have deleted metadata")
		}
		if conversation.Deleted.DeletedByUserId != requesterID {
			t.Errorf("Expected conversation deleted_by_user_id %s, got %s", requesterID, conversation.Deleted.DeletedByUserId)
		}
		if conversation.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected conversation deleted_at_unix_sec to be set")
		}
	})

	t.Run("deletion cascades to media", func(t *testing.T) {
		// Create media first
		media1 := &models.Media{
			UserId:      requesterID,
			ContentType: "image/jpeg",
			Filename:    proto.String("request-image-1.jpg"),
		}
		media2 := &models.Media{
			UserId:      requesterID,
			ContentType: "image/png",
			Filename:    proto.String("request-image-2.png"),
		}
		mediaID1, err := testStorage.Insert(context.Background(), media1)
		if err != nil {
			t.Fatalf("Failed to create media1: %v", err)
		}
		mediaID2, err := testStorage.Insert(context.Background(), media2)
		if err != nil {
			t.Fatalf("Failed to create media2: %v", err)
		}

		// Create a request with media
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
		createReq := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Request with Media",
			Description: "Request that has associated media",
			MediaIds:    []string{mediaID1, mediaID2},
		})
		createResp, err := service.SubmitRequest(ctx, createReq)
		if err != nil {
			t.Fatalf("SubmitRequest failed: %v", err)
		}
		requestID := createResp.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

		// Verify media exists before deletion
		media := &models.Media{}
		err = testStorage.GetByID(context.Background(), mediaID1, media)
		if err != nil {
			t.Fatalf("Failed to get media1 before deletion: %v", err)
		}

		// Delete the request
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: requestID,
		})
		_, err = service.DeleteRequest(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteRequest failed: %v", err)
		}

		// Verify media is no longer retrievable via GetByID (soft delete filtering)
		err = testStorage.GetByID(context.Background(), mediaID1, media)
		if err == nil {
			t.Error("Expected error when getting deleted media1, but got none")
		}
		err = testStorage.GetByID(context.Background(), mediaID2, media)
		if err == nil {
			t.Error("Expected error when getting deleted media2, but got none")
		}

		// Verify media still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(context.Background(), mediaID1, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media1 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media1 to have deleted metadata")
		}
		if media.Deleted.DeletedByUserId != requesterID {
			t.Errorf("Expected media1 deleted_by_user_id %s, got %s", requesterID, media.Deleted.DeletedByUserId)
		}

		err = testStorage.GetByID(context.Background(), mediaID2, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media2 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media2 to have deleted metadata")
		}
	})

	t.Run("deleted request excluded from listings", func(t *testing.T) {
		// Create two requests
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

		createReq1 := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Request To Keep",
			Description: "This one stays",
		})
		keepResp, err := service.SubmitRequest(ctx, createReq1)
		if err != nil {
			t.Fatalf("SubmitRequest 1 failed: %v", err)
		}
		shareRequestInto(t, ctx, service, keepResp.Msg.RequestId, communityID)
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

		createReq2 := connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Request To Delete",
			Description: "This one gets deleted",
		})
		createResp2, err := service.SubmitRequest(ctx, createReq2)
		if err != nil {
			t.Fatalf("SubmitRequest 2 failed: %v", err)
		}
		services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch
		deletedRequestID := createResp2.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp2.Msg.RequestId, communityID)

		// Delete the second request
		deleteReq := connect.NewRequest(&api.DeleteRequestRequest{
			RequestId: deletedRequestID,
		})
		_, err = service.DeleteRequest(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteRequest failed: %v", err)
		}

		// Query requests for the community via CommunityRequest join table - deleted one should not appear
		communityRequests, err := testStorage.QueryByField(context.Background(), "community_id", communityID, &models.CommunityRequest{})
		if err != nil {
			t.Fatalf("QueryByField failed: %v", err)
		}

		for _, cr := range communityRequests {
			communityRequest := cr.(*models.CommunityRequest)
			// Now get the actual request
			request := &models.Request{}
			if err := testStorage.GetByID(context.Background(), communityRequest.RequestId, request); err != nil {
				continue // Skip if request not found
			}
			// Check if this is the deleted request - it should be marked as deleted
			if request.Id == deletedRequestID {
				if request.Deleted == nil {
					t.Error("Deleted request should have Deleted metadata set")
				}
			}
		}
	})

	t.Run("deletion cascades to community_request", func(t *testing.T) {
		ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

		createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
			Title:       "Cascade CR Request",
			Description: "request with cascade coverage",
		}))
		if err != nil {
			t.Fatalf("SubmitRequest: %v", err)
		}
		requestID := createResp.Msg.RequestId
		shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)
		services.WaitForStockImagery(t, stockImageryDone)

		if _, err := service.DeleteRequest(ctx, connect.NewRequest(&api.DeleteRequestRequest{RequestId: requestID})); err != nil {
			t.Fatalf("DeleteRequest: %v", err)
		}

		// After delete, live queries should exclude community_request rows for this request.
		rowsLive, err := testStorage.QueryByField(context.Background(), "request_id", requestID, &models.CommunityRequest{})
		if err != nil {
			t.Fatalf("QueryByField live: %v", err)
		}
		if len(rowsLive) != 0 {
			t.Errorf("expected 0 live community_request rows after delete, got %d", len(rowsLive))
		}

		// IncludeDeleted should still surface them, now with Deleted metadata.
		rowsAll, err := testStorage.QueryByField(context.Background(), "request_id", requestID, &models.CommunityRequest{}, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("QueryByField include-deleted: %v", err)
		}
		if len(rowsAll) == 0 {
			t.Fatal("expected at least one community_request row (soft-deleted) for this request")
		}
		for _, m := range rowsAll {
			cr := m.(*models.CommunityRequest)
			if cr.Deleted == nil || cr.Deleted.DeletedAtUnixSec == 0 {
				t.Errorf("community_request %s: expected soft-delete, got Deleted=%v", cr.Id, cr.Deleted)
			}
			if cr.Deleted != nil && cr.Deleted.DeletedByUserId != requesterID {
				t.Errorf("community_request %s: expected DeletedByUserId=%q, got %q", cr.Id, requesterID, cr.Deleted.DeletedByUserId)
			}
		}
	})
}
