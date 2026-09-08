package request

import (
	"context"
	"sort"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	undotesting "go.ripls.org/ripls/server/undo/testing"
)

// setupRequestWithOffer creates a shared request in a community with
// one offer — the standard pre-condition for MarkRequestFulfilled /
// CancelRequest round-trip tests.
func setupRequestWithOffer(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, done, stockImageryDone chan struct{}) (requestID, communityID, requesterID, offererID string) {
	t.Helper()
	requesterID = setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID = setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID = setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(reqCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID = createResp.Msg.RequestId
	shareRequestInto(t, reqCtx, service, createResp.Msg.RequestId, communityID)

	offCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	if _, err := service.OfferToFulfill(offCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("OfferToFulfill: %v", err)
	}
	services.WaitForNotification(t, done)

	return requestID, communityID, requesterID, offererID
}

// TestUndoMarkRequestFulfilled_RoundTrip covers the full round-trip —
// request + every CommunityRequest row (whose Archived flag is flipped
// by fulfillment) + pre-action chat messages.
func TestUndoMarkRequestFulfilled_RoundTrip(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)
	requestID, _, requesterID, _ := setupRequestWithOffer(t, service, testStorage, done, stockImageryDone)

	ctx := context.Background()
	requestStored := &models.Request{}
	if err := testStorage.GetByID(ctx, requestID, requestStored); err != nil {
		t.Fatalf("load request: %v", err)
	}
	conversationID := requestStored.ConversationId
	preActionChatIDs := requestChatMessageIDs(t, testStorage, conversationID)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "mark-request-fulfilled",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotRequestComposite(t, testStorage, requestID, conversationID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.MarkRequestFulfilled(reqCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
				RequestId: requestID,
			}))
			if err != nil {
				t.Fatalf("MarkRequestFulfilled: %v", err)
			}
			services.WaitForNotification(t, done)
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotRequestComposite(t, testStorage, requestID, conversationID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoMarkRequestFulfilled(reqCtx, connect.NewRequest(&api.UndoMarkRequestFulfilledRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoMarkRequestFulfilled: %v", err)
			}
		},
	})
}

// TestUndoCancelRequest_RoundTrip covers the cancel round-trip.
func TestUndoCancelRequest_RoundTrip(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)
	requestID, _, requesterID, _ := setupRequestWithOffer(t, service, testStorage, done, stockImageryDone)

	ctx := context.Background()
	requestStored := &models.Request{}
	if err := testStorage.GetByID(ctx, requestID, requestStored); err != nil {
		t.Fatalf("load request: %v", err)
	}
	conversationID := requestStored.ConversationId
	preActionChatIDs := requestChatMessageIDs(t, testStorage, conversationID)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	undotesting.RoundTrip(t, service, undotesting.Case[*Service]{
		Name: "cancel-request",
		SnapshotBefore: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotRequestComposite(t, testStorage, requestID, conversationID, preActionChatIDs)
		},
		Mutate: func(t *testing.T, _ *Service) string {
			resp, err := service.CancelRequest(reqCtx, connect.NewRequest(&api.CancelRequestRequest{
				RequestId: requestID,
			}))
			if err != nil {
				t.Fatalf("CancelRequest: %v", err)
			}
			return resp.Msg.CommunityEventId
		},
		SnapshotAfter: func(t *testing.T, _ *Service) undotesting.Snapshot {
			return snapshotRequestComposite(t, testStorage, requestID, conversationID, preActionChatIDs)
		},
		Undo: func(t *testing.T, _ *Service, communityEventID string) {
			if _, err := service.UndoCancelRequest(reqCtx, connect.NewRequest(&api.UndoCancelRequestRequest{
				CommunityEventId: communityEventID,
			})); err != nil {
				t.Fatalf("UndoCancelRequest: %v", err)
			}
		},
	})
}

// TestUndoMarkRequestFulfilled_NotActor asserts a non-requester
// cannot undo the fulfillment.
func TestUndoMarkRequestFulfilled_NotActor(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)
	requestID, _, requesterID, offererID := setupRequestWithOffer(t, service, testStorage, done, stockImageryDone)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	resp, err := service.MarkRequestFulfilled(reqCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled: %v", err)
	}
	services.WaitForNotification(t, done)

	otherCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	_, err = service.UndoMarkRequestFulfilled(otherCtx, connect.NewRequest(&api.UndoMarkRequestFulfilledRequest{
		CommunityEventId: resp.Msg.CommunityEventId,
	}))
	if err == nil {
		t.Fatal("expected error from non-actor undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR {
		t.Errorf("reason: want NOT_ACTOR, got %s", reason)
	}
}

// snapshotRequestComposite captures the request + every
// CommunityRequest row (whose Archived flag may be flipped) + all
// pre-action chat messages, sorted deterministically so the round-trip
// comparison doesn't depend on QueryByField ordering.
func snapshotRequestComposite(t *testing.T, s *storage.ProtoSQLStorage, requestID, conversationID string, preActionChatIDs []string) undotesting.Snapshot {
	t.Helper()
	ctx := context.Background()

	request := &models.Request{}
	if err := s.GetByID(ctx, requestID, request); err != nil {
		t.Fatalf("snapshot: load request: %v", err)
	}
	entries := []undotesting.SnapshotEntry{
		{Label: "request", Message: request},
	}

	communityRequests, err := s.QueryByField(ctx, "request_id", requestID, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("snapshot: query community_requests: %v", err)
	}
	crModels := make([]*models.CommunityRequest, 0, len(communityRequests))
	for _, cr := range communityRequests {
		crModels = append(crModels, cr.(*models.CommunityRequest))
	}
	sort.Slice(crModels, func(i, j int) bool { return crModels[i].Id < crModels[j].Id })
	for _, m := range crModels {
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "community_request:" + m.Id,
			Message: m,
		})
	}

	for _, msgID := range preActionChatIDs {
		msg := &models.ChatMessage{}
		if err := s.GetByID(ctx, msgID, msg); err != nil {
			entries = append(entries, undotesting.SnapshotEntry{
				Label:   "chat:" + msgID + " (missing)",
				Message: &models.ChatMessage{Id: msgID},
			})
			continue
		}
		entries = append(entries, undotesting.SnapshotEntry{
			Label:   "chat:" + msg.Id,
			Message: msg,
		})
	}
	return undotesting.Snapshot{Entries: entries}
}

func requestChatMessageIDs(t *testing.T, s *storage.ProtoSQLStorage, conversationID string) []string {
	t.Helper()
	if conversationID == "" {
		return nil
	}
	msgs, err := s.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("chat ids: query: %v", err)
	}
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.(*models.ChatMessage).Id)
	}
	return ids
}

func extractUndoFailureReason(cerr *connect.Error) api.UndoFailureReason {
	for _, d := range cerr.Details() {
		val, err := d.Value()
		if err != nil {
			continue
		}
		if detail, ok := val.(*api.UndoErrorDetail); ok {
			return detail.Reason
		}
	}
	return api.UndoFailureReason_UNDO_FAILURE_REASON_UNSPECIFIED
}
