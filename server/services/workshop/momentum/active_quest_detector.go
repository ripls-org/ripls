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

// ActiveQuestRecurringInstances is how many prior completed instances the
// detector requires before treating an active experience as the next
// round of an existing rhythm. Default: 1 prior instance (i.e. round 2+).
const ActiveQuestRecurringInstances = 1

// ActiveQuestDetector matches cascade priority 1: the user has an active
// experience that is the next instance of a rhythm they've hosted before.
//
// "Active" here means an experience in EXPERIENCE_STATE_ACTIVE or
// EXPERIENCE_STATE_JOINED — i.e. it hasn't started yet but is planned.
// The detector is intentionally conservative: it only fires when the
// active experience's name matches a prior completed instance, so the
// host gets a "round N+1 is the natural next move" framing.
type ActiveQuestDetector struct {
	// Now defaults to time.Now when nil — exposed for testability.
	Now func() time.Time
}

// Slot returns SlotActiveQuest.
func (ActiveQuestDetector) Slot() Slot { return SlotActiveQuest }

// Detect finds an active hosted experience whose name has at least one
// prior completed instance.
func (d ActiveQuestDetector) Detect(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, _ string,
) (*Detection, error) {
	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("active-quest detector: query experiences: %w", err)
	}

	// Build a name → completed-instance count map for prior rhythm.
	priorByName := make(map[string]int)
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		name := strings.TrimSpace(exp.Name)
		if name == "" {
			continue
		}
		priorByName[name]++
	}

	type active struct {
		exp        *models.Experience
		priorCount int
	}
	var actives []active
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
			exp.State != models.ExperienceState_EXPERIENCE_STATE_JOINED {
			continue
		}
		name := strings.TrimSpace(exp.Name)
		if name == "" {
			continue
		}
		count, ok := priorByName[name]
		if !ok || count < ActiveQuestRecurringInstances {
			continue
		}
		actives = append(actives, active{exp: exp, priorCount: count})
	}
	if len(actives) == 0 {
		return nil, nil
	}

	// Pick the rhythm with the most prior instances (most-established
	// rhythm reads strongest as a Hero card).
	sort.Slice(actives, func(i, j int) bool {
		return actives[i].priorCount > actives[j].priorCount
	})
	pick := actives[0]

	return &Detection{
		Slot:     SlotActiveQuest,
		Headline: fmt.Sprintf("Schedule %s again", shortName(pick.exp.Name)),
		Description: fmt.Sprintf(
			"You've hosted it %d times — worth keeping the rhythm going.",
			pick.priorCount,
		),
		CtaLabel:       "Schedule it",
		CtaAction:      "schedule_repeat",
		ContextID:      pick.exp.Id,
		KickerLabel:    "On the way",
		AtmosphereLine: fmt.Sprintf("Hosted %d times before", pick.priorCount),
	}, nil
}

// suppress "unused import" if time becomes unused after edits.
var _ = time.Now
