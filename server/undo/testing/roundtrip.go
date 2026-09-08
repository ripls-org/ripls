// Package undotesting provides a shared test harness for verifying
// server-authoritative undo RPCs satisfy the load-bearing A→B→A
// correctness invariant: for any sequence [perform-action,
// undo-action], post-undo state equals pre-action state.
//
// See docs/ai/undo_plan.md §10.5 for the design rationale.
//
// Usage: each Undo* RPC ships a test that supplies four callbacks
// (SnapshotBefore, Mutate, SnapshotAfter, Undo) plus the service
// handle it exercises. RoundTrip runs the cycle, compares every
// entity in the composite snapshot, and fails with a precise diff
// when anything doesn't round-trip.
package undotesting

import (
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// Snapshot is the composite, ordered collection of entities whose
// pre-action state the undo must restore. Typical composition: the
// primary entity, any mutated related entities (gear, community-gear,
// sibling transfers), stored ImpactEstimate, relevant chat messages
// filtered through the normal endpoint.
//
// Order matters for diff readability — the harness compares entries
// pairwise by index, so SnapshotBefore and SnapshotBefore-after-undo
// must produce the same sequence.
type Snapshot struct {
	Entries []SnapshotEntry
}

// SnapshotEntry pairs a human-readable label with a proto message so
// round-trip failures name the entity that drifted.
type SnapshotEntry struct {
	Label   string
	Message proto.Message
}

// Case describes a single round-trip test for one undoable action.
// Generic over the RPC-specific service handle so each domain
// (transfer, request, experience) can pass a strongly-typed service.
type Case[S any] struct {
	// Name appears in t.Run for easy identification on failure.
	Name string

	// SnapshotBefore captures the pre-action snapshot. Called both
	// before Mutate and after Undo; the two results must be equal.
	SnapshotBefore func(t *testing.T, s S) Snapshot

	// Mutate performs the forward action and returns the
	// community_event_id that Undo will reverse.
	Mutate func(t *testing.T, s S) (communityEventID string)

	// SnapshotAfter captures the post-action snapshot. Used only to
	// guard against vacuous tests where Mutate was a no-op.
	SnapshotAfter func(t *testing.T, s S) Snapshot

	// Undo performs the reversal. A failing undo call should fail the
	// test via t.Fatal inside the callback — the harness does not
	// inspect its error.
	Undo func(t *testing.T, s S, communityEventID string)
}

// RoundTrip runs one round-trip test case and asserts the
// A→B→undo→A' invariant: every entry in the composite snapshot has
// identical proto contents before the action and after the undo.
//
// The harness does NOT diff CommunityEvent tables between before and
// after — those legitimately differ by rows the undo flow itself
// produces (forward event + retraction). Callers should exclude
// CommunityEvent from the composite snapshot.
func RoundTrip[S any](t *testing.T, service S, tc Case[S]) {
	t.Helper()
	t.Run(tc.Name, func(t *testing.T) {
		t.Helper()

		snapshotA := tc.SnapshotBefore(t, service)
		if len(snapshotA.Entries) == 0 {
			t.Fatal("SnapshotBefore returned an empty snapshot — harness requires at least one entry")
		}

		communityEventID := tc.Mutate(t, service)
		if communityEventID == "" {
			t.Fatal("Mutate returned empty community_event_id — undo cannot be invoked without one")
		}

		snapshotB := tc.SnapshotAfter(t, service)
		if snapshotsEqual(snapshotA, snapshotB) {
			t.Fatal("Mutate did not change the snapshot — this is a vacuous test. " +
				"Check that SnapshotBefore includes every field the action is supposed to mutate.")
		}

		tc.Undo(t, service, communityEventID)

		snapshotAPrime := tc.SnapshotBefore(t, service)
		assertSnapshotsEqual(t, snapshotA, snapshotAPrime)
	})
}

func snapshotsEqual(a, b Snapshot) bool {
	if len(a.Entries) != len(b.Entries) {
		return false
	}
	for i := range a.Entries {
		if a.Entries[i].Label != b.Entries[i].Label {
			return false
		}
		if !proto.Equal(a.Entries[i].Message, b.Entries[i].Message) {
			return false
		}
	}
	return true
}

// assertSnapshotsEqual compares two snapshots entry-by-entry and
// reports a precise per-entity diff on mismatch.
func assertSnapshotsEqual(t *testing.T, want, got Snapshot) {
	t.Helper()
	if len(want.Entries) != len(got.Entries) {
		t.Fatalf("round-trip violation — snapshot length: want %d entries, got %d",
			len(want.Entries), len(got.Entries))
	}
	for i := range want.Entries {
		if want.Entries[i].Label != got.Entries[i].Label {
			t.Errorf("round-trip violation — entry %d label: want %q, got %q",
				i, want.Entries[i].Label, got.Entries[i].Label)
			continue
		}
		if !proto.Equal(want.Entries[i].Message, got.Entries[i].Message) {
			t.Errorf("round-trip violation — entry %q drifted after undo\n"+
				"---BEFORE ACTION---\n%s\n---AFTER UNDO---\n%s",
				want.Entries[i].Label,
				prototext.Format(want.Entries[i].Message),
				prototext.Format(got.Entries[i].Message),
			)
		}
	}
}
