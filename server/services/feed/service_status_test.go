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

// TestGetFeedStatus_TerminalEventsDoNotTriggerNew verifies that terminal events
// (TRANSFER_COMPLETED, TRANSFER_CANCELLED, REQUEST_FULFILLED, REQUEST_CANCELLED)
// are excluded from the freshness check.  These events are never shown in the
// feed, so users can never acquire view records for them.  Including them caused
// communities to be permanently stuck showing "New" in the sidebar.
func TestGetFeedStatus_TerminalEventsDoNotTriggerNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "user@example.com", "User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

	authCtx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)

	baseTime := time.Now().Unix() - 3600

	// Insert ONLY a terminal event (TRANSFER_COMPLETED). The user can never
	// see or view this in the feed, so it must not trigger hasNew=true.
	terminalEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime,
	}
	if _, err := sqlStorage.Insert(ctx, terminalEvent); err != nil {
		t.Fatalf("insert terminal event: %v", err)
	}

	resp, err := svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus: %v", err)
	}
	got := resp.Msg.CommunityIdToHasNew[communityID]
	if got {
		t.Errorf("communityIdToHasNew[%s] = true, want false: "+
			"terminal events must not trigger the New indicator because they "+
			"are never shown in the feed and can never be viewed by the user",
			communityID)
	}
}

// TestGetFeedStatus_VisibleEventWithNoViewTriggerNew verifies that a standard
// visible event with no view record correctly flags the community as new.
func TestGetFeedStatus_VisibleEventWithNoViewTriggerNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "user2@example.com", "User2")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community 2")
	gearID := setupTestGear(t, sqlStorage, userID, "Gear 2")

	authCtx := createAuthenticatedContext(userID, "user2@example.com", models.Role_ROLE_USER)

	// GEAR_SHARED is visible in the feed and has no view record yet.
	visibleEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix() - 3600,
	}
	if _, err := sqlStorage.Insert(ctx, visibleEvent); err != nil {
		t.Fatalf("insert visible event: %v", err)
	}

	resp, err := svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus: %v", err)
	}
	got := resp.Msg.CommunityIdToHasNew[communityID]
	if !got {
		t.Errorf("communityIdToHasNew[%s] = false, want true: "+
			"a visible event with no view record must trigger New", communityID)
	}
}

// TestGetFeedStatus_TerminalPlusViewedVisible verifies the common real-world
// scenario: a gear was shared (GEAR_SHARED) and later completed (TRANSFER_COMPLETED).
// Once the user views the GEAR_SHARED card, the community must show hasNew=false
// even though the TRANSFER_COMPLETED event has no view record.
func TestGetFeedStatus_TerminalPlusViewedVisible(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "user3@example.com", "User3")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community 3")
	gearID := setupTestGear(t, sqlStorage, userID, "Gear 3")

	authCtx := createAuthenticatedContext(userID, "user3@example.com", models.Role_ROLE_USER)

	baseTime := time.Now().Unix() - 7200

	// GEAR_SHARED — visible in the feed.
	visibleEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime,
	}
	visibleEventID, err := sqlStorage.Insert(ctx, visibleEvent)
	if err != nil {
		t.Fatalf("insert visible event: %v", err)
	}

	// TRANSFER_COMPLETED — terminal, never shown in the feed.
	terminalEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime + 3600,
	}
	if _, err := sqlStorage.Insert(ctx, terminalEvent); err != nil {
		t.Fatalf("insert terminal event: %v", err)
	}

	// Before viewing: community should be flagged new.
	resp, err := svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (before view): %v", err)
	}
	if !resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("expected hasNew=true before viewing the feed item")
	}

	// User views the GEAR_SHARED card.
	feedStorage := storage.NewFeedStorage(sqlStorage)
	if markErr := feedStorage.IncrementViewCountBatch(ctx, userID, communityID,
		[]string{visibleEventID}, "gear", nil); markErr != nil {
		t.Fatalf("IncrementViewCountBatch: %v", markErr)
	}

	// After viewing: community must show hasNew=false despite the unviewed
	// TRANSFER_COMPLETED event, because that event is never shown in the feed.
	resp, err = svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (after view): %v", err)
	}
	if resp.Msg.CommunityIdToHasNew[communityID] {
		t.Errorf("communityIdToHasNew[%s] = true after viewing all visible events: "+
			"terminal TRANSFER_COMPLETED must not keep the New badge stuck", communityID)
	}
}

// TestGetFeedStatus_SupersededEventDoesNotTriggerNew verifies that when two events
// share the same item key (e.g. GEAR_SHARED then TRANSFER_INTEREST_EXPRESSED for
// the same gear), only the most-recent (champion) event must be viewed to clear
// the New badge.  The older event is never shown in the feed and therefore can
// never be viewed directly.
func TestGetFeedStatus_SupersededEventDoesNotTriggerNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "user4@example.com", "User4")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community 4")
	gearID := setupTestGear(t, sqlStorage, userID, "Gear 4")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	authCtx := createAuthenticatedContext(userID, "user4@example.com", models.Role_ROLE_USER)

	baseTime := time.Now().Unix() - 7200

	// Older event for the gear — same item key "gear:<gearID>".
	olderEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime,
	}
	if _, err := sqlStorage.Insert(ctx, olderEvent); err != nil {
		t.Fatalf("insert older event: %v", err)
	}

	// Newer event for the same gear — supersedes the older one in the feed.
	newerEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		ActorId:           userID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime + 3600,
	}
	newerEventID, err := sqlStorage.Insert(ctx, newerEvent)
	if err != nil {
		t.Fatalf("insert newer event: %v", err)
	}

	// Before viewing: should be new.
	resp, err := svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (before view): %v", err)
	}
	if !resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("expected hasNew=true before viewing")
	}

	// User views only the champion (newer) event — the older is never shown.
	feedStorage := storage.NewFeedStorage(sqlStorage)
	if markErr := feedStorage.IncrementViewCountBatch(ctx, userID, communityID,
		[]string{newerEventID}, "gear", nil); markErr != nil {
		t.Fatalf("IncrementViewCountBatch: %v", markErr)
	}

	// After viewing the champion: hasNew must be false even though the older
	// GEAR_SHARED event has no view record.
	resp, err = svc.GetFeedStatus(authCtx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus (after view): %v", err)
	}
	if resp.Msg.CommunityIdToHasNew[communityID] {
		t.Errorf("communityIdToHasNew[%s] = true after viewing the champion event: "+
			"superseded older event must not keep the New badge stuck", communityID)
	}
}

// TestIsItemUnread verifies the isItemUnread helper.
func TestIsItemUnread(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name           string
		view           *models.FeedItemView
		lastActivityAt int64
		wantUnread     bool
	}{
		{
			name:           "never seen (nil view) is unread",
			view:           nil,
			lastActivityAt: now,
			wantUnread:     true,
		},
		{
			name: "new activity since last seen is unread",
			view: &models.FeedItemView{
				LastSeenAtUnixSec: now - 3600,
			},
			lastActivityAt: now,
			wantUnread:     true,
		},
		{
			name: "seen and no new activity is read",
			view: &models.FeedItemView{
				LastSeenAtUnixSec: now,
			},
			lastActivityAt: now - 3600,
			wantUnread:     false,
		},
		{
			name: "seen at same time as activity is read",
			view: &models.FeedItemView{
				LastSeenAtUnixSec: now,
			},
			lastActivityAt: now,
			wantUnread:     false,
		},
		{
			name: "legacy view falls back to last_viewed_at",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:   0,
				LastViewedAtUnixSec: now - 3600,
			},
			lastActivityAt: now,
			wantUnread:     true,
		},
		{
			name: "legacy view with no new activity is read",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:   0,
				LastViewedAtUnixSec: now,
			},
			lastActivityAt: now - 3600,
			wantUnread:     false,
		},
		{
			name:           "activity older than 7 days is not unread (never seen)",
			view:           nil,
			lastActivityAt: now - 8*24*3600,
			wantUnread:     false,
		},
		{
			name: "activity older than 7 days is not unread (has view)",
			view: &models.FeedItemView{
				LastSeenAtUnixSec: now - 10*24*3600,
			},
			lastActivityAt: now - 8*24*3600,
			wantUnread:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isItemUnread(tt.view, tt.lastActivityAt, now)
			if got != tt.wantUnread {
				t.Errorf("isItemUnread() = %v, want %v", got, tt.wantUnread)
			}
		})
	}
}

// TestGetFeed_IsUnreadField verifies that GetFeed populates is_unread correctly.
func TestGetFeed_IsUnreadField(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "unread@example.com", "Unread User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Unread Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Unread Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "unread@example.com", models.Role_ROLE_USER)

	// First load: never seen → is_unread = true.
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}
	if !resp.Msg.Items[0].IsUnread {
		t.Error("Expected item to be unread on first view")
	}

	// Mark as viewed.
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventID, "event"); err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Second load: seen, no new activity → is_unread = false.
	resp, err = service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].IsUnread {
		t.Error("Expected item to be read after viewing")
	}
}

// TestGetFeed_ActionRequired verifies that gear items with pending transfer
// interest are marked as action-required and unread. Within the unread tier
// they sort by unread_at desc, so action-required does not override recency.
func TestGetFeed_ActionRequired(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@example.com", "Owner")
	otherID := setupTestUser(t, sqlStorage, "other@example.com", "Other")
	communityID := setupTestCommunity(t, sqlStorage, userID, "AR Community")
	// Add other user as member too.
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           otherID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	// Create two gear items owned by owner.
	gearWithInterest := setupTestGear(t, sqlStorage, userID, "Gear With Interest")
	gearNoInterest := setupTestGear(t, sqlStorage, userID, "Gear No Interest")
	setupCommunityGear(t, sqlStorage, communityID, gearWithInterest, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	setupCommunityGear(t, sqlStorage, communityID, gearNoInterest, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create events — use earlier timestamps for the interest gear so it would
	// normally sort after the no-interest gear.
	eventInterest := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearWithInterest,
		OccurredAtUnixSec: time.Now().Unix() - 3600, // 1 hour ago
	}
	interestEventID, err := sqlStorage.Insert(context.Background(), eventInterest)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	eventNoInterest := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearNoInterest,
		OccurredAtUnixSec: time.Now().Unix(), // now
	}
	noInterestEventID, err := sqlStorage.Insert(context.Background(), eventNoInterest)
	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	// Create a pending transfer interest on gearWithInterest from other user.
	transfer := &models.Transfer{
		GearId:      gearWithInterest,
		OwnerId:     userID,
		RecipientId: otherID,
		State:       models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		CommunityId: communityID,
	}
	if _, err := sqlStorage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "owner@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(resp.Msg.Items))
	}

	// Both items are unread. Unread items sort by unread_at desc, so the
	// newer non-action-required item (now) sorts before the older
	// action-required item (1h ago). Tier separation only applies to read items.
	first := resp.Msg.Items[0]
	second := resp.Msg.Items[1]

	if first.Id != noInterestEventID {
		t.Errorf("Expected newer unread item first, got event %s", first.Id)
	}
	if first.IsActionRequired {
		t.Error("Expected first item (newer unread) to NOT be action-required")
	}
	if !first.IsUnread {
		t.Error("Expected first item to be unread")
	}

	if second.Id != interestEventID {
		t.Errorf("Expected action-required item second, got event %s", second.Id)
	}
	if !second.IsActionRequired {
		t.Error("Expected second item to be action-required")
	}
	if !second.IsUnread {
		t.Error("Expected action-required item to be unread")
	}
}

// TestGetFeed_ActionRequired_NotOwner verifies that pending interest does not
// trigger action-required for users who do not own the gear.
func TestGetFeed_ActionRequired_NotOwner(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner2@example.com", "Owner2")
	viewerID := setupTestUser(t, sqlStorage, "viewer@example.com", "Viewer")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Not Owner Community")
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           viewerID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	gearID := setupTestGear(t, sqlStorage, userID, "Someone Else Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	// Pending interest from viewer (the viewer expressed interest, so the
	// action is on the owner, not the viewer).
	transfer := &models.Transfer{
		GearId:      gearID,
		OwnerId:     userID,
		RecipientId: viewerID,
		State:       models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		CommunityId: communityID,
	}
	if _, err := sqlStorage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	ctx := createAuthenticatedContext(viewerID, "viewer@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].IsActionRequired {
		t.Error("Expected item to NOT be action-required for non-owner")
	}
}

// TestGetFeed_ActionRequired_RecipientSelected verifies that gear with a
// selected recipient is no longer action-required.
func TestGetFeed_ActionRequired_RecipientSelected(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner3@example.com", "Owner3")
	otherID := setupTestUser(t, sqlStorage, "other3@example.com", "Other3")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Selected Community")
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           otherID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	gearID := setupTestGear(t, sqlStorage, userID, "Selected Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	// Transfer with recipient already selected — no longer pending.
	transfer := &models.Transfer{
		GearId:      gearID,
		OwnerId:     userID,
		RecipientId: otherID,
		State:       models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId: communityID,
	}
	if _, err := sqlStorage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "owner3@example.com", models.Role_ROLE_USER)

	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].IsActionRequired {
		t.Error("Expected item to NOT be action-required when recipient already selected")
	}
}

// TestGetFeedStatus_ActionRequired_ClearsAfterViewing reproduces a bug where
// action-required items permanently kept the sidebar "New" badge lit even after
// the user scrolled through all feed items. The action_required flag is a sort
// hint only — it must not override is_unread after the user has viewed the item.
func TestGetFeedStatus_ActionRequired_ClearsAfterViewing(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	feedStorage := storage.NewFeedStorage(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner4@example.com", "Owner4")
	otherID := setupTestUser(t, sqlStorage, "other4@example.com", "Other4")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Status AR Community")
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           otherID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add member: %v", err)
	}

	gearID := setupTestGear(t, sqlStorage, userID, "Status Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	// Create pending interest on the gear.
	transfer := &models.Transfer{
		GearId:      gearID,
		OwnerId:     userID,
		RecipientId: otherID,
		State:       models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		CommunityId: communityID,
	}
	if _, err := sqlStorage.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to create transfer: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "owner4@example.com", models.Role_ROLE_USER)

	// Before viewing: community should show as "new" (never-seen item).
	resp, err := service.GetFeedStatus(ctx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus failed: %v", err)
	}
	if !resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("Expected community to show as new before viewing")
	}

	// User scrolls through the feed, marking the item as viewed.
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventID, "event"); err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// After viewing: community should NOT show as "new" even though pending
	// interest still exists. The user has seen the item — the badge should clear.
	resp, err = service.GetFeedStatus(ctx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus failed: %v", err)
	}
	if resp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("Expected community to NOT show as new after viewing " +
			"(action_required must not permanently override is_unread)")
	}
}

// TestGetFeed_UnreadWindow_OldActivityNotUnread verifies that items with
// activity older than 7 days are not marked as unread, even if never seen.
func TestGetFeed_UnreadWindow_OldActivityNotUnread(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "window@example.com", "Window User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Window Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Old Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create an event that occurred 10 days ago — beyond the 7-day window.
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix() - 10*24*3600,
	}
	if _, err := sqlStorage.Insert(context.Background(), event); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "window@example.com", models.Role_ROLE_USER)

	// The item has never been seen, but activity is >7 days old → not unread.
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].IsUnread {
		t.Error("Expected old item (>7 days) to NOT be unread even though never seen")
	}

	// GetFeedStatus should also return false.
	statusResp, err := service.GetFeedStatus(ctx, connect.NewRequest(&api.GetFeedStatusRequest{}))
	if err != nil {
		t.Fatalf("GetFeedStatus failed: %v", err)
	}
	if statusResp.Msg.CommunityIdToHasNew[communityID] {
		t.Error("Expected community to NOT show as new (all activity >7 days old)")
	}
}
