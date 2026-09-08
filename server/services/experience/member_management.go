package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// SetExperienceMemberRSVP lets the experience owner set an invitee's RSVP on
// their behalf from the Who's In roster (invited → going / maybe / not going).
// Upserts a single RSVP per (experience, user); a new one is scoped to the
// event's own ad-hoc origin community. Owner-only (#2492).
func (s *Service) SetExperienceMemberRSVP(
	ctx context.Context,
	req *connect.Request[api.SetExperienceMemberRSVPRequest],
) (*connect.Response[api.SetExperienceMemberRSVPResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "SetExperienceMemberRSVP",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"member_user_id", req.Msg.MemberUserId,
		"intention", req.Msg.Intention.String(),
	)

	exp, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "SetExperienceMemberRSVP")
	if err != nil {
		return nil, err
	}
	if exp.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the host can set a member's RSVP"))
	}
	if req.Msg.MemberUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("member_user_id is required"))
	}

	intention := convertRSVPIntentionToModel(req.Msg.Intention)
	now := clock.UnixSec(ctx)

	existing, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"user_id":       req.Msg.MemberUserId,
	}, &models.ExperienceRSVP{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "SetExperienceMemberRSVP", err)
	}

	// UNSPECIFIED resets the member back to "invited": clear any RSVP while
	// keeping their membership, so they reappear in the Who's In Invited group.
	if req.Msg.Intention == api.RSVPIntention_RSVP_INTENTION_UNSPECIFIED {
		for _, m := range existing {
			if err := s.storage.Delete(ctx, m.(*models.ExperienceRSVP)); err != nil {
				return nil, connecterr.Internal(ctx, "SetExperienceMemberRSVP", err)
			}
		}
		logger.InfoContext(ctx, "host reset member to invited")
		// No RSVP event maps to "reset"; stream a silent roster refresh so the
		// member's and other viewers' open screens move them back to Invited. (#2492)
		s.publishRosterChangedToOthers(ctx, req.Msg.ExperienceId, authInfo.UserID, "")
		return connect.NewResponse(&api.SetExperienceMemberRSVPResponse{}), nil
	}

	if len(existing) > 0 {
		rsvp := existing[0].(*models.ExperienceRSVP)
		rsvp.Intention = intention
		rsvp.LastUpdatedUnixSec = now
		if err := s.storage.Update(ctx, rsvp); err != nil {
			return nil, connecterr.Internal(ctx, "SetExperienceMemberRSVP", err)
		}
	} else {
		communityID, err := s.originCommunityID(ctx, req.Msg.ExperienceId)
		if err != nil {
			return nil, connecterr.Internal(ctx, "SetExperienceMemberRSVP", err)
		}
		rsvp := &models.ExperienceRSVP{
			ExperienceId:       req.Msg.ExperienceId,
			UserId:             req.Msg.MemberUserId,
			CommunityId:        communityID,
			RsvpedAtUnixSec:    now,
			LastUpdatedUnixSec: now,
		}
		rsvp.Intention = intention
		if _, err := s.storage.Insert(ctx, rsvp); err != nil {
			return nil, connecterr.Internal(ctx, "SetExperienceMemberRSVP", err)
		}
	}

	// Stream the change so the member's (and other viewers') open screens
	// refresh live, instead of only on next load.
	s.publishHostSetRSVPEvent(ctx, req.Msg.ExperienceId, authInfo.UserID, req.Msg.Intention)

	logger.InfoContext(ctx, "host set member RSVP")
	return connect.NewResponse(&api.SetExperienceMemberRSVPResponse{}), nil
}

// RemoveExperienceMember uninvites a directly-invited individual: soft-deletes
// their membership in the event's ad-hoc origin community and clears their
// RSVP for the experience. Owner-only; the host can't remove themselves (#2492).
func (s *Service) RemoveExperienceMember(
	ctx context.Context,
	req *connect.Request[api.RemoveExperienceMemberRequest],
) (*connect.Response[api.RemoveExperienceMemberResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveExperienceMember",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
		"member_user_id", req.Msg.MemberUserId,
	)

	exp, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "RemoveExperienceMember")
	if err != nil {
		return nil, err
	}
	if exp.OwnerId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the host can remove a member"))
	}
	if req.Msg.MemberUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("member_user_id is required"))
	}
	if req.Msg.MemberUserId == exp.OwnerId {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("cannot remove the host"))
	}

	originID, err := s.originCommunityID(ctx, req.Msg.ExperienceId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RemoveExperienceMember", err)
	}
	now := clock.UnixSec(ctx)

	if originID != "" {
		members, err := s.storage.QueryByFields(ctx, map[string]any{
			"community_id": originID,
			"user_id":      req.Msg.MemberUserId,
		}, &models.CommunityUser{})
		if err != nil {
			return nil, connecterr.Internal(ctx, "RemoveExperienceMember", err)
		}
		for _, m := range members {
			cu := m.(*models.CommunityUser)
			cu.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  authInfo.UserID,
				DeletedAtUnixSec: now,
			}
			if err := s.storage.Update(ctx, cu); err != nil {
				return nil, connecterr.Internal(ctx, "RemoveExperienceMember", err)
			}
		}
	}

	// Clear the member's RSVP(s) for this experience so they drop off the roster.
	rsvps, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": req.Msg.ExperienceId,
		"user_id":       req.Msg.MemberUserId,
	}, &models.ExperienceRSVP{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "RemoveExperienceMember", err)
	}
	for _, m := range rsvps {
		if err := s.storage.Delete(ctx, m.(*models.ExperienceRSVP)); err != nil {
			return nil, connecterr.Internal(ctx, "RemoveExperienceMember", err)
		}
	}

	logger.InfoContext(ctx, "host removed member from experience")
	// Stream a silent roster refresh so the host's and other viewers' open
	// rosters drop the removed member live, not just on next load. (#2492)
	s.publishRosterChangedToOthers(ctx, req.Msg.ExperienceId, authInfo.UserID, "")
	return connect.NewResponse(&api.RemoveExperienceMemberResponse{}), nil
}

// ensureRSVPerInOriginCommunity promotes a yes/maybe RSVPer into the event's
// own ad-hoc origin community so their name surfaces individually in the host's
// Who's-In roster — the intuitive "they popped out of the community into the
// list" behaviour (#2548). Someone who can see the event only through a named
// community it is shared to is otherwise just part of that community's collapsed
// member count; once they RSVP, the host expects to see them by name.
//
// Idempotent: an active member is a no-op, and a previously-removed member's
// soft-deleted membership is restored rather than re-inserted (the
// (community_id, user_id) uniqueness would otherwise collide). Skips the owner
// (always a member) and named-only events (no origin community). This lives only
// on the experience RSVP path — gear and requests have no individual-roster
// surface for a name to pop into, so they get no implicit join.
func (s *Service) ensureRSVPerInOriginCommunity(ctx context.Context, experienceID, userID, ownerID string) error {
	if userID == "" || userID == ownerID {
		return nil
	}
	originID, err := s.originCommunityID(ctx, experienceID)
	if err != nil {
		return err
	}
	if originID == "" {
		return nil
	}

	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": originID,
		"user_id":      userID,
	}, &models.CommunityUser{}, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return err
	}

	now := clock.UnixSec(ctx)
	if len(rows) > 0 {
		cu := rows[0].(*models.CommunityUser)
		if cu.Deleted == nil || cu.Deleted.DeletedAtUnixSec == 0 {
			return nil // already an active member
		}
		// Restore the soft-deleted membership. A non-nil zero value is required
		// so the flattened deleted_* columns are written back to 0; a nil
		// sub-message is skipped by the column extractor and would leave the row
		// soft-deleted.
		cu.Deleted = &models.DeletedMetadata{}
		cu.InviterId = ownerID
		cu.CreatedAtUnixSec = now
		return s.storage.Update(ctx, cu)
	}

	_, err = s.storage.Insert(ctx, &models.CommunityUser{
		CommunityId:      originID,
		UserId:           userID,
		InviterId:        ownerID,
		CreatedAtUnixSec: now,
	})
	return err
}

// originCommunityID returns the id of the experience's own ad-hoc origin
// community (the per-item community provisioned at creation), or "" if none.
func (s *Service) originCommunityID(ctx context.Context, experienceID string) (string, error) {
	communities, err := s.storage.QueryByField(
		ctx, "origin_experience_id", experienceID, &models.Community{},
	)
	if err != nil {
		return "", err
	}
	if len(communities) == 0 {
		return "", nil
	}
	return communities[0].(*models.Community).Id, nil
}
