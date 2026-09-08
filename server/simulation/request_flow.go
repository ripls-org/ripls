package simulation

import (
	"fmt"
	"math/rand"
	"time"
)

// Request flow probabilities. The remaining mass after these two is the
// completed (fulfilled) case.
const (
	pRequestCancel     = 0.10
	pRequestInProgress = 0.15
)

// inProgressRequestWindow caps how recent the still-open requests look.
const inProgressRequestWindow = 7 * 24 * time.Hour

// generateRequestFlow creates the multi-step sequence for a community
// request.
//
// Issue #1920: baseTime is the anchor. For fulfilled requests it's the
// FulfillRequest time; for cancelled ones it's CancelRequest; for
// in-progress ones it's SubmitRequest. Earlier steps (and offer steps)
// derive backward.
func generateRequestFlow(rng *rand.Rand, scenario Scenario, requester MemberDef, comm CommunityDef, _ []RequestTemplate, baseTime time.Time) Timeline {
	ref := fmt.Sprintf("req-%s-%s", requester.Email, baseTime.Format("0102-1504"))

	roll := rng.Float64()
	switch {
	case roll < pRequestCancel:
		return requestFlowCancelled(rng, requester, comm, baseTime, ref)
	case roll < pRequestCancel+pRequestInProgress:
		return requestFlowInProgress(rng, scenario, requester, comm, baseTime, ref)
	default:
		return requestFlowFulfilled(rng, requester, comm, baseTime, ref)
	}
}

// requestFlowFulfilled emits SubmitRequest → offers → (maybe withdraw) →
// FulfillRequest, anchored at FulfillRequest.
func requestFlowFulfilled(rng *rand.Rand, requester MemberDef, comm CommunityDef, baseTime time.Time, ref string) Timeline {
	fulfillTime := baseTime
	submitTime := fulfillTime.Add(-jitterDuration(rng, 24*time.Hour, 7*24*time.Hour))
	offerWindow := fulfillTime.Sub(submitTime) - time.Hour
	if offerWindow < 2*time.Hour {
		offerWindow = 2 * time.Hour
	}

	steps := Timeline{
		{Time: submitTime, Actor: requester.Email, CommunityName: comm.Name, Action: ActionSubmitRequest, Ref: ref},
	}

	var helpers []string
	var lastOfferTime time.Time
	for _, member := range comm.Members {
		if member.Email == requester.Email {
			continue
		}
		pw := PersonaConfig()[member.Persona]
		offerWeight := pw.Weights[ActivityOfferHelp]
		if rng.Float64() < offerWeight*2 {
			offerTime := submitTime.Add(jitterDuration(rng, time.Hour, offerWindow))
			steps = append(steps, ActivityStep{
				Time:          offerTime,
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionOfferToFulfill,
				Ref:           ref,
			})
			helpers = append(helpers, member.Email)
			lastOfferTime = offerTime
		}
	}

	// 10% chance one helper withdraws (after their offer, before fulfillment).
	if len(helpers) > 1 && rng.Float64() < 0.10 {
		withdrawer := helpers[len(helpers)-1]
		withdrawTime := lastOfferTime.Add(jitterDuration(rng, time.Hour, 12*time.Hour))
		if withdrawTime.After(fulfillTime) {
			withdrawTime = lastOfferTime.Add(30 * time.Minute)
		}
		if withdrawTime.Before(fulfillTime) {
			steps = append(steps, ActivityStep{
				Time:          withdrawTime,
				Actor:         withdrawer,
				CommunityName: comm.Name,
				Action:        ActionWithdrawOffer,
				Ref:           ref,
			})
		}
	}

	if len(helpers) > 0 {
		steps = append(steps, ActivityStep{
			Time:          fulfillTime,
			Actor:         requester.Email,
			CommunityName: comm.Name,
			Action:        ActionFulfillRequest,
			Ref:           ref,
		})
	}
	return steps
}

// requestFlowCancelled emits SubmitRequest → (offers) → CancelRequest,
// anchored at CancelRequest.
func requestFlowCancelled(rng *rand.Rand, requester MemberDef, comm CommunityDef, baseTime time.Time, ref string) Timeline {
	cancelTime := baseTime
	submitTime := cancelTime.Add(-jitterDuration(rng, 24*time.Hour, 5*24*time.Hour))
	offerWindow := cancelTime.Sub(submitTime) - time.Hour
	if offerWindow < 2*time.Hour {
		offerWindow = 2 * time.Hour
	}

	steps := Timeline{
		{Time: submitTime, Actor: requester.Email, CommunityName: comm.Name, Action: ActionSubmitRequest, Ref: ref},
	}
	for _, member := range comm.Members {
		if member.Email == requester.Email {
			continue
		}
		pw := PersonaConfig()[member.Persona]
		offerWeight := pw.Weights[ActivityOfferHelp]
		if rng.Float64() < offerWeight*2 {
			steps = append(steps, ActivityStep{
				Time:          submitTime.Add(jitterDuration(rng, time.Hour, offerWindow)),
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionOfferToFulfill,
				Ref:           ref,
			})
		}
	}
	steps = append(steps, ActivityStep{
		Time:          cancelTime,
		Actor:         requester.Email,
		CommunityName: comm.Name,
		Action:        ActionCancelRequest,
		Ref:           ref,
	})
	return steps
}

// requestFlowInProgress emits SubmitRequest plus any offers that arrived
// before EndTime, with no terminal step — the request is still open.
func requestFlowInProgress(rng *rand.Rand, scenario Scenario, requester MemberDef, comm CommunityDef, baseTime time.Time, ref string) Timeline {
	// Submit time = anchor; if older than the in-progress window pull
	// forward to a more recent point.
	earliest := scenario.EndTime.Add(-inProgressRequestWindow)
	submitTime := baseTime
	if submitTime.Before(earliest) {
		submitTime = pickAnchorAfter(rng, scenario, earliest)
	}
	steps := Timeline{
		{Time: submitTime, Actor: requester.Email, CommunityName: comm.Name, Action: ActionSubmitRequest, Ref: ref},
	}

	// Offers scatter between submitTime and scenario.EndTime. Need at least
	// 20 minutes of slack so the 10-minute-jittered offers fit.
	offerWindow := scenario.EndTime.Sub(submitTime)
	if offerWindow < 20*time.Minute {
		return steps
	}
	for _, member := range comm.Members {
		if member.Email == requester.Email {
			continue
		}
		pw := PersonaConfig()[member.Persona]
		offerWeight := pw.Weights[ActivityOfferHelp]
		if rng.Float64() < offerWeight*2 {
			offerTime := submitTime.Add(jitterDuration(rng, 10*time.Minute, offerWindow))
			steps = append(steps, ActivityStep{
				Time:          offerTime,
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionOfferToFulfill,
				Ref:           ref,
			})
		}
	}
	return steps
}
