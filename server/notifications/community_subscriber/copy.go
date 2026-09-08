package community_subscriber

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// buildNotification renders the push notification copy for a
// community event using loc to translate. Per the render boundary
// (docs/server/l10n.md), the server is the terminal emitter for push
// payloads, so the title and body strings here are pre-rendered;
// FCM/APNs displays them verbatim.
//
// Pass a Localizer for the recipient's locale — typically resolved
// once per locale in Handle so a fan-out to multiple recipients
// shares one Localizer per language. Passing nil falls back to the
// English default via the Localizer's nil-safe behavior.
func buildNotification(ctx context.Context, s *storage.ProtoSQLStorage, event *models.CommunityEvent, loc *l10n.Localizer) *models.Notification {
	actorName := getUserName(ctx, s, event.ActorId)
	gearName := ""
	if event.GearId != "" {
		gearName = getGearName(ctx, s, event.GearId)
	}
	requestTitle := ""
	if event.GetRequestId() != "" {
		requestTitle = getRequestTitle(ctx, s, event.GetRequestId())
	}
	experienceName := ""
	if event.GetExperienceId() != "" {
		experienceName = getExperienceName(ctx, s, event.GetExperienceId())
	}
	// Resolved once and carried on the payload so off-app renderers (notification_content)
	// can compose self-contained SMS/email copy without re-querying. Used in the
	// push title/body only for membership/lifecycle kinds (set into params below).
	communityName := getCommunityName(ctx, s, event.CommunityId)

	// For a gear-share, resolve whether it's a giveaway or a loan so off-app copy
	// can say "giving away" + Claim vs "lending out" + Borrow.
	var gearAvailability models.Availability
	if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED && event.GearId != "" {
		gearAvailability = getGearAvailability(ctx, s, event.GearId, event.CommunityId)
	}

	// COMMUNITY_DELETED fires after the soft-delete, so the ordinary read filter
	// hides the row; fetch with IncludeDeleted so the copy can still name it. A
	// miss leaves the name empty, which the renderer covers with its stand-in
	// ("your community") rather than a blank slot.
	if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED {
		c := &models.Community{}
		if err := s.GetByID(ctx, event.CommunityId, c, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			logging.LoggerWithContext(ctx).WarnContext(ctx,
				"failed to read deleted community for notification copy",
				"community_id", event.CommunityId, "error", err)
		} else {
			communityName = c.Name
		}
	}

	payload := &models.CommunityEventPayload{
		CommunityId:      event.CommunityId,
		EventId:          event.Id,
		EventType:        event.EventType.String(),
		GearName:         gearName,
		RequestTitle:     requestTitle,
		GearId:           event.GearId,
		ActorName:        actorName,
		ExperienceName:   experienceName,
		CommunityName:    communityName,
		GearAvailability: gearAvailability,
		PlanningItemName: event.PlanningItemName,
	}
	if experienceID := event.GetExperienceId(); experienceID != "" {
		payload.ExperienceId = &experienceID
	}
	if requestID := event.GetRequestId(); requestID != "" {
		payload.RequestId = &requestID
	}
	if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED {
		if tc := event.GetTimeChanged(); tc {
			payload.TimeChanged = &tc
		}
		if lc := event.GetLocationChanged(); lc {
			payload.LocationChanged = &lc
		}
	}
	// A loan and a giveaway are different sentences, and a cancelled offer that
	// was simply no longer needed is a third. Both distinctions ride the payload
	// so push and off-app split the same way — deciding "covered" costs a
	// Transfer read, which only this path does.
	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		payload.TransferType = event.TransferType
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED:
		payload.TransferType = event.TransferType
		payload.TransferOfferCovered = isCoveredOfferCancellation(ctx, s, event)
	}

	// One sentence, rendered from the payload the off-app senders will re-render
	// from. Push's only surface-specific adornment is its title (#2896).
	title, body := notification_content.FromCommunityEvent(ctx, payload).PushCopy(ctx, loc)
	return &models.Notification{
		Title: title,
		Body:  body,
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: payload,
		},
	}
}

// getRequestTitle retrieves a request's title by ID, returning empty string on error.
func getRequestTitle(ctx context.Context, s *storage.ProtoSQLStorage, requestID string) string {
	if requestID == "" {
		return ""
	}
	request := &models.Request{}
	if err := s.GetByID(ctx, requestID, request); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get request title", "target_request_id", requestID, "error", err)
		return ""
	}
	return request.Title
}

// getUserName retrieves a user's name by ID, returning empty string on error.
func getUserName(ctx context.Context, s *storage.ProtoSQLStorage, userID string) string {
	user, err := services.FetchAPIUser(ctx, s, userID)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get user name", "user_id", userID, "error", err)
		return ""
	}
	if user == nil {
		return ""
	}
	return user.Name
}

// getGearName retrieves a gear's name by ID, returning empty string on error.
func getGearName(ctx context.Context, s *storage.ProtoSQLStorage, gearID string) string {
	if gearID == "" {
		return ""
	}
	gear := &models.Gear{}
	if err := s.GetByID(ctx, gearID, gear); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get gear name", "gear_id", gearID, "error", err)
		return ""
	}
	return gear.Name
}

// getExperienceName retrieves an experience's name by ID, returning empty string on error.
func getExperienceName(ctx context.Context, s *storage.ProtoSQLStorage, experienceID string) string {
	if experienceID == "" {
		return ""
	}
	experience := &models.Experience{}
	if err := s.GetByID(ctx, experienceID, experience); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get experience name", "experience_id", experienceID, "error", err)
		return ""
	}
	return experience.Name
}

// getGearAvailability returns the FOR_LOAN/FOR_GIVEAWAY setting of a gear within
// a community (from its CommunityGear junction), or UNSPECIFIED on miss/error.
func getGearAvailability(ctx context.Context, s *storage.ProtoSQLStorage, gearID, communityID string) models.Availability {
	rows, err := s.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil || len(rows) == 0 {
		return models.Availability_AVAILABILITY_UNSPECIFIED
	}
	return rows[0].(*models.CommunityGear).Availability
}

// getCommunityName retrieves a community's name by ID, returning empty string on error.
func getCommunityName(ctx context.Context, s *storage.ProtoSQLStorage, communityID string) string {
	if communityID == "" {
		return ""
	}
	c := &models.Community{}
	if err := s.GetByID(ctx, communityID, c); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get community name", "community_id", communityID, "error", err)
		return ""
	}
	return c.Name
}

// isCoveredOfferCancellation reports whether a TRANSFER_CANCELLED event
// closed a gear-backed request offer (#2702) from the requester's side — the
// transfer is origin-linked and the acting party is its recipient (the
// requester). Covers both the automatic stand-down when the request is
// covered and the requester declining an offer; the copy reads correctly
// for both.
func isCoveredOfferCancellation(ctx context.Context, s *storage.ProtoSQLStorage, event *models.CommunityEvent) bool {
	if event.GetTransferId() == "" {
		return false
	}
	transfer := &models.Transfer{}
	if err := s.GetByID(ctx, event.GetTransferId(), transfer); err != nil {
		return false
	}
	return transfer.GetOriginRequestId() != "" && event.ActorId == transfer.RecipientId
}
