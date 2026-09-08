// Portfolio-wide event listing — the poll backstop for the per-user realtime
// stream (#2867).
//
// The per-community backstop (ListCommunityEvents) had the client run one poll
// timer per community, so a portfolio of 50 cost 50 requests every interval and
// the last community's timer did not even start for two minutes. This RPC
// answers the same question once, for every community at once.

package community

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// userEventsLimit bounds one poll response. Deliberately larger than a normal
// interval's worth of activity, so hitting it means the client has been away
// long enough that a full refresh is the right recovery anyway.
const userEventsLimit = 500

// eventBacklogWindow is how far back any catch-up may reach. Events are
// invalidation signals rather than state to reconstruct, so replaying older
// ones buys a dormant client nothing while costing everyone the query.
const eventBacklogWindow = 7 * 24 * time.Hour

// sevenDaysAgoUnixSec is the floor every since_unix_sec is clamped to, shared
// by the two listings and the stream catch-up so all three agree on the
// horizon.
func sevenDaysAgoUnixSec() int64 {
	return time.Now().Add(-eventBacklogWindow).Unix()
}

// ListUserEvents returns recent events across every community the caller
// belongs to. Membership is resolved inside the storage query, so the cost does
// not scale with portfolio size.
func (s *Service) ListUserEvents(
	ctx context.Context,
	req *connect.Request[api.ListUserEventsRequest],
) (*connect.Response[api.ListUserEventsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"operation", "ListUserEvents",
	)

	// Cap at 7 days ago so a dormant client cannot request an unbounded
	// backlog. A client that sends no timestamp is asking for "whatever is
	// recent", which is the same cap.
	sinceUnixSec := sevenDaysAgoUnixSec()
	if req.Msg.SinceUnixSec != nil {
		sinceUnixSec = max(*req.Msg.SinceUnixSec, sinceUnixSec)
	}

	events, err := s.storage.FindEventsForUserSince(ctx, authInfo.UserID, sinceUnixSec, userEventsLimit)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query events for user", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserEvents", err)
	}
	if len(events) == userEventsLimit {
		logger.InfoContext(ctx, "user event poll truncated at limit",
			"limit", userEventsLimit,
			"since_unix_sec", sinceUnixSec,
		)
	}

	items, err := s.denormalizeEvents(ctx, events, logger, "ListUserEvents")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.ListUserEventsResponse{Events: items}), nil
}

// denormalizeEvents converts stored events to API items, batch-fetching every
// referenced user and gear once rather than per event.
//
// Shared by ListCommunityEvents and ListUserEvents: the two differ only in how
// they select rows, and duplicating ~80 lines of former-member placeholder and
// topic-oneof handling between them is how the two drift apart. operation names
// the calling RPC so the placeholder warnings stay attributable.
func (s *Service) denormalizeEvents(
	ctx context.Context,
	events []*models.CommunityEvent,
	logger *logging.Logger,
	operation string,
) ([]*api.CommunityEventItem, error) {
	// Collect unique user and gear IDs across all events for batch fetching.
	var userIDs []string
	var gearIDs []string
	seenUser := make(map[string]struct{})
	seenGear := make(map[string]struct{})
	for _, event := range events {
		for _, id := range []string{event.ActorId, event.ObjectUserId} {
			if id == "" {
				continue
			}
			if _, ok := seenUser[id]; !ok {
				seenUser[id] = struct{}{}
				userIDs = append(userIDs, id)
			}
		}
		if event.GearId != "" {
			if _, ok := seenGear[event.GearId]; !ok {
				seenGear[event.GearId] = struct{}{}
				gearIDs = append(gearIDs, event.GearId)
			}
		}
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch users for events", "error", err)
		return nil, connecterr.Internal(ctx, operation, err, "detail", "failed to batch fetch users")
	}

	gearMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch gear for events", "error", err)
		return nil, connecterr.Internal(ctx, operation, err, "detail", "failed to batch fetch gear")
	}

	items := make([]*api.CommunityEventItem, 0, len(events))
	for _, event := range events {
		item := &api.CommunityEventItem{
			Id:                event.Id,
			CommunityId:       event.CommunityId,
			EventType:         ModelEventTypeToAPI(event.EventType),
			GearId:            event.GearId,
			OccurredAtUnixSec: event.OccurredAtUnixSec,
		}

		if topic, ok := event.Topic.(*models.CommunityEvent_TransferId); ok {
			item.TransferId = topic.TransferId
		}

		// Deleted actors surface as a FormerMember placeholder so the feed
		// stays usable rather than dropping the row.
		if event.ActorId != "" {
			actor := services.ResolveUserOrFormer(userMap, event.ActorId)
			if actor.FormerMember {
				logger.WarnContext(ctx, "actor is no longer available; using former-member placeholder",
					"operation", operation,
					"actor_id", event.ActorId,
					"community_id", event.CommunityId,
					"community_event_id", event.Id,
					"event_type", event.EventType.String(),
				)
			}
			item.Actor = actor
		}

		if event.ObjectUserId != "" {
			objectUser := services.ResolveUserOrFormer(userMap, event.ObjectUserId)
			if objectUser.FormerMember {
				logger.WarnContext(ctx, "object user is no longer available; using former-member placeholder",
					"operation", operation,
					"object_user_id", event.ObjectUserId,
					"community_id", event.CommunityId,
					"community_event_id", event.Id,
					"event_type", event.EventType.String(),
				)
			}
			item.ObjectUser = objectUser
		}

		// Absent gear is lenient: the name is left empty rather than failing
		// the whole listing.
		if gear, ok := gearMap[event.GearId]; ok {
			item.GearName = gear.Name
		}

		items = append(items, item)
	}
	return items, nil
}
