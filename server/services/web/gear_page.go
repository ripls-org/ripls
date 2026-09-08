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

// gearIntentInterest is the carrier value the SSR CTA threads into the
// Flutter web route (?intent=…). The web gear screen reads it to
// auto-fire ExpressInterest after phone verification (#2492, WEB-4). It
// must match the constant the client reads (web_gear_screen.dart).
const gearIntentInterest = "interest"

// gearPageData holds template variables for the SSR gear landing — a
// shared loan or giveaway item. It mirrors the event landing
// (event_page.go) for the gear flow: a guest views the item on a
// public, pre-auth page and commits to a single action (borrow or
// claim) that auto-fires ExpressInterest after phone verification.
// Field names mirror gear.html — keep in sync.
type gearPageData struct {
	// Lang is the resolved BCP-47 locale tag for the <html lang> attribute.
	Lang string

	// Identity and OG metadata.
	BaseURL string
	// Brand is the instance identity the template renders — product name,
	// app host, custom scheme, store links.
	Brand     branding.Config
	ShortCode string

	// GearID is the resolved gear_id (a transfer-flavored link resolves
	// to its underlying gear).
	GearID   string
	GearName string

	// IntentValue is the carrier the web screen reads to auto-fire
	// ExpressInterest after phone verification (gearIntentInterest).
	IntentValue string

	// Display copy (precomputed in Go so the template stays dumb, and
	// localized in the visitor's Accept-Language via server/l10n — #2090).
	// Adapts to loan vs giveaway; each is a parameterized catalog value, not
	// string concatenation, so word order stays correct across locales.
	Headline  string // "Dana is lending a drill" — OG/title copy (item name inline)
	Kicker    string // "Borrow" / "Free" — hero kicker chip
	OwnerLine string // "Dana is lending this" — OG/meta description (name inline)
	RoleText  string // "is lending this" — bare verb phrase beside the bolded name
	CtaLabel  string // "Ask to borrow" / "I want this"
	CtaAria   string // accessible label for the CTA link

	// OGTitle / OGDescription are the fully-composed, localized link-preview
	// strings ("{Headline} on Ripls" / "{OwnerLine}[ in {Community}].").
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
	// True when HeroImageURL is a real photo (gear/community), not the
	// default OG logo — gates threading it to the phone-first backdrop.
	HasHeroImage bool

	// Owner — first name only (the only PII a public, pre-auth page
	// exposes, matching the event landing's host name).
	OwnerName    string
	OwnerInitial string

	// Capacity. Ad-hoc per-item communities still honor the 32-cap until
	// the exemption lands (COMM-1 deferred), so mirror the event
	// landing's at-capacity guard.
	IsAtCapacity bool
	NumMembers   int32
	MaxMembers   int32

	// AndroidPackage is the app package name for this hostname, embedded
	// into the "Open in Ripls" Intent URI (see event_page.go).
	AndroidPackage string

	// Str holds the localized shared static labels (open-in-app hint,
	// at-capacity banner).
	Str itemStrings
}

// handleGearLanding renders the SSR gear landing for a share link whose
// target oneof is a gear or a transfer (#2492, WEB-4). Like
// handleEventLanding it assumes the caller has already loaded the
// ShareLink row and validated it isn't revoked. A transfer-flavored link
// resolves to the underlying gear (recipients view the item being
// transferred). On an unavailable gear (terminal state, soft-deleted,
// missing/deleted hosting community) it falls back to the shared
// invite-page error template.
func (s *Service) handleGearLanding(w http.ResponseWriter, r *http.Request, shareLink *models.ShareLink) {
	ctx := r.Context()
	shortCode := shareLink.ShortCode
	communityID := shareLink.CommunityId

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HandleGearLanding",
		"short_code", logging.MaskToken(shortCode),
		"community_id", communityID,
	)

	// One localizer per request (Accept-Language), threaded into the build
	// and error-render paths so the landing renders in the visitor's locale.
	loc := s.requestLocalizer(ctx)

	// Resolve the underlying gear. A transfer-flavored link shows the
	// gear behind it. fetchGearPreview returns nil for missing,
	// soft-deleted, or terminal-state gear.
	var gearID string
	switch {
	case shareLink.GetGearId() != "":
		gearID = shareLink.GetGearId()
	case shareLink.GetTransferId() != "":
		transfer := &models.Transfer{}
		if err := s.storage.GetByID(ctx, shareLink.GetTransferId(), transfer); err != nil {
			logger.InfoContext(ctx, "share link points at non-existent transfer", "error", err)
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.item_unavailable")
			return
		}
		if transfer.Deleted != nil {
			logger.InfoContext(ctx, "transfer is soft-deleted")
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.item_unavailable")
			return
		}
		gearID = transfer.GearId
	default:
		logger.WarnContext(ctx, "handleGearLanding called for a non-gear share link")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
		return
	}

	preview := s.fetchGearPreview(ctx, gearID, communityID)
	if preview == nil {
		logger.InfoContext(ctx, "gear not in a shareable state — rendering unavailable page")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.item_unavailable")
		return
	}

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "hosting community not found")
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.item_unavailable")
			return
		}
		logger.ErrorContext(ctx, "failed to fetch community", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}
	if community.Deleted != nil {
		logger.InfoContext(ctx, "hosting community is soft-deleted")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.item_unavailable")
		return
	}

	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, communityID)
	if err != nil {
		logger.Warn("failed to get community member count — assuming not at capacity", "error", err)
		memberCount = 0
	}

	data := s.buildGearPageData(ctx, loc, shortCode, gearID, preview, community, memberCount)

	logger.InfoContext(ctx, "serving gear landing page",
		"item_type", string(preview.ItemType),
		"is_at_capacity", data.IsAtCapacity,
		"has_hero", data.HasHeroImage,
	)
	s.renderGearPage(w, data)
}

// buildGearPageData composes the template payload from the resolved gear
// preview + hosting community. The only outbound I/O is the
// community-image presign in the no-gear-photo fallback.
func (s *Service) buildGearPageData(ctx context.Context, loc *l10n.Localizer, shortCode, gearID string,
	preview *itemPreviewData, community *models.Community, memberCount int,
) gearPageData {
	heroImageURL, hasHeroImage := s.resolveLandingHero(ctx, preview.ItemImageURL, community)

	ownerName := preview.OwnerName
	if ownerName == "" {
		ownerName = loc.T(ctx, "web.common.someone", nil)
	}

	data := gearPageData{
		Lang:             langAttr(loc),
		BaseURL:          s.baseURL(),
		Brand:            s.branding,
		ShortCode:        shortCode,
		GearID:           gearID,
		GearName:         preview.ItemName,
		IntentValue:      gearIntentInterest,
		CommunityName:    communityDisplayName(ctx, loc, community),
		CommunityIsNamed: community.GetName() != "",
		HeroImageURL:     heroImageURL,
		HasHeroImage:     hasHeroImage,
		OwnerName:        ownerName,
		OwnerInitial:     firstInitial(ownerName),
		IsAtCapacity:     memberCount >= communitylib.MaxCommunityMembers,
		NumMembers:       int32(memberCount),
		MaxMembers:       int32(communitylib.MaxCommunityMembers),
		AndroidPackage:   s.branding.AndroidPackageID,
	}

	// Loan vs giveaway copy — parameterized catalog keys (not concatenation)
	// so Spanish word order/verb is correct. The flavor picks the key suffix.
	flavor := "loan"
	if preview.ItemType == itemTypeGearGiveaway {
		flavor = "giveaway"
	}
	nameItem := map[string]any{"Owner": ownerName, "Item": preview.ItemName}
	data.Headline = loc.T(ctx, "web.gear.headline."+flavor, nameItem)
	data.Kicker = loc.T(ctx, "web.gear.kicker."+flavor, nil)
	data.OwnerLine = loc.T(ctx, "web.gear.owner_line."+flavor, map[string]any{"Owner": ownerName})
	data.RoleText = loc.T(ctx, "web.gear.role."+flavor, nil)
	data.CtaLabel = loc.T(ctx, "web.gear.cta_label."+flavor, nil)
	data.CtaAria = loc.T(ctx, "web.gear.cta_aria."+flavor, nameItem)

	// Composed link-preview copy. The in-community variant appends the
	// "in {Community}" clause; it drops for a nameless community.
	data.OGTitle = titleOnRipls(ctx, loc, data.Headline)
	ogDescKey := "web.gear.og_desc"
	if data.CommunityIsNamed {
		ogDescKey = "web.gear.og_desc_in_community"
	}
	data.OGDescription = loc.T(ctx, ogDescKey, map[string]any{"OwnerLine": data.OwnerLine, "Community": data.CommunityName})

	data.Str = buildItemStrings(ctx, loc, int32(memberCount), int32(communitylib.MaxCommunityMembers))
	return data
}

// renderGearPage executes the gear-landing template. Sets the same short
// 30-second SSR cache TTL as the event landing.
func (s *Service) renderGearPage(w http.ResponseWriter, data gearPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Vary", "Accept-Language")
	if err := s.templates.ExecuteTemplate(w, "gear.html", data); err != nil {
		logging.Default().Error("failed to render gear template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// resolveLandingHero picks the hero image for a non-event landing: the
// item photo first, then the community thumb, then the default Ripls
// logo. getCommunityImageURL already returns the default logo when the
// community has no media, so a non-default result is a real photo. The
// bool reports whether the URL is a real photo (vs the default logo) —
// the phone-first screen only uses a real photo as a backdrop.
func (s *Service) resolveLandingHero(ctx context.Context, itemImageURL string, community *models.Community) (string, bool) {
	if itemImageURL != "" {
		return itemImageURL, true
	}
	communityImageURL := s.getCommunityImageURL(ctx, community)
	return communityImageURL, communityImageURL != s.defaultOGImageURL()
}
