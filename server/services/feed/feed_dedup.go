package feed

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// eventGroup holds the most-recent event for a deduplicated feed item and the
// total count of all events sharing that item key.
type eventGroup struct {
	latestEvent *models.CommunityEvent
	totalCount  int
}

// deduplicateEventsByItemKey groups events by item key and returns only the
// most-recent event per key.  Events with no recognised item key are dropped.
func deduplicateEventsByItemKey(events []*models.CommunityEvent) map[string]*eventGroup {
	deduped := make(map[string]*eventGroup, len(events))
	for _, event := range events {
		key := itemKeyForEvent(event)
		if key == "" {
			continue
		}
		if existing, ok := deduped[key]; !ok {
			deduped[key] = &eventGroup{latestEvent: event, totalCount: 1}
		} else {
			existing.totalCount++
			if event.OccurredAtUnixSec > existing.latestEvent.OccurredAtUnixSec {
				existing.latestEvent = event
			}
		}
	}
	return deduped
}

// itemKeyForEvent returns a stable item key for deduplication.
// All events about the same item (gear, request, experience, community) share
// the same key so only the most recent event per item appears in the feed.
func itemKeyForEvent(event *models.CommunityEvent) string {
	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		return "gear:" + event.GearId
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:
		return "request:" + event.GetRequestId()
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO:
		return "experience:" + event.GetExperienceId()
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED:
		return "community:" + event.CommunityId
	default:
		return ""
	}
}

// feedItemEntityKey returns a stable entity-scoped key for cross-community
// deduplication. This mirrors itemKeyForEvent but operates on the assembled
// FeedItem payload rather than the raw CommunityEvent, so it works after
// generateFeedItems has already deduplicated within each community.
// Nudges and stories without an entity ID fall back to item.Id, which is
// already unique across communities.
func feedItemEntityKey(item *api.FeedItem) string {
	switch p := item.Payload.(type) {
	case *api.FeedItem_GearShared:
		return "gear:" + p.GearShared.GearId
	case *api.FeedItem_RequestCreated:
		return "request:" + p.RequestCreated.RequestId
	case *api.FeedItem_ExperienceCreated:
		return "experience:" + p.ExperienceCreated.ExperienceId
	case *api.FeedItem_CommunityCreated:
		return "community:" + p.CommunityCreated.CommunityId
	case *api.FeedItem_Story:
		// Stories fan out per-community at creation time — a single
		// fulfillment / completion event generates one story row per
		// community the parent item was shared to. Dedupe by
		// (story_type, entity_id) so the user sees the story once,
		// regardless of how many communities surfaced it. Story types
		// like NEW_MEMBER_WELCOME carry no entity ID and fall through
		// to item.Id, preserving genuinely distinct per-community
		// welcomes for a multi-community joiner.
		s := p.Story
		typeKey := s.GetStoryType().String()
		switch {
		case s.GetGearId() != "":
			return "story:" + typeKey + ":gear:" + s.GetGearId()
		case s.GetRequestId() != "":
			return "story:" + typeKey + ":request:" + s.GetRequestId()
		case s.GetExperienceId() != "":
			return "story:" + typeKey + ":experience:" + s.GetExperienceId()
		case s.GetLoanId() != "":
			return "story:" + typeKey + ":loan:" + s.GetLoanId()
		}
		return item.Id
	default:
		return item.Id
	}
}
