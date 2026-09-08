package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// fakeUnsharer records the UnshareFromCommunity hook invocation so UnshareItem's
// delegation and argument plumbing can be asserted without a real item service.
type fakeUnsharer struct {
	called    bool
	gotItemID string
	gotCommID string
	gotActor  string
	err       error
}

func (f *fakeUnsharer) unshare(_ context.Context, itemID, communityID, actorUserID string) error {
	f.called = true
	f.gotItemID = itemID
	f.gotCommID = communityID
	f.gotActor = actorUserID
	return f.err
}

// setupUnshareItemTest wires a service with a fake experience unshare hook and an
// authed host context.
func setupUnshareItemTest(t *testing.T) (*Service, *fakeUnsharer, string, context.Context) {
	t.Helper()
	st := setupTestStorage(t)
	svc := setupTestService(t, st)
	hostID := setupTestUser(t, st, "host@example.com", "Host")
	ctx := createAuthenticatedContext(hostID, "host@example.com", models.Role_ROLE_USER)

	un := &fakeUnsharer{}
	svc.SetItemSharer(ItemKindExperience, ItemSharer{UnshareFromCommunity: un.unshare})
	return svc, un, hostID, ctx
}

func unshareExpTarget(expID string) *api.UnshareItemRequest_ExperienceId {
	return &api.UnshareItemRequest_ExperienceId{ExperienceId: expID}
}

func TestUnshareItem_DelegatesToSharer(t *testing.T) {
	svc, un, hostID, ctx := setupUnshareItemTest(t)

	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        unshareExpTarget("exp-1"),
		CommunityId: "comm-1",
	}))
	if err != nil {
		t.Fatalf("UnshareItem: %v", err)
	}
	if !un.called {
		t.Fatal("expected the unshare hook to run")
	}
	if un.gotItemID != "exp-1" || un.gotCommID != "comm-1" || un.gotActor != hostID {
		t.Errorf("hook got (item=%q, community=%q, actor=%q), want (exp-1, comm-1, %q)",
			un.gotItemID, un.gotCommID, un.gotActor, hostID)
	}
}

func TestUnshareItem_ErrorPropagates(t *testing.T) {
	svc, un, _, ctx := setupUnshareItemTest(t)
	un.err = connect.NewError(connect.CodePermissionDenied, context.DeadlineExceeded)

	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        unshareExpTarget("exp-1"),
		CommunityId: "comm-1",
	}))
	if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied to propagate, got %v", err)
	}
}

func TestUnshareItem_NoTarget(t *testing.T) {
	svc, _, _, ctx := setupUnshareItemTest(t)
	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		CommunityId: "comm-1",
	}))
	if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument for missing target, got %v", err)
	}
}

func TestUnshareItem_MissingCommunityID(t *testing.T) {
	svc, un, _, ctx := setupUnshareItemTest(t)
	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item: unshareExpTarget("exp-1"),
	}))
	if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument for missing community_id, got %v", err)
	}
	if un.called {
		t.Error("hook must not run when community_id is absent")
	}
}

func TestUnshareItem_UnregisteredKind(t *testing.T) {
	svc, _, _, ctx := setupUnshareItemTest(t)
	// Only the experience sharer is registered → gear is unsupported.
	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        &api.UnshareItemRequest_GearId{GearId: "gear-x"},
		CommunityId: "comm-1",
	}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented for an unwired item kind, got %v", err)
	}
}

func TestUnshareItem_KindWithoutUnshareHook(t *testing.T) {
	svc, _, _, ctx := setupUnshareItemTest(t)
	// Register a gear sharer that supports sharing but not unsharing.
	svc.SetItemSharer(ItemKindGear, ItemSharer{
		ShareToCommunity: func(context.Context, string, string, string) error { return nil },
	})

	_, err := svc.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        &api.UnshareItemRequest_GearId{GearId: "gear-x"},
		CommunityId: "comm-1",
	}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented when the kind has no unshare hook, got %v", err)
	}
}
