package workshop

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services/workshop/momentum"
	"go.ripls.org/ripls/server/storage"
)

// seedGappedRhythm inserts CalendarGapMinInstances completed experiences
// sharing one name, all finished 20 days ago — inside the 90-day rhythm
// window and past the 14-day gap threshold — so CalendarGapDetector fires.
// Returns the id of the most recent instance, which is the detection's
// ContextID.
//
// It deliberately inserts no CommunityExperience join rows: without them
// RecentlyCompletedEvents finds nothing, so EnsureHeroCard's per-event loop
// stays quiet and the *drift* branch is the only thing that can persist a
// card. That is the branch these tests are about, and it is the one the
// deleted idle-offer detector used to exercise (#2892).
func seedGappedRhythm(t *testing.T, store *storage.ProtoSQLStorage, userID, name string) string {
	t.Helper()
	ctx := context.Background()
	newest := ""
	for i := 0; i < momentum.CalendarGapMinInstances; i++ {
		// i == 0 is the most recent; older instances step a week further back.
		completedAt := time.Now().AddDate(0, 0, -20-(7*i)).Unix()
		id, err := store.Insert(ctx, &models.Experience{
			Name:               name,
			OwnerId:            userID,
			State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
			CompletedAtUnixSec: &completedAt,
		})
		if err != nil {
			t.Fatalf("insert experience: %v", err)
		}
		if i == 0 {
			newest = id
		}
	}
	return newest
}

// heroCardCount returns how many active hero-card nudges exist for the pair.
// Tests assert on this rather than on EnsureHeroCard's return slice alone —
// the two can diverge, and a slice-only assertion passes vacuously.
func heroCardCount(t *testing.T, store *storage.ProtoSQLStorage, userID, communityID string) int {
	t.Helper()
	all, err := storage.QueryByFields[*models.StoredNudge](store, context.Background(),
		map[string]any{"user_id": userID, "community_id": communityID})
	if err != nil {
		t.Fatalf("query nudges: %v", err)
	}
	count := 0
	for _, n := range all {
		if n.Surface == models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD &&
			n.ConsumedAtUnixSec == nil {
			count++
		}
	}
	return count
}

func TestEnsureHeroCard_EmptyCommunityNoOp(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)

	got, err := svc.EnsureHeroCard(context.Background(), "user-1", "comm-empty", time.Now())
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no nudges (no slot matched), got %d", len(got))
	}

	all, err := storage.QueryByFields[*models.StoredNudge](store, context.Background(),
		map[string]any{"user_id": "user-1"})
	if err != nil {
		t.Fatalf("query nudges: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected no nudges persisted, got %d", len(all))
	}
}

func TestEnsureHeroCard_DriftSignalProducesHeroCard(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)

	expID := seedGappedRhythm(t, store, "user-1", "Sunday brunch")

	got, err := svc.EnsureHeroCard(context.Background(), "user-1", "comm-1", time.Now())
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected hero card to be persisted")
	}
	first := got[0]
	if first.Surface != models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD {
		t.Errorf("Surface: want WORKSHOP_HERO_CARD, got %v", first.Surface)
	}
	if first.UserId != "user-1" || first.CommunityId != "comm-1" {
		t.Errorf("ownership mismatch: user=%s community=%s", first.UserId, first.CommunityId)
	}
	if first.CtaAction != "revive_experience" {
		t.Errorf("CtaAction: want revive_experience, got %q", first.CtaAction)
	}
	// The card must point at the experience it names, so the CTA's draft
	// lookup resolves. Pointing it at an entity of another kind is what
	// dead-ended the idle-offer lever (#2892).
	if first.ContextId == nil || *first.ContextId != expID {
		t.Errorf("ContextId = %v, want the most recent instance %s", first.ContextId, expID)
	}
	if n := heroCardCount(t, store, "user-1", "comm-1"); n != 1 {
		t.Errorf("persisted hero cards = %d, want 1", n)
	}
}

// TestEnsureHeroCard_SharedGearAloneProducesNothing — a host whose only
// activity is owning gear shared into the circle gets no card. Before #2892
// they got "Share your {gear} with the circle / Create a sharing event so
// everyone can borrow it at once", which asked for something already done,
// described an object that does not exist, and dead-ended on tap.
func TestEnsureHeroCard_SharedGearAloneProducesNothing(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	gearID, err := store.Insert(ctx, &models.Gear{
		Name: "12-Piece Metric Ratcheting Wrench Set", OwnerId: "user-1",
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{
		CommunityId: "comm-1", GearId: gearID,
	}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}

	got, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", time.Now())
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no hero cards, got %d", len(got))
	}
	if n := heroCardCount(t, store, "user-1", "comm-1"); n != 0 {
		t.Errorf("persisted hero cards = %d, want 0", n)
	}
}

func TestEnsureHeroCard_IsIdempotent(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	seedGappedRhythm(t, store, "user-1", "Sunday brunch")

	first, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", time.Now())
	if err != nil || len(first) == 0 {
		t.Fatalf("first EnsureHeroCard: %v / %v", err, first)
	}
	second, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", time.Now())
	if err != nil {
		t.Fatalf("second EnsureHeroCard: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("expected second call to be a no-op, got %d new nudges", len(second))
	}
	if count := heroCardCount(t, store, "user-1", "comm-1"); count != 1 {
		t.Errorf("expected exactly 1 hero card persisted, got %d", count)
	}
}

func TestEnsureHeroCard_RegeneratesAfterConsumed(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	seedGappedRhythm(t, store, "user-1", "Sunday brunch")

	first, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", time.Now())
	if err != nil || len(first) == 0 {
		t.Fatalf("first EnsureHeroCard: %v / %v", err, first)
	}
	firstNudge := first[0]

	consumedAt := time.Now().Unix()
	firstNudge.ConsumedAtUnixSec = &consumedAt
	consumedAction := firstNudge.CtaAction
	firstNudge.ConsumedAction = &consumedAction
	if err := store.Update(ctx, firstNudge); err != nil {
		t.Fatalf("mark consumed: %v", err)
	}

	second, err := svc.EnsureHeroCard(ctx, "user-1", "comm-1", time.Now())
	if err != nil {
		t.Fatalf("second EnsureHeroCard: %v", err)
	}
	if len(second) == 0 {
		t.Error("expected fresh hero card after the prior was consumed")
	}
	if len(second) > 0 && second[0].Id == firstNudge.Id {
		t.Errorf("expected a new nudge, got the same id %s", firstNudge.Id)
	}
}

// TestEnsureBringBackItems_IsIdempotent — calling twice does not
// double-persist Bring-Back nudges.
func TestEnsureBringBackItems_IsIdempotent(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 4; i++ {
		ts := now.AddDate(0, 0, -30-(7*i)).Unix()
		exp := &models.Experience{
			Name:               "Cookout",
			OwnerId:            "user-1",
			State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
			CompletedAtUnixSec: &ts,
		}
		if _, err := store.Insert(ctx, exp); err != nil {
			t.Fatalf("insert exp: %v", err)
		}
	}

	first, err := svc.EnsureBringBackItems(ctx, "user-1", "comm-1", "", now)
	if err != nil {
		t.Fatalf("first EnsureBringBackItems: %v", err)
	}
	if first < 1 {
		t.Errorf("first call: want >= 1 nudge persisted, got %d", first)
	}
	second, err := svc.EnsureBringBackItems(ctx, "user-1", "comm-1", "", now)
	if err != nil {
		t.Fatalf("second EnsureBringBackItems: %v", err)
	}
	if second != 0 {
		t.Errorf("second call: want 0 nudges persisted (idempotent), got %d", second)
	}
}

// TestContextMediaID_InheritsFromTheReferencedEntity — a "schedule {event}
// again" card wears that event's own photo. These prompts skip stock imagery,
// so without this the card is a bare headline on black; the run's own picture
// is both free and more recognisable than anything a stock query would return.
func TestContextMediaID_InheritsFromTheReferencedEntity(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	expID, err := store.Insert(ctx, &models.Experience{
		Name:     "Wednesday morning run",
		OwnerId:  "user-1",
		MediaIds: []string{"media-run", "media-coffee"},
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if got := svc.contextMediaID(ctx, expID); got != "media-run" {
		t.Errorf("experience media = %q, want media-run (the first)", got)
	}
}

// TestContextMediaID_NoMediaIsNotAnError — an entity with no photo, or an id
// that resolves to nothing, leaves the card on a solid background rather than
// failing materialization.
func TestContextMediaID_NoMediaIsNotAnError(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	bare, err := store.Insert(ctx, &models.Experience{Name: "No photo", OwnerId: "user-1"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	for name, id := range map[string]string{
		"entity with no media": bare,
		"unknown id":           "does-not-exist",
		"empty id":             "",
	} {
		t.Run(name, func(t *testing.T) {
			if got := svc.contextMediaID(ctx, id); got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

// TestEnsureHeroCard_CardCarriesTheEventPhoto — the inheritance reaches the
// persisted row, not just the helper.
func TestEnsureHeroCard_CardCarriesTheEventPhoto(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store, nil)
	ctx := context.Background()

	expID := seedGappedRhythm(t, store, "user-1", "Sunday brunch")
	exp := &models.Experience{}
	if err := store.GetByID(ctx, expID, exp); err != nil {
		t.Fatalf("read experience: %v", err)
	}
	exp.MediaIds = []string{"media-brunch"}
	if err := store.Update(ctx, exp); err != nil {
		t.Fatalf("update experience: %v", err)
	}

	got, err := svc.EnsureHeroCard(ctx, "user-1", "comm-media", time.Now())
	if err != nil {
		t.Fatalf("EnsureHeroCard: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected a hero card for the gapped rhythm")
	}
	if got[0].MediaId == nil || *got[0].MediaId != "media-brunch" {
		t.Errorf("card media = %v, want media-brunch", got[0].MediaId)
	}
}
