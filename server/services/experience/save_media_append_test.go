package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// seedExperienceForMediaAppend creates an experience owned by `ownerID`,
// shared into a single community with `memberID` as a member, with one
// pre-existing media ID. Returns the experience id and the pre-existing
// media id.
func seedExperienceForMediaAppend(
	t *testing.T,
	svc *Service,
	stor *storage.ProtoSQLStorage,
	ownerID, memberID string,
) (string, string) {
	t.Helper()
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)
	createResp, err := svc.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Owner Event",
		Description: "Owner-authored description",
		MediaIds:    []string{"existing-media-1"},
	}))
	if err != nil {
		t.Fatalf("seed: create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share into a community and add memberID as a member.
	communityID := createTestCommunity(t, stor, "Shared Community", ownerID)
	if _, err := stor.Insert(context.Background(), &models.CommunityExperience{
		CommunityId:  communityID,
		ExperienceId: expID,
	}); err != nil {
		t.Fatalf("seed: link community to experience: %v", err)
	}
	createTestCommunityMembership(t, stor, communityID, memberID)
	return expID, "existing-media-1"
}

// TestSaveExperience_NonOwnerAppendsMedia covers the carve-out introduced for
// the media-carousel "Add media" flow: a community member who isn't the owner
// can append media IDs to an experience, but cannot use SaveExperience to
// change any other field or remove/reorder existing media.
func TestSaveExperience_NonOwnerAppendsMedia(t *testing.T) {
	svc, stor, _ := setupTestServiceWithMockBus(t)

	const ownerID = "owner-1"
	const memberID = "member-1"
	const strangerID = "stranger-1"
	createTestUser(t, stor, ownerID, "owner@example.com", "Owner")
	createTestUser(t, stor, memberID, "member@example.com", "Member")
	createTestUser(t, stor, strangerID, "stranger@example.com", "Stranger")

	expID, existingMediaID := seedExperienceForMediaAppend(t, svc, stor, ownerID, memberID)
	memberCtx := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)
	strangerCtx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)

	t.Run("community member can append media", func(t *testing.T) {
		_, err := svc.SaveExperience(memberCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:          &expID,
			Name:        "Owner Event",
			Description: "Owner-authored description",
			MediaIds:    []string{existingMediaID, "new-media-from-member"},
		}))
		if err != nil {
			t.Fatalf("expected non-owner append to succeed, got: %v", err)
		}

		// Verify the append actually persisted.
		stored := &models.Experience{}
		if err := stor.GetByID(context.Background(), expID, stored); err != nil {
			t.Fatalf("load experience after append: %v", err)
		}
		if got, want := stored.MediaIds, []string{existingMediaID, "new-media-from-member"}; !equalStrings(got, want) {
			t.Fatalf("media ids after append: got %v, want %v", got, want)
		}
	})

	t.Run("community member cannot remove existing media", func(t *testing.T) {
		_, err := svc.SaveExperience(memberCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:       &expID,
			Name:     "Owner Event",
			MediaIds: []string{"only-new"}, // drops existing
		}))
		if !isPermissionDenied(err) {
			t.Fatalf("expected permission_denied for removal, got: %v", err)
		}
	})

	t.Run("community member cannot reorder existing media", func(t *testing.T) {
		// existing list after first subtest: [existingMediaID, "new-media-from-member"].
		_, err := svc.SaveExperience(memberCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:       &expID,
			Name:     "Owner Event",
			MediaIds: []string{"new-media-from-member", existingMediaID}, // swapped
		}))
		if !isPermissionDenied(err) {
			t.Fatalf("expected permission_denied for reorder, got: %v", err)
		}
	})

	t.Run("community member cannot change description", func(t *testing.T) {
		_, err := svc.SaveExperience(memberCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:          &expID,
			Name:        "Owner Event",
			Description: "Member-altered description",
			MediaIds:    []string{existingMediaID, "new-media-from-member", "another-add"},
		}))
		if !isPermissionDenied(err) {
			t.Fatalf("expected permission_denied for description change, got: %v", err)
		}
	})

	t.Run("non-member cannot append media", func(t *testing.T) {
		_, err := svc.SaveExperience(strangerCtx, connect.NewRequest(&api.SaveExperienceRequest{
			Id:          &expID,
			Name:        "Owner Event",
			Description: "Owner-authored description",
			MediaIds:    []string{existingMediaID, "new-media-from-member", "stranger-add"},
		}))
		if !isPermissionDenied(err) {
			t.Fatalf("expected permission_denied for non-member, got: %v", err)
		}
	})
}

func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	connectErr := new(connect.Error)
	if !errorsAs(err, &connectErr) {
		return false
	}
	return connectErr.Code() == connect.CodePermissionDenied
}

// errorsAs is a thin wrapper around errors.As so the test file doesn't need
// to import the "errors" package on its own; keeps the helper local.
func errorsAs(err error, target **connect.Error) bool {
	for err != nil {
		if ce, ok := err.(*connect.Error); ok {
			*target = ce
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
			continue
		}
		return false
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
