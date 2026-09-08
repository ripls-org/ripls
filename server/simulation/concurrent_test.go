package simulation

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRefCoordinatorOrdering(t *testing.T) {
	// 3-step chain: step 0 is immediately ready, step 1 waits for 0, step 2 waits for 1.
	timeline := Timeline{
		{Ref: "flow-a", Action: ActionSaveGear},
		{Ref: "flow-a", Action: ActionExpressInterest},
		{Ref: "flow-a", Action: ActionStartLoan},
	}
	coord := newRefCoordinator(timeline)

	ctx := context.Background()
	var order []int
	var mu sync.Mutex

	var wg sync.WaitGroup
	// Launch all 3 in reverse order to prove ordering works.
	for i := 2; i >= 0; i-- {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			if err := coord.waitForReady(ctx, "flow-a", idx); err != nil {
				t.Errorf("step %d: unexpected error: %v", idx, err)
				return
			}
			mu.Lock()
			order = append(order, idx)
			mu.Unlock()
			coord.markCompleted("flow-a", idx)
		}()
	}

	wg.Wait()

	if len(order) != 3 {
		t.Fatalf("expected 3 completions, got %d", len(order))
	}
	for i, v := range order {
		if v != i {
			t.Errorf("expected order[%d]=%d, got %d", i, i, v)
		}
	}
}

func TestRefCoordinatorIndependentRefs(t *testing.T) {
	// Two independent chains should proceed concurrently.
	timeline := Timeline{
		{Ref: "flow-a", Action: ActionSaveGear},
		{Ref: "flow-b", Action: ActionSaveGear},
		{Ref: "flow-a", Action: ActionExpressInterest},
		{Ref: "flow-b", Action: ActionExpressInterest},
	}
	coord := newRefCoordinator(timeline)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Both step 0s should be immediately ready.
	for _, ref := range []string{"flow-a", "flow-b"} {
		wg.Add(1)
		r := ref
		go func() {
			defer wg.Done()
			if err := coord.waitForReady(ctx, r, 0); err != nil {
				t.Errorf("ref %s step 0: unexpected error: %v", r, err)
			}
			coord.markCompleted(r, 0)
		}()
	}
	wg.Wait()

	// Both step 1s should now be ready.
	for _, ref := range []string{"flow-a", "flow-b"} {
		wg.Add(1)
		r := ref
		go func() {
			defer wg.Done()
			if err := coord.waitForReady(ctx, r, 1); err != nil {
				t.Errorf("ref %s step 1: unexpected error: %v", r, err)
			}
			coord.markCompleted(r, 1)
		}()
	}
	wg.Wait()
}

func TestRefCoordinatorContextCancel(t *testing.T) {
	// A blocked step should return when the context is cancelled.
	timeline := Timeline{
		{Ref: "flow-a", Action: ActionSaveGear},
		{Ref: "flow-a", Action: ActionExpressInterest},
	}
	coord := newRefCoordinator(timeline)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// Step 1 waits for step 0, which we never complete.
		done <- coord.waitForReady(ctx, "flow-a", 1)
	}()

	// Cancel the context after a short delay.
	time.Sleep(10 * time.Millisecond) //nolint:forbidigo // lets the goroutine above enter waitForReady before cancellation
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from cancelled context, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled step")
	}
}

func TestClientPoolCreation(t *testing.T) {
	serverURL := "http://localhost:8080"

	state := NewState()
	state.UserTokens["alice@example.com"] = "token-alice"
	state.UserTokens["bob@example.com"] = "token-bob"

	pool := make(map[string]*Client, len(state.UserTokens))
	for email, token := range state.UserTokens {
		c := NewClient(serverURL)
		c.AsUser(token)
		pool[email] = c
	}

	if len(pool) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(pool))
	}

	alice := pool["alice@example.com"]
	if alice == nil {
		t.Fatal("alice client is nil")
	}
	if alice.CurrentToken() != "token-alice" {
		t.Errorf("alice token = %q, want %q", alice.CurrentToken(), "token-alice")
	}

	bob := pool["bob@example.com"]
	if bob == nil {
		t.Fatal("bob client is nil")
	}
	if bob.CurrentToken() != "token-bob" {
		t.Errorf("bob token = %q, want %q", bob.CurrentToken(), "token-bob")
	}

	// Clients must be distinct objects.
	if alice == bob {
		t.Error("alice and bob clients are the same object")
	}
}

func TestBuildRefChains(t *testing.T) {
	timeline := Timeline{
		{Ref: "gear-1"},        // 0: gear-1[0]
		{Ref: "gear-2"},        // 1: gear-2[0]
		{Ref: "gear-1-loan-1"}, // 2: gear-1-loan-1[0]
		{Ref: "gear-1-loan-1"}, // 3: gear-1-loan-1[1]
		{Ref: "gear-2"},        // 4: gear-2[1]
		{Ref: ""},              // 5: empty ref[0]
		{Ref: "gear-1-loan-1"}, // 6: gear-1-loan-1[2]
	}

	indexes := buildRefIndexes(timeline)

	expected := []int{0, 0, 0, 1, 1, 0, 2}
	for i, want := range expected {
		if indexes[i] != want {
			t.Errorf("indexes[%d] = %d, want %d", i, indexes[i], want)
		}
	}
}

func TestConcurrentStateAccess(t *testing.T) {
	// Verify that N goroutines can read/write state maps without races.
	// This test is meaningful when run with -race.
	state := NewState()

	var wg sync.WaitGroup
	n := 50

	// Writers: write to mutable maps under wlock.
	for i := 0; i < n; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			ref := "ref-" + string(rune('a'+idx%26))
			state.wlock(func() {
				state.GearIDs[ref] = "gear-id"
				state.TransferIDs[ref] = "transfer-id"
				state.GearCreated++
			})
		}()
	}

	// Readers: read from maps under rlock.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state.rlock(func() {
				_ = state.CompletedRefs["some-ref"]
				_ = state.FailedRefs["some-ref"]
				_ = len(state.GearIDs)
			})
		}()
	}

	wg.Wait()
}
