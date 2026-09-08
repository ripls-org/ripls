package chat

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestCoalesce_RSVPWithinWindow verifies YES→MAYBE→YES collapses to one message.
func TestCoalesce_RSVPWithinWindow(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	mockBus := chat_event_bus.NewMockBus()
	writer := NewSystemMessageWriter(sqlStorage, mockBus)

	t0 := time.Unix(1_000_000, 0)
	ctx1 := clock.WithSimulationTime(ctx, t0)
	if err := writer.InsertSystemMessage(ctx1, convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "actor1 RSVPed Yes"); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	ctx2 := clock.WithSimulationTime(ctx, t0.Add(1*time.Minute))
	if err := writer.InsertSystemMessage(ctx2, convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE, "actor1 RSVPed Maybe"); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	ctx3 := clock.WithSimulationTime(ctx, t0.Add(2*time.Minute))
	if err := writer.InsertSystemMessage(ctx3, convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "actor1 RSVPed Yes again"); err != nil {
		t.Fatalf("third insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after coalesce, got %d", len(msgs))
	}

	msg := msgs[0].(*models.ChatMessage)
	sys := msg.GetSystemMessage()
	if sys.Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES {
		t.Errorf("action = %v, want RSVP_YES", sys.Action)
	}
	if sys.Description != "actor1 RSVPed Yes again" {
		t.Errorf("description = %q, want final text", sys.Description)
	}

	// First publish is KindSystemMessage (fresh insert); next two are KindSystemMessageUpdate (coalesce).
	calls := mockBus.Captured()
	if len(calls) != 3 {
		t.Fatalf("expected 3 bus publish calls, got %d", len(calls))
	}
	if calls[0].Kind != chat_event_bus.KindSystemMessage {
		t.Errorf("first publish kind = %v, want KindSystemMessage", calls[0].Kind)
	}
	if calls[1].Kind != chat_event_bus.KindSystemMessageUpdate {
		t.Errorf("second publish kind = %v, want KindSystemMessageUpdate", calls[1].Kind)
	}
	if calls[2].Kind != chat_event_bus.KindSystemMessageUpdate {
		t.Errorf("third publish kind = %v, want KindSystemMessageUpdate", calls[2].Kind)
	}
}

// TestCoalesce_LocalizedPayloadMovesWithAction verifies the
// structured-template payload moves with the action on coalesce.
// Regression guard: prior to the fix, the coalesce path updated
// Action + Description but left TemplateKey / TemplateParams stale,
// so an RSVP_YES → RSVP_MAYBE coalesce kept template_key="chat.experience.rsvp_yes"
// and the client rendered "X said yes" forever.
func TestCoalesce_LocalizedPayloadMovesWithAction(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	// Initial localized YES.
	ctx1 := clock.WithSimulationTime(ctx, t0)
	if err := writer.InsertLocalized(ctx1, convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES,
		RSVPYesMessage("Alice")); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Coalesce to localized MAYBE within the window.
	ctx2 := clock.WithSimulationTime(ctx, t0.Add(1*time.Minute))
	if err := writer.InsertLocalized(ctx2, convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE,
		RSVPMaybeMessage("Alice")); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after coalesce, got %d", len(msgs))
	}

	sys := msgs[0].(*models.ChatMessage).GetSystemMessage()
	if sys.GetTemplateKey() != "chat.experience.rsvp_maybe" {
		t.Errorf("TemplateKey = %q after coalesce; want %q (must move with Action)",
			sys.GetTemplateKey(), "chat.experience.rsvp_maybe")
	}
	if sys.Description != "Alice might attend" {
		t.Errorf("Description = %q after coalesce; want updated text", sys.Description)
	}
}

// TestCoalesce_LocalizedCoalescedWithLegacyClearsTemplateKey verifies
// that coalescing a legacy (text-only) emit over a previously-localized
// one clears the stale TemplateKey so the client falls back to the new
// literal text rather than rendering the old template.
func TestCoalesce_LocalizedCoalescedWithLegacyClearsTemplateKey(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	// First emit: localized YES.
	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0),
		convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES,
		RSVPYesMessage("Alice"),
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second emit: legacy text-only path (e.g. a not-yet-migrated
	// emit site, or a custom one-off message) coalesces in.
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0.Add(30*time.Second)),
		convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE,
		"actor1 RSVPed Maybe (legacy text)",
	); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	sys := msgs[0].(*models.ChatMessage).GetSystemMessage()
	if sys.TemplateKey != nil {
		t.Errorf("TemplateKey = %q after legacy coalesce; want nil (must clear)",
			sys.GetTemplateKey())
	}
	if len(sys.TemplateParams) != 0 {
		t.Errorf("TemplateParams = %v; want empty after legacy coalesce", sys.TemplateParams)
	}
}

// TestCoalesce_OutsideWindowInsertsNew verifies a second RSVP after the window creates a new message.
func TestCoalesce_OutsideWindowInsertsNew(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)

	t0 := time.Unix(1_000_000, 0)
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "RSVPed Yes",
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// 6 minutes later — outside the 5-minute window.
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0.Add(6*time.Minute)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_NO, "RSVPed No",
	); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (outside window), got %d", len(msgs))
	}
}

// TestCoalesce_DifferentActorsNeverCoalesce verifies actor isolation.
func TestCoalesce_DifferentActorsNeverCoalesce(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "actor2"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "actor1 Yes",
	); err != nil {
		t.Fatalf("actor1 insert: %v", err)
	}
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0.Add(1*time.Minute)), convID, "actor2",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "actor2 Yes",
	); err != nil {
		t.Fatalf("actor2 insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (different actors), got %d", len(msgs))
	}
}

// TestCoalesce_NonCoalescingActionAlwaysInserts verifies creation anchors and one-shot
// actions (e.g., APPROVED) are never coalesced.
func TestCoalesce_NonCoalescingActionAlwaysInserts(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	nonCoalescingActions := []models.ChatSystemAction{
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED,
	}

	for i, action := range nonCoalescingActions {
		if err := writer.InsertSystemMessage(
			clock.WithSimulationTime(ctx, t0.Add(time.Duration(i)*time.Second)),
			convID, "actor1", action, action.String(),
		); err != nil {
			t.Fatalf("insert %v: %v", action, err)
		}
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != len(nonCoalescingActions) {
		t.Fatalf("expected %d messages (no coalescing), got %d", len(nonCoalescingActions), len(msgs))
	}
}

// TestCoalesce_ReadStatusOnCoalesce verifies actor stays read, others become unread.
func TestCoalesce_ReadStatusOnCoalesce(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	participants := []string{"actor1", "other1", "other2"}
	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: participants,
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "Yes",
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Simulate other1 reading the first message.
	msgs, _ := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	first := msgs[0].(*models.ChatMessage)
	first.ParticipantIdToIsRead["other1"] = true
	_ = sqlStorage.Update(ctx, first)

	// Coalesce.
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0.Add(1*time.Minute)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE, "Maybe",
	); err != nil {
		t.Fatalf("coalesce insert: %v", err)
	}

	msgs, err = sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after coalesce, got %d", len(msgs))
	}

	updated := msgs[0].(*models.ChatMessage)
	if !updated.ParticipantIdToIsRead["actor1"] {
		t.Error("actor1 should be read (actor of the action)")
	}
	if updated.ParticipantIdToIsRead["other1"] {
		t.Error("other1 should be unread after coalesce")
	}
	if updated.ParticipantIdToIsRead["other2"] {
		t.Error("other2 should be unread after coalesce")
	}
}

// TestCoalesce_ConversationTimestampAdvances verifies last_message_at_unix_sec is bumped.
func TestCoalesce_ConversationTimestampAdvances(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)
	t1 := t0.Add(2 * time.Minute)

	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "Yes",
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t1), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_NO, "No",
	); err != nil {
		t.Fatalf("coalesce insert: %v", err)
	}

	conv := &models.ChatConversation{}
	if err := sqlStorage.GetByID(ctx, convID, conv); err != nil {
		t.Fatalf("fetch conversation: %v", err)
	}
	if conv.LastMessageAtUnixSec != t1.Unix() {
		t.Errorf("LastMessageAtUnixSec = %d, want %d", conv.LastMessageAtUnixSec, t1.Unix())
	}
}

// TestCoalesce_OQuery verifies the coalesce path issues a bounded number of queries (O(1) per insert).
func TestCoalesce_OQuery(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	baseCtx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(baseCtx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)

	// Seed the first message.
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(baseCtx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES, "Yes",
	); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Now measure the coalesce path.
	ctx := storage.WithQueryStats(clock.WithSimulationTime(baseCtx, t0.Add(1*time.Minute)))
	storage.AssertMaxQueries(t, ctx, 6, func() {
		// Expect: GetByID(conversation) + QueryByFields(candidates) +
		// Update(message) + media_ids sync UPDATE on the message +
		// Update(conversation) + participant_ids sync UPDATE on the
		// conversation. The two sync UPDATEs are the +2 added by the
		// ChatMessage.media_ids (#1529) and ChatConversation.participant_ids
		// (#2072) TEXT[] denormalizations — constant cost per write,
		// doesn't violate O(1).
		if err := writer.InsertSystemMessage(ctx, convID, "actor1",
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_MAYBE, "Maybe"); err != nil {
			t.Fatalf("coalesce insert: %v", err)
		}
	})
}

// TestCoalesce_CoalesceKeyIsolatesMessages verifies that messages with different coalesce
// keys in the same family never merge with each other.
func TestCoalesce_CoalesceKeyIsolatesMessages(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "other1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)
	keyA := "need-aaa"
	keyB := "need-bbb"

	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_ADDED, "added A",
		SystemMessageInsertOptions{CoalesceKey: &keyA},
	); err != nil {
		t.Fatalf("insert A: %v", err)
	}
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0.Add(1*time.Minute)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_REMOVED, "removed B",
		SystemMessageInsertOptions{CoalesceKey: &keyB},
	); err != nil {
		t.Fatalf("insert B: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	// Different keys → no coalesce → two separate messages.
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (different coalesce keys), got %d", len(msgs))
	}
}

// TestCoalesce_KeyedClaimSupersedesKeylessOffer verifies the #2724
// single-line-per-action chain: "X offered to help" (OFFERED, keyless) is
// superseded in place by the keyed claim line, whose key is upgraded onto the
// row so the later gear-backed escalation line (same key) coalesces too —
// three emits, one visible line carrying the richest info.
func TestCoalesce_KeyedClaimSupersedesKeylessOffer(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "requester1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)
	contribID := "contrib-1"

	// 1. Plain offer — keyless.
	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED,
		OfferedToHelpMessage("Lisa"),
	); err != nil {
		t.Fatalf("offer insert: %v", err)
	}

	// 2. Claim — keyed by the contribution. Must supersede the offer line.
	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0.Add(10*time.Second)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED,
		PlanningNeedClaimedMessage("Lisa", "Lawn mower", ""),
		SystemMessageInsertOptions{CoalesceKey: &contribID},
	); err != nil {
		t.Fatalf("claim insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after claim supersedes offer, got %d", len(msgs))
	}
	sys := msgs[0].(*models.ChatMessage).GetSystemMessage()
	if sys.GetCoalesceKey() != contribID {
		t.Errorf("CoalesceKey = %q after keyed coalesce; want %q (must upgrade)", sys.GetCoalesceKey(), contribID)
	}
	if sys.Description != "Lisa is bringing: Lawn mower" {
		t.Errorf("Description = %q; want claim text", sys.Description)
	}

	// 3. Gear-backed escalation — same key. Must coalesce onto the same row.
	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0.Add(20*time.Second)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED,
		GearOfferedForNeedMessage("Lisa", "Lawn mower", "Honda mower", false),
		SystemMessageInsertOptions{CoalesceKey: &contribID},
	); err != nil {
		t.Fatalf("gear-offer insert: %v", err)
	}

	msgs, err = sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after full chain, got %d", len(msgs))
	}
	sys = msgs[0].(*models.ChatMessage).GetSystemMessage()
	if sys.Description != "Lisa is bringing Lawn mower — lending Honda mower" {
		t.Errorf("Description = %q; want combined line", sys.Description)
	}
	if sys.GetTemplateKey() != "chat.request.gear_offer_lend_for_need" {
		t.Errorf("TemplateKey = %q; want gear_offer_lend_for_need", sys.GetTemplateKey())
	}
}

// TestCoalesce_KeylessNeverSupersedesKeyed verifies the upgrade-only rule: a
// keyless emit ("X offered to help" arriving after a claim) must not clobber
// the richer keyed line — it inserts its own row instead.
func TestCoalesce_KeylessNeverSupersedesKeyed(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1", "requester1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)
	contribID := "contrib-1"

	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED,
		PlanningNeedClaimedMessage("Lisa", "Lawn mower", ""),
		SystemMessageInsertOptions{CoalesceKey: &contribID},
	); err != nil {
		t.Fatalf("claim insert: %v", err)
	}
	if err := writer.InsertLocalized(
		clock.WithSimulationTime(ctx, t0.Add(10*time.Second)), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_OFFERED,
		OfferedToHelpMessage("Lisa"),
	); err != nil {
		t.Fatalf("offer insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (keyless must not clobber keyed), got %d", len(msgs))
	}
}

// TestInsertSystemMessage_SentAtOverride verifies the SentAtUnixSec option is
// stored verbatim, so batch emitters can force a deterministic same-second
// order (#2724).
func TestInsertSystemMessage_SentAtOverride(t *testing.T) {
	sqlStorage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "c1",
		ParticipantIds: []string{"actor1"},
	}
	convID, err := sqlStorage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	writer := NewSystemMessageWriter(sqlStorage, nil)
	t0 := time.Unix(1_000_000, 0)
	override := t0.Unix() + 3

	// Use distinct coalesce keys so the two same-family inserts stay separate rows.
	keyA, keyB := "need-a", "need-b"
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED, "added first",
		SystemMessageInsertOptions{CoalesceKey: &keyA},
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := writer.InsertSystemMessage(
		clock.WithSimulationTime(ctx, t0), convID, "actor1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED, "added second",
		SystemMessageInsertOptions{CoalesceKey: &keyB, SentAtUnixSec: &override},
	); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	msgs, err := sqlStorage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	var gotOverride bool
	for _, m := range msgs {
		msg := m.(*models.ChatMessage)
		if msg.GetSystemMessage().Description == "added second" {
			gotOverride = true
			if msg.SentAtUnixSec != override {
				t.Errorf("SentAtUnixSec = %d, want override %d", msg.SentAtUnixSec, override)
			}
		}
	}
	if !gotOverride {
		t.Fatal("second message not found")
	}
}

func TestIsCreationAnchorMessage(t *testing.T) {
	anchor := []models.ChatSystemAction{
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_REQUEST_CREATED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
	}
	nonAnchor := []models.ChatSystemAction{
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_RSVP_YES,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_DETAIL_CHANGED,
	}

	for _, action := range anchor {
		msg := &models.ChatMessage{
			Message: &models.ChatMessage_SystemMessage{
				SystemMessage: &models.SystemChatMessage{Action: action},
			},
		}
		if !IsCreationAnchorMessage(msg) {
			t.Errorf("expected %v to be a creation anchor", action)
		}
	}

	for _, action := range nonAnchor {
		msg := &models.ChatMessage{
			Message: &models.ChatMessage_SystemMessage{
				SystemMessage: &models.SystemChatMessage{Action: action},
			},
		}
		if IsCreationAnchorMessage(msg) {
			t.Errorf("expected %v to NOT be a creation anchor", action)
		}
	}

	// User messages are never anchors.
	userMsg := &models.ChatMessage{
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{SenderId: "u1", Text: "hi"},
		},
	}
	if IsCreationAnchorMessage(userMsg) {
		t.Error("expected user message to NOT be a creation anchor")
	}
}
