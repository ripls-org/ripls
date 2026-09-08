package simulation

import (
	"fmt"
	"math/rand"
	"time"
)

// Loan flow probabilities. The remaining mass after pCancel + pInProgress
// is the completed case.
const (
	pLoanCancel     = 0.08
	pLoanInProgress = 0.15
)

// inProgressLoanWindow caps how recent the "still active" loans look:
// a loan that started more than this far back would realistically have
// finished by EndTime.
const inProgressLoanWindow = 30 * 24 * time.Hour

// generateLoanFlow creates the multi-step sequence for a gear loan.
// For loans, ExpressInterest auto-selects the borrower (RECIPIENT_SELECTED),
// so no SelectRecipient step is needed. Each flow gets a unique flowID and
// gearBusyUntil prevents overlapping loans on the same item.
//
// Issue #1920: flows are constructed BACKWARD from a terminal anchor time.
// baseTime is interpreted as the anchor (the last meaningful step's Time);
// earlier steps are derived by subtracting jittered intervals from it.
// This guarantees every step.Time ≤ scenario.EndTime as long as baseTime
// itself respects that bound — which the activity loop enforces by
// sampling baseTime inside [StartTime, EndTime].
func generateLoanFlow(rng *rand.Rand, scenario Scenario, borrower MemberDef, comm CommunityDef, gear []gearRecord, baseTime time.Time, flowID int, gearBusyUntil map[string]time.Time) Timeline {
	// Find gear owned by someone else that is currently available at the
	// time we anchor this flow. gearBusyUntil tracks the latest known
	// "occupied through" time for each gear, computed from earlier flows'
	// anchor times.
	var eligible []gearRecord
	for _, g := range gear {
		if g.owner != borrower.Email {
			if busyUntil, ok := gearBusyUntil[g.refKey]; ok && baseTime.Before(busyUntil) {
				continue
			}
			eligible = append(eligible, g)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	target := eligible[rng.Intn(len(eligible))]
	ref := fmt.Sprintf("%s-loan-%d", target.refKey, flowID)

	roll := rng.Float64()
	switch {
	case roll < pLoanCancel:
		return loanFlowCancelled(rng, scenario, borrower, target, comm, baseTime, ref, gearBusyUntil)
	case roll < pLoanCancel+pLoanInProgress:
		return loanFlowInProgress(rng, scenario, borrower, target, comm, baseTime, ref, gearBusyUntil)
	default:
		return loanFlowCompleted(rng, scenario, borrower, target, comm, baseTime, ref, gearBusyUntil)
	}
}

// loanFlowCancelled emits [ExpressInterest, CancelTransfer]. baseTime
// anchors CancelTransfer; ExpressInterest is 1-48h earlier.
func loanFlowCancelled(rng *rand.Rand, _ Scenario, borrower MemberDef, target gearRecord, comm CommunityDef, baseTime time.Time, ref string, gearBusyUntil map[string]time.Time) Timeline {
	cancelTime := baseTime
	interestTime := cancelTime.Add(-jitterDuration(rng, 1*time.Hour, 48*time.Hour))
	commitGearRange(gearBusyUntil, target.refKey, interestTime, cancelTime)
	return Timeline{
		{Time: interestTime, Actor: borrower.Email, CommunityName: comm.Name, Action: ActionExpressInterest, Ref: ref},
		{Time: cancelTime, Actor: target.owner, CommunityName: comm.Name, Action: ActionCancelTransfer, Ref: ref},
	}
}

// loanFlowInProgress emits [ExpressInterest, StartLoan] with no
// CompleteLoan — the loan is still active at scenario.EndTime. The
// startTime is sampled inside the last inProgressLoanWindow of the
// scenario so the "in flight" loans look plausibly recent.
func loanFlowInProgress(rng *rand.Rand, scenario Scenario, borrower MemberDef, target gearRecord, comm CommunityDef, baseTime time.Time, ref string, gearBusyUntil map[string]time.Time) Timeline {
	// Use baseTime as the StartLoan anchor, but if it's older than the
	// in-progress window we'd rather pretend the loan just started; pull
	// it forward to a fresh point in the window.
	earliestStart := scenario.EndTime.Add(-inProgressLoanWindow)
	startTime := baseTime
	if startTime.Before(earliestStart) {
		startTime = pickAnchorAfter(rng, scenario, earliestStart)
	}
	interestTime := startTime.Add(-jitterDuration(rng, 24*time.Hour, 72*time.Hour))
	// Hold the gear well past EndTime so no later flow tries to share it.
	commitGearRange(gearBusyUntil, target.refKey, interestTime, scenario.EndTime.Add(100*365*24*time.Hour))
	return Timeline{
		{Time: interestTime, Actor: borrower.Email, CommunityName: comm.Name, Action: ActionExpressInterest, Ref: ref},
		{Time: startTime, Actor: target.owner, CommunityName: comm.Name, Action: ActionStartLoan, Ref: ref},
	}
}

// loanFlowCompleted emits [ExpressInterest, StartLoan, CompleteLoan].
// baseTime anchors CompleteLoan; StartLoan is 7-30d earlier;
// ExpressInterest is another 24-72h before that.
func loanFlowCompleted(rng *rand.Rand, _ Scenario, borrower MemberDef, target gearRecord, comm CommunityDef, baseTime time.Time, ref string, gearBusyUntil map[string]time.Time) Timeline {
	completeTime := baseTime
	startTime := completeTime.Add(-jitterDuration(rng, 7*24*time.Hour, 30*24*time.Hour))
	interestTime := startTime.Add(-jitterDuration(rng, 24*time.Hour, 72*time.Hour))
	commitGearRange(gearBusyUntil, target.refKey, interestTime, completeTime)
	return Timeline{
		{Time: interestTime, Actor: borrower.Email, CommunityName: comm.Name, Action: ActionExpressInterest, Ref: ref},
		{Time: startTime, Actor: target.owner, CommunityName: comm.Name, Action: ActionStartLoan, Ref: ref},
		{Time: completeTime, Actor: target.owner, CommunityName: comm.Name, Action: ActionCompleteLoan, Ref: ref},
	}
}
