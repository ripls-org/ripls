package community

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// FiresForDeletedCommunity reports whether an event is one of the rare cases
// where notification dispatch must NOT short-circuit on the IsActive guard.
// COMMUNITY_DELETED is the canonical case: by the time the goroutine runs,
// the community is already soft-deleted, but the §8 notification copy is
// exactly what tells members the delete just happened.
//
// Future additions in #1659 (day-before-purge reminder, etc.) extend this
// list; COMMUNITY_RESTORED does NOT need an entry because RestoreCommunity
// clears Community.Deleted before emitting the event, so IsActive returns
// true naturally.
func FiresForDeletedCommunity(eventType models.CommunityEventType) bool {
	switch eventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED:
		return true
	default:
		return false
	}
}

// CategoryFor maps a community event type to the notification category that
// gates its delivery. Returns NOTIFICATION_CATEGORY_UNSPECIFIED for events
// that are not user-toggleable (those still pass through unfiltered).
func CategoryFor(eventType models.CommunityEventType) models.NotificationCategory {
	switch eventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_REQUESTS
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_EXPERIENCES
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_GEAR_SHARED
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_TRANSFER_UPDATES
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_REQUEST_UPDATES
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_PLANNING_UPDATES
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_MEMBERS
	// ITEM_SHARED_WITH_USER deliberately has no category, so it is not
	// toggleable. The categories gate community broadcasts — volume someone
	// may not want. A direct share is one person handing something to one
	// named person, closer to a message than to a feed item, and silently
	// dropping it would leave the sharer believing it landed (#3106).
	default:
		return models.NotificationCategory_NOTIFICATION_CATEGORY_UNSPECIFIED
	}
}

// CategoryEnabled returns true if the given category is enabled for the
// caller given a (possibly nil) preferences row. The semantic is "unset =
// on": missing rows and missing fields both resolve to true.
func CategoryEnabled(prefs *models.CommunityNotificationPreferences, category models.NotificationCategory) bool {
	if prefs == nil {
		return true
	}
	switch category {
	case models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_REQUESTS:
		return prefs.NotifyNewRequests == nil || *prefs.NotifyNewRequests
	case models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_EXPERIENCES:
		return prefs.NotifyNewExperiences == nil || *prefs.NotifyNewExperiences
	case models.NotificationCategory_NOTIFICATION_CATEGORY_GEAR_SHARED:
		return prefs.NotifyGearShared == nil || *prefs.NotifyGearShared
	case models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_COMPLETED:
		return prefs.NotifyExperienceCompleted == nil || *prefs.NotifyExperienceCompleted
	case models.NotificationCategory_NOTIFICATION_CATEGORY_TRANSFER_UPDATES:
		return prefs.NotifyTransferUpdates == nil || *prefs.NotifyTransferUpdates
	case models.NotificationCategory_NOTIFICATION_CATEGORY_REQUEST_UPDATES:
		return prefs.NotifyRequestUpdates == nil || *prefs.NotifyRequestUpdates
	case models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS:
		return prefs.NotifyExperienceRsvps == nil || *prefs.NotifyExperienceRsvps
	case models.NotificationCategory_NOTIFICATION_CATEGORY_PLANNING_UPDATES:
		return prefs.NotifyPlanningUpdates == nil || *prefs.NotifyPlanningUpdates
	case models.NotificationCategory_NOTIFICATION_CATEGORY_CHATS:
		return prefs.NotifyChats == nil || *prefs.NotifyChats
	case models.NotificationCategory_NOTIFICATION_CATEGORY_NEW_MEMBERS:
		return prefs.NotifyNewMembers == nil || *prefs.NotifyNewMembers
	case models.NotificationCategory_NOTIFICATION_CATEGORY_EVENT_REMINDERS:
		return prefs.NotifyEventReminders == nil || *prefs.NotifyEventReminders
	default:
		return true
	}
}

// IsActive reports whether the given community is in an active (not
// soft-deleted) state. The single chokepoint helper for every community-
// deletion-aware decision: notification dispatch (#1622), async write
// guards (#1623), and any future code path that asks "should I do this
// thing scoped to this community?".
//
// Fail mode is open: a transient lookup failure logs a warning and
// returns true. Losing real user-facing work (a missed push, a dropped
// async write) to a transient DB hiccup is worse than the rare case of
// the action proceeding against a community that's about to be deleted.
// The day-30 purge job (#1620) is the safety net for the residual race
// when something slips through against a just-deleted community.
//
// Callers decide what to do with the answer and log the action taken
// (skipping a push, dropping an async write, etc.) so this helper stays
// caller-agnostic.
//
// The day-before-purge reminder ships in #1619 §8 on a separate code path
// that does NOT call this helper — it deliberately fires *for* deleted
// communities. COMMUNITY_RESTORED works naturally because RestoreCommunity
// clears Community.Deleted before dispatching, so the lookup sees an
// active community.
func IsActive(ctx context.Context, s *storage.ProtoSQLStorage, communityID string) bool {
	community := &models.Community{}
	if err := s.GetByID(ctx, communityID, community, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"community active-state lookup failed; failing open",
			"operation", "IsActive",
			"community_id", communityID,
			"error", err,
		)
		return true
	}
	return !(community.Deleted != nil && community.Deleted.DeletedAtUnixSec > 0)
}

// FetchPreferencesForUsers reads preference rows for the given users in a
// community in one batched query. Missing users get a nil entry, which
// CategoryEnabled treats as "all on". On storage error returns nil and an
// error so callers can decide whether to fail open or propagate; the
// notification path is expected to fail open (log + proceed).
func FetchPreferencesForUsers(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityID string,
	userIDs []string,
) (map[string]*models.CommunityNotificationPreferences, error) {
	result := make(map[string]*models.CommunityNotificationPreferences, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	rows, err := s.QueryByFieldIn(ctx, "user_id", userIDs, &models.CommunityNotificationPreferences{})
	if err != nil {
		return nil, fmt.Errorf("query notification preferences: %w", err)
	}

	for _, row := range rows {
		prefs := row.(*models.CommunityNotificationPreferences)
		if prefs.CommunityId != communityID {
			continue
		}
		result[prefs.UserId] = prefs
	}
	return result, nil
}

// GetCommunityMemberIDs retrieves all member user IDs for a community.
// Returns nil on query failure (after logging a warning) or when communityID is empty.
func GetCommunityMemberIDs(ctx context.Context, s *storage.ProtoSQLStorage, communityID string) []string {
	if communityID == "" {
		return nil
	}

	members, err := s.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{})
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "failed to get community members", "community_id", communityID, "error", err)
		return nil
	}

	var memberIDs []string
	for _, m := range members {
		member := m.(*models.CommunityUser)
		memberIDs = append(memberIDs, member.UserId)
	}

	return memberIDs
}
