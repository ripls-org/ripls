package momentum

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.ripls.org/ripls/server/esm"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// RecentEventLookbackDays is the default window for "recently
// completed events" the per-event Hero card model walks. Mirrors the
// 14-day window in docs/ai/workshop.md §3.1.
const RecentEventLookbackDays = 14

// RecentlyCompletedEvents returns the set of completed experiences
// owned by `userID` and joined to `communityID` whose
// `completed_at_unix_sec` falls within the lookback window. Sorted
// newest-first.
//
// Pure read — no side effects. Skips soft-deleted experiences and
// those without a completion timestamp (an experience can be in
// COMPLETED state without one if storage was hand-edited).
func RecentlyCompletedEvents(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, communityID string,
	now time.Time,
	lookbackDays int,
) ([]*models.Experience, error) {
	if lookbackDays <= 0 {
		lookbackDays = RecentEventLookbackDays
	}
	cutoff := now.Add(-time.Duration(lookbackDays) * 24 * time.Hour).Unix()

	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("RecentlyCompletedEvents: query experiences: %w", err)
	}

	// Build the set of experience ids joined to this community so we
	// can filter the host's experiences down to per-circle scope.
	joins, err := storage.QueryByFields[*models.CommunityExperience](store, ctx,
		map[string]any{"community_id": communityID})
	if err != nil {
		return nil, fmt.Errorf("RecentlyCompletedEvents: query joins: %w", err)
	}
	inCircle := map[string]bool{}
	for _, j := range joins {
		inCircle[j.ExperienceId] = true
	}

	out := make([]*models.Experience, 0)
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		if exp.Deleted != nil {
			continue
		}
		if exp.CompletedAtUnixSec == nil || *exp.CompletedAtUnixSec < cutoff {
			continue
		}
		if !inCircle[exp.Id] {
			continue
		}
		out = append(out, exp)
	}
	sort.Slice(out, func(i, j int) bool {
		return *out[i].CompletedAtUnixSec > *out[j].CompletedAtUnixSec
	})
	return out, nil
}

// DetectForEvent picks the best-fit Hero card slot for a specific
// recently-completed event. The per-event model guarantees every
// recent event produces *something* — when no detector slot fires,
// the function emits a generic recap-style "Schedule {event} again?"
// detection so the host sees a card.
//
// Priority within the per-event chain:
//  1. Active follow-on quest (a future-instance experience already
//     planned with the same name).
//  2. ESM repeat signal (a prompt with ≥60% do_again responses).
//  3. Generic recap fallback (always fires).
//
// Returns nil only on hard storage errors; the fallback path keeps
// the slot fully covered.
func DetectForEvent(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, communityID string,
	exp *models.Experience,
	now time.Time,
) (*Detection, error) {
	if exp == nil {
		return nil, nil
	}

	// 1. Active follow-on quest: do we already have a future-instance
	//    experience with the same name?
	if det, err := activeFollowOnDetection(ctx, store, userID, exp); err != nil {
		return nil, err
	} else if det != nil {
		return det, nil
	}

	// 2. ESM repeat signal for this specific event.
	if det, err := esmSignalDetectionForEvent(ctx, store, exp, now); err != nil {
		return nil, err
	} else if det != nil {
		return det, nil
	}

	// 3. Generic recap fallback — guarantees every recent event
	//    surfaces something.
	return recapFallbackDetection(exp), nil
}

// activeFollowOnDetection returns an active-quest-shaped detection
// when the host has a *not-yet-completed* experience whose name
// matches `exp.Name`. The follow-on becomes the natural "Round N+1"
// framing.
func activeFollowOnDetection(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
	exp *models.Experience,
) (*Detection, error) {
	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("activeFollowOnDetection: %w", err)
	}
	expName := strings.TrimSpace(exp.Name)
	if expName == "" {
		return nil, nil
	}
	priorCount := 0
	for _, e := range hosted {
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED &&
			strings.TrimSpace(e.Name) == expName && e.Deleted == nil {
			priorCount++
		}
	}
	for _, e := range hosted {
		if e.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
			e.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
			continue
		}
		if strings.TrimSpace(e.Name) != expName {
			continue
		}
		return &Detection{
			Slot:     SlotActiveQuest,
			Headline: fmt.Sprintf("Confirm next %s", shortName(exp.Name)),
			Description: fmt.Sprintf(
				"You've hosted it %d times — the circle knows the drill.",
				priorCount,
			),
			CtaLabel:       "Schedule it",
			CtaAction:      "schedule_repeat",
			ContextID:      e.Id,
			KickerLabel:    "On the way",
			AtmosphereLine: fmt.Sprintf("Round %d already on deck", priorCount+1),
		}, nil
	}
	return nil, nil
}

// esmSignalDetectionForEvent inspects the ESM prompts attached to
// `exp` and returns a detection when any prompt has ≥60% do_again
// signal among ≥3 respondents. Verdict-after-pulse variant kicks in
// when the prompt's response window has closed.
func esmSignalDetectionForEvent(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	exp *models.Experience,
	now time.Time,
) (*Detection, error) {
	prompts, err := esm.QueryPromptsByExperience(ctx, store, exp.Id)
	if err != nil {
		return nil, nil // not a hard error — try the next path
	}
	for _, prompt := range prompts {
		if prompt.Deleted != nil {
			continue
		}
		rows, err := esm.QueryResponsesByPrompt(ctx, store, prompt.Id)
		if err != nil {
			continue
		}
		signals := esm.Aggregate(rows, derefInt64(exp.CompletedAtUnixSec))
		for _, sig := range signals {
			if sig.RespondentCount < ESMRepeatSignalMinRespondents {
				continue
			}
			doAgain := sig.ResponseDistribution[RepeatOptionKey]
			ratio := float64(doAgain) / float64(sig.RespondentCount)
			if ratio < ESMRepeatSignalThreshold {
				continue
			}
			// Resolve the names of every attendee whose vote was a
			// `do_again`. The host needs to know exactly who is asking,
			// not just a count. Fall back to the count phrasing when
			// the user lookup fails or returns nothing usable.
			names := respondentDoAgainNames(ctx, store, rows)

			closedWindow := prompt.ClosesAtUnixSec > 0 &&
				prompt.ClosesAtUnixSec <= now.Unix()
			if closedWindow {
				atmosphere := formatRespondentsWantAnother(names)
				if atmosphere == "" {
					atmosphere = fmt.Sprintf("%d of %d said yes, window closed",
						doAgain, sig.RespondentCount)
				}
				return &Detection{
					Slot:     SlotESMRepeatSignal,
					Headline: fmt.Sprintf("Make %s a weekly thing", shortName(exp.Name)),
					// "who answered", not "the circle" — the threshold is
					// ≥60% of respondents, min 2 (#2892).
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
				atmosphere = fmt.Sprintf("%d of %d said yes — pulse came back",
					doAgain, sig.RespondentCount)
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
	return nil, nil
}

// recapFallbackDetection returns a generic "Schedule {event} again"
// detection so every recently-completed event surfaces a Hero card —
// even when no specific slot fires. Per docs/ai/workshop.md §3.1, the
// per-event model guarantees there's always *something* per recent
// event.
//
// Headline is action-shaped ("Schedule X again") and AtmosphereLine
// supplies the signal ("Just wrapped {weekday}") so the postcard reads
// as: do this thing — because of that thing.
func recapFallbackDetection(exp *models.Experience) *Detection {
	wrappedSignal := "Just wrapped"
	if exp.CompletedAtUnixSec != nil && *exp.CompletedAtUnixSec > 0 {
		wd := time.Unix(*exp.CompletedAtUnixSec, 0).Weekday().String()
		wrappedSignal = fmt.Sprintf("Just wrapped %s", wd)
	}
	return &Detection{
		Slot:           SlotActiveQuest, // routes through the active-quest framing for materialization
		Headline:       fmt.Sprintf("Schedule %s again", shortName(exp.Name)),
		Description:    "Worth keeping the rhythm going.",
		CtaLabel:       "Schedule it",
		CtaAction:      "schedule_repeat",
		ContextID:      exp.Id,
		KickerLabel:    "Just wrapped",
		AtmosphereLine: wrappedSignal,
	}
}
