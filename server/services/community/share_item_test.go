package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fakeItemSharer stands in for an item service in ShareItem tests. It records
// the communities an item was shared to and, like the real experience sharer,
// writes a CommunityExperience junction so the share-link mint's precondition
// (item is shared with the community) is satisfied.
type fakeItemSharer struct {
	st              *storage.ProtoSQLStorage
	ownerID         string
	verifyOwnerErr  error
	verifiedOwner   bool
	verifyViewerErr error
	verifiedViewer  bool
	sharedTo        []string
}

func (f *fakeItemSharer) verifyOwner(_ context.Context, _, _ string) error {
	f.verifiedOwner = true
	return f.verifyOwnerErr
}

func (f *fakeItemSharer) verifyViewer(_ context.Context, _, _ string) (string, error) {
	f.verifiedViewer = true
	if f.verifyViewerErr != nil {
		return "", f.verifyViewerErr
	}
	return f.ownerID, nil
}

func (f *fakeItemSharer) shareToCommunity(ctx context.Context, itemID, communityID, _ string) error {
	f.sharedTo = append(f.sharedTo, communityID)
	_, err := f.st.Insert(ctx, &models.CommunityExperience{
		CommunityId:     communityID,
		ExperienceId:    itemID,
		SharedAtUnixSec: 1,
	})
	return err
}

// fakeInviteEmailSender captures invite emails dispatched during ShareItem.
type fakeInviteEmailSender struct {
	sent []email.InviteEmailInput
	err  error
}

func (f *fakeInviteEmailSender) SendInviteEmail(_ context.Context, in email.InviteEmailInput) error {
	f.sent = append(f.sent, in)
	return f.err
}

// setupShareItemTest wires a service with a fake experience sharer + email
// sender, an inserted experience owned by host, and an authed host context.
func setupShareItemTest(t *testing.T) (*Service, *storage.ProtoSQLStorage, *fakeItemSharer, *fakeInviteEmailSender, string, string, context.Context) {
	t.Helper()
	st := setupTestStorage(t)
	svc := setupTestService(t, st)

	hostID := setupTestUser(t, st, "host@example.com", "Host")
	ctx := createAuthenticatedContext(hostID, "host@example.com", models.Role_ROLE_USER)

	expID, err := st.Insert(ctx, &models.Experience{OwnerId: hostID, Name: "Picnic"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	sharer := &fakeItemSharer{st: st, ownerID: hostID}
	svc.SetItemSharer(ItemKindExperience, ItemSharer{
		VerifyOwner:      sharer.verifyOwner,
		VerifyViewer:     sharer.verifyViewer,
		ShareToCommunity: sharer.shareToCommunity,
	})
	emailer := &fakeInviteEmailSender{}
	svc.SetInviteEmailSender(emailer)

	return svc, st, sharer, emailer, hostID, expID, ctx
}

func expTarget(expID string) *api.ShareItemRequest_ExperienceId {
	return &api.ShareItemRequest_ExperienceId{ExperienceId: expID}
}

func memberInvitee(uid string) *api.Invitee {
	return &api.Invitee{Identity: &api.Invitee_MemberUserId{MemberUserId: uid}}
}

func phoneInvitee(phone, name string) *api.Invitee {
	return &api.Invitee{Identity: &api.Invitee_PhoneNumber{PhoneNumber: phone}, DisplayName: &name}
}

func emailInvitee(email, name string) *api.Invitee {
	return &api.Invitee{Identity: &api.Invitee_Email{Email: email}, DisplayName: &name}
}

func provisionalsIn(t *testing.T, st *storage.ProtoSQLStorage, communityID string) []*models.ProvisionalUser {
	t.Helper()
	rows, err := st.QueryByField(context.Background(), "community_id", communityID, &models.ProvisionalUser{})
	if err != nil {
		t.Fatalf("query provisionals: %v", err)
	}
	out := make([]*models.ProvisionalUser, len(rows))
	for i, r := range rows {
		out[i] = r.(*models.ProvisionalUser)
	}
	return out
}

func consentRecordsFor(t *testing.T, st *storage.ProtoSQLStorage, communityID string) []*models.InviteConsent {
	t.Helper()
	rows, err := st.QueryByField(context.Background(), "community_id", communityID, &models.InviteConsent{})
	if err != nil {
		t.Fatalf("query invite consents: %v", err)
	}
	out := make([]*models.InviteConsent, len(rows))
	for i, r := range rows {
		out[i] = r.(*models.InviteConsent)
	}
	return out
}

func TestShareItem_MemberOnly(t *testing.T) {
	svc, st, sharer, _, hostID, expID, ctx := setupShareItemTest(t)
	memberID := setupTestUser(t, st, "member@example.com", "Member")

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     expTarget(expID),
		Invitees: []*api.Invitee{memberInvitee(memberID)},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	if !sharer.verifiedOwner {
		t.Error("expected owner verification to run")
	}
	if resp.Msg.AdhocCommunityId == nil || *resp.Msg.AdhocCommunityId == "" {
		t.Fatal("expected an ad-hoc community id")
	}
	if resp.Msg.ShareUrl == nil || *resp.Msg.ShareUrl == "" {
		t.Error("expected a share url")
	}
	adhocID := *resp.Msg.AdhocCommunityId

	// Host + member are members of the ad-hoc community.
	members, err := st.QueryByField(ctx, "community_id", adhocID, &models.CommunityUser{})
	if err != nil {
		t.Fatalf("query members: %v", err)
	}
	got := map[string]bool{}
	for _, m := range members {
		got[m.(*models.CommunityUser).UserId] = true
	}
	if !got[hostID] || !got[memberID] || len(members) != 2 {
		t.Errorf("expected {host, member}, got %v", got)
	}

	// No off-app contacts → no provisional users and no consent record.
	if ps := provisionalsIn(t, st, adhocID); len(ps) != 0 {
		t.Errorf("expected 0 provisional users, got %d", len(ps))
	}
	if cs := consentRecordsFor(t, st, adhocID); len(cs) != 0 {
		t.Errorf("expected 0 consent records for member-only invite, got %d", len(cs))
	}
	// Item shared to the ad-hoc community.
	if len(sharer.sharedTo) != 1 || sharer.sharedTo[0] != adhocID {
		t.Errorf("expected item shared to ad-hoc community, got %v", sharer.sharedTo)
	}
}

// TestShareItem_ReinviteAfterRemoval guards the host loop of un-inviting then
// re-inviting the same member over and over. RemoveExperienceMember soft-deletes
// the member's per-item CommunityUser row; a naive re-invite would Insert a fresh
// row and collide on (community_id, user_id). addMembersToCommunity must instead
// restore the soft-deleted row, so the cycle can repeat without error.
func TestShareItem_ReinviteAfterRemoval(t *testing.T) {
	svc, st, _, _, hostID, expID, ctx := setupShareItemTest(t)
	memberID := setupTestUser(t, st, "member@example.com", "Member")

	share := func() string {
		t.Helper()
		resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
			Item:     expTarget(expID),
			Invitees: []*api.Invitee{memberInvitee(memberID)},
		}))
		if err != nil {
			t.Fatalf("ShareItem: %v", err)
		}
		return *resp.Msg.AdhocCommunityId
	}

	// memberRows returns the member's CommunityUser rows in the community,
	// optionally including soft-deleted ones.
	memberRows := func(communityID string, includeDeleted bool) []*models.CommunityUser {
		t.Helper()
		opts := []storage.QueryOptions{}
		if includeDeleted {
			opts = append(opts, storage.QueryOptions{IncludeDeleted: true})
		}
		rows, err := st.QueryByFields(ctx, map[string]any{
			"community_id": communityID,
			"user_id":      memberID,
		}, &models.CommunityUser{}, opts...)
		if err != nil {
			t.Fatalf("query member rows: %v", err)
		}
		out := make([]*models.CommunityUser, len(rows))
		for i, r := range rows {
			out[i] = r.(*models.CommunityUser)
		}
		return out
	}

	// removeMember mirrors experience.RemoveExperienceMember soft-deleting the
	// member's per-item membership.
	removeMember := func(communityID string, at int64) {
		t.Helper()
		rows := memberRows(communityID, false)
		if len(rows) != 1 {
			t.Fatalf("expected exactly 1 active member row to remove, got %d", len(rows))
		}
		cu := rows[0]
		cu.Deleted = &models.DeletedMetadata{DeletedByUserId: hostID, DeletedAtUnixSec: at}
		if err := st.Update(ctx, cu); err != nil {
			t.Fatalf("soft-delete member: %v", err)
		}
	}

	adhocID := share()
	if got := memberRows(adhocID, false); len(got) != 1 {
		t.Fatalf("after first invite: expected 1 active member row, got %d", len(got))
	}

	for cycle := 1; cycle <= 3; cycle++ {
		removeMember(adhocID, int64(1000+cycle))
		if got := memberRows(adhocID, false); len(got) != 0 {
			t.Fatalf("cycle %d: expected member removed (0 active rows), got %d", cycle, len(got))
		}
		if got := share(); got != adhocID {
			t.Fatalf("cycle %d: re-invite should reuse per-item community %q, got %q", cycle, adhocID, got)
		}
		active := memberRows(adhocID, false)
		if len(active) != 1 {
			t.Fatalf("cycle %d: expected exactly 1 active member row after re-invite, got %d", cycle, len(active))
		}
		if d := active[0].Deleted; d != nil && d.DeletedAtUnixSec != 0 {
			t.Fatalf("cycle %d: restored row still marked deleted", cycle)
		}
		if active[0].InviterId != hostID {
			t.Errorf("cycle %d: restored row inviter = %q, want host %q", cycle, active[0].InviterId, hostID)
		}
	}

	// Restored, never duplicated: exactly one physical row survives all churn.
	if all := memberRows(adhocID, true); len(all) != 1 {
		t.Fatalf("expected exactly 1 physical member row across all cycles, got %d", len(all))
	}
}

func TestShareItem_EmailInviteSendsAndPersistsConsent(t *testing.T) {
	svc, st, _, emailer, _, expID, ctx := setupShareItemTest(t)

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:          expTarget(expID),
		Invitees:      []*api.Invitee{emailInvitee("Friend@Example.com", "Friend")},
		InviteConsent: &api.HostInviteConsent{Attested: true},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	adhocID := *resp.Msg.AdhocCommunityId

	// Provisional user created with the normalized email.
	ps := provisionalsIn(t, st, adhocID)
	if len(ps) != 1 || ps[0].GetEmail() != "friend@example.com" {
		t.Fatalf("expected 1 email provisional (normalized), got %+v", ps)
	}

	// Email dispatched with the share url.
	if len(emailer.sent) != 1 {
		t.Fatalf("expected 1 invite email, got %d", len(emailer.sent))
	}
	if emailer.sent[0].ToEmail != "friend@example.com" || emailer.sent[0].ShareURL != *resp.Msg.ShareUrl {
		t.Errorf("unexpected invite email: %+v", emailer.sent[0])
	}

	// Consent persisted with the invitee recorded.
	cs := consentRecordsFor(t, st, adhocID)
	if len(cs) != 1 {
		t.Fatalf("expected 1 consent record, got %d", len(cs))
	}
	if cs[0].GetOriginExperienceId() != expID {
		t.Errorf("expected consent origin_experience_id %q, got %q", expID, cs[0].GetOriginExperienceId())
	}
	if len(cs[0].Invitees) != 1 || cs[0].Invitees[0].GetEmail() != "friend@example.com" {
		t.Errorf("expected 1 email invitee in consent, got %+v", cs[0].Invitees)
	}
}

func TestShareItem_PhoneRequiresConsent(t *testing.T) {
	svc, _, _, _, _, expID, ctx := setupShareItemTest(t)

	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     expTarget(expID),
		Invitees: []*api.Invitee{phoneInvitee("+15551234567", "Pat")},
		// No consent.
	}))
	if err == nil {
		t.Fatal("expected an error inviting a phone number without consent")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", connect.CodeOf(err))
	}
}

func TestShareItem_PhoneIsHostRelayedNotEmailed(t *testing.T) {
	svc, st, _, emailer, _, expID, ctx := setupShareItemTest(t)

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:          expTarget(expID),
		Invitees:      []*api.Invitee{phoneInvitee("+1 (555) 123-4567", "Pat")},
		InviteConsent: &api.HostInviteConsent{Attested: true},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	adhocID := *resp.Msg.AdhocCommunityId

	// Provisional created with normalized E.164; no email sent (host relays).
	ps := provisionalsIn(t, st, adhocID)
	if len(ps) != 1 || ps[0].GetPhoneNumber() != "+15551234567" {
		t.Fatalf("expected 1 phone provisional in E.164, got %+v", ps)
	}
	if len(emailer.sent) != 0 {
		t.Errorf("expected no emails for a phone invite, got %d", len(emailer.sent))
	}

	// Consent persisted with attested=true.
	cs := consentRecordsFor(t, st, adhocID)
	if len(cs) != 1 || cs[0].Consent == nil || !cs[0].Consent.Attested {
		t.Errorf("expected a persisted attested consent, got %+v", cs)
	}
}

func TestShareItem_SharesToExistingCommunities(t *testing.T) {
	svc, st, sharer, _, hostID, expID, ctx := setupShareItemTest(t)

	// A real community the host owns/belongs to.
	createResp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{Name: "Block"}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}
	existingID := createResp.Msg.Id
	_ = hostID

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                expTarget(expID),
		ShareToCommunityIds: []string{existingID},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	// ShareItem always finds/provisions the item's per-item community and returns
	// it, even when only sharing to existing communities (#2492).
	if resp.Msg.AdhocCommunityId == nil || *resp.Msg.AdhocCommunityId == "" {
		t.Fatal("expected the item's per-item community id")
	}
	adhocID := *resp.Msg.AdhocCommunityId
	// The item is shared into both its per-item community and the requested
	// existing community.
	if len(sharer.sharedTo) != 2 {
		t.Fatalf("expected shares to {per-item, existing}, got %v", sharer.sharedTo)
	}
	gotShared := map[string]bool{}
	for _, c := range sharer.sharedTo {
		gotShared[c] = true
	}
	if !gotShared[adhocID] {
		t.Errorf("expected share to per-item community %q, got %v", adhocID, sharer.sharedTo)
	}
	if !gotShared[existingID] {
		t.Errorf("expected share to existing community %q, got %v", existingID, sharer.sharedTo)
	}
	_ = st
}

// TestShareItem_NonMemberTargetCommunityRejected covers the membership gate on
// share_to_community_ids: an owner may only push their item into communities
// they actually belong to, and a target they are not a member of fails the whole
// call rather than being silently skipped.
func TestShareItem_NonMemberTargetCommunityRejected(t *testing.T) {
	svc, st, sharer, _, _, expID, ctx := setupShareItemTest(t)

	// A community owned by somebody else, which the host is not a member of.
	strangerID := setupTestUser(t, st, "stranger@example.com", "Stranger")
	strangerCtx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)
	createResp, err := svc.CreateCommunity(strangerCtx, connect.NewRequest(&api.CreateCommunityRequest{Name: "Closed"}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}

	_, err = svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                expTarget(expID),
		ShareToCommunityIds: []string{createResp.Msg.Id},
	}))
	if err == nil {
		t.Fatal("expected an error sharing into a community the caller isn't a member of")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
	for _, c := range sharer.sharedTo {
		if c == createResp.Msg.Id {
			t.Errorf("item must not be shared into the non-member community %q", c)
		}
	}
}

func TestShareItem_MixedAudienceAndExistingCommunity(t *testing.T) {
	svc, st, sharer, emailer, _, expID, ctx := setupShareItemTest(t)
	memberID := setupTestUser(t, st, "m2@example.com", "M2")
	createResp, err := svc.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{Name: "Crew"}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}
	existingID := createResp.Msg.Id

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item: expTarget(expID),
		Invitees: []*api.Invitee{
			memberInvitee(memberID),
			phoneInvitee("+15550000001", "Phone Pal"),
			emailInvitee("e2@example.com", "Email Pal"),
		},
		InviteConsent:       &api.HostInviteConsent{Attested: true},
		ShareToCommunityIds: []string{existingID},
	}))
	if err != nil {
		t.Fatalf("ShareItem: %v", err)
	}
	adhocID := *resp.Msg.AdhocCommunityId

	// Two provisionals (phone + email); one email dispatched.
	if ps := provisionalsIn(t, st, adhocID); len(ps) != 2 {
		t.Errorf("expected 2 provisionals, got %d", len(ps))
	}
	if len(emailer.sent) != 1 {
		t.Errorf("expected 1 email, got %d", len(emailer.sent))
	}
	// Shared to both the ad-hoc community and the existing one.
	sharedSet := map[string]bool{}
	for _, c := range sharer.sharedTo {
		sharedSet[c] = true
	}
	if !sharedSet[adhocID] || !sharedSet[existingID] {
		t.Errorf("expected shares to ad-hoc + existing, got %v", sharer.sharedTo)
	}
}

func TestShareItem_UnsupportedItemKind(t *testing.T) {
	svc, _, _, _, _, _, ctx := setupShareItemTest(t)
	// No gear sharer registered.
	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     &api.ShareItemRequest_GearId{GearId: "gear-x"},
		Invitees: []*api.Invitee{memberInvitee("someone")},
	}))
	if err == nil || connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented for an unwired item kind, got %v", err)
	}
}

func TestShareItem_OwnerVerificationFails(t *testing.T) {
	svc, _, sharer, _, _, expID, ctx := setupShareItemTest(t)
	sharer.verifyOwnerErr = connect.NewError(connect.CodePermissionDenied, context.DeadlineExceeded)

	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:     expTarget(expID),
		Invitees: []*api.Invitee{memberInvitee("x")},
	}))
	if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestShareItem_NoTarget(t *testing.T) {
	svc, _, _, _, _, _, ctx := setupShareItemTest(t)
	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{}))
	if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument for missing target, got %v", err)
	}
}

func TestShareItem_LinkOnlyMemberGetsShareLink(t *testing.T) {
	svc, st, sharer, _, hostID, expID, _ := setupShareItemTest(t)
	viewerID := setupTestUser(t, st, "viewer@example.com", "Viewer")
	viewerCtx := createAuthenticatedContext(viewerID, "viewer@example.com", models.Role_ROLE_USER)

	resp, err := svc.ShareItem(viewerCtx, connect.NewRequest(&api.ShareItemRequest{
		Item: expTarget(expID),
	}))
	if err != nil {
		t.Fatalf("ShareItem (link-only, viewer): %v", err)
	}
	if !sharer.verifiedViewer {
		t.Error("expected the viewer check to run on a link-only call")
	}
	if sharer.verifiedOwner {
		t.Error("owner check must not run on a link-only call")
	}
	if resp.Msg.ShareUrl == nil || *resp.Msg.ShareUrl == "" {
		t.Fatal("expected a share url for the viewer")
	}
	if resp.Msg.CanManageAudience == nil || *resp.Msg.CanManageAudience {
		t.Errorf("expected can_manage_audience=false for a non-owner, got %v", resp.Msg.CanManageAudience)
	}
	adhocID := *resp.Msg.AdhocCommunityId

	// The lazily-provisioned per-item community belongs to the item's owner,
	// never the resharing viewer.
	adhoc := &models.Community{}
	if err := st.GetByID(viewerCtx, adhocID, adhoc); err != nil {
		t.Fatalf("get ad-hoc community: %v", err)
	}
	if adhoc.OwnerUserId != hostID {
		t.Errorf("per-item community owner = %q, want item owner %q", adhoc.OwnerUserId, hostID)
	}

	// A link-only call adds no members: the host (seeded at provisioning) is
	// the only one; the viewer is not implicitly joined.
	members, err := st.QueryByField(viewerCtx, "community_id", adhocID, &models.CommunityUser{})
	if err != nil {
		t.Fatalf("query members: %v", err)
	}
	if len(members) != 1 || members[0].(*models.CommunityUser).UserId != hostID {
		t.Errorf("expected only the host as member, got %d rows", len(members))
	}

	// The minted link is the viewer's own (per-inviter links → revocable by them).
	links, err := st.QueryByFields(viewerCtx, map[string]any{
		"inviter_id":    viewerID,
		"experience_id": expID,
	}, &models.ShareLink{})
	if err != nil {
		t.Fatalf("query share links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 share link minted for the viewer, got %d", len(links))
	}
}

func TestShareItem_LinkOnlyOwnerCanManageAudience(t *testing.T) {
	svc, _, sharer, _, _, expID, ctx := setupShareItemTest(t)

	resp, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item: expTarget(expID),
	}))
	if err != nil {
		t.Fatalf("ShareItem (link-only, owner): %v", err)
	}
	if !sharer.verifiedViewer || sharer.verifiedOwner {
		t.Error("link-only calls go through the viewer check, even for the owner")
	}
	if resp.Msg.CanManageAudience == nil || !*resp.Msg.CanManageAudience {
		t.Errorf("expected can_manage_audience=true for the owner, got %v", resp.Msg.CanManageAudience)
	}
}

func TestShareItem_LinkOnlyViewerDenied(t *testing.T) {
	svc, _, sharer, _, _, expID, ctx := setupShareItemTest(t)
	sharer.verifyViewerErr = connect.NewError(connect.CodePermissionDenied, context.DeadlineExceeded)

	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item: expTarget(expID),
	}))
	if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a caller without view access, got %v", err)
	}
}

func TestShareItem_ShareToCommunitiesRequiresOwner(t *testing.T) {
	svc, _, sharer, _, _, expID, ctx := setupShareItemTest(t)
	sharer.verifyOwnerErr = connect.NewError(connect.CodePermissionDenied, context.DeadlineExceeded)

	_, err := svc.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                expTarget(expID),
		ShareToCommunityIds: []string{"some-community"},
	}))
	if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a non-owner sharing to communities, got %v", err)
	}
	if sharer.verifiedViewer {
		t.Error("viewer check must not substitute for the owner check on a mutating call")
	}
}
