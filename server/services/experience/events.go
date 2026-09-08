package experience

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// recordRSVPEvent records a community event for an RSVP action.
// Returns the community ID if successful, empty string otherwise.
func (s *Service) recordRSVPEvent(
	ctx context.Context,
	experienceID string,
	actorID string,
	ownerID string,
	intention api.RSVPIntention,
) string {
	logger := logging.LoggerWithContext(ctx).With(
		"experience_id", experienceID,
		"actor_id", actorID,
		"intention", intention.String(),
	)

	// Look up the community ID for this experience
	communityExperiences, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query community for experience", "error", err)
		return ""
	}
	if len(communityExperiences) == 0 {
		logger.DebugContext(ctx, "no community found for experience")
		return ""
	}

	communityExp := communityExperiences[0].(*models.CommunityExperience)
	communityID := communityExp.CommunityId

	// Map intention to event type
	var eventType models.CommunityEventType
	switch intention {
	case api.RSVPIntention_RSVP_INTENTION_YES:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES
	case api.RSVPIntention_RSVP_INTENTION_MAYBE:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE
	case api.RSVPIntention_RSVP_INTENTION_NO:
		eventType = models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO
	default:
		logger.WarnContext(ctx, "unknown RSVP intention, not recording event")
		return communityID
	}

	// Create the community event
	event := &models.CommunityEvent{
		CommunityId:  communityID,
		EventType:    eventType,
		ActorId:      actorID,
		ObjectUserId: ownerID, // The experience owner who should be notified
		Topic:        &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}

	// Record the event and send notification
	if _, err := s.bus.Publish(ctx, event); err != nil {
		logger.WarnContext(ctx, "failed to record RSVP community event", "error", err)
	}

	// The notifying RSVP event above went only to communityID. Fan a silent
	// roster refresh out to any sibling communities the experience is also
	// shared to, so co-invitees viewing it there see the roster update live
	// without a duplicate push. (#2492)
	s.publishRosterChangedToOthers(ctx, experienceID, actorID, communityID)

	return communityID
}

// rsvpEventType maps an RSVP intention to its community event type. Returns
// false for UNSPECIFIED, which carries no roster event to emit.
func rsvpEventType(intention api.RSVPIntention) (models.CommunityEventType, bool) {
	switch intention {
	case api.RSVPIntention_RSVP_INTENTION_YES:
		return models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, true
	case api.RSVPIntention_RSVP_INTENTION_MAYBE:
		return models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE, true
	case api.RSVPIntention_RSVP_INTENTION_NO:
		return models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO, true
	default:
		return 0, false
	}
}

// publishHostSetRSVPEvent streams a host-initiated RSVP change so the affected
// member's (and other viewers') open screens refresh in real time. It targets
// the event's origin (per-item) community — where the host and directly-invited
// members live — falling back to any community the experience is shared to.
//
// ActorId is the host: the broadcast skips the actor (the host already
// refreshed locally when the RPC returned), so the member and other streamers
// receive it. ObjectUserId is left empty so no push notification fires — the
// host made the change deliberately and online viewers get the live update.
func (s *Service) publishHostSetRSVPEvent(ctx context.Context, experienceID, hostID string, intention api.RSVPIntention) {
	if s.bus == nil {
		return
	}
	eventType, ok := rsvpEventType(intention)
	if !ok {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "publishHostSetRSVPEvent",
		"experience_id", experienceID,
	)
	communityID, err := s.originCommunityID(ctx, experienceID)
	if err != nil {
		logger.WarnContext(ctx, "origin community lookup failed", "error", err)
	}
	if communityID == "" {
		// Named-only experience (no per-item community): fall back to the first
		// community it is shared to.
		if ces, e := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{}); e == nil && len(ces) > 0 {
			communityID = ces[0].(*models.CommunityExperience).CommunityId
		}
	}
	if communityID == "" {
		logger.DebugContext(ctx, "no community to stream host-set RSVP to")
		return
	}
	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   eventType,
		ActorId:     hostID,
		Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
	}
	if _, err := s.bus.Publish(ctx, event); err != nil {
		logger.WarnContext(ctx, "failed to publish host-set RSVP event", "error", err)
	}
	// The RSVP event above went only to communityID; reach viewers streaming a
	// sibling community the experience is also shared to. (#2492)
	s.publishRosterChangedToOthers(ctx, experienceID, hostID, communityID)
}

// publishRosterChangedToOthers streams a silent EXPERIENCE_ROSTER_CHANGED to
// every community the experience is shared to except excludeCommunityID, so
// viewers in a sibling community still see a roster change live when the
// primary event went to only one community. ROSTER_CHANGED is not in the
// notification allow-list (ShouldNotify), so this fires no push — it is a
// pure live-refresh signal. Pass excludeCommunityID == "" to reach every
// community (the reset-to-invited / remove-member paths, which emit no other
// event). The stream broadcaster skips the actor, so the host who made the
// change isn't echoed back to itself (#2492).
func (s *Service) publishRosterChangedToOthers(ctx context.Context, experienceID, actorID, excludeCommunityID string) {
	if s.bus == nil {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "publishRosterChangedToOthers",
		"experience_id", experienceID,
	)
	ces, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query communities for roster fan-out", "error", err)
		return
	}
	for _, m := range ces {
		communityID := m.(*models.CommunityExperience).CommunityId
		if communityID == "" || communityID == excludeCommunityID {
			continue
		}
		event := &models.CommunityEvent{
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED,
			ActorId:     actorID,
			Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
		}
		if _, err := s.bus.Publish(ctx, event); err != nil {
			logger.WarnContext(ctx, "failed to publish roster-changed event",
				"error", err, "community_id", communityID)
		}
	}
}
