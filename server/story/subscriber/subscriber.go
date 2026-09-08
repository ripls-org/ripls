package story_subscriber

import (
	"context"

	"go.ripls.org/ripls/server/community"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/story"
)

// SubscriberName is the stable identifier used in pubsub log lines.
const SubscriberName = "story"

// Subscriber listens for terminal-state community events and produces a
// Story via story.Creator. Replaces the inline storyCreator.CreateStory
// calls that used to live in the four emitter services (#510 PR 4).
type Subscriber struct {
	storage *storage.ProtoSQLStorage
	creator story.Creator
}

// New constructs a story subscriber. creator may be nil — which is treated
// as "story generation disabled," matching the pre-PR-4 behavior where
// services held a nullable story.Creator.
func New(s *storage.ProtoSQLStorage, creator story.Creator) *Subscriber {
	return &Subscriber{storage: s, creator: creator}
}

// Name implements pubsub.Subscriber.
func (s *Subscriber) Name() string { return SubscriberName }

// Handle implements pubsub.Subscriber. Dispatches by event type. Returns
// nil for events the subscriber doesn't care about.
//
// Errors from the per-type generators are logged at WARN inside each
// generator and not returned, so a single story-generation failure
// doesn't poison the dispatch line. The bus's per-dispatch log captures
// the per-call duration regardless.
func (s *Subscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil || s.creator == nil {
		return nil
	}
	event := evt.Event

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "story_subscriber.handle",
		"community_event_id", event.Id,
		"community_id", event.CommunityId,
		"event_type", event.EventType.String(),
	)

	// Soft-delete gate. The audit row is already recorded; this only
	// suppresses story generation for soft-deleted communities. Belt-and-
	// suspenders against the upstream lifecycle paths that already reject
	// deleted-community writes.
	if !community.IsActive(ctx, s.storage, event.CommunityId) {
		logger.InfoContext(ctx, "skipping story generation — community is soft-deleted",
			"reason", "community_deleted",
		)
		return nil
	}

	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:
		s.handleTransferCompleted(ctx, evt)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED:
		s.handleRequestFulfilled(ctx, evt)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		s.handleExperienceCompleted(ctx, evt)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED:
		s.handleInvitationLinkUsed(ctx, evt)
	default:
		// Other event types don't trigger story generation.
	}
	return nil
}
