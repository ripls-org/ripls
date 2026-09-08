// Package web serves dynamic HTML pages with server-side rendering for
// invitation link previews (Open Graph metadata) and other public web pages.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"go.ripls.org/ripls/server/branding"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

//go:embed templates/invite.html templates/event.html templates/gear.html templates/request.html templates/community.html
var templateFS embed.FS

// ogImageDuration is how long presigned URLs for OG images remain valid.
// Social crawlers cache aggressively, so 1 hour is sufficient.
const ogImageDuration = 1 * time.Hour

// invitePageData holds template data for the shared share-link error
// surface (invite.html). Since #2875 every valid share link — community,
// event, gear, request — renders its own phone-first landing, so this
// carries only what the error card needs: the message, the short code (for
// the manual-entry and re-tap recovery affordances), and the default OG
// image.
type invitePageData struct {
	// Lang is the resolved BCP-47 locale tag for the <html lang> attribute
	// (from the visitor's Accept-Language; "en" when unresolved).
	Lang string
	// CommunityImageURL is the default Ripls logo — the error card has no
	// community to show, but the OG tags still need an image.
	CommunityImageURL string
	ShortCode         string
	BaseURL           string
	ErrorMessage      string
	// Brand is the instance identity the template renders — product name,
	// store links, and the legal footer.
	Brand branding.Config

	// Str holds the localized labels + title/OG copy for the error card.
	Str inviteStrings
}

// adHocCommunityNameKey is the l10n catalog key for the public-facing label of
// a nameless (ad-hoc, #2492) community on the SSR landing pages. Rendering the
// empty Community.Name directly produces broken copy ("Shared by Bob in ") and
// OG cards ("Join  on Ripls"). The label is deliberately generic: the landing
// already surfaces the item and host, and a public pre-auth page must not leak
// member names (the in-app group-text name is DISPLAY-1's job). Localized from
// the request's Accept-Language (the /go/ routes are wrapped in
// middleware.AcceptLanguage); see server/l10n/source/*.toml.
const adHocCommunityNameKey = "common.adhoc_community_name"

// communityDisplayName returns the community's name, or a localized generic
// public label when the community is nameless (ad-hoc). Used everywhere the SSR
// templates embed the community name so a nameless community never renders
// empty. The localizer is the request's (Accept-Language-resolved) one, built
// once per request by requestLocalizer.
func communityDisplayName(ctx context.Context, loc *l10n.Localizer, c *models.Community) string {
	if c.GetName() != "" {
		return c.GetName()
	}
	return loc.T(ctx, adHocCommunityNameKey, nil)
}

// requestLocalizer resolves the visitor's locale into a Localizer for the
// SSR landing pages. The /go/ route is wrapped in middleware.AcceptLanguage,
// so LocaleFromContext reflects the visitor's Accept-Language. The l10n
// bundle is process-wide and embedded (validated at New()), so construction
// only fails on a corrupt catalog — impossible in a shipped binary. On that
// impossible error we log and return nil; Localizer.T and langAttr are both
// nil-safe.
func (s *Service) requestLocalizer(ctx context.Context) *l10n.Localizer {
	loc, err := l10n.NewLocalizerForContext(ctx)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to build SSR localizer", "error", err)
		return nil
	}
	return loc
}

// langAttr returns the BCP-47 tag for a page's <html lang> attribute,
// falling back to the default locale ("en") when loc is nil.
func langAttr(loc *l10n.Localizer) string {
	if loc == nil {
		return l10n.DefaultTag.String()
	}
	return loc.Tag().String()
}

// Service serves dynamic HTML pages for public web routes.
type Service struct {
	storage           *storage.ProtoSQLStorage
	bucket            storage.BucketStorage
	hostname          string
	templates         *template.Template
	staticFS          http.Handler
	contentSub        fs.FS  // For existence checks before falling through.
	unsubscribeSecret []byte // HMAC key verifying off-app-email unsubscribe links.

	// smsAuthToken is the Twilio account auth token used to verify the
	// X-Twilio-Signature on both inbound Twilio POSTs: the STOP/HELP/START
	// opt-out webhook (/sms/webhook) and the outbound delivery status callback
	// (/sms/status). Empty when platform SMS is disabled, which disables
	// signature verification (acceptable only in dev/local — production must set
	// it).
	smsAuthToken string

	// emailWebhookSigningKey is the Mailgun *webhook signing key* — a distinct
	// secret from the API key used to send — verifying the HMAC on delivery
	// events posted to /email/status. Empty disables the endpoint (it rejects
	// every request), which is the safe default: an unauthenticated delivery
	// webhook is a way to fabricate deliverability data.
	emailWebhookSigningKey string

	// branding supplies the landings' instance identity: product name in the OG
	// tags, the app host the open-in-app logic compares against, the custom
	// scheme, the store links, and the legal footer. Zero value renders a
	// usable page with those affordances omitted.
	branding branding.Config
}

// SetBranding supplies the instance identity rendered into the landing pages.
// Set once at startup; the zero value is a valid unbranded page.
func (s *Service) SetBranding(b branding.Config) {
	s.branding = b
}

// SetEmailWebhookSigningKey wires the Mailgun webhook signing key used to
// verify inbound delivery-event signatures. Called at startup only when the
// secret is present; left empty otherwise, in which case /email/status rejects
// everything rather than trusting unsigned payloads.
func (s *Service) SetEmailWebhookSigningKey(key string) {
	s.emailWebhookSigningKey = key
}

// SetSMSAuthToken wires the Twilio account auth token used to verify inbound
// Twilio webhook + status-callback signatures. Called at startup only when
// platform SMS credentials are present; left empty otherwise.
func (s *Service) SetSMSAuthToken(token string) {
	s.smsAuthToken = token
}

// New creates a new web service for serving dynamic HTML pages. unsubscribeSecret
// is the HMAC key used to verify off-app-email one-click unsubscribe links
// (see HandleEmailUnsubscribe); it must match the key the notification service
// signs links with.
func New(sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage, hostname string, unsubscribeSecret []byte) (*Service, error) {
	tmpl, err := template.ParseFS(templateFS,
		"templates/invite.html", "templates/event.html",
		"templates/gear.html", "templates/request.html",
		"templates/community.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	// Preflight the l10n bundle: the SSR landings render every visitor-facing
	// string through it (#2090). A corrupt catalog is a build/config error, so
	// fail startup loudly here rather than degrading per-request — this also
	// guarantees requestLocalizer can't fail once the process is up.
	if _, err := l10n.Bundle(); err != nil {
		return nil, fmt.Errorf("failed to load l10n bundle: %w", err)
	}

	// Strip the "static/" prefix so files are served at their natural paths
	// (e.g. /css/shared.css, /assets/logo.png, /.well-known/assetlinks.json).
	contentFS, err := fs.Sub(staticContent, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to create sub filesystem for static assets: %w", err)
	}
	staticHandler := http.FileServer(http.FS(contentFS))

	return &Service{
		storage:           sqlStorage,
		bucket:            bucket,
		hostname:          hostname,
		templates:         tmpl,
		staticFS:          staticHandler,
		contentSub:        contentFS,
		unsubscribeSecret: unsubscribeSecret,
	}, nil
}

// HandleInvitePage serves the invitation page at /go/{code} with OG metadata.
func (s *Service) HandleInvitePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	shortCode := strings.TrimPrefix(r.URL.Path, "/go/")
	shortCode = strings.TrimRight(shortCode, "/")

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HandleInvitePage",
		"short_code", logging.MaskToken(shortCode),
	)

	// One localizer per request, resolved from the visitor's Accept-Language
	// (the /go/ route is wrapped in middleware.AcceptLanguage). Threaded into
	// every render site so the landing renders in the visitor's locale.
	loc := s.requestLocalizer(ctx)

	if shortCode == "" {
		logger.InfoContext(ctx, "invite page requested with empty short code")
		s.renderInviteError(w, ctx, loc, "", "web.error.no_code")
		return
	}

	// Look up share link by short code.
	links, err := s.storage.QueryByField(ctx, "short_code", shortCode, &models.ShareLink{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to look up share link", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}

	if len(links) == 0 {
		logger.InfoContext(ctx, "share link not found")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_invalid")
		return
	}

	invitation := links[0].(*models.ShareLink)

	if invitation.IsRevoked {
		logger.InfoContext(ctx, "share link is revoked")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.link_revoked")
		return
	}

	// Every share-link flavor gets a phone-first SSR landing: a public,
	// pre-auth page with a single primary action that completes after phone
	// verification.
	//   - experience_id       → handleEventLanding (event.html, RSVP).
	//   - gear_id / transfer  → handleGearLanding (gear.html, ExpressInterest).
	//   - request_id          → handleRequestLanding (request.html, OfferToFulfill).
	//   - community_invite_id → handleCommunityLanding (community.html, join).
	// invite.html is no longer a landing at all — it is the shared error
	// surface below (renderInviteError).
	if invitation.GetExperienceId() != "" {
		s.handleEventLanding(w, r, invitation)
		return
	}
	if invitation.GetGearId() != "" || invitation.GetTransferId() != "" {
		s.handleGearLanding(w, r, invitation)
		return
	}
	if invitation.GetRequestId() != "" {
		s.handleRequestLanding(w, r, invitation)
		return
	}
	s.handleCommunityLanding(w, r, invitation)
}

// ServeStaticAssets returns a handler that serves embedded CSS and
// asset files (favicons, logos, marketing-page CSS) from the
// `website/content/` tree. When the requested path doesn't exist in
// that tree, falls through to `ServeApp()` so the Flutter Web
// bundle's `/assets/*` (FontManifest.json, fonts/*, packages/*,
// shaders/*) resolve correctly. Both surfaces use the same `/assets/`
// prefix, so without this fallthrough the more-specific
// `/assets/` mux pattern shadows the catch-all and the bundle's
// font + manifest fetches 404 — which renders every icon as a
// "tofu" missing-glyph box.
func (s *Service) ServeStaticAssets() http.Handler {
	appHandler := s.ServeApp()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the leading slash for fs.Stat (the sub-FS roots at
		// the embed's `content/` directory, with no leading slash).
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel != "" {
			if _, err := fs.Stat(s.contentSub, rel); err == nil {
				s.staticFS.ServeHTTP(w, r)
				return
			}
		}
		appHandler.ServeHTTP(w, r)
	})
}

// ServeWellKnown returns a handler that serves .well-known files with correct
// Content-Type headers. apple-app-site-association has no file extension, so
// http.FileServer can't infer application/json from it.
func (s *Service) ServeWellKnown() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "apple-app-site-association") {
			w.Header().Set("Content-Type", "application/json")
		}
		s.staticFS.ServeHTTP(w, r)
	})
}

// renderInviteError renders the shared invite.html error surface with a
// localized message. It's the single error sink for every share-link
// variant (community, event, gear, request): each caller passes the short
// code and the l10n key for the message; the localizer and Lang come from
// the request. msgKey resolves via the request localizer (English
// fallback, then the key itself, if missing — see Localizer.T).
//
// Sets Vary: Accept-Language so a shared cache keys the localized HTML on
// the visitor's locale rather than serving one language to everyone.
func (s *Service) renderInviteError(w http.ResponseWriter, ctx context.Context, loc *l10n.Localizer, shortCode, msgKey string) {
	data := invitePageData{
		Lang:              langAttr(loc),
		ShortCode:         shortCode,
		BaseURL:           s.baseURL(),
		Brand:             s.branding,
		CommunityImageURL: s.defaultOGImageURL(),
		ErrorMessage:      loc.T(ctx, msgKey, nil),
		Str:               buildInviteStrings(ctx, loc),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept-Language")
	if err := s.templates.ExecuteTemplate(w, "invite.html", data); err != nil {
		logging.Default().Error("failed to render invite template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// getCommunityImageURL returns a presigned URL for the community's first image,
// or the default Ripls logo URL if the community has no images.
func (s *Service) getCommunityImageURL(ctx context.Context, communityModel *models.Community) string {
	if len(communityModel.MediaIds) == 0 {
		return s.defaultOGImageURL()
	}

	mediaID := communityModel.MediaIds[0]
	media := &models.Media{}
	if err := s.storage.GetByID(ctx, mediaID, media); err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to get community media for OG image", "error", err, "media_id", mediaID)
		return s.defaultOGImageURL()
	}

	// Prefer thumbnail: it has EXIF orientation baked in and is smaller,
	// which avoids rotated images in link previews on some platforms (e.g. Signal).
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	if media.GetThumbnailStorageUrl() != "" {
		bucketKey = storage.ThumbnailBucketKey(media.UserId, media.Id)
	}
	url, err := s.bucket.GetSignedURL(ctx, bucketKey, ogImageDuration)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to generate presigned URL for OG image", "error", err, "media_id", mediaID)
		return s.defaultOGImageURL()
	}

	return url
}

// defaultOGImageURL returns the URL for the default Ripls logo used when
// a community has no image.
func (s *Service) defaultOGImageURL() string {
	return fmt.Sprintf("%s/assets/logo.png", s.baseURL())
}

// baseURL returns the full base URL with scheme for the configured hostname.
// Uses http:// for localhost and IP addresses, https:// otherwise.
func (s *Service) baseURL() string {
	return BaseURL(s.hostname)
}

// BaseURL returns the full base URL with scheme for the given hostname.
// Uses http:// for localhost and IP addresses, https:// otherwise.
//
// Delegates to branding so the scheme this picks for a rendered page and the
// one branding.Config.AppBaseURL picks for a mailed link can never diverge —
// they describe the same deployment from opposite ends.
func BaseURL(hostname string) string {
	return branding.BaseURLFromHostname(hostname)
}
