package community_event_bus

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// prefetchEntities hydrates the denormalized fields of pe based on which
// fields are populated on pe.Event. Each fetch failure logs at WARN and
// leaves the corresponding field nil — subscribers must tolerate nil
// entities. Returning an error here would be wrong; the audit row was
// already inserted, and a transient storage hiccup on a denorm read should
// not abort dispatch.
//
// Reads are issued sequentially. We could fan them out via goroutines, but
// today's notification dispatcher does the same reads sequentially and the
// total event set per RPC is small (≤ 5 entities). If a future profile
// shows this matters, switch to a small concurrency primitive.
func prefetchEntities(ctx context.Context, s *storage.ProtoSQLStorage, pe *PublishedEvent) {
	if pe == nil || pe.Event == nil || s == nil {
		return
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "community_event_bus.prefetch",
		"community_event_id", pe.Event.Id,
		"event_type", pe.Event.EventType.String(),
	)

	if pe.Event.GearId != "" {
		gear := &models.Gear{}
		if err := s.GetByID(ctx, pe.Event.GearId, gear); err != nil {
			logger.WarnContext(ctx, "prefetch gear failed", "gear_id", pe.Event.GearId, "error", err)
		} else {
			pe.Gear = gear
		}
	}

	if id := pe.Event.GetTransferId(); id != "" {
		transfer := &models.Transfer{}
		if err := s.GetByID(ctx, id, transfer); err != nil {
			logger.WarnContext(ctx, "prefetch transfer failed", "transfer_id", id, "error", err)
		} else {
			pe.Transfer = transfer
		}
	}

	if id := pe.Event.GetRequestId(); id != "" {
		request := &models.Request{}
		if err := s.GetByID(ctx, id, request); err != nil {
			logger.WarnContext(ctx, "prefetch request failed", "target_request_id", id, "error", err)
		} else {
			pe.Request = request
		}
	}

	if id := pe.Event.GetExperienceId(); id != "" {
		experience := &models.Experience{}
		if err := s.GetByID(ctx, id, experience); err != nil {
			logger.WarnContext(ctx, "prefetch experience failed", "experience_id", id, "error", err)
		} else {
			pe.Experience = experience
		}
	}

	if pe.Event.ActorId != "" {
		actor, err := services.FetchAPIUser(ctx, s, pe.Event.ActorId)
		if err != nil {
			logger.WarnContext(ctx, "prefetch actor failed", "actor_user_id", pe.Event.ActorId, "error", err)
		} else {
			pe.Actor = actor
		}
	}
}

// SnapshotsMembers reports whether events of this type carry a publish-time
// member snapshot (PublishedEvent.MemberIDsAtPublish). These are exactly the
// types whose push audience is the community's full member list minus the
// actor — see the member-broadcast cases in
// server/notifications/community_subscriber/recipients.go, which consume the
// snapshot. Snapshotting at publish pins that audience to membership as of
// the event, so a user who joins between publish and the async dispatch is
// not notified about an event that predates their membership (#2657).
//
// COMMUNITY_DELETED is deliberately absent: its audience comes from the
// community's deleted_snapshot, which is already a point-in-time capture.
func SnapshotsMembers(eventType models.CommunityEventType) bool {
	switch eventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED:
		return true
	default:
		return false
	}
}

// snapshotMembers captures the community's current member user IDs onto
// pe.MemberIDsAtPublish for the SnapshotsMembers event types. Publishers
// write their own membership changes before calling Publish, so the snapshot
// reflects the acting request's writes (an invitation-accept's new member is
// present on its own INVITATION_LINK_USED event). A query failure logs at
// WARN and leaves the field nil — subscribers fall back to a live lookup,
// degrading to pre-snapshot behavior rather than dropping the fan-out.
// (server/community.GetCommunityMemberIDs is the same query; it can't be
// reused here because server/community imports this package.)
func snapshotMembers(ctx context.Context, s *storage.ProtoSQLStorage, pe *PublishedEvent) {
	if pe == nil || pe.Event == nil || s == nil {
		return
	}
	if !SnapshotsMembers(pe.Event.EventType) || pe.Event.CommunityId == "" {
		return
	}
	rows, err := s.QueryByField(ctx, "community_id", pe.Event.CommunityId, &models.CommunityUser{})
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"member snapshot failed; dispatch will fall back to live membership",
			"operation", "community_event_bus.snapshot_members",
			"community_event_id", pe.Event.Id,
			"community_id", pe.Event.CommunityId,
			"event_type", pe.Event.EventType.String(),
			"error", err,
		)
		return
	}
	memberIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		memberIDs = append(memberIDs, row.(*models.CommunityUser).UserId)
	}
	pe.MemberIDsAtPublish = memberIDs
}
