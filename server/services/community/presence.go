package community

import (
	"context"
	"errors"
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

// maxQueuedAsks caps the peek-row list of asks that qualified but
// lost the priority contest; they stay claimable in the open list.
const maxQueuedAsks = 3

// GetCommunityPresenceForViewer selects the pinned-sheet state for the
// community profile. Priority rule (issue #2568): the soonest-deadline
// open ask holds the sheet; ties go to the longest-unclaimed
// (oldest-created) ask; deadline-less asks rank after deadlined ones,
// longest-unclaimed first. The rest queue in the peek row. With no
// qualifying ask, the next upcoming community event holds the sheet;
// else quiet.
//
// The care-rally jump from the design mock is not implemented — there
// is no queryable care-rally state on Request yet; when one exists it
// slots in ahead of the deadline comparison.
func (s *Service) GetCommunityPresenceForViewer(
	ctx context.Context,
	req *connect.Request[api.GetCommunityPresenceForViewerRequest],
) (*connect.Response[api.GetCommunityPresenceForViewerResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("community_id is required"))
	}
	if _, _, err := auth.RequireMemberOfActiveCommunity(
		ctx, s.storage, req.Msg.CommunityId, authInfo.UserID,
	); err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityPresenceForViewer",
		"community_id", req.Msg.CommunityId,
	)

	now := time.Now()
	ask, queued, err := s.detectCommunityAsk(
		ctx, authInfo.UserID, req.Msg.CommunityId, now,
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetCommunityPresenceForViewer.detectCommunityAsk", err,
			"community_id", req.Msg.CommunityId)
	}
	if ask != nil {
		logger.InfoContext(ctx, "presence selected",
			"sheet_kind", "active_ask", "queued_count", len(queued))
		return connect.NewResponse(&api.GetCommunityPresenceForViewerResponse{
			SheetKind:  api.ProfileSheetKind_PROFILE_SHEET_KIND_ACTIVE_ASK,
			ActiveAsk:  ask,
			QueuedAsks: queued,
		}), nil
	}

	event, err := s.detectCommunityEvent(
		ctx, authInfo.UserID, req.Msg.CommunityId, now,
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetCommunityPresenceForViewer.detectCommunityEvent", err,
			"community_id", req.Msg.CommunityId)
	}
	if event != nil {
		logger.InfoContext(ctx, "presence selected", "sheet_kind", "next_event")
		return connect.NewResponse(&api.GetCommunityPresenceForViewerResponse{
			SheetKind: api.ProfileSheetKind_PROFILE_SHEET_KIND_NEXT_EVENT,
			NextEvent: event,
		}), nil
	}

	logger.InfoContext(ctx, "presence selected", "sheet_kind", "quiet")
	return connect.NewResponse(&api.GetCommunityPresenceForViewerResponse{
		SheetKind: api.ProfileSheetKind_PROFILE_SHEET_KIND_QUIET,
	}), nil
}

// detectCommunityAsk applies the group priority rule over the
// community's open asks and returns the winner plus the queued rest.
func (s *Service) detectCommunityAsk(
	ctx context.Context,
	viewerID, communityID string,
	now time.Time,
) (*api.ProfileAskCard, []*api.ProfileQueuedAsk, error) {
	joins, err := storage.QueryByFields[*models.CommunityRequest](
		s.storage, ctx, map[string]any{"community_id": communityID},
	)
	if err != nil {
		return nil, nil, err
	}
	if len(joins) == 0 {
		return nil, nil, nil
	}
	ids := make([]string, 0, len(joins))
	for _, j := range joins {
		ids = append(ids, j.RequestId)
	}
	byID, err := storage.GetByIDs[*models.Request](s.storage, ctx, ids)
	if err != nil {
		return nil, nil, err
	}

	cutoff := now.Add(-time.Duration(needs.LookbackWindowDays) * 24 * time.Hour).Unix()
	nowSec := now.Unix()
	candidates := make([]*models.Request, 0, len(byID))
	for _, r := range byID {
		if presence.AskQualifies(r, nowSec, cutoff) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}

	// Priority: soonest deadline first; ties (and the deadline-less
	// tail) resolve to longest-unclaimed, i.e. oldest created.
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		aHas, bHas := a.NeededByUnixSec != nil, b.NeededByUnixSec != nil
		if aHas != bHas {
			return aHas
		}
		if aHas && *a.NeededByUnixSec != *b.NeededByUnixSec {
			return *a.NeededByUnixSec < *b.NeededByUnixSec
		}
		if a.CreatedAtUnixSec != b.CreatedAtUnixSec {
			return a.CreatedAtUnixSec < b.CreatedAtUnixSec
		}
		return a.Id < b.Id
	})

	pick := candidates[0]
	tally, err := presence.TallyOffers(ctx, s.storage, pick.Id, viewerID)
	if err != nil {
		return nil, nil, err
	}
	faces, err := presence.LoadFaces(ctx, s.storage, tally.OffererIDs)
	if err != nil {
		return nil, nil, err
	}
	card := &api.ProfileAskCard{
		RequestId:       pick.Id,
		Title:           pick.Title,
		CommunityId:     communityID,
		CommittedCount:  tally.Committed,
		Faces:           faces,
		ViewerCommitted: tally.ViewerCommitted,
	}
	if pick.NeededByUnixSec != nil {
		needed := *pick.NeededByUnixSec
		card.NeededByUnixSec = &needed
	}

	queued := make([]*api.ProfileQueuedAsk, 0, maxQueuedAsks)
	for _, r := range candidates[1:] {
		if len(queued) == maxQueuedAsks {
			break
		}
		q := &api.ProfileQueuedAsk{RequestId: r.Id, Title: r.Title}
		if r.NeededByUnixSec != nil {
			needed := *r.NeededByUnixSec
			q.NeededByUnixSec = &needed
		}
		queued = append(queued, q)
	}
	return card, queued, nil
}

// detectCommunityEvent returns the community's soonest upcoming event
// with its attendance, or nil when none is scheduled.
func (s *Service) detectCommunityEvent(
	ctx context.Context,
	viewerID, communityID string,
	now time.Time,
) (*api.ProfileEventCard, error) {
	pivots, err := storage.QueryByFields[*models.CommunityExperience](
		s.storage, ctx, map[string]any{"community_id": communityID},
	)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(pivots))
	for _, ce := range pivots {
		if ce.Deleted != nil || ce.Archived {
			continue
		}
		ids = append(ids, ce.ExperienceId)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	byID, err := storage.GetByIDs[*models.Experience](s.storage, ctx, ids)
	if err != nil {
		return nil, err
	}

	nowSec := now.Unix()
	var next *models.Experience
	for _, e := range byID {
		if e == nil || e.Deleted != nil {
			continue
		}
		if e.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
			e.State != models.ExperienceState_EXPERIENCE_STATE_JOINED &&
			e.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			continue
		}
		start := available_now.ExperienceStartUnixSec(e)
		if start == 0 || start <= nowSec {
			continue
		}
		if next == nil || start < available_now.ExperienceStartUnixSec(next) {
			next = e
		}
	}
	if next == nil {
		return nil, nil
	}

	rsvps, err := storage.QueryByFields[*models.ExperienceRSVP](
		s.storage, ctx, map[string]any{"experience_id": next.Id},
	)
	if err != nil {
		return nil, err
	}
	going := int32(0)
	goingIDs := make([]string, 0, len(rsvps))
	for _, r := range rsvps {
		if r.Deleted != nil || r.GetIntention() != models.RSVPIntention_RSVP_INTENTION_YES || r.UserId == "" {
			continue
		}
		if r.CommunityId != communityID {
			continue
		}
		going++
		if r.UserId != viewerID {
			goingIDs = append(goingIDs, r.UserId)
		}
	}
	faces, err := presence.LoadFaces(ctx, s.storage, goingIDs)
	if err != nil {
		return nil, err
	}
	return &api.ProfileEventCard{
		ExperienceId: next.Id,
		Title:        next.Name,
		StartUnixSec: available_now.ExperienceStartUnixSec(next),
		CommunityId:  communityID,
		GoingCount:   going,
		Faces:        faces,
	}, nil
}
