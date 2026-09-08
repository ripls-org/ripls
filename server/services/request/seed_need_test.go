package request

import (
	"context"
	"sort"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// requestNeeds returns the live planning needs for a request.
func requestNeeds(t *testing.T, st *storage.ProtoSQLStorage, requestID string) []*models.PlanningNeed {
	t.Helper()
	needs, err := storage.QueryByField[*models.PlanningNeed](st, context.Background(), "request_id", requestID)
	if err != nil {
		t.Fatalf("query needs: %v", err)
	}
	return needs
}

// needNames returns the sorted set of need names for a request. Sorted because
// the storage query does not guarantee insertion order.
func needNames(needs []*models.PlanningNeed) []string {
	names := make([]string, 0, len(needs))
	for _, n := range needs {
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names
}

func TestSubmitRequest_SeedsSingleNeedFromSeedNames(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	requesterID := setupTestUser(t, testStorage, "Requester", "seed-name@example.com")
	ctx := createAuthenticatedContext(requesterID, "seed-name@example.com", models.Role_ROLE_USER)

	resp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Looking for a lawn mower this weekend",
		Description:   "Ours gave up mid-mow",
		MediaIds:      []string{"media-static"},
		SeedNeedNames: []string{"Lawn mower"},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}

	needs := requestNeeds(t, testStorage, resp.Msg.RequestId)
	if len(needs) != 1 {
		t.Fatalf("expected exactly 1 seeded need, got %d", len(needs))
	}
	if needs[0].Name != "Lawn mower" {
		t.Errorf("expected seeded need named 'Lawn mower', got %q", needs[0].Name)
	}
	if needs[0].Slots != 1 || needs[0].SlotsRemaining != 1 {
		t.Errorf("expected 1/1 slots, got %d/%d", needs[0].SlotsRemaining, needs[0].Slots)
	}
	if needs[0].ProposerId != requesterID {
		t.Errorf("expected proposer %s, got %s", requesterID, needs[0].ProposerId)
	}
}

// A request whose text plainly names several things is born with one claimable
// need per named thing — the classroom-supply-drive case (#2731).
func TestSubmitRequest_SeedsMultipleNeedsFromSeedNames(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	requesterID := setupTestUser(t, testStorage, "Teacher", "seed-multi@example.com")
	ctx := createAuthenticatedContext(requesterID, "seed-multi@example.com", models.Role_ROLE_USER)

	want := []string{"Picture books", "Whiteboard", "Storage bins", "Art supplies", "Construction paper"}
	resp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Back-to-school supplies for Room 7",
		Description:   "We need picture books, a whiteboard, storage bins, art supplies, and construction paper.",
		MediaIds:      []string{"media-static"},
		SeedNeedNames: want,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}

	needs := requestNeeds(t, testStorage, resp.Msg.RequestId)
	if len(needs) != len(want) {
		t.Fatalf("expected %d seeded needs, got %d (%v)", len(want), len(needs), needNames(needs))
	}
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	got := needNames(needs)
	for i := range sortedWant {
		if got[i] != sortedWant[i] {
			t.Fatalf("seeded needs mismatch: got %v, want %v", got, sortedWant)
		}
	}
	for _, n := range needs {
		if n.Slots != 1 || n.SlotsRemaining != 1 {
			t.Errorf("need %q: expected 1/1 slots, got %d/%d", n.Name, n.SlotsRemaining, n.Slots)
		}
		if n.ProposerId != requesterID {
			t.Errorf("need %q: expected proposer %s, got %s", n.Name, requesterID, n.ProposerId)
		}
	}
}

// The server trims blanks, drops case-insensitive duplicates, and caps the
// count regardless of what a client sends — the repeated field is bounded at
// the create boundary, not only in the AI provider (#2731).
func TestSubmitRequest_SeedNeedsAreTrimmedDedupedAndCapped(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	requesterID := setupTestUser(t, testStorage, "Requester", "seed-cap@example.com")
	ctx := createAuthenticatedContext(requesterID, "seed-cap@example.com", models.Role_ROLE_USER)

	// Blanks are dropped; "Bolts" / "  bolts " collapse to one; then 12 unique
	// names exceed the cap of maxSeedNeeds (8).
	names := []string{"Bolts", "  bolts ", "   ", "Screws"}
	for i := 0; i < 12; i++ {
		names = append(names, "Item "+string(rune('A'+i)))
	}
	resp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Hardware for the build",
		Description:   "A pile of odds and ends.",
		MediaIds:      []string{"media-static"},
		SeedNeedNames: names,
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}

	needs := requestNeeds(t, testStorage, resp.Msg.RequestId)
	if len(needs) != maxSeedNeeds {
		t.Fatalf("expected cap of %d seeded needs, got %d (%v)", maxSeedNeeds, len(needs), needNames(needs))
	}
	// The blank-only entry never becomes a need, and the case-variant duplicate
	// of "Bolts" is not seeded twice.
	seen := map[string]int{}
	for _, n := range needs {
		if n.Name == "" {
			t.Errorf("blank need name was seeded")
		}
		seen[n.Name]++
	}
	if seen["Bolts"] > 1 {
		t.Errorf("case-insensitive duplicate 'Bolts' seeded %d times", seen["Bolts"])
	}
}

func TestSubmitRequest_NoSeedNamesLeavesTheListEmpty(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	requesterID := setupTestUser(t, testStorage, "Requester", "seed-title@example.com")
	ctx := createAuthenticatedContext(requesterID, "seed-title@example.com", models.Role_ROLE_USER)

	resp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Back-to-school supplies for Room 7",
		Description: "The classroom is bare and we need help getting it ready.",
		MediaIds:    []string{"media-static"},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}

	// The title used to be seeded as the need, which restated the request's own
	// headline as a thing to bring. Nobody claims "Back-to-school supplies for
	// Room 7", so the request could never read as covered — and the compose
	// sheet that exists to fill an empty list could never open (#2731).
	needs := requestNeeds(t, testStorage, resp.Msg.RequestId)
	if len(needs) != 0 {
		t.Fatalf("expected no seeded need when nothing concrete was named, got %d: %q",
			len(needs), needs[0].Name)
	}
}
