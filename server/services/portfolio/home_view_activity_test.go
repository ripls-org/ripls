package portfolio

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ---------------------------------------------------------------------------
// Recent activity
// ---------------------------------------------------------------------------.

func TestAssembleHomeActivity_CompletedItemsNewestFirst(t *testing.T) {
	d := newHomeData()
	recent := homeNow.Unix() - 86400
	older := homeNow.Unix() - 5*86400
	ancient := homeNow.Unix() - (homeActivityWindowDays+5)*86400

	// Completed loan (viewer lent).
	t1 := makeLoanTransfer("t-1", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_COMPLETED, 1000)
	t1.ActualReturnUnixSec = &older
	// Completed loan outside the window — excluded.
	t2 := makeLoanTransfer("t-2", selfID, otherID, "g-1", models.TransferState_TRANSFER_STATE_COMPLETED, 1000)
	t2.ActualReturnUnixSec = &ancient
	d.allTransfers = []*models.Transfer{t1, t2}
	d.gearProtoMap["g-1"] = makeGear("g-1", selfID, "Drill")

	// Fulfilled request.
	req := makeRequest("r-1", selfID, models.RequestState_REQUEST_STATE_FULFILLED)
	req.FulfilledAtUnixSec = &recent
	d.reqMap["r-1"] = req

	entries := assembleHomeActivity(d, selfID, homeNow)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Id != "r-1" || entries[1].Id != "t-1" {
		t.Errorf("order = [%s, %s], want newest first", entries[0].Id, entries[1].Id)
	}
	if entries[1].Title != "Drill came back from Sarah" {
		t.Errorf("loan title = %q", entries[1].Title)
	}
}
