package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// TestGetConversation_GearCommunityMemberAccess verifies that any community member
// can access gear conversations in their community (even if not a participant).
func TestGetConversation_GearCommunityMemberAccess(t *testing.T) {
	svc, storage := setupTestChatService(t)

	// Create users
	ownerID := "owner-user"
	participantID := "participant-user"
	communityMemberID := "community-member-user"
	createTestUsers(t, storage, ownerID, "Owner User")
	createTestUsers(t, storage, participantID, "Participant User")
	createTestUsers(t, storage, communityMemberID, "Community Member")

	// Create community
	communityID := createTestCommunity(t, storage, ownerID)

	// Add all users to the community
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, participantID, communityID)
	addUserToCommunity(t, storage, communityMemberID, communityID)

	// Create gear
	gear := &models.Gear{
		OwnerId:  ownerID,
		Name:     "Test Gear",
		MediaIds: []string{"media123"},
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}

	// Create CommunityGear (gear shared in community)
	communityGear := &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	_, err = storage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	// Create a gear conversation with owner and participant
	conversation := &models.ChatConversation{
		CommunityId: communityID,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{
			GearId: gearID,
		}},
		ParticipantIds: []string{ownerID, participantID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test 1: Participant can access
	ctx := contextWithAuth(participantID, "participant@example.com")
	req := connect.NewRequest(&api.GetConversationRequest{
		ConversationId: conversationID,
	})
	resp, err := svc.GetConversation(ctx, req)
	if err != nil {
		t.Fatalf("Participant should be able to access conversation: %v", err)
	}
	if resp.Msg.Conversation.ConversationId != conversationID {
		t.Errorf("Expected conversation_id '%s', got '%s'", conversationID, resp.Msg.Conversation.ConversationId)
	}
	// Verify topic is set
	if resp.Msg.Conversation.GetTopic().GetGearId() != gearID {
		t.Errorf("Expected gear_id '%s', got '%s'", gearID, resp.Msg.Conversation.GetTopic().GetGearId())
	}

	// Test 2: Community member (not a participant) can also access and becomes a participant
	ctx = contextWithAuth(communityMemberID, "member@example.com")
	req = connect.NewRequest(&api.GetConversationRequest{
		ConversationId: conversationID,
	})
	resp, err = svc.GetConversation(ctx, req)
	if err != nil {
		t.Fatalf("Community member should be able to access conversation: %v", err)
	}
	if resp.Msg.Conversation.ConversationId != conversationID {
		t.Errorf("Expected conversation_id '%s', got '%s'", conversationID, resp.Msg.Conversation.ConversationId)
	}

	// Verify the community member was added as a participant
	updatedConversation := &models.ChatConversation{}
	if err := storage.GetByID(context.Background(), conversationID, updatedConversation); err != nil {
		t.Fatalf("Failed to get updated conversation: %v", err)
	}
	found := false
	for _, participantID := range updatedConversation.ParticipantIds {
		if participantID == communityMemberID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Community member should have been added as a participant")
	}
}

// TestGetConversation_GearMultiCommunityMemberAccess verifies that when a gear
// is shared with multiple communities, members of any sharing community can
// access the gear conversation — not only members of conversation.CommunityId
// (the first community of the share). Regression test for #1675.
func TestGetConversation_GearMultiCommunityMemberAccess(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner-user"
	secondaryMemberID := "secondary-community-member"
	createTestUsers(t, store, ownerID, "Owner User")
	createTestUsers(t, store, secondaryMemberID, "Secondary Member")

	// Two communities. The gear is shared with both. The secondary member is
	// only in communityB.
	communityA := createTestCommunity(t, store, ownerID)
	communityB := createTestCommunity(t, store, ownerID)
	addUserToCommunity(t, store, ownerID, communityA)
	addUserToCommunity(t, store, ownerID, communityB)
	addUserToCommunity(t, store, secondaryMemberID, communityB)

	gear := &models.Gear{OwnerId: ownerID, Name: "Multi-shared Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	for _, cid := range []string{communityA, communityB} {
		_, err := store.Insert(context.Background(), &models.CommunityGear{
			GearId:       gearID,
			CommunityId:  cid,
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		})
		if err != nil {
			t.Fatalf("Failed to create CommunityGear for %s: %v", cid, err)
		}
	}

	// Conversation's CommunityId is communityA — the first sharing community.
	// Secondary member is NOT in communityA.
	conversation := &models.ChatConversation{
		CommunityId: communityA,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{
			GearId: gearID,
		}},
		ParticipantIds: []string{ownerID},
	}
	conversationID, err := store.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test 1: GetConversation must succeed for the secondary-community member.
	ctx := contextWithAuth(secondaryMemberID, "secondary@example.com")
	resp, err := svc.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("Secondary-community member should access gear conversation, got: %v", err)
	}
	if resp.Msg.Conversation.ConversationId != conversationID {
		t.Errorf("Expected conversation_id '%s', got '%s'", conversationID, resp.Msg.Conversation.ConversationId)
	}

	// Verify the secondary member was added as a participant.
	updated := &models.ChatConversation{}
	if err := store.GetByID(context.Background(), conversationID, updated); err != nil {
		t.Fatalf("Failed to reload conversation: %v", err)
	}
	found := false
	for _, pid := range updated.ParticipantIds {
		if pid == secondaryMemberID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Secondary-community member should have been added as a participant")
	}

	// Test 2: GetConversationContext must also succeed for the secondary member.
	// (It's the second RPC the client calls in parallel; both must agree.)
	ctxResp, err := svc.GetConversationContext(ctx, connect.NewRequest(&api.GetConversationContextRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("GetConversationContext should succeed for secondary-community member, got: %v", err)
	}
	if ctxResp.Msg.Context.ConversationId != conversationID {
		t.Errorf("Expected context conversation_id '%s', got '%s'", conversationID, ctxResp.Msg.Context.ConversationId)
	}

	// Test 3: A second GetConversation must not regress access — the participant
	// just added must not be evicted by validateAndCleanParticipants on the next
	// call (the gear-topic early-return guards this).
	if _, err := svc.GetConversation(ctx, connect.NewRequest(&api.GetConversationRequest{
		ConversationId: conversationID,
	})); err != nil {
		t.Fatalf("Second GetConversation must not re-strip secondary-community member, got: %v", err)
	}
	updatedAgain := &models.ChatConversation{}
	if err := store.GetByID(context.Background(), conversationID, updatedAgain); err != nil {
		t.Fatalf("Failed to reload conversation after revisit: %v", err)
	}
	stillFound := false
	for _, pid := range updatedAgain.ParticipantIds {
		if pid == secondaryMemberID {
			stillFound = true
			break
		}
	}
	if !stillFound {
		t.Error("Secondary-community member must remain a participant after revisit")
	}
}

// ── StartConversation gear-topic tests ──────────────────────────────────────.

// captureGearTestLogger returns a buf + context that captures slog output for
// assertions in gear StartConversation tests.
func captureGearTestLogger(baseCtx context.Context) (*bytes.Buffer, context.Context) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	base := slog.New(handler)
	logger := &logging.Logger{Logger: base}
	return buf, logging.WithLogger(baseCtx, logger)
}

// decodeGearTestRecords parses one JSON log line per entry.
func decodeGearTestRecords(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// TestStartConversation_GearTopic_OwnerSucceeds verifies that the gear owner can
// start a conversation via StartConversation with a gear_id topic.
func TestStartConversation_GearTopic_OwnerSucceeds(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner1"
	createTestUsers(t, store, ownerID, "Owner")

	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)

	// Create gear owned by owner1.
	gear := &models.Gear{OwnerId: ownerID, Name: "Test Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}

	// Share gear with the community.
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	ctx := contextWithAuth(ownerID, "owner@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("owner should be able to start gear conversation: %v", err)
	}
	if resp.Msg.ConversationId == "" {
		t.Error("expected non-empty conversation_id")
	}

	// Verify conversation_id was persisted on the gear.
	updated := &models.Gear{}
	if err := store.GetByID(context.Background(), gearID, updated); err != nil {
		t.Fatalf("failed to reload gear: %v", err)
	}
	if updated.ConversationId != resp.Msg.ConversationId {
		t.Errorf("expected gear.conversation_id=%q, got %q",
			resp.Msg.ConversationId, updated.ConversationId)
	}
}

// TestStartConversation_GearTopic_CommunityMemberSucceeds verifies that a
// community member who is not the owner can start (or retrieve) the gear chat.
func TestStartConversation_GearTopic_CommunityMemberSucceeds(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "gear-owner"
	memberID := "community-member"
	createTestUsers(t, store, ownerID, "Owner")
	createTestUsers(t, store, memberID, "Member")

	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)
	createTestCommunityMembership(t, store, communityID, memberID)

	gear := &models.Gear{OwnerId: ownerID, Name: "Shared Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	ctx := contextWithAuth(memberID, "member@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("community member should be able to start gear conversation: %v", err)
	}
	if resp.Msg.ConversationId == "" {
		t.Error("expected non-empty conversation_id")
	}

	// Member should have been added as a participant (AddParticipantToConversation
	// is called via finalizeGearConversation → owner; member enters via GetConversation).
	// For StartConversation the owner is added; member access is via GetConversation.
	// Verify gear row was persisted.
	updated := &models.Gear{}
	if err := store.GetByID(context.Background(), gearID, updated); err != nil {
		t.Fatalf("failed to reload gear: %v", err)
	}
	if updated.ConversationId == "" {
		t.Error("conversation_id should have been persisted on gear")
	}
}

// TestStartConversation_GearTopic_NonMemberDenied verifies that a user who is
// not a member of any community the gear is shared with is rejected.
func TestStartConversation_GearTopic_NonMemberDenied(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "gear-owner"
	outsiderID := "outsider"
	createTestUsers(t, store, ownerID, "Owner")
	createTestUsers(t, store, outsiderID, "Outsider")

	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)
	// outsider is NOT a member of the community.

	gear := &models.Gear{OwnerId: ownerID, Name: "Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:      gearID,
		CommunityId: communityID,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	ctx := contextWithAuth(outsiderID, "outsider@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	_, err = svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("expected error for non-member, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
}

// TestStartConversation_GearTopic_UnsharedGear verifies that StartConversation
// fails with CodeFailedPrecondition when the gear has no CommunityGear rows.
func TestStartConversation_GearTopic_UnsharedGear(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner"
	createTestUsers(t, store, ownerID, "Owner")
	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)

	// Gear exists but is not shared with any community.
	gear := &models.Gear{OwnerId: ownerID, Name: "Unshared"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}

	ctx := contextWithAuth(ownerID, "owner@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	_, err = svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("expected error for unshared gear, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected CodeFailedPrecondition, got %v", connect.CodeOf(err))
	}
}

// TestStartConversation_GearTopic_Idempotent verifies that calling
// StartConversation twice returns the same conversation_id.
func TestStartConversation_GearTopic_Idempotent(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner"
	createTestUsers(t, store, ownerID, "Owner")
	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)

	gear := &models.Gear{OwnerId: ownerID, Name: "Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	ctx := contextWithAuth(ownerID, "owner@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	resp1, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	resp2, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if resp1.Msg.ConversationId != resp2.Msg.ConversationId {
		t.Errorf("idempotency violated: got %q then %q",
			resp1.Msg.ConversationId, resp2.Msg.ConversationId)
	}
}

// TestStartConversation_GearTopic_BackfillLogged verifies that a Warn log line
// is emitted when StartConversation creates a conversation for a gear that is
// already shared but whose conversation_id was missing.
func TestStartConversation_GearTopic_BackfillLogged(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner"
	createTestUsers(t, store, ownerID, "Owner")
	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)

	// Gear with no conversation_id (as if ShareGear-time creation missed it).
	gear := &models.Gear{OwnerId: ownerID, Name: "Gear", ConversationId: ""}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	// Inject a capturing logger into the request context.
	buf, ctx := captureGearTestLogger(contextWithAuth(ownerID, "owner@example.com"))

	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	if _, err := svc.StartConversation(ctx, req); err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}

	records := decodeGearTestRecords(buf)
	found := false
	for _, rec := range records {
		if rec["level"] == "WARN" && rec["msg"] == "backfilling gear conversation at chat open" {
			found = true
			if rec["gear_id"] != gearID {
				t.Errorf("expected gear_id=%q in log, got %q", gearID, rec["gear_id"])
			}
			break
		}
	}
	if !found {
		t.Errorf("expected Warn log 'backfilling gear conversation at chat open'; got records: %v", records)
	}
}

// TestStartConversation_GearTopic_BackfillSeedsDescription verifies that when a
// gear conversation is born lazily at chat-open (ShareGear-time creation was
// missed), StartConversation seeds the sharing anchor + the owner's description
// as the first comment — the same opening pair ShareGear writes (#2509) — and
// does not duplicate them on a second call.
func TestStartConversation_GearTopic_BackfillSeedsDescription(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner"
	createTestUsers(t, store, ownerID, "Owner")
	communityID := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityID, ownerID)

	const desc = "16oz Estwing claw hammer, like new."
	gear := &models.Gear{OwnerId: ownerID, Name: "Hammer", Description: desc, ConversationId: ""}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear: %v", err)
	}

	ctx := contextWithAuth(ownerID, "owner@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	resp, err := svc.StartConversation(ctx, req)
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}

	msgs, err := storage.ListByConversation(context.Background(), store, resp.Msg.ConversationId, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 seeded messages (anchor + description), got %d", len(msgs))
	}
	sys := msgs[0].GetSystemMessage()
	if sys == nil || sys.Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED {
		t.Errorf("first message should be the LOAN_SHARED anchor, got %+v", msgs[0])
	}
	user := msgs[1].GetUserMessage()
	if user == nil {
		t.Fatalf("second message should be the description comment, got %+v", msgs[1])
	}
	if user.SenderId != ownerID {
		t.Errorf("comment sender = %q, want owner %q", user.SenderId, ownerID)
	}
	if user.Text != desc {
		t.Errorf("comment text = %q, want %q", user.Text, desc)
	}

	// A second StartConversation must not re-seed (the gear now has a
	// conversation_id, so the backfill branch is skipped).
	if _, err := svc.StartConversation(ctx, req); err != nil {
		t.Fatalf("second StartConversation failed: %v", err)
	}
	msgsAfter, err := storage.ListByConversation(context.Background(), store, resp.Msg.ConversationId, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation (after) failed: %v", err)
	}
	if len(msgsAfter) != 2 {
		t.Errorf("expected no duplicate seeding on re-open, got %d messages", len(msgsAfter))
	}
}

// TestStartConversation_GearTopic_RequiresCommunityIdInShareSet verifies that
// the request's community_id must be a community the gear is actually shared
// with, not just any community the caller is in.
func TestStartConversation_GearTopic_RequiresCommunityIdInShareSet(t *testing.T) {
	svc, store := setupTestChatService(t)

	ownerID := "owner"
	createTestUsers(t, store, ownerID, "Owner")

	// Two communities: gear is only shared with communityA.
	communityA := createTestCommunity(t, store, ownerID)
	communityB := createTestCommunity(t, store, ownerID)
	createTestCommunityMembership(t, store, communityA, ownerID)
	createTestCommunityMembership(t, store, communityB, ownerID)

	gear := &models.Gear{OwnerId: ownerID, Name: "Gear"}
	gearID, err := store.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("failed to insert gear: %v", err)
	}
	_, err = store.Insert(context.Background(), &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityA,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	})
	if err != nil {
		t.Fatalf("failed to share gear with communityA: %v", err)
	}

	// Request uses communityB — the gear is NOT shared there.
	ctx := contextWithAuth(ownerID, "owner@example.com")
	req := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityB,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_GearId{GearId: gearID}},
	})

	_, err = svc.StartConversation(ctx, req)
	if err == nil {
		t.Fatal("expected error when community_id not in share set, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected CodeFailedPrecondition, got %v", connect.CodeOf(err))
	}
}

// TestGetConversation_GearNonCommunityMemberDenied verifies that users who are not
// in the community cannot access gear conversations.
func TestGetConversation_GearNonCommunityMemberDenied(t *testing.T) {
	svc, storage := setupTestChatService(t)

	// Create users
	ownerID := "owner-user"
	participantID := "participant-user"
	outsiderID := "outsider-user"
	createTestUsers(t, storage, ownerID, "Owner User")
	createTestUsers(t, storage, participantID, "Participant User")
	createTestUsers(t, storage, outsiderID, "Outsider User")

	// Create community
	communityID := createTestCommunity(t, storage, ownerID)

	// Add only owner and participant to the community (NOT outsider)
	addUserToCommunity(t, storage, ownerID, communityID)
	addUserToCommunity(t, storage, participantID, communityID)

	// Create gear
	gear := &models.Gear{
		OwnerId: ownerID,
		Name:    "Test Gear",
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}

	// Create a gear conversation
	conversation := &models.ChatConversation{
		CommunityId: communityID,
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{
			GearId: gearID,
		}},
		ParticipantIds: []string{ownerID, participantID},
	}
	conversationID, err := storage.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Test: Outsider (not in community) should be denied
	ctx := contextWithAuth(outsiderID, "outsider@example.com")
	req := connect.NewRequest(&api.GetConversationRequest{
		ConversationId: conversationID,
	})
	_, err = svc.GetConversation(ctx, req)
	if err == nil {
		t.Fatal("Expected error when non-community-member tries to access conversation")
	}

	connectErr := err.(*connect.Error)
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", connectErr.Code())
	}
}
