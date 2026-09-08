package esm

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestAttendeeUserIDs_FiltersByAttendedYes(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	experienceID := "exp-1"
	rsvps := []*models.ExperienceRSVP{
		{ExperienceId: experienceID, UserId: "u-yes-1", Attended: models.AttendedStatus_ATTENDED_STATUS_YES, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: experienceID, UserId: "u-yes-2", Attended: models.AttendedStatus_ATTENDED_STATUS_YES, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: experienceID, UserId: "u-no", Attended: models.AttendedStatus_ATTENDED_STATUS_NO, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: experienceID, UserId: "u-unknown", Attended: models.AttendedStatus_ATTENDED_STATUS_UNKNOWN, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: "different-exp", UserId: "other", Attended: models.AttendedStatus_ATTENDED_STATUS_YES, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
	}
	for _, r := range rsvps {
		if _, err := store.Insert(ctx, r); err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}
	}

	got, err := AttendeeUserIDs(ctx, store, experienceID)
	if err != nil {
		t.Fatalf("AttendeeUserIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 attendees, got %d (%v)", len(got), got)
	}
	want := map[string]bool{"u-yes-1": true, "u-yes-2": true}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected attendee id: %s", id)
		}
	}
}

func TestAttendeeUserIDs_SkipsProvisionalUsers(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	experienceID := "exp-1"
	provID := "prov-123"
	rsvps := []*models.ExperienceRSVP{
		{ExperienceId: experienceID, UserId: "u-yes", Attended: models.AttendedStatus_ATTENDED_STATUS_YES, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: experienceID, UserId: "", ProvisionalUserId: &provID, Attended: models.AttendedStatus_ATTENDED_STATUS_YES, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
	}
	for _, r := range rsvps {
		if _, err := store.Insert(ctx, r); err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}
	}

	got, err := AttendeeUserIDs(ctx, store, experienceID)
	if err != nil {
		t.Fatalf("AttendeeUserIDs: %v", err)
	}
	if len(got) != 1 || got[0] != "u-yes" {
		t.Errorf("expected only the real user, got %v", got)
	}
}

func TestBuildPromptForExperience_Roundtrips(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	now := time.Now()
	completedAt := now.Add(-1 * time.Hour).Unix()
	experience := &models.Experience{
		Name:               "Sunday brunch",
		OwnerId:            "host",
		CompletedAtUnixSec: &completedAt,
	}
	expID, err := store.Insert(ctx, experience)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	draft := PromptDraft{
		Question: "Worth doing again?",
		Options: []*models.ESMResponseOption{
			{Key: "do_again", Label: "Yes"},
			{Key: "skip", Label: "Not really"},
		},
		Recipients:      []string{"u1", "u2", "u3"},
		CreatedByUserID: "operator",
	}

	prompt, responses, err := BuildPromptForExperience(ctx, store, expID, draft, now)
	if err != nil {
		t.Fatalf("BuildPromptForExperience: %v", err)
	}
	if prompt.Id == "" {
		t.Error("prompt id should be assigned")
	}
	if prompt.ClosesAtUnixSec != completedAt+DefaultPromptWindowSeconds {
		t.Errorf("closes_at default: want %d, got %d", completedAt+DefaultPromptWindowSeconds, prompt.ClosesAtUnixSec)
	}
	if len(responses) != 3 {
		t.Fatalf("want 3 response rows, got %d", len(responses))
	}
	for _, r := range responses {
		if r.PromptId != prompt.Id {
			t.Errorf("response row prompt_id mismatch: %s", r.PromptId)
		}
		if r.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED {
			t.Errorf("response row should start unresolved, got %v", r.ConsumedAction)
		}
	}

	// Persist as the recap-story materialization does — one prompt insert
	// plus batched response rows — and verify the rows are reachable.
	if _, err := store.Insert(ctx, prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	msgs := make([]proto.Message, len(responses))
	for i, r := range responses {
		msgs[i] = r
	}
	if _, err := store.InsertBatch(ctx, msgs); err != nil {
		t.Fatalf("insert response rows: %v", err)
	}

	row, err := GetResponseForUser(ctx, store, "u2", prompt.Id)
	if err != nil {
		t.Fatalf("GetResponseForUser: %v", err)
	}
	if row == nil {
		t.Fatalf("expected response row for u2, got nil")
	}
	if row.PromptId != prompt.Id {
		t.Errorf("expected prompt %s, got %s", prompt.Id, row.PromptId)
	}
}

func TestBuildPromptForExperience_RejectsInvalidDrafts(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	expID, err := store.Insert(ctx, &models.Experience{Name: "Test", OwnerId: "host"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	now := time.Now()

	cases := []struct {
		name  string
		draft PromptDraft
	}{
		{"empty question", PromptDraft{
			Question:   "",
			Options:    []*models.ESMResponseOption{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}},
			Recipients: []string{"u1"},
		}},
		{"too few options", PromptDraft{
			Question:   "Q?",
			Options:    []*models.ESMResponseOption{{Key: "a", Label: "A"}},
			Recipients: []string{"u1"},
		}},
		{"too many options", PromptDraft{
			Question: "Q?",
			Options: []*models.ESMResponseOption{
				{Key: "a", Label: "A"},
				{Key: "b", Label: "B"},
				{Key: "c", Label: "C"},
				{Key: "d", Label: "D"},
				{Key: "e", Label: "E"},
			},
			Recipients: []string{"u1"},
		}},
		{"empty recipients", PromptDraft{
			Question:   "Q?",
			Options:    []*models.ESMResponseOption{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}},
			Recipients: []string{},
		}},
		{"option missing key", PromptDraft{
			Question:   "Q?",
			Options:    []*models.ESMResponseOption{{Key: "", Label: "A"}, {Key: "b", Label: "B"}},
			Recipients: []string{"u1"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := BuildPromptForExperience(ctx, store, expID, tc.draft, now)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}
