package momentum

import (
	"context"
	"testing"
)

func TestCopyGenerator_PassthroughOnValidCopy(t *testing.T) {
	det := &Detection{
		Slot:        SlotActiveQuest,
		Headline:    "Time to bring the smoker back.",
		Description: "You've hosted it 5 times — worth keeping the rhythm going.",
		CtaLabel:    "Schedule the smoker again?",
		CtaAction:   "schedule_repeat",
		ContextID:   "exp-1",
		KickerLabel: "On the way",
	}
	got := NewCopyGenerator().Generate(context.Background(), det)
	if got == nil {
		t.Fatal("expected detection to pass guards, got nil")
	}
	if got.Headline != det.Headline {
		t.Errorf("Headline mutated; passthrough should be identity. want %q got %q", det.Headline, got.Headline)
	}
}

func TestCopyGenerator_DropsOnLeverGuardFailure(t *testing.T) {
	// 11-word lever fails the ≤9 ceiling.
	det := &Detection{
		Slot:     SlotActiveQuest,
		CtaLabel: "Schedule the next round of the smoker series for some Saturday soon",
	}
	got := NewCopyGenerator().Generate(context.Background(), det)
	if got != nil {
		t.Errorf("expected nil (guard rejected lever), got %+v", got)
	}
}

func TestCopyGenerator_NilInputReturnsNil(t *testing.T) {
	if got := NewCopyGenerator().Generate(context.Background(), nil); got != nil {
		t.Errorf("expected nil for nil input, got %+v", got)
	}
}
