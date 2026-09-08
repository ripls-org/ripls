package workshop

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// completedExperienceIn seeds a completed experience joined to a
// community at a given offset from `now`. Returns the experience id.
func completedExperienceIn(
	t *testing.T,
	store *storage.ProtoSQLStorage,
	name, ownerID, communityID string,
	completedAtUnixSec int64,
) string {
	t.Helper()
	ctx := context.Background()
	exp := &models.Experience{
		Name:               name,
		OwnerId:            ownerID,
		State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		CompletedAtUnixSec: &completedAtUnixSec,
	}
	expID, err := store.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityExperience{
		CommunityId: communityID, ExperienceId: expID,
	}); err != nil {
		t.Fatalf("insert join: %v", err)
	}
	return expID
}

// TestEnsureHeroCard_PerEventEmitsOnePerRecentEvent — the
// load-bearing per-event guarantee from docs/ai/workshop.md §3.1:
// every recently-completed event surfaces a Hero card.
func TestEnsureHeroCard_PerEventEmitsOnePerRecentEvent(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()
	now := time.Now()

	// Three recently-completed events at different offsets within the
	// 14-day window — none of them have ESM signal or active follow-on,
	// so all three fall through to the generic recap fallback.
	completedExperienceIn(t, store, "Sunday brunch", "user-1", "comm-1",
		now.AddDate(0, 0, -2).Unix())
	completedExperienceIn(t, store, "Smoker night", "user-1", "comm-1",
		now.AddDate(0, 0, -5).Unix())
	completedExperienceIn(t, store, "Trail morning", "user-1", "comm-1",
		now.AddDate(0, 0, -10).Unix())

	got, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", now)
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	if len(got) < 3 {
		t.Errorf("expected at least 3 hero cards (one per recent event), got %d", len(got))
	}
	contextSet := map[string]bool{}
	for _, n := range got {
		if n.Surface != models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD {
			t.Errorf("Surface mismatch: want HERO_CARD, got %v", n.Surface)
		}
		if n.ContextId == nil || *n.ContextId == "" {
			t.Error("expected per-event hero card to carry a context_id")
			continue
		}
		if contextSet[*n.ContextId] {
			t.Errorf("duplicate context_id %s — per-event dedup failed", *n.ContextId)
		}
		contextSet[*n.ContextId] = true
	}
}

// TestEnsureHeroCard_PerEventSkipsAlreadyCovered — re-running
// EnsureHeroCard does not duplicate per-event nudges. Idempotent.
func TestEnsureHeroCard_PerEventSkipsAlreadyCovered(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()
	now := time.Now()

	completedExperienceIn(t, store, "Sunday brunch", "user-1", "comm-1",
		now.AddDate(0, 0, -2).Unix())

	first, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", now)
	if err != nil || len(first) == 0 {
		t.Fatalf("first EnsureHeroCard: %v / %d", err, len(first))
	}
	second, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", now)
	if err != nil {
		t.Fatalf("second EnsureHeroCard: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("expected idempotent second run (0 new nudges), got %d", len(second))
	}
}

// TestEnsureHeroCard_DriftCardDeduplicatesAgainstPerEvent — when a
// drift detector (e.g., calendar gap) returns a detection whose
// context_id matches a per-event hero card already in scope, the
// drift card is suppressed.
func TestEnsureHeroCard_DriftCardDeduplicatesAgainstPerEvent(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()
	now := time.Now()

	// Seed enough Sunday brunch instances that CalendarGapDetector
	// would normally fire (3+ instances, gap from most-recent).
	for i := 0; i < 4; i++ {
		ts := now.AddDate(0, 0, -2-(7*i)).Unix()
		completedExperienceIn(t, store, "Sunday brunch", "user-1", "comm-1", ts)
	}

	// Per-event will fire on the most-recent brunch (within 14d) and
	// emit a hero card with its context_id. Calendar gap would also
	// fire and want the same context_id; the drift dedup should
	// suppress the duplicate.
	got, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", now)
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	contextSet := map[string]bool{}
	for _, n := range got {
		if n.ContextId == nil {
			continue
		}
		if contextSet[*n.ContextId] {
			t.Errorf("duplicate context_id %s — drift dedup failed", *n.ContextId)
		}
		contextSet[*n.ContextId] = true
	}
}
