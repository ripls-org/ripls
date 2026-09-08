package portfolio

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Up next
// ---------------------------------------------------------------------------.

// TestAssembleHomeUpNext_InvitedAndHostSetStatus verifies that an individual the
// host invited to an event (a member of the event's per-item community) sees it
// on their home up-next even before responding, and that the subtitle follows
// the RSVP — Invited / Going / Maybe. A declined invite drops off the agenda.
// Regression for #2492 (invited individuals didn't see the event at all).
func TestAssembleHomeUpNext_InvitedAndHostSetStatus(t *testing.T) {
	const expID = "exp-invited"
	futureTS := homeNow.Unix() + 86400 // tomorrow, after startOfToday

	// selfID is the invitee; otherID is the host/owner.
	build := func(intention models.RSVPIntention) *fetchedData {
		d := newHomeData()
		exp := makeExperienceWithTime(expID, otherID, models.ExperienceState_EXPERIENCE_STATE_JOINED, futureTS)
		exp.Name = "Sunset paddle"
		d.expMap[expID] = exp
		d.activeExpIDs = []string{expID}
		d.invitedExpIDs[expID] = true
		d.expCommunityIDs[expID] = []string{"comm-1"}
		if intention != models.RSVPIntention_RSVP_INTENTION_UNSPECIFIED {
			d.userRSVPIntentions[expID] = intention
			if intention == models.RSVPIntention_RSVP_INTENTION_YES || intention == models.RSVPIntention_RSVP_INTENTION_MAYBE {
				d.userRSVPTimes[expID] = futureTS
			}
		}
		return d
	}

	find := func(entries []*api.HomeUpNextEntry) *api.HomeUpNextEntry {
		for _, e := range entries {
			if e.ContentId == expID {
				return e
			}
		}
		return nil
	}

	cases := []struct {
		name       string
		intention  models.RSVPIntention
		wantShown  bool
		wantStatus api.HomeUpNextStatus
	}{
		{
			"invited, no reply", models.RSVPIntention_RSVP_INTENTION_UNSPECIFIED, true,
			api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_INVITED,
		},
		{
			"host set going", models.RSVPIntention_RSVP_INTENTION_YES, true,
			api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_GOING,
		},
		{
			"host set maybe", models.RSVPIntention_RSVP_INTENTION_MAYBE, true,
			api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_MAYBE,
		},
		{
			"declined drops off", models.RSVPIntention_RSVP_INTENTION_NO, false,
			api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_UNSPECIFIED,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := build(tc.intention)
			entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
			got := find(entries)
			if !tc.wantShown {
				if got != nil {
					t.Fatalf("expected event hidden from up-next, got status %v", got.Status)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected invited event in up-next, not found (%d entries)", len(entries))
			}
			if got.Title != "Sunset paddle" {
				t.Errorf("title = %q, want %q", got.Title, "Sunset paddle")
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %v, want %v", got.Status, tc.wantStatus)
			}
		})
	}
}

func TestAssembleHomeUpNext_EventsAndObligationsAscending(t *testing.T) {
	d := newHomeData()
	future1 := homeNow.Unix() + 3600
	future2 := homeNow.Unix() + 7200

	// Hosted event at future2.
	exp := makeExperienceWithTime("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, future2)
	d.expMap["e-1"] = exp
	d.activeExpIDs = []string{"e-1"}
	d.expAttendeesMap["e-1"] = []string{otherID, "user-3"}
	d.expCommunityIDs["e-1"] = []string{"comm-1"}

	// Due-back obligation at future1 (sorts before the event).
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	tr.ExpectedReturnUnixSec = &future1
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Drill")

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Kind != api.HomeUpNextKind_HOME_UP_NEXT_KIND_GEAR_OBLIGATION {
		t.Errorf("first entry kind = %v, want GEAR_OBLIGATION (ascending sort)", entries[0].Kind)
	}
	// The obligation row ships its parts, not a composed sentence (#2835).
	if entries[0].Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_DUE_BACK {
		t.Errorf("obligation status = %v, want OBLIGATION_DUE_BACK", entries[0].Status)
	}
	if entries[0].GetGearName() != "Drill" || entries[0].GetCounterpartyFirstName() != "Sarah" {
		t.Errorf("obligation parts = %q / %q, want Drill / Sarah",
			entries[0].GetGearName(), entries[0].GetCounterpartyFirstName())
	}
	if entries[1].Kind != api.HomeUpNextKind_HOME_UP_NEXT_KIND_EVENT {
		t.Errorf("second entry kind = %v, want EVENT", entries[1].Kind)
	}
	if entries[1].Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_HOSTING || entries[1].GoingCount != 2 {
		t.Errorf("event status = %v going = %d, want HOSTING / 2",
			entries[1].Status, entries[1].GoingCount)
	}
}

func TestAssembleHomeUpNext_UndatedAndPastExcluded(t *testing.T) {
	d := newHomeData()

	// Undated event.
	undated := makeExperience("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE)
	d.expMap["e-1"] = undated
	// Event from last week.
	past := makeExperienceWithTime("e-2", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, homeNow.Unix()-7*86400)
	d.expMap["e-2"] = past
	d.activeExpIDs = []string{"e-1", "e-2"}

	// Active loan with no return date — undated, lives in Gear instead.
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	d.activeTransfers = []*models.Transfer{tr}

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0 (undated and past excluded)", len(entries))
	}
}

// TestAssembleHomeUpNext_AllDayVsClock pins which rows the client renders as a
// clock and which as an all-day label. The server used to decide that itself in
// a rendered time_display string; since #2835 it ships time_unix_sec plus the
// all_day flag and the client formats in the viewer's locale.
func TestAssembleHomeUpNext_AllDayVsClock(t *testing.T) {
	d := newHomeData()
	tomorrow9am := homeNow.Unix() + 86400 // 2023-11-15 22:13 — a real time-of-day
	tomorrowMidnight := dayStartIn(homeNow, homeTZ) + 86400
	returnAt := homeNow.Unix() + 2*86400
	neededBy := homeNow.Unix() + 2*86400

	// Timed event → real clock.
	timed := makeExperienceWithTime("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, tomorrow9am)
	d.expMap["e-1"] = timed
	// All-day event (midnight + flag) → all-day, not a midnight clock.
	allDay := makeExperienceWithTime("e-2", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, tomorrowMidnight)
	allDay.Time.GetSpecific().IsAllDay = true
	d.expMap["e-2"] = allDay
	d.activeExpIDs = []string{"e-1", "e-2"}

	// Gear obligation (date-only) → all-day.
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	tr.ExpectedReturnUnixSec = &returnAt
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Drill")

	// Dated request (date-only) → all-day.
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.NeededByUnixSec = &neededBy
	d.ownedRequests = []*models.Request{req}

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	allDayByID := map[string]bool{}
	timeByID := map[string]int64{}
	for _, e := range entries {
		allDayByID[e.Id] = e.AllDay
		timeByID[e.Id] = e.TimeUnixSec
	}
	if allDayByID["e-1"] {
		t.Error("timed event all_day = true, want false (client renders a clock)")
	}
	if timeByID["e-1"] != tomorrow9am {
		t.Errorf("timed event time = %d, want %d", timeByID["e-1"], tomorrow9am)
	}
	for _, id := range []string{"e-2", "t-1", "r-1"} {
		if !allDayByID[id] {
			t.Errorf("%s all_day = false, want true", id)
		}
	}
}

func TestAssembleHomeUpNext_WatchedOnlyExperienceExcluded(t *testing.T) {
	d := newHomeData()
	exp := makeExperienceWithTime("e-1", otherID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, homeNow.Unix()+3600)
	d.expMap["e-1"] = exp
	d.activeExpIDs = []string{"e-1"} // present because watched, but no RSVP
	d.watchedExpIDs["e-1"] = true

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 0 {
		t.Fatalf("watched-only experience should not appear in Up next, got %d", len(entries))
	}
}

func TestAssembleHomeUpNext_PickupForRecipient(t *testing.T) {
	d := newHomeData()
	pickup := homeNow.Unix() + 86400
	tr := makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	tr.EstimatedPickupUnixSec = &pickup
	d.activeTransfers = []*models.Transfer{tr}
	d.gearProtoMap["g-1"] = makeGear("g-1", otherID, "Power washer")

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_PICKUP {
		t.Errorf("status = %v, want OBLIGATION_PICKUP", entries[0].Status)
	}
	if entries[0].GetGearName() != "Power washer" || entries[0].GetCounterpartyFirstName() != "Sarah" {
		t.Errorf("parts = %q / %q, want Power washer / Sarah",
			entries[0].GetGearName(), entries[0].GetCounterpartyFirstName())
	}
}

func TestAssembleHomeUpNext_GiveawayHandoff(t *testing.T) {
	d := newHomeData()
	pickup := homeNow.Unix() + 86400

	// Recipient side — picking up a free item.
	recv := makeLoanTransfer("t-1", otherID, selfID, "g-1", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	recv.TransferType = models.TransferType_TRANSFER_TYPE_GIVEAWAY
	recv.EstimatedPickupUnixSec = &pickup
	// Owner side — handing off a free item.
	give := makeLoanTransfer("t-2", selfID, otherID, "g-2", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	give.TransferType = models.TransferType_TRANSFER_TYPE_GIVEAWAY
	give.EstimatedPickupUnixSec = &pickup
	d.activeTransfers = []*models.Transfer{recv, give}
	d.gearProtoMap["g-1"] = makeGear("g-1", otherID, "Free lamp")
	d.gearProtoMap["g-2"] = makeGear("g-2", selfID, "Old desk")

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	byGear := map[string]*api.HomeUpNextEntry{}
	for _, e := range entries {
		if e.Kind != api.HomeUpNextKind_HOME_UP_NEXT_KIND_GEAR_OBLIGATION {
			t.Errorf("kind = %v, want GEAR_OBLIGATION", e.Kind)
		}
		byGear[e.ContentId] = e
	}
	// Receiving side: a pickup obligation flagged as a giveaway, with the
	// parts the client composes "Pick up Free lamp from Sarah" from.
	if byGear["g-1"].Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_PICKUP ||
		!byGear["g-1"].IsGiveaway {
		t.Errorf("receiving entry status = %v giveaway = %v, want OBLIGATION_PICKUP / true",
			byGear["g-1"].Status, byGear["g-1"].IsGiveaway)
	}
	if byGear["g-1"].GetGearName() != "Free lamp" || byGear["g-1"].GetCounterpartyFirstName() != "Sarah" {
		t.Errorf("receiving parts = %q / %q, want Free lamp / Sarah",
			byGear["g-1"].GetGearName(), byGear["g-1"].GetCounterpartyFirstName())
	}
	// Giving side: the same date is a handoff, not a pickup.
	if byGear["g-2"].Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_OBLIGATION_HANDOFF ||
		!byGear["g-2"].IsGiveaway {
		t.Errorf("giving entry status = %v giveaway = %v, want OBLIGATION_HANDOFF / true",
			byGear["g-2"].Status, byGear["g-2"].IsGiveaway)
	}
}

func TestAssembleHomeUpNext_DatedAskAppears(t *testing.T) {
	d := newHomeData()
	neededBy := homeNow.Unix() + 2*86400
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.Title = "Help moving this Saturday"
	req.Category = "muscle + truck"
	req.NeededByUnixSec = &neededBy
	d.ownedRequests = []*models.Request{req}
	d.reqCommunityIDs["r-1"] = []string{"comm-1"}

	extras := emptyHomeExtras()
	extras.needsByRequestID["r-1"] = []*models.PlanningNeed{
		{Id: "n-1", Slots: 2, SlotsRemaining: 1},
	}
	extras.offersByRequestID["r-1"] = []*models.RequestOffer{
		{Id: "o-1", RequestId: "r-1", UserId: otherID, CreatedAtUnixSec: 500},
	}

	entries := assembleHomeUpNext(d, extras, selfID, homeTZ, homeNow)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 dated request", len(entries))
	}
	e := entries[0]
	if e.Kind != api.HomeUpNextKind_HOME_UP_NEXT_KIND_ASK {
		t.Errorf("kind = %v, want ASK", e.Kind)
	}
	if e.KindTag != "muscle + truck" {
		t.Errorf("kind_tag = %q", e.KindTag)
	}
	if e.NeededCount != 1 {
		t.Errorf("needed_count = %d, want 1", e.NeededCount)
	}
	if len(e.Helpers) != 1 {
		t.Errorf("helpers = %d, want 1", len(e.Helpers))
	}
}

func TestAssembleHomeUpNext_UndatedAskExcluded(t *testing.T) {
	d := newHomeData()
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	d.ownedRequests = []*models.Request{req} // no NeededByUnixSec

	entries := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0 (undated request excluded)", len(entries))
	}
}
