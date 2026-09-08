package experience

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// emitPlanningCommunityEvent records a CommunityEvent for a planning action
// on the given experience, fanning out across every community the
// experience is currently shared with. Each per-community emission goes
// through RecordCommunityEventAndNotify so the standard funnel (filter +
// preference gate + stream suppression) applies.
//
// Failures are logged and swallowed: planning chat-system messages remain
// the source of truth in-conversation; missing a push is a soft failure.
// itemName is the name of the need or contribution involved (empty for bulk
// adds where there is no single item); it rides on the event so notification
// copy can say what was needed or brought.
func (s *Service) emitPlanningCommunityEvent(
	ctx context.Context,
	eventType models.CommunityEventType,
	actorID, experienceID, itemName string,
) {
	if s.notificationService == nil {
		return
	}

	links, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{})
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to load community-experience links for planning event",
			"experience_id", experienceID,
			"error", err,
		)
		return
	}

	for _, msg := range links {
		ce := msg.(*models.CommunityExperience)
		if ce.Archived || ce.Deleted != nil {
			continue
		}
		event := &models.CommunityEvent{
			CommunityId:      ce.CommunityId,
			EventType:        eventType,
			ActorId:          actorID,
			Topic:            &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
			PlanningItemName: itemName,
		}
		if _, err := s.bus.Publish(ctx, event); err != nil {
			logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to record planning community event",
				"experience_id", experienceID,
				"community_id", ce.CommunityId,
				"event_type", eventType,
				"error", err,
			)
		}
	}
}
