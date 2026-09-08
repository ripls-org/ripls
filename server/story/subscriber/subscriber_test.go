package story_subscriber

import (
	"context"
	"sync"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/story"
)

// fakeCreator records every CreateStory call so tests can assert on the
// shape of the request without exercising the real story.Generator.
type fakeCreator struct {
	mu       sync.Mutex
	requests []story.CreateStoryRequest
	err      error
}

func (f *fakeCreator) CreateStory(_ context.Context, req story.CreateStoryRequest) (*models.Story, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return &models.Story{Id: "story-fake"}, nil
}

func (f *fakeCreator) Captured() []story.CreateStoryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]story.CreateStoryRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

func TestSubscriber_Name(t *testing.T) {
	s := New(nil, nil)
	if s.Name() != SubscriberName {
		t.Errorf("Name() = %q, want %q", s.Name(), SubscriberName)
	}
}

// TestSubscriber_HandleNilEvent verifies the dispatcher tolerates nil
// envelopes and empty payloads — the bus may occasionally feed a
// malformed envelope (a future durable transport may produce one
// during error recovery).
func TestSubscriber_HandleNilEvent(t *testing.T) {
	s := New(nil, &fakeCreator{})

	if err := s.Handle(context.Background(), nil); err != nil {
		t.Errorf("Handle(nil envelope) = %v, want nil", err)
	}
	if err := s.Handle(context.Background(), &cebus.PublishedEvent{}); err != nil {
		t.Errorf("Handle(empty envelope) = %v, want nil", err)
	}
}

// TestSubscriber_HandleNilCreator verifies that wiring the subscriber
// without a story.Creator (story generation disabled at startup) is a
// quiet no-op rather than a panic. The bus may always feed events; the
// subscriber takes the responsibility to short-circuit.
func TestSubscriber_HandleNilCreator(t *testing.T) {
	s := New(nil, nil)
	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-1",
			CommunityId: "community-1",
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
	}
	if err := s.Handle(context.Background(), evt); err != nil {
		t.Errorf("Handle with nil creator = %v, want nil", err)
	}
}

// TestSubscriber_HandleIgnoresUnrelatedEventTypes verifies the
// dispatcher silently drops events that don't trigger story creation —
// e.g. INTEREST_EXPRESSED, RSVP_YES — without invoking the creator.
// This protects against accidentally generating stories for new event
// types that are added but not classified.
func TestSubscriber_HandleIgnoresUnrelatedEventTypes(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	communityID := activeCommunity(t, s)

	creator := &fakeCreator{}
	sub := New(s, creator)

	skipped := []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}
	for _, et := range skipped {
		t.Run(et.String(), func(t *testing.T) {
			if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
				Event: &models.CommunityEvent{
					Id:          "evt-" + et.String(),
					CommunityId: communityID,
					EventType:   et,
				},
			}); err != nil {
				t.Errorf("Handle: %v", err)
			}
		})
	}

	if got := creator.Captured(); len(got) != 0 {
		t.Errorf("creator was called %d times for unrelated event types, want 0", len(got))
	}
}
