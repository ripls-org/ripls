package profile

import (
	"context"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/available_now"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/needs"
	"go.ripls.org/ripls/server/presence"
	"go.ripls.org/ripls/server/storage"
)

// GetProfilePresenceForViewer selects the pinned-sheet state for the
// target user's profile. Fallback chain: the target's most pressing
// open ask in a viewer↔target shared community → the next upcoming
// event both are in → quiet. A brand-new connection (no shared
// history, exactly one shared upcoming event) gets the cold-start
// treatment instead of next-event.
//
// Everything is scoped to shared communities: entities, faces, and
// counts never reference a community or person the viewer is not
// already connected to.
func (s *Service) GetProfilePresenceForViewer(
	ctx context.Context,
	req *connect.Request[api.GetProfilePresenceForViewerRequest],
) (*connect.Response[api.GetProfilePresenceForViewerResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.TargetUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("target_user_id is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetProfilePresenceForViewer",
		"target_user_id", req.Msg.TargetUserId,
	)

	quiet := &api.GetProfilePresenceForViewerResponse{
		SheetKind: api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET,
	}

	// Self-view: there is no ask to commit to on your own profile.
	if req.Msg.TargetUserId == authInfo.UserID {
		return connect.NewResponse(quiet), nil
	}

	sharedRefs, _, _, _, err := s.resolveSharedCommunities(
		ctx, authInfo.UserID, req.Msg.TargetUserId,
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetProfilePresenceForViewer.resolveSharedCommunities", err,
			"target_user_id", req.Msg.TargetUserId)
	}
	if len(sharedRefs) == 0 {
		return connect.NewResponse(quiet), nil
	}
	sharedSet := make(map[string]struct{}, len(sharedRefs))
	for _, r := range sharedRefs {
		sharedSet[r.Id] = struct{}{}
	}

	now := time.Now()

	ask, err := s.detectTargetAsk(ctx, authInfo.UserID, req.Msg.TargetUserId, sharedSet, now)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetProfilePresenceForViewer.detectTargetAsk", err,
			"target_user_id", req.Msg.TargetUserId)
	}
	if ask != nil {
		logger.InfoContext(ctx, "presence selected", "sheet_kind", "active_ask")
		return connect.NewResponse(&api.GetProfilePresenceForViewerResponse{
			SheetKind: api.ProfileSheetKind_PROFILE_SHEET_KIND_ACTIVE_ASK,
			ActiveAsk: ask,
		}), nil
	}

	nextEvent, pastCount, upcomingCount, err := s.detectSharedEvents(
		ctx, authInfo.UserID, req.Msg.TargetUserId, sharedSet, now,
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetProfilePresenceForViewer.detectSharedEvents", err,
			"target_user_id", req.Msg.TargetUserId)
	}
	if nextEvent == nil {
		logger.InfoContext(ctx, "presence selected", "sheet_kind", "quiet")
		return connect.NewResponse(quiet), nil
	}

	kind := api.ProfileSheetKind_PROFILE_SHEET_KIND_NEXT_EVENT
	suppress := false
	if pastCount == 0 && upcomingCount == 1 {
		kind = api.ProfileSheetKind_PROFILE_SHEET_KIND_COLD_START
		suppress = true
	}
	logger.InfoContext(ctx, "presence selected",
		"sheet_kind", kind.String(),
		"shared_past_events", pastCount,
		"shared_upcoming_events", upcomingCount,
	)
	return connect.NewResponse(&api.GetProfilePresenceForViewerResponse{
		SheetKind:       kind,
		NextEvent:       nextEvent,
		SuppressHistory: suppress,
	}), nil
}

// detectTargetAsk returns the target's most pressing open ask inside
// the shared communities, or nil when none qualifies. Selection:
// asks with a future deadline rank first (soonest wins); deadline-less
// asks created within the needs lookback window follow, most recent
// first. Mirrors the needs detector's freshness gate so a months-old
// deadline-less ask does not hold the sheet. The viewer's own offer
// (if any) sets viewer_committed and is excluded from the face stack.
func (s *Service) detectTargetAsk(
	ctx context.Context,
	viewerID, targetID string,
	sharedSet map[string]struct{},
	now time.Time,
) (*api.ProfileAskCard, error) {
	authored, err := storage.QueryByField[*models.Request](
		s.storage, ctx, "requester_id", targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("query target requests: %w", err)
	}

	cutoff := now.Add(-time.Duration(needs.LookbackWindowDays) * 24 * time.Hour).Unix()
	nowSec := now.Unix()
	open := make(map[string]*models.Request, len(authored))
	openIDs := make([]string, 0, len(authored))
	for _, r := range authored {
		if !presence.AskQualifies(r, nowSec, cutoff) {
			continue
		}
		open[r.Id] = r
		openIDs = append(openIDs, r.Id)
	}
	if len(openIDs) == 0 {
		return nil, nil
	}

	joins, err := storage.QueryByFieldIn[*models.CommunityRequest](
		s.storage, ctx, "request_id", openIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_request joins: %w", err)
	}
	// A request may be shared into several communities; keep the first
	// shared community per request for routing.
	askCommunity := make(map[string]string, len(openIDs))
	for _, j := range joins {
		if _, shared := sharedSet[j.CommunityId]; !shared {
			continue
		}
		if _, seen := askCommunity[j.RequestId]; !seen {
			askCommunity[j.RequestId] = j.CommunityId
		}
	}
	if len(askCommunity) == 0 {
		return nil, nil
	}

	type candidate struct {
		req         *models.Request
		communityID string
	}
	candidates := make([]candidate, 0, len(askCommunity))
	for id, communityID := range askCommunity {
		candidates = append(candidates, candidate{req: open[id], communityID: communityID})
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i].req, candidates[j].req
		aHas, bHas := a.NeededByUnixSec != nil, b.NeededByUnixSec != nil
		if aHas != bHas {
			return aHas
		}
		if aHas {
			if *a.NeededByUnixSec != *b.NeededByUnixSec {
				return *a.NeededByUnixSec < *b.NeededByUnixSec
			}
		} else if a.CreatedAtUnixSec != b.CreatedAtUnixSec {
			return a.CreatedAtUnixSec > b.CreatedAtUnixSec
		}
		return a.Id < b.Id
	})
	pick := candidates[0]

	tally, err := presence.TallyOffers(ctx, s.storage, pick.req.Id, viewerID)
	if err != nil {
		return nil, err
	}
	faces, err := presence.LoadFaces(ctx, s.storage, tally.OffererIDs)
	if err != nil {
		return nil, err
	}

	card := &api.ProfileAskCard{
		RequestId:       pick.req.Id,
		Title:           pick.req.Title,
		CommunityId:     pick.communityID,
		CommittedCount:  tally.Committed,
		Faces:           faces,
		ViewerCommitted: tally.ViewerCommitted,
	}
	if pick.req.NeededByUnixSec != nil {
		needed := *pick.req.NeededByUnixSec
		card.NeededByUnixSec = &needed
	}
	return card, nil
}

// detectSharedEvents intersects the viewer's and target's yes-RSVPs
// inside shared communities and returns the soonest upcoming event
// (with faces), plus the pair's past and upcoming shared-event counts
// — the inputs to the cold-start gate.
func (s *Service) detectSharedEvents(
	ctx context.Context,
	viewerID, targetID string,
	sharedSet map[string]struct{},
	now time.Time,
) (*api.ProfileEventCard, int, int, error) {
	viewerYes, err := s.loadYesRSVPExperienceIDs(ctx, viewerID, sharedSet)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("viewer rsvps: %w", err)
	}
	if len(viewerYes) == 0 {
		return nil, 0, 0, nil
	}
	targetYes, err := s.loadYesRSVPExperienceIDs(ctx, targetID, sharedSet)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("target rsvps: %w", err)
	}

	sharedIDs := make([]string, 0, len(targetYes))
	for id := range targetYes {
		if _, ok := viewerYes[id]; ok {
			sharedIDs = append(sharedIDs, id)
		}
	}
	if len(sharedIDs) == 0 {
		return nil, 0, 0, nil
	}

	experiences, err := storage.GetByIDs[*models.Experience](s.storage, ctx, sharedIDs)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("get shared experiences: %w", err)
	}

	nowSec := now.Unix()
	var next *models.Experience
	var nextCommunity string
	pastCount, upcomingCount := 0, 0
	for _, id := range sharedIDs {
		e, ok := experiences[id]
		if !ok || e == nil || e.Deleted != nil {
			continue
		}
		start := available_now.ExperienceStartUnixSec(e)
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
			(start != 0 && start <= nowSec) {
			pastCount++
			continue
		}
		if e.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
			e.State != models.ExperienceState_EXPERIENCE_STATE_JOINED &&
			e.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			continue
		}
		if start == 0 || start <= nowSec {
			continue
		}
		upcomingCount++
		if next == nil || start < available_now.ExperienceStartUnixSec(next) {
			next = e
			nextCommunity = targetYes[id]
		}
	}
	if next == nil {
		return nil, pastCount, upcomingCount, nil
	}

	goingRSVPs, err := storage.QueryByField[*models.ExperienceRSVP](
		s.storage, ctx, "experience_id", next.Id,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("query next-event rsvps: %w", err)
	}
	going := int32(0)
	goingUserIDs := make([]string, 0, len(goingRSVPs))
	for _, r := range goingRSVPs {
		if r.Deleted != nil || r.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES || r.UserId == "" {
			continue
		}
		// Faces stay inside the shared scope: skip RSVPs recorded in a
		// community the viewer is not a member of.
		if _, ok := sharedSet[r.CommunityId]; !ok {
			continue
		}
		going++
		if r.UserId != viewerID {
			goingUserIDs = append(goingUserIDs, r.UserId)
		}
	}
	faces, err := presence.LoadFaces(ctx, s.storage, goingUserIDs)
	if err != nil {
		return nil, 0, 0, err
	}

	return &api.ProfileEventCard{
		ExperienceId: next.Id,
		Title:        next.Name,
		StartUnixSec: available_now.ExperienceStartUnixSec(next),
		CommunityId:  nextCommunity,
		GoingCount:   going,
		Faces:        faces,
	}, pastCount, upcomingCount, nil
}

// loadYesRSVPExperienceIDs returns the experience ids the user has an
// active yes-RSVP for inside the shared communities, mapped to the
// shared community the RSVP was recorded in.
func (s *Service) loadYesRSVPExperienceIDs(
	ctx context.Context,
	userID string,
	sharedSet map[string]struct{},
) (map[string]string, error) {
	rsvps, err := storage.QueryByField[*models.ExperienceRSVP](
		s.storage, ctx, "user_id", userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query rsvps for user: %w", err)
	}
	out := make(map[string]string, len(rsvps))
	for _, r := range rsvps {
		if r.Deleted != nil || r.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES {
			continue
		}
		if _, ok := sharedSet[r.CommunityId]; !ok {
			continue
		}
		if _, seen := out[r.ExperienceId]; !seen {
			out[r.ExperienceId] = r.CommunityId
		}
	}
	return out, nil
}
