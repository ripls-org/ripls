package portfolio

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// assembleHomeActivity builds the Recent-activity list: completed loans and
// giveaways, concluded events, and fulfilled requests within the activity
// window, newest first, capped at homeMaxActivity.
func assembleHomeActivity(d *fetchedData, userID string, now time.Time) []*api.HomeActivityEntry {
	windowStart := now.Add(-homeActivityWindowDays * 24 * time.Hour).Unix()
	var entries []*api.HomeActivityEntry

	add := func(id, title, communityID string, ts int64, contentID string, itemType api.DailyItemType) {
		if ts < windowStart || ts == 0 {
			return
		}
		entries = append(entries, &api.HomeActivityEntry{
			Id:                id,
			Title:             title,
			Subtitle:          d.communityNameMap[communityID],
			OccurredAtUnixSec: ts,
			ContentId:         contentID,
			ItemType:          itemType,
		})
	}

	for _, t := range d.allTransfers {
		if t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		isOwner := t.OwnerId == userID
		isRecipient := t.RecipientId == userID
		if !isOwner && !isRecipient {
			continue
		}
		gearName := ""
		if g, ok := d.gearProtoMap[t.GearId]; ok {
			gearName = g.(*models.Gear).Name
		}
		ts := transferTerminalAt(t)
		itemType := api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER
		var title string
		switch {
		case t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY && isOwner:
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY
			title = fmt.Sprintf("You gave %s to %s", gearName, homeFirstName(d, t.RecipientId))
		case t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY && isRecipient:
			itemType = api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY
			title = fmt.Sprintf("You received %s from %s", gearName, homeFirstName(d, t.OwnerId))
		case isOwner:
			title = fmt.Sprintf("%s came back from %s", gearName, homeFirstName(d, t.RecipientId))
		default:
			title = fmt.Sprintf("You returned %s to %s", gearName, homeFirstName(d, t.OwnerId))
		}
		add(t.Id, title, t.CommunityId, ts, t.GearId, itemType)
	}

	for expID, m := range d.expMap {
		e := m.(*models.Experience)
		if e.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		hosting := e.OwnerId == userID
		attended := false
		for _, uid := range d.expAttendeesMap[expID] {
			if uid == userID {
				attended = true
				break
			}
		}
		if !hosting && !attended {
			continue
		}
		ts := experienceTerminalAt(e)
		title := "You attended " + e.Name
		if hosting {
			title = "You hosted " + e.Name
		}
		add(expID, title, firstID(d.expCommunityIDs[expID]), ts, expID, api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE)
	}

	for reqID, m := range d.reqMap {
		r := m.(*models.Request)
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		requester := r.RequesterId == userID
		helper := false
		for _, uid := range r.ConfirmedHelperIds {
			if uid == userID {
				helper = true
				break
			}
		}
		if !requester && !helper {
			continue
		}
		var ts int64
		if r.FulfilledAtUnixSec != nil {
			ts = *r.FulfilledAtUnixSec
		}
		title := "You helped with " + requestTitle(r)
		if requester {
			title = "Your request was fulfilled: " + requestTitle(r)
		}
		add(reqID, title, firstID(d.reqCommunityIDs[reqID]), ts, reqID, api.DailyItemType_DAILY_ITEM_TYPE_REQUEST)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].OccurredAtUnixSec > entries[j].OccurredAtUnixSec
	})
	if len(entries) > homeMaxActivity {
		entries = entries[:homeMaxActivity]
	}
	return entries
}

// lastHumanMessageText returns the text of the most recent non-system
// message sent by senderID in the given conversation, or "" when none.
func lastHumanMessageText(messages []proto.Message, senderID string) string {
	var latest *models.ChatMessage
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		if msg.GetSystemMessage() != nil {
			continue
		}
		if messageSenderID(msg) != senderID {
			continue
		}
		if latest == nil || msg.SentAtUnixSec > latest.SentAtUnixSec {
			latest = msg
		}
	}
	if latest == nil {
		return ""
	}
	if um := latest.GetUserMessage(); um != nil {
		return um.Text
	}
	return ""
}

// lastIncomingHumanMessage returns the sender ID and text of the most recent
// non-system message NOT sent by viewerID — who to reply to and the snippet.
// Returns ("", "") when the thread has no incoming human message.
func lastIncomingHumanMessage(messages []proto.Message, viewerID string) (string, string) {
	var latest *models.ChatMessage
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		if msg.GetSystemMessage() != nil {
			continue
		}
		sender := messageSenderID(msg)
		if sender == "" || sender == viewerID {
			continue
		}
		if latest == nil || msg.SentAtUnixSec > latest.SentAtUnixSec {
			latest = msg
		}
	}
	if latest == nil {
		return "", ""
	}
	text := ""
	if um := latest.GetUserMessage(); um != nil {
		text = um.Text
	}
	return messageSenderID(latest), text
}

// latestMessageTime returns the sent time of the most recent message, or 0.
func latestMessageTime(messages []proto.Message) int64 {
	var latest int64
	for _, m := range messages {
		if msg := m.(*models.ChatMessage); msg.SentAtUnixSec > latest {
			latest = msg.SentAtUnixSec
		}
	}
	return latest
}

// transferGearID returns the gear ID for a transfer ID using the prefetched
// transfer conversation map's reverse lookup, or "" when not found.
func transferGearID(d *fetchedData, transferID string) string {
	for _, t := range d.allTransfers {
		if t.Id == transferID {
			return t.GearId
		}
	}
	return ""
}
