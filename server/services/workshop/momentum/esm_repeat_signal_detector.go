package momentum

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/esm"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// RepeatOptionKey is the canonical option_key that ESM prompts use to
// represent "yes, do this again" responses. The Workshop's threshold check
// looks up exactly this key in `ESMSignal.response_distribution`.
const RepeatOptionKey = "do_again"

// ESMRepeatSignalThreshold is the share of `do_again` responses required
// for a recently-completed experience to surface as a Hero card.
// Per the brief: ≥60%.
const ESMRepeatSignalThreshold = 0.60

// ESMRepeatSignalMinRespondents is the minimum attendee respondent count
// required for the verdict-after-pulse signal to fire. Two yes-votes is
// enough signal for the host to consider another instance — the 60%
// ratio gate (ESMRepeatSignalThreshold) keeps a single yes-out-of-two
// from surfacing.
const ESMRepeatSignalMinRespondents = 2

// ESMRepeatSignalLookbackDays is how far back the detector looks for
// recently-completed experiences. Per the brief: within 7 days.
const ESMRepeatSignalLookbackDays = 7

// ESMRepeatSignalDetector matches cascade priority 2: a recently-completed
// experience the host owns whose ESM signal shows ≥60% repeat intent
// among at least two attendee respondents. Aggregation is computed
// server-internally via `esm.AggregateAttendeesOnly`; no client-facing
// RPC is involved.
type ESMRepeatSignalDetector struct {
	// Now is the wall-clock function the detector uses for the lookback
	// window. Defaults to time.Now when nil — exposed for testability.
	Now func() time.Time
}

// Slot returns SlotESMRepeatSignal.
func (ESMRepeatSignalDetector) Slot() Slot { return SlotESMRepeatSignal }

// Detect inspects the user's recently-completed experiences and returns
// a Detection if any has ≥60% repeat signal among ≥2 attendee respondents.
func (d ESMRepeatSignalDetector) Detect(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, _ string,
) (*Detection, error) {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	cutoff := now().Add(-time.Duration(ESMRepeatSignalLookbackDays) * 24 * time.Hour).Unix()

	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("esm-repeat detector: query experiences: %w", err)
	}

	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		if exp.CompletedAtUnixSec == nil || *exp.CompletedAtUnixSec < cutoff {
			continue
		}

		prompts, err := esm.QueryPromptsByExperience(ctx, store, exp.Id)
		if err != nil {
			continue // a single broken prompt query shouldn't block other experiences
		}
		for _, prompt := range prompts {
			if prompt.Deleted != nil {
				continue
			}
			rows, err := esm.QueryResponsesByPrompt(ctx, store, prompt.Id)
			if err != nil {
				continue
			}
			// Filter to rows where the respondent attended the experience.
			// Story-driven non-attendee votes are still persisted and still
			// feed the inline social-proof tally on the story, but they do
			// not move the verdict-after-pulse threshold — only the people
			// who were actually there should drive the host's "schedule it
			// again" signal.
			signals := esm.AggregateAttendeesOnly(rows, derefInt64(exp.CompletedAtUnixSec))
			for _, sig := range signals {
				if sig.RespondentCount < ESMRepeatSignalMinRespondents {
					continue
				}
				doAgain := sig.ResponseDistribution[RepeatOptionKey]
				ratio := float64(doAgain) / float64(sig.RespondentCount)
				if ratio < ESMRepeatSignalThreshold {
					continue
				}
				// Resolve respondent display names for the action card
				// subtitle. Falls back to a count phrasing when the user
				// lookup fails or returns no usable names so the card
				// still renders.
				names := respondentDoAgainNames(ctx, store, rows)

				// Verdict-after-pulse variant: the prompt's response window
				// has closed and the host should now propose a cadence
				// rather than another single instance. Same slot, same
				// signal, different framing — the lever shifts from
				// "Schedule again?" to "Propose weekly?".
				closedWindow := prompt.ClosesAtUnixSec > 0 &&
					prompt.ClosesAtUnixSec <= now().Unix()
				if closedWindow {
					atmosphere := formatRespondentsWantAnother(names)
					if atmosphere == "" {
						atmosphere = fmt.Sprintf("%d of %d said yes, window closed", doAgain, sig.RespondentCount)
					}
					return &Detection{
						Slot:     SlotESMRepeatSignal,
						Headline: fmt.Sprintf("Make %s a weekly thing", shortName(exp.Name)),
						// Not "most of the circle" (#2892): the threshold is
						// ≥60% of *respondents*, and two respondents clear
						// the minimum. In a circle of twenty that sentence
						// was false.
						Description:    "Most who answered want this on the calendar — propose a cadence.",
						CtaLabel:       "Make it weekly",
						CtaAction:      "schedule_repeat",
						ContextID:      exp.Id,
						KickerLabel:    "Pulse came back",
						AtmosphereLine: atmosphere,
					}, nil
				}
				atmosphere := formatRespondentsWantAnother(names)
				if atmosphere == "" {
					atmosphere = fmt.Sprintf("%d of %d said yes — pulse came back", doAgain, sig.RespondentCount)
				}
				return &Detection{
					Slot:           SlotESMRepeatSignal,
					Headline:       fmt.Sprintf("Schedule %s again", shortName(exp.Name)),
					Description:    "Good time to get it back on the calendar.",
					CtaLabel:       "Schedule it",
					CtaAction:      "schedule_repeat",
					ContextID:      exp.Id,
					KickerLabel:    "Worth doing again",
					AtmosphereLine: atmosphere,
				}, nil
			}
		}
	}
	return nil, nil
}

// respondentDoAgainNames returns the display names of attendees whose
// recorded ESM response selected RepeatOptionKey. Filters mirror the
// AggregateAttendeesOnly / Aggregate predicates so the resulting set is
// exactly the people the threshold check counted as yes-votes. Returns
// nil on storage failure or when no usable names resolve — callers
// fall back to a count-shaped atmosphere line in that case.
func respondentDoAgainNames(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	rows []*models.StoredESMResponse,
) []string {
	userIDs := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if row.RespondentWasAttendee == nil || !*row.RespondentWasAttendee {
			continue
		}
		if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED {
			continue
		}
		if row.ResponseOptionKey == nil || *row.ResponseOptionKey != RepeatOptionKey {
			continue
		}
		if row.UserId == "" || seen[row.UserId] {
			continue
		}
		seen[row.UserId] = true
		userIDs = append(userIDs, row.UserId)
	}
	if len(userIDs) == 0 {
		return nil
	}
	users, err := storage.GetByIDs[*models.User](store, ctx, userIDs)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		u, ok := users[id]
		if !ok {
			continue
		}
		first := firstWord(u.Name)
		if first == "" {
			continue
		}
		names = append(names, first)
	}
	return names
}

// formatRespondentsWantAnother formats a list of respondent display names
// into the host-voiced atmosphere line shown under the action card
// headline. Returns "" when names is empty so the caller can choose its
// own count-shaped fallback string.
//
// Every name is included — the host needs to know exactly who is
// asking. The client truncates to two lines with an "(expand)"
// affordance when the list is too long for the postcard width;
// truncation never happens server-side.
func formatRespondentsWantAnother(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%s wants another", names[0])
	case 2:
		return fmt.Sprintf("%s and %s want another", names[0], names[1])
	default:
		// Oxford-comma list: "A, B, and C want another" /
		// "A, B, C, and D want another" / etc.
		head := strings.Join(names[:len(names)-1], ", ")
		return fmt.Sprintf("%s, and %s want another", head, names[len(names)-1])
	}
}

// firstWord returns the first whitespace-delimited token of s. Display
// names like "Alice Smith" become "Alice" so the atmosphere line stays
// compact on the action card width.
func firstWord(s string) string {
	for _, w := range splitWords(s) {
		if w != "" {
			return w
		}
	}
	return ""
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// shortName clips an experience name to the first 3 words to keep the
// resulting lever copy under the 9-word ceiling. v2's AI generation step
// will produce nicer phrasing.
func shortName(name string) string {
	words := splitWords(name)
	if len(words) <= 3 {
		return name
	}
	return joinWords(words[:3])
}

// splitWords / joinWords avoid pulling in regexp for a hot path.
func splitWords(s string) []string {
	out := []string{}
	cur := []rune{}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func joinWords(ws []string) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += " "
		}
		out += w
	}
	return out
}
