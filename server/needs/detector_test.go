package needs

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestDetectThisWeekNeed_NoCommunitiesReturnsNil(t *testing.T) {
	t.Parallel()
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	got, err := DetectThisWeekNeed(context.Background(), store, nil, time.Now())
	if err != nil {
		t.Fatalf("DetectThisWeekNeed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil detection for empty community list, got %+v", got)
	}
}

func TestDetectThisWeekNeed_FiresOnUnderclaimedActiveRequest(t *testing.T) {
	t.Parallel()
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Active, under-claimed Request joined to comm-1.
	req := &models.Request{
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		Title:            "Airport ride Friday",
		Description:      "Need a 6am ride Friday",
		CreatedAtUnixSec: time.Now().Unix(),
	}
	reqID, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	join := &models.CommunityRequest{CommunityId: "comm-1", RequestId: reqID}
	if _, err := store.Insert(ctx, join); err != nil {
		t.Fatalf("insert join: %v", err)
	}

	got, err := DetectThisWeekNeed(ctx, store, []string{"comm-1"}, time.Now())
	if err != nil {
		t.Fatalf("DetectThisWeekNeed: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection, got nil")
	}
	if got.Request.Id != reqID {
		t.Errorf("Request.Id: want %q, got %q", reqID, got.Request.Id)
	}
	if got.CommunityID != "comm-1" {
		t.Errorf("CommunityID: want comm-1, got %q", got.CommunityID)
	}
}

func TestDetectThisWeekNeed_SkipsRequestsWithOffers(t *testing.T) {
	t.Parallel()
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	req := &models.Request{
		State:            models.RequestState_REQUEST_STATE_ACTIVE,
		Title:            "Already claimed",
		CreatedAtUnixSec: time.Now().Unix(),
	}
	reqID, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityRequest{
		CommunityId: "comm-1", RequestId: reqID,
	}); err != nil {
		t.Fatalf("insert join: %v", err)
	}
	if _, err := store.Insert(ctx, &models.RequestOffer{
		RequestId: reqID, UserId: "user-2", CommunityId: "comm-1",
	}); err != nil {
		t.Fatalf("insert offer: %v", err)
	}

	got, err := DetectThisWeekNeed(ctx, store, []string{"comm-1"}, time.Now())
	if err != nil {
		t.Fatalf("DetectThisWeekNeed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (already claimed), got %+v", got)
	}
}

func TestDetectThisWeekNeed_SkipsFulfilledRequests(t *testing.T) {
	t.Parallel()
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	req := &models.Request{
		State:            models.RequestState_REQUEST_STATE_FULFILLED,
		Title:            "Fulfilled",
		CreatedAtUnixSec: time.Now().Unix(),
	}
	reqID, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityRequest{
		CommunityId: "comm-1", RequestId: reqID,
	}); err != nil {
		t.Fatalf("insert join: %v", err)
	}

	got, err := DetectThisWeekNeed(ctx, store, []string{"comm-1"}, time.Now())
	if err != nil {
		t.Fatalf("DetectThisWeekNeed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (fulfilled), got %+v", got)
	}
}
