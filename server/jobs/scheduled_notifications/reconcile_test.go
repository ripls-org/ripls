package scheduled_notifications

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Compile-time assertion that fakeReconcileStorage satisfies ReconcileStorage.
var _ ReconcileStorage = (*fakeReconcileStorage)(nil)

// fakeReconcileStorage is an in-memory ReconcileStorage that records
// every mutation so tests can assert on driver decisions.
type fakeReconcileStorage struct {
	mu               sync.Mutex
	rowsByID         map[string]*models.ScheduledNotification
	nextID           int
	inserts          int
	deletes          int
	fireAtUpdates    int
	failInsertError  error // optional: cause every Insert to fail
	failDeleteError  error // optional: cause every Delete to fail
	failFireAtError  error // optional: cause every UpdateFireAt to fail
	updateReturnsNop bool  // simulate "row was deleted between Existing() and update"
}

func newFakeReconcileStorage() *fakeReconcileStorage {
	return &fakeReconcileStorage{rowsByID: map[string]*models.ScheduledNotification{}}
}

// Insert is kept for direct pre-population in tests but is no longer
// part of ReconcileStorage (the reconciler uses InsertScheduledNotificationIfAbsent).
func (f *fakeReconcileStorage) Insert(_ context.Context, msg proto.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failInsertError != nil {
		return "", f.failInsertError
	}
	row := msg.(*models.ScheduledNotification)
	f.nextID++
	row.Id = fmt.Sprintf("id-%d", f.nextID)
	clone := proto.Clone(row).(*models.ScheduledNotification)
	f.rowsByID[row.Id] = clone
	f.inserts++
	return row.Id, nil
}

func (f *fakeReconcileStorage) InsertScheduledNotificationIfAbsent(_ context.Context, row *models.ScheduledNotification) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failInsertError != nil {
		return false, "", f.failInsertError
	}
	key := UniquenessKey(row)
	// Simulate ON CONFLICT DO NOTHING: check for any existing row
	// with the same uniqueness key.
	for _, existing := range f.rowsByID {
		if UniquenessKey(existing) == key {
			return false, row.Id, nil
		}
	}
	f.nextID++
	row.Id = fmt.Sprintf("id-%d", f.nextID)
	clone := proto.Clone(row).(*models.ScheduledNotification)
	f.rowsByID[row.Id] = clone
	f.inserts++
	return true, row.Id, nil
}

func (f *fakeReconcileStorage) Delete(_ context.Context, msg proto.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDeleteError != nil {
		return f.failDeleteError
	}
	row := msg.(*models.ScheduledNotification)
	delete(f.rowsByID, row.Id)
	f.deletes++
	return nil
}

func (f *fakeReconcileStorage) UpdateScheduledNotificationFireAt(_ context.Context, row *models.ScheduledNotification, fireAtUnixSec, updatedAtUnixSec int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFireAtError != nil {
		return false, f.failFireAtError
	}
	if f.updateReturnsNop {
		return false, nil
	}
	stored, ok := f.rowsByID[row.Id]
	if !ok {
		return false, nil
	}
	stored.FireAtUnixSec = fireAtUnixSec
	stored.UpdatedAtUnixSec = updatedAtUnixSec
	row.FireAtUnixSec = fireAtUnixSec
	row.UpdatedAtUnixSec = updatedAtUnixSec
	f.fireAtUpdates++
	return true, nil
}

// stubReconciler returns canned Desired and Existing sets.
type stubReconciler struct {
	name     string
	desired  []*models.ScheduledNotification
	existing []*models.ScheduledNotification
	desErr   error
	exiErr   error
}

func (s *stubReconciler) Name() string { return s.name }
func (s *stubReconciler) Desired(_ context.Context) ([]*models.ScheduledNotification, error) {
	return s.desired, s.desErr
}

func (s *stubReconciler) Existing(_ context.Context) ([]*models.ScheduledNotification, error) {
	return s.existing, s.exiErr
}

func experienceRow(recipientID, experienceID string, purpose models.ExperienceNotificationPurpose, offset, fireAt int64) *models.ScheduledNotification {
	return &models.ScheduledNotification{
		RecipientUserId: recipientID,
		FireAtUnixSec:   fireAt,
		Item: &models.ScheduledNotification_Experience{
			Experience: &models.ExperienceNotification{
				ExperienceId:            experienceID,
				Purpose:                 purpose,
				OffsetSecondsFromAnchor: offset,
			},
		},
	}
}

func TestReconcile_InsertsMissingRows(t *testing.T) {
	store := newFakeReconcileStorage()
	r := &stubReconciler{
		name: "test",
		desired: []*models.ScheduledNotification{
			experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100),
			experienceRow("u-2", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100),
		},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Inserted != 2 {
		t.Errorf("Inserted = %d, want 2", stats.Inserted)
	}
	if store.inserts != 2 {
		t.Errorf("storage Inserts = %d, want 2", store.inserts)
	}
}

func TestReconcile_UpdatesFireAtWhenChanged(t *testing.T) {
	store := newFakeReconcileStorage()
	existing := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	existing.Id = "id-1"
	store.rowsByID["id-1"] = proto.Clone(existing).(*models.ScheduledNotification)

	desiredNewFireAt := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 500)

	r := &stubReconciler{
		name:     "test",
		desired:  []*models.ScheduledNotification{desiredNewFireAt},
		existing: []*models.ScheduledNotification{existing},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Updated != 1 {
		t.Errorf("Updated = %d, want 1", stats.Updated)
	}
	if store.fireAtUpdates != 1 {
		t.Errorf("storage UpdateFireAt = %d, want 1", store.fireAtUpdates)
	}
	if store.rowsByID["id-1"].FireAtUnixSec != 500 {
		t.Errorf("persisted FireAt = %d, want 500", store.rowsByID["id-1"].FireAtUnixSec)
	}
}

func TestReconcile_LeavesUnchangedRowsAlone(t *testing.T) {
	store := newFakeReconcileStorage()
	existing := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	existing.Id = "id-1"
	store.rowsByID["id-1"] = proto.Clone(existing).(*models.ScheduledNotification)

	desiredSame := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)

	r := &stubReconciler{
		name:     "test",
		desired:  []*models.ScheduledNotification{desiredSame},
		existing: []*models.ScheduledNotification{existing},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", stats.Unchanged)
	}
	if store.inserts != 0 || store.deletes != 0 || store.fireAtUpdates != 0 {
		t.Errorf("expected no mutations; got inserts=%d deletes=%d updates=%d",
			store.inserts, store.deletes, store.fireAtUpdates)
	}
}

func TestReconcile_DeletesFutureRowsOutOfDesiredSet(t *testing.T) {
	// Use a fixed clock and a fire_at in the future relative to it.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ctx := clock.WithSimulationTime(context.Background(), now)
	store := newFakeReconcileStorage()
	stale := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, now.Unix()+3600)
	stale.Id = "id-1"
	store.rowsByID["id-1"] = proto.Clone(stale).(*models.ScheduledNotification)

	r := &stubReconciler{
		name:     "test",
		desired:  nil, // entity went away
		existing: []*models.ScheduledNotification{stale},
	}

	stats, err := Reconcile(ctx, store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if _, present := store.rowsByID["id-1"]; present {
		t.Error("stale row still in storage after reconcile")
	}
}

func TestReconcile_LeavesPastFireAtRowsForDispatcher(t *testing.T) {
	// A row whose fire_at is already in the past must not be deleted
	// by the reconciler — the dispatcher owns past-due rows. This
	// prevents the reconciler from racing ahead of a brief
	// dispatcher outage and silently dropping un-fired reminders.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ctx := clock.WithSimulationTime(context.Background(), now)
	store := newFakeReconcileStorage()

	pastDue := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, now.Unix()-1800)
	pastDue.Id = "id-1"
	store.rowsByID["id-1"] = proto.Clone(pastDue).(*models.ScheduledNotification)

	r := &stubReconciler{
		name:     "test",
		desired:  nil, // reconciler doesn't re-emit (past grace)
		existing: []*models.ScheduledNotification{pastDue},
	}

	stats, err := Reconcile(ctx, store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (past-fire-at row belongs to dispatcher)", stats.Deleted)
	}
	if _, present := store.rowsByID["id-1"]; !present {
		t.Error("past-fire-at row was incorrectly deleted")
	}
}

func TestReconcile_HandlesDuplicateDesiredTuples(t *testing.T) {
	store := newFakeReconcileStorage()
	dup1 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	dup2 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 200)

	r := &stubReconciler{
		name:    "test",
		desired: []*models.ScheduledNotification{dup1, dup2},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Last-write-wins dedup: one insert.
	if stats.Inserted != 1 {
		t.Errorf("Inserted = %d, want 1 (deduped)", stats.Inserted)
	}
}

func TestReconcile_SkipsRowsWithUnsetItemVariant(t *testing.T) {
	store := newFakeReconcileStorage()
	bad := &models.ScheduledNotification{
		RecipientUserId: "u-1",
		FireAtUnixSec:   100,
	}

	r := &stubReconciler{
		name:    "test",
		desired: []*models.ScheduledNotification{bad},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Inserted != 0 {
		t.Errorf("Inserted = %d, want 0 (unset item should be skipped)", stats.Inserted)
	}
}

func TestReconcile_ConcurrentDispatcherDeletedRow(t *testing.T) {
	// The reconciler loads an existing row and tries to update its
	// fire_at. Between Existing() and UpdateScheduledNotificationFireAt,
	// the dispatcher claims and deletes it. UpdateFireAt returns
	// (false, nil); the driver counts it as Unchanged and moves on.
	store := newFakeReconcileStorage()
	store.updateReturnsNop = true

	existing := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	existing.Id = "id-1"

	desiredNewFireAt := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 500)

	r := &stubReconciler{
		name:     "test",
		desired:  []*models.ScheduledNotification{desiredNewFireAt},
		existing: []*models.ScheduledNotification{existing},
	}

	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Updated != 0 {
		t.Errorf("Updated = %d, want 0", stats.Updated)
	}
	if stats.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", stats.Unchanged)
	}
}

func TestReconcile_PropagatesLoadErrors(t *testing.T) {
	store := newFakeReconcileStorage()
	r := &stubReconciler{
		name:   "test",
		desErr: errors.New("storage on fire"),
	}
	if _, err := Reconcile(context.Background(), store, r); err == nil {
		t.Error("expected Desired() error to propagate")
	}
}

func TestReconcile_BestEffortPerRowErrors(t *testing.T) {
	store := newFakeReconcileStorage()
	store.failInsertError = errors.New("constraint violation")

	r := &stubReconciler{
		name: "test",
		desired: []*models.ScheduledNotification{
			experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100),
			experienceRow("u-2", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100),
		},
	}
	stats, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("Reconcile returned top-level error: %v", err)
	}
	if stats.Errored != 2 {
		t.Errorf("Errored = %d, want 2", stats.Errored)
	}
	if stats.Inserted != 0 {
		t.Errorf("Inserted = %d, want 0", stats.Inserted)
	}
}

// TestReconcile_ReapsDuplicateExistingRows is the Phase 1 regression test
// for #2458. Two rows with the same uniqueness key and future fire_at exist
// in storage (post-race steady state). After one Reconcile tick the reaper
// must leave exactly one row.
func TestReconcile_ReapsDuplicateExistingRows(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ctx := clock.WithSimulationTime(context.Background(), now)
	store := newFakeReconcileStorage()

	fireAt := now.Unix() + 3600 // future
	row1 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row1.Id = "id-aaaa" // smaller id — will be kept
	row2 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row2.Id = "id-bbbb"
	store.rowsByID["id-aaaa"] = proto.Clone(row1).(*models.ScheduledNotification)
	store.rowsByID["id-bbbb"] = proto.Clone(row2).(*models.ScheduledNotification)

	r := &stubReconciler{
		name: "test",
		desired: []*models.ScheduledNotification{
			experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt),
		},
		existing: []*models.ScheduledNotification{row1, row2},
	}

	stats, err := Reconcile(ctx, store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.ReapedDuplicates != 1 {
		t.Errorf("ReapedDuplicates = %d, want 1", stats.ReapedDuplicates)
	}
	if len(store.rowsByID) != 1 {
		t.Errorf("storage has %d rows after reap, want 1", len(store.rowsByID))
	}
	if _, ok := store.rowsByID["id-aaaa"]; !ok {
		t.Error("smallest-id row (id-aaaa) should be kept")
	}
	if _, ok := store.rowsByID["id-bbbb"]; ok {
		t.Error("larger-id row (id-bbbb) should be reaped")
	}
}

// TestReconcile_InsertIfAbsentPreventsConflictDuplicate is the Phase 1
// regression test for the multi-replica insert race (#2458). Two reconciler
// ticks both see an empty Existing() set and both try to insert the same key.
// The second tick's InsertScheduledNotificationIfAbsent must return
// inserted=false so no duplicate row is created.
func TestReconcile_InsertIfAbsentPreventsConflictDuplicate(t *testing.T) {
	store := newFakeReconcileStorage()

	fireAt := time.Now().Unix() + 3600
	r := &stubReconciler{
		name: "test",
		desired: []*models.ScheduledNotification{
			experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt),
		},
		// existing = nil: both replicas see an empty set (pre-commit race).
	}

	// First tick (replica A): inserts successfully.
	stats1, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if stats1.Inserted != 1 {
		t.Errorf("first tick: Inserted = %d, want 1", stats1.Inserted)
	}
	if len(store.rowsByID) != 1 {
		t.Errorf("first tick: storage has %d rows, want 1", len(store.rowsByID))
	}

	// Second tick (replica B): also sees empty Existing() (stale read) but
	// the store already has the row. InsertIfAbsent must detect the conflict.
	stats2, err := Reconcile(context.Background(), store, r)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if stats2.Inserted != 0 {
		t.Errorf("second tick: Inserted = %d, want 0 (conflict)", stats2.Inserted)
	}
	if stats2.Unchanged != 1 {
		t.Errorf("second tick: Unchanged = %d, want 1 (conflict counted as unchanged)", stats2.Unchanged)
	}
	if len(store.rowsByID) != 1 {
		t.Errorf("second tick: storage has %d rows, want 1 (no duplicate)", len(store.rowsByID))
	}
}

// TestReconcile_ReaperLeavesPastFireAtDuplicates verifies the dispatcher
// ownership invariant: duplicate rows with a past fire_at are not reaped
// because the dispatcher owns them.
func TestReconcile_ReaperLeavesPastFireAtDuplicates(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ctx := clock.WithSimulationTime(context.Background(), now)
	store := newFakeReconcileStorage()

	fireAt := now.Unix() - 3600 // PAST — dispatcher territory
	row1 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row1.Id = "id-aaaa"
	row2 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row2.Id = "id-bbbb"
	store.rowsByID["id-aaaa"] = proto.Clone(row1).(*models.ScheduledNotification)
	store.rowsByID["id-bbbb"] = proto.Clone(row2).(*models.ScheduledNotification)

	r := &stubReconciler{
		name:     "test",
		desired:  nil, // past grace; reconciler doesn't re-emit
		existing: []*models.ScheduledNotification{row1, row2},
	}

	stats, err := Reconcile(ctx, store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.ReapedDuplicates != 0 {
		t.Errorf("ReapedDuplicates = %d, want 0 (past-fire-at rows belong to dispatcher)", stats.ReapedDuplicates)
	}
	if len(store.rowsByID) != 2 {
		t.Errorf("storage has %d rows, want 2 (neither past-due row should be touched)", len(store.rowsByID))
	}
}

// TestReconcile_ReapsTripletToSingle verifies that the reaper handles three
// duplicate rows: two are reaped, one (smallest id) is kept.
func TestReconcile_ReapsTripletToSingle(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ctx := clock.WithSimulationTime(context.Background(), now)
	store := newFakeReconcileStorage()

	fireAt := now.Unix() + 3600 // future
	row1 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row1.Id = "id-aaaa" // keeper (smallest)
	row2 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row2.Id = "id-bbbb"
	row3 := experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt)
	row3.Id = "id-cccc"
	store.rowsByID["id-aaaa"] = proto.Clone(row1).(*models.ScheduledNotification)
	store.rowsByID["id-bbbb"] = proto.Clone(row2).(*models.ScheduledNotification)
	store.rowsByID["id-cccc"] = proto.Clone(row3).(*models.ScheduledNotification)

	r := &stubReconciler{
		name: "test",
		desired: []*models.ScheduledNotification{
			experienceRow("u-1", "e-1", models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, fireAt),
		},
		existing: []*models.ScheduledNotification{row1, row2, row3},
	}

	stats, err := Reconcile(ctx, store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.ReapedDuplicates != 2 {
		t.Errorf("ReapedDuplicates = %d, want 2", stats.ReapedDuplicates)
	}
	if len(store.rowsByID) != 1 {
		t.Errorf("storage has %d rows, want 1", len(store.rowsByID))
	}
	if _, ok := store.rowsByID["id-aaaa"]; !ok {
		t.Error("smallest-id row (id-aaaa) should be kept")
	}
}
