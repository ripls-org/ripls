package workshop

import (
	"sort"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services/workshop/momentum"
)

// maxBriefPriorities is the maximum number of priorities the brief prose
// paragraph includes as inline levers. Lower-ranked items become CTA rows.
const maxBriefPriorities = 3

// maxBriefCTARows caps the secondary CTA row list. Surfacing-discipline:
// the screen reads as one paragraph + a short row list, not a wall.
const maxBriefCTARows = 5

// interestTierBoost lifts nudges that surface explicit "someone else
// wants this" signal above every other slot in the ranking. Set well
// above the unix-second range so the interest tier strictly dominates
// the recency tier — no amount of recency lifts a non-interest nudge
// above an interest one.
const interestTierBoost int64 = 1_000_000_000_000

// interestKickers are the kicker labels emitted by detectors that
// surface explicit interest from others — currently the two ESM
// Repeat Signal variants. The slot identity is otherwise lost when
// nudges persist (every workshop detector maps to
// NUDGE_SURFACE_WORKSHOP_HERO_CARD), so kicker lookup is the cheapest
// reliable signal-source check on a StoredNudge.
//
// New "interest" detectors should opt in by reusing one of these
// kickers or extending the set.
var interestKickers = map[string]bool{
	"Worth doing again": true, // ESM Repeat — open window
	"Pulse came back":   true, // ESM Repeat — closed window
}

// hasInterestFromOthers reports whether `n` represents an explicit
// "someone else wants this" signal. Drives the two-tier ranking in
// `Rank` — interest nudges sort above everything else; recency
// breaks ties within each tier.
func hasInterestFromOthers(n *models.StoredNudge) bool {
	if n == nil || n.KickerLabel == nil {
		return false
	}
	return interestKickers[*n.KickerLabel]
}

// BriefRanker selects and ranks the top priorities from a set of
// Workshop StoredNudge rows for inclusion in the daily brief.
type BriefRanker struct{}

// RankResult holds the output of ranking a set of nudges.
type RankResult struct {
	// TopPriorities are the top maxBriefPriorities nudges ranked highest,
	// suitable for inline prose levers.
	TopPriorities []momentum.BriefPriority

	// CTARows are additional priorities below the top set, suitable for
	// the secondary compact row list. Deduped by rhythm-key (normalized
	// event name) against the top priorities and capped at
	// maxBriefCTARows. Ordered to favor cta_action diversity.
	CTARows []momentum.BriefCTARow
}

// Rank selects and orders the top priorities from nudges. It returns
// up to maxBriefPriorities items for prose levers and any remainder as
// CTA rows. The ranking is deterministic given the same input set —
// ties are broken by created_at_unix_sec descending (newest first).
//
// Dedup rules:
//   - Multiple nudges sharing the same rhythm key (normalized headline
//     event-name) collapse to one — the highest-ranked instance wins.
//     Prevents the actual.png "Round 2 of Morning Hike at Chautauqua"
//     repeated five times.
//   - When a context_id appears in the top priorities, any CTA row
//     with the same context_id is dropped.
//   - CTA rows are ordered by `cta_action` diversity: alternate
//     between distinct actions before repeating one.
//   - CTA rows cap at `maxBriefCTARows`.
func (*BriefRanker) Rank(nudges []*models.StoredNudge) RankResult {
	if len(nudges) == 0 {
		return RankResult{}
	}

	type scored struct {
		nudge      *models.StoredNudge
		score      int64
		rhythmKey  string
		contextKey string
	}

	// Two-tier score: nudges that surface explicit interest from
	// others (currently the ESM Repeat Signal variants — see
	// `hasInterestFromOthers`) are boosted strictly above every other
	// nudge. Within each tier, more-recent nudges sort first.
	candidates := make([]scored, 0, len(nudges))
	for _, n := range nudges {
		if !isBriefNudgeRenderable(n) {
			continue
		}
		score := n.CreatedAtUnixSec
		if hasInterestFromOthers(n) {
			score += interestTierBoost
		}
		candidates = append(candidates, scored{
			nudge:      n,
			score:      score,
			rhythmKey:  rhythmKeyFromNudge(n),
			contextKey: stringPtrVal(n.ContextId),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Dedup by rhythm key — keep the highest-scored entry per rhythm.
	seenRhythm := map[string]bool{}
	deduped := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if c.rhythmKey != "" && seenRhythm[c.rhythmKey] {
			continue
		}
		if c.rhythmKey != "" {
			seenRhythm[c.rhythmKey] = true
		}
		deduped = append(deduped, c)
	}

	// Split into top priorities (inline levers) and CTA-row candidates.
	topCount := maxBriefPriorities
	if topCount > len(deduped) {
		topCount = len(deduped)
	}
	topSlice := deduped[:topCount]
	rest := deduped[topCount:]

	result := RankResult{}
	topContextIDs := map[string]bool{}
	for _, c := range topSlice {
		p := nudgeToBriefPriority(c.nudge, c.score)
		result.TopPriorities = append(result.TopPriorities, p)
		if p.ContextID != "" {
			topContextIDs[p.ContextID] = true
		}
	}

	// Filter rest by context_id collision against top priorities, then
	// reorder to favor cta_action diversity, then cap.
	rowCandidates := make([]momentum.BriefCTARow, 0, len(rest))
	for _, c := range rest {
		if c.contextKey != "" && topContextIDs[c.contextKey] {
			continue
		}
		p := nudgeToBriefPriority(c.nudge, c.score)
		rowCandidates = append(rowCandidates, momentum.BriefCTARow{
			Headline:       p.Headline,
			AtmosphereLine: p.AtmosphereLine,
			CtaAction:      p.CtaAction,
			ContextID:      p.ContextID,
			CommunityID:    p.CommunityID,
		})
	}
	result.CTARows = diversifyAndCap(rowCandidates, maxBriefCTARows)
	return result
}

// rhythmKeyFromNudge derives a stable rhythm key from a nudge — the
// normalized first sentence of its headline. Used for dedup so two
// nudges naming the same event ("Round 2 of Morning Hike at Chautauqua
// is the natural next move") collapse to one.
//
// When the headline is empty, falls back to context_id so different
// events don't collide on an empty key.
func rhythmKeyFromNudge(n *models.StoredNudge) string {
	if n == nil {
		return ""
	}
	headline := strings.ToLower(strings.TrimSpace(n.Headline))
	if headline == "" {
		return stringPtrVal(n.ContextId)
	}
	// Use the first 60 chars as the rhythm key — captures the rhythm
	// name without being noise-sensitive to round-number suffixes.
	if len(headline) > 60 {
		headline = headline[:60]
	}
	return headline
}

// diversifyAndCap reorders rows so distinct cta_action values
// alternate before any single action repeats. Caps the result at max.
//
// Example: 3 schedule_repeat + 2 propose_share + 1 post_in_chat becomes
// schedule_repeat, propose_share, post_in_chat, schedule_repeat,
// propose_share, schedule_repeat — diverse first, repeats trail.
func diversifyAndCap(rows []momentum.BriefCTARow, hi int) []momentum.BriefCTARow {
	if len(rows) <= 1 {
		if len(rows) > hi {
			return rows[:hi]
		}
		return rows
	}
	// Bucket rows by cta_action, preserving original order within each bucket.
	buckets := map[string][]momentum.BriefCTARow{}
	keys := []string{}
	for _, r := range rows {
		if _, ok := buckets[r.CtaAction]; !ok {
			keys = append(keys, r.CtaAction)
		}
		buckets[r.CtaAction] = append(buckets[r.CtaAction], r)
	}
	out := make([]momentum.BriefCTARow, 0, len(rows))
	for len(out) < len(rows) && len(out) < hi {
		picked := false
		for _, k := range keys {
			if len(buckets[k]) == 0 {
				continue
			}
			out = append(out, buckets[k][0])
			buckets[k] = buckets[k][1:]
			picked = true
			if len(out) >= hi {
				break
			}
		}
		if !picked {
			break
		}
	}
	return out
}

// isBriefNudgeRenderable returns true when the nudge is eligible for the
// brief — same Workshop-surface check as the feed, but brief-specific:
// consumed or deleted nudges are excluded.
func isBriefNudgeRenderable(n *models.StoredNudge) bool {
	if n.ConsumedAtUnixSec != nil {
		return false
	}
	if n.Deleted != nil {
		return false
	}
	switch n.Surface {
	case models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK,
		models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_SEED_CATALYST:
		return true
	}
	return false
}

// surfaceToSlot maps a NudgeSurface enum value to a slot rank key when the
// nudge's cta_action doesn't directly indicate the slot.
func surfaceToSlot(s models.NudgeSurface) string {
	switch s {
	case models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD:
		return "active_quest"
	case models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK:
		return "bring_back"
	case models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_SEED_CATALYST:
		return "seed_catalyst"
	}
	return ""
}

// nudgeToBriefPriority converts a StoredNudge into a BriefPriority.
func nudgeToBriefPriority(n *models.StoredNudge, score int64) momentum.BriefPriority {
	p := momentum.BriefPriority{
		Slot:        surfaceToSlot(n.Surface),
		KickerLabel: stringPtrVal(n.KickerLabel),
		Headline:    n.Headline,
		Description: n.Description,
		CtaLabel:    n.CtaLabel,
		CtaAction:   n.CtaAction,
		ContextID:   stringPtrVal(n.ContextId),
		CommunityID: n.CommunityId,
		RankScore:   score,
	}
	if n.AtmosphereLine != nil {
		p.AtmosphereLine = *n.AtmosphereLine
	}
	return p
}

// stringPtrVal returns the string value of a pointer, or "" when nil.
func stringPtrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
