package planning

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestListNeedsAndContributions_Empty(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	// No needs or contributions exist for this experience.
	result, err := ListNeedsAndContributions(ctx, st, Scope{ExperienceID: "exp-empty"})
	if err != nil {
		t.Fatalf("ListNeedsAndContributions: %v", err)
	}
	if len(result.Needs) != 0 {
		t.Errorf("Needs length = %d, want 0", len(result.Needs))
	}
	if len(result.Contributions) != 0 {
		t.Errorf("Contributions length = %d, want 0", len(result.Contributions))
	}
}

// TestListNeedsAndContributions_IncludesFullyClaimedNeeds verifies the
// v2 behavior: fully-claimed needs (SlotsRemaining == 0) are returned
// alongside available ones so the client's unified Volunteer-sheet
// list can render every Need with its voter stack. The previous v1
// behavior excluded them — see docs/client/needs.md.
func TestListNeedsAndContributions_IncludesFullyClaimedNeeds(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	proposer := &models.User{Email: "proposer@example.com", Name: "Proposer"}
	proposerID, err := st.Insert(ctx, proposer)
	if err != nil {
		t.Fatalf("insert proposer: %v", err)
	}

	scopeID := "exp-includes-claimed"

	available := &models.PlanningNeed{
		ProposerId:     proposerID,
		Name:           "Bring snacks",
		Slots:          2,
		SlotsRemaining: 2,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: scopeID},
	}
	if _, err := st.Insert(ctx, available); err != nil {
		t.Fatalf("insert available need: %v", err)
	}

	full := &models.PlanningNeed{
		ProposerId:     proposerID,
		Name:           "Bring drinks",
		Slots:          1,
		SlotsRemaining: 0,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: scopeID},
	}
	if _, err := st.Insert(ctx, full); err != nil {
		t.Fatalf("insert full need: %v", err)
	}

	result, err := ListNeedsAndContributions(ctx, st, Scope{ExperienceID: scopeID})
	if err != nil {
		t.Fatalf("ListNeedsAndContributions: %v", err)
	}

	if len(result.Needs) != 2 {
		t.Fatalf("Needs length = %d, want 2 (v2 returns fully-claimed needs too)", len(result.Needs))
	}
	names := map[string]bool{}
	for _, n := range result.Needs {
		names[n.Need.Name] = true
	}
	if !names["Bring snacks"] || !names["Bring drinks"] {
		t.Errorf("expected both 'Bring snacks' and 'Bring drinks' in result, got %v", names)
	}
}

func TestListNeedsAndContributions_EnrichesUsers(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	proposer := &models.User{Email: "proposer2@example.com", Name: "Proposer Two"}
	proposerID, err := st.Insert(ctx, proposer)
	if err != nil {
		t.Fatalf("insert proposer: %v", err)
	}

	contributor := &models.User{Email: "contrib@example.com", Name: "Contrib One"}
	contributorID, err := st.Insert(ctx, contributor)
	if err != nil {
		t.Fatalf("insert contributor: %v", err)
	}

	scopeID := "exp-enrich-users"

	need := &models.PlanningNeed{
		ProposerId:     proposerID,
		Name:           "Bring plates",
		SlotsRemaining: 1,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: scopeID},
	}
	if _, err := st.Insert(ctx, need); err != nil {
		t.Fatalf("insert need: %v", err)
	}

	contrib := &models.PlanningContribution{
		ContributorId: contributorID,
		Title:         "I'll bring plates",
		Scope:         &models.PlanningContribution_ExperienceId{ExperienceId: scopeID},
	}
	if _, err := st.Insert(ctx, contrib); err != nil {
		t.Fatalf("insert contribution: %v", err)
	}

	result, err := ListNeedsAndContributions(ctx, st, Scope{ExperienceID: scopeID})
	if err != nil {
		t.Fatalf("ListNeedsAndContributions: %v", err)
	}

	if len(result.Needs) != 1 {
		t.Fatalf("Needs length = %d, want 1", len(result.Needs))
	}
	if result.Needs[0].Proposer == nil {
		t.Fatal("Proposer is nil")
	}
	if result.Needs[0].Proposer.Name != "Proposer Two" {
		t.Errorf("Proposer.Name = %q, want %q", result.Needs[0].Proposer.Name, "Proposer Two")
	}

	if len(result.Contributions) != 1 {
		t.Fatalf("Contributions length = %d, want 1", len(result.Contributions))
	}
	if result.Contributions[0].Contributor == nil {
		t.Fatal("Contributor is nil")
	}
	if result.Contributions[0].Contributor.Name != "Contrib One" {
		t.Errorf("Contributor.Name = %q, want %q", result.Contributions[0].Contributor.Name, "Contrib One")
	}
}

func TestListNeedsAndContributions_DropsOrphanedReferences(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	scopeID := "exp-orphan"

	// Need whose proposer does not exist — should be silently dropped.
	orphan := &models.PlanningNeed{
		ProposerId:     "nonexistent-user-id",
		Name:           "Orphaned need",
		SlotsRemaining: 1,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: scopeID},
	}
	if _, err := st.Insert(ctx, orphan); err != nil {
		t.Fatalf("insert orphan need: %v", err)
	}

	result, err := ListNeedsAndContributions(ctx, st, Scope{ExperienceID: scopeID})
	if err != nil {
		t.Fatalf("ListNeedsAndContributions: %v", err)
	}

	if len(result.Needs) != 0 {
		t.Errorf("Needs length = %d, want 0 (orphaned need should be dropped)", len(result.Needs))
	}
}
