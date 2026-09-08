package portfolio

import (
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// assembleHomeDecisions builds the one flat Needs-you queue. Every row carries
// a typed action; four sources feed it, in precedence order so each underlying
// item appears once:
//
//   - Lend / Give: incoming borrow/giveaway requests on the viewer's gear.
//   - Say thanks: help received — one aggregated row per request with
//     unconfirmed offers, faces stacked.
//   - Reply: unread object conversations (gear / request comments / events).
//   - Mark done: past events the viewer hosted, and past-due requests, to close.
//
// `represented` tracks the content IDs already shown so a later source never
// double-lists the same item. Returns the queue (oldest first, capped) and the
// pre-cap total — the single Needs-you number.
func assembleHomeDecisions(
	d *fetchedData,
	extras *homeExtras,
	userID string,
	tz *time.Location,
	now time.Time,
) ([]*api.HomeDecision, int32) {
	var decisions []*api.HomeDecision
	represented := make(map[string]bool)

	decisions = append(decisions, assembleLendDecisions(d, extras, userID, represented)...)
	decisions = append(decisions, assembleTransferUpdateDecisions(d, extras, userID, represented)...)
	decisions = append(decisions, assembleThanksDecisions(d, extras, represented)...)
	decisions = append(decisions, assembleReplyDecisions(d, extras, userID, represented)...)
	decisions = append(decisions, assembleMarkDoneDecisions(d, extras, userID, tz, now, represented)...)

	// Most recent first, so newly-arrived decisions surface at the top.
	sort.SliceStable(decisions, func(i, j int) bool {
		return decisions[i].CreatedAtUnixSec > decisions[j].CreatedAtUnixSec
	})

	total := int32(len(decisions))
	if len(decisions) > homeMaxDecisions {
		decisions = decisions[:homeMaxDecisions]
	}
	return decisions, total
}

// assembleLendDecisions emits "Lend"/"Give it" rows for incoming
// borrow/giveaway requests on the viewer's gear (INTEREST_EXPRESSED). Once any
// transfer on the same gear has advanced past selection, the remaining
// INTEREST_EXPRESSED claims are superseded and emit nothing (#2724). Marks
// each gear as represented.
func assembleLendDecisions(d *fetchedData, extras *homeExtras, userID string, represented map[string]bool) []*api.HomeDecision {
	// Gear whose lend/give decision is already made: a claimant was selected,
	// or the transfer is underway or finished. INTEREST_EXPRESSED transfers on
	// the same gear target claimants the owner passed over — a CTA for them
	// would ask the owner to hand the item to the one person NOT getting it
	// (#2724). COMPLETED is normally filtered out upstream but is handled here
	// in case it appears.
	decidedGear := make(map[string]bool)
	for _, t := range d.activeTransfers {
		if t.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED ||
			t.State == models.TransferState_TRANSFER_STATE_ACTIVE ||
			t.State == models.TransferState_TRANSFER_STATE_COMPLETED {
			decidedGear[t.GearId] = true
		}
	}

	var decisions []*api.HomeDecision
	for _, t := range d.activeTransfers {
		if t.OwnerId != userID || t.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			continue
		}
		if decidedGear[t.GearId] || extras.dismissedDecisionIDs[t.Id] {
			continue
		}

		gearName := ""
		thumb := ""
		if g, ok := d.gearProtoMap[t.GearId]; ok {
			gear := g.(*models.Gear)
			gearName = gear.Name
			thumb = firstMediaID(gear.MediaIds)
		}

		var counterparty *api.DailyPerson
		if u := d.userMap[t.RecipientId]; u != nil {
			counterparty = dailyPersonFor(u)
		}

		kind := api.HomeDecisionKind_HOME_DECISION_KIND_LENDING_REQUEST
		itemType := api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER
		if t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			kind = api.HomeDecisionKind_HOME_DECISION_KIND_GIVEAWAY_REQUEST
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY
		}

		// "from your conversation": a human message from the requester exists.
		convID := ""
		reason := api.HomeDecisionReason_HOME_DECISION_REASON_UNSPECIFIED
		var preview *string
		if conv := d.transferConvMap[t.Id]; conv != nil {
			if text := lastHumanMessageText(d.messagesByConvID[conv.Id], t.RecipientId); text != "" {
				convID = conv.Id
				reason = api.HomeDecisionReason_HOME_DECISION_REASON_MESSAGE_PREVIEW
				preview = proto.String(text)
			}
		}

		decisions = append(decisions, &api.HomeDecision{
			Id:               t.Id,
			Kind:             kind,
			SubjectTitle:     gearName,
			Reason:           reason,
			MessagePreview:   preview,
			Counterparty:     counterparty,
			CommunityId:      t.CommunityId,
			CommunityName:    d.communityNameMap[t.CommunityId],
			ThumbnailMediaId: thumb,
			ObjectMediaId:    thumb,
			ContentId:        t.GearId,
			ItemType:         itemType,
			TransferId:       t.Id,
			ConversationId:   convID,
			CreatedAtUnixSec: t.LatestRequestUnixSec,
		})
		represented[t.GearId] = true
	}
	return decisions
}

// assembleTransferUpdateDecisions emits a status row for each transfer the
// viewer is receiving — gear owned by someone else. The recipient advances it:
// "Mark picked up" while the handoff is pending (RECIPIENT_SELECTED), then —
// for loans only — "Mark returned" once it's in hand (ACTIVE). Giveaways have
// no return phase, so picking up completes them. Tapping opens the gear
// screen, where the recipient's pickup/return controls live. Marks the gear
// represented so a reply on the same thread doesn't double-list.
func assembleTransferUpdateDecisions(d *fetchedData, extras *homeExtras, userID string, represented map[string]bool) []*api.HomeDecision {
	var decisions []*api.HomeDecision
	for _, t := range d.activeTransfers {
		if t.RecipientId != userID {
			continue
		}
		if represented[t.GearId] || extras.dismissedDecisionIDs[t.Id] {
			continue
		}
		isGiveaway := t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY

		gearName := ""
		thumb := ""
		if g, ok := d.gearProtoMap[t.GearId]; ok {
			gear := g.(*models.Gear)
			gearName = gear.Name
			thumb = firstMediaID(gear.MediaIds)
		}

		var counterparty *api.DailyPerson
		if u := d.userMap[t.OwnerId]; u != nil {
			counterparty = dailyPersonFor(u)
		}

		var sortAt int64
		var action api.HomeTransferAction
		var reason api.HomeDecisionReason
		var dueAt *int64
		switch t.State {
		case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
			// Loans start (RECIPIENT_SELECTED → ACTIVE); giveaways complete on
			// pickup. The client uses this to run the right mutation in place.
			if isGiveaway {
				action = api.HomeTransferAction_HOME_TRANSFER_ACTION_COMPLETE_GIVEAWAY
			} else {
				action = api.HomeTransferAction_HOME_TRANSFER_ACTION_START_LOAN
			}
			if t.EstimatedPickupUnixSec != nil && *t.EstimatedPickupUnixSec > 0 {
				sortAt = *t.EstimatedPickupUnixSec
				reason = api.HomeDecisionReason_HOME_DECISION_REASON_PICKUP_SCHEDULED
				dueAt = proto.Int64(*t.EstimatedPickupUnixSec)
			} else {
				reason = api.HomeDecisionReason_HOME_DECISION_REASON_READY_TO_PICK_UP
			}
		case models.TransferState_TRANSFER_STATE_ACTIVE:
			if isGiveaway {
				// Giveaways complete on pickup; there is no "returned" step.
				continue
			}
			action = api.HomeTransferAction_HOME_TRANSFER_ACTION_COMPLETE_LOAN
			if t.ExpectedReturnUnixSec != nil && *t.ExpectedReturnUnixSec > 0 {
				sortAt = *t.ExpectedReturnUnixSec
				reason = api.HomeDecisionReason_HOME_DECISION_REASON_RETURN_DUE
				dueAt = proto.Int64(*t.ExpectedReturnUnixSec)
			} else {
				reason = api.HomeDecisionReason_HOME_DECISION_REASON_BORROWING
			}
		default:
			continue
		}
		if sortAt == 0 {
			sortAt = t.LatestRequestUnixSec
		}

		decisions = append(decisions, &api.HomeDecision{
			Id:               t.Id,
			Kind:             api.HomeDecisionKind_HOME_DECISION_KIND_TRANSFER_UPDATE,
			SubjectTitle:     gearName,
			Reason:           reason,
			DueAtUnixSec:     dueAt,
			Counterparty:     counterparty,
			CommunityId:      t.CommunityId,
			CommunityName:    d.communityNameMap[t.CommunityId],
			ThumbnailMediaId: thumb,
			ObjectMediaId:    thumb,
			ContentId:        t.GearId,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER,
			TransferId:       t.Id,
			TransferAction:   action,
			CreatedAtUnixSec: sortAt,
		})
		represented[t.GearId] = true
	}
	return decisions
}

// assembleThanksDecisions emits one aggregated "Say thanks" row per request that
// has unconfirmed offers — the helpers who pitched in, stacked. Marks each
// request as represented.
func assembleThanksDecisions(d *fetchedData, extras *homeExtras, represented map[string]bool) []*api.HomeDecision {
	var decisions []*api.HomeDecision
	for _, r := range d.ownedRequests {
		if isRequestTerminal(r.State) {
			continue
		}
		confirmed := make(map[string]bool, len(r.ConfirmedHelperIds))
		for _, id := range r.ConfirmedHelperIds {
			confirmed[id] = true
		}

		var helpers []*api.DailyPerson
		var communityID string
		var earliest int64
		for _, offer := range extras.offersByRequestID[r.Id] {
			if confirmed[offer.UserId] {
				continue
			}
			if u := d.userMap[offer.UserId]; u != nil {
				helpers = append(helpers, dailyPersonFor(u))
			}
			if communityID == "" {
				communityID = offer.CommunityId
			}
			if earliest == 0 || offer.CreatedAtUnixSec < earliest {
				earliest = offer.CreatedAtUnixSec
			}
		}
		if len(helpers) == 0 {
			continue
		}

		decisionID := r.Id + ":thanks"
		if extras.dismissedDecisionIDs[decisionID] {
			represented[r.Id] = true
			continue
		}

		decisions = append(decisions, &api.HomeDecision{
			Id:               decisionID,
			Kind:             api.HomeDecisionKind_HOME_DECISION_KIND_ASK_CLAIM,
			SubjectTitle:     requestTitle(r),
			Counterparty:     helpers[0],
			Helpers:          helpers,
			HelperCount:      int32(len(helpers)),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			ThumbnailMediaId: firstMediaID(r.MediaIds),
			ContentId:        r.Id,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_REQUEST,
			RequestId:        r.Id,
			ConversationId:   r.ConversationId,
			CreatedAtUnixSec: earliest,
		})
		represented[r.Id] = true
	}
	return decisions
}

// assembleReplyDecisions emits "Reply" rows for unread object conversations
// (gear / request comments / events) the viewer participates in, skipping any
// subject already represented by a Lend or Say-thanks row. Marks each subject
// represented.
func assembleReplyDecisions(d *fetchedData, extras *homeExtras, userID string, represented map[string]bool) []*api.HomeDecision {
	var decisions []*api.HomeDecision
	for convID, unread := range d.convUnreadCount {
		if unread <= 0 {
			continue
		}
		conv := d.convByID[convID]
		if conv == nil {
			continue
		}

		// Resolve the subject from the conversation topic.
		var contentID, subjectTitle, objectMedia string
		var itemType api.DailyItemType
		topic := conv.GetTopic()
		switch {
		case topic.GetRequestId() != "":
			contentID = topic.GetRequestId()
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_REQUEST
			if m, ok := d.reqMap[contentID]; ok {
				r := m.(*models.Request)
				subjectTitle = requestTitle(r)
				objectMedia = firstMediaID(r.MediaIds)
			}
		case topic.GetExperienceId() != "":
			contentID = topic.GetExperienceId()
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE
			if m, ok := d.expMap[contentID]; ok {
				e := m.(*models.Experience)
				subjectTitle = e.Name
				objectMedia = firstMediaID(e.MediaIds)
			}
		case topic.GetGearId() != "" || topic.GetTransferId() != "":
			gearID := topic.GetGearId()
			if gearID == "" {
				// Transfer topic — resolve gear via the transfer.
				if conv.GetTopic().GetTransferId() != "" {
					gearID = transferGearID(d, conv.GetTopic().GetTransferId())
				}
			}
			contentID = gearID
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER
			if g, ok := d.gearProtoMap[gearID]; ok {
				gear := g.(*models.Gear)
				subjectTitle = gear.Name
				objectMedia = firstMediaID(gear.MediaIds)
			}
		default:
			// Community-wide or unresolved topics are not 1:1 "reply" items.
			continue
		}
		if contentID == "" || represented[contentID] {
			continue
		}

		// The most recent incoming human message — who to reply to, and the
		// snippet. Skip system-only / outgoing-only threads.
		senderID, text := lastIncomingHumanMessage(d.messagesByConvID[convID], userID)
		if senderID == "" {
			continue
		}
		var counterparty *api.DailyPerson
		if u := d.userMap[senderID]; u != nil {
			counterparty = dailyPersonFor(u)
		} else {
			counterparty = &api.DailyPerson{UserId: senderID}
		}

		decisionID := "reply:" + convID
		if extras.dismissedDecisionIDs[decisionID] {
			represented[contentID] = true
			continue
		}

		var createdAt int64
		if msgs := d.messagesByConvID[convID]; len(msgs) > 0 {
			createdAt = latestMessageTime(msgs)
		}

		decisions = append(decisions, &api.HomeDecision{
			Id:               decisionID,
			Kind:             api.HomeDecisionKind_HOME_DECISION_KIND_REPLY,
			SubjectTitle:     subjectTitle,
			Reason:           api.HomeDecisionReason_HOME_DECISION_REASON_MESSAGE_PREVIEW,
			MessagePreview:   proto.String(text),
			Counterparty:     counterparty,
			CommunityId:      conv.CommunityId,
			CommunityName:    d.communityNameMap[conv.CommunityId],
			ThumbnailMediaId: objectMedia,
			ObjectMediaId:    objectMedia,
			ContentId:        contentID,
			ItemType:         itemType,
			ConversationId:   convID,
			CreatedAtUnixSec: createdAt,
		})
		represented[contentID] = true
	}
	return decisions
}

// assembleMarkDoneDecisions emits "Done" rows for past events the viewer
// hosted (scheduled before today, not yet completed/cancelled) and past-due
// requests (needed-by before today, not fulfilled) that aren't already
// represented.
func assembleMarkDoneDecisions(
	d *fetchedData,
	extras *homeExtras,
	userID string,
	tz *time.Location,
	now time.Time,
	represented map[string]bool,
) []*api.HomeDecision {
	startOfToday := dayStartIn(now, tz)
	var decisions []*api.HomeDecision

	// Past hosted events.
	for expID, m := range d.expMap {
		e := m.(*models.Experience)
		if e.OwnerId != userID || represented[expID] {
			continue
		}
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
			e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			continue
		}
		st := experienceScheduledTime(e)
		if st == 0 || st >= startOfToday {
			continue
		}
		decisionID := "done:exp:" + expID
		if extras.dismissedDecisionIDs[decisionID] {
			continue
		}
		decisions = append(decisions, &api.HomeDecision{
			Id:               decisionID,
			Kind:             api.HomeDecisionKind_HOME_DECISION_KIND_MARK_DONE,
			SubjectTitle:     e.Name,
			Reason:           api.HomeDecisionReason_HOME_DECISION_REASON_PAST_EVENT,
			DueAtUnixSec:     proto.Int64(st),
			CommunityId:      firstID(d.expCommunityIDs[expID]),
			CommunityName:    d.communityNameMap[firstID(d.expCommunityIDs[expID])],
			ThumbnailMediaId: firstMediaID(e.MediaIds),
			ContentId:        expID,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE,
			CreatedAtUnixSec: st,
		})
		represented[expID] = true
	}

	// Past-due requests.
	for _, r := range d.ownedRequests {
		if isRequestTerminal(r.State) || represented[r.Id] {
			continue
		}
		if r.NeededByUnixSec == nil || *r.NeededByUnixSec == 0 || *r.NeededByUnixSec >= startOfToday {
			continue
		}
		decisionID := "done:req:" + r.Id
		if extras.dismissedDecisionIDs[decisionID] {
			continue
		}
		communityID := firstID(d.reqCommunityIDs[r.Id])
		decisions = append(decisions, &api.HomeDecision{
			Id:               decisionID,
			Kind:             api.HomeDecisionKind_HOME_DECISION_KIND_MARK_DONE,
			SubjectTitle:     requestTitle(r),
			Reason:           api.HomeDecisionReason_HOME_DECISION_REASON_PAST_REQUEST,
			DueAtUnixSec:     proto.Int64(*r.NeededByUnixSec),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			ThumbnailMediaId: firstMediaID(r.MediaIds),
			ContentId:        r.Id,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_REQUEST,
			RequestId:        r.Id,
			ConversationId:   r.ConversationId,
			CreatedAtUnixSec: *r.NeededByUnixSec,
		})
		represented[r.Id] = true
	}
	return decisions
}
