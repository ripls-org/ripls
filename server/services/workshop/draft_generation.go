package workshop

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GenerateWorkshopDraft generates an experience draft prefilled from
// the prior-instance experience the Workshop action card references.
// The client hydrates the draft into the experience preview modal so
// the host edits-and-publishes rather than starting blank.
//
// Authorization: the calling user must own the prior experience
// (Experience.owner_id == authenticated user).
//
// Scope: template-shaped prefill — name, description, time,
// location, media_ids, participant_ids inherited from the prior
// instance.
func (s *Service) GenerateWorkshopDraft(
	ctx context.Context,
	req *connect.Request[api.GenerateWorkshopDraftRequest],
) (*connect.Response[api.GenerateWorkshopDraftResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.ExperienceId == "" {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("experience_id is required"),
		)
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GenerateWorkshopDraft",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	prior := &models.Experience{}
	if err := s.storage.GetByID(ctx, req.Msg.ExperienceId, prior); err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("experience not found: %w", err))
	}
	if prior.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("experience belongs to a different user"))
	}

	resp := &api.GenerateWorkshopDraftResponse{
		Name:        prior.Name,
		Description: prior.Description,
		MediaIds:    prior.MediaIds,
	}
	if prior.LocationId != "" {
		loc := prior.LocationId
		resp.LocationId = &loc
	}
	if next, ok := nextWeeklySlot(prior.Time, clock.Now(ctx)); ok {
		resp.TimeUnixSec = &next
	}
	resp.ParticipantIds = s.priorAttendeeIDs(ctx, prior.Id, logger)

	logger.InfoContext(ctx, "workshop draft generated",
		"context_experience_id", prior.Id,
		"participant_count", len(resp.ParticipantIds),
		"media_count", len(resp.MediaIds),
		"suggested_time_unix_sec", resp.GetTimeUnixSec(),
	)

	return connect.NewResponse(resp), nil
}

// maxWeeklyRollForward bounds the roll-forward loop in nextWeeklySlot. A
// prior instance more than this many weeks in the past is a revival, not a
// cadence, so no time is suggested and the host picks one in the modal.
const maxWeeklyRollForward = 520 // ten years of weeks

// nextWeeklySlot infers the draft's suggested start from the prior instance:
// the same weekday and wall-clock time, at the next such slot strictly after
// `now`. A weekly rhythm is by far the dominant repeat cadence, and it is the
// only one a single prior instance can evidence — a host repeating something
// monthly just moves the date in the preview.
//
// The roll-forward steps by calendar days in the prior instance's own
// timezone, so a 6:30am run stays a 6:30am run across a DST boundary rather
// than drifting to 5:30 or 7:30. Returns ok=false when the prior instance has
// no usable start (TBD time, unset time, or a start so old that stepping to
// the present would take more than maxWeeklyRollForward hops).
func nextWeeklySlot(prior *models.ExperienceTime, now time.Time) (int64, bool) {
	startUnixSec, tz := priorStart(prior)
	if startUnixSec <= 0 {
		return 0, false
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	next := time.Unix(startUnixSec, 0).In(loc)
	for hops := 0; !next.After(now); hops++ {
		if hops >= maxWeeklyRollForward {
			return 0, false
		}
		next = next.AddDate(0, 0, 7)
	}
	return next.Unix(), true
}

// priorStart pulls the start instant and IANA timezone out of an
// ExperienceTime. A TBD time (or a nil/unset one) yields a zero start, which
// nextWeeklySlot reads as "nothing to infer from".
func priorStart(t *models.ExperienceTime) (startUnixSec int64, timezone string) {
	switch v := t.GetTimeType().(type) {
	case *models.ExperienceTime_Specific:
		return v.Specific.GetUnixTimestampSec(), v.Specific.GetTimezone()
	case *models.ExperienceTime_Range:
		return v.Range.GetStartUnixSec(), v.Range.GetTimezone()
	default:
		return 0, ""
	}
}

// priorAttendeeIDs returns the user ids that RSVPed YES (attended) to the
// prior instance. Used as the default re-invite set on the draft. Returns
// an empty slice on storage failures — the host can pick attendees in the
// modal.
func (s *Service) priorAttendeeIDs(
	ctx context.Context,
	experienceID string,
	logger interface {
		WarnContext(ctx context.Context, msg string, args ...any)
	},
) []string {
	rsvps, err := storage.QueryByFields[*models.ExperienceRSVP](
		s.storage, ctx,
		map[string]any{"experience_id": experienceID},
	)
	if err != nil {
		logger.WarnContext(ctx, "load RSVPs for draft failed",
			"experience_id", experienceID,
			"error", err,
		)
		return nil
	}
	out := make([]string, 0, len(rsvps))
	seen := map[string]bool{}
	for _, r := range rsvps {
		if r.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
			continue
		}
		if r.UserId == "" || seen[r.UserId] {
			continue
		}
		seen[r.UserId] = true
		out = append(out, r.UserId)
	}
	return out
}
