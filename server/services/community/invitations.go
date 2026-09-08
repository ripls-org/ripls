package community

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// AcceptInvitationLink allows an already logged-in user to accept an invitation link.
func (s *Service) AcceptInvitationLink(
	ctx context.Context,
	req *connect.Request[api.AcceptInvitationLinkRequest],
) (*connect.Response[api.AcceptInvitationLinkResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"short_code", logging.MaskToken(req.Msg.ShortCode),
	)

	logger.InfoContext(ctx, "accepting invitation link")

	// Look up share link by short_code.
	links, err := s.lookupShareLink(ctx, req.Msg.ShortCode)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query share link", "error", err)
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err)
	}

	if len(links) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid invitation code"))
	}

	link := links[0]
	logger = logger.With("community_id", link.CommunityId)

	// This RPC accepts every populated share-link variant
	// (community_invite, gear, transfer, request, event) by joining the
	// user to the community. Event share links additionally require an
	// RSVP write, which the client fires separately via
	// RSVPToExperience after this RPC succeeds (see
	// docs/issues/2050-web-rsvp-actions.md §D1).
	if err := requireAcceptableViaCommunityJoin(link); err != nil {
		return nil, err
	}

	// Verify not revoked.
	if link.IsRevoked {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invitation link has been revoked"))
	}

	// Reject acceptance if the target community has been soft-deleted.
	community, err := auth.RequireActiveCommunity(ctx, s.storage, link.CommunityId)
	if err != nil {
		return nil, err
	}

	// Check if user is already a member.
	isAlreadyMember, err := auth.IsMemberOfCommunity(ctx, s.storage, link.CommunityId, authInfo.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check if user is already a member", "error", err)
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err)
	}

	if isAlreadyMember {
		// Already a member; just return success.
		return connect.NewResponse(&api.AcceptInvitationLinkResponse{
			CommunityId:   link.CommunityId,
			CommunityName: community.Name,
		}), nil
	}

	// Check if community is at capacity.
	memberCount, err := communitylib.GetNumCommunityMembers(ctx, s.storage, link.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get community member count", "error", err)
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err)
	}

	if memberCount >= communitylib.MaxCommunityMembers {
		logger.InfoContext(ctx, "community at capacity", "member_count", memberCount, "max_members", communitylib.MaxCommunityMembers)
		return nil, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("community has reached maximum capacity of %d members", communitylib.MaxCommunityMembers))
	}

	// Add user to community.
	membership := &models.CommunityUser{
		CommunityId:      link.CommunityId,
		UserId:           authInfo.UserID,
		InviterId:        link.InviterId,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}

	if _, err = s.storage.Insert(ctx, membership); err != nil {
		logger.ErrorContext(ctx, "failed to add user to community", "error", err)
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err)
	}

	// Record event. Gear context for the audit record comes from the
	// share_link row's typed target (empty for a plain community invite).
	// Legacy `?gear_id=` request-field pass-through was removed in #2562.
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: link.CommunityId,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		ActorId:     authInfo.UserID,
		GearId:      link.GetGearId(),
		Topic:       &models.CommunityEvent_InvitationShortCode{InvitationShortCode: req.Msg.ShortCode},
	}); err != nil {
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err)
	}

	// Recompute community regions after membership change.
	if err := s.recomputeCommunityRegions(ctx, link.CommunityId); err != nil {
		logger.ErrorContext(ctx, "failed to recompute community regions", "error", err)
		return nil, connecterr.Internal(ctx, "AcceptInvitationLink", err, "detail",
			"failed to recompute community regions")
	}

	// Welcome story generation is owned by the story_subscriber on the
	// community-event bus (#510 PR 4). The INVITATION_LINK_USED event
	// emitted here flows through the bus.

	logger.InfoContext(ctx, "user added to community via invitation link")

	return connect.NewResponse(&api.AcceptInvitationLinkResponse{
		CommunityId:   link.CommunityId,
		CommunityName: community.Name,
	}), nil
}

// shortCodeCharset excludes visually confusable characters (0/O, 1/l/I).
const shortCodeCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"

// shortCodeLength is the number of characters in a generated short code.
const shortCodeLength = 8

// generateShortCode generates a cryptographically random short code for URLs.
func generateShortCode() (string, error) {
	code := make([]byte, shortCodeLength)
	charsetLen := big.NewInt(int64(len(shortCodeCharset)))
	for i := range code {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate short code: %w", err)
		}
		code[i] = shortCodeCharset[n.Int64()]
	}
	return string(code), nil
}

// maxShortCodeAttempts is the maximum number of attempts to generate a unique short code.
const maxShortCodeAttempts = 10

// generateUniqueShortCode generates a short code that doesn't collide with existing ones.
func (s *Service) generateUniqueShortCode(ctx context.Context) (string, error) {
	for range maxShortCodeAttempts {
		code, err := generateShortCode()
		if err != nil {
			return "", err
		}
		existing, err := s.lookupShareLink(ctx, code)
		if err != nil {
			return "", fmt.Errorf("failed to check short code uniqueness: %w", err)
		}
		if len(existing) == 0 {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique short code after %d attempts", maxShortCodeAttempts)
}

// baseURL returns the full base URL with scheme for the given hostname.
// Uses http:// for localhost and IP addresses, https:// otherwise.
func baseURL(hostname string) string {
	host := hostname
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		return "http://" + hostname
	}
	return "https://" + hostname
}

// generateStoryForNewMember moved to
// server/story/subscriber/invitation.go in #510 PR 4. The
// INVITATION_LINK_USED event emitted by AcceptInvitationLink flows
// through the community-event bus and triggers the welcome story.
