package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestFindEventsInWindow(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000_000, 2_000_000
	// Inside the window.
	mustInsertEvent(t, ctx, store, "comm-a", start+10, models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED)
	mustInsertEvent(t, ctx, store, "comm-b", start+50, models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED)
	// Boundary: start is inclusive, end is exclusive.
	mustInsertEvent(t, ctx, store, "comm-a", start, models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT)
	mustInsertEvent(t, ctx, store, "comm-a", end, models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT)
	// Outside.
	mustInsertEvent(t, ctx, store, "comm-a", start-1, models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT)
	mustInsertEvent(t, ctx, store, "comm-a", end+5, models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT)
	got, err := store.FindEventsInWindow(ctx, start, end)
	if err != nil {
		t.Fatalf("FindEventsInWindow: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events in window (incl. start, excl. end), got %d", len(got))
	}
	// Order is ASC by occurred_at.
	wantOrder := []int64{start, start + 10, start + 50}
	for i, ev := range got {
		if ev.OccurredAtUnixSec != wantOrder[i] {
			t.Errorf("event %d occurred_at = %d, want %d", i, ev.OccurredAtUnixSec, wantOrder[i])
		}
	}
}

func TestFindEventsInWindow_RejectsBadRange(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	if _, err := store.FindEventsInWindow(context.Background(), 10, 10); err == nil {
		t.Error("expected error when end == start")
	}
}

func TestCountUserMessagesByCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000_000, 2_000_000

	convA := mustInsertConversation(t, ctx, store, "comm-a")
	convA2 := mustInsertConversation(t, ctx, store, "comm-a")
	convB := mustInsertConversation(t, ctx, store, "comm-b")

	// comm-a: two user messages from two senders across two conversations.
	mustInsertChatUserMessage(t, ctx, store, convA, "alice", start+10)
	mustInsertChatUserMessage(t, ctx, store, convA2, "bob", start+20)
	// System message — skipped.
	mustInsertChatSystemMessage(t, ctx, store, convA, start+30)
	// Soft-deleted user message — skipped.
	delID := mustInsertChatUserMessage(t, ctx, store, convA, "alice", start+40)
	softDeleteChatMessage(t, ctx, store, delID)
	// Out-of-window — skipped.
	mustInsertChatUserMessage(t, ctx, store, convA, "alice", end+1)

	// comm-b: one user message.
	mustInsertChatUserMessage(t, ctx, store, convB, "carol", start+5)

	got, err := store.CountUserMessagesByCommunity(ctx, start, end)
	if err != nil {
		t.Fatalf("CountUserMessagesByCommunity: %v", err)
	}
	byCommunity := map[string]ChatActivityRow{}
	for _, r := range got {
		byCommunity[r.CommunityID] = r
	}
	a, ok := byCommunity["comm-a"]
	if !ok {
		t.Fatal("comm-a missing from results")
	}
	if a.MessageCount != 2 {
		t.Errorf("comm-a MessageCount = %d, want 2", a.MessageCount)
	}
	if a.DistinctSenderCount != 2 {
		t.Errorf("comm-a DistinctSenderCount = %d, want 2", a.DistinctSenderCount)
	}
	if a.DistinctConversationCount != 2 {
		t.Errorf("comm-a DistinctConversationCount = %d, want 2", a.DistinctConversationCount)
	}
	b, ok := byCommunity["comm-b"]
	if !ok {
		t.Fatal("comm-b missing from results")
	}
	if b.MessageCount != 1 {
		t.Errorf("comm-b MessageCount = %d, want 1", b.MessageCount)
	}
}

func TestCountUserMessagesByTopicType(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000_000, 2_000_000

	// Conversations scoped to four different topic types.
	transferConv := mustInsertConversationWithTopic(t, ctx, store, &models.ConversationTopic{
		TopicId: &models.ConversationTopic_TransferId{TransferId: "tr-1"},
	})
	requestConv := mustInsertConversationWithTopic(t, ctx, store, &models.ConversationTopic{
		TopicId: &models.ConversationTopic_RequestId{RequestId: "rq-1"},
	})
	experienceConv := mustInsertConversationWithTopic(t, ctx, store, &models.ConversationTopic{
		TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: "ex-1"},
	})
	gearConv := mustInsertConversationWithTopic(t, ctx, store, &models.ConversationTopic{
		TopicId: &models.ConversationTopic_GearId{GearId: "g-1"},
	})
	communityConv := mustInsertConversationWithTopic(t, ctx, store, &models.ConversationTopic{
		TopicId: &models.ConversationTopic_CommunityId{CommunityId: "c-1"},
	})

	mustInsertChatUserMessage(t, ctx, store, transferConv, "u1", start+10)
	mustInsertChatUserMessage(t, ctx, store, transferConv, "u2", start+20)
	mustInsertChatUserMessage(t, ctx, store, requestConv, "u1", start+30)
	mustInsertChatUserMessage(t, ctx, store, experienceConv, "u3", start+40)
	mustInsertChatUserMessage(t, ctx, store, gearConv, "u1", start+50)
	mustInsertChatUserMessage(t, ctx, store, communityConv, "u4", start+60)
	// System message — skipped.
	mustInsertChatSystemMessage(t, ctx, store, transferConv, start+70)

	got, err := store.CountUserMessagesByTopicType(ctx, start, end)
	if err != nil {
		t.Fatalf("CountUserMessagesByTopicType: %v", err)
	}
	if got.Transfer != 2 {
		t.Errorf("Transfer = %d, want 2", got.Transfer)
	}
	if got.Request != 1 {
		t.Errorf("Request = %d, want 1", got.Request)
	}
	if got.Experience != 1 {
		t.Errorf("Experience = %d, want 1", got.Experience)
	}
	if got.Gear != 1 {
		t.Errorf("Gear = %d, want 1", got.Gear)
	}
	if got.Community != 1 {
		t.Errorf("Community = %d, want 1", got.Community)
	}
	if got.Total() != 6 {
		t.Errorf("Total = %d, want 6", got.Total())
	}
}

func TestFindUsersCreatedInWindow_AndSoftDelete(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000, 2_000
	insideID := mustInsertUser(t, ctx, store, "alice@example.com", "Alice", models.AuthMethod_AUTH_METHOD_GOOGLE, start+10)
	mustInsertUser(t, ctx, store, "bob@example.com", "Bob", models.AuthMethod_AUTH_METHOD_APPLE, end-1)
	mustInsertUser(t, ctx, store, "stale@example.com", "Stale", models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD, start-1)
	mustInsertUser(t, ctx, store, "future@example.com", "Future", models.AuthMethod_AUTH_METHOD_PHONE, end+1)
	deletedID := mustInsertUser(t, ctx, store, "deleted@example.com", "Deleted", models.AuthMethod_AUTH_METHOD_GOOGLE, start+5)
	softDeleteUser(t, ctx, store, deletedID)

	got, err := store.FindUsersCreatedInWindow(ctx, start, end)
	if err != nil {
		t.Fatalf("FindUsersCreatedInWindow: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 users, got %d", len(got))
	}
	if got[0].Id != insideID {
		t.Errorf("first user id = %s, want %s", got[0].Id, insideID)
	}
}

func TestCountDistinctInteractiveSignInsInWindow(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000, 2_000
	interactive := models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE

	// alice: signed in twice → counts as 1 distinct user.
	mustInsertRefreshToken(t, ctx, store, "alice", start+1, interactive)
	mustInsertRefreshToken(t, ctx, store, "alice", start+50, interactive)
	// bob: signed in once.
	mustInsertRefreshToken(t, ctx, store, "bob", start+100, interactive)
	// carol: brand-new signup whose registration token shouldn't count
	// toward returning sign-ins.
	mustInsertRefreshToken(t, ctx, store, "carol", start+200, interactive)
	// dave: interactive token outside window.
	mustInsertRefreshToken(t, ctx, store, "dave", end+1, interactive)
	// erin: rotation-minted token in window — credential refresh, not a
	// sign-in.
	mustInsertRefreshToken(t, ctx, store, "erin", start+300, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_ROTATION)
	// frank: legacy row predating origin stamping — unknown, not counted.
	mustInsertRefreshToken(t, ctx, store, "frank", start+400, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_UNSPECIFIED)

	got, err := store.CountDistinctInteractiveSignInsInWindow(ctx, start, end, []string{"carol"})
	if err != nil {
		t.Fatalf("CountDistinctInteractiveSignInsInWindow: %v", err)
	}
	if got != 2 {
		t.Errorf("CountDistinctInteractiveSignInsInWindow = %d, want 2 (alice + bob)", got)
	}

	// With no exclusions, carol counts too; rotation/legacy still don't.
	got2, err := store.CountDistinctInteractiveSignInsInWindow(ctx, start, end, nil)
	if err != nil {
		t.Fatalf("CountDistinctInteractiveSignInsInWindow nil exclude: %v", err)
	}
	if got2 != 3 {
		t.Errorf("CountDistinctInteractiveSignInsInWindow nil exclude = %d, want 3", got2)
	}
}

func TestUserActiveDayLifecycle(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Stamp alice twice the same UTC day (span widens, one row), bob
	// once, and carol on a much earlier day (prunable).
	day := time.Date(2026, 5, 13, 8, 0, 0, 0, time.UTC).Unix()
	if err := store.RecordUserActivity(ctx, "alice", day); err != nil {
		t.Fatalf("RecordUserActivity alice: %v", err)
	}
	if err := store.RecordUserActivity(ctx, "alice", day+7200); err != nil {
		t.Fatalf("RecordUserActivity alice 2: %v", err)
	}
	if err := store.RecordUserActivity(ctx, "bob", day+100); err != nil {
		t.Fatalf("RecordUserActivity bob: %v", err)
	}
	old := day - 500*24*3600
	if err := store.RecordUserActivity(ctx, "carol", old); err != nil {
		t.Fatalf("RecordUserActivity carol: %v", err)
	}

	spans, err := store.FindUserActiveSpansInWindow(ctx, day-3600, day+10000)
	if err != nil {
		t.Fatalf("FindUserActiveSpansInWindow: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2 (alice merged + bob): %+v", len(spans), spans)
	}
	for _, span := range spans {
		if span.UserID == "alice" {
			if span.FirstSeenUnixSec != day || span.LastSeenUnixSec != day+7200 {
				t.Errorf("alice span = [%d, %d], want [%d, %d]",
					span.FirstSeenUnixSec, span.LastSeenUnixSec, day, day+7200)
			}
		}
	}

	earliest, err := store.EarliestUserActivityUnixSec(ctx)
	if err != nil {
		t.Fatalf("EarliestUserActivityUnixSec: %v", err)
	}
	if earliest != old {
		t.Errorf("earliest = %d, want %d", earliest, old)
	}

	pruned, err := store.PruneUserActivityBefore(ctx, day-400*24*3600)
	if err != nil {
		t.Fatalf("PruneUserActivityBefore: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1 (carol)", pruned)
	}
}

func TestEarliestUserActivityUnixSec_EmptyTable(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	earliest, err := store.EarliestUserActivityUnixSec(context.Background())
	if err != nil {
		t.Fatalf("EarliestUserActivityUnixSec: %v", err)
	}
	if earliest != 0 {
		t.Errorf("earliest on empty table = %d, want 0", earliest)
	}
}

func TestFindDistinctChatSenderIDsInWindow(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000, 2_000
	conv, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId: "comm-1", ParticipantIds: []string{"alice", "bob"}, CreatedAtUnixSec: 1,
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	insertMsg := func(sender string, at int64) {
		t.Helper()
		if _, err := store.Insert(ctx, &models.ChatMessage{
			ConversationId: conv, SentAtUnixSec: at,
			Message: &models.ChatMessage_UserMessage{UserMessage: &models.UserChatMessage{SenderId: sender, Text: "hi"}},
		}); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}
	insertMsg("alice", start+1)
	insertMsg("alice", start+2)
	insertMsg("bob", start+3)
	insertMsg("carol", end+1) // outside window

	got, err := store.FindDistinctChatSenderIDsInWindow(ctx, start, end)
	if err != nil {
		t.Fatalf("FindDistinctChatSenderIDsInWindow: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("senders = %v, want 2 distinct (alice, bob)", got)
	}
}

func TestFindWaitlistSignupsInWindow(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000, 2_000
	mustInsertWaitlistEntry(t, ctx, store, "in@x.com", start+1)
	mustInsertWaitlistEntry(t, ctx, store, "out@x.com", start-1)

	got, err := store.FindWaitlistSignupsInWindow(ctx, start, end)
	if err != nil {
		t.Fatalf("FindWaitlistSignupsInWindow: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 waitlist entry, got %d", len(got))
	}
	if got[0].Email != "in@x.com" {
		t.Errorf("email = %s, want in@x.com", got[0].Email)
	}
}

func TestCountPasswordResetsInWindow(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const start, end int64 = 1_000, 2_000
	mustInsertPasswordReset(t, ctx, store, "u1", start+1)
	mustInsertPasswordReset(t, ctx, store, "u2", start+10)
	mustInsertPasswordReset(t, ctx, store, "u3", end+1) // outside

	got, err := store.CountPasswordResetsInWindow(ctx, start, end)
	if err != nil {
		t.Fatalf("CountPasswordResetsInWindow: %v", err)
	}
	if got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

func TestClaimActivityDigestSend(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const day1 = "2026-05-13"
	const day2 = "2026-05-14"

	// First call wins.
	ok, err := store.ClaimActivityDigestSend(ctx, CadenceDaily, day1)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !ok {
		t.Fatal("first call should have won the claim")
	}

	// Second same-day call loses.
	ok2, err := store.ClaimActivityDigestSend(ctx, CadenceDaily, day1)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if ok2 {
		t.Fatal("second same-day call should not have won")
	}

	// Next-day call wins.
	ok3, err := store.ClaimActivityDigestSend(ctx, CadenceDaily, day2)
	if err != nil {
		t.Fatalf("third claim: %v", err)
	}
	if !ok3 {
		t.Fatal("next-day call should have won")
	}

	last, err := store.GetActivityDigestLastSentKey(ctx, CadenceDaily)
	if err != nil {
		t.Fatalf("GetActivityDigestLastSentKey: %v", err)
	}
	if last != day2 {
		t.Errorf("last sent key = %s, want %s", last, day2)
	}

	// Empty input is rejected.
	if _, err := store.ClaimActivityDigestSend(ctx, CadenceDaily, ""); err == nil {
		t.Error("expected error on empty runKey")
	}

	// Weekly cadence claims independently — daily claims don't block it.
	const week1 = "2026-W19"
	ok4, err := store.ClaimActivityDigestSend(ctx, CadenceWeekly, week1)
	if err != nil {
		t.Fatalf("weekly first claim: %v", err)
	}
	if !ok4 {
		t.Fatal("weekly first claim should have won despite daily already claiming")
	}
	ok5, err := store.ClaimActivityDigestSend(ctx, CadenceWeekly, week1)
	if err != nil {
		t.Fatalf("weekly second claim: %v", err)
	}
	if ok5 {
		t.Fatal("weekly second same-week claim should not have won")
	}
}

// ---- helpers ----.

func mustInsertEvent(
	t *testing.T, ctx context.Context, store *ProtoSQLStorage,
	communityID string, occurredAtUnixSec int64, et models.CommunityEventType,
) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         et,
		ActorId:           "actor-" + communityID,
		OccurredAtUnixSec: occurredAtUnixSec,
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	return id
}

func mustInsertConversationWithTopic(t *testing.T, ctx context.Context, store *ProtoSQLStorage, topic *models.ConversationTopic) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId:      "comm-shared",
		ParticipantIds:   []string{"alice"},
		CreatedAtUnixSec: 1,
		Topic:            topic,
	})
	if err != nil {
		t.Fatalf("insert conversation with topic: %v", err)
	}
	return id
}

func mustInsertConversation(t *testing.T, ctx context.Context, store *ProtoSQLStorage, communityID string) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.ChatConversation{
		CommunityId:      communityID,
		ParticipantIds:   []string{"alice"},
		CreatedAtUnixSec: 1,
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	return id
}

func mustInsertChatUserMessage(
	t *testing.T, ctx context.Context, store *ProtoSQLStorage,
	conversationID, senderID string, sentAt int64,
) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: conversationID,
		SentAtUnixSec:  sentAt,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     "hello",
			},
		},
	})
	if err != nil {
		t.Fatalf("insert chat user message: %v", err)
	}
	return id
}

func mustInsertChatSystemMessage(t *testing.T, ctx context.Context, store *ProtoSQLStorage, conversationID string, sentAt int64) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: conversationID,
		SentAtUnixSec:  sentAt,
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				Action:      models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
				Description: "system",
			},
		},
	})
	if err != nil {
		t.Fatalf("insert chat system message: %v", err)
	}
	return id
}

func softDeleteChatMessage(t *testing.T, ctx context.Context, store *ProtoSQLStorage, id string) {
	t.Helper()
	msg := &models.ChatMessage{}
	if err := store.GetByID(ctx, id, msg); err != nil {
		t.Fatalf("get chat message for soft delete: %v", err)
	}
	msg.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  "tester",
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := store.Update(ctx, msg); err != nil {
		t.Fatalf("update soft delete chat message: %v", err)
	}
}

func mustInsertUser(
	t *testing.T, ctx context.Context, store *ProtoSQLStorage,
	email, name string, am models.AuthMethod, createdAt int64,
) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.User{
		Email:      email,
		Name:       name,
		AuthMethod: am,
		CreatedAt:  createdAt,
	})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func softDeleteUser(t *testing.T, ctx context.Context, store *ProtoSQLStorage, id string) {
	t.Helper()
	u := &models.User{}
	if err := store.GetByID(ctx, id, u); err != nil {
		t.Fatalf("get user for soft delete: %v", err)
	}
	u.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  "tester",
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := store.Update(ctx, u); err != nil {
		t.Fatalf("update soft delete user: %v", err)
	}
}

func mustInsertRefreshToken(t *testing.T, ctx context.Context, store *ProtoSQLStorage, userID string, createdAt int64, origin models.RefreshTokenOrigin) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.RefreshToken{
		UserId:           userID,
		TokenHash:        fmt.Sprintf("%s-hash-%d", userID, createdAt),
		ExpiresAtUnixSec: createdAt + 3600,
		CreatedAtUnixSec: createdAt,
		Origin:           origin,
		IsRevoked:        false,
	})
	if err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}
	return id
}

func mustInsertWaitlistEntry(t *testing.T, ctx context.Context, store *ProtoSQLStorage, email string, createdAt int64) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.WaitlistEntry{
		Email:            email,
		CreatedAtUnixSec: createdAt,
	})
	if err != nil {
		t.Fatalf("insert waitlist entry: %v", err)
	}
	return id
}

func mustInsertPasswordReset(t *testing.T, ctx context.Context, store *ProtoSQLStorage, userID string, createdAt int64) string {
	t.Helper()
	id, err := store.Insert(ctx, &models.PendingPasswordReset{
		UserId:           userID,
		Token:            userID + "-token",
		CreatedAtUnixSec: createdAt,
		ExpiresAtUnixSec: createdAt + 3600,
	})
	if err != nil {
		t.Fatalf("insert password reset: %v", err)
	}
	return id
}
