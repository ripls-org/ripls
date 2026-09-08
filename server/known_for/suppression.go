package known_for

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// LoadSuppressedKeysForCommunities returns the normalized category
// keys suppressed for each community in [communityIDs]. The outer map
// is keyed by community id; the inner set holds the category keys
// suppressed for that community.
//
// A single batched query — never per-community in a loop, per the
// SQL-efficiency rules. Empty input yields an empty map.
func LoadSuppressedKeysForCommunities(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityIDs []string,
) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{})
	if len(communityIDs) == 0 {
		return out, nil
	}
	rows, err := storage.QueryByFieldIn[*models.SuppressedKnownFor](
		s, ctx, "scope_id", communityIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query suppressed_known_for by community: %w", err)
	}
	for _, r := range rows {
		if r == nil {
			continue
		}
		if r.ScopeKind != models.SuppressionScope_SUPPRESSION_SCOPE_COMMUNITY {
			continue
		}
		set, ok := out[r.ScopeId]
		if !ok {
			set = make(map[string]struct{})
			out[r.ScopeId] = set
		}
		set[r.CategoryKey] = struct{}{}
	}
	return out, nil
}

// LoadSuppressedKeysForUser returns the normalized category keys
// suppressed for one user (the per-user "known for" surface). A
// single query. Empty input — an empty user id — yields an empty
// set.
func LoadSuppressedKeysForUser(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	userID string,
) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if userID == "" {
		return out, nil
	}
	rows, err := storage.QueryByField[*models.SuppressedKnownFor](
		s, ctx, "scope_id", userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query suppressed_known_for by user: %w", err)
	}
	for _, r := range rows {
		if r == nil {
			continue
		}
		if r.ScopeKind != models.SuppressionScope_SUPPRESSION_SCOPE_USER {
			continue
		}
		out[r.CategoryKey] = struct{}{}
	}
	return out, nil
}

// CollapseCommunitySuppressions returns the union of suppressed
// category keys across [perCommunity]. The chip-derivation path
// (per-community mode) treats a category as suppressed when ANY
// in-scope community suppresses it — a single member opting out is
// enough to drop the chip from the aggregate. The detail path uses
// the same rule for consistency.
func CollapseCommunitySuppressions(
	perCommunity map[string]map[string]struct{},
) map[string]struct{} {
	out := make(map[string]struct{})
	for _, set := range perCommunity {
		for key := range set {
			out[key] = struct{}{}
		}
	}
	return out
}
