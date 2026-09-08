package community_event_bus

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
)

// Publisher is the surface emit sites depend on. The interface is concrete
// (not parametric) so emitter constructors stay simple — they accept this
// interface only.
type Publisher interface {
	// Publish persists event as a storage row, pre-fetches denormalized
	// context referenced by the event, and dispatches to every subscriber
	// asynchronously. Returns the inserted event ID.
	//
	// Storage insert failures surface as the returned error; subscribers
	// are not invoked. Pre-fetch failures and subscriber failures do NOT
	// surface — they are logged and counted by the log-based metric
	// stream.
	Publish(ctx context.Context, event *models.CommunityEvent) (eventID string, err error)
}

// PublishedEvent is what subscribers receive. The bare *models.CommunityEvent
// is augmented with pre-fetched denormalized context so that each subscriber
// doesn't re-fetch the same gear/transfer/request/experience/actor — see G12
// in the v2 plan.
//
// Each pointer field is populated only when the event references the
// corresponding entity (e.g. Gear is populated only when event.GearId is
// non-empty). Subscribers MUST handle nil fields gracefully; pre-fetch
// failures leave the field nil and are logged at WARN.
type PublishedEvent struct {
	Event *models.CommunityEvent

	Gear       *models.Gear
	Transfer   *models.Transfer
	Request    *models.Request
	Experience *models.Experience
	Actor      *api.User

	// MemberIDsAtPublish pins the push audience for member-broadcast events
	// to the community's membership as of publish time. It is populated only
	// for the event types SnapshotsMembers reports true for — exactly those
	// whose push audience is "all community members minus the actor".
	// Resolving that audience against live membership at dispatch time races
	// joins that land between publish and the async dispatch, pushing events
	// to users whose membership postdates them (#2657).
	//
	// Nil when the event type is not snapshotted or the snapshot read failed;
	// consumers must then fall back to a live membership lookup. Non-nil
	// (possibly empty) on a successful snapshot.
	MemberIDsAtPublish []string
}

// Subscriber is the per-event handler shape. Type alias keeps the underlying
// pubsub interface in scope for callers that want to register, while letting
// subscribers be referred to by the domain-flavored name.
type Subscriber = pubsub.Subscriber[*PublishedEvent]

// Topic is a type alias to the underlying parametric topic. Useful for tests
// that want to construct a MemTopic[*PublishedEvent] directly.
type Topic = pubsub.Topic[*PublishedEvent]

// LogContext implements pubsub.LogContextProvider so dispatch log lines
// automatically include the canonical CommunityEvent identifiers.
//
// Field names match docs/server/observability.md § "Standard Field Names":
//   - community_event_id (the row id; NOT event_id, which collides with the
//     HTTP request_id reservation).
//   - community_id.
//   - event_type (the enum value, e.g. COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE).
func (p *PublishedEvent) LogContext() []any {
	if p == nil || p.Event == nil {
		return nil
	}
	return []any{
		"community_event_id", p.Event.Id,
		"community_id", p.Event.CommunityId,
		"event_type", p.Event.EventType.String(),
	}
}
