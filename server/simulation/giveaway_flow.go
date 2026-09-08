package simulation

import (
	"fmt"
	"math/rand"
	"time"
)

// generateGiveawayFlow creates the multi-step sequence for a gear giveaway.
// For giveaways, ExpressInterest sets state to INTEREST_EXPRESSED, then the
// owner selects a recipient and completes the transfer. Each flow gets a
// unique flowID and gearBusyUntil marks gear as permanently unavailable
// after giveaway.
//
// Issue #1920: baseTime is the anchor (= CompleteGiveaway time). All other
// steps are derived backward from it so step.Time ≤ scenario.EndTime is a
// construction invariant.
func generateGiveawayFlow(rng *rand.Rand, _ Scenario, giver MemberDef, comm CommunityDef, gear []gearRecord, baseTime time.Time, flowID int, gearBusyUntil map[string]time.Time) Timeline {
	// Find gear owned by the giver that is currently available.
	var owned []gearRecord
	for _, g := range gear {
		if g.owner == giver.Email {
			if busyUntil, ok := gearBusyUntil[g.refKey]; ok && baseTime.Before(busyUntil) {
				continue
			}
			owned = append(owned, g)
		}
	}
	if len(owned) == 0 {
		return nil
	}

	target := owned[rng.Intn(len(owned))]
	ref := fmt.Sprintf("%s-giveaway-%d", target.refKey, flowID)

	var steps Timeline

	// Find candidates (everyone except the giver).
	var candidates []MemberDef
	for _, m := range comm.Members {
		if m.Email != giver.Email {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	// Anchor backward: CompleteGiveaway at baseTime, SelectGiveaway 1 minute
	// earlier, first ExpressInterestGiveaway 6-48h before SelectGiveaway.
	completeTime := baseTime
	selectTime := completeTime.Add(-time.Minute)
	firstInterestTime := selectTime.Add(-jitterDuration(rng, 6*time.Hour, 48*time.Hour))

	// First interested party.
	firstInterested := candidates[rng.Intn(len(candidates))]
	steps = append(steps, ActivityStep{
		Time:          firstInterestTime,
		Actor:         firstInterested.Email,
		CommunityName: comm.Name,
		Action:        ActionExpressInterestGiveaway,
		Ref:           ref,
	})

	// Other interested parties slot between firstInterestTime and selectTime.
	otherWindow := selectTime.Sub(firstInterestTime)
	for _, other := range candidates {
		if other.Email == firstInterested.Email {
			continue
		}
		if rng.Float64() < 0.25 {
			// 1h after firstInterestTime to a little before selectTime so the
			// owner's selection still trails the interest expressions.
			offset := jitterDuration(rng, 1*time.Hour, otherWindow-1*time.Hour)
			if offset <= 0 {
				offset = otherWindow / 2
			}
			steps = append(steps, ActivityStep{
				Time:          firstInterestTime.Add(offset),
				Actor:         other.Email,
				CommunityName: comm.Name,
				Action:        ActionExpressInterestGiveaway,
				Ref:           ref,
			})
		}
	}

	// Owner selects recipient.
	steps = append(steps, ActivityStep{
		Time:          selectTime,
		Actor:         giver.Email,
		CommunityName: comm.Name,
		Action:        ActionSelectGiveaway,
		Ref:           ref,
	})

	// Owner completes giveaway.
	steps = append(steps, ActivityStep{
		Time:          completeTime,
		Actor:         giver.Email,
		CommunityName: comm.Name,
		Action:        ActionCompleteGiveaway,
		Ref:           ref,
	})

	// Gear is permanently unavailable after giveaway.
	commitGearRange(gearBusyUntil, target.refKey, firstInterestTime, completeTime.Add(100*365*24*time.Hour))
	return steps
}
