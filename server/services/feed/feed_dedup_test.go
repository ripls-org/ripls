package feed

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Note: itemKeyForEvent is tested in service_test.go::TestItemKeyForEvent.
// This file covers deduplicateEventsByItemKey and feedItemEntityKey.

// TestDeduplicateEventsByItemKey_LatestWins verifies that when two events share
// the same item key, the one with the larger OccurredAtUnixSec becomes the
// representative, and totalCount reflects both contributions.
func TestDeduplicateEventsByItemKey_LatestWins(t *testing.T) {
	gearID := "gear-xyz"
	older := &models.CommunityEvent{
		Id:                "evt-old",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		GearId:            gearID,
		OccurredAtUnixSec: 1000,
	}
	newer := &models.CommunityEvent{
		Id:                "evt-new",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		GearId:            gearID,
		OccurredAtUnixSec: 2000,
	}

	result := deduplicateEventsByItemKey([]*models.CommunityEvent{older, newer})

	key := "gear:" + gearID
	g, ok := result[key]
	if !ok {
		t.Fatalf("expected key %q in result", key)
	}
	if g.totalCount != 2 {
		t.Errorf("totalCount = %d, want 2", g.totalCount)
	}
	if g.latestEvent.Id != "evt-new" {
		t.Errorf("latestEvent.Id = %q, want %q", g.latestEvent.Id, "evt-new")
	}
}

// TestDeduplicateEventsByItemKey_DropsUnkeyedEvents verifies that events with no
// recognised item key (e.g. MEMBER_LEFT) are silently dropped from the result.
func TestDeduplicateEventsByItemKey_DropsUnkeyedEvents(t *testing.T) {
	event := &models.CommunityEvent{
		Id:                "evt-member-left",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT,
		OccurredAtUnixSec: 1000,
	}

	result := deduplicateEventsByItemKey([]*models.CommunityEvent{event})

	if len(result) != 0 {
		t.Errorf("expected empty result for unkeyed event, got %d entries", len(result))
	}
}

// TestDeduplicateEventsByItemKey_GroupsAcrossEventTypes verifies that events with
// different EventTypes but the same gear_id collapse into one group under the
// same "gear:<id>" key.
func TestDeduplicateEventsByItemKey_GroupsAcrossEventTypes(t *testing.T) {
	gearID := "gear-multi"
	events := []*models.CommunityEvent{
		{
			Id:                "evt-share",
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			GearId:            gearID,
			OccurredAtUnixSec: 500,
		},
		{
			Id:                "evt-interest",
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
			GearId:            gearID,
			OccurredAtUnixSec: 600,
		},
	}

	result := deduplicateEventsByItemKey(events)

	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	key := "gear:" + gearID
	g := result[key]
	if g.totalCount != 2 {
		t.Errorf("totalCount = %d, want 2", g.totalCount)
	}
	if g.latestEvent.Id != "evt-interest" {
		t.Errorf("latestEvent.Id = %q, want evt-interest", g.latestEvent.Id)
	}
}

// TestFeedItemEntityKey_AllPayloadVariants exercises feedItemEntityKey across
// every recognised payload type plus the fallback (Story, Nudge).
// The raw-event analogue is TestItemKeyForEvent in service_test.go.
func TestFeedItemEntityKey_AllPayloadVariants(t *testing.T) {
	tests := []struct {
		name    string
		item    *api.FeedItem
		wantKey string
	}{
		{
			name: "GearShared",
			item: &api.FeedItem{
				Id: "fallback-id",
				Payload: &api.FeedItem_GearShared{
					GearShared: &api.GearSharedPayload{GearId: "g1"},
				},
			},
			wantKey: "gear:g1",
		},
		{
			name: "RequestCreated",
			item: &api.FeedItem{
				Id: "fallback-id",
				Payload: &api.FeedItem_RequestCreated{
					RequestCreated: &api.RequestCreatedPayload{RequestId: "r1"},
				},
			},
			wantKey: "request:r1",
		},
		{
			name: "ExperienceCreated",
			item: &api.FeedItem{
				Id: "fallback-id",
				Payload: &api.FeedItem_ExperienceCreated{
					ExperienceCreated: &api.ExperienceCreatedPayload{ExperienceId: "x1"},
				},
			},
			wantKey: "experience:x1",
		},
		{
			name: "CommunityCreated",
			item: &api.FeedItem{
				Id: "fallback-id",
				Payload: &api.FeedItem_CommunityCreated{
					CommunityCreated: &api.CommunityCreatedPayload{CommunityId: "c1"},
				},
			},
			wantKey: "community:c1",
		},
		{
			name: "Story keyed by (type, request_id)",
			item: &api.FeedItem{
				Id: "story-id",
				Payload: &api.FeedItem_Story{
					Story: &api.StoryPayload{
						StoryType: api.StoryType_STORY_TYPE_REQUEST_FULFILLED,
						RequestId: "r1",
					},
				},
			},
			wantKey: "story:STORY_TYPE_REQUEST_FULFILLED:request:r1",
		},
		{
			name: "Story keyed by (type, loan_id)",
			item: &api.FeedItem{
				Id: "story-id",
				Payload: &api.FeedItem_Story{
					Story: &api.StoryPayload{
						StoryType: api.StoryType_STORY_TYPE_LOAN_COMPLETED,
						LoanId:    "l1",
					},
				},
			},
			wantKey: "story:STORY_TYPE_LOAN_COMPLETED:loan:l1",
		},
		{
			name: "Story keyed by (type, experience_id)",
			item: &api.FeedItem{
				Id: "story-id",
				Payload: &api.FeedItem_Story{
					Story: &api.StoryPayload{
						StoryType:    api.StoryType_STORY_TYPE_EXPERIENCE_CONCLUDED,
						ExperienceId: "x1",
					},
				},
			},
			wantKey: "story:STORY_TYPE_EXPERIENCE_CONCLUDED:experience:x1",
		},
		{
			name: "Story keyed by (type, gear_id)",
			item: &api.FeedItem{
				Id: "story-id",
				Payload: &api.FeedItem_Story{
					Story: &api.StoryPayload{
						StoryType: api.StoryType_STORY_TYPE_GIVEAWAY_COMPLETED,
						GearId:    "g1",
					},
				},
			},
			wantKey: "story:STORY_TYPE_GIVEAWAY_COMPLETED:gear:g1",
		},
		{
			name: "Story with no entity ID falls back to item.Id",
			item: &api.FeedItem{
				Id: "welcome-1",
				Payload: &api.FeedItem_Story{
					Story: &api.StoryPayload{
						StoryType: api.StoryType_STORY_TYPE_NEW_MEMBER_WELCOME,
					},
				},
			},
			wantKey: "welcome-1",
		},
		{
			name:    "Nudge fallback to item.Id",
			item:    &api.FeedItem{Id: "nudge-id", Payload: &api.FeedItem_Nudge{}},
			wantKey: "nudge-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := feedItemEntityKey(tc.item)
			if got != tc.wantKey {
				t.Errorf("feedItemEntityKey() = %q, want %q", got, tc.wantKey)
			}
		})
	}
}
