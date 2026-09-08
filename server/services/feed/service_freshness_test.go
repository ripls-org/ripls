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

// TestIsItemFresh tests the freshness expiry logic without database access.
func TestIsItemFresh(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name           string
		view           *models.FeedItemView
		lastActivityAt int64
		now            int64
		expiryHours    int32
		wantFresh      bool
	}{
		{
			name:           "nil view is always fresh",
			view:           nil,
			lastActivityAt: now - 3600,
			now:            now,
			expiryHours:    24,
			wantFresh:      true,
		},
		{
			name: "new activity since last seen is always fresh",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:      now - 3600,
				ViewsSinceLastActivity: 5,
			},
			lastActivityAt: now - 1800, // activity after last seen
			now:            now,
			expiryHours:    24,
			wantFresh:      true,
		},
		{
			name: "viewed recently within expiry window stays fresh",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:      now - 3600, // 1h ago
				ViewsSinceLastActivity: 5,          // view count no longer drives expiry
			},
			lastActivityAt: now - 7200,
			now:            now,
			expiryHours:    24,
			wantFresh:      true,
		},
		{
			name: "exceeded time threshold expires item",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:      now - 25*3600, // 25 hours ago
				ViewsSinceLastActivity: 1,
			},
			lastActivityAt: now - 30*3600,
			now:            now,
			expiryHours:    24,
			wantFresh:      false,
		},
		{
			name: "custom 1h expiry: recently seen stays fresh",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:      now - 60, // 1 min ago
				ViewsSinceLastActivity: 10,
			},
			lastActivityAt: now - 3600,
			now:            now,
			expiryHours:    1,
			wantFresh:      true,
		},
		{
			name: "custom 1h expiry: old seen expires",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:      now - 2*3600, // 2h ago
				ViewsSinceLastActivity: 1,
			},
			lastActivityAt: now - 3*3600,
			now:            now,
			expiryHours:    1,
			wantFresh:      false,
		},
		{
			name: "old record with last_seen_at=0 falls back to last_viewed_at",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:   0,          // pre-Phase1 record: no last_seen_at
				LastViewedAtUnixSec: now - 3600, // viewed 1h ago
			},
			lastActivityAt: now - 7200,
			now:            now,
			expiryHours:    24,
			wantFresh:      true, // 1h < 24h → still fresh
		},
		{
			name: "old record: last_viewed_at beyond expiry also expires",
			view: &models.FeedItemView{
				LastSeenAtUnixSec:   0,
				LastViewedAtUnixSec: now - 25*3600,
			},
			lastActivityAt: now - 30*3600,
			now:            now,
			expiryHours:    24,
			wantFresh:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isItemFresh(tc.view, tc.lastActivityAt, tc.now, 0, tc.expiryHours)
			if got != tc.wantFresh {
				t.Errorf("isItemFresh() = %v, want %v", got, tc.wantFresh)
			}
		})
	}
}

// TestGetFeed_FreshnessFilter verifies that seen items remain in the feed within
// the 24-hour window, and expire when viewed more than 24 hours ago.
func TestGetFeed_FreshnessFilter(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "fresh@test.com", "Fresh User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Fresh Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Fresh Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Insert a gear-shared event with occurred_at = 30h ago so the view record
	// can be set to 25h ago (after the event) to test time-based expiry.
	oldEventTime := time.Now().Unix() - 30*3600
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: oldEventTime,
	}
	eventID, err := sqlStorage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Insert event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "fresh@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})

	// First fetch — item should appear (never seen).
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item before marking viewed, got %d", len(resp.Msg.Items))
	}

	// Mark as viewed many times — view count alone should not expire the item.
	markReq := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: communityID,
		ItemIds:     []string{eventID},
	})
	for i := 0; i < 5; i++ {
		if _, err := service.MarkFeedItemsViewed(ctx, markReq); err != nil {
			t.Fatalf("MarkFeedItemsViewed failed: %v", err)
		}
	}

	// Item was viewed recently — it should still appear within the 24h window.
	resp, err = service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed after marking viewed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Errorf("Expected item to remain fresh within 24h window, got %d items", len(resp.Msg.Items))
	}

	// Backdate the view record to 25 hours ago (event was 30h ago, so
	// lastSeen >= lastActivityAt is satisfied, and 25h > 24h threshold).
	staleTime := time.Now().Unix() - 25*3600
	views, err := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{eventID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}
	view := views[eventID]
	if view == nil {
		t.Fatal("Expected view record to exist after MarkFeedItemsViewed")
	}
	view.LastSeenAtUnixSec = staleTime
	view.LastViewedAtUnixSec = staleTime
	if err := sqlStorage.Update(ctx, view); err != nil {
		t.Fatalf("Failed to backdate view record: %v", err)
	}

	// Now the item should have expired (25h > default 24h threshold).
	resp, err = service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed after stale timestamp: %v", err)
	}
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 items after 24h expiry, got %d", len(resp.Msg.Items))
	}
}

// TestGetFeed_EphemeralItems verifies that experiences, requests, and giveaways
// stay in the feed after being seen (bypassing freshness expiry), while loan
// gear items expire normally.
func TestGetFeed_EphemeralItems(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "ephemeral@test.com", "Ephemeral User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Ephemeral Community")

	// Create one of each: loan gear, giveaway gear, request, experience — all with events 30h ago.
	loanGearID := setupTestGear(t, sqlStorage, userID, "Old Loan Gear")
	setupCommunityGear(t, sqlStorage, communityID, loanGearID, models.Availability_AVAILABILITY_FOR_LOAN)

	giveawayGearID := setupTestGear(t, sqlStorage, userID, "Old Giveaway Gear")
	setupCommunityGear(t, sqlStorage, communityID, giveawayGearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Old Request", models.RequestState_REQUEST_STATE_ACTIVE)
	experienceID := setupTestExperience(t, sqlStorage, userID, "Old Experience")
	setupCommunityExperience(t, sqlStorage, communityID, experienceID, false)

	oldTime := time.Now().Unix() - 30*3600

	loanGearEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            loanGearID,
		OccurredAtUnixSec: oldTime,
	}
	loanGearEventID, err := sqlStorage.Insert(context.Background(), loanGearEvent)
	if err != nil {
		t.Fatalf("Insert loan gear event failed: %v", err)
	}

	giveawayGearEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            giveawayGearID,
		OccurredAtUnixSec: oldTime,
	}
	giveawayGearEventID, err := sqlStorage.Insert(context.Background(), giveawayGearEvent)
	if err != nil {
		t.Fatalf("Insert giveaway gear event failed: %v", err)
	}

	reqEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		ActorId:           userID,
		Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
		OccurredAtUnixSec: oldTime,
	}
	reqEventID, err := sqlStorage.Insert(context.Background(), reqEvent)
	if err != nil {
		t.Fatalf("Insert request event failed: %v", err)
	}

	expEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:           userID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
		OccurredAtUnixSec: oldTime,
	}
	expEventID, err := sqlStorage.Insert(context.Background(), expEvent)
	if err != nil {
		t.Fatalf("Insert experience event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "ephemeral@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})

	// All four should appear initially (never seen).
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 4 {
		t.Fatalf("Expected 4 items initially, got %d", len(resp.Msg.Items))
	}

	// Mark all as viewed, then backdate the view records to 25h ago (past expiry).
	allEventIDs := []string{loanGearEventID, giveawayGearEventID, reqEventID, expEventID}
	markReq := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: communityID,
		ItemIds:     allEventIDs,
	})
	if _, err := service.MarkFeedItemsViewed(ctx, markReq); err != nil {
		t.Fatalf("MarkFeedItemsViewed failed: %v", err)
	}

	staleTime := time.Now().Unix() - 25*3600
	for _, eid := range allEventIDs {
		views, vErr := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{eid})
		if vErr != nil {
			t.Fatalf("GetFeedItemViews failed: %v", vErr)
		}
		view := views[eid]
		if view == nil {
			t.Fatalf("Expected view record for %s", eid)
		}
		view.LastSeenAtUnixSec = staleTime
		view.LastViewedAtUnixSec = staleTime
		if uErr := sqlStorage.Update(ctx, view); uErr != nil {
			t.Fatalf("Failed to backdate view record: %v", uErr)
		}
	}

	// After expiry: loan gear should be gone; giveaway, request, and experience remain.
	resp, err = service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed after stale views: %v", err)
	}
	if len(resp.Msg.Items) != 3 {
		t.Fatalf("Expected 3 ephemeral items after expiry, got %d", len(resp.Msg.Items))
	}

	// Verify surviving items by checking their gear IDs and types.
	seenItemIDs := map[string]bool{}
	seenTypes := map[api.FeedItemType]bool{}
	for _, item := range resp.Msg.Items {
		seenItemIDs[item.Id] = true
		seenTypes[item.ItemType] = true
	}
	if !seenTypes[api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED] {
		t.Error("Expected request item to survive freshness expiry")
	}
	if !seenTypes[api.FeedItemType_FEED_ITEM_TYPE_EXPERIENCE_CREATED] {
		t.Error("Expected experience item to survive freshness expiry")
	}
	if !seenItemIDs[giveawayGearEventID] {
		t.Error("Expected giveaway gear item to survive freshness expiry")
	}
	if seenItemIDs[loanGearEventID] {
		t.Error("Expected loan gear item to be expired")
	}
}

// TestGetFeed_GiveawayTransferEventsEphemeral verifies that transfer lifecycle
// events on giveaway gear also bypass the staleness expiry.
func TestGetFeed_GiveawayTransferEventsEphemeral(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner2@test.com", "Owner2")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Giveaway Community")

	giveawayGearID := setupTestGear(t, sqlStorage, userID, "Giveaway Gear")
	setupCommunityGear(t, sqlStorage, communityID, giveawayGearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)

	oldTime := time.Now().Unix() - 30*3600

	// Use a transfer interest event (a lifecycle event) on the giveaway gear.
	interestEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		ActorId:           userID,
		GearId:            giveawayGearID,
		OccurredAtUnixSec: oldTime,
	}
	interestEventID, err := sqlStorage.Insert(context.Background(), interestEvent)
	if err != nil {
		t.Fatalf("Insert transfer interest event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "owner2@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})

	// Mark as viewed and backdate past expiry.
	markReq := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: communityID,
		ItemIds:     []string{interestEventID},
	})
	if _, err := service.MarkFeedItemsViewed(ctx, markReq); err != nil {
		t.Fatalf("MarkFeedItemsViewed failed: %v", err)
	}

	staleTime := time.Now().Unix() - 25*3600
	views, vErr := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{interestEventID})
	if vErr != nil {
		t.Fatalf("GetFeedItemViews failed: %v", vErr)
	}
	view := views[interestEventID]
	if view == nil {
		t.Fatalf("Expected view record for %s", interestEventID)
	}
	view.LastSeenAtUnixSec = staleTime
	view.LastViewedAtUnixSec = staleTime
	if uErr := sqlStorage.Update(ctx, view); uErr != nil {
		t.Fatalf("Failed to backdate view record: %v", uErr)
	}

	// The giveaway transfer event should survive expiry.
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item (giveaway transfer event), got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].Id != interestEventID {
		t.Errorf("Expected giveaway transfer event to survive expiry, got item %s", resp.Msg.Items[0].Id)
	}
}

// TestGetFeed_LoanTransferEventsEphemeral verifies that transfer lifecycle events
// on loan gear bypass the staleness expiry, while a plain GEAR_SHARED event on
// loan gear still expires normally.
func TestGetFeed_LoanTransferEventsEphemeral(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "loanowner@test.com", "Loan Owner")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Loan Community")

	oldTime := time.Now().Unix() - 30*3600

	transferEventTypes := []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
	}

	for _, eventType := range transferEventTypes {
		t.Run(eventType.String(), func(t *testing.T) {
			loanGearID := setupTestGear(t, sqlStorage, userID, "Loan Gear "+eventType.String())
			setupCommunityGear(t, sqlStorage, communityID, loanGearID, models.Availability_AVAILABILITY_FOR_LOAN)

			event := &models.CommunityEvent{
				CommunityId:       communityID,
				EventType:         eventType,
				ActorId:           userID,
				GearId:            loanGearID,
				OccurredAtUnixSec: oldTime,
			}
			eventID, err := sqlStorage.Insert(context.Background(), event)
			if err != nil {
				t.Fatalf("Insert event failed: %v", err)
			}

			ctx := createAuthenticatedContext(userID, "loanowner@test.com", models.Role_ROLE_USER)
			req := connect.NewRequest(&api.GetFeedRequest{
				CommunityIds: []string{communityID},
				PageSize:     20,
			})

			// Mark as viewed and backdate past expiry.
			markReq := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
				CommunityId: communityID,
				ItemIds:     []string{eventID},
			})
			if _, err := service.MarkFeedItemsViewed(ctx, markReq); err != nil {
				t.Fatalf("MarkFeedItemsViewed failed: %v", err)
			}

			staleTime := time.Now().Unix() - 25*3600
			views, vErr := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{eventID})
			if vErr != nil {
				t.Fatalf("GetFeedItemViews failed: %v", vErr)
			}
			view := views[eventID]
			if view == nil {
				t.Fatalf("Expected view record for %s", eventID)
			}
			view.LastSeenAtUnixSec = staleTime
			view.LastViewedAtUnixSec = staleTime
			if uErr := sqlStorage.Update(ctx, view); uErr != nil {
				t.Fatalf("Failed to backdate view record: %v", uErr)
			}

			resp, err := service.GetFeed(ctx, req)
			if err != nil {
				t.Fatalf("GetFeed failed: %v", err)
			}
			found := false
			for _, item := range resp.Msg.Items {
				if item.Id == eventID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected loan transfer event %s to survive freshness expiry", eventType)
			}
		})
	}
}

// TestGetFeed_LoanGearSharedExpires verifies that a plain GEAR_SHARED event on
// loan gear (gear that is merely available, no open transfer) still expires after
// 24h like any other non-ephemeral item.
func TestGetFeed_LoanGearSharedExpires(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "loanavail@test.com", "Loan Available")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Loan Avail Community")

	loanGearID := setupTestGear(t, sqlStorage, userID, "Available Loan Gear")
	setupCommunityGear(t, sqlStorage, communityID, loanGearID, models.Availability_AVAILABILITY_FOR_LOAN)

	oldTime := time.Now().Unix() - 30*3600
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            loanGearID,
		OccurredAtUnixSec: oldTime,
	}
	eventID, err := sqlStorage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Insert gear shared event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "loanavail@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})

	// Mark as viewed and backdate past expiry.
	markReq := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: communityID,
		ItemIds:     []string{eventID},
	})
	if _, err := service.MarkFeedItemsViewed(ctx, markReq); err != nil {
		t.Fatalf("MarkFeedItemsViewed failed: %v", err)
	}

	staleTime := time.Now().Unix() - 25*3600
	views, vErr := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{eventID})
	if vErr != nil {
		t.Fatalf("GetFeedItemViews failed: %v", vErr)
	}
	view := views[eventID]
	if view == nil {
		t.Fatalf("Expected view record for %s", eventID)
	}
	view.LastSeenAtUnixSec = staleTime
	view.LastViewedAtUnixSec = staleTime
	if uErr := sqlStorage.Update(ctx, view); uErr != nil {
		t.Fatalf("Failed to backdate view record: %v", uErr)
	}

	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	for _, item := range resp.Msg.Items {
		if item.Id == eventID {
			t.Error("Expected available loan gear (GEAR_SHARED) to be expired after 24h")
		}
	}
}

// TestIsWithinHorizon verifies the absolute age bound that keeps the feed from
// accumulating every item a community has ever produced (#2799).
func TestIsWithinHorizon(t *testing.T) {
	now := int64(1_800_000_000)

	tests := []struct {
		name           string
		lastActivityAt int64
		want           bool
	}{
		{"just now", now, true},
		{"one day old", now - 24*3600, true},
		{"one hour inside the horizon", now - feedHorizonSeconds + 3600, true},
		{"exactly at the horizon", now - feedHorizonSeconds, true},
		{"one second past the horizon", now - feedHorizonSeconds - 1, false},
		{"the reported 26-day-old story", now - 26*24*3600, false},
		{"future activity (clock skew)", now + 3600, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWithinHorizon(tc.lastActivityAt, now); got != tc.want {
				t.Errorf("isWithinHorizon(%d, %d) = %v, want %v",
					tc.lastActivityAt, now, got, tc.want)
			}
		})
	}
}

// TestIsWithinHorizonExceedsUnreadWindow pins the relationship the horizon and
// the unread window must keep: nothing may be unread and beyond the horizon at
// the same time, or the sidebar "New" badge would advertise an item GetFeed no
// longer returns.
func TestIsWithinHorizonExceedsUnreadWindow(t *testing.T) {
	if feedHorizonSeconds < unreadWindowSeconds {
		t.Fatalf("feedHorizonSeconds (%d) must be >= unreadWindowSeconds (%d): "+
			"an item could otherwise be unread but absent from the feed",
			feedHorizonSeconds, unreadWindowSeconds)
	}
}

// TestIsLiveOpportunity verifies which events represent an opportunity that is
// still open, and therefore bypass both the staleness expiry and the horizon.
// The past-dated experience and the overdue request are the cases that changed
// with #2799 — they used to be exempt from expiry forever.
func TestIsLiveOpportunity(t *testing.T) {
	now := int64(1_800_000_000)
	future := now + 7*24*3600
	past := now - 7*24*3600

	giveawayGearID := "gear-giveaway"
	loanGearID := "gear-loan"
	upcomingExpID := "exp-upcoming"
	pastExpID := "exp-past"
	todayExpID := "exp-today"
	tbdExpID := "exp-tbd"
	cancelledExpID := "exp-cancelled"
	wantedReqID := "req-wanted"
	overdueReqID := "req-overdue"
	undatedReqID := "req-undated"
	fulfilledReqID := "req-fulfilled"

	specificTime := func(unix int64) *models.ExperienceTime {
		return &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: unix},
			},
		}
	}
	neededBy := func(unix int64) *int64 { return &unix }

	fc := &feedLookups{
		communityGearMap: map[string]*models.CommunityGear{
			giveawayGearID: {Availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY},
			loanGearID:     {Availability: models.Availability_AVAILABILITY_FOR_LOAN},
		},
		experienceMap: map[string]*models.Experience{
			upcomingExpID: {Time: specificTime(future)},
			pastExpID:     {Time: specificTime(past)},
			// Started this morning; still live thanks to the grace window.
			todayExpID:     {Time: specificTime(now - 6*3600)},
			tbdExpID:       {Time: &models.ExperienceTime{TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}}}},
			cancelledExpID: {Time: specificTime(future), State: models.ExperienceState_EXPERIENCE_STATE_CANCELLED},
		},
		requestMap: map[string]*models.Request{
			wantedReqID:    {NeededByUnixSec: neededBy(future)},
			overdueReqID:   {NeededByUnixSec: neededBy(past)},
			undatedReqID:   {},
			fulfilledReqID: {NeededByUnixSec: neededBy(future), State: models.RequestState_REQUEST_STATE_FULFILLED},
		},
	}

	requestEvent := func(t models.CommunityEventType, id string) *models.CommunityEvent {
		return &models.CommunityEvent{EventType: t, Topic: &models.CommunityEvent_RequestId{RequestId: id}}
	}
	experienceEvent := func(t models.CommunityEventType, id string) *models.CommunityEvent {
		return &models.CommunityEvent{EventType: t, Topic: &models.CommunityEvent_ExperienceId{ExperienceId: id}}
	}

	tests := []struct {
		name  string
		event *models.CommunityEvent
		want  bool
	}{
		// Requests: live only while still wanted by a future due date.
		{"request wanted by a future date", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, wantedReqID), true},
		{"request past its needed-by date", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, overdueReqID), false},
		{"request with no needed-by date", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, undatedReqID), false},
		{"fulfilled request", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, fulfilledReqID), false},
		{"offer made on a wanted request", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, wantedReqID), true},
		{"missing request", requestEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, "nope"), false},

		// Experiences: live only while still ahead of the viewer.
		{"upcoming experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, upcomingExpID), true},
		{"experience happening today", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, todayExpID), true},
		{"experience a week in the past", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, pastExpID), false},
		{"experience with no scheduled time", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, tbdExpID), false},
		{"cancelled upcoming experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, cancelledExpID), false},
		{"RSVP yes on an upcoming experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, upcomingExpID), true},
		{"RSVP maybe on a past experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE, pastExpID), false},
		{"RSVP no on an upcoming experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO, upcomingExpID), true},
		{"missing experience", experienceEvent(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, "nope"), false},

		// Gear: unchanged by #2799.
		{"gear shared giveaway", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, GearId: giveawayGearID}, true},
		{"transfer interest giveaway", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED, GearId: giveawayGearID}, true},
		{"transfer recipient selected giveaway", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED, GearId: giveawayGearID}, true},
		{"transfer active giveaway", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE, GearId: giveawayGearID}, true},
		{"gear shared loan", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, GearId: loanGearID}, false},
		{"transfer interest loan", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED, GearId: loanGearID}, true},
		{"transfer recipient selected loan", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED, GearId: loanGearID}, true},
		{"transfer active loan", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE, GearId: loanGearID}, true},
		{"community created", &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLiveOpportunity(tc.event, fc, now); got != tc.want {
				t.Errorf("isLiveOpportunity(%s) = %v, want %v", tc.event.EventType, got, tc.want)
			}
		})
	}
}
