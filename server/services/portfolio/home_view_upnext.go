package portfolio

import (
	"sort"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// assembleHomeUpNext builds the Up-next agenda: events the viewer is going to
// or hosting, plus dated gear obligations (pickups, handoffs, due-backs).
// Entries are ascending by time and capped at homeMaxUpNext. Items without a
// real scheduled time are excluded — times are never invented.
func assembleHomeUpNext(d *fetchedData, extras *homeExtras, userID string, tz *time.Location, now time.Time) []*api.HomeUpNextEntry {
	startOfToday := dayStartIn(now, tz)
	var entries []*api.HomeUpNextEntry

	// Events: hosting, going/maybe, or directly invited (explicit watches alone
	// don't qualify).
	for _, expID := range d.activeExpIDs {
		m, ok := d.expMap[expID]
		if !ok {
			continue
		}
		e := m.(*models.Experience)
		intention := d.userRSVPIntentions[expID]
		_, going := d.userRSVPTimes[expID]
		hosting := e.OwnerId == userID
		invited := d.invitedExpIDs[expID] && intention != models.RSVPIntention_RSVP_INTENTION_NO
		if !hosting && !going && !invited {
			continue
		}
		st := experienceScheduledTime(e)
		// Undated events are excluded; events leave once their day has passed.
		if st == 0 || st < startOfToday {
			continue
		}

		goingCount := len(d.expAttendeesMap[expID])
		var status api.HomeUpNextStatus
		switch {
		case hosting:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_HOSTING
		case intention == models.RSVPIntention_RSVP_INTENTION_MAYBE:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_MAYBE
		case intention == models.RSVPIntention_RSVP_INTENTION_YES:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_GOING
		default:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_INVITED
		}

		communityID := firstID(d.expCommunityIDs[expID])
		entries = append(entries, &api.HomeUpNextEntry{
			Id:               expID,
			Kind:             api.HomeUpNextKind_HOME_UP_NEXT_KIND_EVENT,
			Title:            e.Name,
			Status:           status,
			GoingCount:       int32(goingCount),
			TimeUnixSec:      st,
			AllDay:           eventIsAllDay(e, st, tz),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CommunityIds:     d.expCommunityIDs[expID],
			ThumbnailMediaId: firstMediaID(e.MediaIds),
			ContentId:        expID,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE,
		})
	}

	// Upcoming gear obligations — loan pickups/returns and giveaway handoffs.
	for _, t := range d.activeTransfers {
		if entry := gearObligationEntry(d, t, userID, startOfToday); entry != nil {
			entries = append(entries, entry)
		}
	}

	// The viewer's own dated requests — placed on their needed-by date with the
	// kind-of-help tag, helper faces, and remaining slots.
	for _, r := range d.ownedRequests {
		if isRequestTerminal(r.State) {
			continue
		}
		if r.NeededByUnixSec == nil || *r.NeededByUnixSec < startOfToday {
			continue
		}
		var total, claimed int32
		for _, n := range extras.needsByRequestID[r.Id] {
			total += n.Slots
			claimed += n.Slots - n.SlotsRemaining
		}
		needed := total - claimed
		if needed < 0 {
			needed = 0
		}

		var helpers []*api.DailyPerson
		for _, offer := range extras.offersByRequestID[r.Id] {
			if u := d.userMap[offer.UserId]; u != nil {
				helpers = append(helpers, dailyPersonFor(u))
			}
		}

		communityID := firstID(d.reqCommunityIDs[r.Id])
		entries = append(entries, &api.HomeUpNextEntry{
			Id:     r.Id,
			Kind:   api.HomeUpNextKind_HOME_UP_NEXT_KIND_ASK,
			Title:  requestTitle(r),
			Status: api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_YOUR_REQUEST,
			// needed_by is a date (date picker), so the client shows the day,
			// not a midnight clock.
			TimeUnixSec:      *r.NeededByUnixSec,
			AllDay:           true,
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CommunityIds:     d.reqCommunityIDs[r.Id],
			ThumbnailMediaId: firstMediaID(r.MediaIds),
			ContentId:        r.Id,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_REQUEST,
			KindTag:          r.Category,
			Helpers:          helpers,
			NeededCount:      needed,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].TimeUnixSec < entries[j].TimeUnixSec
	})
	if len(entries) > homeMaxUpNext {
		entries = entries[:homeMaxUpNext]
	}
	return entries
}

// assembleHomeCalendar builds the on-demand calendar timeline. Unlike up_next
// (upcoming-only, for the root hero), this also includes recent past milestones
// so the calendar can be paged backward, and it places each item on the date
// that matters for that item type:
//
//   - Events: their scheduled date (not the create date).
//   - Requests: the needed-by/due date when set, else the first-shared date.
//   - Gear: the date it was first shared / the viewer got access.
//   - Transfers: upcoming loan / giveaway pickups, handoffs, and due-backs.
//
// Past markers are bounded by homeCalendarPastWindow; entries are ascending by
// time and capped at homeMaxCalendar. Times are never invented.
func assembleHomeCalendar(d *fetchedData, extras *homeExtras, userID string, tz *time.Location, now time.Time) []*api.HomeUpNextEntry {
	startOfToday := dayStartIn(now, tz)
	earliest := now.Add(-homeCalendarPastWindow).Unix()
	var entries []*api.HomeUpNextEntry

	// Events on their scheduled date — every experience the viewer can see
	// (shared into one of their communities, hosted, or RSVP'd to), regardless
	// of owner. Past within the window and future.
	seenExp := make(map[string]bool)
	addExp := func(e *models.Experience) {
		if e == nil || seenExp[e.Id] {
			return
		}
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
			e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			return
		}
		if e.Deleted != nil && e.Deleted.DeletedAtUnixSec > 0 {
			return
		}
		st := experienceScheduledTime(e)
		if st == 0 || st < earliest {
			return
		}
		seenExp[e.Id] = true

		intention := d.userRSVPIntentions[e.Id]
		hosting := e.OwnerId == userID
		goingCount := len(d.expAttendeesMap[e.Id])
		var status api.HomeUpNextStatus
		switch {
		case hosting:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_HOSTING
		case intention == models.RSVPIntention_RSVP_INTENTION_YES:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_GOING
		case intention == models.RSVPIntention_RSVP_INTENTION_MAYBE:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_MAYBE
		case d.invitedExpIDs[e.Id] && intention != models.RSVPIntention_RSVP_INTENTION_NO:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_INVITED
		case goingCount > 0:
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OTHERS_GOING
		}

		communityID := firstID(d.expCommunityIDs[e.Id])
		entries = append(entries, &api.HomeUpNextEntry{
			Id:               e.Id,
			Kind:             api.HomeUpNextKind_HOME_UP_NEXT_KIND_EVENT,
			Title:            e.Name,
			Status:           status,
			GoingCount:       int32(goingCount),
			TimeUnixSec:      st,
			AllDay:           eventIsAllDay(e, st, tz),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CommunityIds:     d.expCommunityIDs[e.Id],
			ThumbnailMediaId: firstMediaID(e.MediaIds),
			ContentId:        e.Id,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE,
		})
	}
	for _, m := range d.expMap {
		addExp(m.(*models.Experience))
	}
	for _, e := range d.ownedExperiences {
		addExp(e)
	}

	// Upcoming gear obligations — loan pickups/returns and giveaway handoffs.
	for _, t := range d.activeTransfers {
		if entry := gearObligationEntry(d, t, userID, startOfToday); entry != nil {
			entries = append(entries, entry)
		}
	}

	// Requests the viewer can see — owned plus every request shared into one of
	// their communities, regardless of owner. Placed on the due date when set
	// (future), else the first-shared date.
	seenReq := make(map[string]bool)
	addReq := func(r *models.Request, sharedAt int64) {
		if r == nil || seenReq[r.Id] || isRequestTerminal(r.State) {
			return
		}
		var ts int64
		var status api.HomeUpNextStatus
		if r.NeededByUnixSec != nil && *r.NeededByUnixSec > 0 {
			if *r.NeededByUnixSec < startOfToday {
				return
			}
			ts = *r.NeededByUnixSec
			if r.RequesterId == userID {
				status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_YOUR_REQUEST_DUE
			} else {
				status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_REQUEST_DUE
			}
		} else {
			ts = sharedAt
			if ts == 0 {
				ts = r.CreatedAtUnixSec
			}
			if ts == 0 || ts < earliest {
				return
			}
			if r.RequesterId == userID {
				status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_YOUR_REQUEST
			} else {
				status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_REQUEST_SHARED
			}
		}
		seenReq[r.Id] = true

		var helpers []*api.DailyPerson
		for _, offer := range extras.offersByRequestID[r.Id] {
			if u := d.userMap[offer.UserId]; u != nil {
				helpers = append(helpers, dailyPersonFor(u))
			}
		}

		communityID := firstID(d.reqCommunityIDs[r.Id])
		entries = append(entries, &api.HomeUpNextEntry{
			Id:               r.Id,
			Kind:             api.HomeUpNextKind_HOME_UP_NEXT_KIND_ASK,
			Title:            requestTitle(r),
			Status:           status,
			TimeUnixSec:      ts,
			AllDay:           true,
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CommunityIds:     d.reqCommunityIDs[r.Id],
			ThumbnailMediaId: firstMediaID(r.MediaIds),
			ContentId:        r.Id,
			ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_REQUEST,
			KindTag:          r.Category,
			Helpers:          helpers,
		})
	}
	for _, r := range d.ownedRequests {
		addReq(r, r.CreatedAtUnixSec)
	}
	for _, m := range d.reqMap {
		r := m.(*models.Request)
		var sharedAt int64
		if cr := d.commReqMap[r.Id]; cr != nil {
			sharedAt = cr.SharedAtUnixSec
		}
		addReq(r, sharedAt)
	}

	// Gear first-shared / access markers — owned gear on its created date, and
	// every gear shared into one of the viewer's communities on the date it was
	// shared there (when the viewer got access), regardless of owner.
	seenGear := make(map[string]bool)
	addGear := func(g *models.Gear, accessAt int64) {
		if g == nil || seenGear[g.Id] ||
			g.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
			return
		}
		ts := accessAt
		if ts == 0 {
			ts = g.CreatedAtUnixSec
		}
		if ts == 0 || ts < earliest {
			return
		}
		seenGear[g.Id] = true
		status := api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_ADDED_TO_LIBRARY
		if g.OwnerId != userID {
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_SHARED_WITH_YOU
		}
		communityID := firstID(d.gearCommunityIDs[g.Id])
		entries = append(entries, &api.HomeUpNextEntry{
			Id:               g.Id,
			Kind:             api.HomeUpNextKind_HOME_UP_NEXT_KIND_GEAR_SHARED,
			Title:            g.Name,
			Status:           status,
			TimeUnixSec:      ts,
			AllDay:           true,
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CommunityIds:     d.gearCommunityIDs[g.Id],
			ThumbnailMediaId: firstMediaID(g.MediaIds),
			// Gear screens route via the transfer/gear item type using the gear
			// ID as content_id (there is no dedicated gear DailyItemType).
			ContentId: g.Id,
			ItemType:  api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER,
		})
	}
	for _, g := range d.ownedGear {
		addGear(g, g.CreatedAtUnixSec)
	}
	for _, m := range d.allCommunityGearMap {
		g := m.(*models.Gear)
		var accessAt int64
		if cg := d.cgByGearID[g.Id]; cg != nil {
			accessAt = cg.CreatedAtUnixSec
		}
		addGear(g, accessAt)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].TimeUnixSec < entries[j].TimeUnixSec
	})
	if len(entries) > homeMaxCalendar {
		entries = entries[:homeMaxCalendar]
	}
	return entries
}

// gearObligationEntry builds the Up Next / calendar row for one active
// transfer, or nil when that transfer carries no dated obligation for the
// viewer: not their transfer, no date set, the date already past, or a state
// with nothing left to do.
//
// Both assembleHomeUpNext and assembleHomeCalendar place gear obligations, and
// they were the same sixty-two lines twice before this was extracted (#2816).
func gearObligationEntry(d *fetchedData, t *models.Transfer, userID string, startOfToday int64) *api.HomeUpNextEntry {
	isOwner := t.OwnerId == userID
	isRecipient := t.RecipientId == userID
	if !isOwner && !isRecipient {
		return nil
	}
	// Giveaways complete on pickup, so they have a handoff but never a return.
	isGiveaway := t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY

	gearName := ""
	thumb := ""
	if g, ok := d.gearProtoMap[t.GearId]; ok {
		gear := g.(*models.Gear)
		gearName = gear.Name
		thumb = firstMediaID(gear.MediaIds)
	}
	ownerFirst := homeFirstName(d, t.OwnerId)
	recipientFirst := homeFirstName(d, t.RecipientId)

	var ts int64
	var status api.HomeUpNextStatus
	counterpartyFirst := ownerFirst
	switch t.State {
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		if t.EstimatedPickupUnixSec == nil || *t.EstimatedPickupUnixSec < startOfToday {
			return nil
		}
		ts = *t.EstimatedPickupUnixSec
		if isRecipient {
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_PICKUP
		} else {
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_HANDOFF
			counterpartyFirst = recipientFirst
		}
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		// Giveaways never reach ACTIVE (pickup completes them); only loans have
		// a return obligation.
		if isGiveaway || t.ExpectedReturnUnixSec == nil || *t.ExpectedReturnUnixSec < startOfToday {
			return nil
		}
		ts = *t.ExpectedReturnUnixSec
		if isOwner {
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_DUE_BACK
			counterpartyFirst = recipientFirst
		} else {
			status = api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_RETURN
		}
	default:
		return nil
	}

	return &api.HomeUpNextEntry{
		Id:   t.Id,
		Kind: api.HomeUpNextKind_HOME_UP_NEXT_KIND_GEAR_OBLIGATION,
		// No composed title: the client builds the row headline from
		// [status], [gear_name] and [counterparty_first_name] (#2835).
		Status:                status,
		IsGiveaway:            isGiveaway,
		GearName:              optionalHomeString(gearName),
		CounterpartyFirstName: optionalHomeString(counterpartyFirst),
		// Pickup/return are date-only (set via a date picker), so the client
		// shows the day on its own, never a midnight clock.
		TimeUnixSec:      ts,
		AllDay:           true,
		CommunityId:      t.CommunityId,
		CommunityName:    d.communityNameMap[t.CommunityId],
		CommunityIds:     communityIDList(t.CommunityId),
		ThumbnailMediaId: thumb,
		ContentId:        t.GearId,
		ItemType:         api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER,
	}
}
