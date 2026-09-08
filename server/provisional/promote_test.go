package provisional

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertProvisionalUserWithPhone inserts a phone-keyed ProvisionalUser and returns its ID.
func insertProvisionalUserWithPhone(t *testing.T, store *storage.ProtoSQLStorage, communityID, name, phone string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		CommunityId: communityID,
		Name:        name,
		Contact:     &models.ProvisionalUser_PhoneNumber{PhoneNumber: phone},
	}
	id, err := store.Insert(context.Background(), prov)
	if err != nil {
		t.Fatalf("insertProvisionalUserWithPhone: %v", err)
	}
	return id
}

// TestFindUnclaimedByPhone_MatchesAcrossCommunities verifies the lookup gathers
// every unclaimed phone match regardless of community, and ignores rows keyed by
// a different phone, by email, or by name only.
func TestFindUnclaimedByPhone_MatchesAcrossCommunities(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const phone = "+15551234567"

	commA := insertCommunity(t, store)
	commB := insertCommunity(t, store)

	idA := insertProvisionalUserWithPhone(t, store, commA, "Pat A", phone)
	idB := insertProvisionalUserWithPhone(t, store, commB, "Pat B", phone)

	// Noise that must NOT match.
	insertProvisionalUserWithPhone(t, store, commA, "Other Phone", "+15559999999")
	emailProv := &models.ProvisionalUser{
		CommunityId: commA,
		Name:        "Email Person",
		Contact:     &models.ProvisionalUser_Email{Email: "person@example.com"},
	}
	if _, err := store.Insert(ctx, emailProv); err != nil {
		t.Fatalf("insert email provisional: %v", err)
	}
	insertProvisionalUser(t, store, commA, "Name Only") // phone_number column == ""

	got, err := FindUnclaimedByPhone(ctx, store, phone)
	if err != nil {
		t.Fatalf("FindUnclaimedByPhone: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2 (one per community)", len(got))
	}

	ids := map[string]bool{}
	for _, p := range got {
		ids[p.Id] = true
	}
	if !ids[idA] || !ids[idB] {
		t.Errorf("expected matches %s (commA) and %s (commB), got %v", idA, idB, ids)
	}
}

// TestFindUnclaimedByPhone_ExcludesClaimed verifies an already-claimed match is
// skipped so promotion never re-processes it.
func TestFindUnclaimedByPhone_ExcludesClaimed(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const phone = "+15551112222"

	commA := insertCommunity(t, store)
	commB := insertCommunity(t, store)
	realUserID := insertUser(t, store, "claimer@example.com", "Claimer")

	claimed := &models.ProvisionalUser{
		CommunityId:     commA,
		Name:            "Already Claimed",
		Contact:         &models.ProvisionalUser_PhoneNumber{PhoneNumber: phone},
		ClaimedByUserId: &realUserID,
	}
	if _, err := store.Insert(ctx, claimed); err != nil {
		t.Fatalf("insert claimed provisional: %v", err)
	}
	unclaimedID := insertProvisionalUserWithPhone(t, store, commB, "Unclaimed", phone)

	got, err := FindUnclaimedByPhone(ctx, store, phone)
	if err != nil {
		t.Fatalf("FindUnclaimedByPhone: %v", err)
	}
	if len(got) != 1 || got[0].Id != unclaimedID {
		t.Fatalf("got %d matches, want exactly the unclaimed one (%s)", len(got), unclaimedID)
	}
}

// TestFindUnclaimedByPhone_EmptyPhone verifies an empty phone matches nothing —
// it must never sweep up the email- and name-only rows whose phone_number
// column defaults to the empty string.
func TestFindUnclaimedByPhone_EmptyPhone(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	comm := insertCommunity(t, store)
	insertProvisionalUser(t, store, comm, "Name Only")

	got, err := FindUnclaimedByPhone(ctx, store, "")
	if err != nil {
		t.Fatalf("FindUnclaimedByPhone: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty phone should match nothing, got %d", len(got))
	}
}

// insertProvisionalUserWithEmail inserts an email-keyed ProvisionalUser and returns its ID.
func insertProvisionalUserWithEmail(t *testing.T, store *storage.ProtoSQLStorage, communityID, name, email string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		CommunityId: communityID,
		Name:        name,
		Contact:     &models.ProvisionalUser_Email{Email: email},
	}
	id, err := store.Insert(context.Background(), prov)
	if err != nil {
		t.Fatalf("insertProvisionalUserWithEmail: %v", err)
	}
	return id
}

// TestFindUnclaimedByEmail_MatchesAcrossCommunities mirrors the phone test: the
// lookup gathers every unclaimed email match regardless of community, ignoring
// rows keyed by a different email, by phone, or by name only.
func TestFindUnclaimedByEmail_MatchesAcrossCommunities(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const email = "invitee@example.com"

	commA := insertCommunity(t, store)
	commB := insertCommunity(t, store)

	idA := insertProvisionalUserWithEmail(t, store, commA, "Pat A", email)
	idB := insertProvisionalUserWithEmail(t, store, commB, "Pat B", email)

	// Noise that must NOT match.
	insertProvisionalUserWithEmail(t, store, commA, "Other Email", "other@example.com")
	insertProvisionalUserWithPhone(t, store, commA, "Phone Person", "+15551234567")
	insertProvisionalUser(t, store, commA, "Name Only") // email column == ""

	got, err := FindUnclaimedByEmail(ctx, store, email)
	if err != nil {
		t.Fatalf("FindUnclaimedByEmail: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches, want 2 (one per community)", len(got))
	}
	ids := map[string]bool{}
	for _, p := range got {
		ids[p.Id] = true
	}
	if !ids[idA] || !ids[idB] {
		t.Errorf("expected matches %s (commA) and %s (commB), got %v", idA, idB, ids)
	}
}

// TestFindUnclaimedByEmail_ExcludesClaimed verifies an already-claimed match is
// skipped so promotion never re-processes it.
func TestFindUnclaimedByEmail_ExcludesClaimed(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const email = "claimme@example.com"

	commA := insertCommunity(t, store)
	commB := insertCommunity(t, store)
	realUserID := insertUser(t, store, "claimer2@example.com", "Claimer")

	claimed := &models.ProvisionalUser{
		CommunityId:     commA,
		Name:            "Already Claimed",
		Contact:         &models.ProvisionalUser_Email{Email: email},
		ClaimedByUserId: &realUserID,
	}
	if _, err := store.Insert(ctx, claimed); err != nil {
		t.Fatalf("insert claimed provisional: %v", err)
	}
	unclaimedID := insertProvisionalUserWithEmail(t, store, commB, "Unclaimed", email)

	got, err := FindUnclaimedByEmail(ctx, store, email)
	if err != nil {
		t.Fatalf("FindUnclaimedByEmail: %v", err)
	}
	if len(got) != 1 || got[0].Id != unclaimedID {
		t.Fatalf("got %d matches, want exactly the unclaimed one (%s)", len(got), unclaimedID)
	}
}

// TestFindUnclaimedByEmail_EmptyEmail verifies an empty email matches nothing —
// it must never sweep up the phone- and name-only rows whose email column
// defaults to the empty string.
func TestFindUnclaimedByEmail_EmptyEmail(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	comm := insertCommunity(t, store)
	insertProvisionalUser(t, store, comm, "Name Only")
	insertProvisionalUserWithPhone(t, store, comm, "Phone Person", "+15550001111")

	got, err := FindUnclaimedByEmail(ctx, store, "")
	if err != nil {
		t.Fatalf("FindUnclaimedByEmail: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty email should match nothing, got %d", len(got))
	}
}

// TestPromoteByPhone_JoinsClaimsAndPublishes covers the full promotion path with
// a real publisher: a non-member is joined to the provisional's community, the
// join is announced through the bus, and the provisional is claimed.
func TestPromoteByPhone_JoinsClaimsAndPublishes(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const phone = "+15551234567"
	const userID = "real-user-1"

	comm := insertCommunity(t, store)
	provID := insertProvisionalUserWithPhone(t, store, comm, "Pat", phone)

	bus := cebus.NewMockBus()
	PromoteByPhone(ctx, store, bus, userID, phone)

	// Provisional claimed by the real user.
	prov := &models.ProvisionalUser{}
	if err := store.GetByID(ctx, provID, prov); err != nil {
		t.Fatalf("get provisional: %v", err)
	}
	if prov.ClaimedByUserId == nil || *prov.ClaimedByUserId != userID {
		t.Errorf("claimed_by = %v, want %s", prov.ClaimedByUserId, userID)
	}

	// User joined the community.
	isMember, err := auth.IsMemberOfCommunity(ctx, store, comm, userID)
	if err != nil {
		t.Fatalf("IsMemberOfCommunity: %v", err)
	}
	if !isMember {
		t.Error("user should be a member of the community after promotion")
	}

	// The join was announced exactly once through the publisher.
	captured := bus.Captured()
	if len(captured) != 1 {
		t.Fatalf("published events = %d, want 1", len(captured))
	}
	ev := captured[0]
	if ev.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED {
		t.Errorf("event type = %v, want INVITATION_LINK_USED", ev.EventType)
	}
	if ev.CommunityId != comm || ev.ActorId != userID {
		t.Errorf("event = (community %q, actor %q), want (%q, %q)", ev.CommunityId, ev.ActorId, comm, userID)
	}
}

// TestPromoteByPhone_AlreadyMember_NoJoinEvent verifies the idempotent branch:
// an already-joined member is still merged (provisional claimed) but the join is
// not re-announced.
func TestPromoteByPhone_AlreadyMember_NoJoinEvent(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()
	const phone = "+15559998888"
	const userID = "real-user-2"

	comm := insertCommunity(t, store)
	provID := insertProvisionalUserWithPhone(t, store, comm, "Pat", phone)
	if err := addUserToCommunity(ctx, store, userID, comm, "inviter"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	bus := cebus.NewMockBus()
	PromoteByPhone(ctx, store, bus, userID, phone)

	// Provisional is still claimed/merged...
	prov := &models.ProvisionalUser{}
	if err := store.GetByID(ctx, provID, prov); err != nil {
		t.Fatalf("get provisional: %v", err)
	}
	if prov.ClaimedByUserId == nil || *prov.ClaimedByUserId != userID {
		t.Errorf("claimed_by = %v, want %s", prov.ClaimedByUserId, userID)
	}
	// ...but no join event is published for an existing member.
	if n := len(bus.Captured()); n != 0 {
		t.Errorf("published events = %d, want 0 (already a member)", n)
	}
}

// TestPromoteByPhone_NoMatch_NoOp verifies that a phone with no provisional
// placeholder neither joins a community nor publishes anything.
func TestPromoteByPhone_NoMatch_NoOp(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	bus := cebus.NewMockBus()
	PromoteByPhone(ctx, store, bus, "real-user-3", "+15550000000")

	if n := len(bus.Captured()); n != 0 {
		t.Errorf("published events = %d, want 0 (no provisional match)", n)
	}
}
