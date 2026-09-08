package portfolio

import (
	"sort"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// assembleHomeAsks builds the viewer's open requests with need-slot claim
// progress. Fulfilled requests graduate to Recent activity; cancelled
// requests disappear. Most recently created first, capped at homeMaxAsks.
func assembleHomeAsks(d *fetchedData, extras *homeExtras) []*api.HomeAsk {
	var asks []*api.HomeAsk
	seen := make(map[string]bool, len(d.ownedRequests))
	for _, r := range d.ownedRequests {
		if isRequestTerminal(r.State) {
			continue
		}
		// Defensive de-dupe by request ID — one row per request even if the
		// upstream owned-requests list carries duplicates.
		if seen[r.Id] {
			continue
		}
		seen[r.Id] = true

		var total, claimed int32
		for _, n := range extras.needsByRequestID[r.Id] {
			total += n.Slots
			claimed += n.Slots - n.SlotsRemaining
		}

		communityID := firstID(d.reqCommunityIDs[r.Id])
		asks = append(asks, &api.HomeAsk{
			RequestId:        r.Id,
			Title:            requestTitle(r),
			ClaimedCount:     claimed,
			TotalCount:       total,
			ThumbnailMediaId: firstMediaID(r.MediaIds),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CreatedAtUnixSec: r.CreatedAtUnixSec,
			OfferCount:       int32(len(extras.offersByRequestID[r.Id])),
		})
	}

	// Most recent first. ownedRequests carry created_at directly.
	createdAt := make(map[string]int64, len(d.ownedRequests))
	for _, r := range d.ownedRequests {
		createdAt[r.Id] = r.CreatedAtUnixSec
	}
	sort.SliceStable(asks, func(i, j int) bool {
		return createdAt[asks[i].RequestId] > createdAt[asks[j].RequestId]
	})
	if len(asks) > homeMaxAsks {
		asks = asks[:homeMaxAsks]
	}
	return asks
}

// assembleOwnedEvents builds the viewer's hosted events for "Your stuff": the
// events they own that haven't concluded, soonest dated first then undated.
// Distinct from Up next (dated agenda) and Needs you (action queue) — this is
// the viewer's own content, listed alongside their gear and requests.
func assembleOwnedEvents(d *fetchedData, userID string) []*api.HomeOwnedEvent {
	var events []*api.HomeOwnedEvent
	for expID, m := range d.expMap {
		e := m.(*models.Experience)
		if e.OwnerId != userID {
			continue
		}
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
			e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			continue
		}

		st := experienceScheduledTime(e)
		allDay := experienceIsAllDay(e)

		communityID := firstID(d.expCommunityIDs[expID])
		events = append(events, &api.HomeOwnedEvent{
			EventId:          expID,
			Title:            e.Name,
			ThumbnailMediaId: firstMediaID(e.MediaIds),
			DateUnixSec:      st,
			AllDay:           allDay,
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			CreatedAtUnixSec: e.CreatedAtUnixSec,
		})
	}

	// Soonest dated first; undated (st == 0) sort to the end.
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i].DateUnixSec, events[j].DateUnixSec
		if (a == 0) != (b == 0) {
			return a != 0 // dated before undated
		}
		return a < b
	})
	if len(events) > homeMaxEvents {
		events = events[:homeMaxEvents]
	}
	return events
}

// assembleHomeGear builds the Gear section: loans out and inbound first
// (dated obligations ascending, then undated), then listed gear, then gear
// at home. Returns the rows (capped at homeMaxGear), per-category counts
// computed before the cap, and whether the cap dropped rows.
func assembleHomeGear(
	d *fetchedData,
	userID string,
	now time.Time,
) ([]*api.HomeGearItem, *api.HomeGearCounts, bool) {
	var rows []*api.HomeGearItem
	// gear id → gear name, kept only to order the rows alphabetically; the
	// name itself is no longer on the wire (#2835).
	sortName := map[string]string{}
	engagedGear := make(map[string]bool)

	// Pending-interest counts per gear, for "Listed · N requests waiting".
	interestCount := make(map[string]int32)
	for _, t := range d.activeTransfers {
		if t.OwnerId == userID && t.State == models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			interestCount[t.GearId]++
		}
	}

	// Process ACTIVE before RECIPIENT_SELECTED so the most advanced transfer
	// claims a gear that somehow has both.
	for _, wantActive := range []bool{true, false} {
		for _, t := range d.activeTransfers {
			if t.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
				continue
			}
			isActive := t.State == models.TransferState_TRANSFER_STATE_ACTIVE
			isSelected := t.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
			if (wantActive && !isActive) || (!wantActive && !isSelected) {
				continue
			}
			// "Your stuff" is the viewer's own library only. Gear the viewer
			// is borrowing (recipient transfers) surfaces in Needs you (status
			// updates) and Up next (dated pickups/returns), not here.
			if t.OwnerId != userID || engagedGear[t.GearId] {
				continue
			}

			gearName := ""
			thumb := ""
			var gearCreated int64
			if g, ok := d.gearProtoMap[t.GearId]; ok {
				gear := g.(*models.Gear)
				gearName = gear.Name
				thumb = firstMediaID(gear.MediaIds)
				gearCreated = gear.CreatedAtUnixSec
			}

			row := &api.HomeGearItem{
				GearId:           t.GearId,
				TransferId:       t.Id,
				ThumbnailMediaId: thumb,
				CommunityId:      t.CommunityId,
				CommunityName:    d.communityNameMap[t.CommunityId],
				CreatedAtUnixSec: gearCreated,
			}
			sortName[t.GearId] = gearName

			// Owner-side loans only (recipient/borrowing rows moved to Needs
			// you + Up next).
			switch {
			case isActive:
				row.Category = api.HomeGearCategory_HOME_GEAR_CATEGORY_OUT
				if t.ExpectedReturnUnixSec != nil && *t.ExpectedReturnUnixSec > 0 {
					due := *t.ExpectedReturnUnixSec
					row.DueAtUnixSec = due
					if time.Unix(due, 0).Sub(now) <= homeDueSoonWindow {
						row.PillStyle = api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_AMBER
					} else {
						row.PillStyle = api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_CLAY
					}
				} else {
					row.PillStyle = api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_CLAY
					row.CanSetDate = true
				}
			case isSelected:
				row.Category = api.HomeGearCategory_HOME_GEAR_CATEGORY_OUT
				row.PillStyle = api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_GREEN
				if t.EstimatedPickupUnixSec != nil && *t.EstimatedPickupUnixSec > 0 {
					row.DueAtUnixSec = *t.EstimatedPickupUnixSec
				} else {
					// No pickup date yet — the client offers a "set pickup"
					// affordance so every handoff row either shows the date or
					// prompts for one.
					row.CanSetDate = true
				}
			default:
				continue
			}

			engagedGear[t.GearId] = true
			rows = append(rows, row)
		}
	}

	// Listed and at-home gear from the owned library.
	for _, g := range d.ownedGear {
		if engagedGear[g.Id] || g.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
			continue
		}
		listed := len(d.gearCommunityIDs[g.Id]) > 0 && g.State == models.GearState_GEAR_STATE_AVAILABLE
		communityID := firstID(d.gearCommunityIDs[g.Id])

		row := &api.HomeGearItem{
			GearId:           g.Id,
			ThumbnailMediaId: firstMediaID(g.MediaIds),
			CommunityId:      communityID,
			CommunityName:    d.communityNameMap[communityID],
			PillStyle:        api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_NEUTRAL,
			CreatedAtUnixSec: g.CreatedAtUnixSec,
		}
		if listed {
			row.Category = api.HomeGearCategory_HOME_GEAR_CATEGORY_LISTED
		} else {
			row.Category = api.HomeGearCategory_HOME_GEAR_CATEGORY_WITH_YOU
		}
		sortName[g.Id] = g.Name
		rows = append(rows, row)
	}

	// Order: dated obligations ascending, then undated OUT/BORROWING,
	// then LISTED, then WITH_YOU.
	rank := func(r *api.HomeGearItem) int {
		switch {
		case r.DueAtUnixSec > 0:
			return 0
		case r.Category == api.HomeGearCategory_HOME_GEAR_CATEGORY_OUT,
			r.Category == api.HomeGearCategory_HOME_GEAR_CATEGORY_BORROWING:
			return 1
		case r.Category == api.HomeGearCategory_HOME_GEAR_CATEGORY_LISTED:
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rank(rows[i]), rank(rows[j])
		if ri != rj {
			return ri < rj
		}
		if ri == 0 {
			return rows[i].DueAtUnixSec < rows[j].DueAtUnixSec
		}
		// Alphabetical by gear name. The name is no longer on the wire
		// (#2835), so it is carried alongside purely for this ordering.
		return sortName[rows[i].GearId] < sortName[rows[j].GearId]
	})

	counts := &api.HomeGearCounts{Total: int32(len(rows))}
	for _, r := range rows {
		switch r.Category {
		case api.HomeGearCategory_HOME_GEAR_CATEGORY_OUT:
			counts.Out++
		case api.HomeGearCategory_HOME_GEAR_CATEGORY_BORROWING:
			counts.Borrowing++
		case api.HomeGearCategory_HOME_GEAR_CATEGORY_LISTED:
			counts.Listed++
		case api.HomeGearCategory_HOME_GEAR_CATEGORY_WITH_YOU:
			counts.WithYou++
		}
	}

	hasMore := len(rows) > homeMaxGear
	if hasMore {
		rows = rows[:homeMaxGear]
	}
	return rows, counts, hasMore
}
