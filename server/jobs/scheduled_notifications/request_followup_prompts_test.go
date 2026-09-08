package scheduled_notifications

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fakeRequestWorld implements RequestWorld for testing.
type fakeRequestWorld struct {
	requests       []*models.Request
	requestsByID   map[string]*models.Request
	communities    map[string]*models.Community
	primaryCommIDs map[string]string // requestID → communityID
	timezones      map[string]string // userID → IANA tz
	prefs          map[string]*models.CommunityNotificationPreferences
	prefsErr       error
	listErr        error
	getErr         error
	batchErr       error
}

func (f *fakeRequestWorld) ListOpenRequests(_ context.Context) ([]*models.Request, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.requests, nil
}

func (f *fakeRequestWorld) GetRequest(_ context.Context, id string) (*models.Request, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if r, ok := f.requestsByID[id]; ok {
		return r, nil
	}
	return nil, storage.ErrRecordNotFound
}

func (f *fakeRequestWorld) GetPrimaryCommunityIDsForRequests(_ context.Context, ids []string) (map[string]string, error) {
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if cid, ok := f.primaryCommIDs[id]; ok {
			out[id] = cid
		}
	}
	return out, nil
}

func (f *fakeRequestWorld) GetCommunity(_ context.Context, id string) (*models.Community, error) {
	if c, ok := f.communities[id]; ok {
		return c, nil
	}
	return nil, storage.ErrRecordNotFound
}

func (f *fakeRequestWorld) GetUserTimezones(_ context.Context, userIDs []string) map[string]string {
	out := make(map[string]string, len(userIDs))
	for _, id := range userIDs {
		out[id] = f.timezones[id]
	}
	return out
}

func (f *fakeRequestWorld) GetCommunityNotificationPreferences(_ context.Context, userID, _ string) (*models.CommunityNotificationPreferences, error) {
	if f.prefsErr != nil {
		return nil, f.prefsErr
	}
	return f.prefs[userID], nil
}

func ptr[T any](v T) *T { return &v }

// newOpenRequest builds a minimal open Request for tests.
func newOpenRequest(id, requesterID string, createdAt int64) *models.Request {
	return &models.Request{
		Id:               id,
		RequesterId:      requesterID,
		Title:            "Need a ladder",
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		CreatedAtUnixSec: createdAt,
	}
}

func TestRequestFollowupReconcilerDesired(t *testing.T) {
	now := time.Now().Unix()

	t.Run("emits five offsets for a future request", func(t *testing.T) {
		req := newOpenRequest("r-1", "u-requester", now)
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{"r-1": "c-1"},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("want 5 rows, got %d", len(got))
		}
		for i, row := range got {
			r := row.GetRequest()
			if r == nil {
				t.Fatalf("row[%d] has no request variant", i)
			}
			if r.RequestId != "r-1" {
				t.Errorf("row[%d] RequestId = %q, want r-1", i, r.RequestId)
			}
			wantOffset := requestFollowupOffsets[i]
			if r.OffsetSecondsFromAnchor != wantOffset {
				t.Errorf("row[%d] offset = %d, want %d", i, r.OffsetSecondsFromAnchor, wantOffset)
			}
			wantFireAt := now + wantOffset
			if row.FireAtUnixSec != wantFireAt {
				t.Errorf("row[%d] fire_at = %d, want %d", i, row.FireAtUnixSec, wantFireAt)
			}
			if row.GetCommunityId() != "c-1" {
				t.Errorf("row[%d] community_id = %q, want c-1", i, row.GetCommunityId())
			}
		}
	})

	t.Run("drops past-grace tuples", func(t *testing.T) {
		// Created 5 days ago: day3 and day6 offsets, but day3's fire_at
		// is 2 days in the past (past DesiredFireAtGrace), so only day6+
		// should appear.
		createdAt := now - 5*24*3600
		req := newOpenRequest("r-2", "u-req", createdAt)
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		// day3 fire_at = createdAt+3d = now-2d → past grace; day6=now+1d, day9=now+4d, day12=now+7d, day15=now+10d
		if len(got) != 4 {
			t.Errorf("want 4 rows (day6..day15), got %d", len(got))
		}
	})

	t.Run("skips requests past backlog-grace window", func(t *testing.T) {
		// Created 17 days ago: all offsets (max=15d) are more than 15d+24h past.
		createdAt := now - 17*24*3600
		req := newOpenRequest("r-old", "u-req", createdAt)
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("want 0 rows past backlog-grace, got %d", len(got))
		}
	})

	t.Run("skips soft-deleted requests", func(t *testing.T) {
		req := newOpenRequest("r-del", "u-req", now)
		req.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now - 60}
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("want 0 rows for deleted request, got %d", len(got))
		}
	})

	t.Run("skips requests with zero created_at", func(t *testing.T) {
		req := &models.Request{Id: "r-notime", RequesterId: "u-req", State: models.RequestState_REQUEST_STATE_ACTIVE}
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("want 0 rows for request with no created_at, got %d", len(got))
		}
	})

	t.Run("multiple communities → one row per offset (not per community)", func(t *testing.T) {
		req := newOpenRequest("r-mc", "u-req", now)
		// Only one communityID is returned for this request (the primary).
		world := &fakeRequestWorld{
			requests:       []*models.Request{req},
			primaryCommIDs: map[string]string{"r-mc": "c-primary"},
			timezones:      map[string]string{},
		}
		rec := &RequestFollowupReconciler{world: world, existing: func(_ context.Context) ([]*models.ScheduledNotification, error) { return nil, nil }}
		ctx := clock.WithSimulationTime(context.Background(), time.Unix(now, 0))
		got, err := rec.Desired(ctx)
		if err != nil {
			t.Fatalf("Desired: %v", err)
		}
		if len(got) != 5 {
			t.Errorf("want 5 rows (one per offset), got %d", len(got))
		}
		for _, row := range got {
			if row.GetCommunityId() != "c-primary" {
				t.Errorf("community_id = %q, want c-primary", row.GetCommunityId())
			}
		}
	})
}

func TestRequestFollowupDispatcherRender(t *testing.T) {
	now := time.Now().Unix()
	ctx := context.Background()

	activeReq := &models.Request{
		Id:               "r-1",
		RequesterId:      "u-req",
		Title:            "Need a ladder",
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		CreatedAtUnixSec: now - 4*24*3600,
	}
	community := &models.Community{Id: "c-1"}

	makeRow := func(communityID string) *models.ScheduledNotification {
		row := &models.ScheduledNotification{
			RecipientUserId: "u-req",
			FireAtUnixSec:   now,
			Item: &models.ScheduledNotification_Request{
				Request: &models.RequestNotification{
					RequestId:               "r-1",
					Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
					OffsetSecondsFromAnchor: OffsetRequestFollowupDay3,
				},
			},
		}
		if communityID != "" {
			row.CommunityId = &communityID
		}
		return row
	}

	t.Run("renders push for open request", func(t *testing.T) {
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": activeReq},
			communities:  map[string]*models.Community{"c-1": community},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif == nil {
			t.Fatal("expected non-nil notification")
		}
		ce := notif.GetCommunityEvent()
		if ce == nil {
			t.Fatal("expected CommunityEvent payload")
		}
		if ce.EventType != "REQUEST_FOLLOWUP_PROMPT" {
			t.Errorf("EventType = %q, want REQUEST_FOLLOWUP_PROMPT", ce.EventType)
		}
		if ce.GetRequestId() != "r-1" {
			t.Errorf("request_id = %q, want r-1", ce.GetRequestId())
		}
	})

	t.Run("skips fulfilled request", func(t *testing.T) {
		fulfilled := &models.Request{
			Id:    "r-1",
			State: models.RequestState_REQUEST_STATE_FULFILLED,
		}
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": fulfilled},
			communities:  map[string]*models.Community{"c-1": community},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif != nil {
			t.Error("expected nil notification for fulfilled request")
		}
	})

	t.Run("skips soft-deleted request", func(t *testing.T) {
		deleted := &models.Request{
			Id:      "r-1",
			State:   models.RequestState_REQUEST_STATE_ACTIVE,
			Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - 60},
		}
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": deleted},
			communities:  map[string]*models.Community{"c-1": community},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif != nil {
			t.Error("expected nil notification for deleted request")
		}
	})

	t.Run("skips when preference is explicitly off", func(t *testing.T) {
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": activeReq},
			communities:  map[string]*models.Community{"c-1": community},
			prefs: map[string]*models.CommunityNotificationPreferences{
				"u-req": {NotifyRequestFollowupPrompts: ptr(false)},
			},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif != nil {
			t.Error("expected nil notification when preference is off")
		}
	})

	t.Run("fails open on preference lookup error", func(t *testing.T) {
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": activeReq},
			communities:  map[string]*models.Community{"c-1": community},
			prefsErr:     fmt.Errorf("db error"),
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif == nil {
			t.Error("expected non-nil notification when pref lookup fails (fail open)")
		}
	})

	t.Run("delivers when preference is unset (default on)", func(t *testing.T) {
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": activeReq},
			communities:  map[string]*models.Community{"c-1": community},
			prefs:        map[string]*models.CommunityNotificationPreferences{"u-req": {}},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif == nil {
			t.Error("expected non-nil notification when preference is unset (default on)")
		}
	})

	t.Run("skips soft-deleted community", func(t *testing.T) {
		deletedCommunity := &models.Community{
			Id:      "c-1",
			Deleted: &models.DeletedMetadata{DeletedAtUnixSec: now - 60},
		}
		world := &fakeRequestWorld{
			requestsByID: map[string]*models.Request{"r-1": activeReq},
			communities:  map[string]*models.Community{"c-1": deletedCommunity},
		}
		d := &RequestFollowupDispatcher{world: world}
		notif, err := d.Render(ctx, makeRow("c-1"))
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif != nil {
			t.Error("expected nil notification for deleted community")
		}
	})
}

func TestRequestFollowupN1Guard(t *testing.T) {
	// Verifies that GetPrimaryCommunityIDsForRequests is called once
	// regardless of the number of open requests — the N+1 invariant.
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	now := time.Now().Unix()

	// Insert several open requests.
	for i := 0; i < 3; i++ {
		r := &models.Request{
			RequesterId:      "u-req",
			Title:            "Test",
			State:            models.RequestState_REQUEST_STATE_ACTIVE,
			CreatedAtUnixSec: now,
		}
		if _, err := s.Insert(ctx, r); err != nil {
			t.Fatalf("insert request %d: %v", i, err)
		}
	}

	world := &storageRequestWorld{storage: s}
	rec := &RequestFollowupReconciler{
		world:    world,
		existing: s.FindRequestScheduledNotifications,
	}

	ctxWithStats := storage.WithQueryStats(ctx)
	clockCtx := clock.WithSimulationTime(ctxWithStats, time.Unix(now, 0))

	// 4 queries max: 2 for ListOpenRequests (ACTIVE + OFFERS_RECEIVED),
	// 1 for GetPrimaryCommunityIDsForRequests batch, 1 for user timezones.
	// This bound must not grow with the number of open requests.
	storage.AssertMaxQueries(t, ctxWithStats, 4, func() {
		_, _ = rec.Desired(clockCtx)
	})
}

// TestRequestFollowupDispatcher_Render covers the copy and the payload the
// off-app senders re-render from. The dispatcher had no Render test before
// #2896, which is part of why its copy could reach SMS as
// "Ripls:  posted an update in ".
func TestRequestFollowupDispatcher_Render(t *testing.T) {
	req := &models.Request{
		Id:          "r-1",
		Title:       "Folding Table",
		RequesterId: "u-1",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	world := &fakeRequestWorld{requestsByID: map[string]*models.Request{"r-1": req}}
	d := &RequestFollowupDispatcher{world: world}

	row := &models.ScheduledNotification{
		Id:              "id-1",
		RecipientUserId: "u-1",
		Item: &models.ScheduledNotification_Request{
			Request: &models.RequestNotification{
				RequestId:               "r-1",
				Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
				OffsetSecondsFromAnchor: OffsetRequestFollowupDay3,
			},
		},
	}

	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif == nil {
		t.Fatal("Render returned nil notification")
	}
	if notif.Title != "Still need this?" {
		t.Errorf("Title = %q, want %q", notif.Title, "Still need this?")
	}
	// The copy no longer names a day count: that number lives only in the row's
	// offset, and the payload has no field for it, so the off-app renderers
	// could not have reproduced the sentence.
	want := "You asked for Folding Table — did your community come through?"
	if notif.Body != want {
		t.Errorf("Body = %q, want %q", notif.Body, want)
	}

	payload := notif.GetCommunityEvent()
	if payload == nil {
		t.Fatal("notification missing CommunityEventPayload")
	}
	// Never rename: fcm_service.dart matches this string verbatim to deep-link
	// into the request's fulfillment UI.
	if payload.EventType != "REQUEST_FOLLOWUP_PROMPT" {
		t.Errorf("EventType = %q, want REQUEST_FOLLOWUP_PROMPT", payload.EventType)
	}
	if payload.RequestTitle != "Folding Table" {
		t.Errorf("payload RequestTitle = %q, want Folding Table", payload.RequestTitle)
	}
	if payload.GetRequestId() != "r-1" {
		t.Errorf("payload RequestId = %q, want r-1", payload.GetRequestId())
	}
}
