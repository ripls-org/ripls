package community_subscriber

import (
	"context"

	"go.ripls.org/ripls/server/community"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/storage"
)

// ShouldNotify is the global allow-list of event types that the
// notification pipeline knows how to route. Per-recipient gating happens
// later via the per-user CommunityNotificationPreferences.
//
// Returning false short-circuits the entire dispatch in Handle so we don't
// pay for a recipient lookup on event types that never fire push.
func ShouldNotify(eventType models.CommunityEventType) bool {
	switch eventType {
	// Targeted (per-party) events.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE:
		// REQUEST_FULFILLED intentionally does NOT notify here (product
		// decision — its pushes flow through the publish-time member
		// snapshot). REQUEST_OFFER_SELECTED gained its first emitter with
		// gear-backed offers (#2702): the chosen helper hears "your offer
		// was picked".
		return true
	// Broadcast events (full community minus actor). These are gated per
	// member via the relevant NotificationCategory toggle.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED:
		return true
	// Lifecycle pings targeted at Yes/Maybe RSVPs. Completion is one of these:
	// only people who said they were coming need to know the event wrapped.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		return true
	// Planning events: targeted at experience RSVPs.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED:
		return true
	// Membership events: broadcast to all members minus the new joiner.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED:
		return true
	// A direct share: the one person it was shared with.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER:
		return true
	// Community lifecycle: broadcast to the deleted_snapshot members.
	// Fires AFTER the soft-delete write — see community.FiresForDeletedCommunity.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED:
		return true
	// Community restored: broadcast to the current member set.
	// RestoreCommunity clears Community.Deleted before emitting,
	// so the IsActive guard sees an active community naturally —
	// no firesForDeletedCommunity exception needed.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED:
		return true
	// Ownership transferred (during owner-leave-with-handoff):
	// targeted at the new owner only. The community is active for
	// the duration of the handoff so no firesForDeletedCommunity
	// entry is needed.
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED:
		return true
	default:
		return false
	}
}

// RecipientAudienceLabel returns a short human description of who receives a
// notification of this type — the audience getNotificationRecipients resolves to
// actual user IDs. It's the descriptive companion to ShouldNotify, used by the
// notification_examples review page so each row is labeled with its audience.
// Returns "" for types that fire no notification (ShouldNotify == false).
//
// Keep the case groups in sync with getNotificationRecipients below.
func RecipientAudienceLabel(eventType models.CommunityEventType) string {
	switch eventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN:
		return "Gear owner"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED:
		return "Selected recipient"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		return "The other party (owner or recipient)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN:
		return "Request creator"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED:
		return "Chosen helper"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED:
		return "All offerers (except the actor)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE:
		return "Event host"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		return "Yes/Maybe RSVPs (except the actor)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED:
		return "All community members (except the actor)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER:
		return "The person it was shared with"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED:
		return "Members at delete time (except the deleter)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED:
		return "Current members (except the restorer)"
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED:
		return "New owner"
	default:
		return ""
	}
}

// resolveRecipients returns the push audience for a published event. For the
// event types the bus snapshots (cebus.SnapshotsMembers — the member-broadcast
// types), the audience is the publish-time member capture minus the actor:
// resolving against live membership at dispatch time instead would race
// membership writes that land between publish and this async dispatch,
// pushing events to users whose membership postdates them (#2657). Every
// other type — and a snapshotted type whose capture failed, which falls back
// with a WARN — resolves live via getNotificationRecipients.
func resolveRecipients(ctx context.Context, s *storage.ProtoSQLStorage, evt *cebus.PublishedEvent) []string {
	event := evt.Event
	if evt.MemberIDsAtPublish != nil {
		recipients := make([]string, 0, len(evt.MemberIDsAtPublish))
		for _, id := range evt.MemberIDsAtPublish {
			if id != event.ActorId {
				recipients = append(recipients, id)
			}
		}
		return recipients
	}
	if cebus.SnapshotsMembers(event.EventType) {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"no publish-time member snapshot; resolving broadcast recipients from live membership",
			"operation", "community_notifications.resolve_recipients",
			"community_event_id", event.Id,
			"community_id", event.CommunityId,
			"event_type", event.EventType.String(),
		)
	}
	return getNotificationRecipients(ctx, s, event)
}

// getNotificationRecipients returns the targeted recipients for a notification
// by resolving them from storage at dispatch time. Only users directly
// involved in the transaction are notified, not all community members. For
// the member-broadcast types this live lookup is the fallback path only —
// resolveRecipients prefers the publish-time snapshot when the envelope
// carries one.
func getNotificationRecipients(ctx context.Context, s *storage.ProtoSQLStorage, event *models.CommunityEvent) []string {
	logger := logging.LoggerWithContext(ctx).With(
		"event_type", event.EventType,
		"actor_id", event.ActorId,
	)

	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN:
		// Notify gear owner only — they need to know someone wants
		// (or no longer wants) their gear.
		ownerID := getGearOwnerID(ctx, s, event.GearId)
		if ownerID == "" {
			logger.WarnContext(ctx, "could not find gear owner for notification", "gear_id", event.GearId)
			return nil
		}
		return []string{ownerID}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED:
		// Notify the approved recipient only — they need to know they
		// were selected. Look up recipient from the transfer.
		transfer := &models.Transfer{}
		if err := s.GetByID(ctx, event.GetTransferId(), transfer); err != nil {
			logger.WarnContext(ctx, "could not find transfer recipient for notification", "transfer_id", event.GetTransferId())
			return nil
		}
		// A transfer born as a gear-backed request offer (#2702) starts in
		// RECIPIENT_SELECTED with the requester as its recipient; the claim
		// that preceded it already pushed REQUEST_OFFER_MADE to them, and a
		// "your request was approved" push here would be wrong and duplicate.
		if transfer.GetOriginRequestId() != "" {
			return nil
		}
		if transfer.RecipientId == "" {
			return nil
		}
		return []string{transfer.RecipientId}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		// Notify the other party — they need to know about the cancellation,
		// pickup proposal, or that the transfer is now active.
		transfer := &models.Transfer{}
		if err := s.GetByID(ctx, event.GetTransferId(), transfer); err != nil {
			logger.WarnContext(ctx, "could not find transfer for notification", "transfer_id", event.GetTransferId())
			return nil
		}
		if event.ActorId == transfer.OwnerId {
			return []string{transfer.RecipientId}
		}
		return []string{transfer.OwnerId}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:
		// Notify request creator only — they need to know someone offered help.
		creatorID := getRequestCreatorID(ctx, s, event.GetRequestId())
		if creatorID == "" {
			logger.WarnContext(ctx, "could not find request creator for notification", "target_request_id", event.GetRequestId())
			return nil
		}
		return []string{creatorID}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED:
		// Notify all offerers except the actor — they should know the
		// request was fulfilled.
		offerers := getRequestOfferers(ctx, s, event.GetRequestId())
		if len(offerers) == 0 {
			return nil
		}
		recipients := make([]string, 0, len(offerers))
		for _, offererID := range offerers {
			if offererID != event.ActorId {
				recipients = append(recipients, offererID)
			}
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE:
		// Notify experience owner — they need to know someone RSVPed.
		// The owner is stored in ObjectUserId at emit time.
		if event.ObjectUserId == "" {
			logger.WarnContext(ctx, "no object_user_id for experience RSVP notification")
			return nil
		}
		return []string{event.ObjectUserId}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED:
		// Planning events: notify users who RSVP'd YES or MAYBE to the
		// experience (minus the actor). Spec: "events the user said yes
		// or maybe to."
		experienceID := event.GetExperienceId()
		if experienceID == "" {
			logger.WarnContext(ctx, "planning event missing experience_id")
			return nil
		}
		rsvps, err := s.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
		if err != nil {
			logger.WarnContext(ctx, "failed to load RSVPs for planning notification", "error", err)
			return nil
		}
		var recipients []string
		for _, msg := range rsvps {
			rsvp := msg.(*models.ExperienceRSVP)
			if rsvp.UserId == event.ActorId {
				continue
			}
			if !rsvpstate.IsGoing(rsvp.GetIntention()) {
				continue
			}
			recipients = append(recipients, rsvp.UserId)
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED:
		// Notify the offerer the requester chose. The offerer's user_id is
		// recorded in ObjectUserId at emit time.
		if event.ObjectUserId == "" {
			logger.WarnContext(ctx, "no object_user_id for offer-selected notification")
			return nil
		}
		return []string{event.ObjectUserId}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN:
		// Notify the request creator that an offerer withdrew.
		creatorID := getRequestCreatorID(ctx, s, event.GetRequestId())
		if creatorID == "" {
			logger.WarnContext(ctx, "could not find request creator for offer-withdrawn notification", "target_request_id", event.GetRequestId())
			return nil
		}
		return []string{creatorID}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED:
		// Notify all offerers (except the actor) that the request is gone.
		offerers := getRequestOfferers(ctx, s, event.GetRequestId())
		var recipients []string
		for _, id := range offerers {
			if id != event.ActorId {
				recipients = append(recipients, id)
			}
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		// Notify Yes/Maybe RSVPs minus the actor. Mirrors the planning-events
		// pattern above but scoped to lifecycle transitions. Completion is a
		// lifecycle ping like the rest — only the people who said they were
		// coming care that the event wrapped, not the whole community (the host
		// is the actor and is filtered out). The per-community story/recap card
		// is generated separately in CompleteExperience and is unaffected.
		experienceID := event.GetExperienceId()
		if experienceID == "" {
			logger.WarnContext(ctx, "lifecycle event missing experience_id")
			return nil
		}
		rsvps, err := s.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
		if err != nil {
			logger.WarnContext(ctx, "failed to load RSVPs for lifecycle notification", "error", err)
			return nil
		}
		var recipients []string
		for _, msg := range rsvps {
			rsvp := msg.(*models.ExperienceRSVP)
			if rsvp.UserId == event.ActorId {
				continue
			}
			if !rsvpstate.IsGoing(rsvp.GetIntention()) {
				continue
			}
			recipients = append(recipients, rsvp.UserId)
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER:
		// One recipient, named on the event: the person the item was handed
		// to. Not the member set — this fires as they are added, and the point
		// is to tell them, not everyone already there.
		if event.ObjectUserId == "" || event.ObjectUserId == event.ActorId {
			return nil
		}
		return []string{event.ObjectUserId}

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED:
		// Broadcast to all community members except the actor.
		memberIDs := community.GetCommunityMemberIDs(ctx, s, event.CommunityId)
		var recipients []string
		for _, id := range memberIDs {
			if id != event.ActorId {
				recipients = append(recipients, id)
			}
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED:
		// Recipients are the eligible-restorer set captured at delete time
		// (deleted_snapshot.member_user_ids), minus the deleter. The
		// community is already soft-deleted by the time this runs, so the
		// read uses IncludeDeleted: true.
		c := &models.Community{}
		if err := s.GetByID(ctx, event.CommunityId, c, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			logger.WarnContext(ctx, "failed to read deleted community for snapshot recipients", "error", err)
			return nil
		}
		if c.DeletedSnapshot == nil {
			logger.WarnContext(ctx, "community has no deleted_snapshot; cannot resolve recipients")
			return nil
		}
		var recipients []string
		for _, id := range c.DeletedSnapshot.MemberUserIds {
			if id != event.ActorId {
				recipients = append(recipients, id)
			}
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED:
		// Broadcast to the current member set minus the restorer.
		// #1654's cascade did NOT touch CommunityUser rows, so the
		// snapshot members are still active members after restore — the
		// simple member-list lookup gives the right set without any
		// snapshot-passthrough machinery.
		memberIDs := community.GetCommunityMemberIDs(ctx, s, event.CommunityId)
		var recipients []string
		for _, id := range memberIDs {
			if id != event.ActorId {
				recipients = append(recipients, id)
			}
		}
		return recipients

	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED:
		// New owner only — see §8 of the design doc. Other members can
		// read the event log if they care; we don't broadcast.
		if event.ObjectUserId == "" {
			logger.WarnContext(ctx, "ownership-transferred event missing object_user_id; cannot resolve recipient")
			return nil
		}
		return []string{event.ObjectUserId}

	default:
		return nil
	}
}

// getGearOwnerID retrieves the owner ID of a gear item.
func getGearOwnerID(ctx context.Context, s *storage.ProtoSQLStorage, gearID string) string {
	if gearID == "" {
		return ""
	}
	gear := &models.Gear{}
	if err := s.GetByID(ctx, gearID, gear); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get gear owner", "gear_id", gearID, "error", err)
		return ""
	}
	return gear.OwnerId
}

// getRequestCreatorID retrieves the creator ID of a request.
func getRequestCreatorID(ctx context.Context, s *storage.ProtoSQLStorage, requestID string) string {
	if requestID == "" {
		return ""
	}
	request := &models.Request{}
	if err := s.GetByID(ctx, requestID, request); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get request creator", "target_request_id", requestID, "error", err)
		return ""
	}
	return request.RequesterId
}

// getRequestOfferers retrieves all user IDs who have offered to fulfill a request.
func getRequestOfferers(ctx context.Context, s *storage.ProtoSQLStorage, requestID string) []string {
	if requestID == "" {
		return nil
	}
	offers, err := s.QueryByFields(ctx, map[string]any{
		"request_id": requestID,
		"withdrawn":  false,
	}, &models.RequestOffer{})
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to get request offers", "target_request_id", requestID, "error", err)
		return nil
	}
	offererIDsMap := make(map[string]bool)
	for _, offer := range offers {
		requestOffer := offer.(*models.RequestOffer)
		offererIDsMap[requestOffer.UserId] = true
	}
	offererIDs := make([]string, 0, len(offererIDsMap))
	for userID := range offererIDsMap {
		offererIDs = append(offererIDs, userID)
	}
	return offererIDs
}
