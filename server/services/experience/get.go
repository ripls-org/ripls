package experience

import (
	"context"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/weather"
	"go.ripls.org/ripls/server/weatherapi"
)

// maxInvitedNoReply caps GetExperienceResponse.InvitedNoReply so a large
// community does not produce an unbounded response; the full audience size
// stays available in TotalDistinctMemberCount.
const maxInvitedNoReply = 50

// GetExperience retrieves an experience from the database by its ID.
func (s *Service) GetExperience(
	ctx context.Context,
	req *connect.Request[api.GetExperienceRequest],
) (*connect.Response[api.GetExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.Id,
	)

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.Id, logger.Logger, "GetExperience")
	if err != nil {
		return nil, err
	}

	logger.Debug("retrieved experience")

	// Caller must be an active member of at least one community the experience
	// is shared with, or be the owner.
	sharedCommunityIDs, callerCommunityIDs, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityExperience, expStored.Id, expStored.OwnerId,
	)
	if err != nil {
		return nil, err
	}

	// Build API experience with community context if provided
	apiExp, err := s.buildAPIExperience(ctx, expStored, req.Msg.CommunityId)
	if err != nil {
		return nil, err
	}

	// Build cross-community RSVPs. The list is not community-scoped — once
	// the caller can see the event (gated above) they see every attendee.
	rsvps, err := s.buildRSVPs(ctx, expStored.Id)
	if err != nil {
		return nil, err
	}

	resp := &api.GetExperienceResponse{
		Experience: apiExp,
		Rsvps:      rsvps,
	}

	// Populate shared communities using the full set for counts and the
	// caller-scoped subset for the SharedCommunities list.
	if err := s.populateExperienceSharedCommunities(ctx, expStored.Id, sharedCommunityIDs, callerCommunityIDs, resp); err != nil {
		logger.WarnContext(ctx, "failed to populate experience shared communities", "error", err)
	}

	// Best-effort weather for the event's day at its location (never fails the
	// response).
	s.attachEventForecast(ctx, apiExp, resp)

	return connect.NewResponse(resp), nil
}

// attachEventForecast sets resp.Forecast to the weather for the event's
// scheduled day at its coordinates, when both are known and a weather provider
// is configured. Best-effort: it stays nil on any gap (no provider, no
// scheduled time, no coordinates, or a provider error). The event's own
// SpecificTime.timezone is the day's timezone. See docs/weather.md.
func (s *Service) attachEventForecast(ctx context.Context, exp *api.Experience, resp *api.GetExperienceResponse) {
	if s.weatherProvider == nil || exp == nil {
		return
	}
	spec := exp.GetTime().GetSpecific()
	if spec == nil || spec.GetUnixTimestampSec() == 0 {
		return // no scheduled moment → no day to forecast
	}
	lat, lng := exp.GetLatitudeDeg(), exp.GetLongitudeDeg()
	if lat == 0 && lng == 0 {
		return // no coordinates
	}
	tz := time.UTC
	if name := spec.GetTimezone(); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			tz = loc
		}
	}
	day := time.Unix(spec.GetUnixTimestampSec(), 0).In(tz)
	resp.Forecast = weatherapi.DayForecastFor(ctx, s.weatherProvider, weather.LatLng{Lat: lat, Lng: lng}, day, tz)
}

// populateExperienceSharedCommunities attaches the communities the experience
// is shared with to the response.
//
// sharedCommunityIDs is the full set of communities the experience is in.
// callerCommunityIDs is the caller-scoped subset (communities the caller is
// an active member of). TotalSharedCommunityCount and TotalDistinctMemberCount
// are derived from the full set; SharedCommunities is built from
// callerCommunityIDs to avoid leaking community details to non-members.
//
// Archived rows are included because Archived only governs feed visibility;
// it does not revoke sharing. User-initiated unsharing physically deletes
// the CommunityExperience row instead.
func (s *Service) populateExperienceSharedCommunities(
	ctx context.Context,
	experienceID string,
	sharedCommunityIDs []string,
	callerCommunityIDs []string,
	response *api.GetExperienceResponse,
) error {
	if len(sharedCommunityIDs) == 0 {
		return nil
	}

	ceRaw, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		return fmt.Errorf("failed to query community experiences: %w", err)
	}

	// Build a set of the full shared community IDs for fast lookup.
	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, cid := range sharedCommunityIDs {
		sharedSet[cid] = struct{}{}
	}

	// Filter CommunityExperience rows to the full shared set.
	allFilteredIDs := make([]string, 0, len(sharedCommunityIDs))
	ceByCommunityID := make(map[string]*models.CommunityExperience, len(sharedCommunityIDs))
	for _, m := range ceRaw {
		ce := m.(*models.CommunityExperience)
		if _, ok := sharedSet[ce.CommunityId]; !ok {
			continue
		}
		allFilteredIDs = append(allFilteredIDs, ce.CommunityId)
		ceByCommunityID[ce.CommunityId] = ce
	}
	if len(allFilteredIDs) == 0 {
		return nil
	}

	// Report the full count of communities the experience is shared with.
	response.TotalSharedCommunityCount = int32(len(allFilteredIDs))

	// Build the caller-scoped community set up front so the membership scan can
	// also collect the caller-visible members (used for the invited-no-reply
	// list, which must not leak members of communities the caller is not in).
	callerSet := make(map[string]struct{}, len(callerCommunityIDs))
	for _, cid := range callerCommunityIDs {
		callerSet[cid] = struct{}{}
	}

	// Who has responded (any RSVP) and who is the host — used both for the
	// per-community no-reply counts and the invited-no-reply list.
	responded := make(map[string]struct{}, len(response.Rsvps))
	for _, r := range response.Rsvps {
		if r.User != nil {
			responded[r.User.Id] = struct{}{}
		}
	}
	ownerID := ""
	if response.Experience != nil && response.Experience.Owner != nil {
		ownerID = response.Experience.Owner.Id
	}

	// Fetch memberships for ALL communities to compute the true distinct user count.
	membershipsRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", allFilteredIDs, &models.CommunityUser{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch memberships: %w", err)
	}
	memberCounts := make(map[string]int32, len(allFilteredIDs))
	// Per caller-visible community: members who haven't responded (excl. host).
	noReplyCounts := make(map[string]int32, len(allFilteredIDs))
	distinctUsers := make(map[string]struct{}, len(membershipsRaw))
	callerScopedUsers := make(map[string]struct{}, len(membershipsRaw))
	// Member ids per community, used to surface the origin community's
	// directly-invited individuals (#2492).
	membersByCommunity := make(map[string][]string, len(allFilteredIDs))
	for _, m := range membershipsRaw {
		cu := m.(*models.CommunityUser)
		memberCounts[cu.CommunityId]++
		membersByCommunity[cu.CommunityId] = append(
			membersByCommunity[cu.CommunityId], cu.UserId,
		)
		distinctUsers[cu.UserId] = struct{}{}
		if _, ok := callerSet[cu.CommunityId]; ok {
			callerScopedUsers[cu.UserId] = struct{}{}
			if cu.UserId != ownerID {
				if _, did := responded[cu.UserId]; !did {
					noReplyCounts[cu.CommunityId]++
				}
			}
		}
	}
	response.TotalDistinctMemberCount = int32(len(distinctUsers))
	if len(noReplyCounts) > 0 {
		response.CommunityNoReplyCounts = noReplyCounts
	}

	// Surface the caller-visible members who have not responded yet (no RSVP),
	// excluding the host — the "still no reply" people on the pitching-in panel.
	s.populateInvitedNoReply(ctx, callerScopedUsers, responded, ownerID, response)

	viewerIDs := make([]string, 0, len(callerCommunityIDs))
	for _, cid := range allFilteredIDs {
		if _, ok := callerSet[cid]; ok {
			viewerIDs = append(viewerIDs, cid)
		}
	}
	if len(viewerIDs) == 0 {
		return nil
	}

	communityMap, err := s.storage.GetByIDs(ctx, viewerIDs, &models.Community{})
	if err != nil {
		return fmt.Errorf("failed to batch-fetch communities: %w", err)
	}

	var originCommunityID string
	for _, cid := range viewerIDs {
		cm, ok := communityMap[cid]
		if !ok {
			continue
		}
		community := cm.(*models.Community)
		mediaID := ""
		if len(community.MediaIds) > 0 {
			mediaID = community.MediaIds[0]
		}
		isOrigin := community.GetOriginExperienceId() == experienceID
		if isOrigin {
			originCommunityID = cid
		}
		ce := ceByCommunityID[cid]
		response.SharedCommunities = append(response.SharedCommunities, &api.SharedCommunity{
			CommunityId:       cid,
			CommunityName:     community.Name,
			MediaId:           mediaID,
			MemberCount:       memberCounts[cid],
			SharedAtUnixSec:   ce.SharedAtUnixSec,
			IsOriginCommunity: isOrigin,
		})
	}

	// Surface the origin community's directly-invited individuals who haven't
	// responded (excl. host) so the Who's In roster can list them individually
	// in an "Invited" group; named communities stay collapsed to a no-reply
	// count. The shared helper sorts + caps; passing `responded` drops anyone
	// who already RSVP'd (#2492).
	if originCommunityID != "" {
		invited, err := services.FetchInvitedIndividuals(
			ctx, s.storage, membersByCommunity[originCommunityID], ownerID, responded,
		)
		if err != nil {
			return fmt.Errorf("failed to fetch invited individuals: %w", err)
		}
		response.InvitedIndividuals = invited
	}
	return nil
}

// populateInvitedNoReply fills response.InvitedNoReply with caller-visible
// community members who have no RSVP and are not the host — the "still no
// reply" people on the pitching-in panel. The list is deterministic (sorted by
// user ID) and capped at maxInvitedNoReply.
//
// This is supplementary data: on a fetch failure it is logged and omitted
// rather than failing the whole GetExperience response (the panel simply does
// not show the no-reply section).
func (s *Service) populateInvitedNoReply(
	ctx context.Context,
	callerScopedUsers map[string]struct{},
	responded map[string]struct{},
	ownerID string,
	response *api.GetExperienceResponse,
) {
	noReplyIDs := make([]string, 0, len(callerScopedUsers))
	for uid := range callerScopedUsers {
		if uid == ownerID {
			continue
		}
		if _, ok := responded[uid]; ok {
			continue
		}
		noReplyIDs = append(noReplyIDs, uid)
	}
	if len(noReplyIDs) == 0 {
		return
	}

	// Deterministic order, then cap.
	sort.Strings(noReplyIDs)
	if len(noReplyIDs) > maxInvitedNoReply {
		noReplyIDs = noReplyIDs[:maxInvitedNoReply]
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, noReplyIDs)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to fetch invited-no-reply users", "error", err)
		return
	}
	for _, uid := range noReplyIDs {
		if u := userMap[uid]; u != nil {
			response.InvitedNoReply = append(response.InvitedNoReply, u)
		}
	}
}

// ListExperiences retrieves experiences for a community with optional filters.
func (s *Service) ListExperiences(
	ctx context.Context,
	req *connect.Request[api.ListExperiencesRequest],
) (*connect.Response[api.ListExperiencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
	)

	// Verify the community is active and the caller is an active member,
	// matching ListCommunityGear / ListRequests / ListCommunityEvents.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Query CommunityExperience join table to find experiences in this community
	queryFields := map[string]any{
		"community_id": req.Msg.CommunityId,
	}

	communityExps, err := storage.QueryByFields[*models.CommunityExperience](s.storage, ctx, queryFields)
	if err != nil {
		logger.Error("failed to query community experiences", "error", err)
		return nil, connecterr.Internal(ctx, "ListExperiences", err)
	}

	// Extract experience IDs
	experienceIDs := storage.CollectField(communityExps, func(ce *models.CommunityExperience) string { return ce.ExperienceId })

	// Batch fetch all experiences by IDs
	expProtoMap, err := storage.GetByIDs[*models.Experience](s.storage, ctx, experienceIDs)
	if err != nil {
		logger.Error("failed to batch fetch experiences", "error", err)
		return nil, connecterr.Internal(ctx, "ListExperiences", err)
	}

	// Filter by state before building API responses.
	filtered := make([]*models.Experience, 0, len(experienceIDs))
	for _, expID := range experienceIDs {
		expStored, ok := expProtoMap[expID]
		if !ok {
			continue
		}
		if req.Msg.State != api.ExperienceState_EXPERIENCE_STATE_UNSPECIFIED {
			if convertExperienceState(expStored.State) != req.Msg.State {
				continue
			}
		}
		filtered = append(filtered, expStored)
	}

	items, err := s.buildAPIExperiences(ctx, filtered)
	if err != nil {
		logger.Error("failed to build experiences", "error", err)
		return nil, err
	}

	logger.Debug("listed community experiences", "count", len(items))

	return connect.NewResponse(&api.ListExperiencesResponse{
		Experiences: items,
	}), nil
}

// ListMyExperiences retrieves the authenticated user's created experiences.
func (s *Service) ListMyExperiences(
	ctx context.Context,
	req *connect.Request[api.ListMyExperiencesRequest],
) (*connect.Response[api.ListMyExperiencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
	)

	// Query experiences by owner_id at the database level for efficiency
	experiences, err := storage.QueryByField[*models.Experience](s.storage, ctx, "owner_id", authInfo.UserID)
	if err != nil {
		logger.Error("failed to list user experiences", "error", err)
		return nil, connecterr.Internal(ctx, "ListMyExperiences", err)
	}

	items, err := s.buildAPIExperiences(ctx, experiences)
	if err != nil {
		logger.Error("failed to build experiences", "error", err)
		return nil, err
	}

	logger.Debug("listed user experiences", "count", len(items))

	return connect.NewResponse(&api.ListMyExperiencesResponse{
		Experiences: items,
	}), nil
}
