package community_subscriber

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage wraps storage.SetupTestStorage with t.Cleanup. Mirrors
// the helper of the same name in server/community/membership_test.go;
// duplicated here because Go test files don't expose names cross-package.
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

// insertMembership inserts a CommunityUser record so a user is treated as a
// member of the community for downstream lookups.
func insertMembership(t *testing.T, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}); err != nil {
		t.Fatalf("insertMembership: %v", err)
	}
}
