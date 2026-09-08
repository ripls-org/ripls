package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// refCoordinator enforces ordering of steps within the same ref chain.
// Steps in different ref chains proceed independently. Within a chain,
// each step waits for the previous step to complete before executing.
type refCoordinator struct {
	mu     sync.Mutex
	chains map[string][]chan struct{}
}

// newRefCoordinator builds a coordinator from a timeline, creating per-step
// channels for each ref chain.
func newRefCoordinator(timeline Timeline) *refCoordinator {
	rc := &refCoordinator{
		chains: make(map[string][]chan struct{}),
	}

	// Build ordered step indices per ref.
	refSteps := make(map[string]int)
	for _, step := range timeline {
		if step.Ref == "" {
			continue
		}
		count := refSteps[step.Ref]
		ch := make(chan struct{})
		if count == 0 {
			// First step in chain is immediately ready.
			close(ch)
		}
		rc.chains[step.Ref] = append(rc.chains[step.Ref], ch)
		refSteps[step.Ref] = count + 1
	}

	return rc
}

// waitForReady blocks until the step at refIdx in the given ref chain is
// ready to execute. Returns an error if the context is cancelled.
func (rc *refCoordinator) waitForReady(ctx context.Context, ref string, refIdx int) error {
	rc.mu.Lock()
	chain := rc.chains[ref]
	rc.mu.Unlock()

	if refIdx >= len(chain) {
		return fmt.Errorf("refIdx %d out of range for ref %q (len=%d)", refIdx, ref, len(chain))
	}

	select {
	case <-chain[refIdx]:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// markCompleted signals that the step at refIdx in the given ref chain has
// finished, unblocking the next step.
func (rc *refCoordinator) markCompleted(ref string, refIdx int) {
	rc.mu.Lock()
	chain := rc.chains[ref]
	rc.mu.Unlock()

	nextIdx := refIdx + 1
	if nextIdx < len(chain) {
		close(chain[nextIdx])
	}
}

// buildRefIndexes returns a slice where entry i is the step's position within
// its ref chain. Steps with empty refs get index 0.
func buildRefIndexes(timeline Timeline) []int {
	refCounts := make(map[string]int)
	indexes := make([]int, len(timeline))
	for i, step := range timeline {
		if step.Ref == "" {
			indexes[i] = 0
			continue
		}
		indexes[i] = refCounts[step.Ref]
		refCounts[step.Ref]++
	}
	return indexes
}

// concurrentStep bundles a timeline step with its index and ref chain position.
type concurrentStep struct {
	timelineIdx int
	refIdx      int
	step        ActivityStep
}

// executeConcurrent runs the timeline with per-user goroutines and ref-based
// ordering. Each user gets a dedicated Client and goroutine. A semaphore limits
// the number of goroutines actively executing RPCs.
//
// Locking strategy:
//   - Write actions hold state.wlock for the entire executor call (they
//     interleave map reads, RPCs, and map writes). This serializes write
//     actions but they are a small fraction of total steps.
//   - Read actions run without any lock (they only access immutable maps
//     populated during setup). Counter increments use a brief wlock.
//   - injectContextualReads runs under wlock (same scope as the write action).
//   - Chat injection runs under wlock (accesses mutable conversation maps).
func executeConcurrent(ctx context.Context, serverURL string, state *State, timeline Timeline, rng *rand.Rand, readTraffic bool, maxWorkers int) error {
	// Load templates (shared, read-only across goroutines).
	experiences, _ := LoadExperienceTemplates()
	requests, _ := LoadRequestTemplates()
	venues, _ := LoadVenues()
	chatTemplates, _ := LoadChatTemplates()
	queryPool := SearchQueryPool()

	// Atomic template indexes (shared across goroutines).
	var expIdx atomic.Int64
	var reqIdx atomic.Int64

	// Build per-user client pool. Each user gets a dedicated Client with its
	// own transport and a fixed auth token.
	pool := NewClientPool(serverURL, state.UserTokens)
	getClient := pool.For

	// Build ref coordinator and per-step ref indexes.
	coord := newRefCoordinator(timeline)
	refIndexes := buildRefIndexes(timeline)

	// Collect unique actors and create per-user channels.
	userChans := make(map[string]chan concurrentStep)
	for _, step := range timeline {
		if _, ok := userChans[step.Actor]; !ok {
			userChans[step.Actor] = make(chan concurrentStep, 64)
		}
	}

	// Semaphore for concurrency limiting.
	sem := make(chan struct{}, maxWorkers)

	// Progress tracking.
	var executed atomic.Int64
	var skippedAtomic atomic.Int64
	var failedAtomic atomic.Int64
	totalSteps := int64(len(timeline))

	// Launch per-user goroutines.
	var wg sync.WaitGroup
	for email, ch := range userChans {
		wg.Add(1)
		// Each goroutine gets its own RNG seeded from the main RNG.
		//nolint:gosec // G404: derived from the run's seeded RNG so each goroutine is
		// reproducible too. See runner.go for why determinism is the requirement.
		userRng := rand.New(rand.NewSource(rng.Int63()))
		userEmail := email
		userCh := ch

		go func() {
			defer wg.Done()

			client := pool.For(userEmail)
			if client == nil {
				slog.Warn("no client for user goroutine", "email", userEmail)
				return
			}

			for cs := range userCh {
				step := cs.step

				// Acquire semaphore.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					skippedAtomic.Add(1)
					state.wlock(func() { state.StepsSkipped++ })
					// Still mark completed so downstream steps unblock.
					if step.Ref != "" {
						coord.markCompleted(step.Ref, cs.refIdx)
					}
					continue
				}

				err := executeConcurrentStep(
					ctx, client, state, step, cs, coord, getClient, userRng,
					readTraffic, experiences, requests, venues,
					queryPool, &expIdx, &reqIdx,
				)

				if err != nil {
					failedAtomic.Add(1)
					state.wlock(func() {
						state.StepsFailed++
						if isCriticalAction(step.Action) {
							state.FailedRefs[step.Ref] = true
						}
					})
					slog.Warn("activity step failed",
						"step", cs.timelineIdx,
						"action", step.Action,
						"actor", step.Actor,
						"ref", step.Ref,
						"time", step.Time.Format("2006-01-02"),
						"error", err,
					)
				} else {
					// Send supplemental chat messages after successful actions.
					if chatTemplates != nil {
						state.wlock(func() {
							injectChat(ctx, getClient, state, userRng, chatTemplates, step)
						})
					}
					// Mark terminal actions.
					if isTerminalAction(step.Action) {
						state.wlock(func() {
							state.CompletedRefs[step.Ref] = true
						})
					}
				}

				// Always mark completed on coordinator so dependent steps unblock.
				if step.Ref != "" {
					coord.markCompleted(step.Ref, cs.refIdx)
				}

				// Release semaphore.
				<-sem

				n := executed.Add(1)
				if n%100 == 0 {
					slog.Info("simulation progress (concurrent)",
						"executed", n,
						"total", totalSteps,
						"skipped", skippedAtomic.Load(),
						"failed", failedAtomic.Load(),
						"sim_date", step.Time.Format("2006-01-02"),
					)
				}
			}
		}()
	}

	// Dispatch steps to user channels in timeline order.
	for i, step := range timeline {
		if ctx.Err() != nil {
			skippedAtomic.Add(1)
			state.wlock(func() { state.StepsSkipped++ })
			continue
		}

		userChans[step.Actor] <- concurrentStep{
			timelineIdx: i,
			refIdx:      refIndexes[i],
			step:        step,
		}
	}

	// Close all user channels to signal goroutines to exit.
	for _, ch := range userChans {
		close(ch)
	}

	// Wait for all goroutines to finish.
	wg.Wait()

	return nil
}

// executeConcurrentStep runs a single timeline step within a concurrent worker.
//
// Write actions (state-mutating) hold state.wlock for the entire executor call.
// Read actions (only accessing immutable maps) run without locks; their counter
// increment uses a brief wlock.
func executeConcurrentStep(
	ctx context.Context,
	client *Client,
	state *State,
	step ActivityStep,
	cs concurrentStep,
	coord *refCoordinator,
	getClient func(email string) *Client,
	rng *rand.Rand,
	readTraffic bool,
	experiences []ExperienceTemplate,
	requests []RequestTemplate,
	venues map[string][]Venue,
	queryPool []string,
	expIdx *atomic.Int64,
	reqIdx *atomic.Int64,
) error {
	// Check if this ref is already completed or failed (skip if so).
	if step.Action != ActionSaveGear {
		var skip bool
		state.rlock(func() {
			skip = state.CompletedRefs[step.Ref] || state.FailedRefs[step.Ref]
		})
		if skip {
			state.wlock(func() { state.StepsSkipped++ })
			return nil
		}
	}

	// Wait for ref predecessor (no lock held — this may block).
	if step.Ref != "" {
		if err := coord.waitForReady(ctx, step.Ref, cs.refIdx); err != nil {
			return fmt.Errorf("wait for ref predecessor: %w", err)
		}
		// Re-check after waiting — predecessor may have failed or completed.
		if step.Action != ActionSaveGear {
			var skip bool
			state.rlock(func() {
				skip = state.CompletedRefs[step.Ref] || state.FailedRefs[step.Ref]
			})
			if skip {
				state.wlock(func() { state.StepsSkipped++ })
				return nil
			}
		}
	}

	// Per-step context with timeout.
	stepCtx, stepCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stepCancel()

	client.SetTimestamp(step.Time)

	// Read actions only access immutable maps — no lock needed for the RPC.
	if isReadAction(step.Action) {
		err := executeReadAction(stepCtx, client, state, step, rng, queryPool)
		if err == nil {
			state.wlock(func() { state.ReadsExecuted++ })
		}
		return err
	}

	// Write actions hold wlock for the entire call (interleaves map access + RPCs).
	var err error
	state.wlock(func() {
		// Inject contextual reads (reads mutable maps like GearIDs).
		if readTraffic {
			reads := injectContextualReads(stepCtx, client, state, step, rng)
			state.ReadsExecuted += reads
		}

		err = executeWriteAction(stepCtx, client, state, step, getClient, rng,
			experiences, requests, venues, expIdx, reqIdx)
	})
	return err
}

// executeReadAction dispatches a read action to the appropriate executor.
// Read actions only access immutable maps (CommunityIDs, UserIDs, etc.)
// so they can run without holding the state lock.
func executeReadAction(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand, queryPool []string) error {
	switch step.Action {
	case ActionCheckFeed:
		return executeCheckFeed(ctx, client, state, step, rng)
	case ActionSearchCommunity:
		return executeSearchCommunity(ctx, client, state, step, rng, queryPool)
	case ActionBrowseGear:
		return executeBrowseGear(ctx, client, state, step, rng)
	case ActionViewProfile:
		return executeViewProfile(ctx, client, state, step, rng)
	case ActionCheckStories:
		return executeCheckStories(ctx, client, state, step)
	case ActionCheckConversations:
		return executeCheckConversations(ctx, client, rng)
	case ActionViewImpact:
		return executeViewImpact(ctx, client, state, step, rng)
	default:
		return fmt.Errorf("unknown read action: %d", step.Action)
	}
}

// executeWriteAction dispatches a write action to the appropriate executor.
// Must be called while holding state.wlock.
func executeWriteAction(
	ctx context.Context,
	client *Client,
	state *State,
	step ActivityStep,
	getClient func(email string) *Client,
	rng *rand.Rand,
	experiences []ExperienceTemplate,
	requests []RequestTemplate,
	venues map[string][]Venue,
	expIdx *atomic.Int64,
	reqIdx *atomic.Int64,
) error {
	switch step.Action {
	case ActionSaveGear:
		return executeSaveGear(ctx, client, state, step, rng)
	case ActionExpressInterest:
		return executeExpressInterest(ctx, client, state, step, false, getClient)
	case ActionSelectRecipient:
		return executeSelectRecipient(ctx, client, state, step)
	case ActionStartLoan:
		return executeStartLoan(ctx, client, state, step)
	case ActionCompleteLoan:
		return executeCompleteTransfer(ctx, client, state, step)
	case ActionExpressInterestGiveaway:
		return executeExpressInterest(ctx, client, state, step, true, getClient)
	case ActionSelectGiveaway:
		return executeSelectRecipient(ctx, client, state, step)
	case ActionCompleteGiveaway:
		return executeCompleteTransfer(ctx, client, state, step)
	case ActionSubmitRequest:
		idx := int(reqIdx.Add(1) - 1)
		return executeSubmitRequest(ctx, client, state, step, requests, &idx)
	case ActionOfferToFulfill:
		return executeOfferToFulfill(ctx, client, state, step)
	case ActionFulfillRequest:
		return executeFulfillRequest(ctx, client, state, step)
	case ActionSaveExperience:
		idx := int(expIdx.Add(1) - 1)
		return executeSaveExperience(ctx, client, state, step, experiences, &idx, rng, venues)
	case ActionShareExperience:
		return executeShareExperience(ctx, client, state, step)
	case ActionRSVP:
		return executeRSVP(ctx, client, state, step, false)
	case ActionRSVPNo:
		return executeRSVP(ctx, client, state, step, true)
	case ActionStartExperience:
		return executeStartExperience(ctx, client, state, step)
	case ActionCompleteExperience:
		return executeCompleteExperience(ctx, client, state, step)
	case ActionCancelExperience:
		return executeCancelExperience(ctx, client, state, step)
	case ActionWithdrawInterest:
		return executeWithdrawInterest(ctx, client, state, step)
	case ActionCancelTransfer:
		return executeCancelTransfer(ctx, client, state, step)
	case ActionCancelRequest:
		return executeCancelRequest(ctx, client, state, step)
	case ActionWithdrawOffer:
		return executeWithdrawOffer(ctx, client, state, step)
	default:
		return fmt.Errorf("unknown write action: %d", step.Action)
	}
}
