package community_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"

	"go.ripls.org/ripls/server/community"
)

func TestCommunityCategoryFor(t *testing.T) {
	tests := []struct {
		eventType models.CommunityEventType
		want      models.NotificationCategory
	}{
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_REQUESTS},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_EXPERIENCES},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, models.NotificationCategory_NOTIFICATION_CATEGORY_GEAR_SHARED},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED, models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED, models.NotificationCategory_NOTIFICATION_CATEGORY_TRANSFER_UPDATES},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, models.NotificationCategory_NOTIFICATION_CATEGORY_REQUEST_UPDATES},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED, models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED, models.NotificationCategory_NOTIFICATION_CATEGORY_PLANNING_UPDATES},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED, models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_MEMBERS},
	}
	for _, tt := range tests {
		t.Run(tt.eventType.String(), func(t *testing.T) {
			if got := community.CategoryFor(tt.eventType); got != tt.want {
				t.Errorf("community.CategoryFor(%v) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestCategoryEnabled_NilPrefsAndUnsetFieldsAreOn(t *testing.T) {
	if !community.CategoryEnabled(nil, models.NotificationCategory_NOTIFICATION_CATEGORY_CHATS) {
		t.Error("nil prefs must resolve to enabled")
	}

	prefs := &models.CommunityNotificationPreferences{}
	for _, cat := range []models.NotificationCategory{
		models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_REQUESTS,
		models.NotificationCategory_NOTIFICATION_CATEGORY_CHATS,
		models.NotificationCategory_NOTIFICATION_CATEGORY_TRANSFER_UPDATES,
	} {
		if !community.CategoryEnabled(prefs, cat) {
			t.Errorf("unset field for %v must resolve to enabled", cat)
		}
	}
}

func TestCategoryEnabled_ExplicitFalseDisables(t *testing.T) {
	off := false
	prefs := &models.CommunityNotificationPreferences{
		NotifyChats: &off,
	}
	if community.CategoryEnabled(prefs, models.NotificationCategory_NOTIFICATION_CATEGORY_CHATS) {
		t.Error("explicit false NotifyChats must disable CHATS")
	}
	if !community.CategoryEnabled(prefs, models.NotificationCategory_NOTIFICATION_CATEGORY_TRANSFER_UPDATES) {
		t.Error("explicit false on one category must not affect others")
	}
}

func TestNotifyTargetedUsers_RespectsDisabledCategory(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	creatorID := insertTestUser(t, s, "creator@example.com", "Creator")
	memberID := insertTestUser(t, s, "member@example.com", "Member")
	communityID := insertTestCommunity(t, s, "Test Community", creatorID)

	// Add member as community user
	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      memberID,
	}); err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// Disable NEW_EXPERIENCES for the member.
	off := false
	if _, err := s.Insert(context.Background(), &models.CommunityNotificationPreferences{
		CommunityId:          communityID,
		UserId:               memberID,
		NotifyNewExperiences: &off,
	}); err != nil {
		t.Fatalf("failed to insert prefs: %v", err)
	}

	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:     creatorID,
	}

	sub := New(s, mockNotif)
	_ = sub.Handle(context.Background(), &cebus.PublishedEvent{Event: event})

	for _, call := range mockNotif.GetCalls() {
		if call.UserID == memberID {
			t.Errorf("member with disabled NEW_EXPERIENCES should not have been notified")
		}
	}
}

func TestNotifyTargetedUsers_NotifiesWhenCategoryUnset(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	creatorID := insertTestUser(t, s, "creator2@example.com", "Creator2")
	memberID := insertTestUser(t, s, "member2@example.com", "Member2")
	communityID := insertTestCommunity(t, s, "Test Community 2", creatorID)

	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      memberID,
	}); err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// No prefs row for member → defaults all-on → must be notified.

	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:     creatorID,
	}

	sub := New(s, mockNotif)
	_ = sub.Handle(context.Background(), &cebus.PublishedEvent{Event: event})

	notified := false
	for _, call := range mockNotif.GetCalls() {
		if call.UserID == memberID {
			notified = true
		}
	}
	if !notified {
		t.Error("member with no prefs row should be notified (defaults are on)")
	}
}

func TestPlanningEventRecipients_RsvpYesAndMaybeMinusActor(t *testing.T) {
	s := setupTestStorage(t)

	hostID := insertTestUser(t, s, "host@example.com", "Host")
	yesUserID := insertTestUser(t, s, "yes@example.com", "Yes")
	maybeUserID := insertTestUser(t, s, "maybe@example.com", "Maybe")
	noUserID := insertTestUser(t, s, "no@example.com", "No")
	communityID := insertTestCommunity(t, s, "Planning Community", hostID)

	exp := &models.Experience{Name: "Trail Day", OwnerId: hostID}
	expID, err := s.Insert(context.Background(), exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	rsvps := []struct {
		uid string
		v   models.RSVPIntention
	}{
		{yesUserID, models.RSVPIntention_RSVP_INTENTION_YES},
		{maybeUserID, models.RSVPIntention_RSVP_INTENTION_MAYBE},
		{noUserID, models.RSVPIntention_RSVP_INTENTION_NO},
	}
	for _, r := range rsvps {
		if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
			ExperienceId: expID,
			UserId:       r.uid,
			Intention:    r.v,
		}); err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}
	}

	for _, eventType := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
	} {
		t.Run(eventType.String(), func(t *testing.T) {
			// yesUserID is the actor — they must be excluded even though
			// they RSVP'd yes.
			event := &models.CommunityEvent{
				CommunityId: communityID,
				EventType:   eventType,
				ActorId:     yesUserID,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: expID},
			}

			got := getNotificationRecipients(context.Background(), s, event)
			gotSet := map[string]bool{}
			for _, id := range got {
				gotSet[id] = true
			}
			if !gotSet[maybeUserID] {
				t.Error("MAYBE RSVP should be a recipient")
			}
			if gotSet[yesUserID] {
				t.Error("actor must be excluded")
			}
			if gotSet[noUserID] {
				t.Error("NO RSVP must not be a recipient")
			}
			if gotSet[hostID] {
				t.Error("host without RSVP must not be a recipient")
			}
		})
	}
}

// TestExperienceUpdated_PreferenceGating verifies that EXPERIENCE_UPDATED is
// suppressed when notify_experience_rsvps is explicitly disabled, mirroring
// the same gate that applies to EXPERIENCE_STARTED.
func TestExperienceUpdated_PreferenceGating(t *testing.T) {
	s := setupTestStorage(t)
	mockNotif := notifications.NewMockService()

	ownerID := insertTestUser(t, s, "owner-pref@example.com", "Owner Pref")
	yesUserID := insertTestUser(t, s, "yes-pref@example.com", "Yes Pref")
	communityID := insertTestCommunity(t, s, "Pref Test Community", ownerID)

	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      yesUserID,
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	experience := &models.Experience{Name: "Pref Event", OwnerId: ownerID}
	expID, _ := s.Insert(context.Background(), experience)

	_, _ = s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: expID,
		UserId:       yesUserID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
	})

	// Disable notify_experience_rsvps for yes-user.
	off := false
	if _, err := s.Insert(context.Background(), &models.CommunityNotificationPreferences{
		CommunityId:           communityID,
		UserId:                yesUserID,
		NotifyExperienceRsvps: &off,
	}); err != nil {
		t.Fatalf("insert prefs: %v", err)
	}

	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		ActorId:     ownerID,
		Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: expID},
	}

	sub := New(s, mockNotif)
	_ = sub.Handle(context.Background(), &cebus.PublishedEvent{Event: event})

	for _, call := range mockNotif.GetCalls() {
		if call.UserID == yesUserID {
			t.Errorf("user with notify_experience_rsvps=false should not receive EXPERIENCE_UPDATED notification")
		}
	}
}
