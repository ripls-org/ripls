package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// sharedWithUserEvents returns the direct-share events raised in a community.
func sharedWithUserEvents(t *testing.T, st *storage.ProtoSQLStorage, communityID string) []*models.CommunityEvent {
	t.Helper()
	rows, err := st.QueryByField(context.Background(), "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query community events: %v", err)
	}
	var out []*models.CommunityEvent
	for _, r := range rows {
		e := r.(*models.CommunityEvent)
		if e.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER {
			out = append(out, e)
		}
	}
	return out
}

// Handing an item to someone already on the platform has to tell them. It used
// to add the membership row in silence: off-app invitees got an email, link
// joiners raised INVITATION_LINK_USED, and existing users got nothing (#3106).
func TestShareItem_NotifiesTheMemberItWasSharedWith(t *testing.T) {
	svc, st, _, _, hostID, expID, ctx := setupShareItemTest(t)
	memberID := setupTestUser(t, st, "member@example.com", "Member")

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     expTarget(expID),
		Invitees: []*api.Invitee{memberInvitee(memberID)},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	communityID := *resp.Msg.AdhocCommunityId

	events := sharedWithUserEvents(t, st, communityID)
	if len(events) != 1 {
		t.Fatalf("raised %d direct-share events, want 1", len(events))
	}
	event := events[0]
	if event.ObjectUserId != memberID {
		t.Errorf("object_user_id = %q, want the person it was shared with (%q)", event.ObjectUserId, memberID)
	}
	if event.ActorId != hostID {
		t.Errorf("actor_id = %q, want the sharer (%q)", event.ActorId, hostID)
	}
	// The topic names what was handed over, so the copy can say which event.
	if event.GetExperienceId() != expID {
		t.Errorf("experience_id = %q, want %q", event.GetExperienceId(), expID)
	}
}

// Widening an item's audience must not re-notify the people who already had it.
func TestShareItem_DoesNotRenotifyExistingMembers(t *testing.T) {
	svc, st, _, _, _, expID, ctx := setupShareItemTest(t)
	firstID := setupTestUser(t, st, "first@example.com", "First")
	secondID := setupTestUser(t, st, "second@example.com", "Second")

	share := func(memberIDs ...string) string {
		t.Helper()
		invitees := make([]*api.Invitee, 0, len(memberIDs))
		for _, id := range memberIDs {
			invitees = append(invitees, memberInvitee(id))
		}
		resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
			Item:     expTarget(expID),
			Invitees: invitees,
		}))
		if err != nil {
			t.Fatalf("ShareItem: %v", err)
		}
		return *resp.Msg.AdhocCommunityId
	}

	communityID := share(firstID)
	if got := len(sharedWithUserEvents(t, st, communityID)); got != 1 {
		t.Fatalf("first share raised %d events, want 1", got)
	}

	// Re-sharing with the same person changes nothing: they are already a member.
	share(firstID)
	if got := len(sharedWithUserEvents(t, st, communityID)); got != 1 {
		t.Errorf("re-sharing with an existing member raised %d events in total, want 1", got)
	}

	// Adding someone new tells only them.
	share(firstID, secondID)
	events := sharedWithUserEvents(t, st, communityID)
	if len(events) != 2 {
		t.Fatalf("after adding a second person there are %d events, want 2", len(events))
	}
	recipients := map[string]int{}
	for _, e := range events {
		recipients[e.ObjectUserId]++
	}
	if recipients[firstID] != 1 {
		t.Errorf("the first member was told %d times, want 1", recipients[firstID])
	}
	if recipients[secondID] != 1 {
		t.Errorf("the second member was told %d times, want 1", recipients[secondID])
	}
}

// The copy the recipient reads is chosen by what was handed over, and each kind
// reaches the event through a different field: gear rides gear_id, experience
// and request the topic oneof. A kind that lands in the wrong field renders a
// blank sentence, so every kind is pinned here — the reported bug arrived by
// the gear path (#3106).
func TestShareItem_NamesWhatWasSharedForEveryItemKind(t *testing.T) {
	tests := []struct {
		name string
		kind ItemKind
		item func(t *testing.T, st *storage.ProtoSQLStorage, ctx context.Context, hostID string) (string, func(string) *api.ShareItemRequest)
		// share writes the join row ShareItem's link step then verifies.
		share func(ctx context.Context, st *storage.ProtoSQLStorage, itemID, communityID string) error
		topic func(*models.CommunityEvent) string
	}{
		{
			name: "gear",
			kind: ItemKindGear,
			item: func(t *testing.T, st *storage.ProtoSQLStorage, ctx context.Context, hostID string) (string, func(string) *api.ShareItemRequest) {
				t.Helper()
				id, err := st.Insert(ctx, &models.Gear{
					Name:    "Drill",
					OwnerId: hostID,
					State:   models.GearState_GEAR_STATE_AVAILABLE,
				})
				if err != nil {
					t.Fatalf("insert gear: %v", err)
				}
				return id, func(gearID string) *api.ShareItemRequest {
					return &api.ShareItemRequest{Item: &api.ShareItemRequest_GearId{GearId: gearID}}
				}
			},
			share: func(ctx context.Context, st *storage.ProtoSQLStorage, itemID, communityID string) error {
				_, err := st.Insert(ctx, &models.CommunityGear{
					CommunityId:      communityID,
					GearId:           itemID,
					CreatedAtUnixSec: 1,
				})
				return err
			},
			topic: func(e *models.CommunityEvent) string { return e.GearId },
		},
		{
			name: "request",
			kind: ItemKindRequest,
			item: func(t *testing.T, st *storage.ProtoSQLStorage, ctx context.Context, hostID string) (string, func(string) *api.ShareItemRequest) {
				t.Helper()
				id, err := st.Insert(ctx, &models.Request{
					RequesterId: hostID,
					Title:       "Need a ladder",
					State:       models.RequestState_REQUEST_STATE_ACTIVE,
				})
				if err != nil {
					t.Fatalf("insert request: %v", err)
				}
				return id, func(requestID string) *api.ShareItemRequest {
					return &api.ShareItemRequest{Item: &api.ShareItemRequest_RequestId{RequestId: requestID}}
				}
			},
			share: func(ctx context.Context, st *storage.ProtoSQLStorage, itemID, communityID string) error {
				_, err := st.Insert(ctx, &models.CommunityRequest{
					CommunityId:     communityID,
					RequestId:       itemID,
					SharedAtUnixSec: 1,
				})
				return err
			},
			topic: func(e *models.CommunityEvent) string { return e.GetRequestId() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, st, _, _, hostID, _, ctx := setupShareItemTest(t)
			memberID := setupTestUser(t, st, "member@example.com", "Member")

			itemID, newReq := tc.item(t, st, ctx, hostID)
			sharer := &fakeItemSharer{st: st, ownerID: hostID}
			svc.SetItemSharer(tc.kind, ItemSharer{
				VerifyOwner:  sharer.verifyOwner,
				VerifyViewer: sharer.verifyViewer,
				ShareToCommunity: func(ctx context.Context, itemID, communityID, _ string) error {
					return tc.share(ctx, st, itemID, communityID)
				},
			})

			req := newReq(itemID)
			req.Invitees = []*api.Invitee{memberInvitee(memberID)}
			resp, err := svc.ShareItem(ctx, connect.NewRequest(req))
			if err != nil {
				t.Fatalf("ShareItem: %v", err)
			}

			events := sharedWithUserEvents(t, st, *resp.Msg.AdhocCommunityId)
			if len(events) != 1 {
				t.Fatalf("raised %d direct-share events, want 1", len(events))
			}
			if got := tc.topic(events[0]); got != itemID {
				t.Errorf("event names item %q, want %q — the copy cannot say what was shared", got, itemID)
			}
			if events[0].ObjectUserId != memberID {
				t.Errorf("object_user_id = %q, want %q", events[0].ObjectUserId, memberID)
			}
		})
	}
}

// Sharing with yourself is not a notification.
func TestShareItem_DoesNotNotifyTheSharer(t *testing.T) {
	svc, st, _, _, hostID, expID, ctx := setupShareItemTest(t)

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     expTarget(expID),
		Invitees: []*api.Invitee{memberInvitee(hostID)},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	if got := len(sharedWithUserEvents(t, st, *resp.Msg.AdhocCommunityId)); got != 0 {
		t.Errorf("raised %d events for a share with oneself, want 0", got)
	}
}
