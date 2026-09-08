package login

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/provisional"
	"go.ripls.org/ripls/server/storage"
)

// EmailRegister creates a new user account with email/password authentication.
func (s *Service) EmailRegister(
	ctx context.Context,
	req *connect.Request[api.EmailRegisterRequest],
) (*connect.Response[api.EmailRegisterResponse], error) {
	if req.Msg.Email == "" || req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email and name are required"))
	}

	email := auth.NormalizeEmail(req.Msg.Email)

	// Establish the credential before anything else. Registration requires a
	// proof token: an account created here can only ever be one whose owner
	// proved they receive mail at the address (#2864). The password fallback
	// that sign-in still honours is closed off on this path, which fixes the
	// population of password-holding accounts at its current size — it can
	// shrink as people migrate, but nothing can add to it.
	//
	// This holds in every environment, dev included. Tests that need the
	// pre-#2571 account shape build it out of band (server/cmd/seed-legacy-user)
	// rather than through a branch production does not have, so what e2e
	// exercises is the same code prod runs.
	if _, err := s.resolveEmailCredential(ctx, email, req.Msg.EmailProofToken, "", false); err != nil {
		return nil, err
	}

	// Reject if an account already exists with this email (any auth method).
	existingUser, err := s.userManager.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "EmailRegister", err, "detail", "failed to check existing email")
	}
	if existingUser != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "registration_rejected_duplicate_email",
			"operation", "EmailRegister",
			"user_email", logging.MaskEmail(email),
			"auth_method", "email_password",
			"existing_auth_method", existingUser.AuthMethod.String(),
		)
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("an account already exists for this email — please sign in with %s",
				getAuthMethodName(existingUser.AuthMethod)))
	}

	// Check if invitation short code is provided.
	var invitation *models.ShareLink
	if req.Msg.ShortCode == "" {
		// No short code - only allow in dev mode.
		if s.devAuth {
			invitation = nil
		} else {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("short code is required"))
		}
	} else {
		// Check if this is the first user (bootstrap case).
		hasUsers, err := s.storage.HasAnyUsers(ctx)
		if err != nil {
			return nil, connecterr.Internal(ctx, "EmailRegister", err, "detail", "failed to check existing users")
		}

		if hasUsers {
			invitation, err = s.validateShortCode(ctx, req.Msg.ShortCode)
			if err != nil {
				return nil, err // Already wrapped in connect.Error
			}
		} else {
			// First user bootstrap - accept any code (even invalid ones).
			invitation = nil
		}
	}

	// The account is passwordless from birth — reaching here means ownership of
	// the address was proven, which is the only way in (#2864). Storing no
	// password_hash at all is what makes the eventual column removal a no-op.
	now := clock.UnixSec(ctx)
	user := &models.User{
		Id:         uuid.New().String(),
		Email:      email,
		Name:       req.Msg.Name,
		CreatedAt:  now,
		UpdatedAt:  now,
		Role:       models.Role_ROLE_USER,
		AuthMethod: models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		// auth_method stays EMAIL_PASSWORD: it records how an account was
		// originally created, for analytics and display, and is no longer what
		// decides whether a sign-in is allowed (that is credential presence —
		// see EmailLogin). Rewriting it would falsify history for no gain.
		EmailVerifiedAtUnixSec: &now,
	}
	seedPreferredLanguage(ctx, user)

	// Set inviter if we have an invitation
	if invitation != nil {
		user.InvitedById = invitation.InviterId
	}

	// Tag user with simulation ID if provided.
	if req.Msg.SimulationId != nil {
		user.SimulationId = req.Msg.SimulationId
	}

	// Complete registration
	var accessToken, rawRefreshToken string
	if invitation != nil {
		// Full registration flow with community join
		accessToken, rawRefreshToken, err = s.completeRegistration(ctx, user, invitation, req.Msg.GearId)
		if err != nil {
			return nil, err
		}
	} else {
		// Bootstrap user - just insert user and generate tokens.
		_, err := s.storage.Insert(ctx, user)
		if err != nil {
			return nil, connecterr.Internal(ctx, "EmailRegister", err, "detail", "failed to create user")
		}

		accessToken, err = s.authTokenConfig.GenerateToken(user.Id, user.Email, user.Role)
		if err != nil {
			return nil, connecterr.Internal(ctx, "EmailRegister", err, "detail", "failed to generate token")
		}

		rawRefreshToken, err = s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
		if err != nil {
			return nil, connecterr.Internal(ctx, "EmailRegister", err, "detail", "failed to generate refresh token")
		}
	}

	// Promote any provisional placeholders seeded for this email — in this
	// community or any other — into the new real account (EMAIL-1b), the email
	// analog of PhoneRegister's promoteProvisionalByPhone. Best-effort: a failure
	// is logged inside the helper but never blocks a successful registration (the
	// link-tied merge in completeRegistration is independent).
	s.promoteProvisionalByEmail(ctx, user)

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user registered",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
		"auth_method", "email_password",
	)

	response := &api.EmailRegisterResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}

	return connect.NewResponse(response), nil
}

// OIDCRegister creates a new user account with an OIDC provider (Google, Apple).
func (s *Service) OIDCRegister(
	ctx context.Context,
	req *connect.Request[api.OIDCRegisterRequest],
) (*connect.Response[api.OIDCRegisterResponse], error) {
	if req.Msg.Provider == api.OIDCProvider_OIDC_PROVIDER_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("provider must be specified"))
	}

	if req.Msg.IdToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("id_token is required"))
	}

	if req.Msg.ShortCode == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("short code is required"))
	}

	// Validate the OIDC token with the provider
	idToken, err := s.oidcManager.ValidateToken(ctx, req.Msg.Provider, req.Msg.IdToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("token validation failed: %w", err))
	}

	// Extract user information from the validated token
	userInfo, err := auth.ExtractUserInfo(idToken)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCRegister", err, "detail", "failed to extract user info")
	}

	email := auth.NormalizeEmail(userInfo.Email)

	// Reject if an account already exists with this email (any auth method).
	existingUser, err := s.userManager.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCRegister", err, "detail", "failed to check existing email")
	}
	if existingUser != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "registration_rejected_duplicate_email",
			"operation", "OIDCRegister",
			"user_email", logging.MaskEmail(email),
			"auth_method", req.Msg.Provider.String(),
			"existing_auth_method", existingUser.AuthMethod.String(),
		)
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("an account already exists for this email — please sign in with %s",
				getAuthMethodName(existingUser.AuthMethod)))
	}

	invitation, err := s.validateShortCode(ctx, req.Msg.ShortCode)
	if err != nil {
		return nil, err // Already wrapped in connect.Error
	}

	// Create user with appropriate OIDC auth method
	now := clock.UnixSec(ctx)
	user := &models.User{
		Id:                  uuid.New().String(),
		Email:               email,
		Name:                userInfo.Name,
		CreatedAt:           now,
		UpdatedAt:           now,
		Role:                models.Role_ROLE_USER,
		AuthMethod:          mapProviderToAuthMethod(req.Msg.Provider),
		OidcProviderSubject: userInfo.Subject,
		InvitedById:         invitation.InviterId,
	}
	seedPreferredLanguage(ctx, user)

	// An identity provider that asserts email_verified has already done the
	// ownership check a mailed code does, so the account starts verified. A
	// provider that does not assert it leaves the address unverified — the
	// claim is honored, not assumed (#2571).
	if userInfo.EmailVerified {
		user.EmailVerifiedAtUnixSec = &now
	}

	// Fetch and store avatar if available
	if userInfo.Picture != "" {
		mediaID, err := s.fetchAndStoreAvatar(ctx, user.Id, userInfo.Picture)
		if err != nil {
			// Log error but don't fail registration - avatar is optional
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "failed to fetch avatar",
				"user_id", user.Id,
				"user_email", logging.MaskEmail(user.Email),
				"error", err,
			)
		} else {
			user.MediaIds = []string{mediaID}
		}
	}

	// Complete registration: insert user, join community, record event, generate tokens.
	accessToken, rawRefreshToken, err := s.completeRegistration(ctx, user, invitation, req.Msg.GearId)
	if err != nil {
		return nil, err
	}

	// Promote any provisional placeholders seeded for this email into the new
	// account (EMAIL-1b). An OIDC email is provider-verified, so this is the true
	// analog of phone-OTP promotion (cf. the accepted-risk note on
	// promoteProvisionalByEmail for the unverified email/password path).
	s.promoteProvisionalByEmail(ctx, user)

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user registered",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
		"auth_method", req.Msg.Provider.String(),
	)

	response := &api.OIDCRegisterResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}

	return connect.NewResponse(response), nil
}

// CheckInvitation validates an invitation token and returns associated details.
func (s *Service) CheckInvitation(
	ctx context.Context,
	req *connect.Request[api.CheckInvitationRequest],
) (*connect.Response[api.CheckInvitationResponse], error) {
	if req.Msg.ShortCode == "" {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "short code is required",
		}), nil
	}

	// Look up share link by short_code. The login service does its own
	// storage lookup rather than calling into the community service so the
	// two services stay independent (docs/server/architecture.md §
	// "Service Independence").
	links, err := s.storage.QueryByField(ctx, "short_code", req.Msg.ShortCode, &models.ShareLink{})
	if err != nil {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "failed to validate invitation",
		}), nil
	}

	if len(links) == 0 {
		// Invalid token - check if this is the first user (bootstrap case)
		hasUsers, err := s.storage.HasAnyUsers(ctx)
		if err != nil {
			return connect.NewResponse(&api.CheckInvitationResponse{
				IsValid:      false,
				ErrorMessage: "failed to validate invitation",
			}), nil
		}

		if !hasUsers {
			// First user bootstrap - accept any token
			return connect.NewResponse(&api.CheckInvitationResponse{
				IsValid:       true,
				CommunityId:   "",
				CommunityName: "Bootstrap User",
				InviterName:   "System",
				NumMembers:    0,
				MaxMembers:    1, // Bootstrap community has capacity of 1
				ErrorMessage:  "",
			}), nil
		}

		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "invalid invitation code",
		}), nil
	}

	link := links[0].(*models.ShareLink)

	// Verify not revoked.
	if link.IsRevoked {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "invitation link has been revoked",
		}), nil
	}

	targetKind, targetID := shareLinkTargetKindAndID(link)

	// Get community details.
	communityModel := &models.Community{}
	if err := s.storage.GetByID(ctx, link.CommunityId, communityModel); err != nil {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "failed to retrieve community information",
		}), nil
	}

	// Get inviter details.
	inviter := &models.User{}
	if err := s.storage.GetByID(ctx, link.InviterId, inviter); err != nil {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "failed to retrieve inviter information",
		}), nil
	}

	// Get member count.
	memberCount, err := community.GetNumCommunityMembers(ctx, s.storage, link.CommunityId)
	if err != nil {
		return connect.NewResponse(&api.CheckInvitationResponse{
			IsValid:      false,
			ErrorMessage: "failed to retrieve community information",
		}), nil
	}

	// Generate community image URL if available.
	communityImageURL := s.getCommunityImageURL(ctx, communityModel)

	resp := &api.CheckInvitationResponse{
		IsValid:           true,
		CommunityId:       communityModel.Id,
		CommunityName:     communityModel.Name,
		InviterName:       inviter.Name,
		NumMembers:        int32(memberCount),
		MaxMembers:        int32(community.MaxCommunityMembers),
		ErrorMessage:      "",
		CommunityImageUrl: communityImageURL,
		TargetKind:        &targetKind,
	}
	if targetID != "" {
		resp.TargetId = &targetID
	}
	return connect.NewResponse(resp), nil
}

// shareLinkTargetKindAndID maps the populated oneof variant on a
// ShareLink row to the API-side target_kind enum and item ID. The
// community variant returns an empty target_id by convention — the
// community itself is already identified by CommunityId on the
// response.
func shareLinkTargetKindAndID(link *models.ShareLink) (api.ShareLinkTargetKind, string) {
	switch {
	case link.GetCommunityInviteId() != "":
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_COMMUNITY, ""
	case link.GetGearId() != "":
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_GEAR, link.GetGearId()
	case link.GetTransferId() != "":
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_TRANSFER, link.GetTransferId()
	case link.GetRequestId() != "":
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_REQUEST, link.GetRequestId()
	case link.GetExperienceId() != "":
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_EVENT, link.GetExperienceId()
	default:
		return api.ShareLinkTargetKind_SHARE_LINK_TARGET_KIND_UNSPECIFIED, ""
	}
}

// validateShortCode validates an invitation short code and returns the
// underlying share link. Every target variant (community-invite, gear,
// transfer, request, event) is acceptable here: registration uses the
// link only to join the inviter's community, and the client routes to
// the variant-specific landing screen after sign-in.
func (s *Service) validateShortCode(ctx context.Context, shortCode string) (*models.ShareLink, error) {
	links, err := s.storage.QueryByField(ctx, "short_code", shortCode, &models.ShareLink{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "validateShortCode", err, "detail", "failed to query share link")
	}

	if len(links) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid invitation code"))
	}

	link := links[0].(*models.ShareLink)

	// Verify not revoked.
	if link.IsRevoked {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invitation link has been revoked"))
	}

	// Defensive: a share link must carry some target variant. A row with
	// the oneof unset is malformed and should never have been persisted.
	if link.GetCommunityInviteId() == "" &&
		link.GetGearId() == "" &&
		link.GetTransferId() == "" &&
		link.GetRequestId() == "" &&
		link.GetExperienceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid invitation code"))
	}

	return link, nil
}

// addUserToCommunity adds a user to a community.
func (s *Service) addUserToCommunity(ctx context.Context, userID, communityID, inviterID string) error {
	membership := &models.CommunityUser{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        inviterID,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}

	_, err := s.storage.Insert(ctx, membership)
	return err
}

// recordCommunityEvent records a community event with optional invitation short code and gear ID.
// When a notification service is wired, the event also goes through the
// notify pipeline so existing members get a push (e.g. "new member joined").
func (s *Service) recordCommunityEvent(ctx context.Context, communityID string, eventType models.CommunityEventType, actorID, shortCode, gearID string) {
	event := &models.CommunityEvent{
		Id:                uuid.New().String(),
		CommunityId:       communityID,
		EventType:         eventType,
		ActorId:           actorID,
		GearId:            gearID,
		OccurredAtUnixSec: clock.UnixSec(ctx),
	}

	// Set invitation short code if provided (oneof field).
	if shortCode != "" {
		event.Topic = &models.CommunityEvent_InvitationShortCode{
			InvitationShortCode: shortCode,
		}
	}

	if s.notificationService != nil {
		if _, err := s.bus.Publish(ctx, event); err != nil {
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "failed to record community event",
				"community_id", communityID,
				"event_type", eventType.String(),
				"actor_id", actorID,
				"error", err,
			)
		}
		return
	}

	if _, err := s.storage.Insert(ctx, event); err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "failed to record community event",
			"community_id", communityID,
			"event_type", eventType.String(),
			"actor_id", actorID,
			"error", err,
		)
	}
}

// promoteProvisionalByEmail is the email analog of promoteProvisionalByPhone
// (EMAIL-1b): it claims every provisional placeholder seeded for a registering
// email address, across all communities — joining the real user to each
// provisional user's community and merging its activity history into the real
// account. This turns a deviceless, email-keyed invitee into a full member the
// instant they register, including in communities they were invited to but did
// not register through.
//
// It requires the account's email to have been *proven* — via a mailed one-time
// code or an identity provider's verified claim — and does nothing otherwise.
// That gate is what closes "email shadowing" (#2571): before it, an
// email/password registration proved nothing about the address, so anyone could
// register someone else's address and absorb the placeholders keyed to it.
//
// The gate is belt-and-braces rather than the only defence. Every path that
// reaches here now establishes ownership first, so an unverified caller should
// be impossible; the check makes that a property of this function instead of a
// property of its callers, and the accompanying test fails if a future caller
// forgets.
//
// It is best-effort and idempotent, mirroring promoteProvisionalByPhone: the
// membership join is guarded by an existence check and the merge re-checks
// claimed state, so a partial run (e.g. the link-tied provisional user already
// merged by completeRegistration) is safe and registration still succeeds.
// Genuine failures are logged loudly so data bugs stay visible.
func (s *Service) promoteProvisionalByEmail(ctx context.Context, user *models.User) {
	userID := user.Id
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "promoteProvisionalByEmail",
		"user_id", userID,
		"email", logging.MaskEmail(user.Email),
	)

	// The gate. It reads the account's own stamp rather than taking the
	// caller's word for it, so there is no argument a caller could get wrong.
	if user.EmailVerifiedAtUnixSec == nil {
		logger.InfoContext(ctx, "skipping provisional promotion: email not verified")
		return
	}

	// Match against the same normalized form used when the handle was stored.
	normalized := auth.NormalizeEmail(user.Email)
	if normalized == "" {
		return
	}

	provs, err := provisional.FindUnclaimedByEmail(ctx, s.storage, normalized)
	if err != nil {
		logger.ErrorContext(ctx, "failed to look up provisional users for promotion", "error", err)
		return
	}
	if len(provs) == 0 {
		return
	}

	promoted := 0
	for _, prov := range provs {
		isMember, err := auth.IsMemberOfCommunity(ctx, s.storage, prov.CommunityId, userID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to check membership during promotion",
				"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
			continue
		}
		if !isMember {
			if err := s.addUserToCommunity(ctx, userID, prov.CommunityId, prov.CreatedByUserId); err != nil {
				logger.ErrorContext(ctx, "failed to join community during promotion",
					"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
				continue
			}
			// Surface the new member to the community exactly like an invite-link join.
			s.recordCommunityEvent(ctx, prov.CommunityId,
				models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED, userID, "", "")
		}
		if err := provisional.MergeIntoUser(ctx, s.storage, prov.Id, userID); err != nil {
			logger.ErrorContext(ctx, "failed to merge provisional user during promotion",
				"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
			continue
		}
		promoted++
	}

	if promoted > 0 {
		logger.InfoContext(ctx, "promoted provisional users on email registration", "count", promoted)
	}
}

// completeRegistration handles the common registration flow: insert user, join community,
// record event, and generate JWT and refresh tokens.
func (s *Service) completeRegistration(ctx context.Context, user *models.User, invitation *models.ShareLink, gearID string) (accessToken, rawRefreshToken string, err error) {
	// Check if community is at capacity before creating user (skip for empty community ID)
	if invitation.CommunityId != "" {
		memberCount, err := community.GetNumCommunityMembers(ctx, s.storage, invitation.CommunityId)
		if err != nil {
			return "", "", connecterr.Internal(ctx, "completeRegistration", err, "detail", "failed to check community capacity")
		}

		if memberCount >= community.MaxCommunityMembers {
			return "", "", connect.NewError(connect.CodeResourceExhausted,
				fmt.Errorf("community has reached maximum capacity of %d members", community.MaxCommunityMembers))
		}
	}

	// Insert user into database
	if _, err := s.storage.Insert(ctx, user); err != nil {
		return "", "", connecterr.Internal(ctx, "completeRegistration", err, "detail",

			// Auto-join user to the community (skip if no community ID)
			"failed to create user")
	}

	if invitation.CommunityId != "" {
		if err := s.addUserToCommunity(ctx, user.Id, invitation.CommunityId, invitation.InviterId); err != nil {
			return "", "", connecterr.Internal(ctx, "completeRegistration", err, "detail", "failed to add user to community")
		}

		// Record invitation link used event.
		s.recordCommunityEvent(ctx, invitation.CommunityId, models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED, user.Id, invitation.ShortCode, gearID)

		// If this share link is tied to a provisional user, merge the provisional account into the new user.
		if invitation.ProvisionalUserId != nil {
			if err := provisional.MergeIntoUser(ctx, s.storage, *invitation.ProvisionalUserId, user.Id); err != nil {
				// Log but don't fail registration — provisional merge can be retried separately.
				logger := logging.LoggerWithContext(ctx)
				logger.WarnContext(ctx, "failed to merge provisional user during registration",
					"provisional_user_id", *invitation.ProvisionalUserId,
					"user_id", user.Id,
					"error", err,
				)
			}
		}
	}

	// Generate access token (include phone number for phone auth users).
	var phoneNumber string
	if user.PhoneNumber != nil {
		phoneNumber = *user.PhoneNumber
	}
	accessToken, err = s.authTokenConfig.GenerateToken(user.Id, user.Email, user.Role, phoneNumber)
	if err != nil {
		return "", "", connecterr.Internal(ctx, "completeRegistration", err, "detail",

			// Generate refresh token.
			"failed to generate token")
	}

	rawRefreshToken, err = s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
	if err != nil {
		return "", "", connecterr.Internal(ctx, "completeRegistration", err, "detail", "failed to generate refresh token")
	}

	return accessToken, rawRefreshToken, nil
}

// ogImageDuration is how long presigned URLs for OG community images remain valid.
const ogImageDuration = 1 * time.Hour

// getCommunityImageURL returns a presigned URL for the community's first image,
// or an empty string if no image is available.
func (s *Service) getCommunityImageURL(ctx context.Context, communityModel *models.Community) string {
	if len(communityModel.MediaIds) == 0 {
		return ""
	}

	mediaID := communityModel.MediaIds[0]
	media := &models.Media{}
	if err := s.storage.GetByID(ctx, mediaID, media); err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to get community media for image URL", "error", err, "media_id", mediaID)
		return ""
	}

	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	url, err := s.bucket.GetSignedURL(ctx, bucketKey, ogImageDuration)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to generate presigned URL for community image", "error", err, "media_id", mediaID)
		return ""
	}

	return url
}
