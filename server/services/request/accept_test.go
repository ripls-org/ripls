package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupAcceptFixture inserts a request (with a conversation) by askerID, plus
// a gear-backed offer from helperID: gear, an origin-linked transfer, and the
// linked contribution. Returns the ids the accept flow needs.
func setupAcceptFixture(t *testing.T, st *storage.ProtoSQLStorage, askerID, helperID string) (requestID, communityID, conversationID, contributionID string) {
	t.Helper()
	ctx := context.Background()

	communityID = setupCommunityWithMembers(t, st, askerID, helperID)

	var err error
	conversationID, err = st.Insert(ctx, &models.ChatConversation{
		CommunityId:    communityID,
		ParticipantIds: []string{askerID, helperID},
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	requestID, err = st.Insert(ctx, &models.Request{
		RequesterId:    askerID,
		Title:          "Looking for a lawn mower",
		State:          models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
		ConversationId: conversationID,
	})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := st.Insert(ctx, &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}

	gearID, err := st.Insert(ctx, &models.Gear{
		Name:    "Honda mower",
		OwnerId: helperID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	transferID, err := st.Insert(ctx, &models.Transfer{
		GearId:       gearID,
		OwnerId:      helperID,
		RecipientId:  askerID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:  communityID,
		Origin:       &models.Transfer_OriginRequestId{OriginRequestId: requestID},
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	contributionID, err = st.Insert(ctx, &models.PlanningContribution{
		ContributorId: helperID,
		Title:         "Honda mower",
		Scope:         &models.PlanningContribution_RequestId{RequestId: requestID},
		GearId:        &gearID,
		TransferId:    &transferID,
	})
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}
	return requestID, communityID, conversationID, contributionID
}

// TestService_AcceptRequestOffer_WritesAcceptedSystemMessage verifies the
// accept beat lands in the request conversation as "X accepted Y's offer"
// (#2724) — previously the accept emitted no chat line at all.
func TestService_AcceptRequestOffer_WritesAcceptedSystemMessage(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	askerID := setupTestUser(t, testStorage, "June", "june@example.com")
	helperID := setupTestUser(t, testStorage, "Lisa", "lisa@example.com")
	requestID, _, conversationID, contributionID := setupAcceptFixture(t, testStorage, askerID, helperID)

	ctx := createAuthenticatedContext(askerID, "june@example.com", models.Role_ROLE_USER)
	if _, err := service.AcceptRequestOffer(ctx, connect.NewRequest(&api.AcceptRequestOfferRequest{
		RequestId:      requestID,
		ContributionId: contributionID,
	})); err != nil {
		t.Fatalf("AcceptRequestOffer: %v", err)
	}

	msgs, err := testStorage.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	var found bool
	for _, m := range msgs {
		sys := m.(*models.ChatMessage).GetSystemMessage()
		if sys == nil {
			continue
		}
		if sys.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED {
			found = true
			if sys.Description != "June accepted Lisa's offer" {
				t.Errorf("Description = %q, want accept line", sys.Description)
			}
			if sys.GetTemplateKey() != "chat.request.offer_accepted" {
				t.Errorf("TemplateKey = %q, want chat.request.offer_accepted", sys.GetTemplateKey())
			}
		}
	}
	if !found {
		t.Fatal("no APPROVED system message written on accept")
	}
}

// TestService_AcceptRequestOffer_ToggleOffEmitsNothing verifies clearing an
// acceptance stays silent — no second system line, and the first is not
// duplicated.
func TestService_AcceptRequestOffer_ToggleOffEmitsNothing(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	askerID := setupTestUser(t, testStorage, "June", "june@example.com")
	helperID := setupTestUser(t, testStorage, "Lisa", "lisa@example.com")
	requestID, _, conversationID, contributionID := setupAcceptFixture(t, testStorage, askerID, helperID)

	ctx := createAuthenticatedContext(askerID, "june@example.com", models.Role_ROLE_USER)
	acceptReq := &api.AcceptRequestOfferRequest{RequestId: requestID, ContributionId: contributionID}
	if _, err := service.AcceptRequestOffer(ctx, connect.NewRequest(acceptReq)); err != nil {
		t.Fatalf("AcceptRequestOffer (on): %v", err)
	}
	if _, err := service.AcceptRequestOffer(ctx, connect.NewRequest(acceptReq)); err != nil {
		t.Fatalf("AcceptRequestOffer (off): %v", err)
	}

	msgs, err := testStorage.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	count := 0
	for _, m := range msgs {
		sys := m.(*models.ChatMessage).GetSystemMessage()
		if sys != nil && sys.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED {
			count++
		}
	}
	if count != 1 {
		t.Errorf("APPROVED system messages = %d, want exactly 1 (toggle-off is silent)", count)
	}
}
