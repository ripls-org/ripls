package community_subscriber

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetNotificationRecipients_RequestOfferSelected(t *testing.T) {
	testStorage := setupTestStorage(t)

	requesterID := insertTestUser(t, testStorage, "req@example.com", "Requester")
	offererID := insertTestUser(t, testStorage, "off@example.com", "Offerer")

	request := &models.Request{Title: "Need a tent", RequesterId: requesterID}
	requestID, _ := testStorage.Insert(context.Background(), request)

	event := &models.CommunityEvent{
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED,
		ActorId:      requesterID,
		ObjectUserId: offererID,
		Topic:        &models.CommunityEvent_RequestId{RequestId: requestID},
	}

	got := getNotificationRecipients(context.Background(), testStorage, event)
	if len(got) != 1 || got[0] != offererID {
		t.Errorf("expected recipients=[%s], got %v", offererID, got)
	}
}

func TestGetNotificationRecipients_RequestCancelledFanOut(t *testing.T) {
	testStorage := setupTestStorage(t)

	requesterID := insertTestUser(t, testStorage, "rc@example.com", "Requester")
	offererA := insertTestUser(t, testStorage, "oa@example.com", "OffererA")
	offererB := insertTestUser(t, testStorage, "ob@example.com", "OffererB")

	request := &models.Request{Title: "Need help", RequesterId: requesterID}
	requestID, _ := testStorage.Insert(context.Background(), request)

	for _, uid := range []string{offererA, offererB} {
		if _, err := testStorage.Insert(context.Background(), &models.RequestOffer{
			RequestId: requestID,
			UserId:    uid,
		}); err != nil {
			t.Fatalf("insert offer: %v", err)
		}
	}

	// Requester cancels the request — both offerers should be notified,
	// but the actor (requester) should be excluded.
	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
		ActorId:   requesterID,
		Topic:     &models.CommunityEvent_RequestId{RequestId: requestID},
	}

	got := getNotificationRecipients(context.Background(), testStorage, event)
	gotSet := map[string]bool{}
	for _, id := range got {
		gotSet[id] = true
	}
	if !gotSet[offererA] || !gotSet[offererB] || gotSet[requesterID] {
		t.Errorf("expected both offerers, not requester; got %v", got)
	}
}

func TestGetNotificationRecipients_BroadcastToCommunityMinusActor(t *testing.T) {
	testStorage := setupTestStorage(t)

	creatorID := insertTestUser(t, testStorage, "cb@example.com", "Creator")
	memberA := insertTestUser(t, testStorage, "ma@example.com", "MemberA")
	memberB := insertTestUser(t, testStorage, "mb@example.com", "MemberB")
	communityID := insertTestCommunity(t, testStorage, "Broadcast Community", creatorID)

	for _, uid := range []string{creatorID, memberA, memberB} {
		if _, err := testStorage.Insert(context.Background(), &models.CommunityUser{
			CommunityId: communityID,
			UserId:      uid,
		}); err != nil {
			t.Fatalf("insert member: %v", err)
		}
	}

	for _, eventType := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
	} {
		t.Run(eventType.String(), func(t *testing.T) {
			event := &models.CommunityEvent{
				CommunityId: communityID,
				EventType:   eventType,
				ActorId:     creatorID,
			}
			got := getNotificationRecipients(context.Background(), testStorage, event)
			gotSet := map[string]bool{}
			for _, id := range got {
				gotSet[id] = true
			}
			if !gotSet[memberA] || !gotSet[memberB] || gotSet[creatorID] {
				t.Errorf("expected memberA and memberB but not creator; got %v", got)
			}
		})
	}
}

func TestGetNotificationRecipients_ExperienceStarted_ActorExcluded(t *testing.T) {
	// Actor (owner) must be excluded; YES/MAYBE RSVPs included; NO RSVPs excluded.
	testStorage := setupTestStorage(t)

	ownerID := insertTestUser(t, testStorage, "owner@example.com", "Owner")
	yesUserID := insertTestUser(t, testStorage, "yes@example.com", "Yes User")
	maybeUserID := insertTestUser(t, testStorage, "maybe@example.com", "Maybe User")
	noUserID := insertTestUser(t, testStorage, "no@example.com", "No User")

	experience := &models.Experience{Name: "Sunrise Hike", OwnerId: ownerID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	// Owner auto-RSVP (YES) — must be excluded as actor.
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: ownerID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: yesUserID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: maybeUserID, Intention: models.RSVPIntention_RSVP_INTENTION_MAYBE})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: noUserID, Intention: models.RSVPIntention_RSVP_INTENTION_NO})

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		ActorId:   ownerID,
		Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 2 {
		t.Fatalf("Expected 2 recipients (yesUser + maybeUser), got %d: %v", len(recipients), recipients)
	}
	recipientSet := make(map[string]bool, 2)
	for _, id := range recipients {
		recipientSet[id] = true
	}
	if !recipientSet[yesUserID] {
		t.Error("Expected yesUser in recipients")
	}
	if !recipientSet[maybeUserID] {
		t.Error("Expected maybeUser in recipients")
	}
	if recipientSet[ownerID] {
		t.Error("Actor (owner) must not be in recipients")
	}
	if recipientSet[noUserID] {
		t.Error("NO RSVP user must not be in recipients")
	}
}

func TestGetNotificationRecipients_ExperienceCancelled_ActorExcluded(t *testing.T) {
	// Same recipient semantics as STARTED: YES/MAYBE minus actor.
	testStorage := setupTestStorage(t)

	ownerID := insertTestUser(t, testStorage, "owner@example.com", "Owner")
	yesUserID := insertTestUser(t, testStorage, "yes@example.com", "Yes User")

	experience := &models.Experience{Name: "Beach Day", OwnerId: ownerID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: ownerID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: yesUserID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		ActorId:   ownerID,
		Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 1 || recipients[0] != yesUserID {
		t.Errorf("Expected [yesUser], got %v", recipients)
	}
}

func TestGetNotificationRecipients_ExperienceCompleted_TargetsRSVPsNotCommunity(t *testing.T) {
	// Regression: EXPERIENCE_COMPLETED used to broadcast to the whole community.
	// It now notifies only Yes/Maybe RSVPs minus the actor (the host) — a
	// community member who never RSVP'd must NOT be pinged that an event they
	// weren't going to wrapped up.
	testStorage := setupTestStorage(t)

	ownerID := insertTestUser(t, testStorage, "owner-done@example.com", "Owner")
	yesUserID := insertTestUser(t, testStorage, "yes-done@example.com", "Yes User")
	maybeUserID := insertTestUser(t, testStorage, "maybe-done@example.com", "Maybe User")
	noUserID := insertTestUser(t, testStorage, "no-done@example.com", "No User")
	bystanderID := insertTestUser(t, testStorage, "bystander@example.com", "Bystander")

	communityID := insertTestCommunity(t, testStorage, "Completion Community", ownerID)
	// The bystander is a community member but never RSVP'd — under the old
	// broadcast behavior they would have been notified.
	for _, uid := range []string{ownerID, yesUserID, maybeUserID, noUserID, bystanderID} {
		if _, err := testStorage.Insert(context.Background(), &models.CommunityUser{
			CommunityId: communityID,
			UserId:      uid,
		}); err != nil {
			t.Fatalf("insert member: %v", err)
		}
	}

	experience := &models.Experience{Name: "Beach Cleanup", OwnerId: ownerID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	// Owner is also a YES RSVP but must be excluded as actor.
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: ownerID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: yesUserID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: maybeUserID, Intention: models.RSVPIntention_RSVP_INTENTION_MAYBE})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: noUserID, Intention: models.RSVPIntention_RSVP_INTENTION_NO})

	event := &models.CommunityEvent{
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
		CommunityId: communityID,
		ActorId:     ownerID,
		Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	set := make(map[string]bool, len(recipients))
	for _, id := range recipients {
		set[id] = true
	}
	if len(recipients) != 2 {
		t.Fatalf("expected 2 recipients (yes + maybe), got %d: %v", len(recipients), recipients)
	}
	if !set[yesUserID] || !set[maybeUserID] {
		t.Errorf("expected yes + maybe RSVPs, got %v", recipients)
	}
	if set[ownerID] {
		t.Error("actor (host) must not be in recipients")
	}
	if set[noUserID] {
		t.Error("NO RSVP user must not be in recipients")
	}
	if set[bystanderID] {
		t.Error("a non-RSVP community member must not be notified of completion")
	}
}

func TestGetNotificationRecipients_ExperienceLifecycle_MissingExperienceID(t *testing.T) {
	// Event missing experience_id must return nil without panicking.
	testStorage := setupTestStorage(t)

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		ActorId:   "some-actor",
		// No Topic set — experience_id is empty string.
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)
	if recipients != nil {
		t.Errorf("Expected nil recipients for missing experience_id, got %v", recipients)
	}
}

func TestGetNotificationRecipients_ExperienceUpdated_ActorExcluded(t *testing.T) {
	// EXPERIENCE_UPDATED uses the same Yes/Maybe-minus-actor funnel as
	// EXPERIENCE_STARTED and EXPERIENCE_CANCELLED.
	testStorage := setupTestStorage(t)

	ownerID := insertTestUser(t, testStorage, "owner-upd@example.com", "Owner")
	yesUserID := insertTestUser(t, testStorage, "yes-upd@example.com", "Yes User")
	maybeUserID := insertTestUser(t, testStorage, "maybe-upd@example.com", "Maybe User")
	noUserID := insertTestUser(t, testStorage, "no-upd@example.com", "No User")

	experience := &models.Experience{Name: "Updated Hike", OwnerId: ownerID}
	experienceID, _ := testStorage.Insert(context.Background(), experience)

	// Owner is also a YES RSVP but must be excluded as actor.
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: ownerID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: yesUserID, Intention: models.RSVPIntention_RSVP_INTENTION_YES})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: maybeUserID, Intention: models.RSVPIntention_RSVP_INTENTION_MAYBE})
	_, _ = testStorage.Insert(context.Background(), &models.ExperienceRSVP{ExperienceId: experienceID, UserId: noUserID, Intention: models.RSVPIntention_RSVP_INTENTION_NO})

	event := &models.CommunityEvent{
		EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		ActorId:   ownerID,
		Topic:     &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	recipients := getNotificationRecipients(context.Background(), testStorage, event)

	if len(recipients) != 2 {
		t.Fatalf("expected 2 recipients (yes + maybe), got %d: %v", len(recipients), recipients)
	}
	set := make(map[string]bool, 2)
	for _, id := range recipients {
		set[id] = true
	}
	if !set[yesUserID] {
		t.Error("yes RSVP user must be in recipients")
	}
	if !set[maybeUserID] {
		t.Error("maybe RSVP user must be in recipients")
	}
	if set[ownerID] {
		t.Error("actor (owner) must not be in recipients")
	}
	if set[noUserID] {
		t.Error("NO RSVP user must not be in recipients")
	}
}
