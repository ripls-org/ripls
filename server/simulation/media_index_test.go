package simulation

import (
	"testing"
)

func TestCommunityAssetFilename(t *testing.T) {
	tests := []struct {
		idx  int
		want string
	}{
		{0, "community_000.jpg"},
		{1, "community_001.jpg"},
		{99, "community_099.jpg"},
	}
	for _, tt := range tests {
		got := CommunityAssetFilename(tt.idx)
		if got != tt.want {
			t.Errorf("CommunityAssetFilename(%d) = %q, want %q", tt.idx, got, tt.want)
		}
	}
}

func TestAllUniqueEmails(t *testing.T) {
	emails := AllUniqueEmails()
	if len(emails) == 0 {
		t.Fatal("AllUniqueEmails() returned empty")
	}

	// Verify no duplicates.
	seen := make(map[string]bool)
	for _, e := range emails {
		if seen[e] {
			t.Errorf("duplicate email: %q", e)
		}
		seen[e] = true
	}

	// Verify all scenario members are accounted for.
	expected := make(map[string]bool)
	for _, s := range AllScenarios() {
		for _, c := range s.Communities {
			for _, m := range c.Members {
				expected[m.Email] = true
			}
		}
	}
	if len(emails) != len(expected) {
		t.Errorf("AllUniqueEmails() returned %d emails, want %d", len(emails), len(expected))
	}
	for _, e := range emails {
		if !expected[e] {
			t.Errorf("unexpected email: %q", e)
		}
	}
}

func TestAllUniqueEmailsStableOrder(t *testing.T) {
	first := AllUniqueEmails()
	second := AllUniqueEmails()
	if len(first) != len(second) {
		t.Fatalf("different lengths: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("order differs at index %d: %q vs %q", i, first[i], second[i])
		}
	}
}

func TestAllUniqueCommunities(t *testing.T) {
	communities := AllUniqueCommunities()
	if len(communities) == 0 {
		t.Fatal("AllUniqueCommunities() returned empty")
	}

	// Verify no duplicates.
	seen := make(map[string]bool)
	for _, c := range communities {
		if seen[c.Name] {
			t.Errorf("duplicate community: %q", c.Name)
		}
		seen[c.Name] = true
	}

	// Verify all scenario communities are accounted for.
	expected := make(map[string]bool)
	for _, s := range AllScenarios() {
		for _, c := range s.Communities {
			expected[c.Name] = true
		}
	}
	if len(communities) != len(expected) {
		t.Errorf("AllUniqueCommunities() returned %d communities, want %d", len(communities), len(expected))
	}
	for _, c := range communities {
		if !expected[c.Name] {
			t.Errorf("unexpected community: %q", c.Name)
		}
		if c.Description == "" {
			t.Errorf("community %q has empty description", c.Name)
		}
	}
}

func TestAllUniqueCommunitiesStableOrder(t *testing.T) {
	first := AllUniqueCommunities()
	second := AllUniqueCommunities()
	if len(first) != len(second) {
		t.Fatalf("different lengths: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("order differs at index %d: %q vs %q", i, first[i].Name, second[i].Name)
		}
	}
}
