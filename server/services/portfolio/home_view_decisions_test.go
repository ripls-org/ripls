package portfolio

import (
	"testing"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Needs-you decision classifier
// ---------------------------------------------------------------------------.

func TestAssembleHomeDecisions_LendingRequestQualifies(t *testing.T) {
	d := newHomeData()
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000)
	tr.CommunityId = "comm-1"
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Ryobi chainsaw")

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions (count %d), want 1", len(decisions), count)
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_LENDING_REQUEST {
		t.Errorf("kind = %v, want LENDING_REQUEST", dec.Kind)
	}
	if dec.SubjectTitle != "Ryobi chainsaw" {
		t.Errorf("subject_title = %q, want the gear name", dec.SubjectTitle)
	}
	// No requester message → no context line.
	if dec.Reason != api.HomeDecisionReason_HOME_DECISION_REASON_UNSPECIFIED {
		t.Errorf("reason = %v, want UNSPECIFIED", dec.Reason)
	}
	if dec.TransferId != "t-1" || dec.ContentId != "g-1" {
		t.Errorf("transfer_id=%q content_id=%q", dec.TransferId, dec.ContentId)
	}
	if dec.CommunityName != "Boulder BC" {
		t.Errorf("community_name = %q", dec.CommunityName)
	}
}

func TestAssembleHomeDecisions_LendRequestCarriesMessagePreview(t *testing.T) {
	d := newHomeData()
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000)
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Ryobi chainsaw")
	d.transferConvMap["t-1"] = &models.ChatConversation{Id: "conv-1"}
	d.messagesByConvID["conv-1"] = []proto.Message{
		makeUserMessage("m-1", 900, otherID, "Back Sunday, promise!", nil),
	}

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(decisions))
	}
	dec := decisions[0]
	if dec.Reason != api.HomeDecisionReason_HOME_DECISION_REASON_MESSAGE_PREVIEW {
		t.Errorf("reason = %v, want MESSAGE_PREVIEW", dec.Reason)
	}
	if dec.GetMessagePreview() != "Back Sunday, promise!" {
		t.Errorf("message_preview = %q, want the requester's note", dec.GetMessagePreview())
	}
	if dec.ConversationId != "conv-1" {
		t.Errorf("conversation_id = %q", dec.ConversationId)
	}
}

func TestAssembleHomeDecisions_BorrowingStatusRows(t *testing.T) {
	d := newHomeData()
	pickupAt := homeNow.Unix() + 86400
	returnAt := homeNow.Unix() + 5*86400
	// Awaiting handoff → "Mark picked up".
	pending := makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	pending.EstimatedPickupUnixSec = &pickupAt
	// In hand → "Mark returned".
	held := makeLoanTransfer("t-2", otherID, selfID, "g-2", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	held.ExpectedReturnUnixSec = &returnAt
	d.activeTransfers = []*models.Transfer{pending, held}
	d.gearProtoMap["g-1"] = makeGear("g-1", otherID, "Power washer")
	d.gearProtoMap["g-2"] = makeGear("g-2", otherID, "Tent")

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 2 || len(decisions) != 2 {
		t.Fatalf("got %d decisions (count %d), want 2", len(decisions), count)
	}
	byGear := map[string]*api.HomeDecision{}
	for _, dec := range decisions {
		if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_TRANSFER_UPDATE {
			t.Errorf("kind = %v, want TRANSFER_UPDATE", dec.Kind)
		}
		if dec.ItemType != api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER {
			t.Errorf("item_type = %v, want TRANSFER", dec.ItemType)
		}
		byGear[dec.ContentId] = dec
	}
	if byGear["g-1"].TransferAction != api.HomeTransferAction_HOME_TRANSFER_ACTION_START_LOAN {
		t.Errorf("pending action = %v, want START_LOAN", byGear["g-1"].TransferAction)
	}
	if byGear["g-1"].Reason != api.HomeDecisionReason_HOME_DECISION_REASON_PICKUP_SCHEDULED {
		t.Errorf("pending reason = %v, want PICKUP_SCHEDULED", byGear["g-1"].Reason)
	}
	if byGear["g-1"].GetDueAtUnixSec() != pickupAt {
		t.Errorf("pending due_at = %d, want %d", byGear["g-1"].GetDueAtUnixSec(), pickupAt)
	}
	if byGear["g-2"].TransferAction != api.HomeTransferAction_HOME_TRANSFER_ACTION_COMPLETE_LOAN {
		t.Errorf("held action = %v, want COMPLETE_LOAN", byGear["g-2"].TransferAction)
	}
	if byGear["g-2"].Reason != api.HomeDecisionReason_HOME_DECISION_REASON_RETURN_DUE {
		t.Errorf("held reason = %v, want RETURN_DUE", byGear["g-2"].Reason)
	}
	if byGear["g-2"].GetDueAtUnixSec() != returnAt {
		t.Errorf("held due_at = %d, want %d", byGear["g-2"].GetDueAtUnixSec(), returnAt)
	}
	// Counterparty is the gear's owner, not the viewer.
	if byGear["g-1"].Counterparty.GetUserId() != otherID {
		t.Errorf("counterparty = %q, want owner", byGear["g-1"].Counterparty.GetUserId())
	}
}

func TestAssembleHomeDecisions_UndatedBorrowingReasons(t *testing.T) {
	d := newHomeData()
	// No pickup date → "ready to pick up"; no return date → "borrowing".
	pending := makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	held := makeLoanTransfer("t-2", otherID, selfID, "g-2", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	d.activeTransfers = []*models.Transfer{pending, held}
	d.gearProtoMap["g-1"] = makeGear("g-1", otherID, "Power washer")
	d.gearProtoMap["g-2"] = makeGear("g-2", otherID, "Tent")

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 2 {
		t.Fatalf("got %d decisions, want 2", len(decisions))
	}
	byGear := map[string]*api.HomeDecision{}
	for _, dec := range decisions {
		byGear[dec.ContentId] = dec
	}
	if byGear["g-1"].Reason != api.HomeDecisionReason_HOME_DECISION_REASON_READY_TO_PICK_UP {
		t.Errorf("pending reason = %v, want READY_TO_PICK_UP", byGear["g-1"].Reason)
	}
	if byGear["g-1"].DueAtUnixSec != nil {
		t.Errorf("pending due_at = %v, want absent", *byGear["g-1"].DueAtUnixSec)
	}
	if byGear["g-2"].Reason != api.HomeDecisionReason_HOME_DECISION_REASON_BORROWING {
		t.Errorf("held reason = %v, want BORROWING", byGear["g-2"].Reason)
	}
	if byGear["g-2"].DueAtUnixSec != nil {
		t.Errorf("held due_at = %v, want absent", *byGear["g-2"].DueAtUnixSec)
	}
}

func TestAssembleHomeDecisions_GiveawayReceiptPickupRow(t *testing.T) {
	d := newHomeData()
	// A giveaway the viewer is receiving (awaiting handoff) gets a "Mark
	// picked up" row — giveaways complete on pickup, so there's no "returned".
	pickupAt := homeNow.Unix() + 86400
	gift := makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	gift.TransferType = models.TransferType_TRANSFER_TYPE_GIVEAWAY
	gift.EstimatedPickupUnixSec = &pickupAt
	d.activeTransfers = []*models.Transfer{gift}
	d.gearProtoMap["g-1"] = makeGear("g-1", otherID, "Free lamp")

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions (count %d), want 1", len(decisions), count)
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_TRANSFER_UPDATE {
		t.Errorf("kind = %v, want TRANSFER_UPDATE", dec.Kind)
	}
	if dec.TransferAction != api.HomeTransferAction_HOME_TRANSFER_ACTION_COMPLETE_GIVEAWAY {
		t.Errorf("action = %v, want COMPLETE_GIVEAWAY", dec.TransferAction)
	}
}

func TestAssembleHomeDecisions_NonQualifyingStatesExcluded(t *testing.T) {
	d := newHomeData()
	d.activeTransfers = []*models.Transfer{
		// Viewer is the requester, not the owner — not their decision.
		makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000),
		// Already accepted — no longer a decision.
		makeLoanTransfer("t-2", selfID, otherID, "g-2", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000),
		// Active loan — steady state, not a decision.
		makeLoanTransfer("t-3", selfID, otherID, "g-3", models.TransferState_TRANSFER_STATE_ACTIVE, 1000),
	}

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 0 || len(decisions) != 0 {
		t.Fatalf("got %d decisions (count %d), want 0", len(decisions), count)
	}
}

func TestAssembleHomeDecisions_GiveawayKind(t *testing.T) {
	d := newHomeData()
	tr := makeGiveawayTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED)
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Bookshelf")

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(decisions))
	}
	if decisions[0].Kind != api.HomeDecisionKind_HOME_DECISION_KIND_GIVEAWAY_REQUEST {
		t.Errorf("kind = %v, want GIVEAWAY_REQUEST", decisions[0].Kind)
	}
	if decisions[0].ItemType != api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY {
		t.Errorf("item_type = %v, want GIVEAWAY", decisions[0].ItemType)
	}
}

func TestAssembleHomeDecisions_SupersededClaimSuppressed(t *testing.T) {
	d := newHomeData()
	d.userMap["user-3"] = &api.User{Id: "user-3", Name: "Dana Reyes", MediaId: "m-3"}
	// Two claims on the same giveaway: the owner already picked Dana
	// (RECIPIENT_SELECTED), so Marcus's still-open claim is superseded — no
	// "Give it" row may surface for that gear (#2724).
	marcus := makeGiveawayTransfer("t-marcus", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED)
	dana := makeGiveawayTransfer("t-dana", selfID, "user-3", "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED)
	d.activeTransfers = []*models.Transfer{marcus, dana}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Spaghetti (2 boxes)")

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 0 || len(decisions) != 0 {
		t.Fatalf("superseded claim should emit no decision, got %d (count %d): %+v",
			len(decisions), count, decisions)
	}
}

func TestAssembleHomeDecisions_UndecidedGearUnaffectedBySupersededSibling(t *testing.T) {
	d := newHomeData()
	d.userMap["user-3"] = &api.User{Id: "user-3", Name: "Dana Reyes", MediaId: "m-3"}
	// g-1 is decided (Dana selected, Marcus superseded); g-2 has an open claim
	// with no selection — its "Give it" row must still surface.
	superseded := makeGiveawayTransfer("t-marcus", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED)
	selected := makeGiveawayTransfer("t-dana", selfID, "user-3", "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED)
	open := makeGiveawayTransfer("t-open", selfID, otherID, "g-2", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED)
	d.activeTransfers = []*models.Transfer{superseded, selected, open}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Spaghetti (2 boxes)")
	d.gearProtoMap["g-2"] = makeGear("g-2", selfID, "Bookshelf")

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions (count %d), want 1 for the undecided gear", len(decisions), count)
	}
	dec := decisions[0]
	if dec.Id != "t-open" || dec.ContentId != "g-2" {
		t.Errorf("decision id=%q content_id=%q, want the open claim on g-2", dec.Id, dec.ContentId)
	}
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_GIVEAWAY_REQUEST {
		t.Errorf("kind = %v, want GIVEAWAY_REQUEST", dec.Kind)
	}
}

func TestAssembleHomeDecisions_AskClaimAggregatesPerAsk(t *testing.T) {
	d := newHomeData()
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_OFFERS_RECEIVED)
	req.Title = "Help cutting down dead pines"
	d.ownedRequests = []*models.Request{req}
	d.userMap["user-3"] = &api.User{Id: "user-3", Name: "Gary Park", MediaId: "m-3"}

	extras := emptyHomeExtras()
	// Two unconfirmed offers on one request → ONE aggregated "Say thanks" row.
	extras.offersByRequestID["r-1"] = []*models.RequestOffer{
		{Id: "o-1", RequestId: "r-1", UserId: otherID, CommunityId: "comm-1", CreatedAtUnixSec: 500},
		{Id: "o-2", RequestId: "r-1", UserId: "user-3", CommunityId: "comm-1", CreatedAtUnixSec: 700},
	}

	decisions, count := assembleHomeDecisions(d, extras, selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions (count %d), want 1 aggregated row", len(decisions), count)
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_ASK_CLAIM {
		t.Errorf("kind = %v, want ASK_CLAIM", dec.Kind)
	}
	if dec.Id != "r-1:thanks" {
		t.Errorf("id = %q, want aggregated request:thanks id", dec.Id)
	}
	if dec.SubjectTitle != "Help cutting down dead pines" {
		t.Errorf("subject_title = %q, want the request title", dec.SubjectTitle)
	}
	if dec.HelperCount != 2 || len(dec.Helpers) != 2 {
		t.Errorf("helper_count=%d helpers=%d, want 2/2", dec.HelperCount, len(dec.Helpers))
	}
	// Aggregated rows carry the request, not a single offerer.
	if dec.RequestId != "r-1" || dec.OffererUserId != "" {
		t.Errorf("request=%q offerer=%q (want r-1 / empty)", dec.RequestId, dec.OffererUserId)
	}
	// Sorts by the earliest offer.
	if dec.CreatedAtUnixSec != 500 {
		t.Errorf("created_at = %d, want earliest offer (500)", dec.CreatedAtUnixSec)
	}
}

func TestAssembleHomeDecisions_ReplyFromUnreadThreads(t *testing.T) {
	d := newHomeData()
	// An unread comment thread on the viewer's request → a Reply row.
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.Title = "Looking for a babysitter"
	req.ConversationId = "conv-req"
	d.ownedRequests = []*models.Request{req}
	d.reqMap["r-1"] = req
	d.convByID["conv-req"] = &models.ChatConversation{
		Id:          "conv-req",
		CommunityId: "comm-1",
		Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "r-1"}},
	}
	d.convUnreadCount["conv-req"] = 1
	d.messagesByConvID["conv-req"] = []proto.Message{
		makeUserMessage("m-1", 900, otherID, "I can help!", nil),
	}

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1 reply row", len(decisions))
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_REPLY {
		t.Errorf("kind = %v, want REPLY", dec.Kind)
	}
	if dec.SubjectTitle != "Looking for a babysitter" {
		t.Errorf("subject_title = %q", dec.SubjectTitle)
	}
	if dec.Reason != api.HomeDecisionReason_HOME_DECISION_REASON_MESSAGE_PREVIEW {
		t.Errorf("reason = %v, want MESSAGE_PREVIEW", dec.Reason)
	}
	if dec.GetMessagePreview() != "I can help!" {
		t.Errorf("message_preview = %q, want last incoming snippet", dec.GetMessagePreview())
	}
	if dec.Counterparty.GetUserId() != otherID {
		t.Errorf("counterparty = %q, want the sender", dec.Counterparty.GetUserId())
	}
	if dec.ConversationId != "conv-req" {
		t.Errorf("conversation_id = %q", dec.ConversationId)
	}
}

func TestAssembleHomeDecisions_ReplySuppressedWhenOutgoingOnly(t *testing.T) {
	d := newHomeData()
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.ConversationId = "conv-req"
	d.ownedRequests = []*models.Request{req}
	d.reqMap["r-1"] = req
	d.convByID["conv-req"] = &models.ChatConversation{
		Id:    "conv-req",
		Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "r-1"}},
	}
	d.convUnreadCount["conv-req"] = 1
	// Only the viewer's own message — nothing to reply to.
	d.messagesByConvID["conv-req"] = []proto.Message{
		makeUserMessage("m-1", 900, selfID, "hello?", nil),
	}

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 0 {
		t.Fatalf("got %d decisions, want 0 (no incoming message)", len(decisions))
	}
}

func TestAssembleHomeDecisions_MarkDonePastHostedEvent(t *testing.T) {
	d := newHomeData()
	pastStart := homeNow.Unix() - 3*86400
	exp := makeExperienceWithTime("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, pastStart)
	d.expMap["e-1"] = exp
	d.expCommunityIDs["e-1"] = []string{"comm-1"}

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1 mark-done row", len(decisions))
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_MARK_DONE {
		t.Errorf("kind = %v, want MARK_DONE", dec.Kind)
	}
	if dec.Reason != api.HomeDecisionReason_HOME_DECISION_REASON_PAST_EVENT {
		t.Errorf("reason = %v, want PAST_EVENT", dec.Reason)
	}
	if dec.GetDueAtUnixSec() != pastStart {
		t.Errorf("due_at = %d, want the past scheduled time %d", dec.GetDueAtUnixSec(), pastStart)
	}
	if dec.ItemType != api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE {
		t.Errorf("item_type = %v", dec.ItemType)
	}
}

func TestAssembleHomeDecisions_MarkDonePastDueRequest(t *testing.T) {
	d := newHomeData()
	neededBy := homeNow.Unix() - 2*86400
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.Title = "Borrow a stump grinder"
	req.NeededByUnixSec = &neededBy
	d.ownedRequests = []*models.Request{req}

	decisions, count := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if count != 1 || len(decisions) != 1 {
		t.Fatalf("got %d decisions, want 1 mark-done row", len(decisions))
	}
	dec := decisions[0]
	if dec.Kind != api.HomeDecisionKind_HOME_DECISION_KIND_MARK_DONE {
		t.Errorf("kind = %v, want MARK_DONE", dec.Kind)
	}
	if dec.Reason != api.HomeDecisionReason_HOME_DECISION_REASON_PAST_REQUEST {
		t.Errorf("reason = %v, want PAST_REQUEST", dec.Reason)
	}
	if dec.GetDueAtUnixSec() != neededBy {
		t.Errorf("due_at = %d, want the needed-by date %d", dec.GetDueAtUnixSec(), neededBy)
	}
	if dec.ItemType != api.DailyItemType_DAILY_ITEM_TYPE_REQUEST {
		t.Errorf("item_type = %v", dec.ItemType)
	}
}

func TestAssembleHomeDecisions_MarkDoneExcludesFutureAndCompleted(t *testing.T) {
	d := newHomeData()
	future := makeExperienceWithTime("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, homeNow.Unix()+86400)
	done := makeExperienceWithTime("e-2", selfID, models.ExperienceState_EXPERIENCE_STATE_COMPLETED, homeNow.Unix()-86400)
	d.expMap["e-1"] = future
	d.expMap["e-2"] = done

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 0 {
		t.Fatalf("got %d decisions, want 0 (future + completed excluded)", len(decisions))
	}
}

func TestAssembleHomeDecisions_ConfirmedHelperExcluded(t *testing.T) {
	d := newHomeData()
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_OFFERS_RECEIVED)
	req.ConfirmedHelperIds = []string{otherID}
	d.ownedRequests = []*models.Request{req}

	extras := emptyHomeExtras()
	extras.offersByRequestID["r-1"] = []*models.RequestOffer{
		{Id: "o-1", RequestId: "r-1", UserId: otherID, CreatedAtUnixSec: 500},
	}

	decisions, _ := assembleHomeDecisions(d, extras, selfID, homeTZ, homeNow)
	if len(decisions) != 0 {
		t.Fatalf("confirmed helper should not be a pending decision, got %d", len(decisions))
	}
}

func TestAssembleHomeDecisions_DismissedExcluded(t *testing.T) {
	d := newHomeData()
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000)
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Drill")

	extras := emptyHomeExtras()
	extras.dismissedDecisionIDs["t-1"] = true

	decisions, count := assembleHomeDecisions(d, extras, selfID, homeTZ, homeNow)
	if count != 0 || len(decisions) != 0 {
		t.Fatalf("dismissed decision should be excluded, got %d", len(decisions))
	}
}

func TestAssembleHomeDecisions_MostRecentFirst(t *testing.T) {
	d := newHomeData()
	newer := makeLoanTransfer("t-new", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 2000)
	older := makeLoanTransfer("t-old", selfID, otherID, "g-2", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000)
	d.activeTransfers = []*models.Transfer{newer, older}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Drill")
	d.gearProtoMap["g-2"] = makeGear("g-2", selfID, "Ladder")

	decisions, _ := assembleHomeDecisions(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(decisions) != 2 {
		t.Fatalf("got %d decisions, want 2", len(decisions))
	}
	if decisions[0].Id != "t-new" || decisions[1].Id != "t-old" {
		t.Errorf("order = [%s, %s], want most recent first", decisions[0].Id, decisions[1].Id)
	}
}
