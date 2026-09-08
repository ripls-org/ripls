package notifications

import (
	"context"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// ShareLinkResolver find-or-creates the public `/go` short-link code that lands
// on the entity a notification is about (experience, gear, or request) within a
// community, returning its short code. It is satisfied by the community
// service's ShortLinkCodeForEntity and injected via WithShareLinkResolver.
//
// Off-app notifications (SMS, email, later RCS) must carry a short, legible
// `/go/{code}` link rather than a long entity-ID URL: for many recipients SMS
// is the primary — sometimes only — channel, and `/go` links are also the only
// app-host paths that universal-link into the native app. Returning ("", nil)
// means "no linkable entity"; the sender then falls back to the bare app URL.
type ShareLinkResolver func(ctx context.Context, communityID, experienceID, gearID, requestID string) (string, error)

// WithShareLinkResolver wires the resolver used to turn a notification into a
// `/go` short link. Without it, off-app senders fall back to the bare app URL.
func WithShareLinkResolver(r ShareLinkResolver) Option {
	return func(s *service) { s.shareLinks = r }
}

// DestinationDiscuss is the only value the `to` destination hint may take. It
// tells the `/go/{code}` landing to send the visitor to the community's
// discussion rather than its profile, and to address them as an existing
// member rather than an invitee (#2876).
//
// This is deliberately a closed vocabulary of one. #2562 stripped query
// parameters off `/go/` URLs because they carried *entity ids* that duplicated
// — and could contradict — the target on the ShareLink row. A destination hint
// is a different kind of value: it says where to go *within* the target the row
// already resolved, never what the target is. Anything outside this vocabulary
// is ignored on read, so a forged or stale `to` degrades to the ordinary
// landing.
//
// It is a presentation signal, never an authorization one. It says "this
// recipient is probably already a member"; membership is still established
// server-side by AcceptInvitationLink, and an unauthenticated visitor who
// forges it gets the ordinary phone-first join.
const DestinationDiscuss = "discuss"

// memberJoinedEventType is the CommunityEventType whose off-app link points at
// the community discussion. Its notification body is "{actor} joined
// {community}" with a "Say hi at …" CTA, so the conversation is what the copy
// promises. The other community-scoped events that resolve to the same
// community share link — COMMUNITY_DELETED, COMMUNITY_RESTORED,
// OWNERSHIP_TRANSFERRED — want the community itself, and must not be given
// this hint: a deletion notice does not belong in a chat pane.
const memberJoinedEventType = "COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED"

// notificationLink returns the URL an off-app recipient (email or SMS) should
// land on for this notification: a short `/go` share link to the entity the
// notification is about when one resolves, else the bare baseURL. baseURL is
// the channel's configured AppBaseURL (e.g. "https://app.example.com"). Resolution is
// best-effort — any failure degrades to baseURL so the notification still sends.
func (s *service) notificationLink(ctx context.Context, baseURL string, n *models.Notification) string {
	communityID, experienceID, gearID, requestID := notificationEntity(n)
	if s.shareLinks == nil || communityID == "" {
		return baseURL
	}
	code, err := s.shareLinks(ctx, communityID, experienceID, gearID, requestID)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"off-app: share-link resolution failed; using base url", "error", err)
		return baseURL
	}
	if code == "" {
		return baseURL
	}
	return strings.TrimRight(baseURL, "/") + "/go/" + code + notificationDestination(n)
}

// notificationDestination returns the `?to=` query suffix for this
// notification's link, or "" for the great majority that need none. Only the
// member-joined event gets one, and only when it carries no item entity — an
// item-flavored link already lands on that item's own page, where a community
// destination would be nonsense.
func notificationDestination(n *models.Notification) string {
	ev := n.GetCommunityEvent()
	if ev == nil || ev.GetEventType() != memberJoinedEventType {
		return ""
	}
	if ev.GetGearId() != "" || ev.GetExperienceId() != "" || ev.GetRequestId() != "" {
		return ""
	}
	return "?to=" + DestinationDiscuss
}

// notificationEntity extracts the community and the share-linkable entity ids
// (experience, gear, request) a notification's payload carries. Both payload
// variants expose the same set; fields the payload doesn't set come back empty.
func notificationEntity(n *models.Notification) (communityID, experienceID, gearID, requestID string) {
	if ev := n.GetCommunityEvent(); ev != nil {
		return ev.GetCommunityId(), ev.GetExperienceId(), ev.GetGearId(), ev.GetRequestId()
	}
	if cm := n.GetChatMessage(); cm != nil {
		return cm.GetCommunityId(), cm.GetExperienceId(), cm.GetGearId(), cm.GetRequestId()
	}
	return "", "", "", ""
}
