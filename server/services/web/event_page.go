package web

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"go.ripls.org/ripls/server/branding"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// maxShownAttendees is how many attendee avatars appear in the
// landing's avatar cluster before collapsing into a "+ N others"
// overflow label.
const maxShownAttendees = 4

// maxFetchedAttendees is how many YES-RSVPed users we hydrate with
// name + photo for the landing. Anyone beyond this contributes only
// to the overflow count, not to the rendered avatars or names.
const maxFetchedAttendees = 8

// eventPageData holds template variables for the SSR event landing.
// Field names mirror the template — keep them in sync.
type eventPageData struct {
	// Lang is the resolved BCP-47 locale tag for the <html lang> attribute.
	Lang string

	// Identity and OG metadata.
	BaseURL string
	// Brand is the instance identity the template renders — product name,
	// app host, custom scheme, store links.
	Brand        branding.Config
	ShortCode    string
	ExperienceID string
	EventName    string
	// Title is the localized "Join {Host} for {Event}[ in {Community}]" copy
	// used for <title>, og:title, and twitter:title.
	Title string

	// Hosting community.
	CommunityName string
	// False for a nameless (ad-hoc) community: the title/OG copy drops
	// its "in {community}" suffix and the kicker drops the community
	// segment, rather than rendering the generic fallback label.
	CommunityIsNamed bool

	// Hero image (presigned URL, or empty for the no-hero variant).
	HeroImageURL string
	// True when HeroImageURL is a real photo (experience/community), not the
	// default OG logo — gates threading it to the phone-first RSVP backdrop.
	HasHeroImage bool

	// When / where.
	DayLabel      string // "Saturday"
	WhenShort     string // "Sat, May 24"
	WhenSummary   string // "Saturday, May 24 · 6:00 PM"
	TimeFormatted string // "6:00 PM"
	PlaceName     string // "Two Oaks Community Center" or "Location TBD"

	// Host.
	HostName      string // first name only
	HostInitial   string
	HostAvatarURL string

	// Attendees.
	HasAttendees           bool
	AttendeesShown         []eventAttendee
	AttendeesNamesInline   string
	AttendeesOverflowCount int
	// AttendeesSuffix is the localized text rendered right after the bolded
	// names, e.g. " + 3 others going" (overflow) or " going" (none). Carries
	// its own leading space; plural- and locale-aware (web.event.attendees_*).
	AttendeesSuffix string

	// Capacity.
	IsAtCapacity bool
	NumMembers   int32
	MaxMembers   int32

	// AndroidPackage is this deployment's published Android applicationId,
	// from --android-package-id. Embedded into the open-in-app Intent URI so
	// Chrome on Android force-opens that build — installed users bypass the
	// SSR page; uninstalled users fall through to the browser_fallback_url
	// store listing. Empty omits the intent fallback and leaves the plain web
	// link, which is what an instance with no published Android build wants.
	AndroidPackage string

	// Str holds the localized static labels (CTAs, aria labels, capacity
	// banner, open-in-app hint, JS strings) for the template.
	Str eventStrings
}

// eventAttendee is one row in the rendered avatar cluster.
type eventAttendee struct {
	Name      string
	Initial   string
	AvatarURL string
}

// handleEventLanding renders the SSR event landing page for a
// share link whose target oneof is experience_id. It assumes the
// caller has already loaded the ShareLink row and validated that
// it isn't revoked.
//
// On invalid event state (soft-deleted, cancelled, completed,
// missing hosting community) it falls back to the existing
// invite-page error template — same surface a missing share link
// returns today.
func (s *Service) handleEventLanding(w http.ResponseWriter, r *http.Request, shareLink *models.ShareLink) {
	ctx := r.Context()
	shortCode := shareLink.ShortCode
	experienceID := shareLink.GetExperienceId()

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "HandleEventLanding",
		"short_code", logging.MaskToken(shortCode),
		"community_id", shareLink.CommunityId,
		"experience_id", experienceID,
	)

	// One localizer per request (Accept-Language), threaded into the build
	// and error-render paths so the landing renders in the visitor's locale.
	loc := s.requestLocalizer(ctx)

	// 1. Experience. Pass IncludeDeleted=true so we can render the
	// "no longer available" page for soft-deleted events instead of
	// failing with a generic storage error.
	experience := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, experience, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.InfoContext(ctx, "share link points at non-existent experience")
			s.renderInviteError(w, ctx, loc, shortCode, "web.error.event_unavailable")
			return
		}
		logger.ErrorContext(ctx, "failed to fetch experience", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}
	if !isEventRSVPable(experience) {
		logger.InfoContext(ctx, "event not in an RSVPable state",
			"experience_state", experience.State.String(),
			"experience_deleted", experience.Deleted != nil)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.event_unavailable")
		return
	}

	// 2. Hosting community.
	community := &models.Community{}
	if err := s.storage.GetByID(ctx, shareLink.CommunityId, community); err != nil {
		logger.ErrorContext(ctx, "failed to fetch community", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}
	if community.Deleted != nil {
		logger.InfoContext(ctx, "hosting community is soft-deleted")
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.event_unavailable")
		return
	}

	// 3. Location (best-effort — null/missing is rendered as the localized
	// "Location TBD" label).
	placeName := s.fetchPlaceName(ctx, loc, experience.LocationId, logger)

	// 4. RSVPs for this experience scoped to the hosting community.
	// Dedup by user_id (keep latest by last_updated) so a user with
	// multiple RSVP rows isn't double-counted in the attendee list
	// or the "+ N others" overflow.
	rawRSVPs, err := s.fetchAttendeeRSVPs(ctx, experienceID, shareLink.CommunityId)
	if err != nil {
		logger.Warn("failed to fetch RSVPs — rendering with empty attendee list", "error", err)
		rawRSVPs = nil
	}
	dedupedRSVPs := dedupYesRSVPs(rawRSVPs)

	// 5. Batched user fetch — host + first 8 RSVPed attendees in a single query.
	attendeeUserIDs := topAttendeeUserIDs(dedupedRSVPs, maxFetchedAttendees)
	allUserIDs := attendeeUserIDs
	if experience.OwnerId != "" {
		allUserIDs = append([]string{experience.OwnerId}, attendeeUserIDs...)
	}
	usersByID, err := s.fetchUsersByIDs(ctx, allUserIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch-fetch users", "error", err)
		s.renderInviteError(w, ctx, loc, shortCode, "web.error.generic")
		return
	}

	host, hostOK := usersByID[experience.OwnerId]
	if !hostOK || host == nil {
		logger.Warn("event host user not found — rendering with empty host name",
			"host_user_id", experience.OwnerId)
		host = &models.User{Id: experience.OwnerId}
	}

	// 6. Batched media fetch — hero (experience media[0]) + community
	// thumb (for fallback) + every attendee + host avatar in one query.
	mediaIDs := collectMediaIDs(experience, community, host, usersByID, attendeeUserIDs)
	mediaByID, err := s.fetchMediaByIDs(ctx, mediaIDs)
	if err != nil {
		logger.Warn("failed to batch-fetch media — falling back to default OG image", "error", err)
		mediaByID = nil
	}

	// 7. Community member count (for at-capacity check).
	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, shareLink.CommunityId)
	if err != nil {
		logger.Warn("failed to get community member count — assuming not at capacity", "error", err)
		memberCount = 0
	}

	// Build template data.
	data := s.buildEventPageData(ctx, loc, shortCode, experience, community, host,
		usersByID, dedupedRSVPs, attendeeUserIDs, mediaByID, placeName, memberCount)

	logger.InfoContext(ctx, "serving event landing page",
		"is_at_capacity", data.IsAtCapacity,
		"num_attendees", len(data.AttendeesShown),
		"num_attendees_overflow", data.AttendeesOverflowCount,
		"has_hero", data.HeroImageURL != "" && data.HeroImageURL != s.defaultOGImageURL(),
	)

	s.renderEventPage(w, data)
}

// renderEventPage executes the event template and writes the response.
// Sets a short SSR cache TTL — 30 seconds — to tolerate event
// cancellation without ETag complexity.
//
// TODO(#2051): the landing becomes personalized when the session-
// cookie variant lands ("You're going" / "Maybe" instead of the
// three RSVP CTAs). At that point switch this from
// `Cache-Control: public` to either `private` or add `Vary: Cookie`
// so a CDN doesn't serve one visitor's personalized HTML to
// another.
func (s *Service) renderEventPage(w http.ResponseWriter, data eventPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Vary", "Accept-Language")
	if err := s.templates.ExecuteTemplate(w, "event.html", data); err != nil {
		logging.Default().Error("failed to render event template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// isEventRSVPable reports whether an experience is in a state
// where a new visitor could meaningfully see it. Cancelled and
// completed events return false (no point inviting people to a
// past or aborted event). Soft-deleted events also return false.
// IN_PROCESS (the event is happening right now) is intentionally
// allowed — late RSVPs are valid for ongoing events.
func isEventRSVPable(exp *models.Experience) bool {
	if exp.Deleted != nil {
		return false
	}
	switch exp.State {
	case models.ExperienceState_EXPERIENCE_STATE_CANCELLED,
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED:
		return false
	default:
		return true
	}
}

// fetchPlaceName resolves an experience.location_id to a
// human-readable place label for the meta-pill. Returns the localized
// "Location TBD" label if location_id is empty, the location row
// can't be found, or it has no usable name/address fields.
func (s *Service) fetchPlaceName(ctx context.Context, loc *l10n.Localizer, locationID string, logger *logging.Logger) string {
	tbd := loc.T(ctx, "web.event.location_tbd", nil)
	if locationID == "" {
		return tbd
	}
	place := &models.Location{}
	if err := s.storage.GetByID(ctx, locationID, place); err != nil {
		logger.Warn("failed to fetch location for event landing — using TBD label",
			"error", err, "location_id", locationID)
		return tbd
	}
	if name := place.GetName(); name != "" {
		return name
	}
	if place.Address != nil && place.Address.Locality != "" {
		return place.Address.Locality
	}
	return tbd
}

// fetchAttendeeRSVPs loads YES-RSVPs for an experience scoped to
// the hosting community. Caller is expected to log on error and
// degrade gracefully (empty attendee list). Soft-deleted rows are
// filtered by the storage layer (default QueryOptions behavior).
func (s *Service) fetchAttendeeRSVPs(ctx context.Context, experienceID, communityID string) ([]*models.ExperienceRSVP, error) {
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": experienceID,
		"community_id":  communityID,
		"intention":     int32(models.RSVPIntention_RSVP_INTENTION_YES),
	}, &models.ExperienceRSVP{})
	if err != nil {
		return nil, err
	}
	rsvps := make([]*models.ExperienceRSVP, len(rows))
	for i, r := range rows {
		rsvps[i] = r.(*models.ExperienceRSVP)
	}
	return rsvps, nil
}

// dedupYesRSVPs collapses multiple rows per user (which can happen
// if intention changes are stored as new rows, or as a defensive
// guard against schema evolution) into the most recent row per
// user_id by last_updated_unix_sec. Returns the deduped slice
// sorted by rsvped_at_unix_sec ascending so callers can take the
// first N as "earliest committers." Mirrors the dedup pattern
// used by experience/service.go's buildRSVPs.
//
// Provisional users (empty user_id) are dropped — we can't render a
// real name/photo for them on the landing.
func dedupYesRSVPs(rsvps []*models.ExperienceRSVP) []*models.ExperienceRSVP {
	if len(rsvps) == 0 {
		return nil
	}
	byUser := make(map[string]*models.ExperienceRSVP, len(rsvps))
	for _, r := range rsvps {
		if r.UserId == "" {
			continue
		}
		if existing, ok := byUser[r.UserId]; !ok || r.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
			byUser[r.UserId] = r
		}
	}
	sorted := make([]*models.ExperienceRSVP, 0, len(byUser))
	for _, r := range byUser {
		sorted = append(sorted, r)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].RsvpedAtUnixSec < sorted[j].RsvpedAtUnixSec
	})
	return sorted
}

// topAttendeeUserIDs picks the first `limit` user_ids from a
// deduped RSVP list (see dedupYesRSVPs).
func topAttendeeUserIDs(deduped []*models.ExperienceRSVP, limit int) []string {
	if limit > len(deduped) {
		limit = len(deduped)
	}
	out := make([]string, limit)
	for i := range limit {
		out[i] = deduped[i].UserId
	}
	return out
}

// fetchUsersByIDs is a thin wrapper that returns a typed map
// keyed by user_id. Unfound IDs simply aren't in the result map.
func (s *Service) fetchUsersByIDs(ctx context.Context, ids []string) (map[string]*models.User, error) {
	out := make(map[string]*models.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw, err := s.storage.GetByIDs(ctx, ids, &models.User{})
	if err != nil {
		return nil, err
	}
	for id, m := range raw {
		out[id] = m.(*models.User)
	}
	return out, nil
}

// fetchMediaByIDs returns a typed map keyed by media_id.
func (s *Service) fetchMediaByIDs(ctx context.Context, ids []string) (map[string]*models.Media, error) {
	out := make(map[string]*models.Media, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	raw, err := s.storage.GetByIDs(ctx, ids, &models.Media{})
	if err != nil {
		return nil, err
	}
	for id, m := range raw {
		out[id] = m.(*models.Media)
	}
	return out, nil
}

// collectMediaIDs returns the deduplicated list of media IDs to
// batch-fetch — experience hero (media[0]), community thumb
// (media[0], used as fallback), host avatar (media[0]), and each
// attendee's avatar.
func collectMediaIDs(exp *models.Experience, community *models.Community,
	host *models.User, usersByID map[string]*models.User, attendeeIDs []string,
) []string {
	seen := make(map[string]struct{})
	add := func(id string) {
		if id == "" {
			return
		}
		seen[id] = struct{}{}
	}
	if len(exp.MediaIds) > 0 {
		add(exp.MediaIds[0])
	}
	if len(community.MediaIds) > 0 {
		add(community.MediaIds[0])
	}
	if host != nil && len(host.MediaIds) > 0 {
		add(host.MediaIds[0])
	}
	for _, uid := range attendeeIDs {
		if u, ok := usersByID[uid]; ok && u != nil && len(u.MediaIds) > 0 {
			add(u.MediaIds[0])
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// buildEventPageData composes the template payload from already-
// fetched entities. Doesn't issue any storage queries itself —
// the presigned-URL bucket calls are the only outbound I/O.
func (s *Service) buildEventPageData(ctx context.Context, loc *l10n.Localizer, shortCode string,
	experience *models.Experience, community *models.Community, host *models.User,
	usersByID map[string]*models.User, rsvps []*models.ExperienceRSVP,
	attendeeUserIDs []string, mediaByID map[string]*models.Media,
	placeName string, memberCount int,
) eventPageData {
	dayLabel, whenShort, whenSummary, timeFormatted := formatEventWhen(ctx, loc, experience.Time)

	// Hero image — falls back to community thumb, then default Ripls logo.
	heroImageURL := s.resolveImageURL(ctx, experience.MediaIds, mediaByID)
	if heroImageURL == "" {
		heroImageURL = s.resolveImageURL(ctx, community.MediaIds, mediaByID)
	}
	// A real photo (experience or community) vs the default Ripls logo. The
	// phone-first RSVP screen only uses the hero as a dark-washed backdrop when
	// it's a real photo — the default logo would just look muddy.
	hasHeroImage := heroImageURL != ""
	if heroImageURL == "" {
		heroImageURL = s.defaultOGImageURL()
	}

	hostName := firstName(host.Name)
	hostAvatarURL := s.resolveImageURL(ctx, host.MediaIds, mediaByID)

	// Build attendee list (excluding the host — they're shown separately).
	shown := make([]eventAttendee, 0, len(attendeeUserIDs))
	names := make([]string, 0, len(attendeeUserIDs))
	for _, uid := range attendeeUserIDs {
		if uid == experience.OwnerId {
			continue // don't list the host twice
		}
		u, ok := usersByID[uid]
		if !ok || u == nil {
			continue
		}
		fn := firstName(u.Name)
		names = append(names, fn)
		if len(shown) < maxShownAttendees {
			shown = append(shown, eventAttendee{
				Name:      fn,
				Initial:   firstInitial(fn),
				AvatarURL: s.resolveImageURL(ctx, u.MediaIds, mediaByID),
			})
		}
	}

	// Total YES-RSVPs minus the host (if the host RSVPed) gives the
	// "going" count. We use the actual rsvps slice — it may include
	// users we didn't hydrate (those beyond maxFetchedAttendees).
	totalGoing := 0
	for _, r := range rsvps {
		if r.UserId == "" || r.UserId == experience.OwnerId {
			continue
		}
		totalGoing++
	}
	overflow := max(0, totalGoing-len(shown))

	// Localized suffix after the bolded names. When there's overflow the
	// plural selects on the overflow count; otherwise it selects on the
	// shown-name count (so verb-conjugating locales agree). Empty when no
	// attendees are shown (the block is hidden).
	var attendeesSuffix string
	if len(shown) > 0 {
		if overflow > 0 {
			attendeesSuffix = loc.TCount(ctx, "web.event.attendees_overflow", overflow, nil)
		} else {
			attendeesSuffix = loc.TCount(ctx, "web.event.attendees_going", len(shown), nil)
		}
	}

	atCapacity := memberCount >= communitylib.MaxCommunityMembers

	// Localized title/OG copy — the in-community variant drops cleanly for a
	// nameless community.
	titleData := map[string]any{"Host": hostName, "Event": experience.Name, "Community": community.GetName()}
	titleKey := "web.event.og_title"
	if community.GetName() != "" {
		titleKey = "web.event.og_title_in_community"
	}

	return eventPageData{
		Lang:                   langAttr(loc),
		BaseURL:                s.baseURL(),
		Brand:                  s.branding,
		ShortCode:              shortCode,
		ExperienceID:           experience.Id,
		EventName:              experience.Name,
		Title:                  loc.T(ctx, titleKey, titleData),
		CommunityName:          communityDisplayName(ctx, loc, community),
		CommunityIsNamed:       community.GetName() != "",
		HeroImageURL:           heroImageURL,
		HasHeroImage:           hasHeroImage,
		DayLabel:               dayLabel,
		WhenShort:              whenShort,
		WhenSummary:            whenSummary,
		TimeFormatted:          timeFormatted,
		PlaceName:              placeName,
		HostName:               hostName,
		HostInitial:            firstInitial(hostName),
		HostAvatarURL:          hostAvatarURL,
		HasAttendees:           len(shown) > 0,
		AttendeesShown:         shown,
		AttendeesNamesInline:   joinNames(names, len(shown)),
		AttendeesOverflowCount: overflow,
		AttendeesSuffix:        attendeesSuffix,
		IsAtCapacity:           atCapacity,
		NumMembers:             int32(memberCount),
		MaxMembers:             int32(communitylib.MaxCommunityMembers),
		AndroidPackage:         s.branding.AndroidPackageID,
		Str:                    buildEventStrings(ctx, loc, int32(memberCount), int32(communitylib.MaxCommunityMembers)),
	}
}

// The Android package name now comes from --android-package-id
// (branding.Config.AndroidPackageID) rather than being inferred from the
// hostname. The old heuristic mapped "dev." / "localhost" prefixes to a
// hardcoded dev package and everything else to a hardcoded prod one, which
// baked one publisher's identifiers into the server and silently mislabeled
// any host that didn't fit the naming convention. Each deployment now states
// the package its own build ships as; unset omits the intent:// fallback and
// leaves the plain web link.

// resolveImageURL takes the first media ID off a list and turns
// it into a presigned URL by looking up the row in the pre-batched
// mediaByID map (no storage I/O). Returns empty when there's no
// media or the bucket signing fails — caller decides what to show.
func (s *Service) resolveImageURL(ctx context.Context, mediaIDs []string, mediaByID map[string]*models.Media) string {
	if len(mediaIDs) == 0 || mediaByID == nil {
		return ""
	}
	media, ok := mediaByID[mediaIDs[0]]
	if !ok || media == nil {
		return ""
	}
	// Prefer the thumbnail variant when available — smaller, EXIF
	// rotation baked in, less likely to come back rotated on
	// platforms like Signal.
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	if media.GetThumbnailStorageUrl() != "" {
		bucketKey = storage.ThumbnailBucketKey(media.UserId, media.Id)
	}
	url, err := s.bucket.GetSignedURL(ctx, bucketKey, ogImageDuration)
	if err != nil {
		logging.LoggerWithContext(ctx).Warn(
			"failed to generate presigned URL for event landing image",
			"error", err, "media_id", media.Id,
		)
		return ""
	}
	return url
}

// joinNames formats "Alice, Bob, Carol, Dave" with commas; the
// final "and" is omitted to keep the line scannable. shownCount
// is the cap — names beyond it are dropped (the overflow counter
// handles them separately).
func joinNames(names []string, shownCount int) string {
	if len(names) == 0 {
		return ""
	}
	if shownCount > len(names) {
		shownCount = len(names)
	}
	return strings.Join(names[:shownCount], ", ")
}

// firstInitial returns the first rune of a name, uppercased.
// Falls back to "?" when the name is empty.
func firstInitial(name string) string {
	if name == "" {
		return "?"
	}
	r, _ := utf8.DecodeRuneInString(name)
	return strings.ToUpper(string(r))
}
