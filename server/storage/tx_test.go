package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestWithTx_CommitsOnSuccess(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityID := "tx-commit"

	err := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		if _, err := tx.Insert(ctx, &models.CommunityUser{
			CommunityId: communityID, UserId: "u-1",
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx returned error: %v", err)
	}

	rows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("post-commit query: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("rows = %d, want 1 (inserted row should be visible after commit)", len(rows))
	}
}

func TestWithTx_RollsBackOnError(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityID := "tx-rollback"
	sentinel := errors.New("intentional fn failure")

	err := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		if _, err := tx.Insert(ctx, &models.CommunityUser{
			CommunityId: communityID, UserId: "u-1",
		}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("WithTx error = %v, want sentinel %v", err, sentinel)
	}

	rows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("post-rollback query: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0 (rolled-back insert should not be visible)", len(rows))
	}
}

func TestWithTx_RollsBackOnPanic(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityID := "tx-panic"

	defer func() {
		// Recover the panic that propagates out of WithTx.
		if r := recover(); r == nil {
			t.Errorf("expected panic to propagate, got nil")
		}

		// And verify the row never landed.
		rows, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("post-panic query: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("rows = %d, want 0 (panic should have rolled back)", len(rows))
		}
	}()

	_ = store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		if _, err := tx.Insert(ctx, &models.CommunityUser{
			CommunityId: communityID, UserId: "u-1",
		}); err != nil {
			return err
		}
		panic("boom")
	})
}

func TestWithTx_HardDeleteCascadeIsAtomic(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	communityID, err := store.Insert(ctx, &models.Community{Name: "atom", OwnerUserId: "owner"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: "u-1"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: "g-1"}); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	sentinel := errors.New("simulated mid-cascade failure")
	err = store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		if _, err := HardDeleteCommunityUserByCommunityID(ctx, tx, communityID); err != nil {
			return err
		}
		// Bail before deleting community_gear: the user delete
		// must roll back to keep the cascade atomic.
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}

	users, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("CommunityUser rows = %d, want 1 (rolled back)", len(users))
	}

	gear, err := store.QueryByField(ctx, "community_id", communityID, &models.CommunityGear{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("query gear: %v", err)
	}
	if len(gear) != 1 {
		t.Errorf("CommunityGear rows = %d, want 1 (never reached, but rollback safety check)", len(gear))
	}
}

func TestWithTx_RespectsBeginTxError(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	// A cancelled context should make BeginTx fail before fn
	// runs at all. The error wraps sql.ErrConnDone or
	// context.Canceled depending on driver state; we just assert
	// fn never executed.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	err := store.WithTx(cancelled, nil, func(tx *ProtoSQLStorage) error {
		called = true
		return nil
	})
	if err == nil {
		t.Errorf("expected error from BeginTx with cancelled context")
	}
	if called {
		t.Errorf("fn should not run when BeginTx fails")
	}
}

// TestDeferAfterCommit_RunsAfterCommit verifies that a hook registered with
// deferAfterCommit has NOT run inside the tx callback but HAS run after WithTx
// returns successfully.
func TestDeferAfterCommit_RunsAfterCommit(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	var ran bool

	err := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		tx.deferAfterCommit(func() { ran = true })
		if ran {
			t.Error("hook must not run inside the tx callback")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if !ran {
		t.Error("hook must run after successful commit")
	}
}

// TestDeferAfterCommit_SkippedOnRollback verifies that a deferred hook is NOT
// invoked when the transaction rolls back (fn returns an error).
func TestDeferAfterCommit_SkippedOnRollback(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	var ran bool
	sentinel := errors.New("forced rollback")

	err := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		tx.deferAfterCommit(func() { ran = true })
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTx error = %v, want sentinel", err)
	}
	if ran {
		t.Error("hook must not run after rollback")
	}
}

// TestDeferAfterCommit_ImmediateOutsideTx verifies that calling deferAfterCommit
// on a non-transactional storage runs the hook immediately.
func TestDeferAfterCommit_ImmediateOutsideTx(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	var ran bool
	store.deferAfterCommit(func() { ran = true })
	if !ran {
		t.Error("hook must run immediately when not in a transaction")
	}
}

// readOnlyTxOpts pins the read-only path so tests don't drift if
// we add more opts later.
var readOnlyTxOpts = &sql.TxOptions{ReadOnly: true}

func TestWithTx_ReadOnlyOpts(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Inserting inside a read-only tx must fail at the database
	// level. Pins that opts are wired through.
	err := store.WithTx(ctx, readOnlyTxOpts, func(tx *ProtoSQLStorage) error {
		_, insertErr := tx.Insert(ctx, &models.CommunityUser{
			CommunityId: "ro", UserId: "u-1",
		})
		return insertErr
	})
	if err == nil {
		t.Errorf("expected insert to fail inside read-only tx")
	}
}
