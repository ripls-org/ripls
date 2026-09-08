package experience

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// getUserMessagesForConversation returns all user chat messages in a conversation.
func getUserMessagesForConversation(t *testing.T, testStorage *storage.ProtoSQLStorage, conversationID string) []*models.UserChatMessage {
	t.Helper()
	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	messages, err := testStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("failed to query messages: %v", err)
	}
	var userMsgs []*models.UserChatMessage
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if um, ok := chatMsg.Message.(*models.ChatMessage_UserMessage); ok {
			userMsgs = append(userMsgs, um.UserMessage)
		}
	}
	return userMsgs
}

// TestAddExperienceNeed_WithNote_EmitsUserMessage verifies that when a note is provided
// the note is posted as a user message (not a system message) so participants can react.
func TestAddExperienceNeed_WithNote_EmitsUserMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-need-note-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	beforeUser := getUserMessagesForConversation(t, testStorage, conversationID)

	note := "A 2019 Tempranillo, ideally"
	resp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Red wine",
		Note:         &note,
		Slots:        2,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := resp.Msg.Need.Id

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	afterUser := getUserMessagesForConversation(t, testStorage, conversationID)

	// No new system message should be emitted when a note is present.
	for _, sm := range afterSys[len(beforeSys):] {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED {
			t.Errorf("unexpected PLANNING_NEED_ADDED system message when note is present: %q", sm.Description)
		}
	}

	// A user message carrying the note and the need ID should be present.
	newUserMsgs := afterUser[len(beforeUser):]
	found := false
	for _, um := range newUserMsgs {
		if um.GetNeedId() == needID {
			found = true
			if um.SenderId != ownerID {
				t.Errorf("sender_id = %q, want %q", um.SenderId, ownerID)
			}
			if !strings.Contains(um.Text, "Tempranillo") {
				t.Errorf("text %q missing note content", um.Text)
			}
		}
	}
	if !found {
		t.Errorf("no user message with experience_need_id=%q found", needID)
	}
}

// TestAddExperienceNeed_NoNote_EmitsSystemMessage verifies that when no note is provided
// the bare system message is still emitted.
func TestAddExperienceNeed_NoNote_EmitsSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-need-nosys"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	_, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Chips",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED {
			found = true
			if sm.GetActorId() != ownerID {
				t.Errorf("actor_id = %q, want %q", sm.GetActorId(), ownerID)
			}
			if !strings.Contains(sm.Description, "Chips") {
				t.Errorf("description %q missing need name", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no PLANNING_NEED_ADDED system message; got: %v", actionList(newMsgs))
	}
}

// TestRemoveExperienceNeed_EmitsSystemMessage verifies that a quick add→remove
// coalesces into a single PLANNING_NEED_REMOVED card (no duplicate insert).
func TestRemoveExperienceNeed_EmitsSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-need-remove-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Chips",
		Slots:        1,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}

	beforeCount := len(getSystemMessagesForConversation(t, testStorage, conversationID))

	_, err = service.RemoveExperienceNeed(ownerCtx, connect.NewRequest(&api.RemoveExperienceNeedRequest{
		NeedId:       needResp.Msg.Need.Id,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("RemoveExperienceNeed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)

	// Coalescing: NEED_ADDED updated in place → no new row inserted.
	if len(after) != beforeCount {
		t.Errorf("expected %d system messages (coalesced, no new insert), got %d", beforeCount, len(after))
	}

	found := false
	for _, sm := range after {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_REMOVED {
			found = true
			if !strings.Contains(sm.Description, "Chips") {
				t.Errorf("description %q missing need name", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no PLANNING_NEED_REMOVED system message; got: %v", actionList(after))
	}
}

func TestClaimExperienceNeed_WithNote_EmitsUserMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-claim-note-msg"
	participantID := "participant-claim-note-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, participantID, participantID+"@example.com", "Gary")
	adminCtx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	commExps, err := testStorage.QueryByField(adminCtx, "experience_id", expID, &models.CommunityExperience{})
	if err != nil || len(commExps) == 0 {
		t.Fatalf("no community_experience: %v", err)
	}
	communityID := commExps[0].(*models.CommunityExperience).CommunityId
	createTestCommunityMembership(t, testStorage, communityID, participantID)

	participantCtx := createAuthenticatedContext(participantID, participantID+"@example.com", models.Role_ROLE_USER)
	_, err = service.RSVPToExperience(participantCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Dessert",
		Slots:        2,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}
	needID := needResp.Msg.Need.Id

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	beforeUser := getUserMessagesForConversation(t, testStorage, conversationID)

	claimNote := "Tiramisu"
	_, err = service.ClaimExperienceNeed(participantCtx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needID,
		ExperienceId: expID,
		Note:         &claimNote,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	afterUser := getUserMessagesForConversation(t, testStorage, conversationID)

	// No new PLANNING_NEED_CLAIMED system message when a note is present.
	for _, sm := range afterSys[len(beforeSys):] {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED {
			t.Errorf("unexpected PLANNING_NEED_CLAIMED system message when note is present: %q", sm.Description)
		}
	}

	// A user message carrying the note and the contribution ID should be present.
	newUserMsgs := afterUser[len(beforeUser):]
	found := false
	for _, um := range newUserMsgs {
		if um.ContributionId != nil {
			found = true
			if um.SenderId != participantID {
				t.Errorf("sender_id = %q, want %q", um.SenderId, participantID)
			}
			if !strings.Contains(um.Text, "Tiramisu") {
				t.Errorf("text %q missing note content", um.Text)
			}
		}
	}
	if !found {
		t.Errorf("no user message with experience_contribution_id found after claim with note")
	}
}

func TestClaimExperienceNeed_NoNote_EmitsSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-claim-nosys"
	participantID := "participant-claim-nosys"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, participantID, participantID+"@example.com", "Gary")
	adminCtx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	commExps, err := testStorage.QueryByField(adminCtx, "experience_id", expID, &models.CommunityExperience{})
	if err != nil || len(commExps) == 0 {
		t.Fatalf("no community_experience: %v", err)
	}
	communityID := commExps[0].(*models.CommunityExperience).CommunityId
	createTestCommunityMembership(t, testStorage, communityID, participantID)

	participantCtx := createAuthenticatedContext(participantID, participantID+"@example.com", models.Role_ROLE_USER)
	_, err = service.RSVPToExperience(participantCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	}))
	if err != nil {
		t.Fatalf("RSVPToExperience: %v", err)
	}

	needResp, err := service.AddExperienceNeed(ownerCtx, connect.NewRequest(&api.AddExperienceNeedRequest{
		ExperienceId: expID,
		Name:         "Dessert",
		Slots:        2,
	}))
	if err != nil {
		t.Fatalf("AddExperienceNeed: %v", err)
	}

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	_, err = service.ClaimExperienceNeed(participantCtx, connect.NewRequest(&api.ClaimExperienceNeedRequest{
		NeedId:       needResp.Msg.Need.Id,
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ClaimExperienceNeed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED {
			found = true
			if sm.GetActorId() != participantID {
				t.Errorf("actor_id = %q, want %q", sm.GetActorId(), participantID)
			}
			if !strings.Contains(sm.Description, "Dessert") {
				t.Errorf("description %q missing need name", sm.Description)
			}
			if !strings.Contains(sm.Description, "Gary") {
				t.Errorf("description %q missing actor name", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no PLANNING_NEED_CLAIMED system message; got: %v", actionList(newMsgs))
	}
}

func TestAddExperienceContribution_WithDescription_EmitsUserMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-contrib-desc-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	beforeUser := getUserMessagesForConversation(t, testStorage, conversationID)

	desc := "A 2019 Tempranillo, great with lamb"
	resp, err := service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Red wine",
		Description:  &desc,
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}
	contribID := resp.Msg.Contribution.Id

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	afterUser := getUserMessagesForConversation(t, testStorage, conversationID)

	// No new PLANNING_CONTRIBUTION_ADDED system message when description is present.
	for _, sm := range afterSys[len(beforeSys):] {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED {
			t.Errorf("unexpected PLANNING_CONTRIBUTION_ADDED system message when description is present: %q", sm.Description)
		}
	}

	// A user message carrying the description and contribution ID should be present.
	newUserMsgs := afterUser[len(beforeUser):]
	found := false
	for _, um := range newUserMsgs {
		if um.GetContributionId() == contribID {
			found = true
			if um.SenderId != ownerID {
				t.Errorf("sender_id = %q, want %q", um.SenderId, ownerID)
			}
			if !strings.Contains(um.Text, "Tempranillo") {
				t.Errorf("text %q missing description content", um.Text)
			}
		}
	}
	if !found {
		t.Errorf("no user message with experience_contribution_id=%q found", contribID)
	}
}

func TestAddExperienceContribution_NoDescription_EmitsSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-contrib-nodesc-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	_, err := service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Red wine",
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED {
			found = true
			if !strings.Contains(sm.Description, "Red wine") {
				t.Errorf("description %q missing title", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no PLANNING_CONTRIBUTION_ADDED system message; got: %v", actionList(newMsgs))
	}
}

// TestRemoveExperienceContribution_EmitsSystemMessage verifies that a quick add→remove
// coalesces into a single PLANNING_CONTRIBUTION_REMOVED card (no duplicate insert).
func TestRemoveExperienceContribution_EmitsSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-contrib-remove-msg"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	contribResp, err := service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Salad",
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}

	beforeCount := len(getSystemMessagesForConversation(t, testStorage, conversationID))

	_, err = service.RemoveExperienceContribution(ownerCtx, connect.NewRequest(&api.RemoveExperienceContributionRequest{
		ContributionId: contribResp.Msg.Contribution.Id,
		ExperienceId:   expID,
	}))
	if err != nil {
		t.Fatalf("RemoveExperienceContribution: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)

	// Coalescing: CONTRIBUTION_ADDED updated in place → no new row inserted.
	if len(after) != beforeCount {
		t.Errorf("expected %d system messages (coalesced, no new insert), got %d", beforeCount, len(after))
	}

	found := false
	for _, sm := range after {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_REMOVED {
			found = true
			if !strings.Contains(sm.Description, "Salad") {
				t.Errorf("description %q missing title", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no PLANNING_CONTRIBUTION_REMOVED system message; got: %v", actionList(after))
	}
}

func TestExperienceNeedAddedText_NoteSnippet(t *testing.T) {
	note := "A 2019 Tempranillo, great with the lamb dish"
	got := chat.ExperienceNeedAddedText("Gary", "Red wine", &note)
	if !strings.Contains(got, "Red wine") {
		t.Errorf("missing need name: %q", got)
	}
	if !strings.Contains(got, "Tempranillo") {
		t.Errorf("missing note snippet: %q", got)
	}
}

func TestExperienceNeedAddedText_NoNote(t *testing.T) {
	got := chat.ExperienceNeedAddedText("Gary", "Dessert", nil)
	if strings.Contains(got, "—") {
		t.Errorf("should have no em dash when note is nil: %q", got)
	}
}
