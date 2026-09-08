package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestListExperienceNeedsAndContributions_SurfacesTransfer is the #1142
// read-path guard for #2708: a contribution whose gear was brought to the event
// as a real transfer must surface that transfer's id/state/type on the list
// read path, not just on the write path that created it.
func TestListExperienceNeedsAndContributions_SurfacesTransfer(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	ownerCtx := createAuthenticatedContext("host-2708", "host2708@test.com", models.Role_ROLE_USER)
	createTestUser(t, testStorage, "host-2708", "host2708@test.com", "Host")

	saveResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Yard day",
		Description: "Bring tools",
	}))
	if err != nil {
		t.Fatalf("SaveExperience: %v", err)
	}
	expID := saveResp.Msg.Experience.Id

	addResp, err := service.AddExperienceContribution(ownerCtx, connect.NewRequest(&api.AddExperienceContributionRequest{
		ExperienceId: expID,
		Title:        "Wheelbarrow",
	}))
	if err != nil {
		t.Fatalf("AddExperienceContribution: %v", err)
	}
	contribID := addResp.Msg.Contribution.Id

	// Seed a child transfer and link it onto the contribution, the way
	// OfferExperienceTransfer would.
	transfer := &models.Transfer{
		GearId:       "gear-x",
		OwnerId:      "host-2708",
		RecipientId:  "host-2708",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		Origin:       &models.Transfer_OriginExperienceId{OriginExperienceId: expID},
	}
	transferID, err := testStorage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	contribution := &models.PlanningContribution{}
	if err := testStorage.GetByID(ctx, contribID, contribution); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	contribution.TransferId = &transferID
	if err := testStorage.Update(ctx, contribution); err != nil {
		t.Fatalf("link transfer: %v", err)
	}

	listResp, err := service.ListExperienceNeedsAndContributions(ownerCtx, connect.NewRequest(&api.ListExperienceNeedsAndContributionsRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("ListExperienceNeedsAndContributions: %v", err)
	}

	var found *api.ExperienceContributionResponse
	for _, c := range listResp.Msg.Contributions {
		if c.Id == contribID {
			found = c
		}
	}
	if found == nil {
		t.Fatalf("contribution %s not in list", contribID)
	}
	if found.GetTransferId() != transferID {
		t.Errorf("transfer_id = %q, want %q", found.GetTransferId(), transferID)
	}
	if found.GetTransferState() != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Errorf("transfer_state = %s, want RECIPIENT_SELECTED", found.GetTransferState())
	}
	if found.GetTransferType() != api.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("transfer_type = %s, want LOAN", found.GetTransferType())
	}
}
