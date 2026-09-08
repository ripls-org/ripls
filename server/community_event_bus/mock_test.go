package community_event_bus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestMockBus_CapturesPublishesAndAssignsIDs(t *testing.T) {
	bus := NewMockBus()

	first, err := bus.Publish(context.Background(), &models.CommunityEvent{CommunityId: "c1"})
	if err != nil {
		t.Fatalf("Publish 1: %v", err)
	}
	second, err := bus.Publish(context.Background(), &models.CommunityEvent{CommunityId: "c2"})
	if err != nil {
		t.Fatalf("Publish 2: %v", err)
	}

	if first == "" || second == "" || first == second {
		t.Errorf("expected unique non-empty event IDs, got %q and %q", first, second)
	}

	got := bus.Captured()
	if len(got) != 2 {
		t.Fatalf("captured %d events, want 2", len(got))
	}
	if got[0].Id != first {
		t.Errorf("captured[0].Id = %q, want %q (Publish should write ID back into the event)", got[0].Id, first)
	}
}

func TestMockBus_PublishErrorSurfacedAndRowNotCaptured(t *testing.T) {
	bus := NewMockBus()
	bus.SetPublishError(errors.New("forced storage failure"))

	id, err := bus.Publish(context.Background(), &models.CommunityEvent{CommunityId: "c1"})
	if err == nil || !strings.Contains(err.Error(), "forced") {
		t.Errorf("Publish err = %v, want forced error", err)
	}
	if id != "" {
		t.Errorf("Publish id = %q, want empty on error", id)
	}
	if got := bus.Captured(); len(got) != 0 {
		t.Errorf("captured %d events on forced error, want 0", len(got))
	}
}

func TestMockBus_Reset(t *testing.T) {
	bus := NewMockBus()
	_, _ = bus.Publish(context.Background(), &models.CommunityEvent{})
	bus.Reset()
	if got := bus.Captured(); len(got) != 0 {
		t.Errorf("after Reset, captured = %v, want empty", got)
	}
	id, _ := bus.Publish(context.Background(), &models.CommunityEvent{})
	if id != "evt-1" {
		t.Errorf("after Reset, first id = %q, want evt-1", id)
	}
}
