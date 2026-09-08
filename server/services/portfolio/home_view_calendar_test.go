package portfolio

import (
	"testing"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Calendar
// ---------------------------------------------------------------------------.

func TestAssembleHomeCalendar_AnchorsByItemType(t *testing.T) {
	d := newHomeData()
	recentPast := homeNow.Unix() - 7*86400  // within the 180-day window
	gearShared := homeNow.Unix() - 10*86400 // within window
	reqShared := homeNow.Unix() - 5*86400   // within window
	dueDate := homeNow.Unix() + 3*86400     // upcoming

	// Past event the viewer hosted — calendar shows it (up_next would not).
	pastEvent := makeExperienceWithTime("e-past", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, recentPast)
	d.expMap["e-past"] = pastEvent
	d.activeExpIDs = []string{"e-past"}

	// Gear first-shared marker.
	g := makeGear("g-1", selfID, "Drill")
	g.State = models.GearState_GEAR_STATE_AVAILABLE
	g.CreatedAtUnixSec = gearShared
	// Gear shared long ago — beyond the window, excluded.
	gOld := makeGear("g-old", selfID, "Old tent")
	gOld.State = models.GearState_GEAR_STATE_AVAILABLE
	gOld.CreatedAtUnixSec = homeNow.Unix() - 200*86400
	d.ownedGear = []*models.Gear{g, gOld}

	// Request with no due date → placed on its first-shared (created) date.
	reqShare := makeRequest("r-shared", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	reqShare.CreatedAtUnixSec = reqShared
	// Request with a due date → placed on the due date.
	reqDue := makeRequest("r-due", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	reqDue.NeededByUnixSec = &dueDate
	d.ownedRequests = []*models.Request{reqShare, reqDue}

	entries := assembleHomeCalendar(d, emptyHomeExtras(), selfID, homeTZ, homeNow)

	byID := map[string]*api.HomeUpNextEntry{}
	for _, e := range entries {
		byID[e.Id] = e
	}

	if _, ok := byID["g-old"]; ok {
		t.Errorf("gear shared beyond the past window should be excluded")
	}
	if e := byID["g-1"]; e == nil {
		t.Fatalf("gear first-shared marker missing")
	} else {
		if e.Kind != api.HomeUpNextKind_HOME_UP_NEXT_KIND_GEAR_SHARED {
			t.Errorf("gear marker kind = %v, want GEAR_SHARED", e.Kind)
		}
		if e.TimeUnixSec != gearShared {
			t.Errorf("gear marker time = %d, want %d (created date)", e.TimeUnixSec, gearShared)
		}
		if e.Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_ADDED_TO_LIBRARY {
			t.Errorf("gear marker status = %v, want ADDED_TO_LIBRARY", e.Status)
		}
		if !e.AllDay {
			t.Error("gear marker all_day = false, want true")
		}
	}
	if e := byID["r-shared"]; e == nil {
		t.Fatalf("undated request missing from calendar")
	} else if e.TimeUnixSec != reqShared ||
		e.Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_YOUR_REQUEST {
		t.Errorf("undated request anchored wrong: time=%d status=%v", e.TimeUnixSec, e.Status)
	}
	if e := byID["r-due"]; e == nil {
		t.Fatalf("dated request missing from calendar")
	} else if e.TimeUnixSec != dueDate ||
		e.Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_YOUR_REQUEST_DUE {
		t.Errorf("dated request anchored wrong: time=%d status=%v", e.TimeUnixSec, e.Status)
	}
	if e := byID["e-past"]; e == nil {
		t.Errorf("recent past event missing from calendar (calendar includes recent past)")
	} else if e.TimeUnixSec != recentPast {
		t.Errorf("event anchored at %d, want its scheduled date %d", e.TimeUnixSec, recentPast)
	}

	// The same recent-past event must NOT leak into up_next (upcoming-only).
	upNext := assembleHomeUpNext(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	for _, e := range upNext {
		if e.Id == "e-past" {
			t.Errorf("recent past event leaked into up_next")
		}
	}

	// Entries are ascending by time.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].TimeUnixSec > entries[i].TimeUnixSec {
			t.Errorf("calendar not ascending at %d: %d > %d", i, entries[i-1].TimeUnixSec, entries[i].TimeUnixSec)
		}
	}
}

func TestAssembleHomeCalendar_IncludesItemsFromOtherOwners(t *testing.T) {
	d := newHomeData()
	gearAccess := homeNow.Unix() - 8*86400 // shared into the community 8 days ago
	reqShared := homeNow.Unix() - 6*86400
	eventAt := homeNow.Unix() + 4*86400

	// A community event owned by someone else (viewer has not RSVP'd).
	otherEvent := makeExperienceWithTime("e-other", otherID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, eventAt)
	d.expMap["e-other"] = otherEvent
	d.expCommunityIDs["e-other"] = []string{"comm-1"}

	// A community gear owned by someone else, shared into the viewer's community.
	otherGear := makeGear("g-other", otherID, "Sarah's ladder")
	otherGear.State = models.GearState_GEAR_STATE_AVAILABLE
	d.allCommunityGearMap = map[string]proto.Message{"g-other": otherGear}
	d.cgByGearID = map[string]*models.CommunityGear{
		"g-other": {GearId: "g-other", CommunityId: "comm-1", CreatedAtUnixSec: gearAccess},
	}
	d.gearCommunityIDs["g-other"] = []string{"comm-1"}

	// A community request owned by someone else, no due date.
	otherReq := makeRequest("r-other", otherID, models.RequestState_REQUEST_STATE_ACTIVE)
	d.reqMap["r-other"] = otherReq
	d.commReqMap["r-other"] = &models.CommunityRequest{
		RequestId: "r-other", CommunityId: "comm-1", SharedAtUnixSec: reqShared,
	}
	d.reqCommunityIDs["r-other"] = []string{"comm-1"}

	entries := assembleHomeCalendar(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	byID := map[string]*api.HomeUpNextEntry{}
	for _, e := range entries {
		byID[e.Id] = e
	}

	if e := byID["e-other"]; e == nil {
		t.Errorf("community event from another owner missing from calendar")
	} else if e.TimeUnixSec != eventAt {
		t.Errorf("other-owner event time = %d, want %d", e.TimeUnixSec, eventAt)
	}
	if e := byID["g-other"]; e == nil {
		t.Fatalf("community gear from another owner missing from calendar")
	} else {
		if e.TimeUnixSec != gearAccess {
			t.Errorf("other-owner gear anchored at %d, want access date %d", e.TimeUnixSec, gearAccess)
		}
		if e.Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_SHARED_WITH_YOU {
			t.Errorf("other-owner gear status = %v, want SHARED_WITH_YOU", e.Status)
		}
	}
	if e := byID["r-other"]; e == nil {
		t.Fatalf("community request from another owner missing from calendar")
	} else if e.TimeUnixSec != reqShared ||
		e.Status != api.HomeUpNextStatus_HOME_UP_NEXT_STATUS_REQUEST_SHARED {
		t.Errorf("other-owner request anchored wrong: time=%d status=%v", e.TimeUnixSec, e.Status)
	}
}

func TestOrderedCommunityIDs(t *testing.T) {
	if got := orderedCommunityIDs(nil); len(got) != 0 {
		t.Errorf("empty = %v, want []", got)
	}
	got := orderedCommunityIDs([]string{"c-3", "c-1", "c-2"})
	if len(got) != 3 || got[0] != "c-1" || got[1] != "c-2" || got[2] != "c-3" {
		t.Errorf("got %v, want [c-1 c-2 c-3] (sorted, stable primary)", got)
	}
	if got := orderedCommunityIDs([]string{"", "c-2"}); len(got) != 1 || got[0] != "c-2" {
		t.Errorf("got %v, want [c-2] (skips empty)", got)
	}
}

// TestStableNamedFirst verifies real (named) communities sort before nameless
// ad-hoc per-item communities, stably within each group — the ordering that
// keeps the display-community pick off an item's own ad-hoc backing community
// (#2675).
func TestStableNamedFirst(t *testing.T) {
	names := map[string]string{"named": "Travis Heights", "adhoc": "", "named2": "Boulder BC"}
	ids := []string{"adhoc", "named", "named2"} // raw query order — ad-hoc first
	stableNamedFirst(ids, names)
	want := []string{"named", "named2", "adhoc"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("stableNamedFirst = %v, want %v", ids, want)
		}
	}

	// All-nameless keeps its original order (nothing to prefer).
	only := []string{"a", "b"}
	stableNamedFirst(only, map[string]string{"a": "", "b": ""})
	if only[0] != "a" || only[1] != "b" {
		t.Errorf("all-nameless order changed: %v", only)
	}
}

// TestAssembleHomeCalendar_PrefersNamedCommunity is the #2675 regression: an
// event shared into both a real named community and its nameless ad-hoc per-item
// community is attributed to the NAMED community for display, and carries the
// full set in community_ids so the community-scoped calendar (which matches on
// community_ids) shows it under the real community.
func TestAssembleHomeCalendar_PrefersNamedCommunity(t *testing.T) {
	const expID = "e-craft"
	eventAt := homeNow.Unix() + 40*86400 // ~40 days out, like the report
	d := newHomeData()
	// A real named community plus the event's nameless ad-hoc backing community.
	d.communityNameMap = map[string]string{"travis": "Travis Heights", "adhoc": ""}
	exp := makeExperienceWithTime(expID, selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, eventAt)
	exp.Name = "Neighborhood Craft Circle"
	d.expMap[expID] = exp
	// fetchAll orders each community-id slice named-first; the raw pivot query
	// order is arbitrary (ad-hoc could be first), so mirror the ordering step.
	ids := []string{"adhoc", "travis"}
	stableNamedFirst(ids, d.communityNameMap)
	d.expCommunityIDs[expID] = ids

	entries := assembleHomeCalendar(d, emptyHomeExtras(), selfID, homeTZ, homeNow)
	var got *api.HomeUpNextEntry
	for _, e := range entries {
		if e.ContentId == expID {
			got = e
		}
	}
	if got == nil {
		t.Fatalf("event missing from calendar")
	}
	if got.CommunityId != "travis" {
		t.Errorf("display community_id = %q, want named community 'travis'", got.CommunityId)
	}
	if got.CommunityName != "Travis Heights" {
		t.Errorf("community_name = %q, want 'Travis Heights'", got.CommunityName)
	}
	// The full set, named-first, so an item shared into several communities
	// matches each one's filtered calendar.
	if len(got.CommunityIds) != 2 || got.CommunityIds[0] != "travis" || got.CommunityIds[1] != "adhoc" {
		t.Errorf("community_ids = %v, want [travis adhoc]", got.CommunityIds)
	}
}
