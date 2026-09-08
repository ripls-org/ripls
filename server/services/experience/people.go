package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/services"
)

// GetExperiencePeople retrieves people associated with an experience (host, RSVPs, attendees).
func (s *Service) GetExperiencePeople(
	ctx context.Context,
	req *connect.Request[api.GetExperiencePeopleRequest],
) (*connect.Response[api.GetExperiencePeopleResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"experience_id", req.Msg.ExperienceId,
	)

	logger.DebugContext(ctx, "fetching experience people")

	experience, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "GetExperiencePeople")
	if err != nil {
		return nil, err
	}

	// Fetch host details
	host, err := services.FetchAPIUser(ctx, s.storage, experience.OwnerId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to fetch host",
			"owner_id", experience.OwnerId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetExperiencePeople", fmt.Errorf("failed to fetch host"))
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	// Reject if community is missing, soft-deleted, or caller is not a member.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Build query for RSVPs scoped to the specific community.
	queryFields := map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"community_id":  req.Msg.CommunityId,
	}

	// Fetch RSVPs for this experience in this community.
	rsvpsProto, err := s.storage.QueryByFields(ctx, queryFields, &models.ExperienceRSVP{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query RSVPs",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetExperiencePeople", fmt.Errorf("failed to query RSVPs"))
	}

	// Track unique users for upcoming RSVPs and past attendees
	upcomingRSVPsMap := make(map[string]*api.User)
	pastAttendeesMap := make(map[string]*api.User)

	// Process RSVPs to categorize users
	for _, msg := range rsvpsProto {
		rsvp := msg.(*models.ExperienceRSVP)

		// Skip if user is the host
		if rsvp.UserId == experience.OwnerId {
			continue
		}

		// Fetch user if not already fetched
		user, err := services.FetchAPIUser(ctx, s.storage, rsvp.UserId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch RSVP user",
				"user_id", rsvp.UserId,
				"error", err,
			)
			continue // Skip this RSVP if we can't fetch the user
		}

		// Categorize based on attended status and intention
		if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_YES {
			// User attended a past session
			pastAttendeesMap[rsvp.UserId] = user
		} else if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_UNKNOWN && rsvpstate.IsGoing(rsvp.GetIntention()) {
			// User has RSVP'd for upcoming session (not yet attended)
			if _, alreadyAttended := pastAttendeesMap[rsvp.UserId]; !alreadyAttended {
				upcomingRSVPsMap[rsvp.UserId] = user
			}
		}
	}

	// Convert maps to slices
	var upcomingRSVPs []*api.User
	for _, user := range upcomingRSVPsMap {
		upcomingRSVPs = append(upcomingRSVPs, user)
	}

	var pastAttendees []*api.User
	for _, user := range pastAttendeesMap {
		pastAttendees = append(pastAttendees, user)
	}

	// Calculate total count (host + upcoming RSVPs + past attendees)
	totalCount := int32(1) // Always count the host
	totalCount += int32(len(upcomingRSVPs))
	totalCount += int32(len(pastAttendees))

	logger.DebugContext(ctx, "fetched experience people",
		"total_count", totalCount,
		"upcoming_rsvps_count", len(upcomingRSVPs),
		"past_attendees_count", len(pastAttendees),
	)

	return connect.NewResponse(&api.GetExperiencePeopleResponse{
		Host:          host,
		UpcomingRsvps: upcomingRSVPs,
		PastAttendees: pastAttendees,
		TotalCount:    totalCount,
	}), nil
}
