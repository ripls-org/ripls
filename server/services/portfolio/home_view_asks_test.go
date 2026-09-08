package portfolio

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Your requests
// ---------------------------------------------------------------------------.

func TestAssembleHomeAsks_DeDupesByRequestID(t *testing.T) {
	d := newHomeData()
	// The same request returned multiple times (e.g. one row per shared
	// community) must surface as a single request, not "Fence Demolition" ×3.
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	req.Title = "Fence Demolition Help Needed"
	d.ownedRequests = []*models.Request{req, req, req}

	asks := assembleHomeAsks(d, emptyHomeExtras())
	if len(asks) != 1 {
		t.Fatalf("got %d requests, want 1 (de-duped by request ID)", len(asks))
	}
}

func TestAssembleHomeAsks_ProgressAndLifecycle(t *testing.T) {
	d := newHomeData()
	active := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_ACTIVE)
	active.CreatedAtUnixSec = 2000
	fulfilled := makeRequest("r-2", selfID, models.RequestState_REQUEST_STATE_FULFILLED)
	cancelled := makeRequest("r-3", selfID, models.RequestState_REQUEST_STATE_CANCELLED)
	older := makeRequest("r-4", selfID, models.RequestState_REQUEST_STATE_OFFERS_RECEIVED)
	older.CreatedAtUnixSec = 1000
	d.ownedRequests = []*models.Request{active, fulfilled, cancelled, older}
	d.reqCommunityIDs["r-1"] = []string{"comm-1"}

	extras := emptyHomeExtras()
	extras.needsByRequestID["r-1"] = []*models.PlanningNeed{
		{Id: "n-1", Slots: 5, SlotsRemaining: 1},
		{Id: "n-2", Slots: 2, SlotsRemaining: 0},
	}

	asks := assembleHomeAsks(d, extras)
	if len(asks) != 2 {
		t.Fatalf("got %d requests, want 2 (terminal states excluded)", len(asks))
	}
	// Newest first.
	if asks[0].RequestId != "r-1" || asks[1].RequestId != "r-4" {
		t.Errorf("order = [%s, %s], want newest first", asks[0].RequestId, asks[1].RequestId)
	}
	if asks[0].ClaimedCount != 6 || asks[0].TotalCount != 7 {
		t.Errorf("progress = %d of %d, want 6 of 7", asks[0].ClaimedCount, asks[0].TotalCount)
	}
	if asks[0].CommunityName != "Boulder BC" {
		t.Errorf("community_name = %q", asks[0].CommunityName)
	}
	// No needs posted → zero/zero.
	if asks[1].ClaimedCount != 0 || asks[1].TotalCount != 0 {
		t.Errorf("no-needs request progress = %d of %d, want 0 of 0", asks[1].ClaimedCount, asks[1].TotalCount)
	}
}

func TestAssembleOwnedEvents_HostedOnly(t *testing.T) {
	d := newHomeData()
	soon := homeNow.Unix() + 86400
	later := homeNow.Unix() + 3*86400

	hostedLater := makeExperienceWithTime("e-1", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, later)
	hostedSoon := makeExperienceWithTime("e-2", selfID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, soon)
	concluded := makeExperienceWithTime("e-3", selfID, models.ExperienceState_EXPERIENCE_STATE_COMPLETED, soon)
	someoneElses := makeExperienceWithTime("e-4", otherID, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, soon)
	d.expMap = map[string]proto.Message{
		"e-1": hostedLater, "e-2": hostedSoon, "e-3": concluded, "e-4": someoneElses,
	}
	d.expAttendeesMap["e-2"] = []string{otherID, "u-3"}

	events := assembleOwnedEvents(d, selfID)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (hosted, non-terminal)", len(events))
	}
	// Soonest dated first.
	if events[0].EventId != "e-2" || events[1].EventId != "e-1" {
		t.Errorf("order = %s, %s; want e-2, e-1", events[0].EventId, events[1].EventId)
	}
}
