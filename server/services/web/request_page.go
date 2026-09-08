package web

import (
	"context"
	"errors"
	"net/http"

	"go.ripls.org/ripls/server/branding"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// requestIntentOffer is the carrier value the SSR CTA threads into the
// Flutter web route (?intent=…). The web request screen reads it to
// auto-fire OfferToFulfill after phone verification (#2492, WEB-3). It
// must match the constant the client reads (web_request_screen.dart).
const requestIntentOffer = "offer"

// maxSummarizedNeeds caps the PlanningNeed rows the landing's needs
// summary counts. A single request's supply list sits far below this in
// practice; the cap exists only so the query can't fan out unbounded.
const maxSummarizedNeeds = 200

// requestPageData holds template variables for the SSR request landing —
// a neighbor asking for something. It mirrors the event landing
// (event_page.go) for the request flow: a guest views the request on a
// public, pre-auth page and commits to a single action (offer to help)
// that auto-fires OfferToFulfill after phone verification. Field names
// mirror request.html — keep in sync.
type requestPageData struct {
	// Lang is the resolved BCP-47 locale tag for the <html lang> attribute.
	Lang string

	// Identity and OG metadata.
	BaseURL string
	// Brand is the instance identity the template renders — product name,
	// app host, custom scheme, store links.
	Brand     branding.Config
	ShortCode string

	RequestID    string
	RequestTitle string

	// RequestDescription is the requester's own pitch. Empty when the
	// request has none — the template drops the block rather than
	// rendering an empty band where the pitch belongs.
	RequestDescription string

	// NeedsSummary is the compact supply line — "5 items · 1 still open"
	// ("… · all covered" once every need is claimed). Empty when the
	// request has no needs, which drops the block.
	NeedsSummary string

	// IntentValue is the carrier the web screen reads to auto-fire
	// OfferToFulfill after phone verification (requestIntentOffer).
	IntentValue string

	// Display copy (precomputed in Go so the template stays dumb, and
	// localized in the visitor's Accept-Language via server/l10n — #2090).
	// Each is a parameterized catalog value, not string concatenation.
	Kicker        string // "Wanted" — hero kicker chip
	Headline      string // "Dana is looking for a ladder" — OG/title copy
	RequesterLine string // "Dana is looking for this" — OG/meta description
	RoleText      string // "is looking for this" — verb phrase beside the bolded name
	CtaLabel      string // "Offer to help"
	CtaAria       string // accessible label for the CTA link

	// OGTitle / OGDescription are the fully-composed, localized link-preview
	// strings ("{Headline} on Ripls" / "{RequesterLine}[ in {Community}]. Can
	// you help?").
	OGTitle       string
	OGDescription string

	// Hosting community.
	CommunityName string
	// False for a nameless (ad-hoc) community: descriptions drop their
	// "in {community}" suffix and the kicker drops the community
	// segment, rather than rendering the generic fallback label.
	CommunityIsNamed bool

	// Hero image (presigned URL, or the default OG logo for the no-hero variant).
	HeroImageURL string
	HasHeroImage bool

	// Requester — first name only (the only PII a public, pre-auth page
	// exposes, matching the event landing's host name).
	RequesterName    string
	RequesterInitial string
	// RequesterAvatarURL is a presigned URL for the requester's profile
	// photo; empty falls back to RequesterInitial. Mirrors the event
	// landing's HostAvatarURL.
	RequesterAvatarURL string

	// Capacity (mirrors the event landing — ad-hoc communities still
	// honor the 32-cap until the exemption lands).
	IsAtCapacity bool
	NumMembers   int32
	MaxMembers   int32

	AndroidPackage string

	// Str holds the localized shared static labels (open-in-app hint,
	// at-capacity banner).
	Str itemStrings
}

// handleRequestLanding renders the SSR request landing for a share link
// whose target oneof is a request (#2492, WEB-3). Like handleEventLanding
// it assumes the caller has already loaded the ShareLink row and
// validated it isn't revoked. On an unavailable request (cancelled,
// fulfilled, soft-deleted, missing/deleted hosting community) it falls
// back to the shared invite-page error template.
func (s *Service) handleRequestLanding(w http.ResponseWriter, r *http.Request, shareLink *models.ShareLink) {
	ctx := r.Context()
	shortCode := shareLink.ShortCode
	communityID := shareLink.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HandleRequestLanding",
		"short_code", logging.MaskToken(shortCode),
		"community_id", communityID,
	)

	// One localizer per request (Accept-Language), threaded into the build
	// and error-render paths so the landing renders in the visitor's locale.
	loc := s.requestLocalizer(ctx)

	requestID := shareLink.GetRequestId()
	if requestID == "" {
		logger.WarnContext(ctx, "handleRequestLanding called for a non-request share link")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
		return
	}

	// fetchRequestPreview returns nil for missing, soft-deleted, or
	// terminal-state (cancelled / fulfilled) requests.
	preview := s.fetchRequestPreview(ctx, requestID)
	if preview == nil {
		logger.InfoContext(ctx, "request not in a shareable state — rendering unavailable page")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.request_unavailable")
		return
	}

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "hosting community not found")
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.request_unavailable")
			return
		}
		logger.ErrorContext(ctx, "failed to fetch community", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}
	if community.Deleted != nil {
		logger.InfoContext(ctx, "hosting community is soft-deleted")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.request_unavailable")
		return
	}

	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, communityID)
	if err != nil {
		logger.Warn("failed to get community member count — assuming not at capacity", "error", err)
		memberCount = 0
	}

	needsSummary := s.requestNeedsSummary(ctx, loc, requestID, logger)

	data := s.buildRequestPageData(ctx, loc, shortCode, requestID, preview, community, memberCount, needsSummary)

	logger.InfoContext(ctx, "serving request landing page",
		"is_at_capacity", data.IsAtCapacity,
		"has_hero", data.HasHeroImage,
	)
	s.renderRequestPage(w, data)
}

// requestNeedsSummary composes the landing's compact supply line from the
// request's PlanningNeed rows — "5 items · 1 still open". Returns "" when
// the request has no needs, or when the query fails: the summary is a
// nice-to-have, and a guest who can't see it can still read the pitch and
// offer to help, so a failure degrades the page rather than replacing it
// with an error (same posture as the RSVP/member-count fetches on the
// event landing).
func (s *Service) requestNeedsSummary(ctx context.Context, loc *l10n.Localizer, requestID string, logger *logging.Logger) string {
	needs, err := storage.QueryByFields[*models.PlanningNeed](s.storage, ctx,
		map[string]any{"request_id": requestID},
		storage.QueryOptions{Limit: maxSummarizedNeeds})
	if err != nil {
		logger.Warn("failed to fetch request needs — rendering landing without the needs summary", "error", err)
		return ""
	}
	return formatNeedsSummary(ctx, loc, needs)
}

// formatNeedsSummary renders the localized needs line for the landing, e.g.
// "5 items · 1 still open". The item and open counts drive per-locale plural
// selection. "Still open" counts needs with slots nobody has claimed yet
// (SlotsRemaining > 0); once every need is spoken for the tail reads "all
// covered" rather than "0 still open".
func formatNeedsSummary(ctx context.Context, loc *l10n.Localizer, needs []*models.PlanningNeed) string {
	if len(needs) == 0 {
		return ""
	}
	open := 0
	for _, n := range needs {
		if n.SlotsRemaining > 0 {
			open++
		}
	}
	items := loc.TCount(ctx, "web.request.needs.items", len(needs), nil)
	if open == 0 {
		return loc.T(ctx, "web.request.needs.all_covered", map[string]any{"Items": items})
	}
	return loc.TCount(ctx, "web.request.needs.open", open, map[string]any{"Items": items})
}

// buildRequestPageData composes the template payload from the resolved
// request preview + hosting community.
func (s *Service) buildRequestPageData(ctx context.Context, loc *l10n.Localizer, shortCode, requestID string,
	preview *itemPreviewData, community *models.Community, memberCount int, needsSummary string,
) requestPageData {
	heroImageURL, hasHeroImage := s.resolveLandingHero(ctx, preview.ItemImageURL, community)

	requesterName := preview.OwnerName
	if requesterName == "" {
		requesterName = loc.T(ctx, "web.common.someone", nil)
	}

	requester := map[string]any{"Requester": requesterName}
	nameItem := map[string]any{"Requester": requesterName, "Item": preview.ItemName}
	headline := loc.T(ctx, "web.request.headline", nameItem)
	requesterLine := loc.T(ctx, "web.request.requester_line", requester)
	communityName := communityDisplayName(ctx, loc, community)
	communityIsNamed := community.GetName() != ""

	// Composed link-preview copy — "{Headline} on Ripls" title, and a
	// description that appends the "in {Community}" clause only for a named
	// community and closes with the "Can you help?" prompt.
	ogTitle := titleOnRipls(ctx, loc, headline)
	ogDescKey := "web.request.og_desc"
	if communityIsNamed {
		ogDescKey = "web.request.og_desc_in_community"
	}
	ogDesc := loc.T(ctx, ogDescKey, map[string]any{"RequesterLine": requesterLine, "Community": communityName})

	return requestPageData{
		Lang:               langAttr(loc),
		BaseURL:            s.baseURL(),
		Brand:              s.branding,
		ShortCode:          shortCode,
		RequestID:          requestID,
		RequestTitle:       preview.ItemName,
		RequestDescription: preview.ItemDescription,
		NeedsSummary:       needsSummary,
		IntentValue:        requestIntentOffer,
		Kicker:             loc.T(ctx, "web.request.kicker_wanted", nil),
		Headline:           headline,
		RequesterLine:      requesterLine,
		RoleText:           loc.T(ctx, "web.request.role", nil),
		CtaLabel:           loc.T(ctx, "web.request.cta_label", nil),
		CtaAria:            loc.T(ctx, "web.request.cta_aria", requester),
		OGTitle:            ogTitle,
		OGDescription:      ogDesc,
		CommunityName:      communityName,
		CommunityIsNamed:   communityIsNamed,
		HeroImageURL:       heroImageURL,
		HasHeroImage:       hasHeroImage,
		RequesterName:      requesterName,
		RequesterInitial:   firstInitial(requesterName),
		RequesterAvatarURL: preview.OwnerAvatarURL,
		IsAtCapacity:       memberCount >= communitylib.MaxCommunityMembers,
		NumMembers:         int32(memberCount),
		MaxMembers:         int32(communitylib.MaxCommunityMembers),
		AndroidPackage:     s.branding.AndroidPackageID,
		Str:                buildItemStrings(ctx, loc, int32(memberCount), int32(communitylib.MaxCommunityMembers)),
	}
}

// renderRequestPage executes the request-landing template. Sets the same
// short 30-second SSR cache TTL as the event landing.
func (s *Service) renderRequestPage(w http.ResponseWriter, data requestPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Vary", "Accept-Language")
	if err := s.templates.ExecuteTemplate(w, "request.html", data); err != nil {
		logging.Default().Error("failed to render request template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
