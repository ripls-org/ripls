package story_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestHandleExperienceCompleted_HappyPath(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})
	guestID, _ := s.Insert(context.Background(), &models.User{Name: "Guest", Email: "g@example.com"})
	noShowID, _ := s.Insert(context.Background(), &models.User{Name: "Skipper", Email: "s@example.com"})

	exp := &models.Experience{Name: "Trail Day", OwnerId: hostID}
	expID, err := s.Insert(context.Background(), exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	exp.Id = expID

	rsvps := []models.ExperienceRSVP{
		{ExperienceId: expID, UserId: hostID, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_YES},
		{ExperienceId: expID, UserId: guestID, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_YES},
		{ExperienceId: expID, UserId: noShowID, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_NO},
	}
	for i := range rsvps {
		if _, err := s.Insert(context.Background(), &rsvps[i]); err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}
	}

	creator := &fakeCreator{}
	sub := New(s, creator)

	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-exp-1",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
		},
		Experience: exp,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 story, got %d", len(got))
	}
	req := got[0]
	if req.StoryType != "STORY_TYPE_EXPERIENCE_CONCLUDED" {
		t.Errorf("StoryType = %q, want STORY_TYPE_EXPERIENCE_CONCLUDED", req.StoryType)
	}
	// Owner first, attendees following. The no-show is excluded.
	if len(req.ParticipantIDs) != 2 {
		t.Fatalf("ParticipantIDs = %v, want 2 (host + 1 attendee, no-show excluded)", req.ParticipantIDs)
	}
	if req.ParticipantIDs[0] != hostID {
		t.Errorf("first participant = %q, want host %q", req.ParticipantIDs[0], hostID)
	}
	if req.RelatedExperienceID != expID || req.RelatedEntityName != "Trail Day" {
		t.Errorf("related fields = %q/%q, want %q/Trail Day", req.RelatedExperienceID, req.RelatedEntityName, expID)
	}
}

func TestHandleExperienceCompleted_NoAttendeesSkipsStory(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})

	exp := &models.Experience{Name: "Empty Event", OwnerId: hostID}
	expID, _ := s.Insert(context.Background(), exp)
	exp.Id = expID
	// No RSVPs inserted.

	creator := &fakeCreator{}
	sub := New(s, creator)
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-no-attendees",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
		},
		Experience: exp,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := creator.Captured(); len(calls) != 0 {
		t.Errorf("expected no story when no attendees, got %d", len(calls))
	}
}

func TestHandleExperienceCompleted_IncludesProvisionalAttendeeNames(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})
	provID, _ := s.Insert(context.Background(), &models.ProvisionalUser{Name: "Plus One"})

	exp := &models.Experience{Name: "Mixed", OwnerId: hostID}
	expID, _ := s.Insert(context.Background(), exp)
	exp.Id = expID

	provisionalRef := provID
	if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: expID,
		UserId:       hostID,
		CommunityId:  communityID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
		Attended:     models.AttendedStatus_ATTENDED_STATUS_YES,
	}); err != nil {
		t.Fatalf("insert host rsvp: %v", err)
	}
	if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId:      expID,
		ProvisionalUserId: &provisionalRef,
		CommunityId:       communityID,
		Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
		Attended:          models.AttendedStatus_ATTENDED_STATUS_YES,
	}); err != nil {
		t.Fatalf("insert provisional rsvp: %v", err)
	}

	creator := &fakeCreator{}
	sub := New(s, creator)
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-prov-rsvp",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
		},
		Experience: exp,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 story, got %d", len(got))
	}
	// Real participants: just the host. Provisional user has no ID but
	// contributes to ParticipantNames.
	if len(got[0].ParticipantIDs) != 1 || got[0].ParticipantIDs[0] != hostID {
		t.Errorf("ParticipantIDs = %v, want [%q]", got[0].ParticipantIDs, hostID)
	}
	if len(got[0].ParticipantNames) != 2 {
		t.Errorf("ParticipantNames = %v, want host + prov", got[0].ParticipantNames)
	}
	hasProvisional := false
	for _, n := range got[0].ParticipantNames {
		if n == "Plus One" {
			hasProvisional = true
		}
	}
	if !hasProvisional {
		t.Errorf("ParticipantNames = %v, want prov name 'Plus One' included", got[0].ParticipantNames)
	}
}
