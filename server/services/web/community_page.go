package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"go.ripls.org/ripls/server/branding"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// communityIntentJoin is the carrier value the SSR CTA threads into the
// Flutter web route (?intent=…). The web community screen reads it to
// route an unauthenticated guest phone-first and to join after
// verification (#2875). It must match the constant the client reads
// (web_community_screen.dart).
const communityIntentJoin = "join"

// destinationDiscuss is the one recognized value of the `to` destination hint
// on a `/go/{code}` community link: land in the community's discussion, and
// address the visitor as an existing member rather than an invitee (#2876).
// It must match notifications.DestinationDiscuss, which is what puts the hint
// on the URL, and the value the web community screen reads back as `tab`
// (web_community_screen.dart) — the same must-match arrangement as
// gearIntentInterest / requestIntentOffer.
const destinationDiscuss = "discuss"

// maxCommunityDescriptionRunes caps the community description rendered on
// the public landing. The description is the group's own pitch and is what
// makes the invite legible, but this page is readable by anyone holding an
// 8-character code, so it gets a blurb rather than an essay.
const maxCommunityDescriptionRunes = 280

// communityPageData holds template variables for the SSR community
// landing — a plain "join this group" share link. It mirrors the item
// landings (gear_page.go / request_page.go) for the community flow: a
// guest views the group on a public, pre-auth page and commits to a
// single action (join) that completes after phone verification. Field
// names mirror community.html — keep in sync.
//
// What this page deliberately does not carry: member names or avatars.
// The roster is not public. The inviter's first name is the trust signal
// and the member count is the size signal; that is the whole disclosure.
type communityPageData struct {
	// Lang is the resolved BCP-47 locale tag for the <html lang> attribute.
	Lang string

	// Identity and OG metadata.
	BaseURL string
	// Brand is the instance identity the template renders — product name,
	// app host, custom scheme, store links.
	Brand     branding.Config
	ShortCode string

	CommunityID   string
	CommunityName string
	// False for a nameless (ad-hoc) community: the headline switches to the
	// variant that omits the name entirely rather than rendering the generic
	// fallback label mid-sentence.
	CommunityIsNamed bool

	// CommunityDescription is the group's own pitch, truncated to
	// maxCommunityDescriptionRunes. Empty drops the block rather than
	// rendering an empty band.
	CommunityDescription string

	// IntentValue is the carrier the web screen reads to join after phone
	// verification (communityIntentJoin).
	IntentValue string

	// TabValue is the landing tab threaded into the web screen's URL
	// ("discuss"), or empty for the default community view. Set from the
	// `to` destination hint (#2876).
	TabValue string

	// IsInviteFraming is true for the ordinary case — someone was handed this
	// link and is being invited in. False when the link arrived from a
	// member-facing notification ("say hi"), which drops the "{inviter}
	// invited you" row: the recipient is already a member, and the link's
	// inviter is the community owner rather than anyone they'd recognize
	// from the notification.
	IsInviteFraming bool

	// Display copy (precomputed in Go so the template stays dumb, and
	// localized in the visitor's Accept-Language via server/l10n — #2090).
	// Each is a parameterized catalog value, not string concatenation.
	Kicker      string // "Invitation" — hero kicker chip
	Headline    string // "Dana invited you to join Ferndale Tools" — OG/title copy
	InviterLine string // "Dana invited you" — OG/meta description
	RoleText    string // "invited you" — verb phrase beside the bolded name
	CtaLabel    string // "Join the group"
	CtaAria     string // accessible label for the CTA link

	// MembersSummary is the localized member count ("12 members"). Never
	// names — see the type comment.
	MembersSummary string

	// OGTitle / OGDescription are the fully-composed, localized link-preview
	// strings ("{Headline} on Ripls" / "{InviterLine}. Share and borrow …").
	OGTitle       string
	OGDescription string

	// Hero image (presigned URL, or the default OG logo for the no-hero variant).
	HeroImageURL string
	HasHeroImage bool

	// Inviter — first name only (the only PII a public, pre-auth page
	// exposes, matching the item landings' owner name).
	InviterName      string
	InviterInitial   string
	InviterAvatarURL string

	// Capacity. A full community can't take the guest, so the CTA is
	// replaced by the shared at-capacity banner (communities cap at 32 —
	// see docs/community_health.md).
	IsAtCapacity bool
	NumMembers   int32
	MaxMembers   int32

	AndroidPackage string

	// Str holds the localized shared static labels (open-in-app hint,
	// at-capacity banner).
	Str itemStrings
}

// handleCommunityLanding renders the SSR community landing for a share
// link whose target oneof is the community itself — the plain "join this
// group" invite (#2875). Like the item landings it assumes the caller has
// already loaded the ShareLink row and validated it isn't revoked. On a
// missing or soft-deleted community, or a missing inviter, it falls back
// to the shared invite-page error template.
func (s *Service) handleCommunityLanding(w http.ResponseWriter, r *http.Request, shareLink *models.ShareLink) {
	ctx := r.Context()
	shortCode := shareLink.ShortCode
	communityID := shareLink.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HandleCommunityLanding",
		"short_code", logging.MaskToken(shortCode),
		"community_id", communityID,
	)

	// One localizer per request (Accept-Language), threaded into the build
	// and error-render paths so the landing renders in the visitor's locale.
	loc := s.requestLocalizer(ctx)

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "community not found")
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
			return
		}
		logger.ErrorContext(ctx, "failed to fetch community", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}
	if community.Deleted != nil {
		logger.InfoContext(ctx, "community is soft-deleted")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
		return
	}

	inviter := &models.User{}
	if err := s.storage.GetByID(ctx, shareLink.InviterId, inviter); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "inviter not found", "inviter_id", shareLink.InviterId)
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
			return
		}
		logger.ErrorContext(ctx, "failed to fetch inviter", "error", err, "inviter_id", shareLink.InviterId)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}

	// A failed member count degrades the page rather than replacing it: the
	// count is a size cue, and a guest who can't see it can still read the
	// invite and join. Treating zero as "not at capacity" is the safe
	// direction — AcceptInvitationLink re-checks capacity server-side and
	// rejects an over-cap join, so the worst case is a CTA that errors
	// rather than a member silently exceeding the cap. Same posture as the
	// item landings.
	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, communityID)
	if err != nil {
		logger.Warn("failed to get community member count — rendering without it", "error", err)
		memberCount = 0
	}

	destination := parseDestination(r.URL.Query().Get("to"))

	data := s.buildCommunityPageData(ctx, loc, shortCode, community, inviter, memberCount, destination)

	logger.InfoContext(ctx, "serving community landing page",
		"is_at_capacity", data.IsAtCapacity,
		"has_hero", data.HasHeroImage,
		"destination", destination,
	)
	s.renderCommunityPage(w, data)
}

// parseDestination validates the `to` destination hint against the closed
// vocabulary the notification senders emit. Anything else — a stale link, a
// typo, a forged value — returns "" and the landing renders its ordinary invite
// framing. The hint only chooses copy and a landing tab; it grants nothing, so
// an unrecognized value is a no-op rather than an error.
func parseDestination(raw string) string {
	if raw == destinationDiscuss {
		return destinationDiscuss
	}
	return ""
}

// buildCommunityPageData composes the template payload from the resolved
// community + inviter. The only outbound I/O is the hero and avatar
// presigns.
func (s *Service) buildCommunityPageData(ctx context.Context, loc *l10n.Localizer, shortCode string,
	community *models.Community, inviter *models.User, memberCount int, destination string,
) communityPageData {
	// The community photo is the hero; resolveLandingHero falls back to the
	// default Ripls logo and reports whether the result is a real photo.
	heroImageURL, hasHeroImage := s.resolveLandingHero(ctx, "", community)

	inviterName := firstName(inviter.Name)
	if inviterName == "" {
		inviterName = loc.T(ctx, "web.common.someone", nil)
	}

	communityName := communityDisplayName(ctx, loc, community)
	communityIsNamed := community.GetName() != ""

	// Parameterized catalog values, not concatenation, so word order stays
	// correct across locales. A nameless community takes the variant that
	// omits the name rather than splicing the generic label mid-sentence.
	nameArgs := map[string]any{"Inviter": inviterName, "Community": communityName}
	headlineKey := "web.community.headline_unnamed"
	if communityIsNamed {
		headlineKey = "web.community.headline"
	}
	headline := loc.T(ctx, headlineKey, nameArgs)
	inviterLine := loc.T(ctx, "web.community.inviter_line", map[string]any{"Inviter": inviterName})

	// The member-facing variant ("say hi") drops the invitation framing
	// entirely: no "invited you" headline or row, a discussion kicker, and a
	// CTA that says what it does. The community name carries the page instead.
	kicker := loc.T(ctx, "web.community.kicker", nil)
	ctaLabel := loc.T(ctx, "web.community.cta_label", nil)
	ctaAria := loc.T(ctx, "web.community.cta_aria", nameArgs)
	ogDesc := loc.T(ctx, "web.community.og_desc", map[string]any{"InviterLine": inviterLine})
	if destination == destinationDiscuss {
		kicker = loc.T(ctx, "web.community.kicker_discussion", nil)
		headline = communityName
		ctaLabel = loc.T(ctx, "web.community.cta_label_discussion", nil)
		ctaAria = loc.T(ctx, "web.community.cta_aria_discussion", nameArgs)
		ogDesc = loc.T(ctx, "web.community.og_desc_discussion", nil)
	}

	return communityPageData{
		Lang:                 langAttr(loc),
		BaseURL:              s.baseURL(),
		Brand:                s.branding,
		ShortCode:            shortCode,
		CommunityID:          community.GetId(),
		CommunityName:        communityName,
		CommunityIsNamed:     communityIsNamed,
		CommunityDescription: truncateRunes(community.GetDescription(), maxCommunityDescriptionRunes),
		IntentValue:          communityIntentJoin,
		TabValue:             destination,
		IsInviteFraming:      destination != destinationDiscuss,
		Kicker:               kicker,
		Headline:             headline,
		InviterLine:          inviterLine,
		RoleText:             loc.T(ctx, "web.community.role", nil),
		CtaLabel:             ctaLabel,
		CtaAria:              ctaAria,
		MembersSummary:       loc.TCount(ctx, "web.community.members", memberCount, nil),
		OGTitle:              titleOnRipls(ctx, loc, headline),
		OGDescription:        ogDesc,
		HeroImageURL:         heroImageURL,
		HasHeroImage:         hasHeroImage,
		InviterName:          inviterName,
		InviterInitial:       firstInitial(inviterName),
		InviterAvatarURL:     s.getMediaImageURL(ctx, inviter.MediaIds),
		IsAtCapacity:         memberCount >= communitylib.MaxCommunityMembers,
		NumMembers:           int32(memberCount),
		MaxMembers:           int32(communitylib.MaxCommunityMembers),
		AndroidPackage:       s.branding.AndroidPackageID,
		Str:                  buildCommunityStrings(ctx, loc, int32(memberCount), int32(communitylib.MaxCommunityMembers)),
	}
}

// renderCommunityPage executes the community-landing template. Sets the
// same short 30-second SSR cache TTL as the other landings; the payload is
// viewer-independent, so a shared cache keyed on Accept-Language is correct.
func (s *Service) renderCommunityPage(w http.ResponseWriter, data communityPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Vary", "Accept-Language")
	if err := s.templates.ExecuteTemplate(w, "community.html", data); err != nil {
		logging.Default().Error("failed to render community template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// truncateRunes shortens s to at most maxRunes runes, appending an ellipsis
// when it cuts. Counts runes rather than bytes so a multi-byte description
// isn't split mid-character, and trims trailing space so the ellipsis sits
// against a word.
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return strings.TrimRight(s[:i], " \t\n") + "…"
		}
		count++
	}
	return s
}
