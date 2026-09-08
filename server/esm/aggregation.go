package esm

import (
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Signal is the per-prompt aggregate of response rows. Server-internal: the
// Workshop momentum detectors consume it directly via Aggregate /
// AggregateAttendeesOnly. There is no client-facing RPC that returns it.
type Signal struct {
	ResponseDistribution map[string]int32
	PromptID             string
	RespondentCount      int32
	CompletedAtUnixSec   int64
}

// Aggregate computes per-prompt repeat-signal aggregates from a slice of
// response rows. Pure function: no storage access, deterministic, safe to
// unit-test in isolation.
//
// Only RESPONDED rows count toward RespondentCount and ResponseDistribution;
// DISMISSED and UNRESOLVED rows are ignored. completedAtUnixSec is passed
// through to every emitted Signal so callers don't have to re-fetch the
// experience to know when it ended.
//
// Returns one Signal per prompt_id present in the input. Prompts with zero
// RESPONDED rows still appear with RespondentCount=0 if any rows exist for
// that prompt_id.
func Aggregate(rows []*models.StoredESMResponse, completedAtUnixSec int64) []*Signal {
	type bucket struct {
		distribution map[string]int32
		respondents  int32
	}

	byPrompt := make(map[string]*bucket)

	for _, row := range rows {
		b, ok := byPrompt[row.PromptId]
		if !ok {
			b = &bucket{distribution: make(map[string]int32)}
			byPrompt[row.PromptId] = b
		}
		if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED {
			continue
		}
		b.respondents++
		if row.ResponseOptionKey != nil && *row.ResponseOptionKey != "" {
			b.distribution[*row.ResponseOptionKey]++
		}
	}

	signals := make([]*Signal, 0, len(byPrompt))
	for promptID, b := range byPrompt {
		signals = append(signals, &Signal{
			PromptID:             promptID,
			RespondentCount:      b.respondents,
			ResponseDistribution: b.distribution,
			CompletedAtUnixSec:   completedAtUnixSec,
		})
	}
	return signals
}

// AggregateAttendeesOnly is Aggregate filtered to rows where
// respondent_was_attendee is true. The Workshop's ESM Repeat Signal detector
// calls this so its ≥2 / ≥60% threshold reflects post-event satisfaction
// from people who were actually there, not enthusiasm from the wider story
// audience. Rows with respondent_was_attendee unset (nil) are conservatively
// excluded — the field is set at insert time for every new row, so an unset
// value indicates a pre-migration row the detector should not weigh.
func AggregateAttendeesOnly(rows []*models.StoredESMResponse, completedAtUnixSec int64) []*Signal {
	filtered := make([]*models.StoredESMResponse, 0, len(rows))
	for _, row := range rows {
		if row.RespondentWasAttendee == nil || !*row.RespondentWasAttendee {
			continue
		}
		filtered = append(filtered, row)
	}
	return Aggregate(filtered, completedAtUnixSec)
}
