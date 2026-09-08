package community

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage wraps storage.SetupTestStorage with t.Cleanup.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return s
}

// insertTestUser inserts a minimal User record and returns its ID.
func insertTestUser(t *testing.T, s *storage.ProtoSQLStorage, email, name string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.User{
		Email: email,
		Name:  name,
		Role:  models.Role_ROLE_USER,
	})
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
	return id
}

// insertTestCommunity inserts a minimal Community record and returns its ID.
func insertTestCommunity(t *testing.T, s *storage.ProtoSQLStorage, name, creatorID string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.Community{
		Name:        name,
		CreatorId:   creatorID,
		OwnerUserId: creatorID,
	})
	if err != nil {
		t.Fatalf("insertTestCommunity: %v", err)
	}
	return id
}

// insertMembership inserts a CommunityUser record.
func insertMembership(t *testing.T, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}); err != nil {
		t.Fatalf("insertMembership: %v", err)
	}
}

func TestGetNumCommunityMembers(t *testing.T) {
	s := setupTestStorage(t)

	creatorID := insertTestUser(t, s, "creator@example.com", "Creator")
	communityID := insertTestCommunity(t, s, "Test Community", creatorID)
	insertMembership(t, s, communityID, creatorID)

	t.Run("one member after creation", func(t *testing.T) {
		count, err := GetNumCommunityMembers(context.Background(), s, communityID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("want 1 member, got %d", count)
		}
	})

	t.Run("count increases when member added", func(t *testing.T) {
		memberID := insertTestUser(t, s, "member@example.com", "Member")
		insertMembership(t, s, communityID, memberID)

		count, err := GetNumCommunityMembers(context.Background(), s, communityID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("want 2 members, got %d", count)
		}
	})

	t.Run("non-existent community returns error", func(t *testing.T) {
		_, err := GetNumCommunityMembers(context.Background(), s, "does-not-exist")
		if err == nil {
			t.Fatal("expected error for non-existent community")
		}
		if !strings.Contains(err.Error(), "community not found") {
			t.Errorf("want 'community not found' in error, got: %v", err)
		}
	})
}
