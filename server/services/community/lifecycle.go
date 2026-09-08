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
	"go.ripls.org/ripls/server/storage"
)

// CreateCommunity creates a new community.
func (s *Service) CreateCommunity(
	ctx context.Context,
	req *connect.Request[api.CreateCommunityRequest],
) (*connect.Response[api.CreateCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "creating community", "community_name", req.Msg.Name)

	// Named, user-initiated community: no origin item. Shared creation core
	// (also used by ProvisionAdHocCommunity) seeds membership, regions, the
	// creation event, and the community conversation.
	id, err := s.createCommunity(ctx, newCommunityParams{
		name:         req.Msg.Name,
		description:  req.Msg.Description,
		ownerUserID:  authInfo.UserID,
		mediaIDs:     req.Msg.MediaIds,
		simulationID: req.Msg.SimulationId,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to create community", "error", err)
		return nil, err
	}

	return connect.NewResponse(&api.CreateCommunityResponse{
		Id: id,
	}), nil
}

// UpdateCommunity updates a community.
func (s *Service) UpdateCommunity(
	ctx context.Context,
	req *connect.Request[api.UpdateCommunityRequest],
) (*connect.Response[api.UpdateCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.Id,
	)

	logger.InfoContext(ctx, "updating community")

	// Single round-trip: fetch community + verify membership + reject deleted.
	community, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.Id, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// An unnamed (ad-hoc) community gaining a name is a promotion to a real
	// persistent community (#2492). Capture the pre-update state before the
	// name is overwritten so we can emit COMMUNITY_NAMED once below.
	wasAdHoc := IsAdHoc(community)

	// Update fields
	if req.Msg.Name != "" {
		community.Name = req.Msg.Name
	}
	if req.Msg.Description != "" {
		community.Description = req.Msg.Description
	}
	if len(req.Msg.MediaIds) > 0 {
		community.MediaIds = req.Msg.MediaIds
	}

	// Update timestamp
	community.UpdatedAtUnixSec = clock.UnixSec(ctx)

	// Update in database
	err = s.storage.Update(ctx, community)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update community", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateCommunity", err)
	}

	// Emit the promotion event only on the first unnamed→named transition.
	if wasAdHoc && community.Name != "" {
		logger.InfoContext(ctx, "promoted ad-hoc community to named", "community_name", community.Name)
		if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
			CommunityId: community.Id,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED,
			ActorId:     authInfo.UserID,
		}); err != nil {
			logger.ErrorContext(ctx, "failed to record community-named event", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateCommunity", err, "detail", "failed to record community-named event")
		}
	}

	return connect.NewResponse(&api.UpdateCommunityResponse{}), nil
}

// DeleteCommunity soft-deletes a community. Owner-only. Snapshots the
// active member set as the eligible-restorer set, soft-deletes the join
// rows (gear, requests, experiences, notification preferences, invitation
// links), and emits a COMMUNITY_DELETED event so the dispatcher
// notifies all snapshot members. Reversible within 30 days via
// RestoreCommunity. See docs/community_delete_and_leave.md §2.1, §6.2.
func (s *Service) DeleteCommunity(
	ctx context.Context,
	req *connect.Request[api.DeleteCommunityRequest],
) (*connect.Response[api.DeleteCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteCommunity",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.Id,
	)

	logger.InfoContext(ctx, "deleting community")

	// Fetch the community with IncludeDeleted so a double-tap from a
	// stale UI surfaces as a no-op success rather than NotFound.
	community := &models.Community{}
	err = s.storage.GetByID(ctx, req.Msg.Id, community, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to get community for deletion", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Owner-only. owner_user_id is populated for every active row by
	// #1644's backfill + CHECK constraint; reading it directly is safe.
	if community.OwnerUserId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "community_owner_required_for_delete", "only the owner can delete this community", nil)
	}

	// Already-deleted is a no-op success rather than an error so a
	// double-tap from a stale UI doesn't surface as a failure.
	if community.Deleted != nil && community.Deleted.DeletedAtUnixSec > 0 {
		logger.InfoContext(ctx, "community already deleted; no-op")
		return connect.NewResponse(&api.DeleteCommunityResponse{}), nil
	}

	if err := s.deleteCommunityInternal(ctx, logger, community, authInfo.UserID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.DeleteCommunityResponse{}), nil
}

// deleteCommunityInternal runs the full §2.1 soft-delete sequence
// against an already-fetched, already-authorized community: snapshot
// active members, flip Deleted + DeletedSnapshot in a single Update,
// run the §6.6 cascade across the join rows, and emit
// COMMUNITY_DELETED. Caller is responsible for the owner-only check
// and the already-deleted no-op early-out — so this helper can be
// shared by DeleteCommunity (RPC entry point) and the
// sole-member-leave branch of LeaveCommunity (which has already
// fetched the community and resolved the actor).
//
// Returns connect-coded errors so callers can return them directly.
func (s *Service) deleteCommunityInternal(
	ctx context.Context, logger *logging.Logger,
	community *models.Community, actorUserID string,
) error {
	// Snapshot the active member set BEFORE flipping deleted, since the
	// read filter would otherwise hide the membership rows. This is the
	// eligible-restorer set per §2.5: any user who was a CommunityUser
	// at the moment of deletion may restore.
	members, err := s.storage.QueryByField(ctx, "community_id", community.Id, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community members for snapshot", "error", err)
		return connecterr.Internal(ctx, "DeleteCommunity", err)
	}
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, m.(*models.CommunityUser).UserId)
	}

	now := clock.UnixSec(ctx)
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  actorUserID,
		DeletedAtUnixSec: now,
	}

	community.Deleted = deletedMetadata
	community.DeletedSnapshot = &models.CommunityDeletedSnapshot{
		MemberUserIds: memberIDs,
	}
	// storage.Update auto-syncs the snapshot_member_user_ids TEXT[]
	// column from community.DeletedSnapshot.MemberUserIds inside the
	// same transaction as the proto write — the indexed restore-list
	// surface (#1722) reflects the snapshot atomically with the
	// delete itself. See server/storage/community_schema.go.
	if err := s.storage.Update(ctx, community); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete community", "error", err)
		return connecterr.Internal(ctx, "DeleteCommunity", err)
	}

	cascades := []struct {
		name string
		fn   func(context.Context, *storage.ProtoSQLStorage, string, *models.DeletedMetadata) error
	}{
		{"community_gear", storage.CascadeDeleteCommunityGearByCommunityID},
		{"community_request", storage.CascadeDeleteCommunityRequestByCommunityID},
		{"community_experience", storage.CascadeDeleteCommunityExperienceByCommunityID},
		{"community_notification_preferences", storage.CascadeDeleteCommunityNotificationPreferencesByCommunityID},
		{"community_invitation_link", storage.CascadeDeleteCommunityInvitationLinkByCommunityID},
		{"share_link", storage.CascadeDeleteShareLinksByCommunityID},
	}
	for _, c := range cascades {
		if err := c.fn(ctx, s.storage, community.Id, deletedMetadata); err != nil {
			logger.WarnContext(
				ctx, "cascade soft-delete failed; continuing",
				"cascade_target", c.name,
				"error", err,
			)
		}
	}

	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: community.Id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
		ActorId:     actorUserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record community delete event", "error", err)
		return connecterr.Internal(ctx, "DeleteCommunity", err, "detail", "failed to record community delete event")
	}

	logger.InfoContext(ctx, "community deleted successfully", "snapshot_member_count", len(memberIDs))
	return nil
}

// RestoreCommunity un-soft-deletes a community within the 30-day
// window. Caller must be in the deleted_snapshot.member_user_ids set
// captured at delete time; on success the caller becomes the new
// owner regardless of who deleted it. Cascade rows that the
// community-delete cascade (#1654) soft-deleted are restored too;
// rows that were soft-deleted earlier (e.g. a member's leave) stay
// soft-deleted per design doc §5. See
// docs/community_delete_and_leave.md §2.5, §6.2.
func (s *Service) RestoreCommunity(
	ctx context.Context,
	req *connect.Request[api.RestoreCommunityRequest],
) (*connect.Response[api.RestoreCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)
	logger.InfoContext(ctx, "restoring community")

	community := &models.Community{}
	err = s.storage.GetByID(ctx, req.Msg.CommunityId, community, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to get community for restore", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if community.Deleted == nil || community.Deleted.DeletedAtUnixSec == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("community is not in deleted state"))
	}

	// Eligibility: caller must be in the snapshot.
	snapshot := community.GetDeletedSnapshot()
	inSnapshot := false
	if snapshot != nil {
		for _, uid := range snapshot.MemberUserIds {
			if uid == authInfo.UserID {
				inSnapshot = true
				break
			}
		}
	}
	if !inSnapshot {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("caller is not in the eligible-restorer snapshot"))
	}

	// Capture the cascade-restore conditions before we clear them.
	communityDeletedBy := community.Deleted.DeletedByUserId
	communityDeletedAt := community.Deleted.DeletedAtUnixSec

	// Race-safe claim: clear the flat deleted_* columns atomically.
	// Returns false if another restorer beat us OR if the row is
	// already in a half-restored state from a prior crashed attempt.
	claimed, err := s.storage.ClaimCommunityRestore(ctx, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim community restore", "error", err)
		return nil, connecterr.Internal(ctx, "RestoreCommunity", err)
	}
	if !claimed {
		// Distinguish race-lost from crash-window. Probe the flat
		// column directly: if it's clear AND our binary_proto read
		// said deleted, this is the half-state we should recover.
		// Otherwise it's a normal race-lost.
		flatDeletedBy, found, probeErr := s.storage.ProbeCommunityFlatDeleted(ctx, req.Msg.CommunityId)
		if probeErr != nil {
			logger.ErrorContext(ctx, "failed to probe flat deleted state", "error", probeErr)
			return nil, connecterr.Internal(ctx, "RestoreCommunity", probeErr)
		}
		if !found {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("community %s not found", req.Msg.CommunityId))
		}
		if flatDeletedBy != "" {
			// Flat column is still marked deleted but ClaimCommunityRestore
			// returned false — should be impossible. Defensive fallthrough.
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("community state inconsistent; retry"))
		}
		// Flat column clear, proto says deleted: crash-window state.
		// Fall through to resync.
		logger.WarnContext(ctx, "detected mid-restore inconsistency; recovering",
			"reason", "flat_clear_proto_deleted")
	}

	// Resync the in-memory community to the restored state and write
	// it back. This rewrites binary_proto so future reads see the
	// active state in both layers.
	community.Deleted = nil
	community.DeletedSnapshot = nil
	// Clear the day-before-purge reminder marker so a delete →
	// restore → delete cycle starts a fresh reminder window.
	// See #1659.
	community.PurgeReminderSentAtUnixSec = nil
	community.OwnerUserId = authInfo.UserID
	community.UpdatedAtUnixSec = clock.UnixSec(ctx)
	if err := s.storage.Update(ctx, community); err != nil {
		// Flat column already cleared — this leaves the row in the
		// half-state. Logged at WARN; the recovery branch above
		// handles the next attempt.
		logger.WarnContext(ctx, "claim succeeded but resync update failed; row in half-state",
			"error", err)
		return nil, connecterr.Internal(ctx, "RestoreCommunity", err)
	}

	// Cascade-restore the join rows soft-deleted by the same
	// community-delete cascade. Best-effort: log per-helper failures
	// at WARN and continue.
	cascades := []struct {
		name string
		fn   func(context.Context, *storage.ProtoSQLStorage, string, string, int64) error
	}{
		{"community_gear", storage.CascadeRestoreCommunityGearByCommunityID},
		{"community_request", storage.CascadeRestoreCommunityRequestByCommunityID},
		{"community_experience", storage.CascadeRestoreCommunityExperienceByCommunityID},
		{"community_notification_preferences", storage.CascadeRestoreCommunityNotificationPreferencesByCommunityID},
		{"community_invitation_link", storage.CascadeRestoreCommunityInvitationLinkByCommunityID},
		{"share_link", storage.CascadeRestoreShareLinksByCommunityID},
	}
	for _, c := range cascades {
		if err := c.fn(ctx, s.storage, req.Msg.CommunityId, communityDeletedBy, communityDeletedAt); err != nil {
			logger.WarnContext(
				ctx, "cascade restore failed; continuing",
				"cascade_target", c.name,
				"error", err,
			)
		}
	}

	// Ensure the restorer has an active CommunityUser. The most
	// common case after #1654's cascade (which doesn't touch
	// CommunityUser) is that an active row already exists. The other
	// branches are for symmetry with the future #1656 leave path.
	if err := s.ensureRestorerMembership(ctx, logger, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, connecterr.Internal(ctx, "RestoreCommunity", err)
	}

	// Emit COMMUNITY_RESTORED. By now the community is fully active
	// in both layers, so the dispatcher's IsActive guard sees the
	// active state naturally — no firesForDeletedCommunity exception
	// needed.
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: req.Msg.CommunityId,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED,
		ActorId:     authInfo.UserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record community restore event", "error", err)
		return nil, connecterr.Internal(ctx, "RestoreCommunity", err, "detail", "failed to record community restore event")
	}

	logger.InfoContext(ctx, "community restored successfully")
	return connect.NewResponse(&api.RestoreCommunityResponse{}), nil
}

// ensureRestorerMembership makes sure the restoring user has an
// active CommunityUser row in the just-restored community. Idempotent
// across the three possible starting states: active row already
// present, soft-deleted row from a future #1656 leave, or missing row
// entirely.
func (s *Service) ensureRestorerMembership(ctx context.Context, logger *logging.Logger, communityID, userID string) error {
	rows, err := s.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return fmt.Errorf("query CommunityUser for restorer: %w", err)
	}
	for _, m := range rows {
		cu := m.(*models.CommunityUser)
		if cu.UserId != userID {
			continue
		}
		if cu.Deleted == nil || cu.Deleted.DeletedAtUnixSec == 0 {
			// Active row already exists; nothing to do.
			return nil
		}
		// Un-soft-delete the existing row.
		cu.Deleted = nil
		if err := s.storage.Update(ctx, cu); err != nil {
			return fmt.Errorf("un-soft-delete restorer CommunityUser: %w", err)
		}
		logger.InfoContext(ctx, "un-soft-deleted restorer CommunityUser", "membership_id", cu.Id)
		return nil
	}
	// No row exists — insert a fresh one.
	now := clock.UnixSec(ctx)
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        userID,
		CreatedAtUnixSec: now,
	}
	if _, err := s.storage.Insert(ctx, membership); err != nil {
		return fmt.Errorf("insert restorer CommunityUser: %w", err)
	}
	logger.InfoContext(ctx, "inserted fresh restorer CommunityUser")
	return nil
}

// LeaveCommunity removes the caller from a community. Per
// docs/community_delete_and_leave.md §2.2 / §2.3 / §2.4, the call
// branches on the caller's relationship to the community:
//
//   - Non-owner with co-members: cleanup §4 cascade, soft-delete the
//     leaver's CommunityUser row (preserving the 30-day rejoin
//     window), emit MEMBER_LEFT, recompute regions.
//   - Owner with co-members: req.NewOwnerUserId must point to a
//     current active member; ownership transfers atomically before
//     the leaver row soft-deletes. (See #1656.)
//   - Sole member: short-circuits to the §2.4 conversion that
//     soft-deletes the entire community via deleteCommunityInternal.
//     (See #1656.)
func (s *Service) LeaveCommunity(
	ctx context.Context,
	req *connect.Request[api.LeaveCommunityRequest],
) (*connect.Response[api.LeaveCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "LeaveCommunity",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)
	logger.InfoContext(ctx, "user leaving community")

	// Read with IncludeDeleted so we can return a deterministic
	// FailedPrecondition for "leave a deleted community" rather than
	// NotFound (which would be wrong — the caller may have just been
	// looking at an already-deleted community in their settings list).
	community := &models.Community{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityId, community, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		logger.ErrorContext(ctx, "failed to get community", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if community.Deleted != nil && community.Deleted.DeletedAtUnixSec > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("community is in deleted state; nothing to leave"))
	}

	// Active membership for the caller. Soft-deleted memberships are
	// not active membership for the leave path — they belong to the
	// rejoin path (#1657).
	leaverMembership, err := auth.GetCommunityUser(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query membership", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}
	if leaverMembership == nil {
		return nil, connecterr.UserVisible(ctx, connect.CodeNotFound, "community_not_member", "you're not a member of this community", nil)
	}

	// Branch on sole-member / owner / non-owner. Sole-member trumps
	// the owner branch because a sole-member-leave can't transfer
	// ownership — there's no candidate.
	activeMembers, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to count active members", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}
	if len(activeMembers) == 1 {
		return s.leaveAsSoleMember(ctx, logger, community, authInfo.UserID)
	}
	if community.OwnerUserId == authInfo.UserID {
		return s.leaveAsOwner(ctx, logger, community, authInfo.UserID, req.Msg.GetNewOwnerUserId(), leaverMembership)
	}
	return s.leaveAsNonOwner(ctx, logger, community, authInfo.UserID, leaverMembership)
}

// leaveAsNonOwner runs the §2.2 non-owner leave: cascade the §4
// cleanup, soft-delete the leaver's CommunityUser row, emit
// MEMBER_LEFT, and recompute regions. Best-effort cascade — a failure
// in any single helper logs at WARN and continues, mirroring the
// #1654 DeleteCommunity pattern.
func (s *Service) leaveAsNonOwner(
	ctx context.Context, logger *logging.Logger,
	community *models.Community, leaverUserID string,
	leaverMembership *models.CommunityUser,
) (*connect.Response[api.LeaveCommunityResponse], error) {
	now := clock.UnixSec(ctx)
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  leaverUserID,
		DeletedAtUnixSec: now,
	}

	s.runLeaverCascade(ctx, logger, community.Id, leaverUserID, deletedMetadata)

	// Soft-delete the leaver's CommunityUser row last. Doing it after
	// the cascade means a partial cascade failure leaves the
	// membership row active, which is the right invariant for retry
	// — the leaver can re-tap Leave and the cascade picks up where
	// it left off (idempotent helpers).
	leaverMembership.Deleted = deletedMetadata
	if err := s.storage.Update(ctx, leaverMembership); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete leaver membership", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}

	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: community.Id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT,
		ActorId:     leaverUserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record member-left event", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}

	if err := s.recomputeCommunityRegions(ctx, community.Id); err != nil {
		logger.ErrorContext(ctx, "failed to recompute community regions", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err, "detail", "failed to recompute community regions")
	}

	logger.InfoContext(ctx, "user left community successfully")
	return connect.NewResponse(&api.LeaveCommunityResponse{}), nil
}

// leaveAsOwner runs the §2.3 owner-leave-with-handoff branch:
// validate the candidate, race-safely transfer ownership, resync the
// proto layer, run the §4 cascade, soft-delete the leaver row, then
// emit OWNERSHIP_TRANSFERRED followed by MEMBER_LEFT in that order
// so the audit log reads "ownership moved → previous owner left".
//
// Atomicity comes from ClaimCommunityOwnerHandoff (conditional UPDATE
// with active-membership EXISTS subselect), not from a SQL transaction
// — see the helper's doc for the race semantics. If the claim fails
// we follow up with a probe of the candidate's membership to
// distinguish "ownership_changed" (someone else already moved it) from
// "candidate_not_member" (the candidate left between client list and
// server commit).
func (s *Service) leaveAsOwner(
	ctx context.Context, logger *logging.Logger,
	community *models.Community, leaverUserID, newOwnerUserID string,
	leaverMembership *models.CommunityUser,
) (*connect.Response[api.LeaveCommunityResponse], error) {
	if newOwnerUserID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("owner-leave requires new_owner_user_id"))
	}
	if newOwnerUserID == leaverUserID {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("new_owner_user_id must differ from the caller"))
	}
	logger = logger.With("new_owner_user_id", newOwnerUserID)

	// Pre-flight membership check: gives a clean FailedPrecondition
	// for the common case (candidate not a member) before we touch
	// the conditional UPDATE. Race-safety still relies on the
	// EXISTS subselect inside ClaimCommunityOwnerHandoff.
	candidate, err := auth.GetCommunityUser(ctx, s.storage, community.Id, newOwnerUserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query candidate membership", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}
	if candidate == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("candidate is not an active member: candidate_not_member"))
	}

	claimed, err := s.storage.ClaimCommunityOwnerHandoff(ctx, community.Id, leaverUserID, newOwnerUserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim owner handoff", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}
	if !claimed {
		// Distinguish race-lost categories with a follow-up read.
		// If the candidate's membership disappeared between the
		// pre-flight check and the claim, "candidate_not_member"; if
		// the candidate is still a member, the leaver must have been
		// preempted by another owner-action ("ownership_changed").
		probe, probeErr := auth.GetCommunityUser(ctx, s.storage, community.Id, newOwnerUserID)
		if probeErr != nil {
			logger.ErrorContext(ctx, "failed to probe candidate after claim loss", "error", probeErr)
			return nil, connecterr.Internal(ctx, "LeaveCommunity", probeErr)
		}
		if probe == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("candidate is not an active member: candidate_not_member"))
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("ownership has changed since the request was issued: ownership_changed"))
	}

	// Resync binary_proto so future GetByID reads see the new owner
	// in both layers. ClaimCommunityOwnerHandoff only touches the
	// flat owner_user_id column. Failure here leaves the row in a
	// half-state (flat says new owner, proto says old) — caller can
	// retry safely because ClaimCommunityOwnerHandoff is idempotent
	// (second call returns false, ownership_changed) and the
	// follow-up read converges both layers.
	community.OwnerUserId = newOwnerUserID
	community.UpdatedAtUnixSec = clock.UnixSec(ctx)
	if err := s.storage.Update(ctx, community); err != nil {
		logger.WarnContext(ctx, "claim succeeded but resync update failed; row in half-state",
			"error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}

	now := clock.UnixSec(ctx)
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  leaverUserID,
		DeletedAtUnixSec: now,
	}
	s.runLeaverCascade(ctx, logger, community.Id, leaverUserID, deletedMetadata)

	leaverMembership.Deleted = deletedMetadata
	if err := s.storage.Update(ctx, leaverMembership); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete leaver membership", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}

	// Emit OWNERSHIP_TRANSFERRED first so the audit log reads
	// left-to-right: ownership moved, then the previous owner left.
	// The notification dispatcher routes this event to ObjectUserId
	// (the new owner) only.
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId:  community.Id,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED,
		ActorId:      leaverUserID,
		ObjectUserId: newOwnerUserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record ownership-transferred event", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err, "detail", "failed to record ownership-transferred event")
	}

	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: community.Id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT,
		ActorId:     leaverUserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record member-left event", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err)
	}

	if err := s.recomputeCommunityRegions(ctx, community.Id); err != nil {
		logger.ErrorContext(ctx, "failed to recompute community regions", "error", err)
		return nil, connecterr.Internal(ctx, "LeaveCommunity", err, "detail", "failed to recompute community regions")
	}

	logger.InfoContext(ctx, "owner left community with ownership handoff")
	return connect.NewResponse(&api.LeaveCommunityResponse{}), nil
}

// leaveAsSoleMember runs the §2.4 conversion: when the leaver is the
// only active member, "leave" is an alias for "delete" with the
// leaver as the actor. The community soft-deletes with the leaver as
// the sole entry in deleted_snapshot.member_user_ids, so the §2.5
// restore path remains open to them within the 30-day window.
//
// Reuses deleteCommunityInternal to keep the
// snapshot+cascade+event sequence in lockstep with #1654's path.
// Per the design doc:
//   - The audit event is COMMUNITY_DELETED, not MEMBER_LEFT.
//   - The leaver's CommunityUser row stays active under the soft-deleted
//     community — symmetric with #1654's behavior, and necessary so the
//     restore path can re-establish ownership for the same row.
//   - No region recompute: the community is gone from active queries
//     and will be hard-purged or restored later. CommunityRegion rows
//     are part of the eventual hard-delete cascade (#1620), not the
//     soft-delete cascade.
func (s *Service) leaveAsSoleMember(
	ctx context.Context, logger *logging.Logger,
	community *models.Community, leaverUserID string,
) (*connect.Response[api.LeaveCommunityResponse], error) {
	logger.InfoContext(ctx, "sole-member leave; converting to community soft-delete")
	if err := s.deleteCommunityInternal(ctx, logger, community, leaverUserID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.LeaveCommunityResponse{}), nil
}

// runLeaverCascade fans out the six leaver+community-scoped cascade
// helpers in cascade_leaver.go. Each is best-effort: failures are
// logged at WARN with a cascade_target field and the loop continues.
// The helpers are idempotent so a retry (or a follow-up operation
// that walks the cascade) closes any gap left by a transient
// failure.
func (s *Service) runLeaverCascade(
	ctx context.Context, logger *logging.Logger,
	communityID, leaverUserID string, deletedMetadata *models.DeletedMetadata,
) {
	// Each row has a name for log correlation and a closure that
	// uniformly accepts (ctx, communityID, leaverUserID,
	// deletedMetadata) to keep the loop body uniform. The transfer
	// helper has no DeletedMetadata parameter (it transitions State,
	// not Deleted), so it ignores the argument.
	cascades := []struct {
		name string
		fn   func(context.Context, string, string, *models.DeletedMetadata) error
	}{
		{"community_gear", func(c context.Context, cid, uid string, m *models.DeletedMetadata) error {
			return storage.CascadeDeleteCommunityGearByOwnerInCommunity(c, s.storage, cid, uid, m)
		}},
		{"community_request", func(c context.Context, cid, uid string, m *models.DeletedMetadata) error {
			return storage.CancelLeaverCommunityRequestsInCommunity(c, s.storage, cid, uid, m)
		}},
		{"transfer", func(c context.Context, cid, uid string, _ *models.DeletedMetadata) error {
			return storage.CancelLeaverActiveTransfersInCommunity(c, s.storage, cid, uid)
		}},
		{"experience_rsvp", func(c context.Context, cid, uid string, m *models.DeletedMetadata) error {
			return storage.DropLeaverExperienceRSVPsInCommunity(c, s.storage, cid, uid, m)
		}},
		{"community_notification_preferences", func(c context.Context, cid, uid string, m *models.DeletedMetadata) error {
			return storage.DeleteLeaverCommunityNotificationPreferences(c, s.storage, cid, uid, m)
		}},
		{"community_invitation_link", func(c context.Context, cid, uid string, m *models.DeletedMetadata) error {
			return storage.RevokeLeaverCommunityInvitationLinks(c, s.storage, cid, uid, m)
		}},
		// chat_conversation_participant strips the leaver from ParticipantIds on
		// every gear/experience/request/community-wide conversation in this
		// community, provided the leaver has no remaining active membership in
		// any other community the topic is shared with. Transfer-topic
		// conversations are exempt (intentional escape hatch).
		// The helper is idempotent; the WARN-and-continue pattern here is the
		// documented exception per the cascade header comment — a transient
		// failure leaves the participant list stale but the read-path fix in
		// requireConversationAccess already denies ex-member access, so the
		// failure is self-healing on the next leave/rejoin cycle or a manual
		// re-run. The helper ignores DeletedMetadata; it only strips participants.
		{"chat_conversation_participant", func(c context.Context, cid, uid string, _ *models.DeletedMetadata) error {
			return storage.CascadeRemoveLeaverFromChatConversationsInCommunity(c, s.storage, cid, uid)
		}},
	}
	for _, c := range cascades {
		if err := c.fn(ctx, communityID, leaverUserID, deletedMetadata); err != nil {
			logger.WarnContext(
				ctx, "leaver cascade failed; continuing",
				"cascade_target", c.name,
				"error", err,
			)
		}
	}
}

// RejoinCommunity un-soft-deletes the caller's CommunityUser row in
// a community when they left within the past 30 days. Per
// docs/community_delete_and_leave.md §2.6 / §2.7 this is the inverse
// of the non-owner branch of LeaveCommunity (#1656). The call
// validates the §2.7 cross-cut explicitly: a community in deleted
// state blocks rejoin even within an individual rejoin window.
//
// Negative paths surface as connect.CodeFailedPrecondition with a
// distinct reason in the error string:
//   - community_deleted: the community itself is soft-deleted
//   - no_membership_to_rejoin: caller never had a row here
//   - rejoin_window_expired: caller's soft-delete is older than 30 days
//   - already_member: caller is already an active member
//
// Atomicity comes from ClaimCommunityUserRejoin (conditional UPDATE
// with the window check pushed into SQL). On claim loss a probe
// disambiguates the three race-loss reasons. The handler emits
// MEMBER_REJOINED_WITHIN_WINDOW for the audit trail; per design §8
// no push notification fires.
func (s *Service) RejoinCommunity(
	ctx context.Context,
	req *connect.Request[api.RejoinCommunityRequest],
) (*connect.Response[api.RejoinCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RejoinCommunity",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)
	logger.InfoContext(ctx, "user rejoining community")

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, req.Msg.CommunityId, community, storage.QueryOptions{IncludeDeleted: true}); err != nil {
		logger.ErrorContext(ctx, "failed to get community", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	// §2.7: rejoin clock does not override the community's deleted
	// state. Even within the per-user 30-day window, the user cannot
	// rejoin a community in deleted state — only an eligible member
	// can restore it (#1655).
	if community.Deleted != nil && community.Deleted.DeletedAtUnixSec > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("community is deleted: community_deleted"))
	}

	// Pre-flight membership read. IncludeDeleted=true so we can see
	// the soft-deleted row; the branch below classifies the four
	// negative-path states.
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": req.Msg.CommunityId,
		"user_id":      authInfo.UserID,
	}, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query membership", "error", err)
		return nil, connecterr.Internal(ctx, "RejoinCommunity", err)
	}
	var existing *models.CommunityUser
	if len(rows) > 0 {
		existing = rows[0].(*models.CommunityUser)
	}
	if existing == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no prior membership in this community: no_membership_to_rejoin"))
	}
	if existing.Deleted == nil || existing.Deleted.DeletedAtUnixSec == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("caller is already an active member: already_member"))
	}
	now := clock.UnixSec(ctx)
	if now-existing.Deleted.DeletedAtUnixSec > communitylib.RejoinWindowSeconds {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("rejoin window expired: rejoin_window_expired"))
	}

	claimed, err := s.storage.ClaimCommunityUserRejoin(ctx, req.Msg.CommunityId, authInfo.UserID, now, communitylib.RejoinWindowSeconds)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim community-user rejoin", "error", err)
		return nil, connecterr.Internal(ctx, "RejoinCommunity", err)
	}
	if !claimed {
		// Disambiguate the three race-loss reasons via a flat read.
		probeDeletedAt, found, probeErr := s.storage.ProbeCommunityUserDeletedAt(ctx, req.Msg.CommunityId, authInfo.UserID)
		if probeErr != nil {
			logger.ErrorContext(ctx, "failed to probe community-user state after claim loss", "error", probeErr)
			return nil, connecterr.Internal(ctx, "RejoinCommunity", probeErr)
		}
		if !found {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("no prior membership in this community: no_membership_to_rejoin"))
		}
		if probeDeletedAt == 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("caller is already an active member: already_member"))
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("rejoin window expired: rejoin_window_expired"))
	}

	// Resync the in-memory CommunityUser so binary_proto agrees with
	// the flat columns. created_at_unix_sec is intentionally
	// preserved — that field is the historical "first joined at"
	// timestamp; rejoin does not reset it.
	existing.Deleted = nil
	if err := s.storage.Update(ctx, existing); err != nil {
		logger.WarnContext(ctx, "claim succeeded but resync update failed; row in half-state", "error", err)
		return nil, connecterr.Internal(ctx, "RejoinCommunity", err)
	}

	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: req.Msg.CommunityId,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW,
		ActorId:     authInfo.UserID,
	}); err != nil {
		logger.ErrorContext(ctx, "failed to record member-rejoined event", "error", err)
		return nil, connecterr.Internal(ctx, "RejoinCommunity", err, "detail", "failed to record member-rejoined event")
	}

	if err := s.recomputeCommunityRegions(ctx, req.Msg.CommunityId); err != nil {
		logger.ErrorContext(ctx, "failed to recompute community regions", "error", err)
		return nil, connecterr.Internal(ctx, "RejoinCommunity", err, "detail", "failed to recompute community regions")
	}

	logger.InfoContext(ctx, "user rejoined community successfully")
	return connect.NewResponse(&api.RejoinCommunityResponse{}), nil
}

// GetCommunity retrieves a community by ID.
func (s *Service) GetCommunity(
	ctx context.Context,
	req *connect.Request[api.GetCommunityRequest],
) (*connect.Response[api.GetCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.Id,
	)

	logger.DebugContext(ctx, "requesting community")

	// Soft-deleted communities are filtered out by RequireActiveCommunity
	// (returns NotFound). Settings → Communities deleted-list is the only
	// surface that should see deleted communities; it does not call this RPC.
	community, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.Id)
	if err != nil {
		return nil, err
	}

	// Get member count
	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, req.Msg.Id)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get member count", "error", err)
		return nil, connecterr.Internal(ctx, "GetCommunity", err)
	}

	logger.DebugContext(
		ctx, "get community response",
		"operation", "GetCommunity",
		"media_ids", community.MediaIds,
	)

	return connect.NewResponse(&api.GetCommunityResponse{
		Id:          community.Id,
		Name:        community.Name,
		Description: community.Description,
		MediaIds:    community.MediaIds,
		NumMembers:  int32(memberCount),
		MaxMembers:  int32(communitylib.MaxCommunityMembers),
		OwnerUserId: community.OwnerUserId,
	}), nil
}
