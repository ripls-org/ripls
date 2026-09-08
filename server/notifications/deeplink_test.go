package notifications

import (
	"context"
	"errors"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// communityEventNotification builds a community-event notification carrying the
// given (possibly empty) entity ids. experience_id and request_id are optional
// proto fields (pointers); gear_id and community_id are plain strings.
func communityEventNotification(communityID, experienceID, gearID, requestID string) *models.Notification {
	ev := &models.CommunityEventPayload{CommunityId: communityID, GearId: gearID}
	if experienceID != "" {
		ev.ExperienceId = ptr(experienceID)
	}
	if requestID != "" {
		ev.RequestId = ptr(requestID)
	}
	return &models.Notification{Payload: &models.Notification_CommunityEvent{CommunityEvent: ev}}
}

func TestNotificationEntity(t *testing.T) {
	chat := &models.Notification{Payload: &models.Notification_ChatMessage{
		ChatMessage: &models.ChatMessagePayload{CommunityId: "c2", RequestId: ptr("r2")},
	}}

	tests := []struct {
		name                       string
		n                          *models.Notification
		wantC, wantE, wantG, wantR string
	}{
		{"community event experience", communityEventNotification("c1", "e1", "", ""), "c1", "e1", "", ""},
		{"community event gear", communityEventNotification("c1", "", "g1", ""), "c1", "", "g1", ""},
		{"chat message request", chat, "c2", "", "", "r2"},
		{"no payload", &models.Notification{}, "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, e, g, r := notificationEntity(tt.n)
			if c != tt.wantC || e != tt.wantE || g != tt.wantG || r != tt.wantR {
				t.Errorf("notificationEntity = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
					c, e, g, r, tt.wantC, tt.wantE, tt.wantG, tt.wantR)
			}
		})
	}
}

func TestNotificationLink(t *testing.T) {
	const base = "https://test.example.com"
	okResolver := func(string) ShareLinkResolver {
		return func(context.Context, string, string, string, string) (string, error) { return "ABCD1234", nil }
	}

	tests := []struct {
		name string
		svc  *service
		n    *models.Notification
		want string
	}{
		{
			name: "resolves a /go short link",
			svc:  &service{shareLinks: okResolver("")},
			n:    communityEventNotification("c1", "e1", "", ""),
			want: base + "/go/ABCD1234",
		},
		{
			name: "no resolver wired falls back to base url",
			svc:  &service{},
			n:    communityEventNotification("c1", "e1", "", ""),
			want: base,
		},
		{
			name: "no community falls back to base url",
			svc:  &service{shareLinks: okResolver("")},
			n:    communityEventNotification("", "e1", "", ""),
			want: base,
		},
		{
			name: "empty code falls back to base url",
			svc: &service{shareLinks: func(context.Context, string, string, string, string) (string, error) {
				return "", nil
			}},
			n:    communityEventNotification("c1", "", "", ""),
			want: base,
		},
		{
			name: "resolver error falls back to base url",
			svc: &service{shareLinks: func(context.Context, string, string, string, string) (string, error) {
				return "", errors.New("boom")
			}},
			n:    communityEventNotification("c1", "e1", "", ""),
			want: base,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.svc.notificationLink(context.Background(), base, tt.n); got != tt.want {
				t.Errorf("notificationLink = %q, want %q", got, tt.want)
			}
		})
	}
}

// communityEventOfType builds a community-event notification with an explicit
// event type and no item entity — the shape every community-scoped
// notification (member joined, community deleted, …) arrives in.
func communityEventOfType(eventType string) *models.Notification {
	return &models.Notification{Payload: &models.Notification_CommunityEvent{
		CommunityEvent: &models.CommunityEventPayload{
			CommunityId: "c1",
			EventType:   eventType,
		},
	}}
}

// TestNotificationLink_DestinationHint covers the `?to=discuss` hint (#2876).
// The member-joined notification's CTA reads "Say hi at …", so its link has to
// land in the conversation — but the same community share link is reused by
// three other community-scoped events that must NOT be redirected into a chat
// pane.
func TestNotificationLink_DestinationHint(t *testing.T) {
	const base = "https://test.example.com"
	svc := &service{shareLinks: func(context.Context, string, string, string, string) (string, error) {
		return "ABCD1234", nil
	}}

	tests := []struct {
		name string
		n    *models.Notification
		want string
	}{
		{
			name: "member joined gets the discussion hint",
			n:    communityEventOfType("COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED"),
			want: base + "/go/ABCD1234?to=discuss",
		},
		{
			name: "community deleted does not",
			n:    communityEventOfType("COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED"),
			want: base + "/go/ABCD1234",
		},
		{
			name: "community restored does not",
			n:    communityEventOfType("COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED"),
			want: base + "/go/ABCD1234",
		},
		{
			name: "ownership transferred does not",
			n:    communityEventOfType("COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED"),
			want: base + "/go/ABCD1234",
		},
		{
			name: "an unset event type does not",
			n:    communityEventNotification("c1", "", "", ""),
			want: base + "/go/ABCD1234",
		},
		{
			name: "a chat-message payload does not",
			n: &models.Notification{Payload: &models.Notification_ChatMessage{
				ChatMessage: &models.ChatMessagePayload{CommunityId: "c1"},
			}},
			want: base + "/go/ABCD1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.notificationLink(context.Background(), base, tt.n); got != tt.want {
				t.Errorf("notificationLink = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNotificationLink_DestinationHintSkippedForItemLinks proves the hint never
// rides along on an item-flavored link. A member-joined event should not carry
// one, but if a future caller ever attaches an item id to it, the resulting
// link lands on that item's own page — where "?to=discuss" would be nonsense.
func TestNotificationLink_DestinationHintSkippedForItemLinks(t *testing.T) {
	const base = "https://test.example.com"
	svc := &service{shareLinks: func(context.Context, string, string, string, string) (string, error) {
		return "ABCD1234", nil
	}}
	n := communityEventOfType("COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED")
	n.GetCommunityEvent().GearId = "g1"

	want := base + "/go/ABCD1234"
	if got := svc.notificationLink(context.Background(), base, n); got != want {
		t.Errorf("notificationLink = %q, want %q", got, want)
	}
}

// TestNotificationLink_PassesAllEntityIDs proves notificationLink forwards every
// entity id to the resolver (the experience>gear>request precedence is the
// resolver's job, not the link builder's).
func TestNotificationLink_PassesAllEntityIDs(t *testing.T) {
	var gotC, gotE, gotG, gotR string
	svc := &service{shareLinks: func(_ context.Context, c, e, g, r string) (string, error) {
		gotC, gotE, gotG, gotR = c, e, g, r
		return "CODE", nil
	}}
	svc.notificationLink(context.Background(), "https://test.example.com",
		communityEventNotification("c1", "e1", "g1", ""))
	if gotC != "c1" || gotE != "e1" || gotG != "g1" || gotR != "" {
		t.Errorf("resolver args = (%q,%q,%q,%q), want (c1,e1,g1,)", gotC, gotE, gotG, gotR)
	}
}
