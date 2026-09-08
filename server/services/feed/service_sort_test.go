package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestGetFeed_SortOrder_UnreadBeforeRead verifies the three-tier sort:
// unread first (by unread_at desc), then read (by last_activity_at desc),
// then action-required (by last_activity_at desc).
// TestGetFeed_SortOrder_ChronologicalRegardlessOfReadStatus verifies that feed
// items are sorted by OccurredAtUnixSec desc regardless of read/unread status.
// Unread flags are still set for badge display but do not affect position.
func TestGetFeed_SortOrder_ChronologicalRegardlessOfReadStatus(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "sort@example.com", "Sort User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Sort Community")

	// Create 3 gear items with events at different times.
	// Gear A: oldest event, will be marked as read.
	gearA := setupTestGear(t, sqlStorage, userID, "Gear A")
	setupCommunityGear(t, sqlStorage, communityID, gearA, models.Availability_AVAILABILITY_FOR_LOAN)
	eventA := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearA,
		OccurredAtUnixSec: time.Now().Unix() - 7200, // 2h ago
	}
	eventAID, err := sqlStorage.Insert(context.Background(), eventA)
	if err != nil {
		t.Fatalf("Failed to create event A: %v", err)
	}

	// Gear B: middle event, will remain unread.
	gearB := setupTestGear(t, sqlStorage, userID, "Gear B")
	setupCommunityGear(t, sqlStorage, communityID, gearB, models.Availability_AVAILABILITY_FOR_LOAN)
	eventB := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearB,
		OccurredAtUnixSec: time.Now().Unix() - 3600, // 1h ago
	}
	if _, err := sqlStorage.Insert(context.Background(), eventB); err != nil {
		t.Fatalf("Failed to create event B: %v", err)
	}

	// Gear C: most recent event, will be marked as read.
	gearC := setupTestGear(t, sqlStorage, userID, "Gear C")
	setupCommunityGear(t, sqlStorage, communityID, gearC, models.Availability_AVAILABILITY_FOR_LOAN)
	eventC := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearC,
		OccurredAtUnixSec: time.Now().Unix(), // now
	}
	eventCID, err := sqlStorage.Insert(context.Background(), eventC)
	if err != nil {
		t.Fatalf("Failed to create event C: %v", err)
	}

	// Mark A and C as viewed (read), leave B unread.
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventAID, "event"); err != nil {
		t.Fatalf("Failed to mark A viewed: %v", err)
	}
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventCID, "event"); err != nil {
		t.Fatalf("Failed to mark C viewed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "sort@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(resp.Msg.Items))
	}

	// Chronological order: C (now), B (1h ago), A (2h ago) — read status
	// does not affect position.
	first := resp.Msg.Items[0]
	second := resp.Msg.Items[1]
	third := resp.Msg.Items[2]

	if first.GetGearShared().GearId != gearC {
		t.Errorf("Expected first item to be Gear C (most recent), got gear %s", first.GetGearShared().GearId)
	}
	if first.IsUnread {
		t.Error("Expected first item (Gear C) to be read")
	}

	if second.GetGearShared().GearId != gearB {
		t.Errorf("Expected second item to be Gear B (middle), got gear %s", second.GetGearShared().GearId)
	}
	if !second.IsUnread {
		t.Error("Expected second item (Gear B) to be unread")
	}
	if second.UnreadAtUnixSec == 0 {
		t.Error("Expected unread_at_unix_sec to be set on unread item")
	}

	if third.GetGearShared().GearId != gearA {
		t.Errorf("Expected third item to be Gear A (oldest), got gear %s", third.GetGearShared().GearId)
	}
	if third.IsUnread {
		t.Error("Expected third item (Gear A) to be read")
	}
}

// TestGetFeed_SortOrder_ChronologicalRegardlessOfActionRequired verifies that
// feed items are sorted by OccurredAtUnixSec desc regardless of action-required
// status. The is_action_required flag is still populated for badge display.
func TestGetFeed_SortOrder_ChronologicalRegardlessOfActionRequired(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "sort2@example.com", "Sort2 User")
	otherID := setupTestUser(t, sqlStorage, "other2@example.com", "Other2 User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Sort2 Community")
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           otherID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	// Gear A: older, action-required (pending interest from other user).
	gearA := setupTestGear(t, sqlStorage, userID, "Gear A Action")
	setupCommunityGear(t, sqlStorage, communityID, gearA, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	eventA := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearA,
		OccurredAtUnixSec: time.Now().Unix() - 7200, // 2h ago
	}
	eventAID, err := sqlStorage.Insert(context.Background(), eventA)
	if err != nil {
		t.Fatalf("Failed to create event A: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.Transfer{
		GearId:      gearA,
		OwnerId:     userID,
		RecipientId: otherID,
		State:       models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		CommunityId: communityID,
	}); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	// Gear B: newer, no pending interest (plain read item).
	gearB := setupTestGear(t, sqlStorage, userID, "Gear B Read")
	setupCommunityGear(t, sqlStorage, communityID, gearB, models.Availability_AVAILABILITY_FOR_LOAN)
	eventB := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearB,
		OccurredAtUnixSec: time.Now().Unix(), // now
	}
	eventBID, err := sqlStorage.Insert(context.Background(), eventB)
	if err != nil {
		t.Fatalf("Failed to create event B: %v", err)
	}

	// Mark both items as read.
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventAID, "event"); err != nil {
		t.Fatalf("Failed to mark A viewed: %v", err)
	}
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventBID, "event"); err != nil {
		t.Fatalf("Failed to mark B viewed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "sort2@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(resp.Msg.Items))
	}

	// Chronological order: B (now) before A (2h ago), regardless of
	// action-required status. Flags are still set for badge display.
	first := resp.Msg.Items[0]
	second := resp.Msg.Items[1]

	if first.GetGearShared().GearId != gearB {
		t.Errorf("Expected first item to be Gear B (most recent), got gear %s", first.GetGearShared().GearId)
	}
	if first.IsActionRequired {
		t.Error("Expected Gear B to NOT be action-required")
	}

	if second.GetGearShared().GearId != gearA {
		t.Errorf("Expected second item to be Gear A (older), got gear %s", second.GetGearShared().GearId)
	}
	if !second.IsActionRequired {
		t.Error("Expected Gear A to still have is_action_required set for badge display")
	}
}

// TestGetFeed_SortOrder_NewItemAboveOlderWithConversation verifies that a newly
// created item sorts above an older item that has recent conversation activity.
// This is the core scenario from issue #1179: conversation activity should not
// inflate an older item's sort position above a brand-new item.
func TestGetFeed_SortOrder_NewItemAboveOlderWithConversation(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userA := setupTestUser(t, sqlStorage, "owner@example.com", "Owner")
	userB := setupTestUser(t, sqlStorage, "commenter@example.com", "Commenter")
	communityID := setupTestCommunity(t, sqlStorage, userA, "Sort Conv Community")
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userB,
		InviterId:        userA,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	now := time.Now().Unix()

	// Gear A: created 1h ago.
	gearA := setupTestGear(t, sqlStorage, userA, "Older Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearA, models.Availability_AVAILABILITY_FOR_LOAN)
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userA,
		GearId:            gearA,
		OccurredAtUnixSec: now - 3600,
	}); err != nil {
		t.Fatalf("Failed to create event A: %v", err)
	}

	// Simulate a conversation message on Gear A from userB, posted 30s ago.
	// This gives Gear A a LastActivityAtUnixSec of now-30, which under the old
	// tiered sort would push it above a brand-new item created at now-10.
	convA := &models.ChatConversation{
		CommunityId: communityID,
		Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearA}},
	}
	convAID, err := sqlStorage.Insert(context.Background(), convA)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}
	// Update gear with conversation_id.
	gearModel := &models.Gear{}
	if err := sqlStorage.GetByID(context.Background(), gearA, gearModel); err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	gearModel.ConversationId = convAID
	if err := sqlStorage.Update(context.Background(), gearModel); err != nil {
		t.Fatalf("Failed to update gear: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.ChatMessage{
		ConversationId: convAID,
		SentAtUnixSec:  now - 30,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: userB,
				Text:     "Recent comment!",
			},
		},
	}); err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Gear B: brand new, created 10s ago. No conversation activity.
	gearB := setupTestGear(t, sqlStorage, userA, "Brand New Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearB, models.Availability_AVAILABILITY_FOR_LOAN)
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userA,
		GearId:            gearB,
		OccurredAtUnixSec: now - 10,
	}); err != nil {
		t.Fatalf("Failed to create event B: %v", err)
	}

	// Fetch feed as userA — the brand-new item should be first despite
	// Gear A having more recent conversation activity.
	ctx := createAuthenticatedContext(userA, "owner@example.com", models.Role_ROLE_USER)
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(resp.Msg.Items))
	}

	// Chronological order by creation time: B (10s ago) before A (1h ago).
	first := resp.Msg.Items[0]
	second := resp.Msg.Items[1]

	if first.GetGearShared().GearId != gearB {
		t.Errorf("Expected first item to be Brand New Gear (most recently created), got %s", first.GetGearShared().GearName)
	}
	if second.GetGearShared().GearId != gearA {
		t.Errorf("Expected second item to be Older Gear, got %s", second.GetGearShared().GearName)
	}

	// Verify that LastActivityAtUnixSec still reflects conversation activity
	// (it's still computed for display, just not used for sorting).
	if second.LastActivityAtUnixSec <= second.OccurredAtUnixSec {
		t.Error("Expected Older Gear's LastActivityAtUnixSec to be bumped by conversation activity")
	}
}

// TestGetFeed_CommentActivity_MarksUnread reproduces a bug where User A adds a
// comment to a gear item, User B loads the feed, and the item shows at the top
// but is_unread is false and GetFeedStatus doesn't show "New". The conversation
// activity should make the item unread for User B.
func TestGetFeed_CommentActivity_MarksUnread(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)

	userA := setupTestUser(t, sqlStorage, "usera@example.com", "User A")
	userB := setupTestUser(t, sqlStorage, "userb@example.com", "User B")
	communityID := setupTestCommunity(t, sqlStorage, userA, "Comment Community")

	// Add User B as member.
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userB,
		InviterId:        userA,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	// Create gear shared by User A, with a community-level conversation.
	gearID := setupTestGear(t, sqlStorage, userA, "Commented Gear")
	cgID := setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create the conversation for this gear.
	conv := &models.ChatConversation{
		CommunityId: communityID,
		Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
	}
	convID, err := sqlStorage.Insert(context.Background(), conv)
	if err != nil {
		t.Fatalf("Failed to create conversation: %v", err)
	}

	// Link the conversation to the Gear record.
	_ = cgID // cgID used above for community gear setup
	gearStored := &models.Gear{}
	if err := sqlStorage.GetByID(context.Background(), gearID, gearStored); err != nil {
		t.Fatalf("Failed to get Gear: %v", err)
	}
	gearStored.ConversationId = convID
	if err := sqlStorage.Update(context.Background(), gearStored); err != nil {
		t.Fatalf("Failed to update Gear conversation: %v", err)
	}

	// Create the gear shared event (happened 1 hour ago).
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userA,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix() - 3600,
	}
	eventID, err := sqlStorage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	// User B views the feed (marks the item as seen).
	if err := feedStorage.IncrementViewCount(context.Background(), userB, communityID, eventID, "event"); err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// User A adds a comment to the gear conversation (1 second in the future
	// relative to the view, to ensure the message is strictly newer than
	// last_seen_at even when both happen in the same wall-clock second).
	chatMsg := &models.ChatMessage{
		ConversationId: convID,
		SentAtUnixSec:  time.Now().Unix() + 1,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: userA,
				Text:     "Hey, this gear is great!",
			},
		},
	}
	if _, err := sqlStorage.Insert(context.Background(), chatMsg); err != nil {
		t.Fatalf("Failed to insert chat message: %v", err)
	}

	// Now User B loads the feed. The item should be unread because User A's
	// comment is newer than User B's last_seen_at.
	ctxB := createAuthenticatedContext(userB, "userb@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctxB, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if !item.IsUnread {
		t.Error("Expected item to be unread after User A added a comment")
	}
	if item.UnreadAtUnixSec == 0 {
		t.Error("Expected unread_at_unix_sec to be set")
	}

	// GetFeedStatus should also show the community as "new".
	statusResp, err := service.GetFeedStatus(ctxB, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus failed: %v", err)
	}
	if !statusResp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("Expected community to show as new (comment from another user)")
	}
}

// TestGetFeed_CrossCommunityDeduplication verifies that an item shared with
// multiple communities appears exactly once in a multi-community feed request.
func TestGetFeed_CrossCommunityDeduplication(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	communityAID := setupTestCommunity(t, sqlStorage, userID, "Community A")
	communityBID := setupTestCommunity(t, sqlStorage, userID, "Community B")
	gearID := setupTestGear(t, sqlStorage, userID, "Shared Gear")

	// Share the same gear with both communities.
	setupCommunityGear(t, sqlStorage, communityAID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	setupCommunityGear(t, sqlStorage, communityBID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	createGearSharedEvent(t, sqlStorage, communityAID, userID, gearID)
	createGearSharedEvent(t, sqlStorage, communityBID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "owner@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityAID, communityBID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Count how many times the gear appears.
	gearCount := 0
	for _, item := range resp.Msg.Items {
		if gs := item.GetGearShared(); gs != nil && gs.GearId == gearID {
			gearCount++
		}
	}
	if gearCount != 1 {
		t.Errorf("expected gear to appear exactly once across both communities, got %d", gearCount)
	}
}

// TestGetFeed_CrossCommunityDedup_UsesEarliestTimestamp verifies that when an
// item is shared with multiple communities at different times, the dedup keeps
// the occurrence with the earliest OccurredAtUnixSec so the item sorts by its
// original sharing time.
func TestGetFeed_CrossCommunityDedup_UsesEarliestTimestamp(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "dedup@test.com", "Dedup User")
	communityAID := setupTestCommunity(t, sqlStorage, userID, "Community Early")
	communityBID := setupTestCommunity(t, sqlStorage, userID, "Community Late")
	gearID := setupTestGear(t, sqlStorage, userID, "Cross-Posted Gear")

	now := time.Now().Unix()

	// Share the same gear with both communities at different times.
	setupCommunityGear(t, sqlStorage, communityAID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	setupCommunityGear(t, sqlStorage, communityBID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Community A: shared 2h ago (earlier).
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityAID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: now - 7200,
	}); err != nil {
		t.Fatalf("Failed to create event A: %v", err)
	}

	// Community B: shared 30min ago (later).
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityBID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: now - 1800,
	}); err != nil {
		t.Fatalf("Failed to create event B: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "dedup@test.com", models.Role_ROLE_USER)

	// Request feed for both communities — community B is listed first to
	// verify that the dedup still picks the earlier timestamp from A.
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityBID, communityAID},
		PageSize:     20,
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Find the gear item.
	var gearItem *api.FeedItem
	gearCount := 0
	for _, item := range resp.Msg.Items {
		if gs := item.GetGearShared(); gs != nil && gs.GearId == gearID {
			gearItem = item
			gearCount++
		}
	}
	if gearCount != 1 {
		t.Fatalf("Expected gear to appear exactly once, got %d", gearCount)
	}

	// The kept item should use the earlier timestamp (community A, 2h ago).
	expectedTimestamp := now - 7200
	if gearItem.OccurredAtUnixSec != expectedTimestamp {
		t.Errorf("Expected OccurredAtUnixSec to be %d (earliest sharing), got %d",
			expectedTimestamp, gearItem.OccurredAtUnixSec)
	}
}

// TestGetFeedStatus_ChatActivityTriggersNew verifies the conversation-activity
// branch of GetFeedStatus: a GEAR_SHARED event that the user has already viewed
// must still surface as "new" when another user posts a chat message to the gear's
// conversation after the view was recorded.
func TestGetFeedStatus_ChatActivityTriggersNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	ctx := context.Background()

	userA := setupTestUser(t, sqlStorage, "usera-chat@example.com", "User A")
	userB := setupTestUser(t, sqlStorage, "userb-chat@example.com", "User B")
	communityID := setupTestCommunity(t, sqlStorage, userA, "Chat Status Community")

	// Add User B as member.
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userB,
		InviterId:        userA,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(ctx, membership); err != nil {
		t.Fatalf("add member: %v", err)
	}

	gearID := setupTestGear(t, sqlStorage, userA, "Chat Status Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create a gear-scoped conversation and link it to the Gear record.
	conv := &models.ChatConversation{
		CommunityId: communityID,
		Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
	}
	convID, err := sqlStorage.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	gearStored := &models.Gear{}
	if err := sqlStorage.GetByID(ctx, gearID, gearStored); err != nil {
		t.Fatalf("get gear: %v", err)
	}
	gearStored.ConversationId = convID
	if err := sqlStorage.Update(ctx, gearStored); err != nil {
		t.Fatalf("update gear conversation_id: %v", err)
	}

	// Create the GEAR_SHARED event (1h ago).
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userA,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix() - 3600,
	}
	eventID, err := sqlStorage.Insert(ctx, event)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	// User B views the feed item — badge should clear.
	if err := feedStorage.IncrementViewCount(ctx, userB, communityID, eventID, "event"); err != nil {
		t.Fatalf("IncrementViewCount: %v", err)
	}

	authCtxB := createAuthenticatedContext(userB, "userb-chat@example.com", models.Role_ROLE_USER)

	// Before the new message: hasNew must be false (user B already viewed).
	resp, err := svc.GetFeedStatus(authCtxB, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (before message): %v", err)
	}
	if resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("expected hasNew=false before any new message")
	}

	// User A posts a chat message after User B's view (use +1s to guarantee
	// strict ordering even when both happen within the same wall-clock second).
	chatMsg := &models.ChatMessage{
		ConversationId: convID,
		SentAtUnixSec:  time.Now().Unix() + 1,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: userA,
				Text:     "Hey, interested in this?",
			},
		},
	}
	if _, err := sqlStorage.Insert(ctx, chatMsg); err != nil {
		t.Fatalf("insert chat message: %v", err)
	}

	// After the new message: hasNew must be true for User B.
	resp, err = svc.GetFeedStatus(authCtxB, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (after message): %v", err)
	}
	if !resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("expected hasNew=true after another user posted a chat message")
	}
}
