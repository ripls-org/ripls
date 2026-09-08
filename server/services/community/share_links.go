package community

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// lookupShareLink finds ShareLink rows by short_code. Returns an empty
// slice (not an error) when no row matches; returns an error only when
// the storage call itself fails.
func (s *Service) lookupShareLink(ctx context.Context, shortCode string) ([]*models.ShareLink, error) {
	results, err := s.storage.QueryByField(ctx, "short_code", shortCode, &models.ShareLink{})
	if err != nil {
		return nil, err
	}
	links := make([]*models.ShareLink, len(results))
	for i, r := range results {
		links[i] = r.(*models.ShareLink)
	}
	return links, nil
}

// shareLinkTargetSpec captures the variant-specific bits of the
// polymorphic GetOrCreateShareLink RPC. It's filled in by
// parseShareLinkTarget below and consumed by the generic body.
type shareLinkTargetSpec struct {
	// opLog is the per-variant operation name used in log lines and
	// connecterr.Internal calls — improves grep-ability in logs.
	opLog string

	// targetField is the SQL column that scopes the "active link for
	// this (inviter, target)" lookup.
	targetField string
	targetID    string

	// applyTarget assigns the storage-model oneof variant onto the
	// passed ShareLink row. Kept as a closure because the oneof
	// wrapper interface (isShareLink_Target) is unexported by the
	// generated code, so callers outside this package can't return
	// it from a factory.
	applyTarget func(*models.ShareLink)

	// verify runs the variant-specific preconditions (entity exists,
	// is in the same community, etc.). Runs after the community-
	// membership gate.
	verify func(ctx context.Context, s *Service, communityID string) error
}

// parseShareLinkTarget validates the request's oneof and returns the
// variant-specific spec. Returns an InvalidArgument Connect error if
// the oneof is empty, or Unimplemented for variants that don't have
// a server-side implementation yet.
func parseShareLinkTarget(req *api.GetOrCreateShareLinkRequest) (*shareLinkTargetSpec, error) {
	switch {
	case req.GetCommunityInvite() != "":
		return &shareLinkTargetSpec{
			opLog:       "GetOrCreateShareLink.community_invite",
			targetField: "community_invite_id",
			targetID:    req.GetCommunityInvite(),
			applyTarget: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_CommunityInviteId{
					CommunityInviteId: req.GetCommunityInvite(),
				}
			},
			verify: func(_ context.Context, _ *Service, communityID string) error {
				// community_invite must reference the same community
				// the link is scoped to.
				if req.GetCommunityInvite() != communityID {
					return connect.NewError(connect.CodeInvalidArgument,
						fmt.Errorf("community_invite must equal community_id"))
				}
				return nil
			},
		}, nil
	case req.GetExperienceId() != "":
		return &shareLinkTargetSpec{
			opLog:       "GetOrCreateShareLink.experience",
			targetField: "experience_id",
			targetID:    req.GetExperienceId(),
			applyTarget: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_ExperienceId{
					ExperienceId: req.GetExperienceId(),
				}
			},
			verify: func(ctx context.Context, s *Service, communityID string) error {
				exp := &models.Experience{}
				if err := s.storage.GetByID(ctx, req.GetExperienceId(), exp); err != nil {
					return connect.NewError(connect.CodeNotFound,
						fmt.Errorf("event not found"))
				}
				rows, err := s.storage.QueryByFields(ctx, map[string]any{
					"experience_id": req.GetExperienceId(),
					"community_id":  communityID,
				}, &models.CommunityExperience{})
				if err != nil {
					return connecterr.Internal(ctx, "GetOrCreateShareLink.experience.verify", err)
				}
				if len(rows) == 0 {
					return connect.NewError(connect.CodeFailedPrecondition,
						fmt.Errorf("event is not shared with this community"))
				}
				return nil
			},
		}, nil
	case req.GetGearId() != "":
		return &shareLinkTargetSpec{
			opLog:       "GetOrCreateShareLink.gear",
			targetField: "gear_id",
			targetID:    req.GetGearId(),
			applyTarget: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_GearId{GearId: req.GetGearId()}
			},
			verify: func(ctx context.Context, s *Service, communityID string) error {
				gear := &models.Gear{}
				if err := s.storage.GetByID(ctx, req.GetGearId(), gear); err != nil {
					return connect.NewError(connect.CodeNotFound,
						fmt.Errorf("gear not found"))
				}
				rows, err := s.storage.QueryByFields(ctx, map[string]any{
					"gear_id":      req.GetGearId(),
					"community_id": communityID,
				}, &models.CommunityGear{})
				if err != nil {
					return connecterr.Internal(ctx, "GetOrCreateShareLink.gear.verify", err)
				}
				if len(rows) == 0 {
					return connect.NewError(connect.CodeFailedPrecondition,
						fmt.Errorf("gear is not shared with this community"))
				}
				return nil
			},
		}, nil
	case req.GetTransferId() != "":
		return &shareLinkTargetSpec{
			opLog:       "GetOrCreateShareLink.transfer",
			targetField: "transfer_id",
			targetID:    req.GetTransferId(),
			applyTarget: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_TransferId{TransferId: req.GetTransferId()}
			},
			verify: func(ctx context.Context, s *Service, communityID string) error {
				transfer := &models.Transfer{}
				if err := s.storage.GetByID(ctx, req.GetTransferId(), transfer); err != nil {
					return connect.NewError(connect.CodeNotFound,
						fmt.Errorf("transfer not found"))
				}
				// Transfer carries community_id directly — no join lookup
				// needed.
				if transfer.CommunityId != communityID {
					return connect.NewError(connect.CodeFailedPrecondition,
						fmt.Errorf("transfer is not in this community"))
				}
				return nil
			},
		}, nil
	case req.GetRequestId() != "":
		return &shareLinkTargetSpec{
			opLog:       "GetOrCreateShareLink.request",
			targetField: "request_id",
			targetID:    req.GetRequestId(),
			applyTarget: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_RequestId{RequestId: req.GetRequestId()}
			},
			verify: func(ctx context.Context, s *Service, communityID string) error {
				request := &models.Request{}
				if err := s.storage.GetByID(ctx, req.GetRequestId(), request); err != nil {
					return connect.NewError(connect.CodeNotFound,
						fmt.Errorf("request not found"))
				}
				rows, err := s.storage.QueryByFields(ctx, map[string]any{
					"request_id":   req.GetRequestId(),
					"community_id": communityID,
				}, &models.CommunityRequest{})
				if err != nil {
					return connecterr.Internal(ctx, "GetOrCreateShareLink.request.verify", err)
				}
				if len(rows) == 0 {
					return connect.NewError(connect.CodeFailedPrecondition,
						fmt.Errorf("request is not shared with this community"))
				}
				return nil
			},
		}, nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("target must be set"))
	}
}

// resolveScopingCommunity returns the community id that a ShareLink row should
// be scoped to. For item-target specs (experience, gear, request, transfer) it
// resolves the item's per-item ad-hoc origin community via
// communitylib.FindAdHocByOrigin; for the community_invite variant it returns
// callerCommunityID unchanged.
//
// On a benign miss (no ad-hoc community found — legacy items created before
// #2492), usedFallback=true is returned with callerCommunityID so old items
// still produce a working link. A real storage error returns
// connecterr.Internal — never a silent fallback per
// docs/server/conventions.md §Error Handling.
func (s *Service) resolveScopingCommunity(
	ctx context.Context,
	spec *shareLinkTargetSpec,
	callerCommunityID string,
) (resolvedID string, usedFallback bool, err error) {
	var origin communitylib.Origin
	switch spec.targetField {
	case "experience_id":
		origin = communitylib.Origin{ExperienceID: spec.targetID}
	case "gear_id":
		origin = communitylib.Origin{GearID: spec.targetID}
	case "request_id":
		origin = communitylib.Origin{RequestID: spec.targetID}
	case "transfer_id":
		origin = communitylib.Origin{TransferID: spec.targetID}
	default:
		// community_invite variant — scope is always the caller-supplied community.
		return callerCommunityID, false, nil
	}

	adHocID, found, lookupErr := communitylib.FindAdHocByOrigin(ctx, s.storage, origin)
	if lookupErr != nil {
		return "", false, connecterr.Internal(ctx, spec.opLog, lookupErr)
	}
	if !found {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"no ad-hoc origin community for item; falling back to caller community (legacy item)",
			"operation", spec.opLog,
			spec.targetField, spec.targetID,
			"caller_community_id", callerCommunityID,
		)
		return callerCommunityID, true, nil
	}
	return adHocID, false, nil
}

// GetOrCreateShareLink returns the active share link for
// (caller, target), creating one if it doesn't already exist. The
// target oneof on the request identifies what the link points at;
// each variant runs its own validation before the generic
// query-or-insert body.
//
// Item-target links (gear, experience, request) are always scoped to the
// item's ad-hoc origin community, regardless of the caller-supplied
// community_id. The community_id field is used only for caller-membership auth
// (the caller must be a member of the supplied active community); the row's
// scope is resolved to the ad-hoc. The response's community_id reflects the
// resolved ad-hoc id.
func (s *Service) GetOrCreateShareLink(
	ctx context.Context,
	req *connect.Request[api.GetOrCreateShareLinkRequest],
) (*connect.Response[api.GetOrCreateShareLinkResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Auth gate: caller must be a member of the supplied community. For item
	// targets the row is then re-scoped to the item's ad-hoc origin community
	// below, so the supplied community_id is used only for membership auth —
	// not for the link scope.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	spec, err := parseShareLinkTarget(req.Msg)
	if err != nil {
		return nil, err
	}

	resolvedCommunityID, usedFallback, err := s.resolveScopingCommunity(ctx, spec, req.Msg.CommunityId)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", spec.opLog,
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
		"resolved_community_id", resolvedCommunityID,
		"used_fallback", usedFallback,
		spec.targetField, spec.targetID,
	)

	if err := spec.verify(ctx, s, resolvedCommunityID); err != nil {
		return nil, err
	}

	shortCode, err := s.getOrCreateShareLink(ctx, resolvedCommunityID, authInfo.UserID, spec)
	if err != nil {
		return nil, err
	}

	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, resolvedCommunityID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get member count", "error", err)
		return nil, connecterr.Internal(ctx, spec.opLog, err)
	}

	shareURL := fmt.Sprintf("%s/go/%s", baseURL(s.inviteLinkHostname), shortCode)
	return connect.NewResponse(&api.GetOrCreateShareLinkResponse{
		ShareUrl:    shareURL,
		ShortCode:   shortCode,
		CommunityId: resolvedCommunityID,
		NumMembers:  int32(memberCount),
		MaxMembers:  int32(communitylib.MaxCommunityMembers),
	}), nil
}

// getOrCreateShareLink returns the short code of the active (non-revoked) share
// link for (inviterID, target) within communityID, creating one if none exists.
// The caller is responsible for auth, community membership, and running
// spec.verify. Shared by the GetOrCreateShareLink RPC and ShareItem.
func (s *Service) getOrCreateShareLink(ctx context.Context, communityID, inviterID string, spec *shareLinkTargetSpec) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", spec.opLog,
		"community_id", communityID,
		spec.targetField, spec.targetID,
	)

	existing, err := s.storage.QueryByFields(ctx, map[string]any{
		"inviter_id":     inviterID,
		spec.targetField: spec.targetID,
		"is_revoked":     false,
	}, &models.ShareLink{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing share link", "error", err)
		return "", connecterr.Internal(ctx, spec.opLog, err)
	}

	if len(existing) > 0 {
		link := existing[0].(*models.ShareLink)
		shortCode := link.ShortCode

		// Old records created during the proto field-4 reuse window may carry a
		// UUID-shaped short_code instead of a proper 8-char code. Detect by
		// length and regenerate in place so returned URLs are always short.
		if len(shortCode) != shortCodeLength {
			newCode, backfillErr := s.generateUniqueShortCode(ctx)
			if backfillErr != nil {
				return "", connecterr.Internal(ctx, spec.opLog, backfillErr)
			}
			link.ShortCode = newCode
			if updateErr := s.storage.Update(ctx, link); updateErr != nil {
				logger.ErrorContext(ctx, "failed to backfill stale short code", "error", updateErr)
				return "", connecterr.Internal(ctx, spec.opLog, updateErr)
			}
			logger.InfoContext(ctx, "backfilled stale invitation short code",
				"old_code_len", len(shortCode),
				"new_short_code", newCode,
			)
			shortCode = newCode
		}

		// If the cached row was scoped to a different community (pre-fix, a
		// named-community link), re-scope it to the resolved (ad-hoc) community.
		// The short code is stable, so already-shared URLs self-heal without
		// changing the link the inviter distributed.
		if link.CommunityId != communityID {
			oldCommunityID := link.CommunityId
			link.CommunityId = communityID
			if updateErr := s.storage.Update(ctx, link); updateErr != nil {
				logger.ErrorContext(ctx, "failed to self-heal share link community", "error", updateErr)
				return "", connecterr.Internal(ctx, spec.opLog, updateErr)
			}
			logger.InfoContext(ctx, "self-healed share link community to ad-hoc",
				"old_community_id", oldCommunityID,
				"new_community_id", communityID,
			)
		}

		return shortCode, nil
	}

	shortCode, err := s.generateUniqueShortCode(ctx)
	if err != nil {
		return "", connecterr.Internal(ctx, spec.opLog, err)
	}
	link := &models.ShareLink{
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        shortCode,
		IsRevoked:        false,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}
	spec.applyTarget(link)
	if _, err := s.storage.Insert(ctx, link); err != nil {
		logger.ErrorContext(ctx, "failed to insert share link", "error", err)
		return "", connecterr.Internal(ctx, spec.opLog, err)
	}
	logger.InfoContext(ctx, "created new share link")
	return shortCode, nil
}

// ShortLinkCodeForEntity returns the short code of a non-revoked `/go` share
// link that lands on the given entity (experience, gear, or request) within
// communityID, find-or-creating one keyed on the resolved community's owner.
// Experience wins over gear over request when more than one id is set. With no
// entity id it falls back to a community-invite link (lands on the community
// itself) — so community-wide notifications (a member joined, a community-level
// update) still deep-link somewhere useful instead of the bare app URL.
//
// Item-target links are always scoped to the item's ad-hoc origin community
// (resolved via FindAdHocByOrigin), so the notification link's preview never
// leaks a spurious named community. communityID is used only as the scope for
// community-invite fallback links (no entity set).
//
// It is the server-internal seam the off-app notification channel uses to put a
// short, legible `/go` link in SMS and email instead of a long entity-ID URL.
// Because the recipient is already a member (they are being notified about this
// community), the link's open-join is a no-op for them, and for a per-item
// ad-hoc community the owner is the host who already minted this link, so the
// find-or-create reuses it. Performs no auth (callers are server-internal) and
// skips the RPC's "entity is shared with this community" verify — the
// notification's community already references this entity.
func (s *Service) ShortLinkCodeForEntity(ctx context.Context, communityID, experienceID, gearID, requestID string) (string, error) {
	req := &api.GetOrCreateShareLinkRequest{CommunityId: communityID}
	switch {
	case experienceID != "":
		req.Target = &api.GetOrCreateShareLinkRequest_ExperienceId{ExperienceId: experienceID}
	case gearID != "":
		req.Target = &api.GetOrCreateShareLinkRequest_GearId{GearId: gearID}
	case requestID != "":
		req.Target = &api.GetOrCreateShareLinkRequest_RequestId{RequestId: requestID}
	default:
		// No item entity — land on the community itself (e.g. a member-joined
		// "say hi" notification). The recipient is already a member, so the
		// open-join is a no-op and the link just opens the community.
		req.Target = &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID}
	}

	spec, err := parseShareLinkTarget(req)
	if err != nil {
		return "", err
	}

	resolvedCommunityID, _, err := s.resolveScopingCommunity(ctx, spec, communityID)
	if err != nil {
		return "", err
	}

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, resolvedCommunityID, community); err != nil {
		return "", fmt.Errorf("load community %q for share link: %w", resolvedCommunityID, err)
	}
	return s.getOrCreateShareLink(ctx, resolvedCommunityID, community.GetOwnerUserId(), spec)
}

// targetKindForLog returns a short string identifying the variant of the
// ShareLink's Target oneof, used in structured log fields.
func targetKindForLog(link *models.ShareLink) string {
	switch {
	case link.GetCommunityInviteId() != "":
		return "community_invite"
	case link.GetGearId() != "":
		return "gear"
	case link.GetTransferId() != "":
		return "transfer"
	case link.GetRequestId() != "":
		return "request"
	case link.GetExperienceId() != "":
		return "experience"
	default:
		return "unknown"
	}
}

// RevokeShareLink revokes a shareable link by its short code. The caller must
// be the link's inviter. Idempotent — revoking an already-revoked link returns
// success without re-writing the row.
func (s *Service) RevokeShareLink(
	ctx context.Context,
	req *connect.Request[api.RevokeShareLinkRequest],
) (*connect.Response[api.RevokeShareLinkResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	links, err := s.lookupShareLink(ctx, req.Msg.ShortCode)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RevokeShareLink", err)
	}
	if len(links) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no active share link found"))
	}

	link := links[0]
	if link.InviterId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not the link's inviter"))
	}

	if link.IsRevoked {
		return connect.NewResponse(&api.RevokeShareLinkResponse{}), nil
	}

	link.IsRevoked = true
	if err := s.storage.Update(ctx, link); err != nil {
		return nil, connecterr.Internal(ctx, "RevokeShareLink", err)
	}

	logging.LoggerWithContext(ctx).InfoContext(ctx, "revoked share link",
		"user_id", authInfo.UserID,
		"community_id", link.CommunityId,
		"target_kind", targetKindForLog(link),
		"short_code", logging.MaskToken(link.ShortCode),
	)
	return connect.NewResponse(&api.RevokeShareLinkResponse{}), nil
}

// requireAcceptableViaCommunityJoin returns an InvalidArgument Connect
// error for share links that cannot be accepted by the
// "join the community" flow. Every populated target variant
// (community_invite, gear, transfer, request, event) shares
// community-join semantics on accept — the typed target is preview
// metadata that the client uses for post-accept navigation. Event
// share links additionally require an RSVP write, which the client
// fires separately via RSVPToExperience after this RPC succeeds (see
// docs/issues/2050-web-rsvp-actions.md §D1).
func requireAcceptableViaCommunityJoin(link *models.ShareLink) error {
	if link.GetCommunityInviteId() == "" &&
		link.GetGearId() == "" &&
		link.GetTransferId() == "" &&
		link.GetRequestId() == "" &&
		link.GetExperienceId() == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("share link has no target variant set"))
	}
	return nil
}
