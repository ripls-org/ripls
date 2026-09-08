package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text untouched", "drill", "drill"},
		{"spaces untouched", "power drill", "power drill"},
		{"percent escaped", "50%", `50\%`},
		{"underscore escaped", "a_c", `a\_c`},
		{"backslash escaped", `a\b`, `a\\b`},
		{"only wildcards", "%_%", `\%\_\%`},
		{"trailing backslash does not dangle", `abc\`, `abc\\`},
		{"empty", "", ""},
		{"unicode passes through", "café", "café"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLikePattern(tt.input); got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLikeContains(t *testing.T) {
	if got, want := likeContains("drill"), "%drill%"; got != want {
		t.Errorf("likeContains(%q) = %q, want %q", "drill", got, want)
	}
	if got, want := likeContains("50%"), `%50\%%`; got != want {
		t.Errorf("likeContains(%q) = %q, want %q", "50%", got, want)
	}
}

// TestSearchCommunityUsersByName_WildcardsAreLiteral is the behaviour change in
// #2795: a member who typed `%` used to match every user in the community
// because the term was interpolated into the pattern unescaped. The term was
// always *bound*, so this was never injection — it was a correctness and
// unbounded-scan bug. After escaping, `%` matches only names containing a
// literal percent sign.
func TestSearchCommunityUsersByName_WildcardsAreLiteral(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	owner := &models.User{Name: "Owner", Email: "owner@example.com"}
	ownerID, err := storage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Insert owner error = %v", err)
	}
	community := &models.Community{Name: "Test Community", CreatorId: ownerID, OwnerUserId: ownerID}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Insert community error = %v", err)
	}

	names := []string{"Alice", "Bob", "50% Off Dave", "a_c Underscore"}
	for _, name := range names {
		user := &models.User{Name: name, Email: name + "@example.com"}
		userID, err := storage.Insert(ctx, user)
		if err != nil {
			t.Fatalf("Insert user %q error = %v", name, err)
		}
		cu := &models.CommunityUser{
			CommunityId:      communityID,
			UserId:           userID,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		if _, err := storage.Insert(ctx, cu); err != nil {
			t.Fatalf("Insert community user error = %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		wantNames []string
	}{
		{
			name:      "percent matches only a literal percent",
			query:     "%",
			wantNames: []string{"50% Off Dave"},
		},
		{
			name:      "underscore matches only a literal underscore",
			query:     "_",
			wantNames: []string{"a_c Underscore"},
		},
		{
			name:      "ordinary substring search still works",
			query:     "li",
			wantNames: []string{"Alice"},
		},
		{
			name:      "case-insensitive matching preserved",
			query:     "BOB",
			wantNames: []string{"Bob"},
		},
		{
			name:      "a lone backslash matches nothing rather than erroring",
			query:     `\`,
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users, err := storage.SearchCommunityUsersByName(ctx, communityID, tt.query, 10)
			if err != nil {
				t.Fatalf("SearchCommunityUsersByName(%q) error = %v", tt.query, err)
			}
			got := make([]string, len(users))
			for i, u := range users {
				got[i] = u.Name
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("SearchCommunityUsersByName(%q) = %v, want %v", tt.query, got, tt.wantNames)
			}
			for _, want := range tt.wantNames {
				found := false
				for _, g := range got {
					if g == want {
						found = true
					}
				}
				if !found {
					t.Errorf("SearchCommunityUsersByName(%q) = %v, missing %q", tt.query, got, want)
				}
			}
		})
	}
}

// TestSearchCommunityUsersByName_LimitIsBound covers the LIMIT placeholder: the
// bound value must still cap the result set.
func TestSearchCommunityUsersByName_LimitIsBound(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	owner := &models.User{Name: "Limit Owner", Email: "limitowner@example.com"}
	ownerID, err := storage.Insert(ctx, owner)
	if err != nil {
		t.Fatalf("Insert owner error = %v", err)
	}
	community := &models.Community{Name: "Limit Community", CreatorId: ownerID, OwnerUserId: ownerID}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Insert community error = %v", err)
	}

	for i := 0; i < 5; i++ {
		user := &models.User{
			Name:  "Common Name " + string(rune('A'+i)),
			Email: string(rune('a'+i)) + "@example.com",
		}
		userID, err := storage.Insert(ctx, user)
		if err != nil {
			t.Fatalf("Insert user error = %v", err)
		}
		cu := &models.CommunityUser{
			CommunityId:      communityID,
			UserId:           userID,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		if _, err := storage.Insert(ctx, cu); err != nil {
			t.Fatalf("Insert community user error = %v", err)
		}
	}

	users, err := storage.SearchCommunityUsersByName(ctx, communityID, "Common", 2)
	if err != nil {
		t.Fatalf("SearchCommunityUsersByName error = %v", err)
	}
	if len(users) != 2 {
		t.Errorf("SearchCommunityUsersByName with limit 2 returned %d users, want 2", len(users))
	}
}

// TestClampStoryLimit covers the ceiling added in #2795. Before it, only the
// floor existed, so a caller could ask for an unbounded number of rows.
func TestClampStoryLimit(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{0, defaultStoryLimit},
		{-1, defaultStoryLimit},
		{10, 10},
		{maxStoryLimit, maxStoryLimit},
		{maxStoryLimit + 1, maxStoryLimit},
		{1 << 30, maxStoryLimit},
	}
	for _, tt := range tests {
		if got := clampStoryLimit(tt.in); got != tt.want {
			t.Errorf("clampStoryLimit(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
