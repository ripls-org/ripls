package esm

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func ptr[T any](v T) *T { return &v }

func responseRow(promptID, userID, optKey string, action models.ESMConsumedAction) *models.StoredESMResponse {
	row := &models.StoredESMResponse{
		PromptId:       promptID,
		UserId:         userID,
		ConsumedAction: action,
	}
	if optKey != "" {
		row.ResponseOptionKey = ptr(optKey)
	}
	return row
}

func TestAggregate_EmptyInput(t *testing.T) {
	got := Aggregate(nil, 100)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d signals", len(got))
	}
}

func TestAggregate_OnlyRespondedRowsCount(t *testing.T) {
	rows := []*models.StoredESMResponse{
		responseRow("p1", "u1", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED),
		responseRow("p1", "u2", "skip", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED),
		responseRow("p1", "u3", "", models.ESMConsumedAction_ESM_CONSUMED_ACTION_DISMISSED),
		responseRow("p1", "u4", "", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED),
	}

	got := Aggregate(rows, 12345)
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	sig := got[0]
	if sig.PromptID != "p1" {
		t.Errorf("PromptID: want p1, got %s", sig.PromptID)
	}
	if sig.RespondentCount != 2 {
		t.Errorf("RespondentCount: want 2, got %d", sig.RespondentCount)
	}
	if sig.ResponseDistribution["do_again"] != 1 {
		t.Errorf("distribution[do_again]: want 1, got %d", sig.ResponseDistribution["do_again"])
	}
	if sig.ResponseDistribution["skip"] != 1 {
		t.Errorf("distribution[skip]: want 1, got %d", sig.ResponseDistribution["skip"])
	}
	if sig.CompletedAtUnixSec != 12345 {
		t.Errorf("CompletedAtUnixSec: want 12345, got %d", sig.CompletedAtUnixSec)
	}
}

func TestAggregate_MultiplePrompts(t *testing.T) {
	rows := []*models.StoredESMResponse{
		responseRow("p1", "u1", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED),
		responseRow("p1", "u2", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED),
		responseRow("p2", "u1", "yes", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED),
	}

	got := Aggregate(rows, 999)
	if len(got) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(got))
	}
	byID := map[string]int32{}
	for _, sig := range got {
		byID[sig.PromptID] = sig.RespondentCount
	}
	if byID["p1"] != 2 {
		t.Errorf("p1 RespondentCount: want 2, got %d", byID["p1"])
	}
	if byID["p2"] != 1 {
		t.Errorf("p2 RespondentCount: want 1, got %d", byID["p2"])
	}
}

func TestAggregate_RepeatThreshold60Percent(t *testing.T) {
	// Mirrors the Workshop's cascade priority (2) check: 8 of 11 == ~73% should
	// pass the ≥60% threshold using the literal "do_again" key.
	rows := make([]*models.StoredESMResponse, 0, 11)
	for i := 0; i < 8; i++ {
		rows = append(rows, responseRow("p1", "yes-user", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED))
	}
	for i := 0; i < 3; i++ {
		rows = append(rows, responseRow("p1", "no-user", "skip", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED))
	}

	got := Aggregate(rows, 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	sig := got[0]
	doAgain := float64(sig.ResponseDistribution["do_again"])
	respondents := float64(sig.RespondentCount)
	pct := doAgain / respondents
	if pct < 0.60 {
		t.Errorf("expected %.2f >= 0.60", pct)
	}
}

func TestAggregate_BelowThreshold(t *testing.T) {
	// 4 of 10 == 40% should fail the ≥60% threshold but still surface counts.
	rows := make([]*models.StoredESMResponse, 0, 10)
	for i := 0; i < 4; i++ {
		rows = append(rows, responseRow("p1", "yes-user", "do_again", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED))
	}
	for i := 0; i < 6; i++ {
		rows = append(rows, responseRow("p1", "no-user", "skip", models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED))
	}

	got := Aggregate(rows, 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	sig := got[0]
	if sig.ResponseDistribution["do_again"] != 4 {
		t.Errorf("do_again count: want 4, got %d", sig.ResponseDistribution["do_again"])
	}
	if sig.ResponseDistribution["skip"] != 6 {
		t.Errorf("skip count: want 6, got %d", sig.ResponseDistribution["skip"])
	}
}

func TestAggregate_PromptWithOnlyDismissals(t *testing.T) {
	// A prompt with only dismissals/unresolved still appears in the result with
	// respondent_count=0 — callers can detect "we sent prompts but no one responded".
	rows := []*models.StoredESMResponse{
		responseRow("p1", "u1", "", models.ESMConsumedAction_ESM_CONSUMED_ACTION_DISMISSED),
		responseRow("p1", "u2", "", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED),
	}

	got := Aggregate(rows, 0)
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	if got[0].RespondentCount != 0 {
		t.Errorf("RespondentCount: want 0, got %d", got[0].RespondentCount)
	}
	if len(got[0].ResponseDistribution) != 0 {
		t.Errorf("expected empty distribution, got %v", got[0].ResponseDistribution)
	}
}
