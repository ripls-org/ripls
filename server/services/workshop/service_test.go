package workshop

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func insertWorkshopNudge(
	t *testing.T,
	store *storage.ProtoSQLStorage,
	userID, communityID string,
	surface models.NudgeSurface,
	headline string,
	createdAt int64,
) *models.StoredNudge {
	t.Helper()
	mediaID := "media-1"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          surface,
		NudgeVariant:     1,
		Headline:         headline,
		Description:      "test description",
		CtaLabel:         "Schedule round 6",
		CtaAction:        "schedule_repeat",
		MediaId:          &mediaID,
		StockQuery:       "test",
		CreatedAtUnixSec: createdAt,
	}
	if _, err := store.Insert(context.Background(), nudge); err != nil {
		t.Fatalf("insert nudge: %v", err)
	}
	return nudge
}
