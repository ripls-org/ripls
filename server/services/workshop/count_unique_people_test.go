package workshop

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertMember writes a CommunityUser row for (communityID, userID).
func insertMember(t *testing.T, store *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	cu := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}
	if _, err := store.Insert(context.Background(), cu); err != nil {
		t.Fatalf("insert community_user (%s, %s): %v", communityID, userID, err)
	}
}

func TestCountUniquePeople_Empty(t *testing.T) {
	store := setupTestStorage(t)
	s := &Service{storage: store}

	got, err := s.countUniquePeople(context.Background(), nil)
	if err != nil {
		t.Fatalf("countUniquePeople: %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}

	got, err = s.countUniquePeople(context.Background(), []string{})
	if err != nil {
		t.Fatalf("countUniquePeople (empty slice): %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0 for empty community set", got)
	}
}

func TestCountUniquePeople_SingleCommunity(t *testing.T) {
	store := setupTestStorage(t)
	s := &Service{storage: store}

	for _, uid := range []string{"u1", "u2", "u3", "u4", "u5"} {
		insertMember(t, store, "c1", uid)
	}

	got, err := s.countUniquePeople(context.Background(), []string{"c1"})
	if err != nil {
		t.Fatalf("countUniquePeople: %v", err)
	}
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestCountUniquePeople_DedupesAcrossCommunities(t *testing.T) {
	store := setupTestStorage(t)
	s := &Service{storage: store}

	// c1 members: u1, u2, u3
	for _, uid := range []string{"u1", "u2", "u3"} {
		insertMember(t, store, "c1", uid)
	}
	// c2 members: u3, u4, u5 (u3 is shared with c1)
	for _, uid := range []string{"u3", "u4", "u5"} {
		insertMember(t, store, "c2", uid)
	}

	got, err := s.countUniquePeople(context.Background(), []string{"c1", "c2"})
	if err != nil {
		t.Fatalf("countUniquePeople: %v", err)
	}
	if got != 5 {
		t.Errorf("got %d, want 5 (u3 must be counted once across both communities)", got)
	}
}

func TestCountUniquePeople_IgnoresOutOfScopeCommunities(t *testing.T) {
	store := setupTestStorage(t)
	s := &Service{storage: store}

	insertMember(t, store, "c1", "u1")
	insertMember(t, store, "c1", "u2")
	insertMember(t, store, "out-of-scope", "u3")
	insertMember(t, store, "out-of-scope", "u4")

	got, err := s.countUniquePeople(context.Background(), []string{"c1"})
	if err != nil {
		t.Fatalf("countUniquePeople: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (out-of-scope members must be excluded)", got)
	}
}

// TestCountUniquePeople_NoN1Queries pins the helper to a single batched
// SQL round-trip regardless of community count. A regression that
// reintroduces per-community fetching will trip this immediately.
func TestCountUniquePeople_NoN1Queries(t *testing.T) {
	store := setupTestStorage(t)
	s := &Service{storage: store}

	communityIDs := []string{"c1", "c2", "c3", "c4", "c5"}
	for _, cid := range communityIDs {
		for i := 0; i < 4; i++ {
			insertMember(t, store, cid, cid+"-u"+string(rune('a'+i)))
		}
	}

	statsCtx := storage.WithQueryStats(context.Background())
	storage.AssertMaxQueries(t, statsCtx, 1, func() {
		got, err := s.countUniquePeople(statsCtx, communityIDs)
		if err != nil {
			t.Fatalf("countUniquePeople: %v", err)
		}
		// 5 communities * 4 members each, no overlap → 20.
		if got != 20 {
			t.Errorf("got %d, want 20", got)
		}
	})
}
