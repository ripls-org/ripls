package integration_tests

// Over-the-wire audience helpers for integration tests.
//
// Every item type shares and unshares through the same two RPCs —
// CommunityService.ShareItem and UnshareItem (#2526) — so these wrap the oneof
// plumbing once instead of at every call site. They live apart from
// test_helpers.go, which is at the file-size limit.

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// shareGearIntoCommunity sets the gear's Lend/Give mode and then shares it into
// a community over the wire through CommunityService.ShareItem — the unified
// add-path for every item type. Pass AVAILABILITY_UNSPECIFIED to leave the
// existing mode (FOR_LOAN by default) alone.
//
// Availability is item-wide (#2492/#2687), not per-community, so it is its own
// call rather than an argument on the share — and it must come FIRST. ShareItem
// inherits whatever the gear already carries, and the GEAR_SHARED community
// event it emits picks its copy ("New loan" vs "New giveaway") from that
// inherited mode. Setting availability afterwards flips the junction but leaves
// the already-sent notification saying the wrong thing.
//
// In the app this ordering is automatic: SaveGear carries the creation-time
// choice into the per-item community's first share (#2687).
func shareGearIntoCommunity(
	t *testing.T,
	ctx context.Context,
	communityClient apiconnect.CommunityServiceClient,
	gearID, communityID string,
	availability api.Availability,
) {
	t.Helper()

	if availability != api.Availability_AVAILABILITY_UNSPECIFIED {
		if _, err := communityClient.SetGearAvailability(ctx, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId:       gearID,
			Availability: availability,
		})); err != nil {
			t.Fatalf("SetGearAvailability(gear %s -> %v): %v", gearID, availability, err)
		}
	}

	if _, err := communityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_GearId{GearId: gearID},
		ShareToCommunityIds: []string{communityID},
	})); err != nil {
		t.Fatalf("ShareItem(gear %s -> community %s): %v", gearID, communityID, err)
	}
}

// shareExperienceIntoCommunity shares an experience into a community over the
// wire through CommunityService.ShareItem.
func shareExperienceIntoCommunity(
	t *testing.T,
	ctx context.Context,
	communityClient apiconnect.CommunityServiceClient,
	experienceID, communityID string,
) {
	t.Helper()

	if _, err := communityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_ExperienceId{ExperienceId: experienceID},
		ShareToCommunityIds: []string{communityID},
	})); err != nil {
		t.Fatalf("ShareItem(experience %s -> community %s): %v", experienceID, communityID, err)
	}
}

// shareRequestIntoCommunity shares a request into a community over the wire
// through CommunityService.ShareItem.
func shareRequestIntoCommunity(
	t *testing.T,
	ctx context.Context,
	communityClient apiconnect.CommunityServiceClient,
	requestID, communityID string,
) {
	t.Helper()

	if _, err := communityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_RequestId{RequestId: requestID},
		ShareToCommunityIds: []string{communityID},
	})); err != nil {
		t.Fatalf("ShareItem(request %s -> community %s): %v", requestID, communityID, err)
	}
}

// unshareExperienceFromCommunity removes an experience from a community over the
// wire through CommunityService.UnshareItem, returning the error so callers can
// assert both the success and the rejection paths.
func unshareExperienceFromCommunity(
	ctx context.Context,
	communityClient apiconnect.CommunityServiceClient,
	experienceID, communityID string,
) error {
	_, err := communityClient.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        &api.UnshareItemRequest_ExperienceId{ExperienceId: experienceID},
		CommunityId: communityID,
	}))
	return err
}

// unshareRequestFromCommunity removes a request from a community over the wire
// through CommunityService.UnshareItem, returning the error so callers can
// assert both the success and the rejection paths (notably the last-community
// guard, which is request-specific).
func unshareRequestFromCommunity(
	ctx context.Context,
	communityClient apiconnect.CommunityServiceClient,
	requestID, communityID string,
) error {
	_, err := communityClient.UnshareItem(ctx, connect.NewRequest(&api.UnshareItemRequest{
		Item:        &api.UnshareItemRequest_RequestId{RequestId: requestID},
		CommunityId: communityID,
	}))
	return err
}
