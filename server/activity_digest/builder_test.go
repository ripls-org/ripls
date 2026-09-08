package activity_digest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fixtureClock returns a deterministic now() for the digest output.
func fixtureClock() func() time.Time {
	return func() time.Time {
		return time.Unix(2_000_000, 0).UTC()
	}
}

// localDateInUTC returns a time.Time at noon UTC inside the given day.
func localDateInUTC(t *testing.T, year, month, day int) time.Time {
	t.Helper()
	return time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.UTC)
}

func seedCommunity(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	id, err := s.Insert(ctx, &models.Community{Name: name, OwnerUserId: "test-owner-" + name})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return id
}

func seedUser(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, name string, am models.AuthMethod, createdAt int64) string {
	t.Helper()
	id, err := s.Insert(ctx, &models.User{
		Name:       name,
		Email:      name + "@example.com",
		AuthMethod: am,
		CreatedAt:  createdAt,
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestBuild_EmptyWindow(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.TotalMemberActionCount != 0 {
		t.Errorf("TotalMemberActionCount = %d, want 0", got.TotalMemberActionCount)
	}
	if got.TotalUserMessageCount != 0 {
		t.Errorf("TotalUserMessageCount = %d, want 0", got.TotalUserMessageCount)
	}
	if len(got.Communities) != 0 {
		t.Errorf("Communities = %d, want 0", len(got.Communities))
	}
	if got.ActivityTotals != (email.ActivityTotals{}) {
		t.Errorf("ActivityTotals should be zero value: %+v", got.ActivityTotals)
	}
}

func TestBuild_ActivityTotalsAggregateByEventType(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Test Coop")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	// Insert representative events of every aggregated type. UNDONE
	// events must be skipped.
	events := []struct {
		et    models.CommunityEventType
		tt    models.TransferType
		count int
	}{
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 3},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE, models.TransferType_TRANSFER_TYPE_LOAN, 2},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE, models.TransferType_TRANSFER_TYPE_GIVEAWAY, 1},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED, models.TransferType_TRANSFER_TYPE_LOAN, 1},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 4},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 2},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 1},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 2},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 5},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 2},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 1},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED, models.TransferType_TRANSFER_TYPE_UNSPECIFIED, 2},
		// UNDONE — must be excluded from totals AND keptEvents.
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE, models.TransferType_TRANSFER_TYPE_LOAN, 1},
	}
	ts := dayStart
	for _, e := range events {
		for range e.count {
			if _, err := store.Insert(ctx, &models.CommunityEvent{
				CommunityId:       commID,
				EventType:         e.et,
				TransferType:      e.tt,
				ActorId:           alice,
				OccurredAtUnixSec: ts,
			}); err != nil {
				t.Fatalf("insert event %v: %v", e.et, err)
			}
			ts++
		}
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	want := email.ActivityTotals{
		GearShared:          3,
		LoansStarted:        2,
		GiveawaysStarted:    1,
		LoansCompleted:      1,
		RequestsPosted:      4,
		RequestOffersMade:   2,
		RequestsFulfilled:   1,
		ExperiencesPosted:   2,
		RSVPYes:             5,
		RSVPMaybe:           2,
		RSVPNo:              1,
		InvitationsRedeemed: 2,
	}
	if got.ActivityTotals != want {
		t.Errorf("ActivityTotals mismatch.\nwant=%+v\ngot =%+v", want, got.ActivityTotals)
	}
	// UNDONE not counted as an action, but reconciled in the footer count.
	if got.TotalMemberActionCount != 26 { // 27 total - 1 undone
		t.Errorf("TotalMemberActionCount = %d, want 26", got.TotalMemberActionCount)
	}
	if got.UndoneEventCount != 1 {
		t.Errorf("UndoneEventCount = %d, want 1", got.UndoneEventCount)
	}
}

// TestBuild_ReconciliationEveryEventType inserts exactly one event of
// every CommunityEventType and asserts the registry-driven counting
// reconciles: counted + undone + internal == fetched. This is the
// runtime companion to TestEveryDigestEventTypeClassified — it would
// catch a registry entry whose disposition mis-buckets a type.
func TestBuild_ReconciliationEveryEventType(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Every Event")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	total := 0
	wantUndone := 0
	wantInternal := 0
	ts := dayStart
	for value := range models.CommunityEventType_name {
		et := models.CommunityEventType(value)
		if et == models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED {
			continue
		}
		switch Registry[et].Disposition {
		case DispositionExcludedRetraction:
			wantUndone++
		case DispositionExcludedInternal:
			wantInternal++
		}
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId:       commID,
			EventType:         et,
			TransferType:      models.TransferType_TRANSFER_TYPE_LOAN,
			ActorId:           alice,
			OccurredAtUnixSec: ts,
		}); err != nil {
			t.Fatalf("insert event %v: %v", et, err)
		}
		total++
		ts++
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.UndoneEventCount != wantUndone {
		t.Errorf("UndoneEventCount = %d, want %d", got.UndoneEventCount, wantUndone)
	}
	if got.InternalEventCount != wantInternal {
		t.Errorf("InternalEventCount = %d, want %d", got.InternalEventCount, wantInternal)
	}
	if got.TotalMemberActionCount != total-wantUndone-wantInternal {
		t.Errorf("TotalMemberActionCount = %d, want %d (= %d fetched - %d undone - %d internal)",
			got.TotalMemberActionCount, total-wantUndone-wantInternal, total, wantUndone, wantInternal)
	}
	if got.TotalMemberActionCount+got.UndoneEventCount+got.InternalEventCount != total {
		t.Errorf("counted %d + undone %d + internal %d != fetched %d",
			got.TotalMemberActionCount, got.UndoneEventCount, got.InternalEventCount, total)
	}
}

func TestBuild_ChatBreakdownByTopic(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Beta Hood")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	convTransfer, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId:      commID,
		ParticipantIds:   []string{alice},
		CreatedAtUnixSec: 1,
		Topic:            &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: "tr-1"}},
	})
	if err != nil {
		t.Fatalf("insert transfer conv: %v", err)
	}
	convRequest, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId:      commID,
		ParticipantIds:   []string{alice},
		CreatedAtUnixSec: 1,
		Topic:            &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "rq-1"}},
	})
	if err != nil {
		t.Fatalf("insert request conv: %v", err)
	}

	for i := range 3 {
		if _, err := store.Insert(ctx, &models.ChatMessage{
			ConversationId: convTransfer,
			SentAtUnixSec:  dayStart + int64(i),
			Message:        &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: alice, Text: "hi"}},
		}); err != nil {
			t.Fatalf("insert transfer chat: %v", err)
		}
	}
	if _, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: convRequest,
		SentAtUnixSec:  dayStart + 100,
		Message:        &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: alice, Text: "hi"}},
	}); err != nil {
		t.Fatalf("insert request chat: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.ChatByTopic.AboutTransfers != 3 {
		t.Errorf("AboutTransfers = %d, want 3", got.ChatByTopic.AboutTransfers)
	}
	if got.ChatByTopic.AboutRequests != 1 {
		t.Errorf("AboutRequests = %d, want 1", got.ChatByTopic.AboutRequests)
	}
	if got.TotalUserMessageCount != 4 {
		t.Errorf("TotalUserMessageCount = %d, want 4", got.TotalUserMessageCount)
	}
}

func TestBuild_OrdersCommunitiesByActivityScore(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commA := seedCommunity(t, ctx, store, "AAA")
	commB := seedCommunity(t, ctx, store, "BBB")
	commC := seedCommunity(t, ctx, store, "CCC")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	// A: 3 events (score 6). B: 10 chat msgs (score 10). C: 1 event (score 2).
	for i := range 3 {
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId: commA, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId: alice, OccurredAtUnixSec: dayStart + int64(i),
		}); err != nil {
			t.Fatalf("insert A event: %v", err)
		}
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: commC, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId: alice, OccurredAtUnixSec: dayStart + 1,
	}); err != nil {
		t.Fatalf("insert C event: %v", err)
	}
	convB, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId: commB, ParticipantIds: []string{alice}, CreatedAtUnixSec: 1,
	})
	if err != nil {
		t.Fatalf("insert conv: %v", err)
	}
	for i := range 10 {
		if _, err := store.Insert(ctx, &models.ChatMessage{
			ConversationId: convB, SentAtUnixSec: dayStart + int64(i),
			Message: &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: alice, Text: "hi"}},
		}); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Communities) != 3 {
		t.Fatalf("Communities = %d, want 3", len(got.Communities))
	}
	wantOrder := []string{"BBB", "AAA", "CCC"}
	for i, want := range wantOrder {
		if got.Communities[i].Name != want {
			t.Errorf("Communities[%d].Name = %q, want %q", i, got.Communities[i].Name, want)
		}
	}
}

func TestBuild_EngagementSection(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	carolID := seedUser(t, ctx, store, "Carol", models.AuthMethod_AUTH_METHOD_GOOGLE, dayStart+5)
	seedUser(t, ctx, store, "Dave", models.AuthMethod_AUTH_METHOD_PHONE, dayStart+10)
	returning := seedUser(t, ctx, store, "Existing", models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD, dayStart-100000)
	rotator := seedUser(t, ctx, store, "Rotator", models.AuthMethod_AUTH_METHOD_GOOGLE, dayStart-100000)
	if _, err := store.Insert(ctx, &models.RefreshToken{
		UserId: returning, TokenHash: "existing-hash", CreatedAtUnixSec: dayStart + 20, ExpiresAtUnixSec: dayStart + 4000,
		Origin: models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE,
	}); err != nil {
		t.Fatalf("insert returning refresh token: %v", err)
	}
	if _, err := store.Insert(ctx, &models.RefreshToken{
		UserId: carolID, TokenHash: "carol-hash", CreatedAtUnixSec: dayStart + 25, ExpiresAtUnixSec: dayStart + 4000,
		Origin: models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE,
	}); err != nil {
		t.Fatalf("insert carol refresh token: %v", err)
	}
	// A rotation-minted token is credential refresh, not a sign-in.
	if _, err := store.Insert(ctx, &models.RefreshToken{
		UserId: rotator, TokenHash: "rotator-hash", CreatedAtUnixSec: dayStart + 30, ExpiresAtUnixSec: dayStart + 4000,
		Origin: models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_ROTATION,
	}); err != nil {
		t.Fatalf("insert rotation refresh token: %v", err)
	}
	if _, err := store.Insert(ctx, &models.WaitlistEntry{
		Email: "wait@x.com", CreatedAtUnixSec: dayStart + 30,
	}); err != nil {
		t.Fatalf("insert waitlist: %v", err)
	}
	if _, err := store.Insert(ctx, &models.PendingPasswordReset{
		UserId: returning, Token: "tok", CreatedAtUnixSec: dayStart + 40, ExpiresAtUnixSec: dayStart + 4000,
	}); err != nil {
		t.Fatalf("insert password reset: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.NewUsers) != 2 {
		t.Errorf("NewUsers count = %d, want 2", len(got.NewUsers))
	}
	if got.InteractiveSignInCount != 1 {
		t.Errorf("InteractiveSignInCount = %d, want 1 (rotation + new signups excluded)", got.InteractiveSignInCount)
	}
}

func TestBuild_ActiveUsersAndContributors(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Actives")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	bob := seedUser(t, ctx, store, "Bob", models.AuthMethod_AUTH_METHOD_PHONE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	// alice acts (event); bob only chats; carol is only app-active.
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: commID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId: alice, OccurredAtUnixSec: dayStart + 10,
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	conv, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId: commID, ParticipantIds: []string{alice, bob}, CreatedAtUnixSec: 1,
	})
	if err != nil {
		t.Fatalf("insert conv: %v", err)
	}
	if _, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: conv, SentAtUnixSec: dayStart + 20,
		Message: &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: bob, Text: "hi"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	// Stamp table: coverage predates the window; alice + carol were
	// app-active in the window.
	if err := store.RecordUserActivity(ctx, alice, dayStart-90000); err != nil {
		t.Fatalf("record pre-window activity: %v", err)
	}
	if err := store.RecordUserActivity(ctx, alice, dayStart+100); err != nil {
		t.Fatalf("record alice activity: %v", err)
	}
	if err := store.RecordUserActivity(ctx, "carol", dayStart+200); err != nil {
		t.Fatalf("record carol activity: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.ActiveContributorCount != 2 { // alice (event) + bob (chat)
		t.Errorf("ActiveContributorCount = %d, want 2", got.ActiveContributorCount)
	}
	if !got.ActiveUserDataCoversWindow {
		t.Errorf("ActiveUserDataCoversWindow = false, want true (stamp predates window)")
	}
	if got.ActiveUserCount != 2 { // alice + carol
		t.Errorf("ActiveUserCount = %d, want 2", got.ActiveUserCount)
	}
	if len(got.DailyActives) != 0 {
		t.Errorf("daily digest should carry no DailyActives, got %+v", got.DailyActives)
	}
}

func TestBuildRange_WeeklyDailyActives(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Weekly")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC) // Monday
	end := start.AddDate(0, 0, 7)

	// Actions on Monday and Wednesday; app-activity on Tuesday.
	for _, dayOffset := range []int{0, 2} {
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId: commID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId: alice, OccurredAtUnixSec: start.AddDate(0, 0, dayOffset).Unix() + 3600,
		}); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}
	if err := store.RecordUserActivity(ctx, alice, start.Unix()-90000); err != nil {
		t.Fatalf("record pre-window activity: %v", err)
	}
	if err := store.RecordUserActivity(ctx, alice, start.AddDate(0, 0, 1).Unix()+3600); err != nil {
		t.Fatalf("record tuesday activity: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.BuildRange(ctx, start, end, email.DigestPeriodWeekly)
	if err != nil {
		t.Fatalf("BuildRange: %v", err)
	}

	if len(got.DailyActives) != 7 {
		t.Fatalf("DailyActives = %d rows, want 7", len(got.DailyActives))
	}
	if got.DailyActives[0].Contributors != 1 {
		t.Errorf("Monday contributors = %d, want 1", got.DailyActives[0].Contributors)
	}
	if got.DailyActives[1].ActiveUsers != 1 {
		t.Errorf("Tuesday actives = %d, want 1", got.DailyActives[1].ActiveUsers)
	}
	if got.DailyActives[1].Contributors != 0 {
		t.Errorf("Tuesday contributors = %d, want 0", got.DailyActives[1].Contributors)
	}
	if got.DailyActives[3].ActiveUsers != 0 {
		t.Errorf("Thursday actives = %d, want 0 (covered, no activity)", got.DailyActives[3].ActiveUsers)
	}
}

func TestBuild_CommunityRowContext(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	bob := seedUser(t, ctx, store, "Bob", models.AuthMethod_AUTH_METHOD_PHONE, 0)

	// Ad-hoc (nameless) community spun up around a gear item, with two
	// members and one counted event.
	gearID, err := store.Insert(ctx, &models.Gear{Name: "Canoe", OwnerId: alice})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	adHocID, err := store.Insert(ctx, &models.Community{
		OwnerUserId: alice,
		OriginItem:  &models.Community_OriginGearId{OriginGearId: gearID},
	})
	if err != nil {
		t.Fatalf("insert ad-hoc community: %v", err)
	}
	for _, uid := range []string{alice, bob} {
		if _, err := store.Insert(ctx, &models.CommunityUser{
			CommunityId: adHocID, UserId: uid, CreatedAtUnixSec: dayStart - 100,
		}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: adHocID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId: alice, OccurredAtUnixSec: dayStart + 5,
	}); err != nil {
		t.Fatalf("insert ad-hoc event: %v", err)
	}

	// Named community soft-deleted during the window — the digest counts
	// its COMMUNITY_DELETED event, so it must resolve (not "Unknown").
	deletedID, err := store.Insert(ctx, &models.Community{
		Name: "Old Crew", OwnerUserId: alice,
		Deleted: &models.DeletedMetadata{DeletedByUserId: alice, DeletedAtUnixSec: dayStart + 50},
	})
	if err != nil {
		t.Fatalf("insert deleted community: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: deletedID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
		ActorId: alice, OccurredAtUnixSec: dayStart + 50,
	}); err != nil {
		t.Fatalf("insert deleted event: %v", err)
	}

	// New user in the window whose earliest membership is the ad-hoc
	// community — their line should carry the ad-hoc label.
	newbie := seedUser(t, ctx, store, "Newbie", models.AuthMethod_AUTH_METHOD_PHONE, dayStart+10)
	if _, err := store.Insert(ctx, &models.CommunityUser{
		CommunityId: adHocID, UserId: newbie, CreatedAtUnixSec: dayStart + 11,
	}); err != nil {
		t.Fatalf("insert newbie membership: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var adHocRow, deletedRow *email.DigestCommunity
	for i := range got.Communities {
		switch {
		case got.Communities[i].IsAdHoc:
			adHocRow = &got.Communities[i]
		case got.Communities[i].Name == "Old Crew":
			deletedRow = &got.Communities[i]
		}
	}
	if adHocRow == nil {
		t.Fatalf("no ad-hoc row in %+v", got.Communities)
	}
	if adHocRow.OriginLabel != `gear "Canoe"` {
		t.Errorf("ad-hoc OriginLabel = %q, want `gear \"Canoe\"`", adHocRow.OriginLabel)
	}
	if adHocRow.MemberCount != 3 { // alice, bob, newbie
		t.Errorf("ad-hoc MemberCount = %d, want 3", adHocRow.MemberCount)
	}
	if len(adHocRow.TopCategories) != 1 || adHocRow.TopCategories[0].Label != "gear action" || adHocRow.TopCategories[0].Count != 1 {
		t.Errorf("ad-hoc TopCategories = %+v, want [{gear action 1}]", adHocRow.TopCategories)
	}
	if deletedRow == nil {
		t.Fatalf("no Old Crew row in %+v", got.Communities)
	}
	if !deletedRow.Deleted {
		t.Errorf("Old Crew row not marked Deleted")
	}

	var newbieUser *email.DigestNewUser
	for i := range got.NewUsers {
		if got.NewUsers[i].Name == "Newbie" {
			newbieUser = &got.NewUsers[i]
		}
	}
	if newbieUser == nil {
		t.Fatalf("Newbie missing from NewUsers: %+v", got.NewUsers)
	}
	if want := `ad hoc group around gear "Canoe"`; newbieUser.PrimaryCommunityName != want {
		t.Errorf("PrimaryCommunityName = %q, want %q", newbieUser.PrimaryCommunityName, want)
	}
}

func TestBuild_CommunityTailRollups(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)

	// 8 high-activity named communities fill the listed rows.
	for i := range 8 {
		cid := seedCommunity(t, ctx, store, fmt.Sprintf("Named-%d", i))
		for j := range 2 {
			if _, err := store.Insert(ctx, &models.CommunityEvent{
				CommunityId: cid, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
				ActorId: alice, OccurredAtUnixSec: dayStart + int64(i*10+j),
			}); err != nil {
				t.Fatalf("insert named event: %v", err)
			}
		}
	}
	// One low-activity named community and two ad-hoc communities land in
	// the tail rollups.
	lowID := seedCommunity(t, ctx, store, "Low")
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: lowID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId: alice, OccurredAtUnixSec: dayStart + 100,
	}); err != nil {
		t.Fatalf("insert low event: %v", err)
	}
	expID, err := store.Insert(ctx, &models.Experience{Name: "Picnic", OwnerId: alice})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	adHoc1, err := store.Insert(ctx, &models.Community{
		OwnerUserId: alice,
		OriginItem:  &models.Community_OriginExperienceId{OriginExperienceId: expID},
	})
	if err != nil {
		t.Fatalf("insert ad-hoc 1: %v", err)
	}
	adHoc2, err := store.Insert(ctx, &models.Community{OwnerUserId: alice})
	if err != nil {
		t.Fatalf("insert ad-hoc 2: %v", err)
	}
	for _, cid := range []string{adHoc1, adHoc2} {
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId: cid, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
			ActorId: alice, OccurredAtUnixSec: dayStart + 200,
		}); err != nil {
			t.Fatalf("insert ad-hoc event: %v", err)
		}
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(got.Communities) != 8 {
		t.Fatalf("listed rows = %d, want 8", len(got.Communities))
	}
	if got.NamedTailRollup == nil || got.NamedTailRollup.CommunityCount != 1 {
		t.Errorf("NamedTailRollup = %+v, want 1 community", got.NamedTailRollup)
	}
	if got.AdHocTailRollup == nil || got.AdHocTailRollup.CommunityCount != 2 {
		t.Fatalf("AdHocTailRollup = %+v, want 2 communities", got.AdHocTailRollup)
	}
	wantKinds := []email.DigestCategoryCount{
		{Label: "unknown origin", Count: 1},
		{Label: "around events", Count: 1},
	}
	gotKinds := got.AdHocTailRollup.KindCounts
	if len(gotKinds) != 2 {
		t.Fatalf("KindCounts = %+v, want 2 entries", gotKinds)
	}
	// Order among equal counts is by kind string ("" sorts first).
	if gotKinds[0] != wantKinds[0] || gotKinds[1] != wantKinds[1] {
		t.Errorf("KindCounts = %+v, want %+v", gotKinds, wantKinds)
	}
}

func TestBuild_PriorWindowDeltas(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commID := seedCommunity(t, ctx, store, "Deltas")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	priorDayStart := dayStart - 24*3600

	// Current window: 3 gear shares. Prior window: 1 gear share + 1
	// UNDONE row (must not count toward the prior action total).
	for i := range 3 {
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId: commID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId: alice, OccurredAtUnixSec: dayStart + int64(i),
		}); err != nil {
			t.Fatalf("insert current event: %v", err)
		}
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: commID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId: alice, OccurredAtUnixSec: priorDayStart + 10,
	}); err != nil {
		t.Fatalf("insert prior event: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId: commID, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		ActorId:      alice, OccurredAtUnixSec: priorDayStart + 20,
	}); err != nil {
		t.Fatalf("insert prior undone event: %v", err)
	}

	b := New(store, fixtureClock())
	got, err := b.Build(ctx, localDateInUTC(t, 2026, 5, 13))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Prior == nil {
		t.Fatal("Prior is nil, want prior-window metrics")
	}
	if got.Prior.TotalMemberActionCount != 1 {
		t.Errorf("Prior.TotalMemberActionCount = %d, want 1 (UNDONE excluded)", got.Prior.TotalMemberActionCount)
	}
	if got.Prior.ActivityTotals.GearShared != 1 {
		t.Errorf("Prior GearShared = %d, want 1", got.Prior.ActivityTotals.GearShared)
	}
	if got.Prior.ActiveContributorCount != 1 {
		t.Errorf("Prior.ActiveContributorCount = %d, want 1", got.Prior.ActiveContributorCount)
	}
	if got.Prior.WindowLabel != "2026-05-12" {
		t.Errorf("Prior.WindowLabel = %q, want 2026-05-12", got.Prior.WindowLabel)
	}
	if got.TotalMemberActionCount != 3 {
		t.Errorf("current TotalMemberActionCount = %d, want 3", got.TotalMemberActionCount)
	}
}

func TestBuild_QueryBudget(t *testing.T) {
	store, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	commA := seedCommunity(t, ctx, store, "Alpha")
	commB := seedCommunity(t, ctx, store, "Bravo")
	alice := seedUser(t, ctx, store, "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, 0)
	dayStart := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC).Unix()

	for _, comm := range []string{commA, commB} {
		if _, err := store.Insert(ctx, &models.CommunityEvent{
			CommunityId: comm, EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId: alice, OccurredAtUnixSec: dayStart + 5,
		}); err != nil {
			t.Fatalf("insert event: %v", err)
		}
		conv, err := store.Insert(ctx, &models.ChatConversation{
			CommunityId: comm, ParticipantIds: []string{alice}, CreatedAtUnixSec: 1,
		})
		if err != nil {
			t.Fatalf("insert conv: %v", err)
		}
		if _, err := store.Insert(ctx, &models.ChatMessage{
			ConversationId: conv, SentAtUnixSec: dayStart + 7,
			Message: &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: alice, Text: "hi"}},
		}); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}
	seedUser(t, ctx, store, "Newbie", models.AuthMethod_AUTH_METHOD_PHONE, dayStart+15)
	if _, err := store.Insert(ctx, &models.RefreshToken{
		UserId: alice, TokenHash: "alice-hash", CreatedAtUnixSec: dayStart + 20, ExpiresAtUnixSec: dayStart + 4000,
	}); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}
	if _, err := store.Insert(ctx, &models.WaitlistEntry{Email: "w@x", CreatedAtUnixSec: dayStart + 25}); err != nil {
		t.Fatalf("insert waitlist: %v", err)
	}
	if _, err := store.Insert(ctx, &models.PendingPasswordReset{
		UserId: alice, Token: "tok", CreatedAtUnixSec: dayStart + 30, ExpiresAtUnixSec: dayStart + 4000,
	}); err != nil {
		t.Fatalf("insert reset: %v", err)
	}

	// Budget, itemized: 9 parallel source queries (events, chat by
	// community, chat by topic, distinct chat senders, new users,
	// waitlist, password resets, active spans, earliest activity) +
	// 1 interactive sign-in count + 1 new-user earliest-community lookup
	// + 1 batch community lookup + 1 member-count GROUP BY + 5
	// prior-window headline queries (events, chat by community, distinct
	// senders, user count, active spans) = 18 baseline (origin-item
	// resolution adds none here — all fixture communities are named).
	// Small cushion for internal storage-layer overhead.
	const maxQueries = 21
	statsCtx := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		b := New(store, fixtureClock())
		if _, err := b.Build(statsCtx, localDateInUTC(t, 2026, 5, 13)); err != nil {
			t.Fatalf("Build: %v", err)
		}
	})
}
