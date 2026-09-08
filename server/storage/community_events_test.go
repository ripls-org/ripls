package storage

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// insertEvent adds a CommunityEvent row and returns its id.
func insertEvent(t *testing.T, s *ProtoSQLStorage, communityID string, occurredAt int64) string {
	t.Helper()
	id := uuid.New().String()
	event := &models.CommunityEvent{
		Id:                id,
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		OccurredAtUnixSec: occurredAt,
	}
	if _, err := s.Insert(context.Background(), event); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	return id
}

func TestFindEventsForUserSince(t *testing.T) {
	ctx := context.Background()
	s, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	memberID := uuid.New().String()
	strangerID := uuid.New().String()

	// Two communities the member belongs to, one they do not.
	var joined []string
	for range 2 {
		communityID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.Community{
			Id:          communityID,
			Name:        "Joined",
			CreatorId:   memberID,
			OwnerUserId: memberID,
		}); err != nil {
			t.Fatalf("insert community: %v", err)
		}
		if _, err := s.Insert(ctx, &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      memberID,
		}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
		joined = append(joined, communityID)
	}

	foreignID := uuid.New().String()
	if _, err := s.Insert(ctx, &models.Community{
		Id:          foreignID,
		Name:        "Someone else's",
		CreatorId:   strangerID,
		OwnerUserId: strangerID,
	}); err != nil {
		t.Fatalf("insert foreign community: %v", err)
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: foreignID,
		UserId:      strangerID,
	}); err != nil {
		t.Fatalf("insert foreign membership: %v", err)
	}

	insertEvent(t, s, joined[0], 1000)
	insertEvent(t, s, joined[1], 2000)
	insertEvent(t, s, joined[0], 3000)
	foreignEventID := insertEvent(t, s, foreignID, 2500)

	t.Run("spans every community the user belongs to", func(t *testing.T) {
		events, err := s.FindEventsForUserSince(ctx, memberID, 0, 100)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		if len(events) != 3 {
			t.Fatalf("expected 3 events across both joined communities, got %d", len(events))
		}
		for _, event := range events {
			if event.Id == foreignEventID {
				t.Fatal("returned an event from a community the user does not belong to")
			}
		}
	})

	t.Run("since is exclusive", func(t *testing.T) {
		events, err := s.FindEventsForUserSince(ctx, memberID, 2000, 100)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected only the event after 2000, got %d", len(events))
		}
		if events[0].OccurredAtUnixSec != 3000 {
			t.Errorf("expected the 3000 event, got %d", events[0].OccurredAtUnixSec)
		}
	})

	t.Run("ordered oldest first", func(t *testing.T) {
		events, err := s.FindEventsForUserSince(ctx, memberID, 0, 100)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		for i := 1; i < len(events); i++ {
			if events[i].OccurredAtUnixSec < events[i-1].OccurredAtUnixSec {
				t.Fatalf("events out of order at %d", i)
			}
		}
	})

	t.Run("limit truncates to the oldest, keeping the page resumable", func(t *testing.T) {
		events, err := s.FindEventsForUserSince(ctx, memberID, 0, 2)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected the limit to be honored, got %d", len(events))
		}
		// Dropping the NEWEST rather than the oldest is what lets the caller's
		// next call continue from where this one stopped.
		if events[0].OccurredAtUnixSec != 1000 || events[1].OccurredAtUnixSec != 2000 {
			t.Errorf("expected the two oldest events, got %d and %d",
				events[0].OccurredAtUnixSec, events[1].OccurredAtUnixSec)
		}
	})

	t.Run("a user who left stops receiving that community's events", func(t *testing.T) {
		leaverID := uuid.New().String()
		membershipID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.CommunityUser{
			Id:          membershipID,
			CommunityId: joined[0],
			UserId:      leaverID,
		}); err != nil {
			t.Fatalf("insert leaver membership: %v", err)
		}

		before, err := s.FindEventsForUserSince(ctx, leaverID, 0, 100)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		if len(before) == 0 {
			t.Fatal("expected the member to see events before leaving")
		}

		membership := &models.CommunityUser{}
		if err := s.GetByID(ctx, membershipID, membership); err != nil {
			t.Fatalf("get membership: %v", err)
		}
		membership.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  leaverID,
			DeletedAtUnixSec: 4000,
		}
		if err := s.Update(ctx, membership); err != nil {
			t.Fatalf("soft-delete membership: %v", err)
		}

		after, err := s.FindEventsForUserSince(ctx, leaverID, 0, 100)
		if err != nil {
			t.Fatalf("FindEventsForUserSince: %v", err)
		}
		if len(after) != 0 {
			t.Errorf("expected no events after leaving, got %d", len(after))
		}
	})

	t.Run("rejects a non-positive limit", func(t *testing.T) {
		if _, err := s.FindEventsForUserSince(ctx, memberID, 0, 0); err == nil {
			t.Error("expected an error for a zero limit")
		}
	})

	t.Run("rejects an empty user id", func(t *testing.T) {
		if _, err := s.FindEventsForUserSince(ctx, "", 0, 100); err == nil {
			t.Error("expected an error for an empty user id")
		}
	})
}
