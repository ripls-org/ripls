package gear

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// TestService_GetGear_DeletedGearReturnsTombstone exercises the lenient-read
// path added for #1701. A non-owner participant in a Transfer for a now-deleted
// gear (the prod alert signature) must receive a 200 tombstone with deleted=true
// instead of a 404 ERROR.
func TestService_GetGear_DeletedGearReturnsTombstone(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ownerCtx := createAuthenticatedContext("owner-1701", "owner-1701@example.com", models.Role_ROLE_USER)
	nonOwnerCtx := createAuthenticatedContext("recip-1701", "recip-1701@example.com", models.Role_ROLE_USER)

	owner := &models.User{Id: "owner-1701", Email: "owner-1701@example.com", Name: "Owner"}
	if _, err := testStorage.Insert(ownerCtx, owner); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	recipient := &models.User{Id: "recip-1701", Email: "recip-1701@example.com", Name: "Recipient"}
	if _, err := testStorage.Insert(ownerCtx, recipient); err != nil {
		t.Fatalf("insert recipient: %v", err)
	}

	createResp, err := service.SaveGear(ownerCtx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Cordless Drill"),
		Description: proto.String("18V, like new"),
	}))
	if err != nil {
		t.Fatalf("SaveGear: %v", err)
	}
	gearID := createResp.Msg.Id

	// Add a Transfer row so this is a faithful prod repro: recipient is a
	// non-owner participant who'd otherwise lose access on gear delete.
	if _, err := testStorage.Insert(ownerCtx, &models.Transfer{
		Id:           "transfer-1701",
		GearId:       gearID,
		OwnerId:      "owner-1701",
		RecipientId:  "recip-1701",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	// Owner deletes the gear.
	if _, err := service.DeleteGear(ownerCtx, connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})); err != nil {
		t.Fatalf("DeleteGear: %v", err)
	}

	// Non-owner GetGear must return a tombstone, not 404. This is the exact
	// alert path that fired in prod.
	resp, err := service.GetGear(nonOwnerCtx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
	if err != nil {
		t.Fatalf("GetGear on soft-deleted gear must return tombstone, got error: %v", err)
	}
	if !resp.Msg.GetDeleted() {
		t.Errorf("expected deleted=true on tombstone, got false")
	}
	if resp.Msg.Id != gearID {
		t.Errorf("expected tombstone id=%q, got %q", gearID, resp.Msg.Id)
	}
	if resp.Msg.Name != "Cordless Drill" {
		t.Errorf("expected last-known name preserved, got %q", resp.Msg.Name)
	}
	if resp.Msg.Owner == nil || resp.Msg.Owner.Id != "owner-1701" {
		t.Errorf("expected owner id preserved on tombstone, got %+v", resp.Msg.Owner)
	}
}

// TestService_GetGear_GenuinelyMissingGearStillErrors pins the invariant from
// #1184: the lenient-read path must only swallow soft-deleted gear, not truly
// missing IDs. A real data corruption / bad client id must still ERROR so
// oncall is alerted.
func TestService_GetGear_GenuinelyMissingGearStillErrors(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("user-1701-missing", "missing@example.com", models.Role_ROLE_USER)
	user := &models.User{Id: "user-1701-missing", Email: "missing@example.com", Name: "Missing"}
	if _, err := testStorage.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	_, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: "00000000-0000-0000-0000-000000000000"}))
	if err == nil {
		t.Fatal("expected error for nonexistent gear, got nil")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestService_GetGearPeople_DeletedGearReturnsPeople verifies the second
// transfer-bound surface from the inventory in #1701: GetGearPeople must keep
// rendering owner + borrowers from surviving Transfer rows after the gear is
// soft-deleted.
func TestService_GetGearPeople_DeletedGearReturnsPeople(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ownerCtx := createAuthenticatedContext("owner-people-1701", "op-1701@example.com", models.Role_ROLE_USER)
	borrowerCtx := createAuthenticatedContext("borrower-people-1701", "bp-1701@example.com", models.Role_ROLE_USER)

	if _, err := testStorage.Insert(ownerCtx, &models.User{Id: "owner-people-1701", Email: "op-1701@example.com", Name: "Owner P"}); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	if _, err := testStorage.Insert(ownerCtx, &models.User{Id: "borrower-people-1701", Email: "bp-1701@example.com", Name: "Borrower P"}); err != nil {
		t.Fatalf("insert borrower: %v", err)
	}

	createResp, err := service.SaveGear(ownerCtx, connect.NewRequest(&api.SaveGearRequest{
		Name: proto.String("Pressure Washer"),
	}))
	if err != nil {
		t.Fatalf("SaveGear: %v", err)
	}
	gearID := createResp.Msg.Id

	// A completed transfer (audit-trail row) — survives the cascade and
	// remains visible after delete.
	if _, err := testStorage.Insert(ownerCtx, &models.Transfer{
		Id:           "transfer-people-1701",
		GearId:       gearID,
		OwnerId:      "owner-people-1701",
		RecipientId:  "borrower-people-1701",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	if _, err := service.DeleteGear(ownerCtx, connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})); err != nil {
		t.Fatalf("DeleteGear: %v", err)
	}

	// Borrower (non-owner) should still see owner + past borrower after delete.
	resp, err := service.GetGearPeople(borrowerCtx, connect.NewRequest(&api.GetGearPeopleRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("GetGearPeople on soft-deleted gear: %v", err)
	}
	if resp.Msg.Owner == nil || resp.Msg.Owner.Id != "owner-people-1701" {
		t.Errorf("expected owner preserved, got %+v", resp.Msg.Owner)
	}
	foundPastBorrower := false
	for _, b := range resp.Msg.PastBorrowers {
		if b.Id == "borrower-people-1701" {
			foundPastBorrower = true
		}
	}
	if !foundPastBorrower {
		t.Errorf("expected borrower in past_borrowers, got %+v", resp.Msg.PastBorrowers)
	}
}

// TestService_DeleteGear_TransferRowsSurviveCascade pins the deliberate
// non-cascade decision from #1701: Transfer rows are audit trail. A future
// drift to Option B (cascade-soft-delete Transfers) must fail this test before
// merge.
func TestService_DeleteGear_TransferRowsSurviveCascade(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("survive-owner", "so@example.com", models.Role_ROLE_USER)
	if _, err := testStorage.Insert(ctx, &models.User{Id: "survive-owner", Email: "so@example.com", Name: "S"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	createResp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name: proto.String("Compound Miter Saw"),
	}))
	if err != nil {
		t.Fatalf("SaveGear: %v", err)
	}
	gearID := createResp.Msg.Id

	// One pending and one completed transfer — both must remain in storage
	// (not soft-deleted) after DeleteGear runs.
	transfers := []*models.Transfer{
		{
			Id:           "survive-pending",
			GearId:       gearID,
			OwnerId:      "survive-owner",
			RecipientId:  "any-recipient",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		},
		{
			Id:           "survive-completed",
			GearId:       gearID,
			OwnerId:      "survive-owner",
			RecipientId:  "any-recipient",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		},
	}
	for _, tr := range transfers {
		if _, err := testStorage.Insert(ctx, tr); err != nil {
			t.Fatalf("insert transfer %s: %v", tr.Id, err)
		}
	}

	if _, err := service.DeleteGear(ctx, connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})); err != nil {
		t.Fatalf("DeleteGear: %v", err)
	}

	for _, want := range transfers {
		got := &models.Transfer{}
		if err := testStorage.GetByID(ctx, want.Id, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("GetByID transfer %s: %v", want.Id, err)
		}
		if got.Deleted != nil {
			t.Errorf("transfer %s was cascade-soft-deleted (Deleted=%+v); #1701 mandates audit-trail preservation",
				want.Id, got.Deleted)
		}
	}
}
