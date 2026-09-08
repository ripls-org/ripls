package esm

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	esmlib "go.ripls.org/ripls/server/esm"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func authedCtx(userID string) context.Context {
	return authn.SetInfo(context.Background(), &auth.Info{UserID: userID, Email: "u@test.com", Role: models.Role_ROLE_USER})
}

func insertPrompt(t *testing.T, store *storage.ProtoSQLStorage, experienceID string, closesAt int64) *models.StoredESMPrompt {
	t.Helper()
	prompt := &models.StoredESMPrompt{
		ExperienceId: experienceID,
		CommunityId:  "comm-1",
		Question:     "Worth doing again?",
		ResponseOptions: []*models.ESMResponseOption{
			{Key: "do_again", Label: "Yes"},
			{Key: "skip", Label: "Not really"},
		},
		CreatedAtUnixSec: time.Now().Unix(),
		ClosesAtUnixSec:  closesAt,
		CreatedByUserId:  "operator",
	}
	if _, err := store.Insert(context.Background(), prompt); err != nil {
		t.Fatalf("insert prompt: %v", err)
	}
	return prompt
}

func insertResponseRow(t *testing.T, store *storage.ProtoSQLStorage, promptID, experienceID, userID string, action models.ESMConsumedAction) *models.StoredESMResponse {
	t.Helper()
	row := &models.StoredESMResponse{
		PromptId:       promptID,
		ExperienceId:   experienceID,
		UserId:         userID,
		ConsumedAction: action,
	}
	if _, err := store.Insert(context.Background(), row); err != nil {
		t.Fatalf("insert response row: %v", err)
	}
	return row
}

func TestRespondToESMPrompt_RecordsResponse(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	key := "do_again"
	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
	}))
	if err != nil {
		t.Fatalf("RespondToESMPrompt: %v", err)
	}

	row, err := esmlib.GetResponseForUser(context.Background(), store, "u1", prompt.Id)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_RESPONDED {
		t.Errorf("ConsumedAction: want RESPONDED, got %v", row.ConsumedAction)
	}
	if row.ResponseOptionKey == nil || *row.ResponseOptionKey != "do_again" {
		t.Errorf("ResponseOptionKey: want do_again, got %v", row.ResponseOptionKey)
	}
}

func TestRespondToESMPrompt_RecordsDismissal(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:  prompt.Id,
		Dismissed: true,
	}))
	if err != nil {
		t.Fatalf("RespondToESMPrompt: %v", err)
	}

	row, err := esmlib.GetResponseForUser(context.Background(), store, "u1", prompt.Id)
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if row.ConsumedAction != models.ESMConsumedAction_ESM_CONSUMED_ACTION_DISMISSED {
		t.Errorf("ConsumedAction: want DISMISSED, got %v", row.ConsumedAction)
	}
}

func TestRespondToESMPrompt_RejectsNonCommunityMember(t *testing.T) {
	// Story-audience access lets any community member vote, but a caller
	// who is neither pre-targeted nor a member of the prompt's community
	// must still be rejected. With no community row in the test DB the
	// active-community gate returns NotFound; with a community present it
	// returns PermissionDenied. Either rejection class is acceptable —
	// the contract this test pins is "rejected, never persisted."
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	key := "do_again"
	_, err := svc.RespondToESMPrompt(authedCtx("not-a-recipient"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
	}))
	if err == nil {
		t.Fatal("expected rejection error for non-recipient non-member, got nil")
	}
	code := connect.CodeOf(err)
	if code != connect.CodePermissionDenied && code != connect.CodeNotFound {
		t.Errorf("error code: want PermissionDenied or NotFound, got %v", code)
	}
}

func TestRespondToESMPrompt_RejectsExpired(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(-1*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	key := "do_again"
	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
	}))
	if err == nil {
		t.Fatal("expected FailedPrecondition error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("error code: want FailedPrecondition, got %v", connect.CodeOf(err))
	}
}

func TestRespondToESMPrompt_AllowsVoteChange(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	first := "do_again"
	if _, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &first,
	})); err != nil {
		t.Fatalf("first response: %v", err)
	}

	second := "skip"
	if _, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &second,
	})); err != nil {
		t.Fatalf("vote-change response should succeed, got %v", err)
	}
}

func TestRespondToESMPrompt_RejectsDismissAfterResponding(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	key := "do_again"
	if _, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
	})); err != nil {
		t.Fatalf("first response: %v", err)
	}

	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:  prompt.Id,
		Dismissed: true,
	}))
	if err == nil {
		t.Fatal("expected FailedPrecondition for dismiss-after-vote, got nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("error code: want FailedPrecondition, got %v", connect.CodeOf(err))
	}
}

func TestRespondToESMPrompt_RejectsUnknownOptionKey(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	bogus := "not-a-key"
	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &bogus,
	}))
	if err == nil {
		t.Fatal("expected InvalidArgument error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code: want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestRespondToESMPrompt_RejectsConflictingArgs(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())
	insertResponseRow(t, store, prompt.Id, "exp-1", "u1", models.ESMConsumedAction_ESM_CONSUMED_ACTION_UNSPECIFIED)

	key := "do_again"
	_, err := svc.RespondToESMPrompt(authedCtx("u1"), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
		Dismissed:         true,
	}))
	if err == nil {
		t.Fatal("expected InvalidArgument error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code: want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestRespondToESMPrompt_RequiresAuth(t *testing.T) {
	store := setupTestStorage(t)
	svc := New(store)

	prompt := insertPrompt(t, store, "exp-1", time.Now().Add(48*time.Hour).Unix())

	key := "do_again"
	_, err := svc.RespondToESMPrompt(context.Background(), connect.NewRequest(&api.RespondToESMPromptRequest{
		PromptId:          prompt.Id,
		ResponseOptionKey: &key,
	}))
	if err == nil {
		t.Fatal("expected Unauthenticated error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("error code: want Unauthenticated, got %v", connect.CodeOf(err))
	}
}
