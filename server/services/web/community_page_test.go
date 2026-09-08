package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertNamedCommunity inserts a community with an explicit name and
// description, for the landing assertions that care about both.
func insertNamedCommunity(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
	creatorID, name, description string,
) *models.Community {
	t.Helper()
	now := time.Now().Unix()
	c := &models.Community{
		Id:               uuid.New().String(),
		Name:             name,
		Description:      description,
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	if _, err := sqlStorage.Insert(ctx, c); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return c
}

// TestHandleCommunityLanding_Valid covers the happy path: a plain community
// invite renders the phone-first landing (not the install-only card that
// preceded it), with a CTA into the Flutter web join route.
func TestHandleCommunityLanding_Valid(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice Nguyen")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id,
		"Ferndale Tool Library", "Neighbors sharing what they own.")
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=30" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=30")
	}
	if got := resp.Header.Get("Vary"); got != "Accept-Language" {
		t.Errorf("Vary = %q, want %q", got, "Accept-Language")
	}

	body := w.Body.String()
	assertContains(t, body, "Ferndale Tool Library")
	assertContains(t, body, "Neighbors sharing what they own.")
	assertContains(t, body, "Invitation")  // hero kicker
	assertContains(t, body, "invited you") // inviter role line
	assertContains(t, body, "1 member")    // member count, singular
	assertContains(t, body, "Join the group")

	// First name only — the landing never exposes a full name.
	assertContains(t, body, "Alice")
	assertNotContains(t, body, "Nguyen")

	// The CTA hands off to the Flutter web join route with the intent
	// carrier and the share code, which is what makes the loop phone-first.
	assertContains(t, body,
		fmt.Sprintf("/group/%s?intent=join&amp;code=%s", community.Id, invitation.ShortCode))

	// The install-only landing this replaced is gone.
	assertNotContains(t, body, "You&#39;re Invited")
	assertNotContains(t, body, "Open Ripls App")
	assertNotContains(t, body, "download the Ripls app")
}

// TestHandleCommunityLanding_DiscussDestination covers the `?to=discuss` hint
// (#2876): a link that arrived from a "say hi" notification drops the
// invitation framing — its recipients are members by construction — and points
// the CTA at the conversation.
func TestHandleCommunityLanding_DiscussDestination(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode+"?to=discuss", nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	body := w.Body.String()

	// Member framing: discussion kicker, conversation CTA, community name.
	assertContains(t, body, "Discussion")
	assertContains(t, body, "Open the discussion")
	assertContains(t, body, "Ferndale Tool Library")

	// The invitation framing is gone — this reader has been a member for weeks.
	assertNotContains(t, body, "invited you")
	assertNotContains(t, body, "Join the group")

	// And the CTA threads the tab through to the web screen, after the join
	// intent and the share code (html/template percent-escapes the name).
	assertContains(t, body,
		fmt.Sprintf("/group/%s?intent=join&amp;code=%s&amp;n=Ferndale%%20Tool%%20Library&amp;tab=discuss",
			community.Id, invitation.ShortCode))
}

// TestHandleCommunityLanding_DestinationHintIsNotAuthorization is the security
// invariant behind the hint: it selects copy and a landing tab, nothing else. A
// stranger who forges `?to=discuss` on someone's invite link gets the same
// public page and the same join CTA — no member roster, no conversation
// content, no shortcut past AcceptInvitationLink.
func TestHandleCommunityLanding_DestinationHintIsNotAuthorization(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	member := createTestUser(t, ctx, sqlStorage, "Zebediah")
	createTestMembership(t, ctx, sqlStorage, community.Id, member.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode+"?to=discuss", nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	// Still a public, pre-auth page: no roster, whatever the hint says.
	assertNotContains(t, body, "Zebediah")
	// Still routed through the ordinary join — the tab rides along with it.
	assertContains(t, body, "intent=join")
}

// TestHandleCommunityLanding_UnknownDestination proves the hint is a closed
// vocabulary: anything unrecognized is ignored rather than erroring or leaking
// into the CTA, so a stale or hand-edited link still renders the normal invite.
func TestHandleCommunityLanding_UnknownDestination(t *testing.T) {
	for _, raw := range []string{"bogus", "DISCUSS", "discuss ", "", "../../etc"} {
		t.Run("to="+raw, func(t *testing.T) {
			svc, sqlStorage, _ := setupTestService(t)
			ctx := context.Background()

			inviter := createTestUser(t, ctx, sqlStorage, "Alice")
			community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
			createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
			invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

			req := httptest.NewRequest(http.MethodGet,
				"/go/"+invitation.ShortCode+"?to="+url.QueryEscape(raw), nil)
			w := httptest.NewRecorder()
			svc.HandleInvitePage(w, req)

			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Result().StatusCode)
			}
			body := w.Body.String()
			assertContains(t, body, "Join the group")
			assertContains(t, body, "invited you")
			assertNotContains(t, body, "tab=")
		})
	}
}

// TestHandleCommunityLanding_HidesMemberNames is the privacy invariant: the
// page is readable by anyone holding an 8-character code, so it discloses a
// member *count* and the inviter's first name — never the roster.
func TestHandleCommunityLanding_HidesMemberNames(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)

	member := createTestUser(t, ctx, sqlStorage, "Zebediah")
	createTestMembership(t, ctx, sqlStorage, community.Id, member.Id)

	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "2 members")
	assertNotContains(t, body, "Zebediah")
}

// TestHandleCommunityLanding_AtCapacity verifies a full community swaps the
// join CTA for the shared at-capacity banner, with the join-flavored verb
// rather than the item pages' "respond".
func TestHandleCommunityLanding_AtCapacity(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
	for i := 0; i < communitylib.MaxCommunityMembers; i++ {
		u := createTestUser(t, ctx, sqlStorage, fmt.Sprintf("Member%d", i))
		createTestMembership(t, ctx, sqlStorage, community.Id, u.Id)
	}
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "This community is full")
	assertContains(t, body, "Can&#39;t join because this community is full")
	// No CTA to a flow that would be rejected server-side anyway.
	assertNotContains(t, body, "/group/"+community.Id)
}

// TestHandleCommunityLanding_NamelessCommunity verifies an ad-hoc community
// (the no-entity fallback that community-wide notification links resolve to)
// renders the generic public label rather than an empty name, and takes the
// headline variant that omits the name entirely.
func TestHandleCommunityLanding_NamelessCommunity(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamelessCommunity(t, ctx, sqlStorage, inviter.Id)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, wantAdHocLabel)
	assertContains(t, body, `content="Alice invited you to a Ripls group on Ripls"`)
	// The named-community headline would splice the label mid-sentence
	// ("invited you to join a Ripls group"); the unnamed variant must win.
	assertNotContains(t, body, "invited you to join a Ripls group")
}

// TestHandleCommunityLanding_Unavailable covers the states that can't render a
// landing — a deleted community, a soft-deleted one, and a missing inviter —
// all of which fall back to the shared error surface rather than a broken page.
func TestHandleCommunityLanding_Unavailable(t *testing.T) {
	cases := []struct {
		name string
		// mutate breaks the world after the invite is created; it returns
		// nothing because each case edits storage in place.
		mutate func(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
			community *models.Community, inviter *models.User)
	}{
		{
			name: "community deleted outright",
			mutate: func(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
				community *models.Community, _ *models.User,
			) {
				if err := sqlStorage.Delete(ctx, community); err != nil {
					t.Fatalf("delete community: %v", err)
				}
			},
		},
		{
			name: "community soft-deleted",
			mutate: func(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
				community *models.Community, _ *models.User,
			) {
				community.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
				if err := sqlStorage.Update(ctx, community); err != nil {
					t.Fatalf("soft-delete community: %v", err)
				}
			},
		},
		{
			name: "inviter gone",
			mutate: func(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
				_ *models.Community, inviter *models.User,
			) {
				if err := sqlStorage.Delete(ctx, inviter); err != nil {
					t.Fatalf("delete inviter: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, sqlStorage, _ := setupTestService(t)
			ctx := context.Background()

			inviter := createTestUser(t, ctx, sqlStorage, "Alice")
			community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", "")
			createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
			invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

			tc.mutate(t, ctx, sqlStorage, community, inviter)

			req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
			w := httptest.NewRecorder()
			svc.HandleInvitePage(w, req)

			if w.Result().StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 (the error surface is a rendered page)", w.Result().StatusCode)
			}
			body := w.Body.String()
			assertContains(t, body, "Invalid Invitation")
			assertNotContains(t, body, "Join the group")
		})
	}
}

// TestHandleCommunityLanding_TruncatesDescription verifies a long group
// description is clipped rather than dumped whole onto a public page.
func TestHandleCommunityLanding_TruncatesDescription(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	long := strings.Repeat("borrow ", 100) // 700 chars, well past the cap
	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamedCommunity(t, ctx, sqlStorage, inviter.Id, "Ferndale Tool Library", long)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "…")
	assertNotContains(t, body, long)
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"under the cap is untouched", "short", 10, "short"},
		{"exactly at the cap is untouched", "abcde", 5, "abcde"},
		{"over the cap gets an ellipsis", "abcdefgh", 5, "abcde…"},
		{"trailing space is trimmed before the ellipsis", "abcd efgh", 5, "abcd…"},
		{"empty stays empty", "", 5, ""},
		// Counts runes, not bytes: 6 accented chars are 12 bytes, so a
		// byte-based cut would split a character mid-sequence.
		{"multi-byte counts runes", "ááááá á", 5, "ááááá…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateRunes(tc.in, tc.limit); got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}
