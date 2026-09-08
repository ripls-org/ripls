package chat

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestSendMessage_ProcessesMentionsAndAddsUsers(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	member2ID := "member2"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")
	createTestUsers(t, sqlStorage, member2ID, "Bob")

	// Create community and memberships
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)
	createTestCommunityMembership(t, sqlStorage, communityID, member2ID)

	// Create a transfer with only owner and member1
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	// Start conversation (only owner and member1 are participants)
	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Verify initial participants (should be owner + member1)
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != 2 {
		t.Errorf("Expected 2 initial participants, got %d", len(conversation.ParticipantIds))
	}

	// Send message with @-mention of member2 (not a participant yet)
	messageText := "Hey @[user:" + member2ID + ":Bob], join us!"
	ctx = contextWithAuth(member1ID, "member1@test.com")
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify member2 was added as participant
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != 3 {
		t.Errorf("Expected 3 participants after mention, got %d", len(conversation.ParticipantIds))
	}

	// Verify member2 is in participants
	found := false
	for _, pid := range conversation.ParticipantIds {
		if pid == member2ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Mentioned user %s not found in participants", member2ID)
	}

	// Verify system message was created
	messages, err := sqlStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	systemMsgCount := 0
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if sysMsg, ok := chatMsg.Message.(*models.ChatMessage_SystemMessage); ok {
			if sysMsg.SystemMessage.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_ADDED_VIA_MENTION {
				systemMsgCount++
				if sysMsg.SystemMessage.Description != "Bob was added by Alice" {
					t.Errorf("Unexpected system message: %s", sysMsg.SystemMessage.Description)
				}
			}
		}
	}
	if systemMsgCount != 1 {
		t.Errorf("Expected 1 added-via-mention system message, got %d", systemMsgCount)
	}
}

func TestSendMessage_MentionIgnoresNonCommunityMembers(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	nonMemberID := "nonmember1"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")
	createTestUsers(t, sqlStorage, nonMemberID, "NonMember")

	// Create community - only owner and member1 are members
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)
	// Note: nonMemberID is NOT added to community

	// Create transfer and conversation
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Get initial participant count
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	initialCount := len(conversation.ParticipantIds)

	// Send message mentioning non-member
	messageText := "Hey @[user:" + nonMemberID + ":NonMember]!"
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify participant count unchanged (non-member not added)
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != initialCount {
		t.Errorf("Expected %d participants (unchanged), got %d", initialCount, len(conversation.ParticipantIds))
	}
}

func TestSendMessage_MentionIgnoresExistingParticipants(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")

	// Create community and memberships
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)

	// Create transfer and conversation
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Get initial participant count
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	initialCount := len(conversation.ParticipantIds)

	// Send message mentioning existing participant (member1)
	messageText := "Hey @[user:" + member1ID + ":Alice], how are you?"
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify participant count unchanged
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != initialCount {
		t.Errorf("Expected %d participants (unchanged), got %d", initialCount, len(conversation.ParticipantIds))
	}

	// Verify no system message was created for existing participant
	messages, err := sqlStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if sysMsg, ok := chatMsg.Message.(*models.ChatMessage_SystemMessage); ok {
			if sysMsg.SystemMessage.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_ADDED_VIA_MENTION {
				t.Error("Unexpected added-via-mention system message for existing participant")
			}
		}
	}
}

func TestSendMessage_MentionIgnoresSelfMention(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")

	// Create community and memberships
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)

	// Create transfer and conversation
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Get initial participant count
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	initialCount := len(conversation.ParticipantIds)

	// Send message mentioning self (owner mentions themselves)
	messageText := "I @[user:" + ownerID + ":Owner] approve this!"
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify participant count unchanged
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != initialCount {
		t.Errorf("Expected %d participants (unchanged), got %d", initialCount, len(conversation.ParticipantIds))
	}
}

func TestSendMessage_MessageWithoutMentionsWorks(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")

	// Create community and memberships
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)

	// Create transfer and conversation
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Send regular message without mentions
	messageText := "Hello everyone!"
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify message was stored correctly
	messages, err := sqlStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	found := false
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if userMsg, ok := chatMsg.Message.(*models.ChatMessage_UserMessage); ok {
			if userMsg.UserMessage.Text == messageText {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("Message not found in conversation")
	}
}

func TestSendMessage_MultipleMentionsAddsAllUsers(t *testing.T) {
	service, sqlStorage := setupTestChatService(t)

	// Create users
	ownerID := "owner1"
	member1ID := "member1"
	member2ID := "member2"
	member3ID := "member3"
	createTestUsers(t, sqlStorage, ownerID, "Owner")
	createTestUsers(t, sqlStorage, member1ID, "Alice")
	createTestUsers(t, sqlStorage, member2ID, "Bob")
	createTestUsers(t, sqlStorage, member3ID, "Carol")

	// Create community and memberships
	communityID := createTestCommunity(t, sqlStorage, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, ownerID)
	createTestCommunityMembership(t, sqlStorage, communityID, member1ID)
	createTestCommunityMembership(t, sqlStorage, communityID, member2ID)
	createTestCommunityMembership(t, sqlStorage, communityID, member3ID)

	// Create transfer and conversation with only owner and member1
	transferID := createTestLoanWithCommunity(t, sqlStorage, ownerID, member1ID, communityID)

	ctx := contextWithAuth(ownerID, "owner@test.com")
	startResp, err := service.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: transferID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Send message mentioning both member2 and member3
	messageText := "Hey @[user:" + member2ID + ":Bob] and @[user:" + member3ID + ":Carol], check this out!"
	ctx = contextWithAuth(member1ID, "member1@test.com")
	_, err = service.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           messageText,
	}))
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Verify both member2 and member3 were added
	conversation := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, conversationID, conversation); err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}
	if len(conversation.ParticipantIds) != 4 {
		t.Errorf("Expected 4 participants after mentions, got %d", len(conversation.ParticipantIds))
	}

	// Verify both users are in participants
	participantSet := make(map[string]bool)
	for _, pid := range conversation.ParticipantIds {
		participantSet[pid] = true
	}
	if !participantSet[member2ID] {
		t.Errorf("Mentioned user %s (Bob) not found in participants", member2ID)
	}
	if !participantSet[member3ID] {
		t.Errorf("Mentioned user %s (Carol) not found in participants", member3ID)
	}

	// Verify 2 system messages were created
	messages, err := sqlStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}

	systemMsgCount := 0
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if sysMsg, ok := chatMsg.Message.(*models.ChatMessage_SystemMessage); ok {
			if sysMsg.SystemMessage.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_ADDED_VIA_MENTION {
				systemMsgCount++
			}
		}
	}
	if systemMsgCount != 2 {
		t.Errorf("Expected 2 added-via-mention system messages, got %d", systemMsgCount)
	}
}
