package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestRemoveRequestNeed_CascadesLinkedContributions exercises the
// cascade soft-delete added so "Cancel the breakdown" actually wipes
// every claim that pointed at the cancelled need — not just the need
// itself.
func TestRemoveRequestNeed_CascadesLinkedContributions(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	// Seed a Request with one Need plus a pre-claim by the requester.
	requestID, needIDs := seedRequestWithNeeds(t, service, requesterID, communityID,
		[]string{"Saw"}, stockImageryDone)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	claimResp, err := service.BatchClaimRequestNeeds(reqCtx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims:    []*api.BatchClaimRequestNeedItem{{NeedId: needIDs[0]}},
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}
	if claimResp.Msg.Results[0].ErrorMessage != nil {
		t.Fatalf("seed claim failed: %s", *claimResp.Msg.Results[0].ErrorMessage)
	}
	contribID := claimResp.Msg.Results[0].Contribution.Id

	// Remove the need — cascade should soft-delete the linked contrib.
	if _, err := service.RemoveRequestNeed(reqCtx, connect.NewRequest(&api.RemoveRequestNeedRequest{
		NeedId:    needIDs[0],
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("RemoveRequestNeed failed: %v", err)
	}

	storedContrib := &models.PlanningContribution{}
	if err := testStorage.GetByID(context.Background(), contribID, storedContrib,
		storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("failed to reload contribution: %v", err)
	}
	if storedContrib.Deleted == nil {
		t.Fatal("expected contribution to be soft-deleted by cascade, but Deleted is nil")
	}

	// And the post-mutation list call returns neither — that's what
	// keeps the client list from showing zombies.
	listResp, err := service.ListRequestNeedsAndContributions(reqCtx,
		connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("ListRequestNeedsAndContributions failed: %v", err)
	}
	// The request's seeded need (#2702) legitimately remains; the removed need
	// must be gone.
	for _, n := range listResp.Msg.Needs {
		if n.Id == needIDs[0] {
			t.Errorf("expected removed need %s to be gone, still listed", needIDs[0])
		}
	}
	if len(listResp.Msg.Contributions) != 0 {
		t.Errorf("expected zero active contributions, got %d", len(listResp.Msg.Contributions))
	}
}

// TestRemoveRequestNeed_CascadeIsIdempotent reuses an already-deleted
// contribution path and asserts the cascade doesn't double-touch.
func TestRemoveRequestNeed_CascadeIsIdempotent(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)
	requestID, needIDs := seedRequestWithNeeds(t, service, requesterID, communityID,
		[]string{"Saw"}, stockImageryDone)

	reqCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	claimResp, err := service.BatchClaimRequestNeeds(reqCtx, connect.NewRequest(&api.BatchClaimRequestNeedsRequest{
		RequestId: requestID,
		Claims:    []*api.BatchClaimRequestNeedItem{{NeedId: needIDs[0]}},
	}))
	if err != nil {
		t.Fatalf("BatchClaimRequestNeeds failed: %v", err)
	}
	contribID := claimResp.Msg.Results[0].Contribution.Id

	// Manually soft-delete the contribution first so the cascade has
	// nothing fresh to do.
	storedContrib := &models.PlanningContribution{}
	if err := testStorage.GetByID(context.Background(), contribID, storedContrib); err != nil {
		t.Fatalf("failed to load contribution: %v", err)
	}
	storedContrib.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
	if err := testStorage.Update(context.Background(), storedContrib); err != nil {
		t.Fatalf("failed to pre-soft-delete contribution: %v", err)
	}
	preDeletedAt := storedContrib.Deleted.DeletedAtUnixSec

	// The cascade should now be a no-op.
	if _, err := service.RemoveRequestNeed(reqCtx, connect.NewRequest(&api.RemoveRequestNeedRequest{
		NeedId:    needIDs[0],
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("RemoveRequestNeed failed: %v", err)
	}

	reloaded := &models.PlanningContribution{}
	if err := testStorage.GetByID(context.Background(), contribID, reloaded,
		storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("failed to reload contribution: %v", err)
	}
	if reloaded.Deleted == nil || reloaded.Deleted.DeletedAtUnixSec != preDeletedAt {
		t.Errorf("expected pre-existing soft-delete timestamp to be preserved; got %+v", reloaded.Deleted)
	}
}
