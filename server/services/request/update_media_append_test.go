package request

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestService_UpdateRequest_NonRequesterAppendsMedia covers the carve-out
// added for the media-carousel "Add media" flow: a community member who
// isn't the requester can append media IDs to a request, but cannot use
// UpdateRequest to change any other field or remove/reorder existing media.
func TestService_UpdateRequest_NonRequesterAppendsMedia(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	memberID := setupTestUser(t, testStorage, "Member", "member@example.com")
	strangerID := setupTestUser(t, testStorage, "Stranger", "stranger@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, memberID)

	// Seed: requester submits a request with one media item.
	media := &models.Media{
		UserId:      requesterID,
		ContentType: "image/jpeg",
		Filename:    nil,
		SizeBytes:   1024,
		StorageUrl:  "https://example.com/existing.jpg",
	}
	existingMediaID, err := testStorage.Insert(context.Background(), media)
	if err != nil {
		t.Fatalf("seed existing media: %v", err)
	}

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Power Drill",
		Description: "Looking for a power drill",
		MediaIds:    []string{existingMediaID},
	}))
	if err != nil {
		t.Fatalf("seed submit request: %v", err)
	}
	// SubmitRequest only triggers the async stock-imagery fetch when no
	// MediaIds were supplied; we seeded with one, so there's nothing to wait
	// on.
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)

	memberMedia := &models.Media{
		UserId:      memberID,
		ContentType: "image/jpeg",
		SizeBytes:   2048,
		StorageUrl:  "https://example.com/member.jpg",
	}
	memberMediaID, err := testStorage.Insert(context.Background(), memberMedia)
	if err != nil {
		t.Fatalf("seed member media: %v", err)
	}

	memberCtx := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)
	strangerCtx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)

	t.Run("community member can append media", func(t *testing.T) {
		// Re-fetch the request to get the canonical title/description (the server
		// may rewrite these during SubmitRequest's AI enrichment).
		stored := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
			t.Fatalf("load request: %v", err)
		}
		_, err := service.UpdateRequest(memberCtx, connect.NewRequest(&api.UpdateRequestRequest{
			RequestId:   requestID,
			Title:       stored.Title,
			Description: stored.Description,
			MediaIds:    append(append([]string{}, stored.MediaIds...), memberMediaID),
		}))
		if err != nil {
			t.Fatalf("expected non-requester append to succeed, got: %v", err)
		}

		after := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, after); err != nil {
			t.Fatalf("load request after append: %v", err)
		}
		if got, last := len(after.MediaIds), after.MediaIds[len(after.MediaIds)-1]; last != memberMediaID || got != len(stored.MediaIds)+1 {
			t.Fatalf("media ids after append: got %v (len=%d), expected stored+%q", after.MediaIds, got, memberMediaID)
		}
	})

	t.Run("community member cannot change description", func(t *testing.T) {
		stored := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
			t.Fatalf("load request: %v", err)
		}
		_, err := service.UpdateRequest(memberCtx, connect.NewRequest(&api.UpdateRequestRequest{
			RequestId:   requestID,
			Title:       stored.Title,
			Description: "Member-altered description",
			MediaIds:    append(append([]string{}, stored.MediaIds...), "another-add"),
		}))
		if !requestIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for description change, got: %v", err)
		}
	})

	t.Run("community member cannot remove existing media", func(t *testing.T) {
		stored := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
			t.Fatalf("load request: %v", err)
		}
		_, err := service.UpdateRequest(memberCtx, connect.NewRequest(&api.UpdateRequestRequest{
			RequestId:   requestID,
			Title:       stored.Title,
			Description: stored.Description,
			MediaIds:    []string{memberMediaID}, // drops existing
		}))
		if !requestIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for removal, got: %v", err)
		}
	})

	t.Run("non-member cannot append media", func(t *testing.T) {
		stored := &models.Request{}
		if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
			t.Fatalf("load request: %v", err)
		}
		_, err := service.UpdateRequest(strangerCtx, connect.NewRequest(&api.UpdateRequestRequest{
			RequestId:   requestID,
			Title:       stored.Title,
			Description: stored.Description,
			MediaIds:    append(append([]string{}, stored.MediaIds...), "stranger-add"),
		}))
		if !requestIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for non-member, got: %v", err)
		}
	})
}

func requestIsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return false
	}
	return ce.Code() == connect.CodePermissionDenied
}
