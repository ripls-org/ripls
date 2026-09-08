package undotesting

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// inMemoryState is a simple in-memory store for round-trip tests.
// Keys are string IDs, values are proto messages.
type inMemoryState struct {
	data map[string]proto.Message
}

func newInMemoryState() *inMemoryState {
	return &inMemoryState{data: make(map[string]proto.Message)}
}

// snapshot captures all entries as a Snapshot (ordered by insertion order via fixed keys).
func (s *inMemoryState) snapshot(keys []string) Snapshot {
	entries := make([]SnapshotEntry, 0, len(keys))
	for _, k := range keys {
		if msg, ok := s.data[k]; ok {
			entries = append(entries, SnapshotEntry{Label: k, Message: proto.Clone(msg)})
		}
	}
	return Snapshot{Entries: entries}
}

// TestRoundTrip_Success verifies that a correctly implemented undo cycle passes.
func TestRoundTrip_Success(t *testing.T) {
	state := newInMemoryState()
	state.data["user"] = &models.User{Id: "u1", Name: "Alice"}

	keys := []string{"user"}
	tc := Case[*inMemoryState]{
		Name: "name-change",
		SnapshotBefore: func(_ *testing.T, s *inMemoryState) Snapshot {
			return s.snapshot(keys)
		},
		Mutate: func(_ *testing.T, s *inMemoryState) string {
			s.data["user"] = &models.User{Id: "u1", Name: "Bob"}
			return "evt-001"
		},
		SnapshotAfter: func(_ *testing.T, s *inMemoryState) Snapshot {
			return s.snapshot(keys)
		},
		Undo: func(_ *testing.T, s *inMemoryState, _ string) {
			s.data["user"] = &models.User{Id: "u1", Name: "Alice"}
		},
	}

	RoundTrip(t, state, tc)
}

// TestSnapshotsEqual_Equal confirms identical snapshots are equal.
func TestSnapshotsEqual_Equal(t *testing.T) {
	msg := &models.User{Id: "u1", Name: "Alice"}
	snap := Snapshot{Entries: []SnapshotEntry{{Label: "user", Message: msg}}}
	if !snapshotsEqual(snap, snap) {
		t.Error("snapshotsEqual returned false for identical snapshots")
	}
}

// TestSnapshotsEqual_DifferentLength confirms snapshots of different lengths are not equal.
func TestSnapshotsEqual_DifferentLength(t *testing.T) {
	snap1 := Snapshot{Entries: []SnapshotEntry{
		{Label: "user", Message: &models.User{Id: "u1"}},
	}}
	snap2 := Snapshot{}
	if snapshotsEqual(snap1, snap2) {
		t.Error("snapshotsEqual returned true for snapshots with different entry counts")
	}
}

// TestSnapshotsEqual_DifferentMessage confirms snapshots differ when proto content differs.
func TestSnapshotsEqual_DifferentMessage(t *testing.T) {
	snap1 := Snapshot{Entries: []SnapshotEntry{{Label: "user", Message: &models.User{Id: "u1", Name: "Alice"}}}}
	snap2 := Snapshot{Entries: []SnapshotEntry{{Label: "user", Message: &models.User{Id: "u1", Name: "Bob"}}}}
	if snapshotsEqual(snap1, snap2) {
		t.Error("snapshotsEqual returned true for snapshots with differing message content")
	}
}

// TestSnapshotsEqual_DifferentLabel confirms snapshots differ when labels differ.
func TestSnapshotsEqual_DifferentLabel(t *testing.T) {
	msg := &models.User{Id: "u1"}
	snap1 := Snapshot{Entries: []SnapshotEntry{{Label: "user", Message: msg}}}
	snap2 := Snapshot{Entries: []SnapshotEntry{{Label: "profile", Message: msg}}}
	if snapshotsEqual(snap1, snap2) {
		t.Error("snapshotsEqual returned true for snapshots with differing labels")
	}
}

// TestAssertSnapshotsEqual_Equal confirms equal snapshots produce no test failure.
func TestAssertSnapshotsEqual_Equal(t *testing.T) {
	msg := &models.User{Id: "u1", Name: "Alice"}
	snap := Snapshot{Entries: []SnapshotEntry{{Label: "user", Message: proto.Clone(msg)}}}
	assertSnapshotsEqual(t, snap, snap)
	// If assertSnapshotsEqual called t.Errorf or t.Fatalf, t.Failed() would be true
	// and the test would be reported as failed. Reaching here means it correctly
	// detected equal snapshots.
}

// The tests below verify that RoundTrip reports the correct failure messages when
// the callbacks violate the harness contract. They use the subprocess idiom so that
// the expected failures from the harness do not mark the parent test as failed.
// Each sub-test sets ROUNDTRIP_FAILURE_CASE to direct the subprocess into the path
// that exercises the specific failure branch.

const envFailureCase = "ROUNDTRIP_FAILURE_CASE"

// runFailureSubprocess runs the current test binary in a subprocess with the given
// env value and asserts that it exits non-zero and produces output containing want.
func runFailureSubprocess(t *testing.T, caseEnv, want string) {
	t.Helper()
	cmd := exec.Command(os.Args[0],
		"-test.run=TestRoundTrip_FailureCases/"+strings.ReplaceAll(caseEnv, "_", ""),
		"-test.v",
	)
	cmd.Env = append(os.Environ(), envFailureCase+"="+caseEnv)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected subprocess to fail, but it passed. Output:\n%s", out)
		return
	}
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("expected output to contain %q. Got:\n%s", want, out)
	}
}

// TestRoundTrip_FailureCases bundles all expected-failure sub-tests so the
// subprocess flag (-test.run) can target them by sub-test name.
func TestRoundTrip_FailureCases(t *testing.T) {
	caseEnv := os.Getenv(envFailureCase)

	t.Run("EmptySnapshot", func(t *testing.T) {
		if caseEnv == "EmptySnapshot" {
			// Subprocess branch: exercise the empty-snapshot fatal path.
			state := newInMemoryState()
			tc := Case[*inMemoryState]{
				Name:           "empty-snap",
				SnapshotBefore: func(_ *testing.T, _ *inMemoryState) Snapshot { return Snapshot{} },
				Mutate:         func(_ *testing.T, _ *inMemoryState) string { return "evt-001" },
				SnapshotAfter:  func(_ *testing.T, _ *inMemoryState) Snapshot { return Snapshot{} },
				Undo:           func(_ *testing.T, _ *inMemoryState, _ string) {},
			}
			RoundTrip(t, state, tc)
			return
		}
		runFailureSubprocess(t, "EmptySnapshot", "at least one entry")
	})

	t.Run("EmptyCommunityEventID", func(t *testing.T) {
		if caseEnv == "EmptyCommunityEventID" {
			state := newInMemoryState()
			state.data["user"] = &models.User{Id: "u1"}
			keys := []string{"user"}
			tc := Case[*inMemoryState]{
				Name: "empty-event-id",
				SnapshotBefore: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Mutate: func(_ *testing.T, s *inMemoryState) string {
					s.data["user"] = &models.User{Id: "u1", Name: "Changed"}
					return "" // empty event ID
				},
				SnapshotAfter: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Undo: func(_ *testing.T, _ *inMemoryState, _ string) {},
			}
			RoundTrip(t, state, tc)
			return
		}
		runFailureSubprocess(t, "EmptyCommunityEventID", "empty community_event_id")
	})

	t.Run("VacuousMutate", func(t *testing.T) {
		if caseEnv == "VacuousMutate" {
			state := newInMemoryState()
			state.data["user"] = &models.User{Id: "u1", Name: "Alice"}
			keys := []string{"user"}
			tc := Case[*inMemoryState]{
				Name: "vacuous",
				SnapshotBefore: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Mutate: func(_ *testing.T, _ *inMemoryState) string {
					return "evt-001" // no-op: state unchanged
				},
				SnapshotAfter: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Undo: func(_ *testing.T, _ *inMemoryState, _ string) {},
			}
			RoundTrip(t, state, tc)
			return
		}
		runFailureSubprocess(t, "VacuousMutate", "vacuous test")
	})

	t.Run("DriftAfterUndo", func(t *testing.T) {
		if caseEnv == "DriftAfterUndo" {
			state := newInMemoryState()
			state.data["user"] = &models.User{Id: "u1", Name: "Alice"}
			keys := []string{"user"}
			tc := Case[*inMemoryState]{
				Name: "drift",
				SnapshotBefore: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Mutate: func(_ *testing.T, s *inMemoryState) string {
					s.data["user"] = &models.User{Id: "u1", Name: "Bob"}
					return "evt-001"
				},
				SnapshotAfter: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Undo: func(_ *testing.T, _ *inMemoryState, _ string) {
					// Bug: undo does not restore original state
				},
			}
			RoundTrip(t, state, tc)
			return
		}
		runFailureSubprocess(t, "DriftAfterUndo", "drifted after undo")
	})

	t.Run("LengthMismatchAfterUndo", func(t *testing.T) {
		if caseEnv == "LengthMismatchAfterUndo" {
			state := newInMemoryState()
			state.data["user"] = &models.User{Id: "u1", Name: "Alice"}
			state.data["peer"] = &models.User{Id: "u2", Name: "Bob"}
			keys := []string{"user", "peer"}
			tc := Case[*inMemoryState]{
				Name: "length-mismatch",
				SnapshotBefore: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Mutate: func(_ *testing.T, s *inMemoryState) string {
					s.data["user"] = &models.User{Id: "u1", Name: "Charlie"}
					return "evt-001"
				},
				SnapshotAfter: func(_ *testing.T, s *inMemoryState) Snapshot {
					return s.snapshot(keys)
				},
				Undo: func(_ *testing.T, s *inMemoryState, _ string) {
					// Bug: only restores one entry; SnapshotBefore will return two
					// but the undo removes "peer" so snapshot returns one entry.
					delete(s.data, "peer")
					s.data["user"] = &models.User{Id: "u1", Name: "Alice"}
				},
			}
			RoundTrip(t, state, tc)
			return
		}
		runFailureSubprocess(t, "LengthMismatchAfterUndo", "snapshot length")
	})
}
