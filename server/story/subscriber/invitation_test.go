package story_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestHandleInvitationLinkUsed_HappyPath(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	memberID, _ := s.Insert(context.Background(), &models.User{Name: "New Member", Email: "nm@example.com"})

	creator := &fakeCreator{}
	sub := New(s, creator)
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-inv-1",
			CommunityId: communityID,
			ActorId:     memberID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		},
		Actor: &api.User{Id: memberID, Name: "New Member"},
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 story, got %d", len(got))
	}
	r := got[0]
	if r.StoryType != "STORY_TYPE_NEW_MEMBER_WELCOME" {
		t.Errorf("StoryType = %q, want STORY_TYPE_NEW_MEMBER_WELCOME", r.StoryType)
	}
	if r.RelatedEntityID != memberID || r.RelatedCommunityID != communityID {
		t.Errorf("related fields = %q/%q, want member/community", r.RelatedEntityID, r.RelatedCommunityID)
	}
	if len(r.ParticipantIDs) != 1 || r.ParticipantIDs[0] != memberID {
		t.Errorf("ParticipantIDs = %v, want [%q]", r.ParticipantIDs, memberID)
	}
	if r.CommunityName != "Test Community" {
		t.Errorf("CommunityName = %q, want Test Community", r.CommunityName)
	}
}

func TestHandleInvitationLinkUsed_NoActorIsNoOp(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	communityID := activeCommunity(t, s)

	creator := &fakeCreator{}
	sub := New(s, creator)
	// Actor is nil — pre-fetch failed or actor wasn't denormalized.
	// Subscriber must skip silently rather than panic on a nil deref.
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-no-actor",
			CommunityId: communityID,
			ActorId:     "some-id",
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		},
		Actor: nil,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := creator.Captured(); len(calls) != 0 {
		t.Errorf("expected no story when actor is nil, got %d", len(calls))
	}
}
