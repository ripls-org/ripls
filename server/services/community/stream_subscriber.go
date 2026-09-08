package community

import (
	"context"

	communitylib "go.ripls.org/ripls/server/community"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/logging"
)

// StreamSubscriberName is the stable identifier used by pubsub log lines for
// the stream broadcast subscriber.
const StreamSubscriberName = "community_stream"

// StreamSubscriber returns a community_event_bus.Subscriber that delivers every
// published event to the per-user streams of the event's community members. It
// wraps the service's in-memory stream registry (userStreams /
// broadcastToUserStreams / HasActiveUserStream — see user_streaming.go) so the
// registry stays the single source of truth for active streams.
//
// The subscriber is co-located with the rest of the Service rather than in
// the notifications/community_subscriber package because it shares per-stream
// state with StreamUserEvents — moving it across a package boundary would
// require exposing the registry, which we don't want.
//
// Soft-delete behavior: NO gate. Streams self-clean when communities are
// soft-deleted via the existing membership cascade; broadcasting to no
// subscribers is a no-op. See server/notifications/community_subscriber for
// the contrasting subscriber that DOES gate.
func (s *Service) StreamSubscriber() cebus.Subscriber {
	return &streamSubscriber{svc: s}
}

// streamSubscriber implements pubsub.Subscriber[*cebus.PublishedEvent].
type streamSubscriber struct {
	svc *Service
}

func (ss *streamSubscriber) Name() string { return StreamSubscriberName }

func (ss *streamSubscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event

	item, err := ss.svc.communityEventToItem(ctx, event)
	if err != nil {
		// Match the pre-bus behavior: log and swallow. The event row is
		// already persisted; a denormalization failure should not abort
		// dispatch. Other subscribers continue to fire.
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to convert event for broadcast",
			"operation", "community_stream.handle",
			"community_event_id", event.Id,
			"error", err,
		)
		return nil
	}
	// A per-user stream names no community, so the audience has to be resolved
	// here. Membership is read live, per event, which is precisely what lets a
	// join or leave take effect with no client action and no reconnect — there
	// is no subscription list to keep in sync.
	//
	// The gate matters: without it every event would pay a membership query
	// even when nobody is streaming. With it, the query is only run when at
	// least one user stream is open, and the result is bounded by the 32-member
	// community cap.
	if ss.svc.hasAnyUserStream() {
		members := communitylib.GetCommunityMemberIDs(ctx, ss.svc.storage, event.CommunityId)
		ss.svc.broadcastToUserStreams(ctx, members, item)
	}
	return nil
}
