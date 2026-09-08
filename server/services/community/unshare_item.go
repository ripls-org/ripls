package community

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// UnshareItem removes an item from one community's audience — the uniform
// remove-path counterpart to ShareItem across all item types. It is owner-only
// and delegates to the per-item-type sharer registered via SetItemSharer, which
// preserves that type's removal semantics: a request cannot leave its last
// community; gear and experiences hard-delete their junction while a request is
// soft-archived; gear emits a GEAR_UNSHARED event. The owner check (and its
// per-type "not the owner" message) lives inside each sharer's
// UnshareFromCommunity hook, so the copy matches the legacy per-type RPCs.
func (s *Service) UnshareItem(
	ctx context.Context,
	req *connect.Request[api.UnshareItemRequest],
) (*connect.Response[api.UnshareItemResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	kind, itemID, err := parseUnshareItemTarget(req.Msg)
	if err != nil {
		return nil, err
	}

	communityID := req.Msg.GetCommunityId()
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id must be set"))
	}

	sharer, ok := s.itemSharers[kind]
	if !ok || sharer.UnshareFromCommunity == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("unsharing %s items is not supported yet", kind))
	}

	if err := sharer.UnshareFromCommunity(ctx, itemID, communityID, authInfo.UserID); err != nil {
		return nil, err
	}

	logging.LoggerWithContext(ctx).InfoContext(ctx, "unshared item",
		"operation", "UnshareItem",
		"user_id", authInfo.UserID,
		"item_kind", string(kind),
		"item_id", itemID,
		"community_id", communityID,
	)
	return connect.NewResponse(&api.UnshareItemResponse{}), nil
}

// parseUnshareItemTarget validates the request's item oneof and returns the item
// kind and id.
func parseUnshareItemTarget(req *api.UnshareItemRequest) (ItemKind, string, error) {
	switch {
	case req.GetExperienceId() != "":
		return ItemKindExperience, req.GetExperienceId(), nil
	case req.GetGearId() != "":
		return ItemKindGear, req.GetGearId(), nil
	case req.GetRequestId() != "":
		return ItemKindRequest, req.GetRequestId(), nil
	case req.GetTransferId() != "":
		return ItemKindTransfer, req.GetTransferId(), nil
	default:
		return "", "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item target must be set"))
	}
}
