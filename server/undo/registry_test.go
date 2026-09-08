package undo

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestEveryEventTypeClassified is the load-bearing enforcement for the
// generalized-undo feature (docs/ai/undo_plan.md §10.6). It fails when
// any CommunityEventType enum value lacks an entry in Registry —
// which is what we want when a contributor adds a new state-changing
// event without thinking through its undo classification.
//
// The fix when this test fails is to add an entry to
// server/undo/registry.go. The Classification doc comments describe
// the five valid categories; pick the one that matches and include a
// rationale sentence.
func TestEveryEventTypeClassified(t *testing.T) {
	for value, name := range models.CommunityEventType_name {
		eventType := models.CommunityEventType(value)

		// UNSPECIFIED is the proto enum zero value and not a real event.
		if eventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED {
			continue
		}

		entry, ok := Registry[eventType]
		if !ok {
			t.Errorf("CommunityEventType %s has no undo classification — add an entry in server/undo/registry.go", name)
			continue
		}
		if entry.Class == ClassificationUnspecified {
			t.Errorf("CommunityEventType %s has ClassificationUnspecified — pick one of the five real classifications", name)
		}
		if entry.Rationale == "" {
			t.Errorf("CommunityEventType %s is missing a Rationale — classifications without a reason become stale", name)
		}
		if (entry.Class == ClassificationServerSnackbar || entry.Class == ClassificationServerStory) && entry.UndoRPC == "" {
			t.Errorf("CommunityEventType %s is server-authoritative but has no UndoRPC name — set Entry.UndoRPC to the Undo* RPC(s) that reverse it", name)
		}
	}
}

// TestEveryServerUndoableActionHasUndoDataValidator fires once the
// Undo* RPC family lands (Phase 4+). In Phase 2 it is a warning only —
// server-authoritative entries that lack a ValidateUndoData function
// are expected until their implementing phase ships.
//
// When a new server-authoritative action is added in Phase 4+, this
// test promotes to a hard failure via removing the t.Logf exception
// and replacing it with t.Errorf. Leaving the scaffolding in place
// so the test is wired up and runs on every CI job.
func TestEveryServerUndoableActionHasUndoDataValidator(t *testing.T) {
	for eventType, entry := range Registry {
		if entry.Class != ClassificationServerSnackbar && entry.Class != ClassificationServerStory {
			continue
		}
		if entry.ValidateUndoData == nil {
			t.Logf("TODO: CommunityEventType %s has no ValidateUndoData — populate during Phase 4+ when %s ships",
				eventType.String(), entry.UndoRPC)
		}
	}
}

// TestClassificationString sanity-checks the human-readable labels
// used by the docs generator and registry_test output so mapping
// regressions show up immediately rather than in downstream tooling.
func TestClassificationString(t *testing.T) {
	cases := []struct {
		class Classification
		want  string
	}{
		{ClassificationUnspecified, "unspecified"},
		{ClassificationClientOnly, "client-only"},
		{ClassificationServerSnackbar, "server-snackbar"},
		{ClassificationServerStory, "server-story"},
		{ClassificationRetentionRestore, "retention-restore"},
		{ClassificationIrreversible, "irreversible"},
		{ClassificationInternal, "internal"},
	}
	for _, c := range cases {
		if got := c.class.String(); got != c.want {
			t.Errorf("Classification(%d).String() = %q, want %q", c.class, got, c.want)
		}
	}
}
