package momentum

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func ptrInt64(v int64) *int64 { return &v }
func ptrStr(v string) *string { return &v }

func insertExperience(t *testing.T, store *storage.ProtoSQLStorage, name, ownerID string, state models.ExperienceState, completedAt int64) string {
	t.Helper()
	ctx := context.Background()
	exp := &models.Experience{
		Name:    name,
		OwnerId: ownerID,
		State:   state,
	}
	if completedAt > 0 {
		exp.CompletedAtUnixSec = ptrInt64(completedAt)
	}
	id, err := store.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	return id
}

// --- ESMRepeatSignalDetector ---.

func TestESMRepeatSignalDetector_NoExperiencesNoOp(t *testing.T) {
	store := setupTestStorage(t)
	got, err := ESMRepeatSignalDetector{}.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestESMRepeatSignalDetector_HighRepeatFires(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-2 * 24 * time.Hour).Unix()

	expID := insertExperience(t, store, "Sunday brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, completedAt)

	// Create one prompt with 4 of 5 do_again responses.
	prompt := &models.StoredESMPrompt{
		Id:               "prompt-1",
		ExperienceId:     expID,
		Question:         "Worth doing again?",
		ResponseOptions:  []*models.ESMResponseOption{{Key: RepeatOptionKey, Label: "Yes"}, {Key: "skip", Label: "No"}},
		ClosesAtUnixSec:  now.Add(48 * time.Hour).Unix(),
		CreatedAtUnixSec: completedAt,
	}
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	yes := true
	rows := []*models.StoredESMResponse{
		{PromptId: "prompt-1", ExperienceId: expID, UserId: "a", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-1", ExperienceId: expID, UserId: "b", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-1", ExperienceId: expID, UserId: "c", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-1", ExperienceId: expID, UserId: "d", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-1", ExperienceId: expID, UserId: "e", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip"), RespondentWasAttendee: &yes},
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	det := ESMRepeatSignalDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(ctx, store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection")
	}
	if got.Slot != SlotESMRepeatSignal {
		t.Errorf("Slot: want ESMRepeatSignal, got %v", got.Slot)
	}
	if got.CtaAction != "schedule_repeat" {
		t.Errorf("CtaAction: want schedule_repeat, got %q", got.CtaAction)
	}
	if got.ContextID != expID {
		t.Errorf("ContextID: want %s, got %s", expID, got.ContextID)
	}
	if err := ValidateLeverCopy(got.CtaLabel); err != nil {
		t.Errorf("lever fails ValidateLeverCopy: %v", err)
	}
}

func TestESMRepeatSignalDetector_BelowThresholdNoOp(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-2 * 24 * time.Hour).Unix()

	expID := insertExperience(t, store, "Brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, completedAt)

	prompt := &models.StoredESMPrompt{
		Id:               "prompt-low",
		ExperienceId:     expID,
		ResponseOptions:  []*models.ESMResponseOption{{Key: RepeatOptionKey, Label: "Yes"}, {Key: "skip", Label: "No"}},
		CreatedAtUnixSec: completedAt,
		ClosesAtUnixSec:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	// 1 of 5 repeats — well below 60%.
	rows := []*models.StoredESMResponse{
		{PromptId: "prompt-low", ExperienceId: expID, UserId: "a", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey)},
		{PromptId: "prompt-low", ExperienceId: expID, UserId: "b", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip")},
		{PromptId: "prompt-low", ExperienceId: expID, UserId: "c", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip")},
		{PromptId: "prompt-low", ExperienceId: expID, UserId: "d", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip")},
		{PromptId: "prompt-low", ExperienceId: expID, UserId: "e", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip")},
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	det := ESMRepeatSignalDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(ctx, store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (below threshold), got %v", got)
	}
}

func TestESMRepeatSignalDetector_TwoYesesFireWithNames(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-2 * 24 * time.Hour).Unix()

	expID := insertExperience(t, store, "Sunday brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, completedAt)

	if _, err := store.Insert(ctx, &models.User{Id: "alice-id", Name: "Alice Smith"}); err != nil {
		t.Fatalf("insert user alice: %v", err)
	}
	if _, err := store.Insert(ctx, &models.User{Id: "bob-id", Name: "Bob Jones"}); err != nil {
		t.Fatalf("insert user bob: %v", err)
	}

	prompt := &models.StoredESMPrompt{
		Id:               "prompt-2",
		ExperienceId:     expID,
		ResponseOptions:  []*models.ESMResponseOption{{Key: RepeatOptionKey, Label: "Yes"}, {Key: "skip", Label: "No"}},
		CreatedAtUnixSec: completedAt,
		ClosesAtUnixSec:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	yes := true
	rows := []*models.StoredESMResponse{
		{PromptId: "prompt-2", ExperienceId: expID, UserId: "alice-id", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-2", ExperienceId: expID, UserId: "bob-id", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	det := ESMRepeatSignalDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(ctx, store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection on 2-of-2 yeses")
	}
	if got.AtmosphereLine != "Alice and Bob want another" {
		t.Errorf("AtmosphereLine: want %q, got %q",
			"Alice and Bob want another", got.AtmosphereLine)
	}
}

func TestESMRepeatSignalDetector_OneYesOneSkipNoOp(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-2 * 24 * time.Hour).Unix()

	expID := insertExperience(t, store, "Brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, completedAt)

	prompt := &models.StoredESMPrompt{
		Id:               "prompt-half",
		ExperienceId:     expID,
		ResponseOptions:  []*models.ESMResponseOption{{Key: RepeatOptionKey, Label: "Yes"}, {Key: "skip", Label: "No"}},
		CreatedAtUnixSec: completedAt,
		ClosesAtUnixSec:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	// 1 of 2 → 50% → still suppressed by the 60% ratio gate.
	yes := true
	rows := []*models.StoredESMResponse{
		{PromptId: "prompt-half", ExperienceId: expID, UserId: "a", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr(RepeatOptionKey), RespondentWasAttendee: &yes},
		{PromptId: "prompt-half", ExperienceId: expID, UserId: "b", ConsumedAction: models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED, ResponseOptionKey: ptrStr("skip"), RespondentWasAttendee: &yes},
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	det := ESMRepeatSignalDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(ctx, store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (1 of 2 below 60%% ratio), got %v", got)
	}
}

func TestESMRepeatSignalDetector_FourYesesFormatsWithOthers(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	completedAt := now.Add(-2 * 24 * time.Hour).Unix()

	expID := insertExperience(t, store, "Sunday brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, completedAt)

	for _, u := range []*models.User{
		{Id: "alice-id", Name: "Alice"},
		{Id: "bob-id", Name: "Bob"},
		{Id: "carol-id", Name: "Carol"},
		{Id: "dave-id", Name: "Dave"},
	} {
		if _, err := store.Insert(ctx, u); err != nil {
			t.Fatalf("insert user %s: %v", u.Id, err)
		}
	}

	prompt := &models.StoredESMPrompt{
		Id:               "prompt-many",
		ExperienceId:     expID,
		ResponseOptions:  []*models.ESMResponseOption{{Key: RepeatOptionKey, Label: "Yes"}},
		CreatedAtUnixSec: completedAt,
		ClosesAtUnixSec:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	yes := true
	respUserIDs := []string{"alice-id", "bob-id", "carol-id", "dave-id"}
	rows := make([]*models.StoredESMResponse, 0, len(respUserIDs))
	for _, uid := range respUserIDs {
		rows = append(rows, &models.StoredESMResponse{
			PromptId:              "prompt-many",
			ExperienceId:          expID,
			UserId:                uid,
			ConsumedAction:        models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED,
			ResponseOptionKey:     ptrStr(RepeatOptionKey),
			RespondentWasAttendee: &yes,
		})
	}
	msgs := make([]proto.Message, len(rows))
	for i, r := range rows {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	det := ESMRepeatSignalDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(ctx, store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection on 4-of-4 yeses")
	}
	// Format: every yes-voter's first name is listed in iteration
	// order, joined by Oxford-comma punctuation. Truncation happens
	// client-side, never in the wire payload — the host needs to know
	// who is asking.
	if got.AtmosphereLine != "Alice, Bob, Carol, and Dave want another" {
		t.Errorf("AtmosphereLine: want %q, got %q",
			"Alice, Bob, Carol, and Dave want another",
			got.AtmosphereLine)
	}
}

// --- CalendarGapDetector ---.

func TestCalendarGapDetector_RhythmWithGapFires(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// 4 instances of "Sunday brunch", most recent 30 days ago — within
	// 90-day rhythm window, 30 days >= 14-day gap requirement.
	for i := 0; i < 4; i++ {
		ts := now.AddDate(0, 0, -30-7*i).Unix()
		insertExperience(t, store, "Sunday brunch", "u1",
			models.ExperienceState_EXPERIENCE_STATE_COMPLETED, ts)
	}

	det := CalendarGapDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection")
	}
	if got.Slot != SlotCalendarGap {
		t.Errorf("Slot: want CalendarGap, got %v", got.Slot)
	}
	if got.CtaAction != "revive_experience" {
		t.Errorf("CtaAction: want revive_experience, got %q", got.CtaAction)
	}
	if err := ValidateLeverCopy(got.CtaLabel); err != nil {
		t.Errorf("lever fails ValidateLeverCopy: %v", err)
	}
}

func TestCalendarGapDetector_RecentInstanceSuppresses(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// 4 instances, most recent 3 days ago — not a gap yet.
	for i := 0; i < 4; i++ {
		ts := now.AddDate(0, 0, -3-7*i).Unix()
		insertExperience(t, store, "Sunday brunch", "u1",
			models.ExperienceState_EXPERIENCE_STATE_COMPLETED, ts)
	}

	det := CalendarGapDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (no gap yet), got %v", got)
	}
}

func TestCalendarGapDetector_TooFewInstancesNoOp(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// Only 2 instances — below CalendarGapMinInstances (3).
	insertExperience(t, store, "Brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, now.AddDate(0, 0, -30).Unix())
	insertExperience(t, store, "Brunch", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED, now.AddDate(0, 0, -60).Unix())

	det := CalendarGapDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (no rhythm), got %v", got)
	}
}

// --- SeasonalTriggerDetector ---.

func TestSeasonalTriggerDetector_AnniversaryFires(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	// One year ago, ±5 days — well inside the 30-day anniversary window.
	insertExperience(t, store, "Bike tune-up day", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		now.AddDate(-1, 0, 5).Unix())

	det := SeasonalTriggerDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection")
	}
	if got.Slot != SlotSeasonalTrigger {
		t.Errorf("Slot: want SeasonalTrigger, got %v", got.Slot)
	}
	if got.CtaAction != "revive_experience" {
		t.Errorf("CtaAction: want revive_experience, got %q", got.CtaAction)
	}
	if err := ValidateLeverCopy(got.CtaLabel); err != nil {
		t.Errorf("lever fails ValidateLeverCopy: %v", err)
	}
}

func TestSeasonalTriggerDetector_RecentRevivalSuppresses(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	// One year ago.
	insertExperience(t, store, "Bike tune-up day", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		now.AddDate(-1, 0, 5).Unix())
	// And again last week — already revived.
	insertExperience(t, store, "Bike tune-up day", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		now.AddDate(0, 0, -7).Unix())

	det := SeasonalTriggerDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (already revived), got %v", got)
	}
}

func TestSeasonalTriggerDetector_OutsideWindowNoOp(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	// Two years ago — outside the anniversary window.
	insertExperience(t, store, "Bike tune-up", "u1",
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		now.AddDate(-2, 0, 0).Unix())

	det := SeasonalTriggerDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (outside window), got %v", got)
	}
}

// --- ActiveQuestDetector ---.

func TestActiveQuestDetector_ActiveWithPriorRoundFires(t *testing.T) {
	store := setupTestStorage(t)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	// 2 prior completed instances of "Sunday smoker".
	for i := 0; i < 2; i++ {
		insertExperience(t, store, "Sunday smoker", "u1",
			models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
			now.AddDate(0, 0, -7-(i*7)).Unix())
	}
	// And one currently active.
	activeID := insertExperience(t, store, "Sunday smoker", "u1",
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE, 0)

	det := ActiveQuestDetector{Now: func() time.Time { return now }}
	got, err := det.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got == nil {
		t.Fatal("expected detection")
	}
	if got.Slot != SlotActiveQuest {
		t.Errorf("Slot: want ActiveQuest, got %v", got.Slot)
	}
	if got.ContextID != activeID {
		t.Errorf("ContextID: want %s, got %s", activeID, got.ContextID)
	}
	if got.CtaAction != "schedule_repeat" {
		t.Errorf("CtaAction: want schedule_repeat, got %q", got.CtaAction)
	}
	if err := ValidateLeverCopy(got.CtaLabel); err != nil {
		t.Errorf("lever fails ValidateLeverCopy: %v", err)
	}
}

func TestActiveQuestDetector_NoPriorInstanceNoOp(t *testing.T) {
	store := setupTestStorage(t)

	insertExperience(t, store, "Brand new thing", "u1",
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE, 0)

	got, err := ActiveQuestDetector{}.Detect(context.Background(), store, "u1", "c1")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (no prior round), got %v", got)
	}
}

// --- DefaultDetectors order check ---.

func TestDefaultDetectors_AllImplementedNotStubbed(t *testing.T) {
	got := DefaultDetectors()
	// Four of the five cascade slots are filled; SlotIdleOffer is
	// deliberately unfilled (#2892).
	if len(got) != 4 {
		t.Fatalf("DefaultDetectors length: want 4, got %d", len(got))
	}
	for _, d := range got {
		// Sanity: no detector should be the stubDetector type.
		if _, isStub := d.(stubDetector); isStub {
			t.Errorf("detector for %v is still a stub", d.Slot())
		}
	}
}
