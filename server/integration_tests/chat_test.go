package integration_tests

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// TestServer_Integration_ChatMessaging tests end-to-end chat functionality including:
// - Creating a conversation about a loan
// - Sending messages between lender and borrower
// - Real-time message delivery via streaming
// - Message history retrieval.
func TestServer_Integration_ChatMessaging(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Register first user (lender)
	lenderToken, lenderID := registerFirstUser(t, serverURL, "lender@example.com", "Lender User")

	// Create authenticated clients for lender
	lenderGearClient := createAuthGearClient(lenderToken, serverURL)
	lenderCommunityClient := createAuthCommunityClient(lenderToken, serverURL)
	lenderChatClient := createAuthChatClient(lenderToken, serverURL)

	// Create a community
	createResp, err := lenderCommunityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "Community for testing chat",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	// Register second user (borrower) via community invitation
	borrowerToken, _ := registerUserByInvite(t, serverURL, lenderToken, communityID, "borrower@example.com", "Borrower User")

	// Create authenticated clients for borrower
	borrowerChatClient := createAuthChatClient(borrowerToken, serverURL)
	borrowerLoanClient := createAuthLoanClient(borrowerToken, serverURL)

	// Add gear
	gearResp, err := lenderGearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Test Drill"),
		Description: proto.String("For chat testing"),
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	gearID := gearResp.Msg.Id

	// SaveGear provisions the gear's per-item community (#2492) and shares the
	// gear into it. The gear's single conversation is bound to this per-item
	// community, so every conversation lookup below reports it — not the named
	// community the gear is additionally shared into via ShareGear.
	itemCommunityID := gearResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("Expected SaveGear to return the per-item community ID")
	}

	// Share gear with community
	shareGearIntoCommunity(t, ctx, lenderCommunityClient, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	// Request and approve transfer
	expressInterestResp, err := borrowerLoanClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}

	// Get the transfer ID and conversation ID from the enriched response
	// Loans are auto-approved, so conversation is created immediately on ExpressInterest
	transferID := expressInterestResp.Msg.Transfer.Id
	conversationID := expressInterestResp.Msg.Transfer.GetConversationId()
	if conversationID == "" {
		t.Fatal("Expected conversation ID to be returned from ExpressInterest")
	}

	// Test 2: Verify we can get the conversation history (conversation already exists from ExpressInterest/SelectRecipient)
	historyResp0, err := lenderChatClient.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}

	// Should have 3 messages at this point:
	// 1. LOAN_SHARED system message from ShareGear (anchors gear info card)
	// 2. The gear's description seeded as the lender's first comment, posted by
	//    ShareGear right after the anchor (PostCreationDescription).
	// 3. JOINED from ExpressInterest (loans are auto-approved, no SELECT_RECIPIENT message)
	if len(historyResp0.Msg.Messages) != 3 {
		t.Errorf("Expected 3 messages before user messages (LOAN_SHARED + description comment + JOINED), got %d", len(historyResp0.Msg.Messages))
	}

	// Test 3: Send messages
	sendResp1, err := lenderChatClient.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "Hi, when can you pick up the drill?",
	}))
	if err != nil {
		t.Fatalf("SendMessage (lender) failed: %v", err)
	}

	if sendResp1.Msg.MessageId == "" {
		t.Fatal("Expected message ID to be returned")
	}

	sendResp2, err := borrowerChatClient.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "How about tomorrow at 2pm?",
	}))
	if err != nil {
		t.Fatalf("SendMessage (borrower) failed: %v", err)
	}

	if sendResp2.Msg.MessageId == "" {
		t.Fatal("Expected message ID to be returned")
	}

	// Test 4: Get conversation history
	historyResp, err := lenderChatClient.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}

	// Expect 5 messages: 2 system (LOAN_SHARED + JOINED) + the seeded description
	// comment + 2 user messages.
	if len(historyResp.Msg.Messages) != 5 {
		t.Errorf("Expected 5 messages in history (2 system + description comment + 2 user), got %d", len(historyResp.Msg.Messages))
	}

	// Messages should be ordered newest first
	// (The borrower's message was sent second, so it should appear first)
	foundNewest := false
	for _, msg := range historyResp.Msg.Messages {
		// Extract text from user message oneof
		if msg.GetUserMessage() != nil && msg.GetUserMessage().Text == "How about tomorrow at 2pm?" {
			foundNewest = true
			break
		}
	}
	if !foundNewest {
		t.Error("Expected borrower's message in history")
	}

	// Test 5: Real-time streaming
	// Use a timeout context so stream.Receive() can't block the test forever.
	// 30s gives enough headroom for slow CI environments.
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Second)
	defer streamCancel()

	stream, err := borrowerChatClient.StreamMessages(streamCtx, connect.NewRequest(&api.StreamMessagesRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Receive stream messages in a background goroutine so we can select on
	// a channel with a timeout. Calling stream.Receive() directly in a
	// select/default is broken: Receive blocks, preventing the timeout case
	// from ever firing.
	type streamMsg struct {
		resp *api.StreamMessagesResponse
	}
	msgCh := make(chan streamMsg, 10)
	streamDone := make(chan error, 1)
	go func() {
		for stream.Receive() {
			msgCh <- streamMsg{resp: stream.Msg()}
		}
		streamDone <- stream.Err()
	}()

	// Drain all existing messages: 2 system (LOAN_SHARED + JOINED) + the seeded
	// description comment + 2 user = 5 total.
	for i := range 5 {
		select {
		case <-msgCh:
		case err := <-streamDone:
			t.Fatalf("Stream closed early after %d messages: %v", i, err)
		case <-streamCtx.Done():
			t.Fatalf("Timeout draining existing messages after %d", i)
		}
	}

	// Send a new message while streaming.
	sendResp3, err := lenderChatClient.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "That works for me!",
	}))
	if err != nil {
		t.Fatalf("SendMessage (lender, while streaming) failed: %v", err)
	}

	// Wait for the new message via the channel with a proper timeout.
	select {
	case r := <-msgCh:
		if r.resp.MessageId != sendResp3.Msg.MessageId {
			t.Errorf("Expected message ID %s, got %s", sendResp3.Msg.MessageId, r.resp.MessageId)
		}
		if r.resp.GetUserMessage() == nil {
			t.Error("Expected user message variant")
		} else {
			if r.resp.GetUserMessage().Text != "That works for me!" {
				t.Errorf("Expected 'That works for me!', got: %s", r.resp.GetUserMessage().Text)
			}
			if r.resp.GetUserMessage().Sender.Id != lenderID {
				t.Errorf("Expected sender ID %s, got %s", lenderID, r.resp.GetUserMessage().Sender.Id)
			}
		}
	case err := <-streamDone:
		t.Fatalf("Stream closed before receiving new message: %v", err)
	case <-streamCtx.Done():
		t.Fatal("Timeout waiting for new message on stream")
	}

	streamCancel()

	// Test 6: List conversations
	listResp, err := borrowerChatClient.ListConversations(ctx, connect.NewRequest(&api.ListConversationsRequest{}))
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	// Should have 1 conversation: the gear conversation (created when gear was shared, used for all loan/giveaway discussions)
	if len(listResp.Msg.Conversations) != 1 {
		t.Errorf("Expected 1 conversation, got %d", len(listResp.Msg.Conversations))
	}

	// Find the gear conversation
	var gearConv *api.ConversationItem
	for _, conv := range listResp.Msg.Conversations {
		if conv.ConversationId == conversationID {
			gearConv = conv
			break
		}
	}

	if gearConv == nil {
		t.Errorf("Expected to find gear conversation ID %s in list", conversationID)
	} else if gearConv.CommunityId != itemCommunityID {
		// Verify the conversation reports the gear's per-item community.
		t.Errorf("Expected community ID %s in conversation list, got %s", itemCommunityID, gearConv.CommunityId)
	}

	// Test 7: Mark messages as read
	markResp, err := borrowerChatClient.MarkMessagesRead(ctx, connect.NewRequest(&api.MarkMessagesReadRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("MarkMessagesRead failed: %v", err)
	}

	// Should mark 4 messages as read (with auto-approval, no APPROVED system message):
	// - 1 system message (LOAN_SHARED from ShareGear)
	// - 1 system message (JOINED from borrower's ExpressInterest)
	// - 1 description comment seeded on behalf of the lender by ShareGear
	// - 1 user message from lender
	// - NOT borrower's own user message (already marked read when sent)
	if markResp.Msg.MessagesMarked != 4 {
		t.Errorf("Expected 4 messages marked as read, got %d", markResp.Msg.MessagesMarked)
	}

	// List conversations again - unread count should be 0 for the gear conversation
	listResp2, err := borrowerChatClient.ListConversations(ctx, connect.NewRequest(&api.ListConversationsRequest{}))
	if err != nil {
		t.Fatalf("ListConversations (after mark read) failed: %v", err)
	}

	if len(listResp2.Msg.Conversations) != 1 {
		t.Fatalf("Expected 1 conversation, got %d", len(listResp2.Msg.Conversations))
	}

	// Find the gear conversation and verify unread count is 0
	var gearConv2 *api.ConversationItem
	for _, conv := range listResp2.Msg.Conversations {
		if conv.ConversationId == conversationID {
			gearConv2 = conv
			break
		}
	}

	if gearConv2 == nil {
		t.Fatalf("Expected to find gear conversation ID %s in list", conversationID)
	}

	if gearConv2.UnreadCount != 0 {
		t.Errorf("Expected 0 unread messages after marking as read, got %d", gearConv2.UnreadCount)
	}

	// Verify community_id is still correct after all operations
	if listResp2.Msg.Conversations[0].CommunityId != itemCommunityID {
		t.Errorf("Expected community ID %s to persist, got %s", itemCommunityID, listResp2.Msg.Conversations[0].CommunityId)
	}

	// Test 9: Get conversation by transfer ID
	getConvResp, err := lenderChatClient.GetConversationForTransfer(ctx, connect.NewRequest(&api.GetConversationForTransferRequest{
		TransferId: transferID,
	}))
	if err != nil {
		t.Fatalf("GetConversationForTransfer failed: %v", err)
	}

	if getConvResp.Msg.Conversation.ConversationId != conversationID {
		t.Errorf("Expected conversation ID %s from transfer lookup, got %s", conversationID, getConvResp.Msg.Conversation.ConversationId)
	}

	if getConvResp.Msg.Conversation.CommunityId != itemCommunityID {
		t.Errorf("Expected community ID %s in conversation, got %s", itemCommunityID, getConvResp.Msg.Conversation.CommunityId)
	}
}

// TestServer_Integration_GearConversationContext tests that conversation context
// includes transfer status for both loans and giveaways.
func TestServer_Integration_GearConversationContext(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Setup: Create owner and community
	ownerToken, _ := registerFirstUser(t, serverURL, "owner@example.com", "Owner User")
	ownerGearClient := createAuthGearClient(ownerToken, serverURL)
	ownerCommunityClient := createAuthCommunityClient(ownerToken, serverURL)
	ownerChatClient := createAuthChatClient(ownerToken, serverURL)

	communityResp, err := ownerCommunityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "Testing conversation context",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := communityResp.Msg.Id

	// Register two users
	user1Token, user1ID := registerUserByInvite(t, serverURL, ownerToken, communityID, "user1@example.com", "User One")
	user2Token, _ := registerUserByInvite(t, serverURL, ownerToken, communityID, "user2@example.com", "User Two")

	user1ChatClient := createAuthChatClient(user1Token, serverURL)
	user1TransferClient := createAuthLoanClient(user1Token, serverURL)
	user2TransferClient := createAuthLoanClient(user2Token, serverURL)

	// Test 1: Giveaway conversation context with transfer status
	// (Using giveaway since loans are auto-approved and skip INTEREST_EXPRESSED state)
	t.Run("LoanTransferStatus", func(t *testing.T) {
		// Add gear for giveaway
		gearResp, err := ownerGearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Power Drill"),
			Description: proto.String("For giveaway testing"),
		}))
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}
		loanGearID := gearResp.Msg.Id

		// Share as giveaway (giveaways stay in INTEREST_EXPRESSED state)
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, loanGearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

		// The gear's conversation is bound to its per-item community (#2492), so
		// GetConversationContext reads availability from that community's share.
		// Mirror the host tapping "Give" in the Share sheet by flipping the
		// per-item share (FOR_LOAN by default) to giveaway too.
		itemCommunityID := gearResp.Msg.GetItemCommunityId()
		if itemCommunityID == "" {
			t.Fatal("Expected SaveGear to return the per-item community ID")
		}
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, loanGearID, itemCommunityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

		// User1 expresses interest
		interestResp, err := user1TransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: loanGearID,
		}))
		if err != nil {
			t.Fatalf("ExpressInterest failed: %v", err)
		}
		conversationID := interestResp.Msg.Transfer.GetConversationId()

		// Get conversation context as user1 (borrower)
		contextResp, err := user1ChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversationContext failed: %v", err)
		}

		// Verify basic context
		if contextResp.Msg.Context.TopicTitle != "Power Drill" {
			t.Errorf("Expected topic title 'Power Drill', got: %s", contextResp.Msg.Context.TopicTitle)
		}
		if contextResp.Msg.Context.Availability != api.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected AVAILABILITY_FOR_GIVEAWAY, got: %v", contextResp.Msg.Context.Availability)
		}

		// Verify transfer context exists for borrower
		if contextResp.Msg.Context.GearTransferContext == nil {
			t.Fatal("Expected GearTransferContext to be present for borrower")
		}

		// Verify borrower's transfer is included
		if contextResp.Msg.Context.GearTransferContext.UserTransfer == nil {
			t.Fatal("Expected UserTransfer to be present for borrower")
		}
		if contextResp.Msg.Context.GearTransferContext.UserTransfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected transfer state INTEREST_EXPRESSED, got: %v",
				contextResp.Msg.Context.GearTransferContext.UserTransfer.State)
		}

		// Verify available actions include withdraw interest
		foundWithdraw := false
		for _, action := range contextResp.Msg.Context.GearTransferContext.AvailableActions {
			if action == api.TransferAction_TRANSFER_ACTION_WITHDRAW_INTEREST {
				foundWithdraw = true
			}
		}
		if !foundWithdraw {
			t.Error("Expected WITHDRAW_INTEREST action to be available for borrower")
		}

		// Get conversation context as owner
		ownerContextResp, err := ownerChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("Owner GetConversationContext failed: %v", err)
		}

		// Owner should see pending requests
		if ownerContextResp.Msg.Context.GearTransferContext == nil {
			t.Fatal("Expected GearTransferContext to be present for owner")
		}
		if len(ownerContextResp.Msg.Context.GearTransferContext.PendingRequests) != 1 {
			t.Errorf("Expected 1 pending request for owner, got: %d",
				len(ownerContextResp.Msg.Context.GearTransferContext.PendingRequests))
		}

		// Verify owner can select recipient
		foundSelect := false
		for _, action := range ownerContextResp.Msg.Context.GearTransferContext.AvailableActions {
			if action == api.TransferAction_TRANSFER_ACTION_SELECT_RECIPIENT {
				foundSelect = true
			}
		}
		if !foundSelect {
			t.Error("Expected SELECT_RECIPIENT action to be available for owner")
		}
	})

	// Test 2: Giveaway conversation context with transfer status
	t.Run("GiveawayTransferStatus", func(t *testing.T) {
		// Add gear for giveaway
		gearResp, err := ownerGearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Camping Stove"),
			Description: proto.String("For giveaway testing"),
		}))
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}
		giveawayGearID := gearResp.Msg.Id

		// Share as giveaway
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, giveawayGearID, communityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

		// The gear's conversation is bound to its per-item community (#2492), so
		// GetConversationContext reads availability from that community's share.
		// Mirror the host tapping "Give" in the Share sheet by flipping the
		// per-item share (FOR_LOAN by default) to giveaway too.
		itemCommunityID := gearResp.Msg.GetItemCommunityId()
		if itemCommunityID == "" {
			t.Fatal("Expected SaveGear to return the per-item community ID")
		}
		shareGearIntoCommunity(t, ctx, ownerCommunityClient, giveawayGearID, itemCommunityID, api.Availability_AVAILABILITY_FOR_GIVEAWAY)

		// User1 and User2 both express interest
		interest1Resp, err := user1TransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: giveawayGearID,
		}))
		if err != nil {
			t.Fatalf("User1 ExpressInterest failed: %v", err)
		}
		conversationID := interest1Resp.Msg.Transfer.GetConversationId()

		_, err = user2TransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
			GearId: giveawayGearID,
		}))
		if err != nil {
			t.Fatalf("User2 ExpressInterest failed: %v", err)
		}

		// Get conversation context as user1
		contextResp, err := user1ChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("GetConversationContext failed: %v", err)
		}

		// Verify basic context
		if contextResp.Msg.Context.TopicTitle != "Camping Stove" {
			t.Errorf("Expected topic title 'Camping Stove', got: %s", contextResp.Msg.Context.TopicTitle)
		}
		if contextResp.Msg.Context.Availability != api.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected AVAILABILITY_FOR_GIVEAWAY, got: %v", contextResp.Msg.Context.Availability)
		}

		// Verify transfer context exists for giveaway participant
		if contextResp.Msg.Context.GearTransferContext == nil {
			t.Fatal("Expected GearTransferContext to be present for giveaway participant")
		}

		// Verify user1's giveaway transfer is included
		if contextResp.Msg.Context.GearTransferContext.UserTransfer == nil {
			t.Fatal("Expected UserTransfer to be present for giveaway participant")
		}
		if contextResp.Msg.Context.GearTransferContext.UserTransfer.TransferType != api.TransferType_TRANSFER_TYPE_GIVEAWAY {
			t.Errorf("Expected transfer type GIVEAWAY, got: %v",
				contextResp.Msg.Context.GearTransferContext.UserTransfer.TransferType)
		}
		if contextResp.Msg.Context.GearTransferContext.UserTransfer.State != api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			t.Errorf("Expected transfer state INTEREST_EXPRESSED, got: %v",
				contextResp.Msg.Context.GearTransferContext.UserTransfer.State)
		}

		// Get conversation context as owner
		ownerContextResp, err := ownerChatClient.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
			ConversationId: conversationID,
		}))
		if err != nil {
			t.Fatalf("Owner GetConversationContext failed: %v", err)
		}

		// Owner should see 2 pending requests (both users expressed interest)
		if ownerContextResp.Msg.Context.GearTransferContext == nil {
			t.Fatal("Expected GearTransferContext to be present for owner")
		}
		if len(ownerContextResp.Msg.Context.GearTransferContext.PendingRequests) != 2 {
			t.Errorf("Expected 2 pending giveaway requests for owner, got: %d",
				len(ownerContextResp.Msg.Context.GearTransferContext.PendingRequests))
		}

		// Verify pending requests include the correct users
		requestUserIDs := make(map[string]bool)
		for _, req := range ownerContextResp.Msg.Context.GearTransferContext.PendingRequests {
			requestUserIDs[req.Borrower.Id] = true
		}
		if !requestUserIDs[user1ID] {
			t.Error("Expected user1 to be in pending requests")
		}
	})
}
