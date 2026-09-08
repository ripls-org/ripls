package story_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestHandleRequestFulfilled_PrefersConfirmedHelpers(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	requesterID, _ := s.Insert(context.Background(), &models.User{Name: "Asker", Email: "a@example.com"})
	helperA, _ := s.Insert(context.Background(), &models.User{Name: "Helper A", Email: "ha@example.com"})
	helperB, _ := s.Insert(context.Background(), &models.User{Name: "Helper B", Email: "hb@example.com"})
	otherOfferer, _ := s.Insert(context.Background(), &models.User{Name: "Other", Email: "o@example.com"})

	req := &models.Request{
		Title:              "Need help moving",
		RequesterId:        requesterID,
		ConfirmedHelperIds: []string{helperA, helperB},
	}
	reqID, _ := s.Insert(context.Background(), req)
	req.Id = reqID

	// Insert an unrelated offer that should NOT appear in participants
	// because confirmed helpers take precedence.
	if _, err := s.Insert(context.Background(), &models.RequestOffer{
		RequestId: reqID,
		UserId:    otherOfferer,
		Withdrawn: false,
	}); err != nil {
		t.Fatalf("insert offer: %v", err)
	}

	creator := &fakeCreator{}
	sub := New(s, creator)

	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-req-1",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		},
		Request: req,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 story, got %d", len(got))
	}
	r := got[0]
	if r.StoryType != "STORY_TYPE_REQUEST_FULFILLED" {
		t.Errorf("StoryType = %q, want STORY_TYPE_REQUEST_FULFILLED", r.StoryType)
	}
	// Order: requester first, then helpers.
	if len(r.ParticipantIDs) != 3 {
		t.Fatalf("ParticipantIDs = %v, want 3 (requester + 2 helpers)", r.ParticipantIDs)
	}
	if r.ParticipantIDs[0] != requesterID {
		t.Errorf("first participant = %q, want requester %q", r.ParticipantIDs[0], requesterID)
	}
	// Other offerer must NOT be in the list — confirmed-helpers wins.
	for _, id := range r.ParticipantIDs {
		if id == otherOfferer {
			t.Errorf("other offerer should NOT appear when ConfirmedHelperIds is set; got %v", r.ParticipantIDs)
		}
	}
	if r.RelatedRequestID != reqID || r.RelatedEntityName != "Need help moving" {
		t.Errorf("related fields = %q/%q, want %q/Need help moving", r.RelatedRequestID, r.RelatedEntityName, reqID)
	}
}

func TestHandleRequestFulfilled_FallsBackToOffersCappedAtTwo(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	requesterID, _ := s.Insert(context.Background(), &models.User{Name: "Asker", Email: "a@example.com"})
	offerers := []string{}
	for i := range 3 {
		uid, _ := s.Insert(context.Background(), &models.User{
			Name:  "Offerer",
			Email: "o" + string(rune('0'+i)) + "@example.com",
		})
		offerers = append(offerers, uid)
		if _, err := s.Insert(context.Background(), &models.RequestOffer{
			UserId:    uid,
			Withdrawn: false,
			// RequestId set below after request insert.
		}); err != nil {
			t.Fatalf("insert offer: %v", err)
		}
	}

	req := &models.Request{
		Title:       "Need a ladder",
		RequesterId: requesterID,
		// No ConfirmedHelperIds, so subscriber falls back to offers.
	}
	reqID, _ := s.Insert(context.Background(), req)
	req.Id = reqID

	// Update the offers to point at this request.
	offers, err := s.QueryByField(context.Background(), "user_id", offerers[0], &models.RequestOffer{})
	if err != nil {
		t.Fatalf("query offers: %v", err)
	}
	for _, m := range offers {
		o := m.(*models.RequestOffer)
		o.RequestId = reqID
		if err := s.Update(context.Background(), o); err != nil {
			t.Fatalf("update offer: %v", err)
		}
	}
	// Same for the other two.
	for _, uid := range offerers[1:] {
		offs, _ := s.QueryByField(context.Background(), "user_id", uid, &models.RequestOffer{})
		for _, m := range offs {
			o := m.(*models.RequestOffer)
			o.RequestId = reqID
			_ = s.Update(context.Background(), o)
		}
	}

	creator := &fakeCreator{}
	sub := New(s, creator)
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-req-2",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		},
		Request: req,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("expected 1 story, got %d", len(got))
	}
	// Requester + at most 2 helpers (cap is enforced even with 3 offers).
	if len(got[0].ParticipantIDs) > 3 {
		t.Errorf("ParticipantIDs = %v, want at most 3 (requester + ≤2 helpers)", got[0].ParticipantIDs)
	}
}
