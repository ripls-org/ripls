package community

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/contact"
	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// maxInviteesPerShare bounds how many people a single ShareItem call may invite.
// It caps the off-app fan-out (an SMS-pumping / email-abuse surface) per call;
// hosts inviting larger audiences share the open link instead.
const maxInviteesPerShare = 50

// ItemKind identifies the item type an audience is being provisioned for.
type ItemKind string

const (
	ItemKindExperience ItemKind = "experience"
	ItemKindGear       ItemKind = "gear"
	ItemKindRequest    ItemKind = "request"
	ItemKindTransfer   ItemKind = "transfer"
)

// ItemSharer binds the per-item-type operations ShareItem needs: verifying the
// caller may manage the item's audience, and sharing the item with a community.
// Provided as method values during wiring (SetItemSharer) so the community
// service doesn't import the item-service packages.
type ItemSharer struct {
	// VerifyOwner returns an error unless actorUserID may manage the item's
	// audience (typically: owns the item).
	VerifyOwner func(ctx context.Context, itemID, actorUserID string) error
	// VerifyViewer returns the item's owner ID, or an error unless actorUserID
	// has view access to the item (the owner, or an active member of a
	// community the item is shared with). Gates the link-only ShareItem path —
	// any member may reshare the item's open link (#2630).
	VerifyViewer func(ctx context.Context, itemID, actorUserID string) (string, error)
	// ShareToCommunity shares the item with communityID, idempotently.
	ShareToCommunity func(ctx context.Context, itemID, communityID, actorUserID string) error
	// UnshareFromCommunity removes the item from communityID. It is
	// self-contained: it performs its own owner check (with the item type's own
	// "not the owner" message) and preserves that type's removal semantics — soft
	// vs hard delete of the junction, the request last-community guard, and any
	// audience event (gear emits GEAR_UNSHARED). Implements
	// CommunityService.UnshareItem for this item kind. May be nil for kinds that
	// don't support removal (e.g. transfer), in which case UnshareItem returns
	// Unimplemented.
	UnshareFromCommunity func(ctx context.Context, itemID, communityID, actorUserID string) error
}

// InviteEmailSender sends a single off-app invite email. Injected via
// SetInviteEmailSender (the off-app email channel); nil disables platform email
// invites (e.g. in tests or when the channel is off), in which case email
// invitees still join the ad-hoc community and can be reached via the open link.
type InviteEmailSender interface {
	SendInviteEmail(ctx context.Context, in email.InviteEmailInput) error
}

// SetInviteEmailSender wires the off-app invite email sender (optional).
func (s *Service) SetInviteEmailSender(sender InviteEmailSender) {
	s.inviteEmailSender = sender
}

// ShareItem adds to an item's per-item community: it invites individuals
// (provisional phone/email members + host-relay) and/or shares the item to
// additional existing communities, and returns the per-item community + its open
// link. The per-item community itself is provisioned at item creation (#2492) —
// ShareItem finds it by origin (and provisions-if-missing as a fallback for
// legacy items), it does NOT mint a fresh ad-hoc community per call. Phone
// invitees are host-relayed by the client via the returned share_url.
func (s *Service) ShareItem(
	ctx context.Context,
	req *connect.Request[api.ShareItemRequest],
) (*connect.Response[api.ShareItemResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	kind, itemID, origin, err := parseShareItemTarget(req.Msg)
	if err != nil {
		return nil, err
	}

	sharer, ok := s.itemSharers[kind]
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("sharing %s items is not supported yet", kind))
	}

	// Tiered permissions (#2630): mutating the audience (inviting individuals
	// or sharing to more communities) requires the item's owner; a link-only
	// call — what the share sheet sends on open — is allowed for anyone with
	// view access, so any member can reshare the item's open link.
	mutatesAudience := len(req.Msg.GetInvitees()) > 0 || len(req.Msg.GetShareToCommunityIds()) > 0
	// itemOwnerID attributes provisioning and the origin-community share to the
	// item's owner even when a non-owner member triggers them lazily.
	itemOwnerID := authInfo.UserID
	if mutatesAudience {
		if err := sharer.VerifyOwner(ctx, itemID, authInfo.UserID); err != nil {
			return nil, err
		}
	} else {
		if sharer.VerifyViewer == nil {
			return nil, connect.NewError(connect.CodeUnimplemented,
				fmt.Errorf("link resharing is not supported for %s items", kind))
		}
		ownerID, err := sharer.VerifyViewer(ctx, itemID, authInfo.UserID)
		if err != nil {
			return nil, err
		}
		itemOwnerID = ownerID
	}
	callerIsOwner := itemOwnerID == authInfo.UserID

	// Classify and normalize invitees.
	invitees := req.Msg.GetInvitees()
	if len(invitees) > maxInviteesPerShare {
		return nil, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
			"too_many_invitees", "too many people invited at once", nil)
	}

	classified, err := classifyInvitees(ctx, invitees)
	if err != nil {
		return nil, err
	}

	// Attestation is required before inviting phone numbers.
	if classified.hasPhone {
		c := req.Msg.GetInviteConsent()
		if c == nil || !c.GetAttested() {
			return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition,
				"invite_consent_required",
				"please confirm you know the people you're texting", nil)
		}
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ShareItem",
		"user_id", authInfo.UserID,
		"item_kind", string(kind),
		"item_id", itemID,
		"caller_is_owner", callerIsOwner,
		"member_invitees", len(classified.memberUserIDs),
		"phone_invitees", len(classified.phoneHandles),
		"email_invitees", len(classified.emailHandles),
	)

	// Find the item's per-item community (provisioned at creation) via the shared
	// community library. The provision-if-missing fallback covers gear (whose
	// community is provisioned lazily here) and any legacy items predating #2492.
	// Provisioning and the origin share are attributed to the item's owner — a
	// non-owner resharer must never become host of the item's origin community.
	adhocID, err := communitylib.ProvisionPerItemCommunity(ctx, s.storage, s.bus, itemOwnerID, origin.toLib())
	if err != nil {
		return nil, connecterr.Internal(ctx, "ShareItem", err)
	}
	// Ensure the item is shared into it — idempotent (a no-op for items already
	// shared; shares gear / legacy items on first ShareItem).
	if err := s.shareItemViaSharer(ctx, kind, itemID, adhocID, itemOwnerID); err != nil {
		return nil, err
	}

	resp := &api.ShareItemResponse{
		AdhocCommunityId:  &adhocID,
		CanManageAudience: &callerIsOwner,
	}

	// Mint the open share link for the item within the per-item community (host
	// relay + QR). Always returned so the client has a link to copy/share.
	shareURL, err := s.mintItemShareLink(ctx, adhocID, authInfo.UserID, kind, itemID)
	if err != nil {
		return nil, err
	}
	resp.ShareUrl = &shareURL

	// Attach invited individuals (real-user members + provisional phone/email
	// members + consent + email).
	if classified.hasIndividuals() {
		if err := s.attachInvitedAudience(ctx, authInfo.UserID, adhocID, origin, classified, req.Msg.GetInviteConsent(), shareURL); err != nil {
			return nil, err
		}
	}

	// Additionally share with selected existing communities (membership-gated).
	for _, cid := range req.Msg.GetShareToCommunityIds() {
		if cid == "" {
			continue
		}
		if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, cid, authInfo.UserID); err != nil {
			return nil, err
		}
		if err := sharer.ShareToCommunity(ctx, itemID, cid, authInfo.UserID); err != nil {
			return nil, err
		}
	}

	logger.InfoContext(ctx, "shared item", "adhoc_community_id", adhocID)
	return connect.NewResponse(resp), nil
}

// shareItemViaSharer shares an item into a community using the registered
// ItemSharer (experience / gear / request). Auth is the caller's responsibility.
func (s *Service) shareItemViaSharer(ctx context.Context, kind ItemKind, itemID, communityID, actorUserID string) error {
	sharer, ok := s.itemSharers[kind]
	if !ok {
		return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("no item sharer registered for %s", kind))
	}
	return sharer.ShareToCommunity(ctx, itemID, communityID, actorUserID)
}

// attachInvitedAudience adds invited individuals to the item's per-item
// community: seeds real-user members, attaches a provisional member per off-app
// contact handle, persists the host's consent, and sends email invites
// (best-effort; phone is host-relayed via shareURL). The community itself and the
// item-share into it are handled by the caller (ShareItem).
func (s *Service) attachInvitedAudience(
	ctx context.Context,
	hostUserID string,
	communityID string,
	origin AdHocOrigin,
	classified classifiedInvitees,
	consent *api.HostInviteConsent,
	shareURL string,
) error {
	// Seed any real-user invitees as members (idempotent).
	if err := s.addMembersToCommunity(ctx, communityID, hostUserID, classified.memberUserIDs); err != nil {
		return err
	}

	// Attach a provisional member per off-app contact handle.
	provisionals := make([]*models.ProvisionalUser, 0, len(classified.contacts))
	for _, ci := range classified.contacts {
		name := ci.name
		if name == "" {
			name = defaultProvisionalName(ci.phone, ci.email)
		}
		prov, perr := s.createOrGetProvisionalUser(ctx, communityID, name, ci.phone, ci.email, hostUserID)
		if perr != nil {
			return perr
		}
		provisionals = append(provisionals, prov)
	}

	// Persist the host's consent — the compliance artifact exists only for
	// off-app invites (member-only invites need no attestation).
	if len(classified.contacts) > 0 {
		if err := s.persistInviteConsent(ctx, hostUserID, communityID, origin, classified.modelInvitees, consent); err != nil {
			return err
		}
	}

	// Send email invites immediately (best-effort; phone is host-relayed).
	s.dispatchEmailInvites(ctx, provisionals, shareURL, hostUserID)
	return nil
}

// addMembersToCommunity idempotently seeds real-user invitees as members of the
// community, skipping the inviter and anyone already an active member. A
// soft-deleted membership (e.g. the person was removed from the item's event
// earlier) is restored rather than re-inserted, which would otherwise collide
// on the (community_id, user_id) uniqueness and fail the whole share.
func (s *Service) addMembersToCommunity(ctx context.Context, communityID, inviterID string, memberUserIDs []string) error {
	now := clock.UnixSec(ctx)
	for _, uid := range memberUserIDs {
		if uid == "" || uid == inviterID {
			continue
		}
		// Read any existing row, including soft-deleted ones, so a re-invite
		// after a removal restores the membership instead of duplicating it.
		rows, err := s.storage.QueryByFields(ctx, map[string]any{
			"community_id": communityID,
			"user_id":      uid,
		}, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			return connecterr.Internal(ctx, "addMembersToCommunity", err)
		}
		if len(rows) > 0 {
			cu := rows[0].(*models.CommunityUser)
			if cu.Deleted == nil || cu.Deleted.DeletedAtUnixSec == 0 {
				continue // already an active member
			}
			// Restore the soft-deleted membership. A zero-valued (non-nil)
			// DeletedMetadata is required so Update writes the flattened
			// deleted_* columns back to 0 — a nil sub-message is skipped by the
			// column extractor, leaving the row soft-deleted.
			cu.Deleted = &models.DeletedMetadata{}
			cu.InviterId = inviterID
			cu.CreatedAtUnixSec = now
			if err := s.storage.Update(ctx, cu); err != nil {
				return connecterr.Internal(ctx, "addMembersToCommunity", err)
			}
			continue
		}
		if _, err := s.storage.Insert(ctx, &models.CommunityUser{
			CommunityId:      communityID,
			UserId:           uid,
			InviterId:        inviterID,
			CreatedAtUnixSec: now,
		}); err != nil {
			return connecterr.Internal(ctx, "addMembersToCommunity", err)
		}
	}
	return nil
}

// contactInvitee is one normalized off-app invitee (exactly one of phone/email).
type contactInvitee struct {
	phone string
	email string
	name  string
}

// classifiedInvitees is the normalized, partitioned invitee set.
type classifiedInvitees struct {
	memberUserIDs []string
	phoneHandles  []string
	emailHandles  []string
	contacts      []contactInvitee
	// modelInvitees mirrors the invitees with normalized handles, for the
	// persisted consent record.
	modelInvitees []*models.Invitee
	hasPhone      bool
}

func (c classifiedInvitees) hasIndividuals() bool {
	return len(c.memberUserIDs) > 0 || len(c.contacts) > 0
}

// classifyInvitees normalizes and partitions the invitee list into members and
// off-app contacts, rejecting malformed handles.
func classifyInvitees(ctx context.Context, invitees []*api.Invitee) (classifiedInvitees, error) {
	var c classifiedInvitees
	for _, inv := range invitees {
		switch inv.GetIdentity().(type) {
		case *api.Invitee_MemberUserId:
			uid := inv.GetMemberUserId()
			if uid == "" {
				continue
			}
			c.memberUserIDs = append(c.memberUserIDs, uid)
			c.modelInvitees = append(c.modelInvitees, &models.Invitee{
				Identity: &models.Invitee_MemberUserId{MemberUserId: uid},
			})
		case *api.Invitee_PhoneNumber:
			normalized, nerr := contact.NormalizePhoneE164(inv.GetPhoneNumber())
			if nerr != nil {
				return c, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
					"invalid_invitee_phone", "one of the phone numbers is invalid", nil)
			}
			c.hasPhone = true
			c.phoneHandles = append(c.phoneHandles, normalized)
			c.contacts = append(c.contacts, contactInvitee{phone: normalized, name: inv.GetDisplayName()})
			c.modelInvitees = append(c.modelInvitees, normalizedModelInvitee(normalized, "", inv.GetDisplayName()))
		case *api.Invitee_Email:
			email := auth.NormalizeEmail(inv.GetEmail())
			if !looksLikeEmail(email) {
				return c, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
					"invalid_invitee_email", "one of the email addresses is invalid", nil)
			}
			c.emailHandles = append(c.emailHandles, email)
			c.contacts = append(c.contacts, contactInvitee{email: email, name: inv.GetDisplayName()})
			c.modelInvitees = append(c.modelInvitees, normalizedModelInvitee("", email, inv.GetDisplayName()))
		}
	}
	return c, nil
}

func normalizedModelInvitee(phone, email, displayName string) *models.Invitee {
	m := &models.Invitee{}
	switch {
	case phone != "":
		m.Identity = &models.Invitee_PhoneNumber{PhoneNumber: phone}
	case email != "":
		m.Identity = &models.Invitee_Email{Email: email}
	}
	if displayName != "" {
		m.DisplayName = &displayName
	}
	return m
}

// parseShareItemTarget validates the request's item oneof and returns the item
// kind, id, and the matching ad-hoc origin.
func parseShareItemTarget(req *api.ShareItemRequest) (ItemKind, string, AdHocOrigin, error) {
	switch {
	case req.GetExperienceId() != "":
		id := req.GetExperienceId()
		return ItemKindExperience, id, AdHocOrigin{ExperienceID: id}, nil
	case req.GetGearId() != "":
		id := req.GetGearId()
		return ItemKindGear, id, AdHocOrigin{GearID: id}, nil
	case req.GetRequestId() != "":
		id := req.GetRequestId()
		return ItemKindRequest, id, AdHocOrigin{RequestID: id}, nil
	case req.GetTransferId() != "":
		id := req.GetTransferId()
		return ItemKindTransfer, id, AdHocOrigin{TransferID: id}, nil
	default:
		return "", "", AdHocOrigin{}, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item target must be set"))
	}
}

// mintItemShareLink returns the open share URL for the item target within the
// community, reusing the GetOrCreateShareLink core. The item must already be
// shared with the community (spec.verify enforces this).
func (s *Service) mintItemShareLink(ctx context.Context, communityID, inviterID string, kind ItemKind, itemID string) (string, error) {
	linkReq := &api.GetOrCreateShareLinkRequest{CommunityId: communityID}
	switch kind {
	case ItemKindExperience:
		linkReq.Target = &api.GetOrCreateShareLinkRequest_ExperienceId{ExperienceId: itemID}
	case ItemKindGear:
		linkReq.Target = &api.GetOrCreateShareLinkRequest_GearId{GearId: itemID}
	case ItemKindRequest:
		linkReq.Target = &api.GetOrCreateShareLinkRequest_RequestId{RequestId: itemID}
	case ItemKindTransfer:
		linkReq.Target = &api.GetOrCreateShareLinkRequest_TransferId{TransferId: itemID}
	}

	spec, err := parseShareLinkTarget(linkReq)
	if err != nil {
		return "", err
	}
	if err := spec.verify(ctx, s, communityID); err != nil {
		return "", err
	}
	shortCode, err := s.getOrCreateShareLink(ctx, communityID, inviterID, spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/go/%s", baseURL(s.inviteLinkHostname), shortCode), nil
}

// persistInviteConsent writes the durable audit record of the host's attestation
// for this invite send.
func (s *Service) persistInviteConsent(
	ctx context.Context,
	hostUserID, communityID string,
	origin AdHocOrigin,
	modelInvitees []*models.Invitee,
	consent *api.HostInviteConsent,
) error {
	rec := &models.InviteConsent{
		HostUserId:       hostUserID,
		CommunityId:      communityID,
		Invitees:         modelInvitees,
		Consent:          apiHostConsentToModels(consent),
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}
	origin.applyToInviteConsent(rec)

	if locale := l10n.LocaleFromContext(ctx).String(); locale != "" && locale != "und" {
		rec.AcceptLanguage = &locale
	}

	if _, err := s.storage.Insert(ctx, rec); err != nil {
		return connecterr.Internal(ctx, "persistInviteConsent", err)
	}
	return nil
}

// dispatchEmailInvites sends an invite email to each email-handle provisional,
// best-effort. Phone provisionals are host-relayed by the client and are skipped
// here.
func (s *Service) dispatchEmailInvites(ctx context.Context, provisionals []*models.ProvisionalUser, shareURL, hostUserID string) {
	if s.inviteEmailSender == nil {
		return
	}
	logger := logging.LoggerWithContext(ctx)
	hostName := s.getUserDisplayName(ctx, hostUserID)
	preferredLanguage := l10n.LocaleFromContext(ctx).String()

	for _, p := range provisionals {
		addr := p.GetEmail()
		if addr == "" {
			continue
		}
		err := s.inviteEmailSender.SendInviteEmail(ctx, email.InviteEmailInput{
			ToEmail:           addr,
			RecipientName:     p.Name,
			HostName:          hostName,
			ShareURL:          shareURL,
			PreferredLanguage: preferredLanguage,
		})
		if err != nil {
			logger.WarnContext(ctx, "failed to send invite email",
				"error", err, "to", logging.MaskEmail(addr))
			continue
		}
		logger.InfoContext(ctx, "sent invite email", "to", logging.MaskEmail(addr))
	}
}

// apiHostConsentToModels converts the API consent message to its storage mirror.
func apiHostConsentToModels(c *api.HostInviteConsent) *models.HostInviteConsent {
	if c == nil {
		return nil
	}
	m := &models.HostInviteConsent{Attested: c.GetAttested()}
	if c.AttestedAtUnixSec != nil {
		v := c.GetAttestedAtUnixSec()
		m.AttestedAtUnixSec = &v
	}
	return m
}

// applyToInviteConsent sets the InviteConsent.origin_item oneof from the origin.
func (o AdHocOrigin) applyToInviteConsent(c *models.InviteConsent) {
	switch {
	case o.ExperienceID != "":
		c.OriginItem = &models.InviteConsent_OriginExperienceId{OriginExperienceId: o.ExperienceID}
	case o.GearID != "":
		c.OriginItem = &models.InviteConsent_OriginGearId{OriginGearId: o.GearID}
	case o.RequestID != "":
		c.OriginItem = &models.InviteConsent_OriginRequestId{OriginRequestId: o.RequestID}
	case o.TransferID != "":
		c.OriginItem = &models.InviteConsent_OriginTransferId{OriginTransferId: o.TransferID}
	}
}

// defaultProvisionalName derives a fallback display name for an off-app invitee
// when the client supplied none. The client normally provides a name from the
// contact picker or manual entry.
func defaultProvisionalName(phone, email string) string {
	if email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at]
		}
		return email
	}
	if len(phone) >= 4 {
		return "Guest " + phone[len(phone)-4:]
	}
	return "Guest"
}

// looksLikeEmail is a light structural check mirroring CreateProvisionalUser.
func looksLikeEmail(email string) bool {
	return strings.Contains(email, "@") && !strings.HasPrefix(email, "@") && !strings.HasSuffix(email, "@")
}
