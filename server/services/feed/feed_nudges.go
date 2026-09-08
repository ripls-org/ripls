package feed

import (
	"context"
	"math/rand"
	"time"

	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// appendTerminator returns the content feed with the "you're all caught up"
// terminator card appended.
//
// Generic nudges used to be interleaved here, one every eight items, drawn
// from a per-user pool an LLM wrote and a stock-image fetch illustrated.
// #2936 removed them: a card that invents a reason to prompt you is the
// slop that issue was about, and the Home surface now offers Plan an event /
// Ask for help / Offer something unconditionally instead. The terminator
// survives because it is hand-written copy stating a fact — there is nothing
// more to see.
func (s *Service) appendTerminator(
	ctx context.Context,
	userID string,
	contentItems []*api.FeedItem,
	logger *logging.Logger,
) []*api.FeedItem {
	result := make([]*api.FeedItem, 0, len(contentItems)+1)
	result = append(result, contentItems...)

	// Append terminator.
	terminator := s.buildTerminator(ctx, userID, logger)
	if terminator != nil {
		result = append(result, terminator)
	}

	return result
}

// pruneExpiredNudges returns the fresh subset of active nudges, soft-deleting
// (asynchronously) any older than the 24h freshness window. Shared by the feed
// interleave and the inbox surface so both apply identical expiry.
func (s *Service) pruneExpiredNudges(ctx context.Context, active []*models.StoredNudge) []*models.StoredNudge {
	const nudgeExpiryHours = 24
	now := time.Now().Unix()

	fresh := make([]*models.StoredNudge, 0, len(active))
	for _, n := range active {
		age := now - n.CreatedAtUnixSec
		if age > int64(nudgeExpiryHours)*3600 {
			nudgeID := n.Id
			logging.GoSafe(ctx, "expire-nudge", func() {
				bCtx := context.WithoutCancel(ctx)
				if delErr := softDeleteNudge(bCtx, s.sqlStorage, nudgeID); delErr != nil {
					logging.LoggerWithContext(bCtx).WarnContext(bCtx, "failed to expire nudge", "nudge_id", nudgeID, "error", delErr)
				} else {
					logging.LoggerWithContext(bCtx).InfoContext(bCtx, "nudge expired", "operation", "ExpireNudge", "nudge_id", nudgeID, "reason", "freshness")
				}
			})
			continue
		}
		fresh = append(fresh, n)
	}
	return fresh
}

// InboxNudge returns one nudge for the inbox to render on an empty calendar
// day / zero-state, or nil when none is available. It reuses the feed nudge
// pool (same generation, CTA routing, and ConsumeNudge lifecycle) but takes a
// wider slice of it — see [inboxAudience]: text-only nudges, and the momentum
// engine's host prompts ("schedule {event} again"), which have had no reader
// since the Workshop tab left the nav. It searches across *all* the viewer's
// communities, so a nudge generated for any of them (including by ordinary feed
// browsing) surfaces here. A media-ready nudge is preferred when one exists.
// When nothing is available it kicks off async generation for the first
// community (ready on a subsequent load), so the caller should treat a nil
// return as "no nudge right now," not an error.
//
// This is the exported seam the portfolio service depends on, keeping nudge
// generation/storage owned by the feed service (no cross-service reach-in).
func (s *Service) InboxNudge(ctx context.Context, userID string, communityIDs []string) (*api.NudgePayload, error) {
	if userID == "" || len(communityIDs) == 0 {
		return nil, nil
	}

	// Host prompts only. The generic pool is gone (#2936): a nudge that
	// invents a reason to prompt you was the slop this issue removed, and
	// the Home zero state now always offers Plan an event / Ask for help /
	// Offer something instead. A host prompt is different in kind — it
	// rides a real signal ("schedule your Wednesday run again") about
	// something the viewer actually did, so it still earns the slot above
	// those buttons when the momentum engine fires one.
	for _, communityID := range communityIDs {
		if communityID == "" {
			continue
		}
		active, err := queryActiveNudges(ctx, s.sqlStorage, userID, communityID, inboxAudience)
		if err != nil {
			return nil, err
		}
		for _, n := range s.pruneExpiredNudges(ctx, active) {
			if isHostPrompt(n) {
				return nudgeToPayload(n), nil
			}
		}
	}
	return nil, nil
}

// buildTerminator produces the feed-end "go outside" nudge.
//
// TerminatorsPerDay global records are generated once per day and shared across
// all users and communities — the same 5 headlines, descriptions, and background
// images are served to everyone. On each call, one is chosen at random from
// today's global pool, preferring records that already have imagery.
//
// The triggering user's ID is used only for media ownership when fetching stock
// imagery for the first time; the terminator records themselves are stored under
// the globalTerminatorUserID sentinel.
func (s *Service) buildTerminator(ctx context.Context, userID string, logger *logging.Logger) *api.FeedItem {
	today := time.Now().UTC().YearDay()

	// Soft-delete global terminators from previous days.
	stale, err := getPreviousGlobalTerminators(ctx, s.sqlStorage, today)
	if err != nil {
		logger.WarnContext(ctx, "failed to query previous global terminators", "error", err)
	}
	for _, old := range stale {
		nudgeID := old.Id
		logging.GoSafe(ctx, "delete-stale-terminator", func() {
			bCtx := context.WithoutCancel(ctx)
			_ = softDeleteNudge(bCtx, s.sqlStorage, nudgeID)
		})
	}

	// Fetch today's global pool.
	existing, err := getGlobalTerminatorsForToday(ctx, s.sqlStorage, today)
	if err != nil {
		logger.WarnContext(ctx, "failed to query today's global terminators", "error", err)
	}

	// Fill up to TerminatorsPerDay. Each slot uses a distinct pool entry.
	// The triggering user's ID is used for media ownership only.
	for slot := len(existing); slot < TerminatorsPerDay; slot++ {
		entry := GetTerminatorForSlot(today, slot)
		nudge := &models.StoredNudge{
			Id:               uuid.NewString(),
			UserId:           globalTerminatorUserID,
			NudgeVariant:     3, // Headline Card variant
			Headline:         entry.Headline,
			Description:      entry.Description,
			CtaLabel:         "Plan something",
			CtaAction:        "plan_experience",
			StockQuery:       entry.StockQuery,
			CreatedAtUnixSec: timeNowUnix(),
			IsTerminator:     true,
		}
		if err := insertNudge(ctx, s.sqlStorage, nudge); err != nil {
			logger.WarnContext(ctx, "failed to insert global terminator nudge", "error", err, "slot", slot)
			continue
		}
		nudgeID := nudge.Id
		stockQuery := nudge.StockQuery
		logging.GoSafe(ctx, "fetch-nudge-imagery", func() {
			s.fetchNudgeImagery(context.WithoutCancel(ctx), userID, nudgeID, stockQuery)
		})
		existing = append(existing, nudge)
	}

	// Pick one at random, preferring records that already have imagery.
	return nudgeToFeedItem(pickTerminator(existing))
}

// pickTerminator selects one terminator from candidates, preferring records
// that already have a media ID. Falls back to any record if none have imagery.
func pickTerminator(candidates []*models.StoredNudge) *models.StoredNudge {
	if len(candidates) == 0 {
		// Fallback: return a synthetic record without persistence.
		entry := terminatorFallback
		return &models.StoredNudge{
			Id:           uuid.NewString(),
			NudgeVariant: 3,
			Headline:     entry.Headline,
			Description:  entry.Description,
			CtaLabel:     "Plan something",
			CtaAction:    "plan_experience",
			IsTerminator: true,
		}
	}

	var withMedia []*models.StoredNudge
	for _, n := range candidates {
		if n.MediaId != nil && *n.MediaId != "" {
			withMedia = append(withMedia, n)
		}
	}
	pool := withMedia
	if len(pool) == 0 {
		pool = candidates
	}
	//nolint:gosec // G404: picks which nudge to show for variety. Nothing secret
	// depends on it being unpredictable.
	return pool[rand.Intn(len(pool))]
}

// nudgeToPayload converts a stored nudge into the API NudgePayload shared by
// the feed and any other surface that renders nudges (e.g. the inbox).
func nudgeToPayload(n *models.StoredNudge) *api.NudgePayload {
	mediaIDs := []string{}
	if n.MediaId != nil && *n.MediaId != "" {
		mediaIDs = []string{*n.MediaId}
	}

	stats := make([]*api.NudgeStat, 0, len(n.Stats))
	for _, s := range n.Stats {
		stats = append(stats, &api.NudgeStat{Value: s.Value, Label: s.Label})
	}

	payload := &api.NudgePayload{
		NudgeId:        n.Id,
		NudgeVariant:   n.NudgeVariant,
		Headline:       n.Headline,
		Description:    n.Description,
		CtaLabel:       n.CtaLabel,
		CtaAction:      n.CtaAction,
		MediaIds:       mediaIDs,
		ParticipantIds: n.ParticipantIds,
		Stats:          stats,
	}
	if n.SecondaryCtaLabel != nil {
		payload.SecondaryCtaLabel = n.SecondaryCtaLabel
	}
	if n.SecondaryCtaAction != nil {
		payload.SecondaryCtaAction = n.SecondaryCtaAction
	}
	if n.LocationHint != nil {
		payload.LocationHint = n.LocationHint
	}
	// The entity a host-prompt CTA acts on ("schedule this again" → which
	// event). Feed-surface nudges dispatch by cta_action alone and leave it
	// unset; dropping it for the ones that do carry it lands the host in an
	// empty create modal instead of the pre-filled draft.
	if n.ContextId != nil && *n.ContextId != "" {
		payload.ContextId = n.ContextId
	}
	return payload
}

// nudgeToFeedItem converts a stored nudge into an API FeedItem.
func nudgeToFeedItem(n *models.StoredNudge) *api.FeedItem {
	return &api.FeedItem{
		Id:                    n.Id,
		ItemType:              api.FeedItemType_FEED_ITEM_TYPE_NUDGE,
		OccurredAtUnixSec:     n.CreatedAtUnixSec,
		LastActivityAtUnixSec: n.CreatedAtUnixSec,
		Payload: &api.FeedItem_Nudge{
			Nudge: nudgeToPayload(n),
		},
	}
}
