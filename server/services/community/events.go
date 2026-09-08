package community

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// ModelEventTypeToAPI converts a storage CommunityEventType to its API form
// by NAME. The two enums number their values differently past the first few —
// a numeric cast mislabels later types (storage REQUEST_FULFILLED=13 would
// surface as API REQUEST_OFFER_SELECTED=13). Storage-only types (undo
// retractions) have no API mirror and map to UNSPECIFIED; planning
// list changes DO mirror into the API so open rosters refresh live (#2724).
func ModelEventTypeToAPI(t models.CommunityEventType) api.CommunityEventType {
	if v, ok := api.CommunityEventType_value[t.String()]; ok {
		return api.CommunityEventType(v)
	}
	return api.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED
}

// ListCommunityEvents lists events for a community.
func (s *Service) ListCommunityEvents(
	ctx context.Context,
	req *connect.Request[api.ListCommunityEventsRequest],
) (*connect.Response[api.ListCommunityEventsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	logger.DebugContext(ctx, "listing events for community")

	// Single round-trip: fetch community + verify membership + reject deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Get all events for this community
	messages, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityEvent{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community events", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityEvents", err)
	}

	eventTypeSet := make(map[api.CommunityEventType]bool, len(req.Msg.EventTypes))
	for _, et := range req.Msg.EventTypes {
		eventTypeSet[et] = true
	}

	// A timestamp filter is capped at 7 days ago; with none supplied there is
	// no lower bound, which is the historical behavior of this RPC.
	var sinceUnixSec int64 = -1
	if req.Msg.SinceUnixSec != nil {
		sinceUnixSec = max(*req.Msg.SinceUnixSec, sevenDaysAgoUnixSec())
	}

	events := make([]*models.CommunityEvent, 0, len(messages))
	for _, msg := range messages {
		event := msg.(*models.CommunityEvent)
		if len(eventTypeSet) > 0 && !eventTypeSet[ModelEventTypeToAPI(event.EventType)] {
			continue
		}
		if sinceUnixSec >= 0 && event.OccurredAtUnixSec <= sinceUnixSec {
			continue
		}
		events = append(events, event)
	}

	items, err := s.denormalizeEvents(ctx, events, logger, "ListCommunityEvents")
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.ListCommunityEventsResponse{
		Events: items,
	}), nil
}

// communityEventToItem converts one stored CommunityEvent to an API
// CommunityEventItem with denormalized user and gear data. Used by the realtime
// paths — the bus fan-out (stream_subscriber.go) and the per-user stream's
// catch-up replay (user_streaming.go) — which each hold a single event and have
// no batch to amortize over.
//
// The listings use denormalizeEvents (user_events.go) instead, which batches the
// same lookups across a slice. The two differ in one respect worth knowing
// about: denormalizeEvents substitutes a FormerMember placeholder for a deleted
// actor, while this one leaves the field unset. A streamed event is an
// invalidation signal the client re-fetches behind, so the missing placeholder
// costs nothing there — but keep the two in step if that stops being true.
func (s *Service) communityEventToItem(ctx context.Context, event *models.CommunityEvent) (*api.CommunityEventItem, error) {
	item := &api.CommunityEventItem{
		Id:                event.Id,
		CommunityId:       event.CommunityId,
		EventType:         ModelEventTypeToAPI(event.EventType),
		GearId:            event.GearId,
		OccurredAtUnixSec: event.OccurredAtUnixSec,
	}

	// Extract transfer_id from topic oneof.
	if topic, ok := event.Topic.(*models.CommunityEvent_TransferId); ok {
		item.TransferId = topic.TransferId
	}

	// Denormalize actor.
	if event.ActorId != "" {
		actor, err := services.FetchAPIUser(ctx, s.storage, event.ActorId)
		if err == nil {
			item.Actor = actor
		}
	}

	// Denormalize object user.
	if event.ObjectUserId != "" {
		objectUser, err := services.FetchAPIUser(ctx, s.storage, event.ObjectUserId)
		if err == nil {
			item.ObjectUser = objectUser
		}
	}

	// Denormalize gear name.
	if event.GearId != "" {
		gear := &models.Gear{}
		if err := s.storage.GetByID(ctx, event.GearId, gear); err == nil {
			item.GearName = gear.Name
		}
	}

	return item, nil
}
