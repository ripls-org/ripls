package simulation

import (
	"fmt"
	"math/rand"
	"time"
)

// Experience flow probabilities. The remaining mass after these two is the
// "completed" case (Save → Share → RSVPs → Start → Complete).
const (
	pExperienceCancel     = 0.07
	pExperienceInProgress = 0.15
)

// inProgressExperienceWindow caps how recent the "scheduled but not yet
// started" experiences look. Beyond this far back, the host would
// realistically have started or cancelled.
const inProgressExperienceWindow = 21 * 24 * time.Hour

// generateExperienceFlow creates the multi-step sequence for hosting an
// experience.
//
// Issue #1920: baseTime is the anchor. For completed experiences it's the
// CompleteExperience time; for cancelled ones it's CancelExperience; for
// in-progress ones it's ShareExperience (the experience is upcoming).
// Every other step is derived backward.
func generateExperienceFlow(rng *rand.Rand, scenario Scenario, host MemberDef, comm CommunityDef, templates []ExperienceTemplate, baseTime time.Time) Timeline {
	ref := fmt.Sprintf("exp-%s-%s", host.Email, baseTime.Format("0102-1504"))
	tmpl := templates[rng.Intn(len(templates))]

	roll := rng.Float64()
	switch {
	case roll < pExperienceCancel:
		return experienceFlowCancelled(rng, host, comm, tmpl, baseTime, ref)
	case roll < pExperienceCancel+pExperienceInProgress:
		return experienceFlowInProgress(rng, scenario, host, comm, tmpl, baseTime, ref)
	default:
		return experienceFlowCompleted(rng, host, comm, tmpl, baseTime, ref)
	}
}

// experienceFlowCompleted emits the full lifecycle anchored at
// CompleteExperience.
func experienceFlowCompleted(rng *rand.Rand, host MemberDef, comm CommunityDef, tmpl ExperienceTemplate, baseTime time.Time, ref string) Timeline {
	completeTime := baseTime
	duration := time.Duration(tmpl.DurationMinutes) * time.Minute
	startTime := completeTime.Add(-duration)
	// rsvpDeadline = startTime. Distance from share to start is the planning
	// window (7-30 days in the old forward model). We preserve that range so
	// the gap between share and start still feels realistic.
	planningWindow := jitterDuration(rng, 7*24*time.Hour, 30*24*time.Hour)
	shareTime := startTime.Add(-planningWindow)
	saveTime := shareTime.Add(-jitterDuration(rng, 1*time.Minute, 30*time.Minute))

	steps := Timeline{
		{Time: saveTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionSaveExperience, Ref: ref},
		{Time: shareTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionShareExperience, Ref: ref},
	}
	steps = append(steps, generateExperienceRSVPs(rng, host, comm, tmpl, ref, shareTime, startTime)...)
	steps = append(steps,
		ActivityStep{Time: startTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionStartExperience, Ref: ref},
		ActivityStep{Time: completeTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionCompleteExperience, Ref: ref},
	)
	return steps
}

// experienceFlowCancelled emits Save → Share → (RSVPs) → CancelExperience
// anchored at CancelExperience.
func experienceFlowCancelled(rng *rand.Rand, host MemberDef, comm CommunityDef, tmpl ExperienceTemplate, baseTime time.Time, ref string) Timeline {
	cancelTime := baseTime
	// In the old model, cancellation fired 2-7d after baseTime (= Save). To
	// preserve the same spacing, share/save fall 2-7d before the cancel.
	shareTime := cancelTime.Add(-jitterDuration(rng, 2*24*time.Hour, 7*24*time.Hour))
	saveTime := shareTime.Add(-jitterDuration(rng, 1*time.Minute, 30*time.Minute))

	steps := Timeline{
		{Time: saveTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionSaveExperience, Ref: ref},
		{Time: shareTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionShareExperience, Ref: ref},
	}
	// RSVPs must arrive before cancellation; use cancelTime as the deadline.
	steps = append(steps, generateExperienceRSVPs(rng, host, comm, tmpl, ref, shareTime, cancelTime)...)
	steps = append(steps,
		ActivityStep{Time: cancelTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionCancelExperience, Ref: ref},
	)
	return steps
}

// experienceFlowInProgress emits Save → Share → (RSVPs) with no Start/
// Complete — the experience is scheduled but hasn't happened yet at
// scenario.EndTime. Share time lands inside the recent window.
func experienceFlowInProgress(rng *rand.Rand, scenario Scenario, host MemberDef, comm CommunityDef, tmpl ExperienceTemplate, baseTime time.Time, ref string) Timeline {
	// Share time = anchor (baseTime), but if baseTime is older than the
	// in-progress window, pull it forward so the upcoming experience
	// reads as plausibly current.
	earliestShare := scenario.EndTime.Add(-inProgressExperienceWindow)
	shareTime := baseTime
	if shareTime.Before(earliestShare) {
		shareTime = pickAnchorAfter(rng, scenario, earliestShare)
	}
	saveTime := shareTime.Add(-jitterDuration(rng, 1*time.Minute, 30*time.Minute))

	// RSVPs scatter between shareTime and scenario.EndTime — they've been
	// arriving but the experience hasn't started yet.
	steps := Timeline{
		{Time: saveTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionSaveExperience, Ref: ref},
		{Time: shareTime, Actor: host.Email, CommunityName: comm.Name, Action: ActionShareExperience, Ref: ref},
	}
	steps = append(steps, generateExperienceRSVPs(rng, host, comm, tmpl, ref, shareTime, scenario.EndTime)...)
	return steps
}

// generateExperienceRSVPs samples per-member RSVP steps inside the window
// (shareTime, deadline]. Yes-RSVPs respect the experience's MaxParticipants
// cap (minus one for the host's implicit YES). Same semantics as the old
// forward implementation, but the window is now passed explicitly so each
// caller (completed / cancelled / in-progress) controls the deadline.
func generateExperienceRSVPs(rng *rand.Rand, host MemberDef, comm CommunityDef, tmpl ExperienceTemplate, ref string, shareTime, deadline time.Time) Timeline {
	// Need at least 2h of slack so the 1h-jittered RSVPs fit inside the window.
	if deadline.Sub(shareTime) < 2*time.Hour {
		return nil
	}
	window := deadline.Sub(shareTime)

	yesCap := -1
	if tmpl.MaxParticipants > 0 {
		yesCap = tmpl.MaxParticipants - 1
	}
	yesCount := 0
	var steps Timeline
	memberOrder := rng.Perm(len(comm.Members))
	for _, idx := range memberOrder {
		member := comm.Members[idx]
		if member.Email == host.Email {
			continue
		}
		pw := PersonaConfig()[member.Persona]
		attendWeight := pw.Weights[ActivityAttendExperience]
		roll := rng.Float64()
		if roll < attendWeight*2 {
			if yesCap >= 0 && yesCount >= yesCap {
				continue
			}
			rsvpTime := shareTime.Add(jitterDuration(rng, 1*time.Hour, window-time.Hour))
			steps = append(steps, ActivityStep{
				Time:          rsvpTime,
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionRSVP,
				Ref:           ref,
			})
			yesCount++
		} else if roll < attendWeight*2+0.15 {
			rsvpTime := shareTime.Add(jitterDuration(rng, 2*time.Hour, window-time.Hour))
			steps = append(steps, ActivityStep{
				Time:          rsvpTime,
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionRSVPNo,
				Ref:           ref,
			})
		}
	}
	return steps
}
