package momentum

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// BringBackMaxItems is the maximum number of Bring-Something-Back items
// the orchestrator returns. Caps the surface so it never overflows the
// surfacing-discipline budget on the Workshop screen.
const BringBackMaxItems = 3

// BringBackMinInstances is how many completed instances of a name are
// required to consider it "loved" — same threshold as CalendarGap so
// items don't bounce in and out.
const BringBackMinInstances = CalendarGapMinInstances

// BringBackLookbackDays is how far back the orchestrator looks for
// past instances. Default: 365 days (a full year of "things this circle
// has loved" at the most generous edge).
const BringBackLookbackDays = 365

// BringBackMinGapDays is the minimum days-since-last-instance for an
// item to qualify for revival framing. Same threshold as CalendarGap.
const BringBackMinGapDays = CalendarGapMinGapDays

// DetectBringBackItems returns a list of love-revival detections suitable
// for rendering on the Bring-Something-Back surface. Multiple detections
// are returned (capped at BringBackMaxItems), in most-recently-active
// order so the freshest revivals surface first.
//
// Excludes any name returned by the highest-priority Quest hero detection
// (callers pass questHeroContextID) so the same item doesn't appear twice.
func DetectBringBackItems(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, _ string,
	excludeContextID string,
	now time.Time,
) ([]Detection, error) {
	rhythmCutoff := now.Add(-time.Duration(BringBackLookbackDays) * 24 * time.Hour).Unix()
	gapCutoff := now.Add(-time.Duration(BringBackMinGapDays) * 24 * time.Hour).Unix()

	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("bring-back: query experiences: %w", err)
	}

	type instance struct {
		id          string
		completedAt int64
	}
	byName := make(map[string][]instance)
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		if exp.CompletedAtUnixSec == nil || *exp.CompletedAtUnixSec < rhythmCutoff {
			continue
		}
		name := strings.TrimSpace(exp.Name)
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], instance{
			id:          exp.Id,
			completedAt: *exp.CompletedAtUnixSec,
		})
	}

	type revivable struct {
		name            string
		count           int
		mostRecentAt    int64
		mostRecentExpID string
	}
	var picks []revivable
	for name, insts := range byName {
		if len(insts) < BringBackMinInstances {
			continue
		}
		sort.Slice(insts, func(i, j int) bool {
			return insts[i].completedAt > insts[j].completedAt
		})
		if insts[0].completedAt > gapCutoff {
			continue // no gap yet
		}
		if insts[0].id == excludeContextID {
			continue // already covered by a Hero card
		}
		picks = append(picks, revivable{
			name:            name,
			count:           len(insts),
			mostRecentAt:    insts[0].completedAt,
			mostRecentExpID: insts[0].id,
		})
	}
	if len(picks) == 0 {
		return nil, nil
	}

	sort.Slice(picks, func(i, j int) bool {
		return picks[i].mostRecentAt > picks[j].mostRecentAt
	})
	if len(picks) > BringBackMaxItems {
		picks = picks[:BringBackMaxItems]
	}

	out := make([]Detection, 0, len(picks))
	for _, p := range picks {
		out = append(out, Detection{
			Slot:           SlotCalendarGap, // bring-back items share the calendar-gap framing
			Headline:       fmt.Sprintf("Bring back %s", shortName(p.name)),
			Description:    "The rhythm is worth keeping.",
			CtaLabel:       "Bring it back",
			CtaAction:      "revive_experience",
			ContextID:      p.mostRecentExpID,
			KickerLabel:    "Things this circle has loved",
			AtmosphereLine: fmt.Sprintf("Ran %d times, then quiet", p.count),
		})
	}
	return out, nil
}
