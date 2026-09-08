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

// CalendarGapMinInstances is how many completed experiences with the same
// name (within the rhythm window) are required before the detector
// considers a "rhythm" to exist. Conservative default: 3 instances.
const CalendarGapMinInstances = 3

// CalendarGapRhythmWindowDays is how far back the detector looks for
// rhythm instances. Default: 90 days (matches "ran six times last fall"
// language in the brief).
const CalendarGapRhythmWindowDays = 90

// CalendarGapMinGapDays is the minimum days-since-last-instance for a
// rhythm to count as having a gap. Default: 14 days (two missed weeks).
const CalendarGapMinGapDays = 14

// CalendarGapDetector matches cascade priority 3: a recurring rhythm in
// the user's hosted experiences with a detectable gap from the last
// instance. v1 detects rhythms by exact name match; future iterations
// could use semantic similarity.
type CalendarGapDetector struct {
	// Now defaults to time.Now when nil — exposed for testability.
	Now func() time.Time
}

// Slot returns SlotCalendarGap.
func (CalendarGapDetector) Slot() Slot { return SlotCalendarGap }

// Detect groups the user's completed experiences by name within the
// rhythm window, finds the most recently-active rhythm with a gap, and
// returns a love-revival Detection.
func (d CalendarGapDetector) Detect(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, _ string,
) (*Detection, error) {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	rhythmCutoff := now().Add(-time.Duration(CalendarGapRhythmWindowDays) * 24 * time.Hour).Unix()
	gapCutoff := now().Add(-time.Duration(CalendarGapMinGapDays) * 24 * time.Hour).Unix()

	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("calendar-gap detector: query experiences: %w", err)
	}

	// Group completed experiences by name; only those completed within the
	// rhythm window count.
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

	type rhythm struct {
		name            string
		instances       []instance
		mostRecentAt    int64
		mostRecentExpID string
	}
	var rhythms []rhythm
	for name, insts := range byName {
		if len(insts) < CalendarGapMinInstances {
			continue
		}
		sort.Slice(insts, func(i, j int) bool {
			return insts[i].completedAt > insts[j].completedAt
		})
		// Require a gap from the most recent instance.
		if insts[0].completedAt > gapCutoff {
			continue
		}
		rhythms = append(rhythms, rhythm{
			name:            name,
			instances:       insts,
			mostRecentAt:    insts[0].completedAt,
			mostRecentExpID: insts[0].id,
		})
	}
	if len(rhythms) == 0 {
		return nil, nil
	}

	// Pick the rhythm with the most recent activity (least-stale gap reads
	// best on the love-revival surface — "you did this five times last
	// month" feels more revivable than "...two years ago").
	sort.Slice(rhythms, func(i, j int) bool {
		return rhythms[i].mostRecentAt > rhythms[j].mostRecentAt
	})
	pick := rhythms[0]

	// No EvidenceQuote: the quotation that used to sit here was invented and
	// attributed to a real study (#2892). Evidence returns when there is a
	// real quote and a real citation to carry.
	return &Detection{
		Slot:           SlotCalendarGap,
		Headline:       fmt.Sprintf("Bring back %s", shortName(pick.name)),
		Description:    "The rhythm is real — worth a spark.",
		CtaLabel:       "Bring it back",
		CtaAction:      "revive_experience",
		ContextID:      pick.mostRecentExpID,
		KickerLabel:    "Things this circle has loved",
		AtmosphereLine: fmt.Sprintf("Ran %d times, then quiet", len(pick.instances)),
	}, nil
}
