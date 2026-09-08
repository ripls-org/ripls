package workshop

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services/workshop/momentum"
	"go.ripls.org/ripls/server/storage"
)

// TestPersistedWorkshopNudges_PassLeverCopy walks every workshop-surface
// StoredNudge currently in the test database and asserts each cta_label
// passes the brief's lever-copy writing rule.
func TestPersistedWorkshopNudges_PassLeverCopy(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	insertWorkshopNudge(t, store, "user-1", "comm-1",
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD, "Quest", 100)
	insertWorkshopNudge(t, store, "user-1", "comm-1",
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK, "Back", 90)

	all, err := storage.QueryByFields[*models.StoredNudge](store, ctx, map[string]any{
		"user_id": "user-1",
	})
	if err != nil {
		t.Fatalf("query nudges: %v", err)
	}
	for _, n := range all {
		if !isWorkshopSurface(n.Surface) {
			continue
		}
		if err := momentum.ValidateLeverCopy(n.CtaLabel); err != nil {
			t.Errorf("nudge %s (%s) fails lever-copy rule: %v", n.Id, n.Surface, err)
		}
	}
}

func isWorkshopSurface(s models.NudgeSurface) bool {
	switch s {
	case models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK,
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_SEED_CATALYST:
		return true
	}
	return false
}
