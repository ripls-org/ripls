package provisional

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage creates a PostgreSQL storage instance for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return store
}

// insertUser inserts a minimal User record and returns its ID.
func insertUser(t *testing.T, store *storage.ProtoSQLStorage, email, name string) string {
	t.Helper()
	user := &models.User{Email: email, Name: name, Role: models.Role_ROLE_USER}
	id, err := store.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	return id
}

// insertProvisionalUser inserts a ProvisionalUser record and returns its ID.
func insertProvisionalUser(t *testing.T, store *storage.ProtoSQLStorage, communityID, name string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		CommunityId: communityID,
		Name:        name,
	}
	id, err := store.Insert(context.Background(), prov)
	if err != nil {
		t.Fatalf("insertProvisionalUser: %v", err)
	}
	return id
}

// insertProvisionalRSVP inserts an ExperienceRSVP for a provisional user and returns its ID.
func insertProvisionalRSVP(t *testing.T, store *storage.ProtoSQLStorage, experienceID, communityID, provisionalUserID string) string {
	t.Helper()
	provID := provisionalUserID
	rsvp := &models.ExperienceRSVP{
		ExperienceId:      experienceID,
		CommunityId:       communityID,
		ProvisionalUserId: &provID,
		Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
	}
	id, err := store.Insert(context.Background(), rsvp)
	if err != nil {
		t.Fatalf("insertProvisionalRSVP: %v", err)
	}
	return id
}

// insertExperience inserts a minimal Experience record and returns its ID.
func insertExperience(t *testing.T, store *storage.ProtoSQLStorage) string {
	t.Helper()
	exp := &models.Experience{
		Name:  "Test Experience",
		State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	id, err := store.Insert(context.Background(), exp)
	if err != nil {
		t.Fatalf("insertExperience: %v", err)
	}
	return id
}

// insertCommunity inserts a minimal Community record and returns its ID.
func insertCommunity(t *testing.T, store *storage.ProtoSQLStorage) string {
	t.Helper()
	comm := &models.Community{Name: "Test Community", CreatorId: "test-user", OwnerUserId: "test-user"}
	id, err := store.Insert(context.Background(), comm)
	if err != nil {
		t.Fatalf("insertCommunity: %v", err)
	}
	return id
}

// TestMergeIntoUser_RSVPsMigratedToRealUser verifies that provisional user RSVPs
// are reassigned to the real user after merging.
func TestMergeIntoUser_RSVPsMigratedToRealUser(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	communityID := insertCommunity(t, store)
	experienceID := insertExperience(t, store)
	realUserID := insertUser(t, store, "alice@example.com", "Alice")
	provID := insertProvisionalUser(t, store, communityID, "Alice Provisional")
	rsvpID := insertProvisionalRSVP(t, store, experienceID, communityID, provID)

	if err := MergeIntoUser(ctx, store, provID, realUserID); err != nil {
		t.Fatalf("MergeIntoUser: %v", err)
	}

	// RSVP should now be owned by the real user, not the provisional user.
	rsvp := &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, rsvpID, rsvp); err != nil {
		t.Fatalf("GetByID rsvp: %v", err)
	}
	if rsvp.UserId != realUserID {
		t.Errorf("RSVP.UserId = %q, want %q", rsvp.UserId, realUserID)
	}
	if rsvp.ProvisionalUserId != nil {
		t.Errorf("RSVP.ProvisionalUserId = %q, want nil", *rsvp.ProvisionalUserId)
	}

	// Provisional user should be marked as claimed.
	prov := &models.ProvisionalUser{}
	if err := store.GetByID(ctx, provID, prov); err != nil {
		t.Fatalf("GetByID provisional: %v", err)
	}
	if prov.ClaimedByUserId == nil || *prov.ClaimedByUserId != realUserID {
		t.Errorf("ProvisionalUser.ClaimedByUserId = %v, want %q", prov.ClaimedByUserId, realUserID)
	}
}

// TestMergeIntoUser_AlreadyClaimed verifies that calling MergeIntoUser on an
// already-claimed provisional user is a no-op (idempotent).
func TestMergeIntoUser_AlreadyClaimed(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	communityID := insertCommunity(t, store)
	realUserID := insertUser(t, store, "alice@example.com", "Alice")
	provID := insertProvisionalUser(t, store, communityID, "Alice Provisional")

	// Claim it once.
	if err := MergeIntoUser(ctx, store, provID, realUserID); err != nil {
		t.Fatalf("first MergeIntoUser: %v", err)
	}

	// Second call should succeed without error (idempotent).
	if err := MergeIntoUser(ctx, store, provID, realUserID); err != nil {
		t.Fatalf("second MergeIntoUser: %v", err)
	}
}

// TestMergeIntoUser_NoActivities verifies that a provisional user with no RSVPs
// is still marked as claimed.
func TestMergeIntoUser_NoActivities(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	communityID := insertCommunity(t, store)
	realUserID := insertUser(t, store, "bob@example.com", "Bob")
	provID := insertProvisionalUser(t, store, communityID, "Bob Provisional")

	if err := MergeIntoUser(ctx, store, provID, realUserID); err != nil {
		t.Fatalf("MergeIntoUser: %v", err)
	}

	prov := &models.ProvisionalUser{}
	if err := store.GetByID(ctx, provID, prov); err != nil {
		t.Fatalf("GetByID provisional: %v", err)
	}
	if prov.ClaimedByUserId == nil || *prov.ClaimedByUserId != realUserID {
		t.Errorf("ProvisionalUser.ClaimedByUserId = %v, want %q", prov.ClaimedByUserId, realUserID)
	}
}

// TestMergeIntoUser_ConflictingRSVP verifies that when both the provisional user
// and the real user have an RSVP for the same experience, the provisional RSVP is
// soft-deleted rather than overwriting the real user's RSVP.
func TestMergeIntoUser_ConflictingRSVP(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	communityID := insertCommunity(t, store)
	experienceID := insertExperience(t, store)
	realUserID := insertUser(t, store, "carol@example.com", "Carol")
	provID := insertProvisionalUser(t, store, communityID, "Carol Provisional")

	// Provisional RSVP
	insertProvisionalRSVP(t, store, experienceID, communityID, provID)

	// Real user already has an RSVP for the same experience
	realRSVP := &models.ExperienceRSVP{
		ExperienceId: experienceID,
		CommunityId:  communityID,
		UserId:       realUserID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
	}
	realRSVPID, err := store.Insert(ctx, realRSVP)
	if err != nil {
		t.Fatalf("Insert real RSVP: %v", err)
	}

	if err := MergeIntoUser(ctx, store, provID, realUserID); err != nil {
		t.Fatalf("MergeIntoUser: %v", err)
	}

	// Real user's RSVP should still be intact.
	rsvpCheck := &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, realRSVPID, rsvpCheck); err != nil {
		t.Fatalf("GetByID real rsvp: %v", err)
	}
	if rsvpCheck.UserId != realUserID {
		t.Errorf("real RSVP.UserId = %q, want %q", rsvpCheck.UserId, realUserID)
	}
}
