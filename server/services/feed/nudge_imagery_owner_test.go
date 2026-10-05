package feed

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/storage"
)

// A terminator nudge is one shared card served to everyone, so its imagery has
// to be readable by everyone. Owning the copy by whoever's feed load happened
// to trigger it left every other viewer with PermissionDenied from GetMedia and
// a broken image (#3105); media.SystemUserID is public by design.
func TestFetchNudgeImagery_OwnsImageryAsSystem(t *testing.T) {
	ctx := context.Background()
	sqlStorage := setupTestStorage(t)

	bucket, err := storage.NewLocalBucketStorage(t.TempDir()+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("NewLocalBucketStorage: %v", err)
	}
	svc := &Service{
		sqlStorage:           sqlStorage,
		bucketStorage:        bucket,
		stockImageryProvider: mediapkg.NewFakeProvider(sqlStorage, bucket),
	}

	nudge := &models.StoredNudge{
		UserId:       GlobalTerminatorUserID,
		IsTerminator: true,
		StockQuery:   "morning trail hike sunrise friends",
	}
	nudgeID, err := sqlStorage.Insert(ctx, nudge)
	if err != nil {
		t.Fatalf("insert nudge: %v", err)
	}

	svc.fetchNudgeImagery(ctx, nudgeID, nudge.StockQuery)

	stored := &models.StoredNudge{}
	if err := sqlStorage.GetByID(ctx, nudgeID, stored); err != nil {
		t.Fatalf("reload nudge: %v", err)
	}
	if stored.GetMediaId() == "" {
		t.Fatal("no imagery attached to the nudge")
	}

	media := &models.Media{}
	if err := sqlStorage.GetByID(ctx, stored.GetMediaId(), media); err != nil {
		t.Fatalf("load nudge media: %v", err)
	}
	if media.UserId != mediapkg.SystemUserID {
		t.Errorf("nudge imagery owned by %q, want %q — any other viewer would be denied",
			media.UserId, mediapkg.SystemUserID)
	}
}
