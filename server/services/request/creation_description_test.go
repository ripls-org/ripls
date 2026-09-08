package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// submitRequestWithDescription submits a request with the given description and
// returns its conversation ID.
func submitRequestWithDescription(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, requesterID, communityID, description string, stockImageryDone chan struct{}) (conversationID string) {
	t.Helper()
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	resp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Power Drill",
		Description: description,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)

	requestStored := &models.Request{}
	if err := testStorage.GetByID(context.Background(), resp.Msg.RequestId, requestStored); err != nil {
		t.Fatalf("failed to read request: %v", err)
	}
	return requestStored.ConversationId
}

func TestSubmitRequest_SeedsDescriptionAsFirstComment(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	const desc = "Need a power drill for the weekend, returning Sunday."
	conversationID := submitRequestWithDescription(t, service, testStorage, requesterID, communityID, desc, stockImageryDone)

	msgs, err := storage.ListByConversation(context.Background(), testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (anchor + description), got %d", len(msgs))
	}
	if !chat.IsCreationAnchorMessage(msgs[0]) {
		t.Errorf("first message should be the REQUEST_CREATED anchor, got %+v", msgs[0])
	}
	user := msgs[1].GetUserMessage()
	if user == nil {
		t.Fatalf("second message should be the description user comment, got %+v", msgs[1])
	}
	if user.SenderId != requesterID {
		t.Errorf("comment sender = %q, want requester %q", user.SenderId, requesterID)
	}
	if user.Text != desc {
		t.Errorf("comment text = %q, want %q", user.Text, desc)
	}
}

func TestSubmitRequest_EmptyDescriptionSeedsNoComment(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	conversationID := submitRequestWithDescription(t, service, testStorage, requesterID, communityID, "", stockImageryDone)

	msgs, err := storage.ListByConversation(context.Background(), testStorage, conversationID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	for _, m := range msgs {
		if m.GetUserMessage() != nil {
			t.Errorf("expected no user comment for empty description, got %q", m.GetUserMessage().Text)
		}
	}
}
