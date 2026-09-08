package presence

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

func insertUser(t *testing.T, store *storage.ProtoSQLStorage, name string, mediaIDs []string) string {
	t.Helper()
	id, err := store.Insert(context.Background(), &models.User{
		Name:     name,
		Role:     models.Role_ROLE_USER,
		MediaIds: mediaIDs,
	})
	if err != nil {
		t.Fatalf("insert user %s: %v", name, err)
	}
	return id
}

func insertOffer(t *testing.T, store *storage.ProtoSQLStorage, requestID, userID string, withdrawn bool) {
	t.Helper()
	if _, err := store.Insert(context.Background(), &models.RequestOffer{
		RequestId: requestID,
		UserId:    userID,
		Withdrawn: withdrawn,
	}); err != nil {
		t.Fatalf("insert offer (request=%s user=%s): %v", requestID, userID, err)
	}
}

func TestLoadFaces(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	alice := insertUser(t, store, "Alice", []string{"media-a", "media-a2"})
	bob := insertUser(t, store, "Bob", nil)
	carol := insertUser(t, store, "Carol", []string{""}) // empty first media id
	dave := insertUser(t, store, "Dave", []string{"media-d"})

	t.Run("empty input returns nil", func(t *testing.T) {
		faces, err := LoadFaces(ctx, store, nil)
		if err != nil {
			t.Fatalf("LoadFaces: %v", err)
		}
		if faces != nil {
			t.Fatalf("want nil faces, got %v", faces)
		}
	})

	t.Run("preserves input order and maps fields", func(t *testing.T) {
		faces, err := LoadFaces(ctx, store, []string{bob, alice})
		if err != nil {
			t.Fatalf("LoadFaces: %v", err)
		}
		if len(faces) != 2 {
			t.Fatalf("want 2 faces, got %d", len(faces))
		}
		if faces[0].UserId != bob || faces[0].DisplayName != "Bob" {
			t.Errorf("face[0] = %+v, want Bob", faces[0])
		}
		if faces[0].MediaId != nil {
			t.Errorf("Bob has no media, want nil MediaId, got %q", *faces[0].MediaId)
		}
		if faces[1].UserId != alice {
			t.Errorf("face[1].UserId = %q, want alice", faces[1].UserId)
		}
		if faces[1].MediaId == nil || *faces[1].MediaId != "media-a" {
			t.Errorf("Alice MediaId = %v, want media-a (first only)", faces[1].MediaId)
		}
	})

	t.Run("empty first media id yields nil MediaId", func(t *testing.T) {
		faces, err := LoadFaces(ctx, store, []string{carol})
		if err != nil {
			t.Fatalf("LoadFaces: %v", err)
		}
		if len(faces) != 1 || faces[0].MediaId != nil {
			t.Errorf("Carol face = %+v, want single face with nil MediaId", faces)
		}
	})

	t.Run("caps at MaxFaces before loading", func(t *testing.T) {
		// A fifth id that does not exist proves the cap is applied to the
		// input slice before the DB read — it is sliced off, so its
		// absence never matters.
		ids := []string{alice, bob, carol, dave, "missing-user"}
		faces, err := LoadFaces(ctx, store, ids)
		if err != nil {
			t.Fatalf("LoadFaces: %v", err)
		}
		if len(faces) != MaxFaces {
			t.Fatalf("want %d faces (capped), got %d", MaxFaces, len(faces))
		}
	})

	t.Run("skips ids with no matching user", func(t *testing.T) {
		faces, err := LoadFaces(ctx, store, []string{alice, "no-such-id"})
		if err != nil {
			t.Fatalf("LoadFaces: %v", err)
		}
		if len(faces) != 1 || faces[0].UserId != alice {
			t.Errorf("want only Alice, got %+v", faces)
		}
	})
}

func TestAskQualifies(t *testing.T) {
	const now, cutoff = int64(1000), int64(500)
	future := now + 100
	past := now - 100

	int64Ptr := func(v int64) *int64 { return &v }

	tests := []struct {
		name string
		req  *models.Request
		want bool
	}{
		{"nil request", nil, false},
		{
			"deleted request",
			&models.Request{
				State:   models.RequestState_REQUEST_STATE_ACTIVE,
				Deleted: &models.DeletedMetadata{},
			},
			false,
		},
		{
			"fulfilled state",
			&models.Request{State: models.RequestState_REQUEST_STATE_FULFILLED, CreatedAtUnixSec: now},
			false,
		},
		{
			"active with future deadline",
			&models.Request{State: models.RequestState_REQUEST_STATE_ACTIVE, NeededByUnixSec: int64Ptr(future)},
			true,
		},
		{
			"active with past deadline",
			&models.Request{State: models.RequestState_REQUEST_STATE_ACTIVE, NeededByUnixSec: int64Ptr(past)},
			false,
		},
		{
			"deadline takes precedence over freshness",
			&models.Request{
				State:            models.RequestState_REQUEST_STATE_ACTIVE,
				NeededByUnixSec:  int64Ptr(past),
				CreatedAtUnixSec: now, // fresh, but past deadline wins
			},
			false,
		},
		{
			"offers-received, no deadline, fresh",
			&models.Request{State: models.RequestState_REQUEST_STATE_OFFERS_RECEIVED, CreatedAtUnixSec: cutoff},
			true,
		},
		{
			"no deadline, stale",
			&models.Request{State: models.RequestState_REQUEST_STATE_ACTIVE, CreatedAtUnixSec: cutoff - 1},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AskQualifies(tt.req, now, cutoff); got != tt.want {
				t.Errorf("AskQualifies() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTallyOffers(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	const requestID, viewerID = "req-1", "viewer-1"

	t.Run("no offers", func(t *testing.T) {
		tally, err := TallyOffers(ctx, store, "empty-request", viewerID)
		if err != nil {
			t.Fatalf("TallyOffers: %v", err)
		}
		if tally.Committed != 0 || tally.ViewerCommitted || len(tally.OffererIDs) != 0 {
			t.Errorf("want zero tally, got %+v", tally)
		}
	})

	insertOffer(t, store, requestID, viewerID, false) // viewer's own offer
	insertOffer(t, store, requestID, "offerer-a", false)
	insertOffer(t, store, requestID, "offerer-b", false)
	insertOffer(t, store, requestID, "offerer-c", true) // withdrawn: ignored
	insertOffer(t, store, "other-request", "offerer-d", false)

	t.Run("counts active offers, excludes viewer and withdrawn", func(t *testing.T) {
		tally, err := TallyOffers(ctx, store, requestID, viewerID)
		if err != nil {
			t.Fatalf("TallyOffers: %v", err)
		}
		if tally.Committed != 3 {
			t.Errorf("Committed = %d, want 3", tally.Committed)
		}
		if !tally.ViewerCommitted {
			t.Errorf("ViewerCommitted = false, want true")
		}
		want := map[string]bool{"offerer-a": true, "offerer-b": true}
		if len(tally.OffererIDs) != len(want) {
			t.Fatalf("OffererIDs = %v, want the two non-viewer offerers", tally.OffererIDs)
		}
		for _, id := range tally.OffererIDs {
			if !want[id] {
				t.Errorf("unexpected offerer id %q in %v", id, tally.OffererIDs)
			}
		}
	})

	t.Run("viewer not among offerers", func(t *testing.T) {
		tally, err := TallyOffers(ctx, store, requestID, "outsider")
		if err != nil {
			t.Fatalf("TallyOffers: %v", err)
		}
		if tally.ViewerCommitted {
			t.Errorf("ViewerCommitted = true for outsider, want false")
		}
		if tally.Committed != 3 {
			t.Errorf("Committed = %d, want 3", tally.Committed)
		}
	})
}
