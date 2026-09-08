package community

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/contact"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// CreateProvisionalUser creates a lightweight placeholder account for a non-registered participant.
func (s *Service) CreateProvisionalUser(
	ctx context.Context,
	req *connect.Request[api.CreateProvisionalUserRequest],
) (*connect.Response[api.CreateProvisionalUserResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Resolve the optional off-app contact handle (oneof). Normalize it so the
	// same person dedups and routes consistently regardless of input format.
	var phone, email string
	switch req.Msg.Contact.(type) {
	case *api.CreateProvisionalUserRequest_PhoneNumber:
		normalized, normErr := contact.NormalizePhoneE164(req.Msg.GetPhoneNumber())
		if normErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid phone number: %w", normErr))
		}
		phone = normalized
	case *api.CreateProvisionalUserRequest_Email:
		email = auth.NormalizeEmail(req.Msg.GetEmail())
		if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid email address"))
		}
	}

	prov, err := s.createOrGetProvisionalUser(ctx, req.Msg.CommunityId, req.Msg.Name, phone, email, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&api.CreateProvisionalUserResponse{
		ProvisionalUser: provisionalUserModelToAPI(prov),
	}), nil
}

// createOrGetProvisionalUser inserts a provisional member for a normalized
// contact handle (exactly one of phone/email non-empty; name required) in a
// community, reusing an existing one for the same handle (community-scoped
// dedup; cross-community dedup happens later on promotion). The caller is
// responsible for auth and handle normalization. Shared by the
// CreateProvisionalUser RPC and ShareItem.
func (s *Service) createOrGetProvisionalUser(ctx context.Context, communityID, name, phone, email, createdByUserID string) (*models.ProvisionalUser, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "createOrGetProvisionalUser",
		"community_id", communityID,
	)

	if phone != "" || email != "" {
		existing, lookupErr := s.findProvisionalUserByHandle(ctx, communityID, phone, email)
		if lookupErr != nil {
			logger.ErrorContext(ctx, "failed to look up provisional user by handle", "error", lookupErr)
			return nil, connecterr.Internal(ctx, "createOrGetProvisionalUser", lookupErr)
		}
		if existing != nil {
			logger.InfoContext(ctx, "reusing existing provisional user for contact handle",
				"provisional_user_id", existing.Id, "contact_kind", contactKind(existing))
			return existing, nil
		}
	}

	prov := &models.ProvisionalUser{
		Name:             name,
		CommunityId:      communityID,
		CreatedByUserId:  createdByUserID,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}
	switch {
	case phone != "":
		prov.Contact = &models.ProvisionalUser_PhoneNumber{PhoneNumber: phone}
	case email != "":
		prov.Contact = &models.ProvisionalUser_Email{Email: email}
	}

	id, err := s.storage.Insert(ctx, prov)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert provisional user", "error", err)
		return nil, connecterr.Internal(ctx, "createOrGetProvisionalUser", err)
	}
	prov.Id = id

	logger.InfoContext(ctx, "provisional user created",
		"provisional_user_id", id,
		"contact_kind", contactKind(prov),
		"contact", maskedHandle(prov))
	return prov, nil
}

// findProvisionalUserByHandle returns the non-deleted provisional user in the
// community whose normalized phone or email handle matches, or nil if none.
// Dedup is community-scoped; cross-community dedup happens on promotion.
func (s *Service) findProvisionalUserByHandle(ctx context.Context, communityID, phone, email string) (*models.ProvisionalUser, error) {
	results, err := s.storage.QueryByField(ctx, "community_id", communityID, &models.ProvisionalUser{})
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		p := r.(*models.ProvisionalUser)
		if p.Deleted != nil {
			continue
		}
		if phone != "" && p.GetPhoneNumber() == phone {
			return p, nil
		}
		if email != "" && p.GetEmail() == email {
			return p, nil
		}
	}
	return nil, nil
}

// contactKind reports "phone", "email", or "none" for a provisional user —
// safe for logs/metrics, never exposing the handle value itself.
func contactKind(p *models.ProvisionalUser) string {
	switch p.Contact.(type) {
	case *models.ProvisionalUser_PhoneNumber:
		return "phone"
	case *models.ProvisionalUser_Email:
		return "email"
	default:
		return "none"
	}
}

// maskedHandle returns a PII-masked form of the contact handle for logs.
func maskedHandle(p *models.ProvisionalUser) string {
	switch p.Contact.(type) {
	case *models.ProvisionalUser_PhoneNumber:
		return logging.MaskPhone(p.GetPhoneNumber())
	case *models.ProvisionalUser_Email:
		return logging.MaskEmail(p.GetEmail())
	default:
		return ""
	}
}

// ListProvisionalUsers lists all provisional users in a community.
func (s *Service) ListProvisionalUsers(
	ctx context.Context,
	req *connect.Request[api.ListProvisionalUsersRequest],
) (*connect.Response[api.ListProvisionalUsersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	results, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.ProvisionalUser{})
	if err != nil {
		logger := logging.LoggerWithContext(ctx).With("operation", "ListProvisionalUsers", "community_id", req.Msg.CommunityId)
		logger.ErrorContext(ctx, "failed to query provisional users", "error", err)
		return nil, connecterr.Internal(ctx, "ListProvisionalUsers", err)
	}

	apiProvisionals := make([]*api.ProvisionalUser, 0, len(results))
	for _, r := range results {
		prov := r.(*models.ProvisionalUser)
		if prov.Deleted != nil {
			continue
		}
		apiProvisionals = append(apiProvisionals, provisionalUserModelToAPI(prov))
	}

	return connect.NewResponse(&api.ListProvisionalUsersResponse{
		ProvisionalUsers: apiProvisionals,
	}), nil
}

// SearchProvisionalUsers searches provisional users in a community by name using case-insensitive matching.
func (s *Service) SearchProvisionalUsers(
	ctx context.Context,
	req *connect.Request[api.SearchProvisionalUsersRequest],
) (*connect.Response[api.SearchProvisionalUsersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	provs, err := s.storage.SearchProvisionalUsersByName(ctx, req.Msg.CommunityId, req.Msg.Query, limit)
	if err != nil {
		logger := logging.LoggerWithContext(ctx).With("operation", "SearchProvisionalUsers", "community_id", req.Msg.CommunityId)
		logger.ErrorContext(ctx, "failed to search provisional users", "error", err)
		return nil, connecterr.Internal(ctx, "SearchProvisionalUsers", err)
	}

	apiProvisionals := make([]*api.ProvisionalUser, len(provs))
	for i, prov := range provs {
		apiProvisionals[i] = provisionalUserModelToAPI(prov)
	}

	return connect.NewResponse(&api.SearchProvisionalUsersResponse{
		ProvisionalUsers: apiProvisionals,
	}), nil
}

// GetProvisionalUserInviteLink gets or creates an invitation link tied to a provisional user.
// When the invitee registers using this link, the provisional account is merged into their new real account.
func (s *Service) GetProvisionalUserInviteLink(
	ctx context.Context,
	req *connect.Request[api.GetProvisionalUserInviteLinkRequest],
) (*connect.Response[api.GetProvisionalUserInviteLinkResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if req.Msg.ProvisionalUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("provisional_user_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetProvisionalUserInviteLink",
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
		"provisional_user_id", req.Msg.ProvisionalUserId,
	)

	// Verify provisional user exists and belongs to this community.
	prov := &models.ProvisionalUser{}
	if err := s.storage.GetByID(ctx, req.Msg.ProvisionalUserId, prov); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provisional user not found"))
	}
	if prov.CommunityId != req.Msg.CommunityId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provisional user not found in this community"))
	}

	// Check if a share link already exists for this provisional user.
	if prov.InviteLinkId != nil {
		existing := &models.ShareLink{}
		if err := s.storage.GetByID(ctx, *prov.InviteLinkId, existing); err == nil && !existing.IsRevoked {
			inviteURL := fmt.Sprintf("%s/go/%s", baseURL(s.inviteLinkHostname), existing.ShortCode)
			return connect.NewResponse(&api.GetProvisionalUserInviteLinkResponse{
				ShortCode: existing.ShortCode,
				InviteUrl: inviteURL,
			}), nil
		}
	}

	// Generate a new share link tied to this provisional user.
	shortCode, err := s.generateUniqueShortCode(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate short code", "error", err)
		return nil, connecterr.Internal(ctx, "GetProvisionalUserInviteLink", err)
	}

	provisionalUserID := req.Msg.ProvisionalUserId
	link := &models.ShareLink{
		CommunityId:       req.Msg.CommunityId,
		InviterId:         authInfo.UserID,
		ShortCode:         shortCode,
		IsRevoked:         false,
		CreatedAtUnixSec:  clock.UnixSec(ctx),
		ProvisionalUserId: &provisionalUserID,
		Target: &models.ShareLink_CommunityInviteId{
			CommunityInviteId: req.Msg.CommunityId,
		},
	}

	linkID, err := s.storage.Insert(ctx, link)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert invite link", "error", err)
		return nil, connecterr.Internal(ctx, "GetProvisionalUserInviteLink", err)
	}

	// Store the link ID on the provisional user for future lookups.
	prov.InviteLinkId = &linkID
	if err := s.storage.Update(ctx, prov); err != nil {
		logger.ErrorContext(ctx, "failed to update provisional user with invite link id", "error", err)
		return nil, connecterr.Internal(ctx, "GetProvisionalUserInviteLink", err)
	}

	logger.InfoContext(ctx, "created provisional user invite link", "short_code", shortCode)

	inviteURL := fmt.Sprintf("%s/go/%s", baseURL(s.inviteLinkHostname), shortCode)
	return connect.NewResponse(&api.GetProvisionalUserInviteLinkResponse{
		ShortCode: shortCode,
		InviteUrl: inviteURL,
	}), nil
}

// GetProvisionalUserActivities returns the participation history for a provisional user.
func (s *Service) GetProvisionalUserActivities(
	ctx context.Context,
	req *connect.Request[api.GetProvisionalUserActivitiesRequest],
) (*connect.Response[api.GetProvisionalUserActivitiesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if req.Msg.ProvisionalUserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("provisional_user_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetProvisionalUserActivities",
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
		"provisional_user_id", req.Msg.ProvisionalUserId,
	)

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Fetch all attended RSVPs for this provisional user in this community.
	rsvps, err := storage.QueryByFields[*models.ExperienceRSVP](s.storage, ctx, map[string]any{
		"provisional_user_id": req.Msg.ProvisionalUserId,
		"community_id":        req.Msg.CommunityId,
		"attended":            int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query provisional user rsvps", "error", err)
		return nil, connecterr.Internal(ctx, "GetProvisionalUserActivities", err)
	}

	totalCount := len(rsvps)

	// Apply limit before loading experiences.
	if len(rsvps) > limit {
		rsvps = rsvps[:limit]
	}

	if len(rsvps) == 0 {
		return connect.NewResponse(&api.GetProvisionalUserActivitiesResponse{
			Activities:         []*api.ProvisionalUserActivityItem{},
			TotalActivityCount: int32(totalCount),
		}), nil
	}

	expIDs := make([]string, len(rsvps))
	for i, r := range rsvps {
		expIDs[i] = r.ExperienceId
	}

	expMap, err := storage.GetByIDs[*models.Experience](s.storage, ctx, expIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load experiences", "error", err)
		return nil, connecterr.Internal(ctx, "GetProvisionalUserActivities", err)
	}

	// Build activity items, sorted most-recent-first by completed_at.
	activities := make([]*api.ProvisionalUserActivityItem, 0, len(rsvps))
	for _, exp := range expMap {
		var completedAt int64
		if exp.CompletedAtUnixSec != nil {
			completedAt = *exp.CompletedAtUnixSec
		}
		emptyConvID := ""
		activities = append(activities, &api.ProvisionalUserActivityItem{
			Experience: &api.Experience{
				Id:                 exp.Id,
				Name:               exp.Name,
				Description:        exp.Description,
				MediaIds:           exp.MediaIds,
				LocationId:         exp.LocationId,
				State:              provisionalConvertExperienceState(exp.State),
				Time:               services.ConvertTimeModelsToAPI(exp.Time),
				MaxParticipants:    exp.MaxParticipants,
				CompletedAtUnixSec: exp.CompletedAtUnixSec,
				CompletionSummary:  exp.CompletionSummary,
				ConversationId:     &emptyConvID,
			},
			CompletedAtUnixSec: completedAt,
		})
	}

	sort.Slice(activities, func(i, j int) bool {
		return activities[i].CompletedAtUnixSec > activities[j].CompletedAtUnixSec
	})

	logger.InfoContext(ctx, "fetched provisional user activities", "count", len(activities), "total", totalCount)

	return connect.NewResponse(&api.GetProvisionalUserActivitiesResponse{
		Activities:         activities,
		TotalActivityCount: int32(totalCount),
	}), nil
}

// provisionalUserModelToAPI converts a storage ProvisionalUser to its API representation.
func provisionalUserModelToAPI(s *models.ProvisionalUser) *api.ProvisionalUser {
	return &api.ProvisionalUser{
		Id:        s.Id,
		Name:      s.Name,
		IsClaimed: s.ClaimedByUserId != nil,
	}
}

// provisionalConvertExperienceState converts storage ExperienceState to API ExperienceState.
func provisionalConvertExperienceState(state models.ExperienceState) api.ExperienceState {
	switch state {
	case models.ExperienceState_EXPERIENCE_STATE_ACTIVE:
		return api.ExperienceState_EXPERIENCE_STATE_ACTIVE
	case models.ExperienceState_EXPERIENCE_STATE_JOINED:
		return api.ExperienceState_EXPERIENCE_STATE_JOINED
	case models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
		return api.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
	case models.ExperienceState_EXPERIENCE_STATE_COMPLETED:
		return api.ExperienceState_EXPERIENCE_STATE_COMPLETED
	case models.ExperienceState_EXPERIENCE_STATE_CANCELLED:
		return api.ExperienceState_EXPERIENCE_STATE_CANCELLED
	default:
		return api.ExperienceState_EXPERIENCE_STATE_UNSPECIFIED
	}
}
