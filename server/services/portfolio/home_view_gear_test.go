package portfolio

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Gear
// ---------------------------------------------------------------------------.

func TestAssembleHomeGear_CategoriesPillsAndOrdering(t *testing.T) {
	d := newHomeData()
	dueSoon := homeNow.Unix() + 2*86400  // within 72h → amber
	dueLater := homeNow.Unix() + 9*86400 // beyond 72h → clay
	pickupAt := homeNow.Unix() + 86400
	startedAt := homeNow.Unix() - 11*86400

	// Out, due soon.
	t1 := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	t1.ActualPickupUnixSec = &startedAt
	t1.ExpectedReturnUnixSec = &dueSoon
	// Out, no date.
	t2 := makeLoanTransfer("t-2", selfID, otherID, "g-2", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	t2.ActualPickupUnixSec = &startedAt
	// Borrowing, pickup pending — owned by someone else, so it belongs in
	// Needs you (a status update), NOT in "Your stuff". It must be excluded
	// from the gear rows here.
	t3 := makeLoanTransfer("t-3", otherID, selfID, "g-3", models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, 1000)
	t3.EstimatedPickupUnixSec = &pickupAt
	// Out, due later.
	t4 := makeLoanTransfer("t-4", selfID, otherID, "g-4", models.TransferState_TRANSFER_STATE_ACTIVE, 1000)
	t4.ActualPickupUnixSec = &startedAt
	t4.ExpectedReturnUnixSec = &dueLater

	d.activeTransfers = []*models.Transfer{t4, t2, t3, t1}
	for _, g := range []struct{ id, name string }{
		{"g-1", "Drill"}, {"g-2", "Ladder"}, {"g-3", "Power washer"}, {"g-4", "Tent"},
	} {
		owner := selfID
		if g.id == "g-3" {
			owner = otherID
		}
		d.gearProtoMap[g.id] = makeGear(g.id, owner, g.name)
	}
	// Listed gear (no transfer).
	listed := makeGear("g-5", selfID, "Chainsaw")
	listed.State = models.GearState_GEAR_STATE_AVAILABLE
	// At-home gear (not shared anywhere).
	home := makeGear("g-6", selfID, "Snowshoes")
	home.State = models.GearState_GEAR_STATE_AVAILABLE
	d.ownedGear = []*models.Gear{listed, home}
	d.gearCommunityIDs["g-5"] = []string{"comm-1"}

	rows, counts, hasMore := assembleHomeGear(d, selfID, homeNow)
	if hasMore {
		t.Errorf("hasMore = true, want false")
	}
	// g-3 (borrowing) is excluded — only the viewer's own gear counts.
	if counts.Total != 5 || counts.Out != 3 || counts.Borrowing != 0 || counts.Listed != 1 || counts.WithYou != 1 {
		t.Errorf("counts = %+v", counts)
	}

	// Ordering: dated ascending (t1 due-soon, t4 due-later), then undated out
	// (t2), then listed, then with-you. The borrowed g-3 is gone.
	wantOrder := []string{"g-1", "g-4", "g-2", "g-5", "g-6"}
	if len(rows) != len(wantOrder) {
		t.Fatalf("got %d rows, want %d", len(rows), len(wantOrder))
	}
	for i, want := range wantOrder {
		if rows[i].GearId != want {
			t.Errorf("rows[%d] = %s, want %s", i, rows[i].GearId, want)
		}
	}

	byGear := map[string]*api.HomeGearItem{}
	for _, r := range rows {
		byGear[r.GearId] = r
	}
	if _, ok := byGear["g-3"]; ok {
		t.Errorf("borrowed g-3 should not appear in Your stuff")
	}
	if byGear["g-1"].PillStyle != api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_AMBER {
		t.Errorf("due-soon pill = %v, want AMBER", byGear["g-1"].PillStyle)
	}
	if byGear["g-4"].PillStyle != api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_CLAY {
		t.Errorf("due-later pill = %v, want CLAY", byGear["g-4"].PillStyle)
	}
	if byGear["g-2"].PillStyle != api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_CLAY || !byGear["g-2"].CanSetDate {
		t.Errorf("no-date row: pill %v canSetDate %v, want CLAY + true", byGear["g-2"].PillStyle, byGear["g-2"].CanSetDate)
	}
	if byGear["g-5"].PillStyle != api.HomeGearPillStyle_HOME_GEAR_PILL_STYLE_NEUTRAL {
		t.Errorf("listed pill = %v, want NEUTRAL", byGear["g-5"].PillStyle)
	}
}

func TestAssembleHomeGear_ListedShowsWaitingRequests(t *testing.T) {
	d := newHomeData()
	// A pending interest on listed gear does NOT make it OUT — it stays
	// listed with a waiting count (the decision lives in Needs you).
	tr := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED, 1000)
	d.activeTransfers = []*models.Transfer{tr}
	g := makeGear("g-1", selfID, "Chainsaw")
	g.State = models.GearState_GEAR_STATE_AVAILABLE
	d.ownedGear = []*models.Gear{g}
	d.gearCommunityIDs["g-1"] = []string{"comm-1"}

	rows, counts, _ := assembleHomeGear(d, selfID, homeNow)
	if len(rows) != 1 || counts.Listed != 1 || counts.Out != 0 {
		t.Fatalf("rows=%d counts=%+v, want 1 listed", len(rows), counts)
	}
	if rows[0].Category != api.HomeGearCategory_HOME_GEAR_CATEGORY_LISTED {
		t.Errorf("category = %v, want LISTED", rows[0].Category)
	}
}
