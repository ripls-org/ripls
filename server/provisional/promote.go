package provisional

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/contact"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// FindUnclaimedByPhone returns every non-deleted, unclaimed provisional user —
// across all communities — whose contact handle is the given E.164 phone
// number. It is the cross-community lookup behind promote-on-verify: a single
// verified phone can match provisional placeholders seeded in several different
// communities, and each must be merged into the one real account so the person
// never ends up with duplicate identities.
//
// phoneNumber must already be normalized (server/contact.NormalizePhoneE164);
// matching is exact against the stored handle. An empty phoneNumber returns no
// results — the empty string is the default of the phone_number column for
// every email- and name-only provisional user, so querying it would match the
// wrong rows.
func FindUnclaimedByPhone(ctx context.Context, store *storage.ProtoSQLStorage, phoneNumber string) ([]*models.ProvisionalUser, error) {
	if phoneNumber == "" {
		return nil, nil
	}

	results, err := store.QueryByField(ctx, "phone_number", phoneNumber, &models.ProvisionalUser{})
	if err != nil {
		return nil, fmt.Errorf("failed to query provisional users by phone: %w", err)
	}

	return filterUnclaimed(results), nil
}

// FindUnclaimedByEmail is the email analog of FindUnclaimedByPhone: it returns
// every non-deleted, unclaimed provisional user — across all communities —
// whose contact handle is the given email address. It is the cross-community
// lookup behind email-side promote-on-register (EMAIL-1b).
//
// email must already be normalized (server/auth.NormalizeEmail); matching is
// exact against the stored handle. An empty email returns no results — the
// empty string is the default of the email column for every phone- and
// name-only provisional user, so querying it would match the wrong rows.
func FindUnclaimedByEmail(ctx context.Context, store *storage.ProtoSQLStorage, email string) ([]*models.ProvisionalUser, error) {
	if email == "" {
		return nil, nil
	}

	results, err := store.QueryByField(ctx, "email", email, &models.ProvisionalUser{})
	if err != nil {
		return nil, fmt.Errorf("failed to query provisional users by email: %w", err)
	}
	return filterUnclaimed(results), nil
}

// PromoteByPhone claims every provisional placeholder seeded for a
// freshly-verified phone number, across all communities: it joins the real user
// to each provisional user's community (when not already a member) and merges
// the provisional activity history into the real account. This is what turns a
// deviceless, contact-keyed invitee into a full member the instant they verify
// their phone — including in communities they were invited to but did not
// register through.
//
// It is shared by both phone-first registration (login.Service.PhoneRegister)
// and add-phone-to-existing-account (user.Service.AddPhoneNumber) so the
// cross-community claim behaves identically on both paths. Callers must have
// already established that the phone is unclaimed by a real account (e.g. via
// UserManager.GetUserByPhone), so there is no cross-auth-method collision to
// resolve here.
//
// It is best-effort and idempotent. The membership join is guarded by an
// existence check and the merge re-checks claimed state, so a partial run (e.g.
// the link-tied provisional user already merged by completeRegistration, or a
// failure on one community) is safe: the rest still promote. Genuine failures
// are logged loudly so data bugs stay visible.
//
// publisher records the "new member joined" community event with notification
// fan-out; pass nil (e.g. in tests with no bus wired) to persist the event
// directly without fan-out. A nil interface is handled — do not pass a typed-nil
// Publisher.
func PromoteByPhone(ctx context.Context, store *storage.ProtoSQLStorage, publisher cebus.Publisher, userID, phoneNumber string) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "provisional.PromoteByPhone",
		"user_id", userID,
		"phone", logging.MaskPhone(phoneNumber),
	)

	// Match against the same normalized form used when the handle was stored.
	normalized, err := contact.NormalizePhoneE164(phoneNumber)
	if err != nil {
		logger.WarnContext(ctx, "skipping provisional promotion: phone not normalizable", "error", err)
		return
	}

	provs, err := FindUnclaimedByPhone(ctx, store, normalized)
	if err != nil {
		logger.ErrorContext(ctx, "failed to look up provisional users for promotion", "error", err)
		return
	}
	if len(provs) == 0 {
		return
	}

	promoted := 0
	for _, prov := range provs {
		isMember, err := auth.IsMemberOfCommunity(ctx, store, prov.CommunityId, userID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to check membership during promotion",
				"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
			continue
		}
		if !isMember {
			if err := addUserToCommunity(ctx, store, userID, prov.CommunityId, prov.CreatedByUserId); err != nil {
				logger.ErrorContext(ctx, "failed to join community during promotion",
					"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
				continue
			}
			// Surface the new member to the community exactly like an invite-link join.
			recordJoinEvent(ctx, store, publisher, prov.CommunityId, userID)
		}
		if err := MergeIntoUser(ctx, store, prov.Id, userID); err != nil {
			logger.ErrorContext(ctx, "failed to merge provisional user during promotion",
				"community_id", prov.CommunityId, "provisional_user_id", prov.Id, "error", err)
			continue
		}
		promoted++
	}

	if promoted > 0 {
		logger.InfoContext(ctx, "promoted provisional users on phone verification", "count", promoted)
	}
}

// addUserToCommunity inserts a CommunityUser membership row. Mirrors the
// equivalent helper in the login service so promotion joins look identical to an
// invite-link join.
func addUserToCommunity(ctx context.Context, store *storage.ProtoSQLStorage, userID, communityID, inviterID string) error {
	membership := &models.CommunityUser{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        inviterID,
		CreatedAtUnixSec: clock.UnixSec(ctx),
	}
	_, err := store.Insert(ctx, membership)
	return err
}

// recordJoinEvent records an INVITATION_LINK_USED community event for a promoted
// member. When publisher is non-nil the event is published (persisted + notified
// via fan-out); otherwise it is inserted directly so the row still exists with
// no notifications (the no-bus path used in tests). Best-effort: a failure is
// logged but never blocks promotion.
func recordJoinEvent(ctx context.Context, store *storage.ProtoSQLStorage, publisher cebus.Publisher, communityID, actorID string) {
	event := &models.CommunityEvent{
		Id:                uuid.New().String(),
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
		ActorId:           actorID,
		OccurredAtUnixSec: clock.UnixSec(ctx),
	}

	var err error
	if publisher != nil {
		_, err = publisher.Publish(ctx, event)
	} else {
		_, err = store.Insert(ctx, event)
	}
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to record community join event during promotion",
			"community_id", communityID, "actor_id", actorID, "error", err)
	}
}

// filterUnclaimed drops soft-deleted and already-claimed provisional users from
// a QueryByField result set. QueryByField already filters soft-deleted rows; the
// explicit Deleted check is defensive and mirrors findProvisionalUserByHandle.
func filterUnclaimed(results []proto.Message) []*models.ProvisionalUser {
	unclaimed := make([]*models.ProvisionalUser, 0, len(results))
	for _, r := range results {
		prov := r.(*models.ProvisionalUser)
		if prov.Deleted != nil || prov.ClaimedByUserId != nil {
			continue
		}
		unclaimed = append(unclaimed, prov)
	}
	return unclaimed
}
