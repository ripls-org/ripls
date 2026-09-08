package request

// Race regression tests for #2900: async writers clobbering concurrent state changes.
//
// The *BeforeWrite channels are unbuffered: the goroutine blocks at send until
// the test receives. This gives the test a deterministic window to commit a
// CancelRequest/UpdateRequest BEFORE releasing the goroutine into its WithTx.
// With the fix (SELECT FOR UPDATE + locked reload) the goroutine reads the
// post-cancel state and preserves it; without the fix (UpdateIfNotDeleted on a
// stale snapshot) it overwrites it, causing the final state to be ACTIVE.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// activateRaceChannels wires unbuffered *BeforeWrite channels into svc and returns
// all four race-test signal channels. The channels are nil in a plain NewForTesting
// service so that goroutines run unblocked in non-race tests; this function enables
// the blocking behavior needed for the deterministic race pattern (#2900).
//
// Returns: (stockImageryBeforeWrite, suggestionChipsBeforeWrite, suggestionChipsDone, stockImageryDone).
// stockImageryDone is already wired by NewForTesting; it is returned here for convenience.
func activateRaceChannels(svc *Service) (imgBefore, chipsBefore, chipsDone chan struct{}) {
	imgBefore = make(chan struct{})    // Unbuffered: goroutine blocks until test receives.
	chipsBefore = make(chan struct{})  // Unbuffered: goroutine blocks until test receives.
	chipsDone = make(chan struct{}, 1) // Buffered 1: goroutine signals without blocking.
	svc.stockImageryBeforeWrite = imgBefore
	svc.suggestionChipsBeforeWrite = chipsBefore
	svc.suggestionChipsDone = chipsDone
	return imgBefore, chipsBefore, chipsDone
}

// setupRaceTestService creates a service wired with all the test-signal channels
// needed for deterministic race testing. Returns the service plus every signal
// channel; callers that don't need a particular channel can ignore it.
func setupRaceTestService(t *testing.T) (
	svc *Service,
	testStorage *storage.ProtoSQLStorage,
	stockImageryDone chan struct{},
	stockImageryBeforeWrite chan struct{},
	suggestionChipsBeforeWrite chan struct{},
	suggestionChipsDone chan struct{},
) {
	t.Helper()
	ts := setupTestStorage(t)

	tmpDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("setupRaceTestService: bucket: %v", err)
	}

	mockNotif := notifications.NewMockService()
	fakeProvider := media.NewFakeProvider(ts, bucket)
	mockAI := ai.NewMockProvider()
	systemMsgWriter := chat.NewSystemMessageWriter(ts, nil)

	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(ts, topic)
	notifSub := commsub.New(ts, mockNotif)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("setupRaceTestService: subscribe: %v", err)
	}

	service, notifDone, imgDone := NewForTesting(ts, bucket, mockNotif, bus, fakeProvider, mockAI, systemMsgWriter)
	imgBefore, chipsBefore, chipsDone := activateRaceChannels(service)

	tap := &doneTapSubscriber{ch: notifDone}
	if _, err := bus.Subscribe(tap); err != nil {
		t.Fatalf("setupRaceTestService: subscribe tap: %v", err)
	}

	return service, ts, imgDone, imgBefore, chipsBefore, chipsDone
}

// waitCh waits for a signal on ch with a 10-second timeout.
func waitCh(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for %s (10s)", label)
	}
}

// TestService_CancelRequest_RaceWithSuggestionChips verifies that a CancelRequest
// committed while the suggestion-chips goroutine is paused at its *BeforeWrite
// point is preserved in the final state. Without the tx-guarded fix the goroutine
// would overwrite State=CANCELLED with its stale State=ACTIVE snapshot.
func TestService_CancelRequest_RaceWithSuggestionChips(t *testing.T) {
	svc, testStorage, stockImageryDone, stockImageryBeforeWrite, chipsBefore, chipsDone := setupRaceTestService(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Provide MediaIds so the stock-imagery goroutine is not launched —
	// only the chips goroutine participates in this test.
	submitResp, err := svc.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Power drill",
		Description: "Looking for a power drill for a weekend project",
		MediaIds:    []string{"placeholder-media-id"},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId

	// Sanity: stock imagery goroutine must NOT have been launched (channel empty).
	select {
	case <-stockImageryDone:
		t.Fatal("stock imagery goroutine launched unexpectedly when MediaIds was provided")
	case <-stockImageryBeforeWrite:
		t.Fatal("stockImageryBeforeWrite triggered unexpectedly when MediaIds was provided")
	default:
	}

	// The chips goroutine is running and will block at chipsBefore before entering
	// WithTx. Call CancelRequest NOW — the goroutine hasn't entered its tx yet, so
	// this commit lands before the goroutine reads the row under its lock.
	if _, err := svc.CancelRequest(ctx, connect.NewRequest(&api.CancelRequestRequest{
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}

	// Release the chips goroutine into WithTx. It will do SELECT FOR UPDATE,
	// read State=CANCELLED (committed above), copy chips onto that snapshot,
	// and write State=CANCELLED with AdditionalAsks populated.
	waitCh(t, chipsBefore, "suggestionChipsBeforeWrite")

	// Wait for the goroutine to commit (or fail).
	services.WaitForSuggestionChips(t, chipsDone)

	// Final state must be CANCELLED. Without the fix the goroutine would have
	// overwritten it back to ACTIVE.
	final := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, final); err != nil {
		t.Fatalf("GetByID after race: %v", err)
	}
	if final.State != models.RequestState_REQUEST_STATE_CANCELLED {
		t.Errorf("State = %v, want CANCELLED — async writer clobbered CancelRequest", final.State)
	}
	// The goroutine read CANCELLED state under lock, copied its fields onto it,
	// and committed: AdditionalAsks must be populated.
	if len(final.AdditionalAsks) == 0 {
		t.Error("AdditionalAsks is empty — chips goroutine did not persist its fields")
	}
}

// TestService_UpdateRequest_RaceWithSuggestionChips verifies that an UpdateRequest
// (title change) committed while the chips goroutine is paused at its *BeforeWrite
// point is preserved — specifically, the new title survives the goroutine's write.
func TestService_UpdateRequest_RaceWithSuggestionChips(t *testing.T) {
	svc, testStorage, _, stockImageryBeforeWrite, chipsBefore, chipsDone := setupRaceTestService(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	submitResp, err := svc.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Original title",
		Description: "Looking for something",
		MediaIds:    []string{"placeholder-media-id"},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId

	select {
	case <-stockImageryBeforeWrite:
		t.Fatal("stockImageryBeforeWrite triggered unexpectedly when MediaIds was provided")
	default:
	}

	// Update the title while the chips goroutine is running toward its *BeforeWrite.
	if _, err := svc.UpdateRequest(ctx, connect.NewRequest(&api.UpdateRequestRequest{
		RequestId:   requestID,
		Title:       "Updated title",
		Description: "Looking for something (updated)",
	})); err != nil {
		t.Fatalf("UpdateRequest: %v", err)
	}

	// Release the chips goroutine. It reads the post-update snapshot under lock,
	// copies chips onto it, and commits — the title must survive.
	waitCh(t, chipsBefore, "suggestionChipsBeforeWrite")
	services.WaitForSuggestionChips(t, chipsDone)

	final := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, final); err != nil {
		t.Fatalf("GetByID after race: %v", err)
	}
	if final.Title != "Updated title" {
		t.Errorf("Title = %q, want %q — chips goroutine reverted the update", final.Title, "Updated title")
	}
	if len(final.AdditionalAsks) == 0 {
		t.Error("AdditionalAsks is empty — chips goroutine did not persist its fields")
	}
}

// TestService_DeleteRequest_RaceWithSuggestionChips verifies that the chips goroutine
// does not resurrect a request that is soft-deleted while the goroutine is in flight.
// The goroutine is paused at *BeforeWrite after it passes the pre-tx early-out
// (which sees ACTIVE); DeleteRequest then commits; when released the goroutine reads
// the deleted row under its SELECT FOR UPDATE lock and skips the write.
//
// Both outcomes are valid: if the goroutine wins the row lock first (commits
// ACTIVE + chips) DeleteRequest then commits Deleted; if DeleteRequest wins, the
// goroutine reads Deleted under lock and skips. Either way Deleted != nil.
func TestService_DeleteRequest_RaceWithSuggestionChips(t *testing.T) {
	svc, testStorage, _, _, chipsBefore, chipsDone := setupRaceTestService(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	submitResp, err := svc.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Item to delete",
		Description: "Will be deleted mid-race",
		MediaIds:    []string{"placeholder-media-id"},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId

	// Wait for the chips goroutine to pass the pre-tx early-out (which sees ACTIVE
	// because DeleteRequest hasn't run yet) and block at *BeforeWrite.
	// Receiving here releases the goroutine into WithTx.
	waitCh(t, chipsBefore, "suggestionChipsBeforeWrite")

	// Delete the request while the goroutine is entering WithTx.
	// The goroutine's SELECT FOR UPDATE and DeleteRequest's UPDATE race for the row
	// lock — whichever wins, the final state has Deleted != nil.
	if _, err := svc.DeleteRequest(ctx, connect.NewRequest(&api.DeleteRequestRequest{
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}

	services.WaitForSuggestionChips(t, chipsDone)

	// The request must be soft-deleted regardless of which writer won the lock.
	deletedReq := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, deletedReq,
		storage.QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("GetByID(IncludeDeleted) after race: %v", err)
	}
	if deletedReq.Deleted == nil {
		t.Error("request is not soft-deleted — chips goroutine resurrected a deleted row")
	}
}

// TestService_CancelRequest_RaceWithStockImagery verifies that a CancelRequest
// committed while the stock-imagery goroutine is paused at its *BeforeWrite
// point is preserved. The goroutine reads State=CANCELLED under lock, sets only
// MediaIds, and commits — the CANCELLED state must survive.
func TestService_CancelRequest_RaceWithStockImagery(t *testing.T) {
	svc, testStorage, stockImageryDone, stockImageryBeforeWrite, chipsBefore, chipsDone := setupRaceTestService(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// No MediaIds → stock-imagery goroutine IS launched.
	// Override the AI provider to return nil so the chips goroutine exits early
	// and never reaches its *BeforeWrite, leaving only the imagery goroutine active.
	svc.aiProvider.(*ai.MockProvider).GenerateRequestSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.RequestSuggestionResult, error) {
		return nil, nil
	}

	submitResp, err := svc.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Camping tent",
		Description: "Need a four-person tent for this weekend",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId

	// Chips goroutine exits early (AI returned nil); drain its done channel.
	services.WaitForSuggestionChips(t, chipsDone)
	// Sanity: chipsBefore should not have fired.
	select {
	case <-chipsBefore:
		t.Fatal("chips goroutine reached *BeforeWrite unexpectedly — AI mock returned nil")
	default:
	}

	// Cancel the request while the stock-imagery goroutine is paused at its *BeforeWrite.
	if _, err := svc.CancelRequest(ctx, connect.NewRequest(&api.CancelRequestRequest{
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("CancelRequest: %v", err)
	}

	// Release the imagery goroutine into WithTx. It reads State=CANCELLED under lock,
	// sets MediaIds = [mediaID], and commits — State must remain CANCELLED.
	waitCh(t, stockImageryBeforeWrite, "stockImageryBeforeWrite")
	services.WaitForStockImagery(t, stockImageryDone)

	final := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, final); err != nil {
		t.Fatalf("GetByID after race: %v", err)
	}
	if final.State != models.RequestState_REQUEST_STATE_CANCELLED {
		t.Errorf("State = %v, want CANCELLED — stock imagery goroutine clobbered CancelRequest", final.State)
	}
	// MediaIds must be populated — the goroutine committed its field.
	if len(final.MediaIds) == 0 {
		t.Error("MediaIds is empty — stock imagery goroutine did not persist its field")
	}
}
