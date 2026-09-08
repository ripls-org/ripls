package notifications

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/eventtime"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

const (
	// maxSummaryDescription bounds the description shown in the email card.
	maxSummaryDescription = 240
	// heroImageMaxBytes caps the inline hero so a huge original doesn't bloat the
	// message; oversized media is skipped and the card renders without a photo.
	heroImageMaxBytes = 2 << 20 // 2 MiB
)

// buildEmailExtras assembles the rich-card summary and the inline hero image for
// an off-app notification email from the community-event payload — the email
// analogue of the SSR landing-page summary. It is best-effort: any storage or
// bucket miss degrades gracefully (nil summary / no image) so the email still
// sends. loc renders the summary labels in the recipient's locale.
func (s *service) buildEmailExtras(ctx context.Context, ev *models.CommunityEventPayload, loc *l10n.Localizer) (*email.ItemSummary, []email.InlineImage, string) {
	if ev == nil {
		return nil, nil, ""
	}
	switch {
	case ev.GetExperienceId() != "":
		return s.experienceEmailExtras(ctx, ev.GetExperienceId(), loc)
	case ev.GetGearId() != "":
		return s.gearEmailExtras(ctx, ev.GetGearId(), ev.GetCommunityId(), loc)
	case ev.GetRequestId() != "":
		return s.requestEmailExtras(ctx, ev.GetRequestId(), loc)
	default:
		return nil, nil, ""
	}
}

func (s *service) experienceEmailExtras(ctx context.Context, experienceID string, loc *l10n.Localizer) (*email.ItemSummary, []email.InlineImage, string) {
	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, exp); err != nil {
		return nil, nil, ""
	}
	if exp.Deleted != nil ||
		exp.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED ||
		exp.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		return nil, nil, ""
	}
	sum := &email.ItemSummary{
		Eyebrow:     loc.T(ctx, "email.summary.eyebrow.event", nil),
		Name:        exp.Name,
		Date:        formatSummaryDate(exp.Time),
		Description: truncateDescription(exp.Description),
	}
	if owner := s.userFirstName(ctx, exp.OwnerId); owner != "" {
		sum.OwnerLine = loc.T(ctx, "email.summary.hosted_by", map[string]any{"Name": owner})
	}
	if n := s.countGoing(ctx, experienceID); n > 0 {
		sum.Going = loc.T(ctx, "email.summary.going", map[string]any{"Count": n})
	}
	inline, heroURL := s.heroInline(ctx, exp.MediaIds)
	return sum, inline, heroURL
}

func (s *service) gearEmailExtras(ctx context.Context, gearID, communityID string, loc *l10n.Localizer) (*email.ItemSummary, []email.InlineImage, string) {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return nil, nil, ""
	}
	if gear.Deleted != nil || gear.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
		return nil, nil, ""
	}
	eyebrowKey := "email.summary.eyebrow.loan"
	if s.gearIsGiveaway(ctx, gearID, communityID) {
		eyebrowKey = "email.summary.eyebrow.giveaway"
	}
	sum := &email.ItemSummary{
		Eyebrow:     loc.T(ctx, eyebrowKey, nil),
		Name:        gear.Name,
		Description: truncateDescription(gear.Description),
	}
	if owner := s.userFirstName(ctx, gear.OwnerId); owner != "" {
		sum.OwnerLine = loc.T(ctx, "email.summary.shared_by", map[string]any{"Name": owner})
	}
	inline, heroURL := s.heroInline(ctx, gear.MediaIds)
	return sum, inline, heroURL
}

func (s *service) requestEmailExtras(ctx context.Context, requestID string, loc *l10n.Localizer) (*email.ItemSummary, []email.InlineImage, string) {
	req := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, req); err != nil {
		return nil, nil, ""
	}
	if req.Deleted != nil ||
		req.State == models.RequestState_REQUEST_STATE_CANCELLED ||
		req.State == models.RequestState_REQUEST_STATE_FULFILLED {
		return nil, nil, ""
	}
	sum := &email.ItemSummary{
		Eyebrow:     loc.T(ctx, "email.summary.eyebrow.request", nil),
		Name:        req.Title,
		Description: truncateDescription(req.Description),
	}
	if owner := s.userFirstName(ctx, req.RequesterId); owner != "" {
		sum.OwnerLine = loc.T(ctx, "email.summary.requested_by", map[string]any{"Name": owner})
	}
	inline, heroURL := s.heroInline(ctx, req.MediaIds)
	return sum, inline, heroURL
}

// heroInline downloads the first media item's bytes from the bucket and returns
// it as an inline image plus the "cid:" src that references it. Embedding the
// bytes (rather than a presigned URL) means the hero renders whenever the mail
// is opened. Returns (nil, "") when there is no media, no bucket, the object is
// missing, or it exceeds heroImageMaxBytes.
func (s *service) heroInline(ctx context.Context, mediaIDs []string) ([]email.InlineImage, string) {
	if len(mediaIDs) == 0 || s.offAppEmail.Bucket == nil {
		return nil, ""
	}
	media := &models.Media{}
	if err := s.storage.GetByID(ctx, mediaIDs[0], media); err != nil {
		return nil, ""
	}
	// Prefer the thumbnail (orientation baked in, smaller) when present.
	key := storage.MediaBucketKey(media.UserId, media.Id)
	if media.GetThumbnailStorageUrl() != "" {
		key = storage.ThumbnailBucketKey(media.UserId, media.Id)
	}
	data, contentType, err := s.offAppEmail.Bucket.Get(ctx, key)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"off-app email: hero image fetch failed", "media_id", media.Id, "error", err)
		return nil, ""
	}
	if len(data) == 0 || len(data) > heroImageMaxBytes {
		return nil, ""
	}
	filename := "hero." + imageExt(contentType)
	return []email.InlineImage{{Filename: filename, Data: data}}, "cid:" + filename
}

// gearIsGiveaway reports whether the gear is shared for giveaway in the
// community (vs. loan), read from the CommunityGear junction.
func (s *service) gearIsGiveaway(ctx context.Context, gearID, communityID string) bool {
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil || len(rows) == 0 {
		return false
	}
	cg, ok := rows[0].(*models.CommunityGear)
	return ok && cg.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY
}

// countGoing returns the number of YES RSVPs for an experience.
func (s *service) countGoing(ctx context.Context, experienceID string) int {
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": experienceID,
	}, &models.ExperienceRSVP{})
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if rsvp, ok := r.(*models.ExperienceRSVP); ok && rsvp.GetIntention() == models.RSVPIntention_RSVP_INTENTION_YES {
			n++
		}
	}
	return n
}

// userFirstName returns a user's first name, or "" on any miss.
func (s *service) userFirstName(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	u := &models.User{}
	if err := s.storage.GetByID(ctx, userID, u); err != nil {
		return ""
	}
	return summaryFirstName(u.Name)
}

// summaryFirstName returns the leading whitespace-separated token of name.
func summaryFirstName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// truncateDescription trims and rune-safely caps a description for the card.
func truncateDescription(d string) string {
	d = strings.TrimSpace(d)
	r := []rune(d)
	if len(r) <= maxSummaryDescription {
		return d
	}
	return strings.TrimSpace(string(r[:maxSummaryDescription])) + "…"
}

// imageExt maps a content type to a filename extension for the inline CID part.
func imageExt(contentType string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return "png"
	case strings.Contains(contentType, "webp"):
		return "webp"
	case strings.Contains(contentType, "gif"):
		return "gif"
	default:
		return "jpg"
	}
}

// formatSummaryDate renders an experience time as a short "Month Day" line, or
// "" when the time is TBD/unset. Mirrors the SSR landing page's date format.
func formatSummaryDate(t *models.ExperienceTime) string {
	m, ok := eventtime.Resolve(t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s %d", m.Time.Format("January"), m.Time.Day())
}
