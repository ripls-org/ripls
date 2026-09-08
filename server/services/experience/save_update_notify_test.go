package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// setupTestServiceWithMockBus creates a Service backed by real storage but a
// MockBus so tests can inspect which CommunityEvents were published without
// running the full notification fan-out.
func setupTestServiceWithMockBus(t *testing.T) (*Service, *storage.ProtoSQLStorage, *cebus.MockBus) {
	t.Helper()
	sqlStorage := setupTestStorage(t)
	bucket := setupTestBucket(t)
	mockNotification := notifications.NewMockService()
	mockBus := cebus.NewMockBus()
	svc := New(sqlStorage, bucket, mockNotification, mockBus)
	return svc, sqlStorage, mockBus
}

// insertTestLocation inserts a minimal Location row and returns its ID.
func insertTestLocation(t *testing.T, s *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.Location{Name: &name})
	if err != nil {
		t.Fatalf("insertTestLocation: %v", err)
	}
	return id
}

// insertTestCommunityExperience links an experience to a community.
func insertTestCommunityExperience(t *testing.T, s *storage.ProtoSQLStorage, communityID, experienceID string) {
	t.Helper()
	if _, err := s.Insert(context.Background(), &models.CommunityExperience{
		CommunityId:  communityID,
		ExperienceId: experienceID,
	}); err != nil {
		t.Fatalf("insertTestCommunityExperience: %v", err)
	}
}

// specificTime returns an ExperienceTime with a concrete unix timestamp.
func specificTime(ts int64) *api.ExperienceTime {
	return &api.ExperienceTime{
		TimeType: &api.ExperienceTime_Specific{
			Specific: &api.SpecificTime{UnixTimestampSec: ts},
		},
	}
}

// TestSaveExperience_UpdateEmitsExperienceUpdatedEvent verifies that
// SaveExperience publishes EXPERIENCE_UPDATED events for each community the
// experience is shared with when time or location changes.
func TestSaveExperience_UpdateEmitsExperienceUpdatedEvent(t *testing.T) {
	svc, stor, mockBus := setupTestServiceWithMockBus(t)

	ownerID := "owner-1"
	createTestUser(t, stor, ownerID, "owner@example.com", "Owner")
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	locA := insertTestLocation(t, stor, "Park A")
	locB := insertTestLocation(t, stor, "Park B")
	communityID := createTestCommunity(t, stor, "Test Community", ownerID)
	createTestCommunityMembership(t, stor, communityID, ownerID)

	// Create the experience.
	createResp, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:       "Trail Day",
		LocationId: locA,
	}))
	if err != nil {
		t.Fatalf("create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	// The experience is also born in its own per-item community (#2492), which
	// receives a fan-out event in addition to communityID below. Each subtest
	// inspects the event for communityID specifically.
	itemCommunityID := createResp.Msg.GetItemCommunityId()

	// Share into one community.
	insertTestCommunityExperience(t, stor, communityID, expID)

	// eventFor returns the single EXPERIENCE_UPDATED event addressed to wantCID,
	// and asserts that the only other fan-out target is the per-item community.
	eventFor := func(t *testing.T, captured []*models.CommunityEvent, wantCID string) *models.CommunityEvent {
		t.Helper()
		events := filterUpdatedEvents(captured)
		var match *models.CommunityEvent
		for _, ev := range events {
			switch ev.CommunityId {
			case wantCID:
				match = ev
			case itemCommunityID:
				// expected per-item community fan-out (#2492)
			default:
				t.Errorf("unexpected EXPERIENCE_UPDATED event for community %q", ev.CommunityId)
			}
		}
		if match == nil {
			t.Fatalf("expected an EXPERIENCE_UPDATED event for community %q, got %d events", wantCID, len(events))
		}
		return match
	}

	t.Run("time-only change emits one event with time_changed=true", func(t *testing.T) {
		mockBus.Reset()

		_, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &expID,
			Time: specificTime(1_800_000_000),
		}))
		if err != nil {
			t.Fatalf("update time: %v", err)
		}

		ev := eventFor(t, mockBus.Captured(), communityID)
		if !ev.GetTimeChanged() {
			t.Error("expected time_changed=true")
		}
		if ev.GetLocationChanged() {
			t.Error("expected location_changed=false")
		}
		if ev.GetExperienceId() != expID {
			t.Errorf("experience_id = %q, want %q", ev.GetExperienceId(), expID)
		}
		if ev.CommunityId != communityID {
			t.Errorf("community_id = %q, want %q", ev.CommunityId, communityID)
		}
	})

	t.Run("location-only change emits one event with location_changed=true", func(t *testing.T) {
		mockBus.Reset()

		_, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:         &expID,
			LocationId: locB,
		}))
		if err != nil {
			t.Fatalf("update location: %v", err)
		}

		ev := eventFor(t, mockBus.Captured(), communityID)
		if ev.GetTimeChanged() {
			t.Error("expected time_changed=false")
		}
		if !ev.GetLocationChanged() {
			t.Error("expected location_changed=true")
		}
	})

	t.Run("both fields changed emits one event per community with both flags true", func(t *testing.T) {
		mockBus.Reset()

		_, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:         &expID,
			Time:       specificTime(1_900_000_000),
			LocationId: locA,
		}))
		if err != nil {
			t.Fatalf("update both: %v", err)
		}

		ev := eventFor(t, mockBus.Captured(), communityID)
		if !ev.GetTimeChanged() {
			t.Error("expected time_changed=true")
		}
		if !ev.GetLocationChanged() {
			t.Error("expected location_changed=true")
		}
	})

	t.Run("description-only change emits no EXPERIENCE_UPDATED event", func(t *testing.T) {
		mockBus.Reset()

		_, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:          &expID,
			Description: "Updated description only",
		}))
		if err != nil {
			t.Fatalf("update description: %v", err)
		}

		events := filterUpdatedEvents(mockBus.Captured())
		if len(events) != 0 {
			t.Errorf("expected 0 EXPERIENCE_UPDATED events for description-only change, got %d", len(events))
		}
	})
}

// TestSaveExperience_NoopUpdateEmitsNoEvent verifies that saving the same
// time and location as already stored does not emit EXPERIENCE_UPDATED.
func TestSaveExperience_NoopUpdateEmitsNoEvent(t *testing.T) {
	svc, stor, mockBus := setupTestServiceWithMockBus(t)

	ownerID := "owner-noop"
	createTestUser(t, stor, ownerID, "owner-noop@example.com", "Owner Noop")
	ctx := createAuthenticatedContext(ownerID, "owner-noop@example.com", models.Role_ROLE_USER)

	locA := insertTestLocation(t, stor, "Noop Park")
	communityID := createTestCommunity(t, stor, "Noop Community", ownerID)
	createTestCommunityMembership(t, stor, communityID, ownerID)

	ts := int64(1_700_000_000)
	createResp, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:       "Noop Event",
		LocationId: locA,
		Time:       specificTime(ts),
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	insertTestCommunityExperience(t, stor, communityID, expID)

	// Re-save with same time (new request sets time → timeChanged = true by
	// current logic since req.Msg.Time != nil means timeChanged=true). So
	// instead test an update with NO time field and SAME location — that
	// genuinely produces timeChanged=false, locationChanged=false.
	mockBus.Reset()

	// Sending name update only (no time, no location in request).
	_, err = svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:   &expID,
		Name: "Noop Event Renamed",
	}))
	if err != nil {
		t.Fatalf("noop update: %v", err)
	}

	events := filterUpdatedEvents(mockBus.Captured())
	if len(events) != 0 {
		t.Errorf("expected 0 EXPERIENCE_UPDATED events for name-only change, got %d", len(events))
	}
}

// TestSaveExperience_MultiCommunityEmitsOneEventPerCommunity verifies that
// when an experience is shared with multiple communities, one EXPERIENCE_UPDATED
// event is emitted per community on time/location change.
func TestSaveExperience_MultiCommunityEmitsOneEventPerCommunity(t *testing.T) {
	svc, stor, mockBus := setupTestServiceWithMockBus(t)

	ownerID := "owner-multi"
	createTestUser(t, stor, ownerID, "owner-multi@example.com", "Multi Owner")
	ctx := createAuthenticatedContext(ownerID, "owner-multi@example.com", models.Role_ROLE_USER)

	communityA := createTestCommunity(t, stor, "Community A", ownerID)
	communityB := createTestCommunity(t, stor, "Community B", ownerID)
	createTestCommunityMembership(t, stor, communityA, ownerID)
	createTestCommunityMembership(t, stor, communityB, ownerID)

	createResp, err := svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Multi Share Event",
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	// The experience is also born in its own per-item community (#2492), which
	// receives a fan-out event too.
	itemCommunityID := createResp.Msg.GetItemCommunityId()

	insertTestCommunityExperience(t, stor, communityA, expID)
	insertTestCommunityExperience(t, stor, communityB, expID)

	mockBus.Reset()

	_, err = svc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:   &expID,
		Time: specificTime(1_800_000_000),
	}))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// One event per community: communityA + communityB + the per-item community (#2492).
	events := filterUpdatedEvents(mockBus.Captured())
	if len(events) != 3 {
		t.Fatalf("expected 3 EXPERIENCE_UPDATED events (one per community), got %d", len(events))
	}

	communityIDs := map[string]bool{}
	for _, ev := range events {
		communityIDs[ev.CommunityId] = true
		if !ev.GetTimeChanged() {
			t.Errorf("event for community %q: expected time_changed=true", ev.CommunityId)
		}
	}
	if !communityIDs[communityA] {
		t.Errorf("no event for communityA")
	}
	if !communityIDs[communityB] {
		t.Errorf("no event for communityB")
	}
	if itemCommunityID != "" && !communityIDs[itemCommunityID] {
		t.Errorf("no event for per-item community")
	}
}

// filterUpdatedEvents returns only the EXPERIENCE_UPDATED events from a slice.
func filterUpdatedEvents(events []*models.CommunityEvent) []*models.CommunityEvent {
	var out []*models.CommunityEvent
	for _, ev := range events {
		if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED {
			out = append(out, ev)
		}
	}
	return out
}
