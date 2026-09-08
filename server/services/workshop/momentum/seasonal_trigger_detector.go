package momentum

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// SeasonalAnniversaryDays is the size of the same-time-last-year window
// the detector uses (centred on today's date a year ago). Default: 30
// days, reading the anniversary as a season (e.g. "early April") rather
// than an exact date.
const SeasonalAnniversaryDays = 30

// SeasonalNotRecentDays is how recently the detector requires the
// experience NOT to have been hosted, to avoid double-firing when a host
// has already revived their seasonal tradition. Default: 60 days.
const SeasonalNotRecentDays = 60

// SeasonalTriggerDetector matches cascade priority 4: the user hosted
// something around the same date last year and hasn't hosted it again
// recently. The lever frames the suggestion as a tradition revival.
type SeasonalTriggerDetector struct {
	// Now defaults to time.Now when nil — exposed for testability.
	Now func() time.Time
}

// Slot returns SlotSeasonalTrigger.
func (SeasonalTriggerDetector) Slot() Slot { return SlotSeasonalTrigger }

// Detect inspects the user's hosted experience history for a seasonal
// match: completed roughly a year ago, not hosted again recently.
func (d SeasonalTriggerDetector) Detect(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, _ string,
) (*Detection, error) {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}

	currentTime := now()
	anniversaryCenter := currentTime.AddDate(-1, 0, 0).Unix()
	anniversaryWindow := int64(SeasonalAnniversaryDays) * 24 * 3600
	anniversaryStart := anniversaryCenter - anniversaryWindow/2
	anniversaryEnd := anniversaryCenter + anniversaryWindow/2

	notRecentCutoff := currentTime.Add(-time.Duration(SeasonalNotRecentDays) * 24 * time.Hour).Unix()

	hosted, err := storage.QueryByFields[*models.Experience](store, ctx,
		map[string]any{"owner_id": userID})
	if err != nil {
		return nil, fmt.Errorf("seasonal detector: query experiences: %w", err)
	}

	// First pass: collect candidates from the anniversary window.
	type candidate struct {
		id          string
		name        string
		completedAt int64
	}
	var candidates []candidate
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		if exp.CompletedAtUnixSec == nil {
			continue
		}
		t := *exp.CompletedAtUnixSec
		if t < anniversaryStart || t > anniversaryEnd {
			continue
		}
		name := strings.TrimSpace(exp.Name)
		if name == "" {
			continue
		}
		candidates = append(candidates, candidate{
			id: exp.Id, name: name, completedAt: t,
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Build the "hosted recently" name index so we can skip names already
	// revived in the last ~60 days.
	hostedRecently := map[string]bool{}
	for _, exp := range hosted {
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED &&
			exp.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
			exp.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			continue
		}
		if exp.CompletedAtUnixSec != nil && *exp.CompletedAtUnixSec >= notRecentCutoff {
			hostedRecently[strings.TrimSpace(exp.Name)] = true
		}
		if exp.StartedAtUnixSec != nil && *exp.StartedAtUnixSec >= notRecentCutoff {
			hostedRecently[strings.TrimSpace(exp.Name)] = true
		}
	}

	for _, c := range candidates {
		if hostedRecently[c.name] {
			continue
		}
		// Two things this copy deliberately does not say (#2892). Not "same
		// week": the window is ±15 days, so the anniversary is a season, not
		// a week. Not "and it landed": the detector knows the event
		// completed, never how it went. And no EvidenceQuote — the one that
		// used to sit here was invented and attributed to a real book.
		return &Detection{
			Slot:           SlotSeasonalTrigger,
			Headline:       fmt.Sprintf("Bring back %s", shortName(c.name)),
			Description:    "It ran around this time last year — worth doing again.",
			CtaLabel:       "Bring it back",
			CtaAction:      "revive_experience",
			ContextID:      c.id,
			KickerLabel:    "This time last year",
			AtmosphereLine: "Ran around this time last year",
		}, nil
	}
	return nil, nil
}
