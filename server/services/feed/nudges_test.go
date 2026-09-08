package feed

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
)

// --- terminator pool ---.

func TestGetTerminatorForSlot_Rotation(t *testing.T) {
	poolSize := len(terminatorPool)
	if poolSize == 0 {
		t.Fatal("terminatorPool must not be empty")
	}

	// Slots within a day must all differ (pool is large enough).
	if poolSize >= TerminatorsPerDay {
		headlines := make(map[string]bool)
		for slot := 0; slot < TerminatorsPerDay; slot++ {
			e := GetTerminatorForSlot(1, slot)
			if headlines[e.Headline] {
				t.Errorf("slot %d: duplicate headline %q within same day", slot, e.Headline)
			}
			headlines[e.Headline] = true
		}
	}

	// The full cycle repeats after poolSize days (slot 0).
	e1 := GetTerminatorForSlot(1, 0)
	cycleLen := poolSize / TerminatorsPerDay
	if cycleLen < 1 {
		cycleLen = 1
	}
	e2 := GetTerminatorForSlot(1+cycleLen*TerminatorsPerDay, 0)
	if e1.Headline != e2.Headline {
		t.Errorf("expected slot 0 to repeat after full cycle: day 1=%q, day %d=%q",
			e1.Headline, 1+cycleLen*TerminatorsPerDay, e2.Headline)
	}
}

func TestGetTerminatorForSlot_NeverEmpty(t *testing.T) {
	for _, day := range []int{0, 1, 365, 366, 730} {
		for slot := 0; slot < TerminatorsPerDay; slot++ {
			e := GetTerminatorForSlot(day, slot)
			if e.Headline == "" {
				t.Errorf("day %d slot %d: expected non-empty headline", day, slot)
			}
			if e.StockQuery == "" {
				t.Errorf("day %d slot %d: expected non-empty StockQuery", day, slot)
			}
		}
	}
}

// --- nudgeToFeedItem ---.

func TestNudgeToFeedItem_Fields(t *testing.T) {
	mediaID := "media-1"
	secLabel := "Secondary"
	secAction := "secondary_action"
	hint := "outdoor"

	n := &models.StoredNudge{
		Id:                 "nudge-1",
		NudgeVariant:       2,
		Headline:           "Test headline",
		Description:        "Test description",
		CtaLabel:           "Do it",
		CtaAction:          "plan_experience",
		MediaId:            &mediaID,
		SecondaryCtaLabel:  &secLabel,
		SecondaryCtaAction: &secAction,
		LocationHint:       &hint,
		CreatedAtUnixSec:   12345,
		Stats: []*models.StoredNudgeStat{
			{Value: "5", Label: "items shared"},
		},
	}

	item := nudgeToFeedItem(n)

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_NUDGE {
		t.Errorf("expected NUDGE item type, got %v", item.ItemType)
	}
	if item.Id != "nudge-1" {
		t.Errorf("expected id nudge-1, got %q", item.Id)
	}
	if item.OccurredAtUnixSec != 12345 {
		t.Errorf("expected OccurredAtUnixSec 12345, got %d", item.OccurredAtUnixSec)
	}

	nudge := item.GetNudge()
	if nudge == nil {
		t.Fatal("expected nudge payload")
	}
	if nudge.NudgeId != "nudge-1" {
		t.Errorf("expected nudge_id nudge-1, got %q", nudge.NudgeId)
	}
	if nudge.NudgeVariant != 2 {
		t.Errorf("expected variant 2, got %d", nudge.NudgeVariant)
	}
	if len(nudge.MediaIds) != 1 || nudge.MediaIds[0] != mediaID {
		t.Errorf("expected mediaIds [%q], got %v", mediaID, nudge.MediaIds)
	}
	if nudge.SecondaryCtaLabel == nil || *nudge.SecondaryCtaLabel != secLabel {
		t.Errorf("expected SecondaryCtaLabel %q", secLabel)
	}
	if nudge.SecondaryCtaAction == nil || *nudge.SecondaryCtaAction != secAction {
		t.Errorf("expected SecondaryCtaAction %q", secAction)
	}
	if nudge.LocationHint == nil || *nudge.LocationHint != hint {
		t.Errorf("expected LocationHint %q", hint)
	}
	if len(nudge.Stats) != 1 || nudge.Stats[0].Value != "5" {
		t.Errorf("expected stats [{5, items shared}], got %v", nudge.Stats)
	}
}

func TestNudgeToFeedItem_NoMedia(t *testing.T) {
	n := &models.StoredNudge{
		Id:               "nudge-2",
		NudgeVariant:     1,
		Headline:         "No image yet",
		Description:      "desc",
		CtaLabel:         "Go",
		CtaAction:        "check_listings",
		CreatedAtUnixSec: 0,
	}

	item := nudgeToFeedItem(n)
	nudge := item.GetNudge()
	if len(nudge.MediaIds) != 0 {
		t.Errorf("expected no media IDs, got %v", nudge.MediaIds)
	}
}

// --- terminator placement ---.

// TestAppendTerminator_NoInterleavedNudges pins the invariant #2936
// established: the only nudge-typed card the feed produces is the
// terminator. Generic nudges used to be interleaved every eight items from
// a per-user LLM-written pool.
func TestAppendTerminator_NoInterleavedNudges(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "nudge@test.com", "Nudge User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Insert 16 content items — twice the old interleave interval, so a
	// regression that restores interleaving shows up as extra cards.
	const contentCount = 16
	content := make([]*api.FeedItem, contentCount)
	for i := range content {
		content[i] = &api.FeedItem{
			Id:       "content-" + string(rune('a'+i)),
			ItemType: api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED,
		}
	}

	// Stored nudges in the pool must not surface in the feed.
	now := time.Now().Unix()
	mediaID := "media-1"
	for i := 0; i < 2; i++ {
		_, err := sqlStorage.Insert(context.Background(), &models.StoredNudge{
			UserId:           userID,
			CommunityId:      communityID,
			NudgeVariant:     1,
			Headline:         "Test nudge",
			Description:      "desc",
			CtaLabel:         "Go",
			CtaAction:        "plan_experience",
			MediaId:          &mediaID,
			CreatedAtUnixSec: now,
			IsTerminator:     false,
		})
		if err != nil {
			t.Fatalf("failed to insert nudge: %v", err)
		}
	}

	logger := logging.LoggerWithContext(context.Background())
	result := svc.appendTerminator(context.Background(), userID, content, logger)

	nudgeCount := 0
	for _, item := range result {
		if item.ItemType == api.FeedItemType_FEED_ITEM_TYPE_NUDGE {
			nudgeCount++
		}
	}

	if nudgeCount > 1 {
		t.Errorf("expected at most 1 nudge item (the terminator), got %d — "+
			"pooled nudges must not be interleaved into the feed (#2936)", nudgeCount)
	}
	if len(result) < contentCount {
		t.Errorf("content items were dropped: got %d, want at least %d", len(result), contentCount)
	}
}

func TestAppendTerminator_EmptyContent(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "empty@test.com", "Empty User")

	logger := logging.LoggerWithContext(context.Background())
	result := svc.appendTerminator(context.Background(), userID, nil, logger)

	// With no content only the terminator is appended.
	if len(result) > 1 {
		t.Errorf("expected at most 1 item (terminator), got %d", len(result))
	}
}

// --- nudge lifecycle (storage layer) ---.

func TestNudgeLifecycle_InsertAndFetch(t *testing.T) {
	sqlStorage := setupTestStorage(t)

	userID := setupTestUser(t, sqlStorage, "lifecycle@test.com", "Lifecycle User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Lifecycle Community")

	ctx := context.Background()
	mediaID := "media-123"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Lifecycle test",
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

	fetched, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	if len(fetched) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(fetched))
	}
	if fetched[0].Headline != "Lifecycle test" {
		t.Errorf("unexpected headline: %q", fetched[0].Headline)
	}
}

func TestNudgeLifecycle_ConsumeExcludesFromFetch(t *testing.T) {
	sqlStorage := setupTestStorage(t)

	userID := setupTestUser(t, sqlStorage, "consume@test.com", "Consume User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Consume Community")

	ctx := context.Background()
	mediaID := "media-abc"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     2,
		Headline:         "Will be consumed",
		Description:      "desc",
		CtaLabel:         "Tap",
		CtaAction:        "check_listings",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
		IsTerminator:     false,
	}

	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	if err := markNudgeConsumed(ctx, sqlStorage, nudge.Id, "check_listings"); err != nil {
		t.Fatalf("markNudgeConsumed: %v", err)
	}

	fetched, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	for _, n := range fetched {
		if n.Id == nudge.Id {
			t.Error("consumed nudge should not appear in active nudges")
		}
	}
}

func TestNudgeLifecycle_SoftDeleteExcludesFromFetch(t *testing.T) {
	sqlStorage := setupTestStorage(t)

	userID := setupTestUser(t, sqlStorage, "softdel@test.com", "SoftDel User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "SoftDel Community")

	ctx := context.Background()
	mediaID := "media-del"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Will be deleted",
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
	if err := softDeleteNudge(ctx, sqlStorage, nudge.Id); err != nil {
		t.Fatalf("softDeleteNudge: %v", err)
	}

	fetched, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	for _, n := range fetched {
		if n.Id == nudge.Id {
			t.Error("soft-deleted nudge should not appear in active nudges")
		}
	}
}

// --- global terminator pool per day ---.

func TestBuildTerminator_CreatesGlobalPool(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "term@test.com", "Term User")

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)

	// First call creates TerminatorsPerDay global records.
	item := svc.buildTerminator(ctx, userID, logger)
	if item == nil {
		t.Fatal("expected non-nil terminator item")
	}

	today := time.Now().UTC().YearDay()
	all, err := getGlobalTerminatorsForToday(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getGlobalTerminatorsForToday: %v", err)
	}
	if len(all) != TerminatorsPerDay {
		t.Errorf("expected %d global terminator records, got %d", TerminatorsPerDay, len(all))
	}

	// Second call (different user) should not create additional records.
	userID2 := setupTestUser(t, sqlStorage, "term2@test.com", "Term User 2")
	svc.buildTerminator(ctx, userID2, logger)
	all2, err := getGlobalTerminatorsForToday(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getGlobalTerminatorsForToday second call: %v", err)
	}
	if len(all2) != TerminatorsPerDay {
		t.Errorf("expected %d records after second user call, got %d", TerminatorsPerDay, len(all2))
	}
}

func TestBuildTerminator_AllHeadlinesDiffer(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "term3@test.com", "Term User 3")

	ctx := context.Background()
	logger := logging.LoggerWithContext(ctx)

	svc.buildTerminator(ctx, userID, logger)

	today := time.Now().UTC().YearDay()
	all, err := getGlobalTerminatorsForToday(ctx, sqlStorage, today)
	if err != nil {
		t.Fatalf("getGlobalTerminatorsForToday: %v", err)
	}

	seen := make(map[string]bool)
	for _, n := range all {
		if seen[n.Headline] {
			t.Errorf("duplicate headline across today's global terminators: %q", n.Headline)
		}
		seen[n.Headline] = true
	}
}

// --- semaphore concurrency bound ---.

// TestFetchNudgeImagery_ConcurrencyBound spawns 32 concurrent fetchNudgeImagery
// calls against a mock provider that holds each call for 10 ms, then asserts
// that no more than nudgeImageryCap calls were ever in flight simultaneously.
func TestFetchNudgeImagery_ConcurrencyBound(t *testing.T) {
	const totalCalls = 32

	var inflight atomic.Int32
	var maxSeen atomic.Int32

	mock := &concurrencyCountingProvider{inflight: &inflight, maxSeen: &maxSeen}
	svc := &Service{stockImageryProvider: mock}

	var wg sync.WaitGroup
	for i := 0; i < totalCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.fetchNudgeImagery(context.Background(), "user-1", fmt.Sprintf("nudge-%d", i), fmt.Sprintf("query %d", i))
		}()
	}
	wg.Wait()

	if got := maxSeen.Load(); got > int32(nudgeImageryCap) {
		t.Errorf("max observed concurrency %d exceeded cap %d", got, nudgeImageryCap)
	}
}

// concurrencyCountingProvider is a StockImageryProvider that tracks peak in-flight
// concurrency and returns an error so fetchNudgeImagery exits without hitting storage.
type concurrencyCountingProvider struct {
	inflight *atomic.Int32
	maxSeen  *atomic.Int32
}

func (p *concurrencyCountingProvider) GetStockImage(_ context.Context, _ string, _ *mediapkg.StockImageOptions) (*models.StockImage, error) {
	cur := p.inflight.Add(1)
	defer p.inflight.Add(-1)

	// Atomically update maxSeen to the highest observed value.
	for {
		old := p.maxSeen.Load()
		if cur <= old || p.maxSeen.CompareAndSwap(old, cur) {
			break
		}
	}

	// Hold briefly so concurrent goroutines can pile up and be measured.
	time.Sleep(10 * time.Millisecond) //nolint:forbidigo // fake provider intentionally stalls to allow concurrent goroutines to accumulate
	return nil, fmt.Errorf("mock: no image")
}

func (p *concurrencyCountingProvider) SearchStockImageCandidates(_ context.Context, _ string, _ int) ([]mediapkg.StockImageCandidate, error) {
	return nil, nil
}

func (p *concurrencyCountingProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (p *concurrencyCountingProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return nil, nil
}

// --- pickTerminator ---.

// TestPickTerminator_EmptyFallback verifies that an empty candidates slice causes
// pickTerminator to return a synthetic record derived from terminatorFallback.
func TestPickTerminator_EmptyFallback(t *testing.T) {
	result := pickTerminator(nil)
	if result == nil {
		t.Fatal("expected non-nil result for empty candidates")
	}
	if result.Headline == "" {
		t.Error("expected non-empty Headline on synthetic fallback record")
	}
	if !result.IsTerminator {
		t.Error("expected IsTerminator=true on fallback record")
	}
}

// TestPickTerminator_PrefersWithMedia verifies that when some candidates have a
// media_id and others do not, pickTerminator only selects from the ones with media.
func TestPickTerminator_PrefersWithMedia(t *testing.T) {
	mediaID := "media-term"
	withMedia := &models.StoredNudge{Id: "has-media", MediaId: &mediaID, IsTerminator: true}
	noMedia1 := &models.StoredNudge{Id: "no-media-1", IsTerminator: true}
	noMedia2 := &models.StoredNudge{Id: "no-media-2", IsTerminator: true}

	candidates := []*models.StoredNudge{noMedia1, withMedia, noMedia2}

	// Run enough iterations to confirm the selection always comes from the
	// with-media pool (rand.Intn selects from filtered slice).
	for i := 0; i < 20; i++ {
		result := pickTerminator(candidates)
		if result.Id != "has-media" {
			t.Errorf("iteration %d: expected has-media, got %q", i, result.Id)
		}
	}
}

// TestPickTerminator_FallsBackToFullPool verifies that when no candidates have
// media, pickTerminator falls back to selecting from the full pool.
func TestPickTerminator_FallsBackToFullPool(t *testing.T) {
	noMedia1 := &models.StoredNudge{Id: "no-media-a", IsTerminator: true}
	noMedia2 := &models.StoredNudge{Id: "no-media-b", IsTerminator: true}

	candidates := []*models.StoredNudge{noMedia1, noMedia2}

	// Any result must come from the full pool.
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		result := pickTerminator(candidates)
		if result == nil {
			t.Fatalf("iteration %d: got nil result", i)
		}
		seen[result.Id] = true
	}
	for id := range seen {
		if id != "no-media-a" && id != "no-media-b" {
			t.Errorf("unexpected ID %q selected from full pool", id)
		}
	}
}

// --- feed nudge exclusion ---.

// TestAppendTerminator_ExcludesPooledNudges verifies that a stored nudge never
// reaches the feed, stale or otherwise. Before #2936 this guarded the 24h
// freshness filter on the interleave path; now the exclusion is total, and the
// stale row makes the assertion concrete rather than vacuous.
func TestAppendTerminator_ExcludesPooledNudges(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	svc := setupTestService(sqlStorage)
	ctx := context.Background()

	userID := setupTestUser(t, sqlStorage, "expire-nudge@test.com", "Expire User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Expire Community")

	// Insert a nudge that is 25h old (beyond the 24h expiry window).
	mediaID := "media-old"
	staleNudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		NudgeVariant:     1,
		Headline:         "Stale nudge",
		Description:      "desc",
		CtaLabel:         "Go",
		CtaAction:        "plan_experience",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix() - 25*3600,
		IsTerminator:     false,
	}
	if err := insertNudge(ctx, sqlStorage, staleNudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	logger := logging.LoggerWithContext(ctx)
	result := svc.appendTerminator(ctx, userID, nil, logger)

	// The pooled nudge payload (non-terminator) must not appear in the result.
	for _, item := range result {
		if n := item.GetNudge(); n != nil && n.NudgeId == staleNudge.Id {
			t.Errorf("stale nudge %q appeared in feed after 25h — expected expiry filtering", staleNudge.Id)
		}
	}
}
