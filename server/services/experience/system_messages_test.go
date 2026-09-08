package experience

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestServiceWithWriter creates a test service with a SystemMessageWriter attached.
func setupTestServiceWithWriter(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	t.Helper()
	service, testStorage, _ := setupTestService(t)
	writer := chat.NewSystemMessageWriter(testStorage, nil)
	service.SetSystemMessageWriter(writer)
	return service, testStorage
}

// getSystemMessagesForConversation returns all system messages in a conversation.
func getSystemMessagesForConversation(t *testing.T, testStorage *storage.ProtoSQLStorage, conversationID string) []*models.SystemChatMessage {
	t.Helper()
	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	messages, err := testStorage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("failed to query messages: %v", err)
	}
	var sysMsgs []*models.SystemChatMessage
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if sm, ok := chatMsg.Message.(*models.ChatMessage_SystemMessage); ok {
			sysMsgs = append(sysMsgs, sm.SystemMessage)
		}
	}
	return sysMsgs
}

// setupSharedExperience creates a user, experience, shares it to a community, and returns
// the experience ID and conversation ID.
func setupSharedExperience(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, ownerID string) (expID, conversationID string) {
	t.Helper()
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	createTestUser(t, testStorage, ownerID, ownerID+"@example.com", "Owner")
	communityID := createTestCommunity(t, testStorage, "Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)

	// Create experience
	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Test Experience",
		Description: "Testing system messages",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID = saveResp.Msg.Experience.Id

	// Share to community to create conversation
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Fetch the conversation ID from the experience record (canonical location).
	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	expStored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, expStored); err != nil {
		t.Fatalf("failed to find experience: %v", err)
	}
	conversationID = expStored.ConversationId
	return expID, conversationID
}

func TestProposeTime_EmitsTimeProposedSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-tp-sys"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	t.Run("first owner proposal emits TIME_PROPOSED with asking-the-group text", func(t *testing.T) {
		before := getSystemMessagesForConversation(t, testStorage, conversationID)

		_, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: 1735747200, // 2025-01-01 16:00 UTC
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		}))
		if err != nil {
			t.Fatalf("ProposeTime failed: %v", err)
		}

		after := getSystemMessagesForConversation(t, testStorage, conversationID)
		newMsgs := after[len(before):]
		if len(newMsgs) == 0 {
			t.Fatal("expected a new system message after first ProposeTime, got none")
		}

		found := false
		for _, sm := range newMsgs {
			if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED {
				found = true
				if sm.GetActorId() != ownerID {
					t.Errorf("actor_id = %q, want %q", sm.GetActorId(), ownerID)
				}
				if !strings.Contains(sm.Description, "asking the group") {
					t.Errorf("description %q does not contain 'asking the group'", sm.Description)
				}
				if !strings.Contains(sm.Description, "Owner") {
					t.Errorf("description %q does not contain actor name 'Owner'", sm.Description)
				}
			}
		}
		if !found {
			t.Errorf("no TIME_PROPOSED system message found; got actions: %v", actionList(newMsgs))
		}
	})

	t.Run("second owner proposal (same poll) emits no additional system message", func(t *testing.T) {
		before := getSystemMessagesForConversation(t, testStorage, conversationID)

		_, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: 1735833600,
						Timezone:         "UTC",
						DurationMinutes:  90,
					},
				},
			},
		}))
		if err != nil {
			t.Fatalf("ProposeTime (second option) failed: %v", err)
		}

		after := getSystemMessagesForConversation(t, testStorage, conversationID)
		newMsgs := after[len(before):]

		for _, sm := range newMsgs {
			if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED {
				t.Errorf("unexpected TIME_PROPOSED message for second proposal: %q", sm.Description)
			}
		}
	})
}

// TestProposeTime_SecondPollEmitsNewSystemMessage verifies that closing a poll
// and creating a second one produces a second TIME_PROPOSED system message with
// a distinct poll_id, rather than silently reusing the first message.
func TestProposeTime_SecondPollEmitsNewSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-second-poll"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	proposeAt := func(ts int64) {
		t.Helper()
		_, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: ts,
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		}))
		if err != nil {
			t.Fatalf("ProposeTime failed: %v", err)
		}
	}

	// === Poll 1 ===
	proposeAt(1735747200)

	msgsAfterPoll1 := getSystemMessagesForConversation(t, testStorage, conversationID)
	var poll1Msgs []*models.SystemChatMessage
	for _, sm := range msgsAfterPoll1 {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED {
			poll1Msgs = append(poll1Msgs, sm)
		}
	}
	if len(poll1Msgs) != 1 {
		t.Fatalf("want 1 TIME_PROPOSED message after poll 1, got %d", len(poll1Msgs))
	}
	firstPollID := poll1Msgs[0].GetPollId()
	if firstPollID == "" {
		t.Error("first TIME_PROPOSED message should have a non-empty poll_id")
	}

	// Close poll 1.
	_, err := service.CancelTimePoll(ownerCtx, connect.NewRequest(&api.CancelTimePollRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CancelTimePoll failed: %v", err)
	}

	// === Poll 2 ===
	proposeAt(1735920000)

	msgsAfterPoll2 := getSystemMessagesForConversation(t, testStorage, conversationID)
	var poll2Msgs []*models.SystemChatMessage
	for _, sm := range msgsAfterPoll2 {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED {
			poll2Msgs = append(poll2Msgs, sm)
		}
	}

	if len(poll2Msgs) != 2 {
		t.Fatalf("want 2 TIME_PROPOSED messages after poll 2, got %d", len(poll2Msgs))
	}

	secondPollID := ""
	for _, sm := range poll2Msgs {
		if sm.GetPollId() != firstPollID {
			secondPollID = sm.GetPollId()
		}
	}
	if secondPollID == "" {
		t.Error("second TIME_PROPOSED message should have a different poll_id than the first")
	}

	// Experience should reflect the new poll as active.
	ctx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	expStored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, expStored); err != nil {
		t.Fatalf("failed to read experience: %v", err)
	}
	if !expStored.TimePollActive {
		t.Error("time_poll_active should be true after starting second poll")
	}
	if expStored.GetCurrentPollId() != secondPollID {
		t.Errorf("current_poll_id = %q, want second poll id %q", expStored.GetCurrentPollId(), secondPollID)
	}
}

func TestConfirmTime_EmitsDetailChangedSystemMessage(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-confirm-sys"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	// Propose a time first so we have something to confirm.
	propResp, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
		ExperienceId: expID,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735747200, // 2025-01-01 16:00 UTC
					Timezone:         "UTC",
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("ProposeTime failed: %v", err)
	}

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	_, err = service.ConfirmTime(ownerCtx, connect.NewRequest(&api.ConfirmTimeRequest{
		ExperienceId: expID,
		ProposalId:   propResp.Msg.Proposal.Id,
	}))
	if err != nil {
		t.Fatalf("ConfirmTime failed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED {
			found = true
			if sm.GetActorId() != ownerID {
				t.Errorf("actor_id = %q, want %q", sm.GetActorId(), ownerID)
			}
			if !strings.Contains(sm.Description, "confirmed the time") {
				t.Errorf("description %q does not contain 'confirmed the time'", sm.Description)
			}
			if !strings.Contains(sm.Description, "Owner") {
				t.Errorf("description %q does not contain actor name 'Owner'", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no DETAIL_CHANGED system message found for ConfirmTime; got actions: %v", actionList(newMsgs))
	}
}

func TestSaveExperience_EmitsDetailChangedOnTimeUpdate(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-dc-time"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	id := expID
	_, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:   &id,
		Name: "Test Experience", // unchanged
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1735747200,
					Timezone:         "UTC",
					DurationMinutes:  120,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience update failed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED {
			found = true
			if sm.GetActorId() != ownerID {
				t.Errorf("actor_id = %q, want %q", sm.GetActorId(), ownerID)
			}
			if !strings.Contains(sm.Description, "updated time") {
				t.Errorf("description %q does not contain 'updated time'", sm.Description)
			}
			if !strings.Contains(sm.Description, "→") {
				t.Errorf("description %q does not contain '→'", sm.Description)
			}
			if !strings.Contains(sm.Description, "Owner") {
				t.Errorf("description %q does not contain actor name 'Owner'", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no DETAIL_CHANGED system message found; got actions: %v", actionList(newMsgs))
	}
}

func TestSaveExperience_EmitsDetailChangedOnLocationUpdate(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-dc-loc"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	// Insert a location with a name
	locCtx := createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN)
	loc := &models.Location{
		Name: proto.String("Golden Gate Park"),
	}
	locID, err := testStorage.Insert(locCtx, loc)
	if err != nil {
		t.Fatalf("failed to insert location: %v", err)
	}

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	id := expID
	_, err = service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:         &id,
		LocationId: locID,
	}))
	if err != nil {
		t.Fatalf("SaveExperience location update failed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	found := false
	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED {
			found = true
			if !strings.Contains(sm.Description, "updated location") {
				t.Errorf("description %q does not contain 'updated location'", sm.Description)
			}
			if !strings.Contains(sm.Description, "Golden Gate Park") {
				t.Errorf("description %q does not contain location name 'Golden Gate Park'", sm.Description)
			}
		}
	}
	if !found {
		t.Errorf("no DETAIL_CHANGED system message found; got actions: %v", actionList(newMsgs))
	}
}

func TestSaveExperience_NoSystemMessageOnNameOnlyUpdate(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-dc-nochange"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	before := getSystemMessagesForConversation(t, testStorage, conversationID)

	id := expID
	_, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:          &id,
		Name:        "Renamed Experience",
		Description: "Updated description only",
	}))
	if err != nil {
		t.Fatalf("SaveExperience update failed: %v", err)
	}

	after := getSystemMessagesForConversation(t, testStorage, conversationID)
	newMsgs := after[len(before):]

	for _, sm := range newMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED {
			t.Errorf("unexpected DETAIL_CHANGED system message for name-only update: %q", sm.Description)
		}
	}
}

func TestExperienceTimeParam(t *testing.T) {
	tests := []struct {
		name            string
		input           *models.ExperienceTime
		wantTBD         bool
		wantDescription string // the organizer's own words, or "" for a concrete instant
		wantUnixSec     int64
		wantOffsetMin   int
	}{
		{
			name:    "nil time is TBD",
			input:   nil,
			wantTBD: true,
		},
		{
			name: "TBD type is TBD",
			input: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
			},
			wantTBD: true,
		},
		{
			// No rendered timestamp: the client formats the instant in the
			// viewer's locale (#2844).
			name: "specific time carries the instant and offset, no rendered text",
			input: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Specific{
					Specific: &models.SpecificTime{
						UnixTimestampSec: 1735747200, // Wed, Jan 1 2025 16:00:00 UTC
						Timezone:         "UTC",
					},
				},
			},
			wantUnixSec: 1735747200,
		},
		{
			name: "specific time uses the event timezone's offset",
			input: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Specific{
					Specific: &models.SpecificTime{
						UnixTimestampSec: 1735747200, // Jan 1 2025 — MST (UTC-7)
						Timezone:         "America/Denver",
					},
				},
			},
			wantUnixSec:   1735747200,
			wantOffsetMin: -420,
		},
		{
			name: "range with description passes the user's text through",
			input: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Range{
					Range: &models.TimeRange{Description: "Late March"},
				},
			},
			wantDescription: "Late March",
		},
		{
			name: "range without description is TBD",
			input: &models.ExperienceTime{
				TimeType: &models.ExperienceTime_Range{
					Range: &models.TimeRange{},
				},
			},
			wantTBD: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := experienceTimeParam(tt.input)
			if got.TBD != tt.wantTBD {
				t.Errorf("TBD = %v, want %v", got.TBD, tt.wantTBD)
			}
			// Description carries the organizer's own words and nothing else —
			// a concrete instant leaves it empty rather than filling it with a
			// server-rendered English timestamp (#2844).
			if got.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDescription)
			}
			if got.UnixSec != tt.wantUnixSec {
				t.Errorf("UnixSec = %d, want %d", got.UnixSec, tt.wantUnixSec)
			}
			if got.UTCOffsetMin != tt.wantOffsetMin {
				t.Errorf("UTCOffsetMin = %d, want %d", got.UTCOffsetMin, tt.wantOffsetMin)
			}
		})
	}
}

// actionList returns a readable list of actions for test error messages.
func actionList(msgs []*models.SystemChatMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Action.String()
	}
	return out
}

// TestConfirmTime_QueryCount verifies ConfirmTime does not regress to N+1 queries.
func TestConfirmTime_QueryCount(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-qc"
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	expID, _ := setupSharedExperience(t, service, testStorage, ownerID)

	// Create multiple proposals so the batch unconfirm path is exercised.
	var proposalIDs []string
	for i := 0; i < 5; i++ {
		resp, err := service.ProposeTime(ownerCtx, connect.NewRequest(&api.ProposeTimeRequest{
			ExperienceId: expID,
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: int64(1735747200 + i*3600),
						Timezone:         "UTC",
						DurationMinutes:  60,
					},
				},
			},
		}))
		if err != nil {
			t.Fatalf("ProposeTime failed: %v", err)
		}
		proposalIDs = append(proposalIDs, resp.Msg.Proposal.Id)
	}

	// Enable query counting and confirm time.
	// Budget: GetByID(experience) + GetByID(proposal) + Update(proposal) + Update(experience)
	//         + QueryByField(proposals) + 0 unconfirm updates (none are confirmed yet)
	//         + QueryByField(community_experiences) + InsertSystemMessage (~3 queries)
	// Total should be well under 15.
	statsCtx := storage.WithQueryStats(ownerCtx)
	storage.AssertMaxQueries(t, statsCtx, 15, func() {
		_, err := service.ConfirmTime(statsCtx, connect.NewRequest(&api.ConfirmTimeRequest{
			ExperienceId: expID,
			ProposalId:   proposalIDs[0],
		}))
		if err != nil {
			t.Fatalf("ConfirmTime failed: %v", err)
		}
	})
}

// TestRSVP_SystemMessage_SameIntentionNoDuplicate verifies that re-submitting
// the same RSVP intention does not produce duplicate system messages.
func TestRSVP_SystemMessage_SameIntentionNoDuplicate(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-rsvp-dedup"
	rsvperID := "rsvper-dedup"

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	// Add the RSVPer as a community member.
	createTestUser(t, testStorage, rsvperID, rsvperID+"@example.com", "RSVP User")
	commMembers, err := testStorage.QueryByField(
		createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN),
		"experience_id",
		expID,
		&models.CommunityExperience{},
	)
	if err != nil || len(commMembers) == 0 {
		t.Fatalf("expected community_experience record: %v", err)
	}
	communityID := commMembers[0].(*models.CommunityExperience).CommunityId
	createTestCommunityMembership(t, testStorage, communityID, rsvperID)

	rsvperCtx := createAuthenticatedContext(rsvperID, rsvperID+"@example.com", models.Role_ROLE_USER)

	rsvpYes := func() {
		t.Helper()
		_, err := service.RSVPToExperience(rsvperCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
	}

	// First RSVP — should produce one RSVP_YES message.
	rsvpYes()
	msgs := getSystemMessagesForConversation(t, testStorage, conversationID)
	rsvpMsgs := filterRSVPSystemMessages(msgs)
	if len(rsvpMsgs) != 1 {
		t.Errorf("after first RSVP: want 1 system message, got %d", len(rsvpMsgs))
	}

	// Re-submit the same intention twice more — must not add duplicate messages.
	rsvpYes()
	rsvpYes()
	msgs = getSystemMessagesForConversation(t, testStorage, conversationID)
	rsvpMsgs = filterRSVPSystemMessages(msgs)
	if len(rsvpMsgs) != 1 {
		t.Errorf("after re-submitting same intention: want 1 system message, got %d", len(rsvpMsgs))
	}
}

// TestRSVP_SystemMessage_IntentionChangeCoalesces verifies that rapid RSVP changes
// (Yes → No → Yes) coalesce into a single card showing the final state.
func TestRSVP_SystemMessage_IntentionChangeCoalesces(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "owner-rsvp-change"
	rsvperID := "rsvper-change"

	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)

	createTestUser(t, testStorage, rsvperID, rsvperID+"@example.com", "RSVP User")
	commMembers, err := testStorage.QueryByField(
		createAuthenticatedContext("system", "", models.Role_ROLE_ADMIN),
		"experience_id",
		expID,
		&models.CommunityExperience{},
	)
	if err != nil || len(commMembers) == 0 {
		t.Fatalf("expected community_experience record: %v", err)
	}
	communityID := commMembers[0].(*models.CommunityExperience).CommunityId
	createTestCommunityMembership(t, testStorage, communityID, rsvperID)

	rsvperCtx := createAuthenticatedContext(rsvperID, rsvperID+"@example.com", models.Role_ROLE_USER)

	rsvp := func(intention api.RSVPIntention) {
		t.Helper()
		_, err := service.RSVPToExperience(rsvperCtx, connect.NewRequest(&api.RSVPToExperienceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Intention:    intention,
		}))
		if err != nil {
			t.Fatalf("RSVPToExperience failed: %v", err)
		}
	}

	rsvp(api.RSVPIntention_RSVP_INTENTION_YES)
	rsvp(api.RSVPIntention_RSVP_INTENTION_NO)
	rsvp(api.RSVPIntention_RSVP_INTENTION_YES)

	msgs := getSystemMessagesForConversation(t, testStorage, conversationID)
	rsvpMsgs := filterRSVPSystemMessages(msgs)

	// All three rapid changes collapse into a single card showing the final state.
	if len(rsvpMsgs) != 1 {
		t.Errorf("after Yes→No→Yes: want 1 coalesced system message, got %d", len(rsvpMsgs))
	}
	if len(rsvpMsgs) == 1 && rsvpMsgs[0].Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES {
		t.Errorf("coalesced card action = %v, want RSVP_YES", rsvpMsgs[0].Action)
	}
}

// filterRSVPSystemMessages returns only the RSVP-related system messages.
func filterRSVPSystemMessages(msgs []*models.SystemChatMessage) []*models.SystemChatMessage {
	var out []*models.SystemChatMessage
	for _, m := range msgs {
		switch m.Action {
		case models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_NO:
			out = append(out, m)
		}
	}
	return out
}
