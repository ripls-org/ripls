package feed

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// nudgeForTest builds a minimal StoredNudge suitable for storage tests.
func nudgeForTest(userID, communityID string, isTerminator bool, createdAt int64) *models.StoredNudge {
	return &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Test headline",
		Description:      "Test description",
		CtaLabel:         "Go",
		CtaAction:        "plan_experience",
		StockQuery:       "outdoor activity",
		CreatedAtUnixSec: createdAt,
		IsTerminator:     isTerminator,
	}
}

// insertGlobalTerminatorPair inserts two global terminator nudges (today and 25h
// ago) and returns (todayNudge, yesterdayNudge, todayDayOfYear).
func insertGlobalTerminatorPair(t *testing.T, sqlStorage *storage.ProtoSQLStorage, ctx context.Context) (*models.StoredNudge, *models.StoredNudge, int) {
	t.Helper()
	now := time.Now().Unix()
	todayNudge := nudgeForTest(GlobalTerminatorUserID, "", true, now)
	if err := insertNudge(ctx, sqlStorage, todayNudge); err != nil {
		t.Fatalf("insertNudge today: %v", err)
	}
	yesterdayNudge := nudgeForTest(GlobalTerminatorUserID, "", true, now-25*3600)
	if err := insertNudge(ctx, sqlStorage, yesterdayNudge); err != nil {
		t.Fatalf("insertNudge yesterday: %v", err)
	}
	return todayNudge, yesterdayNudge, unixSecToDayOfYear(now)
}

// TestUpdateNudgeMediaID_SetsField verifies that updateNudgeMediaID persists a
// new media_id on an existing nudge.
func TestUpdateNudgeMediaID_SetsField(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "media@test.com", "Media User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Media Community")

	nudge := nudgeForTest(userID, communityID, false, time.Now().Unix())
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	const newMediaID = "media-set-123"
	if err := updateNudgeMediaID(ctx, sqlStorage, nudge.Id, newMediaID); err != nil {
		t.Fatalf("updateNudgeMediaID: %v", err)
	}

	fetched := &models.StoredNudge{}
	if err := sqlStorage.GetByID(ctx, nudge.Id, fetched); err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if fetched.MediaId == nil || *fetched.MediaId != newMediaID {
		t.Errorf("MediaId = %v, want %q", fetched.MediaId, newMediaID)
	}
}

// TestUpdateNudgeMediaID_NotFound verifies that updateNudgeMediaID returns a
// non-nil error when the nudge ID does not exist.
func TestUpdateNudgeMediaID_NotFound(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	err := updateNudgeMediaID(ctx, sqlStorage, "00000000-0000-0000-0000-000000000000", "media-x")
	if err == nil {
		t.Error("expected error for unknown nudge ID, got nil")
	}
}

// TestGetGlobalTerminatorsForToday_FiltersByDay verifies that terminators
// created on a different day-of-year are excluded from the result.
func TestGetGlobalTerminatorsForToday_FiltersByDay(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	todayNudge, _, today := insertGlobalTerminatorPair(t, sqlStorage, ctx)

	results, err := getGlobalTerminatorsForToday(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getGlobalTerminatorsForToday: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (today's only), got %d", len(results))
	}
	if results[0].Id != todayNudge.Id {
		t.Errorf("returned nudge ID %q, want %q", results[0].Id, todayNudge.Id)
	}
}

// TestGetGlobalTerminatorsForToday_ExcludesSoftDeleted verifies that soft-deleted
// global terminators are not returned even if they were created today.
func TestGetGlobalTerminatorsForToday_ExcludesSoftDeleted(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	now := time.Now().Unix()
	nudge := nudgeForTest(GlobalTerminatorUserID, "", true, now)
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}
	if err := softDeleteNudge(ctx, sqlStorage, nudge.Id); err != nil {
		t.Fatalf("softDeleteNudge: %v", err)
	}

	today := unixSecToDayOfYear(now)
	results, err := getGlobalTerminatorsForToday(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getGlobalTerminatorsForToday: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (soft-deleted), got %d", len(results))
	}
}

// TestGetPreviousGlobalTerminators_ReturnsStale verifies that terminators from a
// previous day are returned by getPreviousGlobalTerminators.
func TestGetPreviousGlobalTerminators_ReturnsStale(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	_, yesterdayNudge, today := insertGlobalTerminatorPair(t, sqlStorage, ctx)

	stale, err := getPreviousGlobalTerminators(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getPreviousGlobalTerminators: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale result, got %d", len(stale))
	}
	if stale[0].Id != yesterdayNudge.Id {
		t.Errorf("stale nudge ID %q, want %q", stale[0].Id, yesterdayNudge.Id)
	}
}

// TestGetPreviousGlobalTerminators_ExcludesSoftDeleted verifies that soft-deleted
// stale terminators are not returned.
func TestGetPreviousGlobalTerminators_ExcludesSoftDeleted(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	now := time.Now().Unix()
	yesterdayNudge := nudgeForTest(GlobalTerminatorUserID, "", true, now-25*3600)
	if err := insertNudge(ctx, sqlStorage, yesterdayNudge); err != nil {
		t.Fatalf("insertNudge yesterday: %v", err)
	}
	if err := softDeleteNudge(ctx, sqlStorage, yesterdayNudge.Id); err != nil {
		t.Fatalf("softDeleteNudge: %v", err)
	}

	today := unixSecToDayOfYear(now)
	stale, err := getPreviousGlobalTerminators(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getPreviousGlobalTerminators: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 results (soft-deleted stale), got %d", len(stale))
	}
}
