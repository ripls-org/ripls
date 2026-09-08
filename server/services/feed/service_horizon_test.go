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

// insertStory writes a story to a community at a given age.
func insertStory(t *testing.T, s *storage.ProtoSQLStorage, communityID, title string, createdAt int64) string {
	t.Helper()
	story := &models.Story{
		CommunityId:      communityID,
		Title:            title,
		Description:      "Something happened.",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		MediaIds:         []string{"media-1"},
		CreatedAtUnixSec: createdAt,
	}
	id, err := s.Insert(context.Background(), story)
	if err != nil {
		t.Fatalf("Failed to insert story %q: %v", title, err)
	}
	return id
}

// feedItemIDs collects the item IDs returned by a GetFeed response.
func feedItemIDs(items []*api.FeedItem) map[string]bool {
	ids := make(map[string]bool, len(items))
	for _, item := range items {
		ids[item.Id] = true
	}
	return ids
}

// TestGetFeed_HorizonDropsStaleItemsKeepsLiveOnes is the #2799 regression: an
// old story the viewer never saw must fall off the feed, while an event posted
// even longer ago but still ahead of the viewer must stay. Nothing here is ever
// marked viewed — that is the point. On the Home pulse nothing ever is, so the
// seen-based expiry never fires and the horizon is the only bound.
func TestGetFeed_HorizonDropsStaleItemsKeepsLiveOnes(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "horizon@test.com", "Horizon User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Horizon Community")

	now := time.Now().Unix()
	day := int64(24 * 3600)

	// The reported case: stories from ~26 days ago, never seen.
	staleStoryID := insertStory(t, sqlStorage, communityID, "Doug Left Amazon", now-26*day)
	// A story from inside the window stays.
	freshStoryID := insertStory(t, sqlStorage, communityID, "Bolivia Viewing Party", now-8*day)

	// An event posted 40 days ago that happens tomorrow. Its creation event is
	// far outside the horizon, so only the liveness test can save it — and it
	// must, or the fix would delete upcoming plans from the Home feed.
	upcomingExpID := setupTestExperienceAt(t, sqlStorage, userID, "Love Your Neighbor 5K", now+day)
	setupCommunityExperience(t, sqlStorage, communityID, upcomingExpID, false)
	upcomingEventID, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:           userID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: upcomingExpID},
		OccurredAtUnixSec: now - 40*day,
	})
	if err != nil {
		t.Fatalf("Insert upcoming experience event failed: %v", err)
	}

	// An event that already happened and was never wrapped up. Today it keeps
	// advertising itself — and offering a Join button — forever.
	pastExpID := setupTestExperienceAt(t, sqlStorage, userID, "Talent Show", now-25*day)
	setupCommunityExperience(t, sqlStorage, communityID, pastExpID, false)
	pastEventID, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:           userID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: pastExpID},
		OccurredAtUnixSec: now - 30*day,
	})
	if err != nil {
		t.Fatalf("Insert past experience event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "horizon@test.com", models.Role_ROLE_USER)
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	got := feedItemIDs(resp.Msg.Items)

	if got[staleStoryID] {
		t.Error("26-day-old story should have fallen past the horizon")
	}
	if !got[freshStoryID] {
		t.Error("8-day-old story should still be within the horizon")
	}
	if !got[upcomingEventID] {
		t.Error("event happening tomorrow should survive regardless of when it was posted")
	}
	if got[pastEventID] {
		t.Error("event that already happened should no longer be a live opportunity")
	}
}

// TestGetFeed_HorizonKeepsOpenGiveaway verifies the horizon does not sweep away
// a giveaway that is still on offer, however long it has been listed.
func TestGetFeed_HorizonKeepsOpenGiveaway(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "giveaway@test.com", "Giveaway User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Giveaway Community")

	now := time.Now().Unix()
	old := now - 40*24*3600

	giveawayGearID := setupTestGear(t, sqlStorage, userID, "Spatulas & Scoops")
	setupCommunityGear(t, sqlStorage, communityID, giveawayGearID, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	giveawayEventID, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            giveawayGearID,
		OccurredAtUnixSec: old,
	})
	if err != nil {
		t.Fatalf("Insert giveaway event failed: %v", err)
	}

	// A loan listing with no open transfer is not a live opportunity, so the
	// same age drops it.
	loanGearID := setupTestGear(t, sqlStorage, userID, "Extension Ladder")
	setupCommunityGear(t, sqlStorage, communityID, loanGearID, models.Availability_AVAILABILITY_FOR_LOAN)
	loanEventID, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            loanGearID,
		OccurredAtUnixSec: old,
	})
	if err != nil {
		t.Fatalf("Insert loan event failed: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "giveaway@test.com", models.Role_ROLE_USER)
	resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	}))
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	got := feedItemIDs(resp.Msg.Items)

	if !got[giveawayEventID] {
		t.Error("open giveaway should stay in the feed past the horizon")
	}
	if got[loanEventID] {
		t.Error("idle loan listing should fall past the horizon")
	}
}

// TestGetFeed_ViewerRSVPIsScopedToViewer is the #2800 regression, and the
// leakage guard. Two members RSVP differently to the same event; each must see
// their own answer and never the other's. A single-member fixture would pass
// even if the map were keyed wrong, which is the whole failure mode.
func TestGetFeed_ViewerRSVPIsScopedToViewer(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	hostID := setupTestUser(t, sqlStorage, "host@test.com", "Leslie Hall")
	goingID := setupTestUser(t, sqlStorage, "going@test.com", "Bryan Going")
	maybeID := setupTestUser(t, sqlStorage, "maybe@test.com", "Marc Maybe")
	silentID := setupTestUser(t, sqlStorage, "silent@test.com", "Susan Silent")

	communityID := setupTestCommunity(t, sqlStorage, hostID, "RSVP Community")
	for _, id := range []string{goingID, maybeID, silentID} {
		addCommunityMember(t, sqlStorage, communityID, id)
	}

	expID := setupTestExperience(t, sqlStorage, hostID, "Love Your Neighbor 5K")
	setupCommunityExperience(t, sqlStorage, communityID, expID, false)
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:           hostID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: expID},
		OccurredAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("Insert experience event failed: %v", err)
	}

	for _, rsvp := range []struct {
		userID    string
		intention models.RSVPIntention
	}{
		{goingID, models.RSVPIntention_RSVP_INTENTION_YES},
		{maybeID, models.RSVPIntention_RSVP_INTENTION_MAYBE},
	} {
		if _, err := sqlStorage.Insert(context.Background(), &models.ExperienceRSVP{
			ExperienceId:    expID,
			UserId:          rsvp.userID,
			CommunityId:     communityID,
			Intention:       rsvp.intention,
			RsvpedAtUnixSec: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("Insert RSVP failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		userID    string
		email     string
		wantSet   bool
		wantValue api.FeedRSVPIntention
	}{
		{"viewer who said yes", goingID, "going@test.com", true, api.FeedRSVPIntention_FEED_RSVP_INTENTION_YES},
		{"viewer who said maybe", maybeID, "maybe@test.com", true, api.FeedRSVPIntention_FEED_RSVP_INTENTION_MAYBE},
		{"viewer who never responded", silentID, "silent@test.com", false, api.FeedRSVPIntention_FEED_RSVP_INTENTION_UNSPECIFIED},
		{"host who never responded", hostID, "host@test.com", false, api.FeedRSVPIntention_FEED_RSVP_INTENTION_UNSPECIFIED},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := createAuthenticatedContext(tc.userID, tc.email, models.Role_ROLE_USER)
			resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
				CommunityIds: []string{communityID},
				PageSize:     20,
			}))
			if err != nil {
				t.Fatalf("GetFeed failed: %v", err)
			}

			var payload *api.ExperienceCreatedPayload
			for _, item := range resp.Msg.Items {
				if p := item.GetExperienceCreated(); p != nil {
					payload = p
				}
			}
			if payload == nil {
				t.Fatal("Expected an experience card in the feed")
			}

			if got := payload.ViewerRsvp != nil; got != tc.wantSet {
				t.Fatalf("viewer_rsvp presence = %v, want %v", got, tc.wantSet)
			}
			if tc.wantSet && payload.GetViewerRsvp() != tc.wantValue {
				t.Errorf("viewer_rsvp = %v, want %v", payload.GetViewerRsvp(), tc.wantValue)
			}
			// The counts stay aggregate — two people answered regardless of
			// who is looking.
			if payload.YesCount != 1 || payload.MaybeCount != 1 {
				t.Errorf("counts = yes %d / maybe %d, want 1 / 1", payload.YesCount, payload.MaybeCount)
			}
		})
	}
}

// TestGetFeed_ViewerHasOfferedIsScopedToViewer is the request-card counterpart:
// one member's offer must not make the card read as answered for anybody else,
// and a withdrawn offer must not count as an offer at all.
func TestGetFeed_ViewerHasOfferedIsScopedToViewer(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	requesterID := setupTestUser(t, sqlStorage, "requester@test.com", "Gary Soto")
	offererID := setupTestUser(t, sqlStorage, "offerer@test.com", "Carmen Ruiz")
	withdrawnID := setupTestUser(t, sqlStorage, "withdrawn@test.com", "Wendy Withdrawn")
	bystanderID := setupTestUser(t, sqlStorage, "bystander@test.com", "Betty Bystander")

	communityID := setupTestCommunity(t, sqlStorage, requesterID, "Offer Community")
	for _, id := range []string{offererID, withdrawnID, bystanderID} {
		addCommunityMember(t, sqlStorage, communityID, id)
	}

	requestID := setupTestRequest(t, sqlStorage, requesterID, communityID,
		"Need a stand mixer", models.RequestState_REQUEST_STATE_ACTIVE)
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		ActorId:           requesterID,
		Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
		OccurredAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("Insert request event failed: %v", err)
	}

	for _, offer := range []struct {
		userID    string
		withdrawn bool
	}{{offererID, false}, {withdrawnID, true}} {
		if _, err := sqlStorage.Insert(context.Background(), &models.RequestOffer{
			RequestId:        requestID,
			UserId:           offer.userID,
			CommunityId:      communityID,
			Withdrawn:        offer.withdrawn,
			CreatedAtUnixSec: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("Insert offer failed: %v", err)
		}
	}

	tests := []struct {
		name           string
		userID         string
		email          string
		wantHasOffered bool
		wantCanEdit    bool
	}{
		{"viewer who offered", offererID, "offerer@test.com", true, false},
		{"viewer who withdrew", withdrawnID, "withdrawn@test.com", false, false},
		{"viewer who never offered", bystanderID, "bystander@test.com", false, false},
		{"the requester", requesterID, "requester@test.com", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := createAuthenticatedContext(tc.userID, tc.email, models.Role_ROLE_USER)
			resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
				CommunityIds: []string{communityID},
				PageSize:     20,
			}))
			if err != nil {
				t.Fatalf("GetFeed failed: %v", err)
			}

			var payload *api.RequestCreatedPayload
			for _, item := range resp.Msg.Items {
				if p := item.GetRequestCreated(); p != nil {
					payload = p
				}
			}
			if payload == nil {
				t.Fatal("Expected a request card in the feed")
			}

			if payload.ViewerHasOffered == nil {
				t.Fatal("viewer_has_offered should be set when the offer fetch succeeded")
			}
			if got := payload.GetViewerHasOffered(); got != tc.wantHasOffered {
				t.Errorf("viewer_has_offered = %v, want %v", got, tc.wantHasOffered)
			}
			if got := payload.CanEdit; got != tc.wantCanEdit {
				t.Errorf("can_edit = %v, want %v", got, tc.wantCanEdit)
			}
			// Only the live offer counts.
			if payload.OfferCount != 1 {
				t.Errorf("offer_count = %d, want 1", payload.OfferCount)
			}
		})
	}
}

// TestGetFeed_ViewerStateAddsNoQueries pins the claim that viewer-scoped
// participation rides along on batch fetches the feed already performs. If
// someone later resolves it per item, this fails rather than quietly turning
// the Home screen into an N+1.
func TestGetFeed_ViewerStateAddsNoQueries(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	hostID := setupTestUser(t, sqlStorage, "queries@test.com", "Query Host")
	viewerID := setupTestUser(t, sqlStorage, "queryviewer@test.com", "Query Viewer")
	communityID := setupTestCommunity(t, sqlStorage, hostID, "Query Community")
	addCommunityMember(t, sqlStorage, communityID, viewerID)

	// Several events and requests, so a per-item lookup would show up clearly
	// as a count that scales with the fixture.
	for i := range 4 {
		expID := setupTestExperience(t, sqlStorage, hostID, "Event")
		setupCommunityExperience(t, sqlStorage, communityID, expID, false)
		if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
			CommunityId:       communityID,
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
			ActorId:           hostID,
			Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: expID},
			OccurredAtUnixSec: time.Now().Unix() - int64(i),
		}); err != nil {
			t.Fatalf("Insert experience event failed: %v", err)
		}
		if _, err := sqlStorage.Insert(context.Background(), &models.ExperienceRSVP{
			ExperienceId:    expID,
			UserId:          viewerID,
			CommunityId:     communityID,
			Intention:       models.RSVPIntention_RSVP_INTENTION_YES,
			RsvpedAtUnixSec: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("Insert RSVP failed: %v", err)
		}

		requestID := setupTestRequest(t, sqlStorage, hostID, communityID,
			"Need something", models.RequestState_REQUEST_STATE_ACTIVE)
		if _, err := sqlStorage.Insert(context.Background(), &models.CommunityEvent{
			CommunityId:       communityID,
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			ActorId:           hostID,
			Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
			OccurredAtUnixSec: time.Now().Unix() - int64(i),
		}); err != nil {
			t.Fatalf("Insert request event failed: %v", err)
		}
		if _, err := sqlStorage.Insert(context.Background(), &models.RequestOffer{
			RequestId:        requestID,
			UserId:           viewerID,
			CommunityId:      communityID,
			CreatedAtUnixSec: time.Now().Unix(),
		}); err != nil {
			t.Fatalf("Insert offer failed: %v", err)
		}
	}

	ctx := storage.WithQueryStats(
		createAuthenticatedContext(viewerID, "queryviewer@test.com", models.Role_ROLE_USER),
	)

	// The batch-fetch pipeline is a fixed set of queries per community; the
	// bound is deliberately generous, since the point is that it does not grow
	// with the number of items.
	const maxQueries = 30
	storage.AssertMaxQueries(t, ctx, maxQueries, func() {
		resp, err := service.GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
			CommunityIds: []string{communityID},
			PageSize:     20,
		}))
		if err != nil {
			t.Fatalf("GetFeed failed: %v", err)
		}
		if len(resp.Msg.Items) != 8 {
			t.Fatalf("Expected 8 items, got %d", len(resp.Msg.Items))
		}
		for _, item := range resp.Msg.Items {
			if p := item.GetExperienceCreated(); p != nil &&
				p.GetViewerRsvp() != api.FeedRSVPIntention_FEED_RSVP_INTENTION_YES {
				t.Errorf("expected viewer_rsvp YES on %s", item.Id)
			}
			if p := item.GetRequestCreated(); p != nil && !p.GetViewerHasOffered() {
				t.Errorf("expected viewer_has_offered on %s", item.Id)
			}
		}
	})
}
