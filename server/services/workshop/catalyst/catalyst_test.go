package catalyst

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func ptr[T any](v T) *T { return &v }

func insertExperienceInCommunity(t *testing.T, store *storage.ProtoSQLStorage, name, ownerID, communityID string, completedAt int64) {
	t.Helper()
	ctx := context.Background()
	exp := &models.Experience{
		Name:               name,
		OwnerId:            ownerID,
		State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		CompletedAtUnixSec: ptr(completedAt),
	}
	expID, err := store.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("insert exp: %v", err)
	}
	ce := &models.CommunityExperience{
		CommunityId:  communityID,
		ExperienceId: expID,
	}
	if _, err := store.Insert(ctx, ce); err != nil {
		t.Fatalf("insert community_experience: %v", err)
	}
}

func insertGearInCommunity(t *testing.T, store *storage.ProtoSQLStorage, name, ownerID, communityID string) {
	t.Helper()
	ctx := context.Background()
	gear := &models.Gear{Name: name, OwnerId: ownerID}
	gearID, err := store.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID}
	if _, err := store.Insert(ctx, cg); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}
}

// --- HostModeEligibleUserIDs ---.

func TestHostModeEligibleUserIDs_HostingMakesEligible(t *testing.T) {
	store := setupTestStorage(t)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)
	insertExperienceInCommunity(t, store, "Hike", "u2", "c1", 200)

	got, err := HostModeEligibleUserIDs(context.Background(), store, "c1", "")
	if err != nil {
		t.Fatalf("HostModeEligibleUserIDs: %v", err)
	}
	want := map[string]bool{"u1": true, "u2": true}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected eligible user: %s", id)
		}
	}
	if len(got) != 2 {
		t.Errorf("want 2 eligible, got %d", len(got))
	}
}

func TestHostModeEligibleUserIDs_GearListingMakesEligible(t *testing.T) {
	store := setupTestStorage(t)
	insertGearInCommunity(t, store, "drill", "u3", "c1")

	got, err := HostModeEligibleUserIDs(context.Background(), store, "c1", "")
	if err != nil {
		t.Fatalf("HostModeEligibleUserIDs: %v", err)
	}
	if len(got) != 1 || got[0] != "u3" {
		t.Errorf("want [u3], got %v", got)
	}
}

func TestHostModeEligibleUserIDs_ExcludesHostThemself(t *testing.T) {
	store := setupTestStorage(t)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)
	insertExperienceInCommunity(t, store, "Hike", "u2", "c1", 200)

	got, err := HostModeEligibleUserIDs(context.Background(), store, "c1", "u1")
	if err != nil {
		t.Fatalf("HostModeEligibleUserIDs: %v", err)
	}
	if len(got) != 1 || got[0] != "u2" {
		t.Errorf("want [u2] only, got %v", got)
	}
}

// --- DetectLoad ---.

func TestDetectLoad_BalancedRhythmNotLopsided(t *testing.T) {
	store := setupTestStorage(t)
	// 3 instances of "Brunch", different hosts each time.
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 300)
	insertExperienceInCommunity(t, store, "Brunch", "u2", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u3", "c1", 100)

	got, err := DetectLoad(context.Background(), store, "c1")
	if err != nil {
		t.Fatalf("DetectLoad: %v", err)
	}
	if got.Lopsided {
		t.Errorf("expected balanced (not lopsided), got %+v", got)
	}
}

func TestDetectLoad_SameHost3InARowIsLopsided(t *testing.T) {
	store := setupTestStorage(t)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 300)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)

	got, err := DetectLoad(context.Background(), store, "c1")
	if err != nil {
		t.Fatalf("DetectLoad: %v", err)
	}
	if !got.Lopsided {
		t.Errorf("expected lopsided, got %+v", got)
	}
	if got.OverloadedUserID != "u1" {
		t.Errorf("OverloadedUserID: want u1, got %s", got.OverloadedUserID)
	}
	if got.RhythmName != "Brunch" {
		t.Errorf("RhythmName: want Brunch, got %s", got.RhythmName)
	}
	if got.StreakLength < LoadStreakThreshold {
		t.Errorf("StreakLength: want >= %d, got %d", LoadStreakThreshold, got.StreakLength)
	}
}

func TestDetectLoad_TooFewInstancesNotLopsided(t *testing.T) {
	store := setupTestStorage(t)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)

	got, err := DetectLoad(context.Background(), store, "c1")
	if err != nil {
		t.Fatalf("DetectLoad: %v", err)
	}
	if got.Lopsided {
		t.Errorf("expected not-lopsided (only 2 instances), got %+v", got)
	}
}

// --- GetSuggestion ---.

func TestGetSuggestion_BalancedReturnsNoSuggestion(t *testing.T) {
	store := setupTestStorage(t)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 300)
	insertExperienceInCommunity(t, store, "Brunch", "u2", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u3", "c1", 100)

	got, err := GetSuggestion(context.Background(), store, "c1", "u1", time.Now())
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	if got.LoadSignal.Lopsided {
		t.Error("expected balanced load signal")
	}
	if got.SuggestedUserID != "" {
		t.Errorf("expected no suggestion, got %s", got.SuggestedUserID)
	}
	if got.NoEligibleRecipient {
		t.Error("expected NoEligibleRecipient false (load is balanced)")
	}
}

func TestGetSuggestion_LopsidedWithEligibleCandidate(t *testing.T) {
	store := setupTestStorage(t)
	// u1 hosts brunch 3 times in a row — lopsided.
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 300)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)
	// u2 has hosted in this community before (different rhythm) — eligible.
	insertExperienceInCommunity(t, store, "Hike", "u2", "c1", 50)

	got, err := GetSuggestion(context.Background(), store, "c1", "u1", time.Now())
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	if !got.LoadSignal.Lopsided {
		t.Error("expected lopsided load")
	}
	if got.SuggestedUserID != "u2" {
		t.Errorf("SuggestedUserID: want u2, got %s", got.SuggestedUserID)
	}
	if got.NoEligibleRecipient {
		t.Error("expected NoEligibleRecipient false")
	}
}

func TestGetSuggestion_LopsidedNoEligibleSetsFlag(t *testing.T) {
	store := setupTestStorage(t)
	// u1 hosts brunch 3 times — lopsided.
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 300)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 200)
	insertExperienceInCommunity(t, store, "Brunch", "u1", "c1", 100)
	// No other eligible candidates — only u1 has done originating actions.

	got, err := GetSuggestion(context.Background(), store, "c1", "u1", time.Now())
	if err != nil {
		t.Fatalf("GetSuggestion: %v", err)
	}
	if !got.LoadSignal.Lopsided {
		t.Error("expected lopsided load")
	}
	if got.SuggestedUserID != "" {
		t.Errorf("expected empty SuggestedUserID, got %s", got.SuggestedUserID)
	}
	if !got.NoEligibleRecipient {
		t.Error("expected NoEligibleRecipient true (only host is eligible)")
	}
}
