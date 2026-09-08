package catalyst

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// LoadStreakThreshold is how many consecutive same-host instances of a
// rhythm constitute a lopsided load. Per the brief: "same person hosted
// the last three or more in a row." Default: 3.
const LoadStreakThreshold = 3

// LoadSignal describes the load-distribution picture for a circle.
//
// Lopsided is true when at least one rhythm in the circle has been hosted
// LoadStreakThreshold+ times in a row by the same user. OverloadedUserID
// names that user when lopsided is true.
type LoadSignal struct {
	OverloadedUserID string
	// RhythmName names the recurring activity that is lopsided (used by
	// the lever copy: "you've hosted the last three Sunday brunches").
	RhythmName string
	// StreakLength is the number of consecutive same-host instances.
	StreakLength int
	Lopsided     bool
}

// DetectLoad inspects the user's hosted experience history within the
// circle and returns a LoadSignal describing whether the load is
// lopsided. Pure function over storage; safe to unit-test.
//
// v1 detects lopsidedness by exact rhythm name (same as CalendarGap
// detection in the momentum library). The 60%-of-last-five detection
// variant from the brief is left as a future refinement.
func DetectLoad(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityID string,
) (*LoadSignal, error) {
	communityExps, err := storage.QueryByFields[*models.CommunityExperience](
		store, ctx, map[string]any{"community_id": communityID},
	)
	if err != nil {
		return nil, fmt.Errorf("load detector: query community_experience: %w", err)
	}
	if len(communityExps) == 0 {
		return &LoadSignal{Lopsided: false}, nil
	}

	// Group experiences in the community by name, sorted by completed_at
	// DESC. For each rhythm, walk from most recent backward; if the same
	// owner_id streaks for LoadStreakThreshold instances, flag lopsided.
	type instance struct {
		ownerID     string
		completedAt int64
	}
	byName := make(map[string][]instance)
	for _, ce := range communityExps {
		exp := &models.Experience{}
		if err := store.GetByID(ctx, ce.ExperienceId, exp); err != nil {
			continue
		}
		if exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		if exp.CompletedAtUnixSec == nil {
			continue
		}
		name := strings.TrimSpace(exp.Name)
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], instance{
			ownerID:     exp.OwnerId,
			completedAt: *exp.CompletedAtUnixSec,
		})
	}

	for name, insts := range byName {
		if len(insts) < LoadStreakThreshold {
			continue
		}
		sort.Slice(insts, func(i, j int) bool {
			return insts[i].completedAt > insts[j].completedAt
		})
		streakOwner := insts[0].ownerID
		streak := 1
		for i := 1; i < len(insts) && i < LoadStreakThreshold; i++ {
			if insts[i].ownerID != streakOwner {
				break
			}
			streak++
		}
		if streak >= LoadStreakThreshold && streakOwner != "" {
			return &LoadSignal{
				Lopsided:         true,
				OverloadedUserID: streakOwner,
				RhythmName:       name,
				StreakLength:     streak,
			}, nil
		}
	}
	return &LoadSignal{Lopsided: false}, nil
}
