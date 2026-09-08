package services

import (
	"context"
	"fmt"
	"sort"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// MaxInvitedIndividuals caps the directly-invited-individuals list on item GET
// responses so an item shared into a huge ad-hoc community can't return an
// unbounded roster. The full audience size is total_distinct_member_count.
const MaxInvitedIndividuals = 50

// PrimaryAvatarMediaID returns the user's primary avatar media id.
//
// User carries the avatar in two storage fields — User.media_id and
// User.media_ids[0] — that the proto invariant says are kept in sync.
// #2083 is deprecating the singular field; this helper is the read-side
// counterpart so callers don't reach into models.User.MediaId directly.
// Returns "" when the user has no avatar.
func PrimaryAvatarMediaID(user *models.User) string {
	if user == nil || len(user.MediaIds) == 0 {
		return ""
	}
	return user.MediaIds[0]
}

// ToAPIUser converts a models.User to api.User for API responses.
// Returns nil if the input user is nil.
func ToAPIUser(user *models.User) *api.User {
	if user == nil {
		return nil
	}
	return &api.User{
		Id:      user.Id,
		Name:    user.Name,
		MediaId: PrimaryAvatarMediaID(user),
	}
}

// FetchAPIUser fetches a user from storage by ID and converts it to api.User.
// Returns an error if the user is not found or cannot be fetched.
func FetchAPIUser(ctx context.Context, s *storage.ProtoSQLStorage, userID string) (*api.User, error) {
	if userID == "" {
		return nil, nil
	}

	user := &models.User{}
	if err := s.GetByID(ctx, userID, user); err != nil {
		return nil, err
	}

	return ToAPIUser(user), nil
}

// FetchAPIUsers fetches multiple users from storage by IDs and converts them to api.User.
// If a user is not found, it is skipped (not included in the result).
// Preserves input order and duplicates.
func FetchAPIUsers(ctx context.Context, s *storage.ProtoSQLStorage, userIDs []string) ([]*api.User, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Batch fetch all users, then build ordered result
	userMap, err := FetchAPIUsersBatch(ctx, s, userIDs)
	if err != nil {
		return nil, err
	}

	var result []*api.User
	for _, userID := range userIDs {
		if u, ok := userMap[userID]; ok {
			result = append(result, u)
		}
	}

	return result, nil
}

// FormerMemberPlaceholder builds a placeholder *api.User for a referenced
// user id that can no longer be resolved (e.g. soft-deleted account).
// The placeholder carries the original id (clients use it for list keys
// and de-duplication) but no PII: name and media_id are empty. Clients
// detect the placeholder via FormerMember=true and render a localized
// "Former Member" string.
func FormerMemberPlaceholder(userID string) *api.User {
	return &api.User{
		Id:           userID,
		FormerMember: true,
	}
}

// ResolveUserOrFormer looks up userID in userMap and returns the resolved
// *api.User, or a former-member placeholder when the id is non-empty but
// missing. Empty userID returns nil so callers preserve the proto's
// "field absent" semantics for optional user references.
func ResolveUserOrFormer(userMap map[string]*api.User, userID string) *api.User {
	if userID == "" {
		return nil
	}
	if u, ok := userMap[userID]; ok && u != nil {
		return u
	}
	return FormerMemberPlaceholder(userID)
}

// FetchAPIUsersBatch fetches multiple users from storage by IDs in a single batch query
// and returns them as a map of userID → *api.User for O(1) lookups.
// Empty IDs are ignored. Missing users are silently omitted from the result.
func FetchAPIUsersBatch(ctx context.Context, s *storage.ProtoSQLStorage, userIDs []string) (map[string]*api.User, error) {
	if len(userIDs) == 0 {
		return map[string]*api.User{}, nil
	}

	// Deduplicate IDs
	seen := make(map[string]struct{}, len(userIDs))
	unique := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}

	if len(unique) == 0 {
		return map[string]*api.User{}, nil
	}

	// Single batch query
	userMap, err := storage.GetByIDs[*models.User](s, ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("batch fetch users: %w", err)
	}

	// Convert to api.User map
	result := make(map[string]*api.User, len(userMap))
	for id, user := range userMap {
		result[id] = ToAPIUser(user)
	}

	return result, nil
}

// FetchInvitedIndividuals returns the API users for the people individually
// invited to an item's ad-hoc origin community — the contacts a host invited
// one by one (#2492). It is the shared core behind the "Shared with" / "Who's
// In" rosters across gear, requests, and experiences.
//
// memberIDs are the origin community's member user IDs. ownerID and any user ID
// present in exclude are dropped (exclude may be nil — experiences pass the set
// of users who already responded so the list shows only the "still no reply"
// invitees; gear/requests pass nil). The result is sorted by user ID for stable
// output, then capped at MaxInvitedIndividuals. Returns nil (no error) when no
// invitees remain.
func FetchInvitedIndividuals(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	memberIDs []string,
	ownerID string,
	exclude map[string]struct{},
) ([]*api.User, error) {
	invitedIDs := make([]string, 0, len(memberIDs))
	for _, uid := range memberIDs {
		if uid == ownerID {
			continue
		}
		if exclude != nil {
			if _, skip := exclude[uid]; skip {
				continue
			}
		}
		invitedIDs = append(invitedIDs, uid)
	}
	sort.Strings(invitedIDs)
	if len(invitedIDs) > MaxInvitedIndividuals {
		invitedIDs = invitedIDs[:MaxInvitedIndividuals]
	}
	if len(invitedIDs) == 0 {
		return nil, nil
	}

	users, err := FetchAPIUsersBatch(ctx, s, invitedIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch invited individuals: %w", err)
	}
	invited := make([]*api.User, 0, len(invitedIDs))
	for _, uid := range invitedIDs {
		if u, ok := users[uid]; ok {
			invited = append(invited, u)
		}
	}
	return invited, nil
}
