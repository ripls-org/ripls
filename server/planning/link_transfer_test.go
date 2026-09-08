package planning

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertLinkableContribution inserts a request-scoped contribution owned by
// userID, optionally linking gearID, and returns its id.
func insertLinkableContribution(t *testing.T, st *storage.ProtoSQLStorage, userID, requestID, gearID string) string {
	t.Helper()
	c := &models.PlanningContribution{
		ContributorId: userID,
		Title:         "Lawn mower",
		Scope:         &models.PlanningContribution_RequestId{RequestId: requestID},
	}
	if gearID != "" {
		c.GearId = &gearID
	}
	id, err := st.Insert(context.Background(), c)
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}
	return id
}

func TestValidateTransferLink_OK(t *testing.T) {
	st := setupTestStorage(t)
	id := insertLinkableContribution(t, st, "user-1", "req-1", "gear-1")

	c, err := ValidateTransferLink(context.Background(), st, "user-1", id, Scope{RequestID: "req-1"}, "gear-1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if c.Id != id {
		t.Errorf("expected contribution %s, got %s", id, c.Id)
	}
}

func TestValidateTransferLink_ExperienceScope_OK(t *testing.T) {
	st := setupTestStorage(t)
	c := &models.PlanningContribution{
		ContributorId: "user-1",
		Title:         "Wheelbarrow",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: "exp-1"},
		GearId:        ptr("gear-1"),
	}
	id, err := st.Insert(context.Background(), c)
	if err != nil {
		t.Fatalf("insert contribution: %v", err)
	}

	got, err := ValidateTransferLink(context.Background(), st, "user-1", id, Scope{ExperienceID: "exp-1"}, "gear-1")
	if err != nil {
		t.Fatalf("expected success for experience scope, got %v", err)
	}
	if got.Id != id {
		t.Errorf("expected contribution %s, got %s", id, got.Id)
	}

	// Wrong scope kind (request id for an experience contribution) is rejected.
	if _, err := ValidateTransferLink(context.Background(), st, "user-1", id, Scope{RequestID: "exp-1"}, "gear-1"); err == nil {
		t.Fatal("expected rejection when scope kind mismatches")
	}
}

func ptr(s string) *string { return &s }

func TestValidateTransferLink_NoGearOnContribution_OK(t *testing.T) {
	st := setupTestStorage(t)
	id := insertLinkableContribution(t, st, "user-1", "req-1", "")

	if _, err := ValidateTransferLink(context.Background(), st, "user-1", id, Scope{RequestID: "req-1"}, "gear-1"); err != nil {
		t.Fatalf("expected success for gear-less contribution, got %v", err)
	}
}

func TestValidateTransferLink_Rejections(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	id := insertLinkableContribution(t, st, "user-1", "req-1", "gear-1")

	tests := []struct {
		name                                      string
		userID, contributionID, requestID, gearID string
	}{
		{"missing contribution", "user-1", "nope", "req-1", "gear-1"},
		{"wrong owner", "user-2", id, "req-1", "gear-1"},
		{"wrong request scope", "user-1", id, "req-2", "gear-1"},
		{"different gear", "user-1", id, "req-1", "gear-2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateTransferLink(ctx, st, tc.userID, tc.contributionID, Scope{RequestID: tc.requestID}, tc.gearID)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("expected CodeInvalidArgument, got %v", got)
			}
		})
	}
}

func TestUnlinkTransfer_ClearsMatchingLink(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	id := insertLinkableContribution(t, st, "user-1", "req-1", "gear-1")

	c := &models.PlanningContribution{}
	if err := st.GetByID(ctx, id, c); err != nil {
		t.Fatalf("load contribution: %v", err)
	}
	transferID := "transfer-1"
	c.TransferId = &transferID
	if err := st.Update(ctx, c); err != nil {
		t.Fatalf("link contribution: %v", err)
	}

	if err := UnlinkTransfer(ctx, st, Scope{RequestID: "req-1"}, "transfer-1"); err != nil {
		t.Fatalf("UnlinkTransfer: %v", err)
	}
	if err := st.GetByID(ctx, id, c); err != nil {
		t.Fatalf("reload contribution: %v", err)
	}
	if c.GetTransferId() != "" {
		t.Errorf("expected transfer link cleared, got %q", c.GetTransferId())
	}

	// Replayed unlink is a no-op.
	if err := UnlinkTransfer(ctx, st, Scope{RequestID: "req-1"}, "transfer-1"); err != nil {
		t.Fatalf("replayed UnlinkTransfer: %v", err)
	}
}
