package services

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage creates a PostgreSQL database for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

// createTestUser inserts a test user and returns the assigned ID.
func createTestUser(t *testing.T, s *storage.ProtoSQLStorage, name, email string) string {
	t.Helper()
	user := &models.User{
		Name:  name,
		Email: email,
		Role:  models.Role_ROLE_USER,
	}
	id, err := s.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user %q: %v", name, err)
	}
	return id
}

// TestToAPIUser_OmitsSensitiveFields guards the denormalized user loader
// (FetchAPIUser / FetchAPIUsersBatch) against re-introducing the email /
// location disclosure closed in #2138. api.User structurally carries no email
// or location fields, so the loader cannot leak them even though models.User
// does; ToAPIUser must only copy id, name, and the primary avatar.
func TestToAPIUser_OmitsSensitiveFields(t *testing.T) {
	user := &models.User{
		Id:                         "user-123",
		Name:                       "Test User",
		Email:                      "secret@example.com",
		PrimaryResidenceLocationId: "location-home",
		OtherLocationIds:           []string{"location-cabin"},
		MediaIds:                   []string{"media-avatar"},
		Description:                "bio text",
	}

	got := ToAPIUser(user)

	if got.Id != "user-123" {
		t.Errorf("Id = %q, want %q", got.Id, "user-123")
	}
	if got.Name != "Test User" {
		t.Errorf("Name = %q, want %q", got.Name, "Test User")
	}
	if got.MediaId != "media-avatar" {
		t.Errorf("MediaId = %q, want %q", got.MediaId, "media-avatar")
	}

	// Structural guard: if a future proto change adds an email or location
	// field to api.User, this fails and forces a review of whether ToAPIUser
	// would start leaking it through the denormalized loader.
	fields := got.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		switch name := string(fields.Get(i).Name()); name {
		case "email", "primary_residence_location_id", "other_location_ids":
			t.Errorf("api.User unexpectedly carries sensitive field %q; "+
				"ToAPIUser must not be able to leak it (#2138)", name)
		}
	}
}

func TestFetchAPIUsersBatch(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()

	t.Run("empty input returns empty map", func(t *testing.T) {
		result, err := FetchAPIUsersBatch(ctx, s, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty map, got %d entries", len(result))
		}
	})

	t.Run("all empty strings returns empty map", func(t *testing.T) {
		result, err := FetchAPIUsersBatch(ctx, s, []string{"", ""})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty map, got %d entries", len(result))
		}
	})

	t.Run("fetches single user", func(t *testing.T) {
		id := createTestUser(t, s, "Alice", "alice@test.com")

		result, err := FetchAPIUsersBatch(ctx, s, []string{id})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 user, got %d", len(result))
		}
		if result[id].Name != "Alice" {
			t.Errorf("expected name Alice, got %q", result[id].Name)
		}
	})

	t.Run("fetches multiple users", func(t *testing.T) {
		id1 := createTestUser(t, s, "Bob", "bob@test.com")
		id2 := createTestUser(t, s, "Carol", "carol@test.com")
		id3 := createTestUser(t, s, "Dave", "dave@test.com")

		result, err := FetchAPIUsersBatch(ctx, s, []string{id1, id2, id3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 users, got %d", len(result))
		}
		if result[id1].Name != "Bob" {
			t.Errorf("expected Bob, got %q", result[id1].Name)
		}
		if result[id2].Name != "Carol" {
			t.Errorf("expected Carol, got %q", result[id2].Name)
		}
		if result[id3].Name != "Dave" {
			t.Errorf("expected Dave, got %q", result[id3].Name)
		}
	})

	t.Run("deduplicates IDs", func(t *testing.T) {
		id := createTestUser(t, s, "Eve", "eve@test.com")

		result, err := FetchAPIUsersBatch(ctx, s, []string{id, id, id})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 user (deduplicated), got %d", len(result))
		}
		if result[id].Name != "Eve" {
			t.Errorf("expected Eve, got %q", result[id].Name)
		}
	})

	t.Run("skips missing users", func(t *testing.T) {
		id := createTestUser(t, s, "Frank", "frank@test.com")

		result, err := FetchAPIUsersBatch(ctx, s, []string{id, "nonexistent-id"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 user, got %d", len(result))
		}
		if result[id].Name != "Frank" {
			t.Errorf("expected Frank, got %q", result[id].Name)
		}
		if _, ok := result["nonexistent-id"]; ok {
			t.Error("expected nonexistent-id to be absent from result")
		}
	})

	t.Run("filters out empty strings among valid IDs", func(t *testing.T) {
		id := createTestUser(t, s, "Grace", "grace@test.com")

		result, err := FetchAPIUsersBatch(ctx, s, []string{"", id, ""})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 user, got %d", len(result))
		}
		if result[id].Name != "Grace" {
			t.Errorf("expected Grace, got %q", result[id].Name)
		}
	})

	t.Run("converts to api.User with correct fields", func(t *testing.T) {
		user := &models.User{
			Name:     "Heidi",
			Email:    "heidi@test.com",
			Role:     models.Role_ROLE_USER,
			MediaIds: []string{"media-123"},
		}
		id, err := s.Insert(ctx, user)
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		result, err := FetchAPIUsersBatch(ctx, s, []string{id})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		apiUser := result[id]
		if apiUser.Id != id {
			t.Errorf("expected ID %q, got %q", id, apiUser.Id)
		}
		if apiUser.Name != "Heidi" {
			t.Errorf("expected name Heidi, got %q", apiUser.Name)
		}
		if apiUser.MediaId != "media-123" {
			t.Errorf("expected media ID media-123, got %q", apiUser.MediaId)
		}
	})
}

// TestFetchInvitedIndividuals covers the shared "Shared with" / "Who's In"
// roster core (#2492): owner + exclude-set filtering, deterministic sort, and
// the MaxInvitedIndividuals cap that the gear/request/experience builders rely
// on.
func TestFetchInvitedIndividuals(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()

	t.Run("excludes owner and the exclude set, sorted by id", func(t *testing.T) {
		owner := createTestUser(t, s, "Owner", "o@test.com")
		a := createTestUser(t, s, "A", "a2@test.com")
		b := createTestUser(t, s, "B", "b2@test.com")
		c := createTestUser(t, s, "C", "c2@test.com")
		// owner is dropped; c is in the exclude set (e.g. already responded).
		members := []string{a, owner, b, c}
		exclude := map[string]struct{}{c: {}}

		got, err := FetchInvitedIndividuals(ctx, s, members, owner, exclude)
		if err != nil {
			t.Fatalf("FetchInvitedIndividuals: %v", err)
		}
		ids := make([]string, len(got))
		for i, u := range got {
			ids[i] = u.Id
		}
		want := []string{a, b}
		sort.Strings(want)
		if !reflect.DeepEqual(ids, want) {
			t.Errorf("ids = %v, want %v (owner + excluded dropped, sorted)", ids, want)
		}
	})

	t.Run("nil exclude keeps everyone but the owner", func(t *testing.T) {
		owner := createTestUser(t, s, "Owner2", "o2@test.com")
		a := createTestUser(t, s, "AA", "aa@test.com")
		got, err := FetchInvitedIndividuals(ctx, s, []string{owner, a}, owner, nil)
		if err != nil {
			t.Fatalf("FetchInvitedIndividuals: %v", err)
		}
		if len(got) != 1 || got[0].Id != a {
			t.Errorf("got %v, want exactly [%s]", got, a)
		}
	})

	t.Run("caps at MaxInvitedIndividuals, returning the lowest-sorted ids", func(t *testing.T) {
		ids := make([]string, 0, MaxInvitedIndividuals+5)
		for i := 0; i < MaxInvitedIndividuals+5; i++ {
			ids = append(ids, createTestUser(t, s, fmt.Sprintf("Cap%d", i), fmt.Sprintf("cap%d@test.com", i)))
		}
		got, err := FetchInvitedIndividuals(ctx, s, ids, "", nil)
		if err != nil {
			t.Fatalf("FetchInvitedIndividuals: %v", err)
		}
		if len(got) != MaxInvitedIndividuals {
			t.Fatalf("len = %d, want cap %d", len(got), MaxInvitedIndividuals)
		}
		sorted := append([]string(nil), ids...)
		sort.Strings(sorted)
		for i, u := range got {
			if u.Id != sorted[i] {
				t.Errorf("at %d: id = %s, want %s (lowest-sorted survive the cap)", i, u.Id, sorted[i])
			}
		}
	})

	t.Run("returns nil when everyone is excluded", func(t *testing.T) {
		owner := createTestUser(t, s, "Solo", "solo@test.com")
		got, err := FetchInvitedIndividuals(ctx, s, []string{owner}, owner, nil)
		if err != nil {
			t.Fatalf("FetchInvitedIndividuals: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestFetchAPIUsers_UsesBatch(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()

	id1 := createTestUser(t, s, "Batch1", "batch1@test.com")
	id2 := createTestUser(t, s, "Batch2", "batch2@test.com")

	t.Run("preserves order", func(t *testing.T) {
		result, err := FetchAPIUsers(ctx, s, []string{id2, id1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 users, got %d", len(result))
		}
		if result[0].Name != "Batch2" {
			t.Errorf("expected first user Batch2, got %q", result[0].Name)
		}
		if result[1].Name != "Batch1" {
			t.Errorf("expected second user Batch1, got %q", result[1].Name)
		}
	})

	t.Run("preserves duplicates", func(t *testing.T) {
		result, err := FetchAPIUsers(ctx, s, []string{id1, id1, id2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 users (with duplicate), got %d", len(result))
		}
		if result[0].Id != id1 || result[1].Id != id1 || result[2].Id != id2 {
			t.Errorf("unexpected order: %v, %v, %v", result[0].Id, result[1].Id, result[2].Id)
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		result, err := FetchAPIUsers(ctx, s, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil, got %v", result)
		}
	})
}

func TestFormerMemberPlaceholder(t *testing.T) {
	got := FormerMemberPlaceholder("user-123")
	if got == nil {
		t.Fatal("expected non-nil placeholder")
	}
	if got.Id != "user-123" {
		t.Errorf("Id = %q, want %q", got.Id, "user-123")
	}
	if !got.FormerMember {
		t.Error("FormerMember = false, want true")
	}
	if got.Name != "" {
		t.Errorf("Name = %q, want empty (PII must not be set on placeholder)", got.Name)
	}
	if got.MediaId != "" {
		t.Errorf("MediaId = %q, want empty (PII must not be set on placeholder)", got.MediaId)
	}
}

func TestResolveUserOrFormer(t *testing.T) {
	alice := &api.User{Id: "alice-id", Name: "Alice"}
	userMap := map[string]*api.User{"alice-id": alice}

	t.Run("empty id returns nil", func(t *testing.T) {
		if got := ResolveUserOrFormer(userMap, ""); got != nil {
			t.Errorf("expected nil for empty id, got %+v", got)
		}
	})

	t.Run("present id returns mapped user", func(t *testing.T) {
		got := ResolveUserOrFormer(userMap, "alice-id")
		if got != alice {
			t.Errorf("expected mapped user, got %+v", got)
		}
	})

	t.Run("missing id returns former-member placeholder", func(t *testing.T) {
		got := ResolveUserOrFormer(userMap, "ghost-id")
		if got == nil {
			t.Fatal("expected placeholder, got nil")
		}
		if got.Id != "ghost-id" {
			t.Errorf("Id = %q, want %q", got.Id, "ghost-id")
		}
		if !got.FormerMember {
			t.Error("FormerMember = false, want true")
		}
	})

	t.Run("nil-valued map entry treated as missing", func(t *testing.T) {
		mp := map[string]*api.User{"orphan-id": nil}
		got := ResolveUserOrFormer(mp, "orphan-id")
		if got == nil || !got.FormerMember || got.Id != "orphan-id" {
			t.Errorf("expected placeholder with id=orphan-id, got %+v", got)
		}
	})
}
