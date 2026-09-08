package feed

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestConsumeNudge_RequiresAuth verifies that calling ConsumeNudge without an
// authenticated context returns CodeUnauthenticated.
func TestConsumeNudge_RequiresAuth(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	_, err := svc.ConsumeNudge(context.Background(), connect.NewRequest(&api.ConsumeNudgeRequest{
		NudgeId: "any-nudge-id",
		Action:  "plan_experience",
	}))
	if err == nil {
		t.Fatal("expected unauthenticated error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

// TestConsumeNudge_MarksConsumed inserts a nudge, calls ConsumeNudge, then
// asserts the nudge is excluded from active nudges and has consumed fields set.
func TestConsumeNudge_MarksConsumed(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "consume-nudge@test.com", "Consume User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Consume Community")

	mediaID := "media-consume"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Will be consumed",
		Description:      "desc",
		CtaLabel:         "Go",
		CtaAction:        "plan_experience",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
		IsTerminator:     false,
	}
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	authCtx := createAuthenticatedContext(userID, "consume-nudge@test.com", models.Role_ROLE_USER)
	_, err := svc.ConsumeNudge(authCtx, connect.NewRequest(&api.ConsumeNudgeRequest{
		NudgeId: nudge.Id,
		Action:  "plan_experience",
	}))
	if err != nil {
		t.Fatalf("ConsumeNudge: %v", err)
	}

	// The nudge must no longer appear in the active pool.
	active, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	for _, n := range active {
		if n.Id == nudge.Id {
			t.Error("consumed nudge must not appear in active nudges")
		}
	}

	// The raw record must have ConsumedAtUnixSec set and ConsumedAction matching.
	fetched := &models.StoredNudge{}
	if err := sqlStorage.GetByID(ctx, nudge.Id, fetched); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched.ConsumedAtUnixSec == nil {
		t.Error("ConsumedAtUnixSec must be set after ConsumeNudge")
	}
	if fetched.ConsumedAction == nil || *fetched.ConsumedAction != "plan_experience" {
		t.Errorf("ConsumedAction = %v, want %q", fetched.ConsumedAction, "plan_experience")
	}
}

// TestConsumeNudge_RepeatedCalls_NoError verifies that calling ConsumeNudge twice
// for the same nudge ID returns success both times. The semantics are "last writer
// wins; both calls are accepted" — no deduplication guard exists.
func TestConsumeNudge_RepeatedCalls_NoError(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "repeat-nudge@test.com", "Repeat User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Repeat Community")

	mediaID := "media-repeat"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Repeated consume",
		Description:      "desc",
		CtaLabel:         "Go",
		CtaAction:        "check_listings",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
		IsTerminator:     false,
	}
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	authCtx := createAuthenticatedContext(userID, "repeat-nudge@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.ConsumeNudgeRequest{
		NudgeId: nudge.Id,
		Action:  "check_listings",
	})

	if _, err := svc.ConsumeNudge(authCtx, req); err != nil {
		t.Fatalf("first ConsumeNudge: %v", err)
	}
	if _, err := svc.ConsumeNudge(authCtx, req); err != nil {
		t.Fatalf("second ConsumeNudge: %v", err)
	}

	// Nudge must still be excluded from active pool after both calls.
	active, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	for _, n := range active {
		if n.Id == nudge.Id {
			t.Error("nudge must remain absent from active pool after repeated ConsumeNudge calls")
		}
	}
}

// TestConsumeNudge_ConcurrentCallers_NoError spawns 10 goroutines all calling
// ConsumeNudge for the same nudge ID and asserts that none return an error.
// This exercises the "idempotency under concurrent consumers" requirement from
// the issue: the implementation uses a last-writer-wins update, so all concurrent
// calls are accepted.
func TestConsumeNudge_ConcurrentCallers_NoError(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "concurrent-nudge@test.com", "Concurrent User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Concurrent Community")

	mediaID := "media-concurrent"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     2,
		Headline:         "Concurrent consume",
		Description:      "desc",
		CtaLabel:         "Go",
		CtaAction:        "plan_experience",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
		IsTerminator:     false,
	}
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	authCtx := createAuthenticatedContext(userID, "concurrent-nudge@test.com", models.Role_ROLE_USER)

	const concurrency = 10
	errs := make([]error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.ConsumeNudge(authCtx, connect.NewRequest(&api.ConsumeNudgeRequest{
				NudgeId: nudge.Id,
				Action:  "plan_experience",
			}))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: ConsumeNudge error: %v", i, err)
		}
	}

	// Nudge must be excluded from active pool after concurrent consumption.
	active, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	for _, n := range active {
		if n.Id == nudge.Id {
			t.Error("nudge must not appear in active pool after concurrent ConsumeNudge calls")
		}
	}
}

// TestConsumeNudge_UnknownNudge verifies that ConsumeNudge returns CodeInternal
// when the nudge ID does not exist. The storage layer surfaces a "not found"
// error which the handler wraps as Internal — this test pins that behavior.
func TestConsumeNudge_UnknownNudge(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "unknown-nudge@test.com", "Unknown User")
	authCtx := createAuthenticatedContext(userID, "unknown-nudge@test.com", models.Role_ROLE_USER)

	_, err := svc.ConsumeNudge(authCtx, connect.NewRequest(&api.ConsumeNudgeRequest{
		NudgeId: "00000000-0000-0000-0000-000000000000",
		Action:  "plan_experience",
	}))
	if err == nil {
		t.Fatal("expected error for unknown nudge ID, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}
