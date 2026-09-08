package profile

import (
	"context"
	"fmt"
	"math"
	"sort"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/available_now"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/known_for"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// gramsToPoundsRatio converts grams to pounds for the CO₂ ticker
// field; the wire field name is co2_avoided_pounds and the client
// converts to kilograms for display.
const gramsToPoundsRatio = 0.00220462

// GetUserProfileForViewer returns the target user's profile rendered
// through the calling viewer's lens. The hero / ticker numbers carry
// the target's full aggregate activity. Identity-level fields name
// only the communities the viewer and target share; non-shared
// communities collapse to a single count and never appear by name or
// id in the response or in log fields.
func (s *Service) GetUserProfileForViewer(
	ctx context.Context,
	req *connect.Request[api.GetUserProfileForViewerRequest],
) (*connect.Response[api.GetUserProfileForViewerResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.TargetUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("target_user_id is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserProfileForViewer",
		"target_user_id", req.Msg.TargetUserId,
	)

	target, err := s.calculator.GetUser(ctx, req.Msg.TargetUserId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.GetUser", err,
			"target_user_id", req.Msg.TargetUserId)
	}

	sharedRefs, otherCount, earliestJoined, targetActiveCommunityIDs, err := s.resolveSharedCommunities(
		ctx, authInfo.UserID, req.Msg.TargetUserId,
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.resolveSharedCommunities", err,
			"target_user_id", req.Msg.TargetUserId)
	}

	selfView := req.Msg.TargetUserId == authInfo.UserID
	logger = logger.With(
		"shared_community_count", len(sharedRefs),
		"other_community_count", otherCount,
		"self_view", selfView,
	)

	payload, err := s.buildPayload(ctx, req.Msg.TargetUserId, earliestJoined)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.buildPayload", err,
			"target_user_id", req.Msg.TargetUserId)
	}

	// Postcards point the viewer at the target's gear / experiences /
	// help — meaningless when viewer == target. The implicit gate on
	// len(sharedRefs) > 0 collapses out on self-view (you don't share
	// communities with yourself), so the explicit self-view check is
	// retained as belt-and-suspenders only.
	var postcards []*api.BriefCTARow
	if !selfView {
		sharedCommunityIDs := make([]string, 0, len(sharedRefs))
		for _, r := range sharedRefs {
			sharedCommunityIDs = append(sharedCommunityIDs, r.Id)
		}
		postcards, err = s.generatePostcards(
			ctx, req.Msg.TargetUserId, firstName(target.Name), sharedCommunityIDs,
		)
		if err != nil {
			return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.generatePostcards", err,
				"target_user_id", req.Msg.TargetUserId)
		}
	}
	payload.CtaRows = postcards
	logger = logger.With("postcard_count", len(postcards))

	// Available Now rail — gear / open requests / upcoming experiences
	// the target offers, sourced from the shared `available_now`
	// library so the workshop surface reads from the same vocabulary.
	// Known For chips — capability tags derived from the target's
	// completed gear loans, sourced from the shared `known_for`
	// library. Other-view sources both from the viewer-target
	// community intersection; self-view sources from the target's
	// full active community set so the user sees their own offerings
	// on their own profile (#1996).
	scopeIDs := make([]string, 0, len(sharedRefs))
	if selfView {
		scopeIDs = append(scopeIDs, targetActiveCommunityIDs...)
	} else {
		for _, r := range sharedRefs {
			scopeIDs = append(scopeIDs, r.Id)
		}
	}
	availableNow, err := available_now.Gather(ctx, s.storage, available_now.Options{
		Mode:         available_now.ModePerUser,
		TargetUserID: req.Msg.TargetUserId,
		CommunityIDs: scopeIDs,
	})
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.availableNow.Gather", err,
			"target_user_id", req.Msg.TargetUserId)
	}
	knownFor, err := known_for.Derive(ctx, s.storage, known_for.Options{
		Mode:         known_for.ModePerUser,
		OwnerID:      req.Msg.TargetUserId,
		CommunityIDs: scopeIDs,
	})
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetUserProfileForViewer.knownFor.Derive", err,
			"target_user_id", req.Msg.TargetUserId)
	}
	logger = logger.With(
		"available_now_count", len(availableNow),
		"known_for_count", len(knownFor),
	)

	resp := &api.GetUserProfileForViewerResponse{
		TargetUserId:        req.Msg.TargetUserId,
		TargetName:          target.Name,
		SharedCommunities:   sharedRefs,
		OtherCommunityCount: otherCount,
		Payload:             payload,
		AvailableNowItems:   availableNow,
		KnownFor:            knownFor,
	}

	if target.Description != "" {
		d := target.Description
		resp.TargetDescription = &d
	}
	if len(target.MediaIds) > 0 && target.MediaIds[0] != "" {
		m := target.MediaIds[0]
		resp.TargetMediaId = &m
	}

	logger.InfoContext(ctx, "profile assembled")
	return connect.NewResponse(resp), nil
}

// resolveSharedCommunities loads the viewer's and target's active
// community memberships, intersects them, looks up the names of the
// shared communities for the inline subtitle, and returns the count
// of the target's communities the viewer is not in. earliestJoined
// is the earliest CreatedAtUnixSec across the target's active
// memberships — drives the hero "since" anchor.
// targetActiveCommunityIDs carries the full list of the target's
// active (non-deleted) community IDs — used by self-view to source
// the Available Now rail and Known For chips from the target's own
// membership set instead of the (empty) viewer-target intersection.
func (s *Service) resolveSharedCommunities(
	ctx context.Context,
	viewerID, targetID string,
) ([]*api.SharedCommunityRef, int32, int64, []string, error) {
	viewerMemberships, err := storage.QueryByField[*models.CommunityUser](
		s.storage, ctx, "user_id", viewerID,
	)
	if err != nil {
		return nil, 0, 0, nil, fmt.Errorf("query viewer memberships: %w", err)
	}
	targetMemberships, err := storage.QueryByField[*models.CommunityUser](
		s.storage, ctx, "user_id", targetID,
	)
	if err != nil {
		return nil, 0, 0, nil, fmt.Errorf("query target memberships: %w", err)
	}

	viewerActive := make(map[string]struct{}, len(viewerMemberships))
	for _, m := range viewerMemberships {
		if m.Deleted != nil {
			continue
		}
		viewerActive[m.CommunityId] = struct{}{}
	}

	type targetActive struct {
		communityID   string
		joinedUnixSec int64
	}
	targetCommunities := make([]targetActive, 0, len(targetMemberships))
	var earliestJoined int64
	for _, m := range targetMemberships {
		if m.Deleted != nil {
			continue
		}
		targetCommunities = append(targetCommunities, targetActive{
			communityID:   m.CommunityId,
			joinedUnixSec: m.CreatedAtUnixSec,
		})
		if earliestJoined == 0 || m.CreatedAtUnixSec < earliestJoined {
			earliestJoined = m.CreatedAtUnixSec
		}
	}

	// Sort target communities most-recently-joined first so the shared
	// subset renders in the same order on the client.
	sort.Slice(targetCommunities, func(i, j int) bool {
		return targetCommunities[i].joinedUnixSec > targetCommunities[j].joinedUnixSec
	})

	sharedIDs := make([]string, 0, len(targetCommunities))
	targetActiveIDs := make([]string, 0, len(targetCommunities))
	var otherCount int32
	for _, tc := range targetCommunities {
		targetActiveIDs = append(targetActiveIDs, tc.communityID)
		if _, ok := viewerActive[tc.communityID]; ok {
			sharedIDs = append(sharedIDs, tc.communityID)
		} else {
			otherCount++
		}
	}

	if len(sharedIDs) == 0 {
		return nil, otherCount, earliestJoined, targetActiveIDs, nil
	}

	communities, err := storage.GetByIDs[*models.Community](s.storage, ctx, sharedIDs)
	if err != nil {
		return nil, 0, 0, nil, fmt.Errorf("load shared community names: %w", err)
	}

	refs := make([]*api.SharedCommunityRef, 0, len(sharedIDs))
	for _, id := range sharedIDs {
		c, ok := communities[id]
		if !ok || c == nil {
			continue
		}
		// Skip soft-deleted communities — they should not be named in the
		// subtitle even when both users were once members.
		if c.Deleted != nil {
			continue
		}
		ref := &api.SharedCommunityRef{
			Id:   c.Id,
			Name: c.Name,
		}
		if len(c.MediaIds) > 0 && c.MediaIds[0] != "" {
			primary := c.MediaIds[0]
			ref.MediaId = &primary
		}
		refs = append(refs, ref)
	}
	return refs, otherCount, earliestJoined, targetActiveIDs, nil
}

// buildPayload computes the hero + ticker numbers for the target user
// across their full activity. Postcards (cta_rows) ship in a later
// phase; the field stays empty here.
func (s *Service) buildPayload(
	ctx context.Context,
	targetID string,
	earliestJoinedUnixSec int64,
) (*api.BriefPayload, error) {
	payload := &api.BriefPayload{}

	if earliestJoinedUnixSec > 0 {
		since := earliestJoinedUnixSec
		payload.SinceUnixSec = &since
	}

	acts, err := s.calculator.CalculateUserActivityCounts(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("activity counts: %w", err)
	}
	savings, err := s.calculator.CalculateUserImpactSavings(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("impact savings: %w", err)
	}

	actsCount := acts.TotalLoans + acts.TotalBorrows + acts.CompletedGiveaways +
		acts.FulfilledRequests + acts.EventsCreated + acts.ItemsShared
	payload.ActsCount = &actsCount

	// "Plans" ticker for the profile's stat rows. EventsCreated is the
	// closest existing figure — events the target hosted, not the
	// broader "events attended too" a true plans-together count would
	// want; swap to a dedicated count if one is added.
	eventsCreated := acts.EventsCreated
	payload.EventsCount = &eventsCreated

	if savings.CostSavings != nil {
		usd := int32(math.Round(float64(savings.CostSavings.Mean)))
		payload.ReplacedCostUsd = &usd
	}
	if savings.CarbonSavings != nil {
		pounds := int32(math.Round(float64(savings.CarbonSavings.Mean) * gramsToPoundsRatio))
		payload.Co2AvoidedPounds = &pounds
	}
	if savings.TimeBanked != nil {
		hours := int32(math.Round(float64(savings.TimeBanked.Mean) / 60))
		payload.HoursTogether = &hours
	}

	return payload, nil
}
