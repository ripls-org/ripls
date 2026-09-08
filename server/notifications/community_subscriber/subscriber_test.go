package community_subscriber

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
)

// activeCommunity inserts a community that satisfies the
// community_owner_required CHECK constraint. Returns the community ID
// and its owner ID.
func activeCommunity(t *testing.T) (communityID, ownerID string) {
	t.Helper()
	s := setupTestStorage(t)
	ownerID = insertTestUser(t, s, "owner@example.com", "Owner")
	communityID = insertTestCommunity(t, s, "Test Community", ownerID)
	insertMembership(t, s, communityID, ownerID)
	return communityID, ownerID
}

func TestSubscriber_Name(t *testing.T) {
	if got := New(nil, nil).Name(); got != SubscriberName {
		t.Errorf("Name() = %q, want %q", got, SubscriberName)
	}
}

func TestSubscriber_SetStreamChecker(t *testing.T) {
	sub := New(nil, nil)
	if got := sub.loadStreamChecker(); got != nil {
		t.Errorf("loadStreamChecker on fresh subscriber = %v, want nil", got)
	}

	var calls int
	sub.SetStreamChecker(func(_ string) bool {
		calls++
		return true
	})
	if got := sub.loadStreamChecker(); got == nil {
		t.Fatal("loadStreamChecker after SetStreamChecker = nil, want non-nil")
	}
	_ = sub.loadStreamChecker()("u")
	if calls != 1 {
		t.Errorf("registered fn called %d times, want 1", calls)
	}

	// SetStreamChecker(nil) clears the hook.
	sub.SetStreamChecker(nil)
	if got := sub.loadStreamChecker(); got != nil {
		t.Errorf("loadStreamChecker after SetStreamChecker(nil) = %v, want nil", got)
	}
}

func TestSubscriber_HandleNilOrEmptyEvent(t *testing.T) {
	mock := notifications.NewMockService()
	sub := New(nil, mock)

	if err := sub.Handle(context.Background(), nil); err != nil {
		t.Errorf("Handle(nil) = %v, want nil", err)
	}
	if err := sub.Handle(context.Background(), &cebus.PublishedEvent{}); err != nil {
		t.Errorf("Handle(empty envelope) = %v, want nil", err)
	}
	if len(mock.GetCalls()) != 0 {
		t.Errorf("NotifyUser called %d times for nil events, want 0", len(mock.GetCalls()))
	}
}

func TestSubscriber_HandleShortCircuitsOnUnnotifiedEventType(t *testing.T) {
	store := setupTestStorage(t)
	communityID, _ := activeCommunity(t)
	mock := notifications.NewMockService()
	sub := New(store, mock)

	// EXPERIENCE_RSVP_NO is not in shouldNotify — the dispatcher must
	// return without touching storage, preferences, or push.
	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-skip",
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := mock.GetCalls(); len(calls) != 0 {
		t.Errorf("NotifyUser called %d times for unnotified event type, want 0", len(calls))
	}
}

func TestSubscriber_HandleSoftDeletedCommunityIsSkipped(t *testing.T) {
	store := setupTestStorage(t)
	ownerID := insertTestUser(t, store, "owner@example.com", "Owner")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	communityID := insertTestCommunity(t, store, "Doomed", ownerID)
	insertMembership(t, store, communityID, ownerID)
	insertMembership(t, store, communityID, memberID)

	// Soft-delete the community via direct storage write.
	c := &models.Community{}
	if err := store.GetByID(context.Background(), communityID, c); err != nil {
		t.Fatalf("get community: %v", err)
	}
	c.Deleted = &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: 1700000000}
	if err := store.Update(context.Background(), c); err != nil {
		t.Fatalf("force-delete: %v", err)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	// GEAR_SHARED is a notify-able event type, but the soft-delete gate
	// must suppress it because GEAR_SHARED is NOT a
	// FiresForDeletedCommunity type.
	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-after-delete",
		CommunityId: communityID,
		ActorId:     ownerID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := mock.GetCalls(); len(calls) != 0 {
		t.Errorf("NotifyUser called %d times for soft-deleted community, want 0", len(calls))
	}
}

func TestSubscriber_HandleSoftDeletedCommunityFiresAllowedEventTypes(t *testing.T) {
	store := setupTestStorage(t)
	ownerID := insertTestUser(t, store, "owner@example.com", "Owner")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	communityID := insertTestCommunity(t, store, "Doomed", ownerID)
	insertMembership(t, store, communityID, ownerID)
	insertMembership(t, store, communityID, memberID)

	// Soft-delete and populate the deleted_snapshot the COMMUNITY_DELETED
	// recipient lookup reads.
	c := &models.Community{}
	if err := store.GetByID(context.Background(), communityID, c); err != nil {
		t.Fatalf("get community: %v", err)
	}
	c.Deleted = &models.DeletedMetadata{DeletedByUserId: ownerID, DeletedAtUnixSec: 1700000000}
	c.DeletedSnapshot = &models.CommunityDeletedSnapshot{MemberUserIds: []string{ownerID, memberID}}
	if err := store.Update(context.Background(), c); err != nil {
		t.Fatalf("force-delete: %v", err)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	// COMMUNITY_DELETED is in FiresForDeletedCommunity — the gate
	// must NOT suppress it.
	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-deleted-notify",
		CommunityId: communityID,
		ActorId:     ownerID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Member should receive the notification (deleter is excluded).
	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("NotifyUser calls = %d, want 1 (member, not actor)", len(calls))
	}
	if calls[0].UserID != memberID {
		t.Errorf("recipient = %q, want member %q", calls[0].UserID, memberID)
	}
}

func TestSubscriber_HandleActorIsNeverNotified(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	communityID := insertTestCommunity(t, store, "Test Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, memberID)

	mock := notifications.NewMockService()
	sub := New(store, mock)

	// Broadcast event whose recipient set INCLUDES the actor —
	// getNotificationRecipients for GEAR_SHARED returns all members
	// minus actor, but Handle has a belt-and-suspenders actor check.
	// Set up a path that would re-introduce the actor: directly drive
	// the broadcast recipient resolution with a MEMBER_JOINED-style
	// event by setting ObjectUserId to actorID for an event whose
	// recipient resolver pulls members from CommunityUser.
	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-broadcast",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for _, call := range mock.GetCalls() {
		if call.UserID == actorID {
			t.Errorf("actor was notified: %+v", call)
		}
	}
}

func TestSubscriber_HandleSuppressesStreamingUsers(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	streamingID := insertTestUser(t, store, "streamer@example.com", "Streamer")
	communityID := insertTestCommunity(t, store, "Test Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, memberID)
	insertMembership(t, store, communityID, streamingID)

	mock := notifications.NewMockService()
	sub := New(store, mock)

	// Stream-checker says streamingID is in an active stream — that
	// user must not receive a push.
	sub.SetStreamChecker(func(userID string) bool {
		return userID == streamingID
	})

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-stream-supp",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for _, call := range mock.GetCalls() {
		if call.UserID == streamingID {
			t.Errorf("streaming user was notified, push should be suppressed: %+v", call)
		}
	}
	// member is not streaming — must be notified.
	notifiedMember := false
	for _, call := range mock.GetCalls() {
		if call.UserID == memberID {
			notifiedMember = true
		}
	}
	if !notifiedMember {
		t.Errorf("non-streaming member was not notified; want notification (calls=%+v)", mock.GetCalls())
	}
}

func TestSubscriber_HandleAppliesPerUserCategoryPreference(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	optedOutID := insertTestUser(t, store, "out@example.com", "OptedOut")
	memberID := insertTestUser(t, store, "in@example.com", "OptedIn")
	communityID := insertTestCommunity(t, store, "Test Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, optedOutID)
	insertMembership(t, store, communityID, memberID)

	// Opt the user out of GEAR_SHARED.
	off := false
	if _, err := store.Insert(context.Background(), &models.CommunityNotificationPreferences{
		CommunityId:      communityID,
		UserId:           optedOutID,
		NotifyGearShared: &off,
	}); err != nil {
		t.Fatalf("insert prefs: %v", err)
	}

	mock := notifications.NewMockService()
	sub := New(store, mock)

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-prefs",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	for _, call := range mock.GetCalls() {
		if call.UserID == optedOutID {
			t.Errorf("opted-out member was notified: %+v", call)
		}
	}
	// memberID (no prefs row) should still be notified.
	notifiedOptedIn := false
	for _, call := range mock.GetCalls() {
		if call.UserID == memberID {
			notifiedOptedIn = true
		}
	}
	if !notifiedOptedIn {
		t.Errorf("opted-in member without prefs row was not notified (defaults are on); calls=%+v", mock.GetCalls())
	}
}

func TestSubscriber_HandleProceedsWhenNoRecipients(t *testing.T) {
	// Edge case: shouldNotify returns true for the event type but the
	// recipient resolver returns an empty list (e.g. an
	// experience-RSVP-yes with no ObjectUserId set). Handle must log
	// debug + return without invoking the notification service.
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	communityID := insertTestCommunity(t, store, "Test Community", actorID)
	insertMembership(t, store, communityID, actorID)

	mock := notifications.NewMockService()
	sub := New(store, mock)

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-no-recipients",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		// ObjectUserId deliberately unset — recipients() returns nil.
	}}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := mock.GetCalls(); len(calls) != 0 {
		t.Errorf("NotifyUser called %d times when recipients is empty, want 0", len(calls))
	}
}

// sinkCtx returns a context that captures all INFO+ log records via logging.TestSink.
func sinkCtx() (context.Context, *logging.TestSink) {
	sink := logging.NewTestSink(slog.LevelInfo)
	ctx := logging.WithLogger(context.Background(), sink.NewLogger())
	return ctx, sink
}

// outcomeFor scans records for the first entry matching userID and returns its
// "outcome" attribute, or "" if none is found.
func outcomeFor(records []logging.LogRecord, userID string) string {
	for _, r := range records {
		if uid, _ := r.Attrs["user_id"].(string); uid == userID {
			if out, _ := r.Attrs["outcome"].(string); out != "" {
				return out
			}
		}
	}
	return ""
}

func TestSubscriber_OutcomeActorSuppressed(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	communityID := insertTestCommunity(t, store, "Community", actorID)
	insertMembership(t, store, communityID, actorID)

	ctx, sink := sinkCtx()
	sub := New(store, notifications.NewMockService())

	// EXPERIENCE_RSVP_YES resolves ObjectUserId as the sole recipient. When
	// ObjectUserId == ActorId (owner RSVP-ing their own event), the recipient
	// list is {actorID} and the per-loop actor guard must suppress it.
	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:           "evt-actor",
		CommunityId:  communityID,
		ActorId:      actorID,
		ObjectUserId: actorID,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
	}}
	if err := sub.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := outcomeFor(sink.Records(), actorID); got != outcomeSuppressedActor {
		t.Errorf("actor outcome = %q, want %q", got, outcomeSuppressedActor)
	}
}

func TestSubscriber_OutcomeActiveStreamSuppressed(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	streamingID := insertTestUser(t, store, "streaming@example.com", "Streamer")
	communityID := insertTestCommunity(t, store, "Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, streamingID)

	ctx, sink := sinkCtx()
	sub := New(store, notifications.NewMockService())
	sub.SetStreamChecker(func(userID string) bool { return userID == streamingID })

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-stream",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := outcomeFor(sink.Records(), streamingID); got != outcomeSuppressedActiveStream {
		t.Errorf("streaming user outcome = %q, want %q", got, outcomeSuppressedActiveStream)
	}
}

func TestSubscriber_OutcomeCategorySuppressed(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	optedOutID := insertTestUser(t, store, "out@example.com", "OptedOut")
	communityID := insertTestCommunity(t, store, "Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, optedOutID)

	off := false
	if _, err := store.Insert(context.Background(), &models.CommunityNotificationPreferences{
		CommunityId:      communityID,
		UserId:           optedOutID,
		NotifyGearShared: &off,
	}); err != nil {
		t.Fatalf("insert prefs: %v", err)
	}

	ctx, sink := sinkCtx()
	sub := New(store, notifications.NewMockService())

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-cat",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := outcomeFor(sink.Records(), optedOutID); got != outcomeSuppressedCategory {
		t.Errorf("opted-out user outcome = %q, want %q", got, outcomeSuppressedCategory)
	}
}

func TestSubscriber_OutcomeSent(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	communityID := insertTestCommunity(t, store, "Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, memberID)

	ctx, sink := sinkCtx()
	sub := New(store, notifications.NewMockService())

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-sent",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := outcomeFor(sink.Records(), memberID); got != outcomeSent {
		t.Errorf("member outcome = %q, want %q", got, outcomeSent)
	}
}

func TestSubscriber_OutcomeSendFailed(t *testing.T) {
	store := setupTestStorage(t)
	actorID := insertTestUser(t, store, "actor@example.com", "Actor")
	memberID := insertTestUser(t, store, "member@example.com", "Member")
	communityID := insertTestCommunity(t, store, "Community", actorID)
	insertMembership(t, store, communityID, actorID)
	insertMembership(t, store, communityID, memberID)

	ctx, sink := sinkCtx()

	failingService := &failService{}
	sub := New(store, failingService)

	evt := &cebus.PublishedEvent{Event: &models.CommunityEvent{
		Id:          "evt-fail",
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
	}}
	if err := sub.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	// send_failed logs at WARN; sinkCtx captures INFO+, WARN is included.
	if got := outcomeFor(sink.Records(), memberID); got != outcomeSendFailed {
		t.Errorf("member outcome on send failure = %q, want %q", got, outcomeSendFailed)
	}
}

// failService is a Service that always returns an error from NotifyUser.
type failService struct{}

func (f *failService) NotifyUser(_ context.Context, _ string, _ *models.Notification) error {
	return errSendFailed
}
func (f *failService) HasDevices(_ context.Context, _ string) bool        { return true }
func (f *failService) UnregisterDevice(_ context.Context, _ string) error { return nil }
func (f *failService) SendPhoneOptInWelcome(_ context.Context, _ *models.User) error {
	return nil
}

var errSendFailed = fmt.Errorf("simulated send failure")
