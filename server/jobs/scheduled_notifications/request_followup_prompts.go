package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/storage"
)

// Request followup-prompt offsets relative to the request's
// created_at_unix_sec. Each offset fires one push to the requester
// while the request remains open. The reconciler emits one row per
// (request, offset) so the dispatcher can fire them independently.
const (
	OffsetRequestFollowupDay3  int64 = 3 * 24 * 3600
	OffsetRequestFollowupDay6  int64 = 6 * 24 * 3600
	OffsetRequestFollowupDay9  int64 = 9 * 24 * 3600
	OffsetRequestFollowupDay12 int64 = 12 * 24 * 3600
	OffsetRequestFollowupDay15 int64 = 15 * 24 * 3600
)

var requestFollowupOffsets = []int64{
	OffsetRequestFollowupDay3,
	OffsetRequestFollowupDay6,
	OffsetRequestFollowupDay9,
	OffsetRequestFollowupDay12,
	OffsetRequestFollowupDay15,
}

// requestBacklogGrace bounds the reconciler's "still in scope" window
// past the latest reminder slot. Once now > created_at + maxOffset +
// grace, the reconciler stops emitting tuples for the request, so
// existing rows can age out via the dispatcher and nothing re-emits
// indefinitely. This is what prevents the cycling-reminder pattern
// from running forever.
const requestBacklogGrace = 24 * 3600

// RequestWorld is the subset of storage and preferences the
// request-followup-prompt reconciler and dispatcher need. Defined as
// an interface so tests can pass a lightweight fake without Postgres.
type RequestWorld interface {
	// ListOpenRequests returns all Request rows in ACTIVE or
	// OFFERS_RECEIVED state that have not been soft-deleted.
	ListOpenRequests(ctx context.Context) ([]*models.Request, error)

	// GetRequest loads one Request by id.
	GetRequest(ctx context.Context, requestID string) (*models.Request, error)

	// GetPrimaryCommunityIDsForRequests returns the earliest
	// non-archived CommunityRequest.community_id for each request
	// in the batch. Requests with no non-archived CommunityRequest are
	// absent from the map. One query for the whole batch (no N+1).
	GetPrimaryCommunityIDsForRequests(ctx context.Context, requestIDs []string) (map[string]string, error)

	// GetCommunity loads one Community by id.
	GetCommunity(ctx context.Context, communityID string) (*models.Community, error)

	// GetUserTimezones batch-fetches IANA timezone strings for a slice
	// of user IDs. Missing or errored users map to "". Called once per
	// reconciler tick with all unique requester IDs so the count does
	// not scale with the number of open requests.
	GetUserTimezones(ctx context.Context, userIDs []string) map[string]string

	// GetCommunityNotificationPreferences returns the per-community
	// preferences row for (userID, communityID), or nil if no row
	// exists (treated as "all on"). Returns an error only on storage
	// failure.
	GetCommunityNotificationPreferences(ctx context.Context, userID, communityID string) (*models.CommunityNotificationPreferences, error)
}

// RequestFollowupReconciler emits desired-state tuples prompting the
// requester to mark open requests fulfilled or cancelled, at 3, 6, 9,
// 12, and 15 days after creation. One row per (request, offset); only
// emits while the request remains open and within the backlog-grace
// window.
type RequestFollowupReconciler struct {
	world    RequestWorld
	existing func(ctx context.Context) ([]*models.ScheduledNotification, error)
}

// NewRequestFollowupReconciler wires the reconciler against real storage.
func NewRequestFollowupReconciler(s *storage.ProtoSQLStorage) *RequestFollowupReconciler {
	return &RequestFollowupReconciler{
		world:    &storageRequestWorld{storage: s},
		existing: s.FindRequestScheduledNotifications,
	}
}

// Name returns the stable identifier used in structured logs.
func (r *RequestFollowupReconciler) Name() string {
	return "request_followup_prompts"
}

// Existing returns the rows currently in storage whose item variant is request.
func (r *RequestFollowupReconciler) Existing(ctx context.Context) ([]*models.ScheduledNotification, error) {
	return r.existing(ctx)
}

// Desired walks open requests and produces one desired tuple per
// (request, offset) pair whose fire_at is still within the window.
func (r *RequestFollowupReconciler) Desired(ctx context.Context) ([]*models.ScheduledNotification, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationReconcile",
		"reconciler", r.Name(),
	)

	requests, err := r.world.ListOpenRequests(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open requests: %w", err)
	}

	// Collect unique request IDs and requester IDs for batch lookups.
	// Both GetPrimaryCommunityIDsForRequests and GetUserTimezones are
	// called once per tick so the query count is constant regardless of
	// the number of open requests.
	requestIDs := make([]string, 0, len(requests))
	requesterIDSet := make(map[string]struct{}, len(requests))
	for _, req := range requests {
		requestIDs = append(requestIDs, req.Id)
		requesterIDSet[req.RequesterId] = struct{}{}
	}
	primaryCommunityIDs, err := r.world.GetPrimaryCommunityIDsForRequests(ctx, requestIDs)
	if err != nil {
		logger.WarnContext(ctx, "batch community lookup failed; skipping reconcile tick", "error", err)
		return nil, fmt.Errorf("batch community lookup: %w", err)
	}
	requesterIDs := make([]string, 0, len(requesterIDSet))
	for uid := range requesterIDSet {
		requesterIDs = append(requesterIDs, uid)
	}
	timezones := r.world.GetUserTimezones(ctx, requesterIDs)

	maxOffset := requestFollowupOffsets[len(requestFollowupOffsets)-1]
	now := clock.UnixSec(ctx)

	var out []*models.ScheduledNotification
	for _, req := range requests {
		if req.GetDeleted() != nil {
			continue
		}
		if req.RequesterId == "" {
			continue
		}
		if req.CreatedAtUnixSec == 0 {
			// Pre-existing row without a creation timestamp; backlog
			// grace would pass anyway but skip explicitly for clarity.
			continue
		}
		createdAt := req.CreatedAtUnixSec

		// Drop requests past the backlog-grace window so the cycling-
		// reminder pattern doesn't re-emit rows indefinitely for old
		// unsatisfied requests.
		if createdAt+maxOffset < now-requestBacklogGrace {
			continue
		}

		communityID := primaryCommunityIDs[req.Id]
		tz := timezones[req.RequesterId]

		for _, offset := range requestFollowupOffsets {
			fireAt := createdAt + offset
			if fireAt < now-DesiredFireAtGrace {
				continue
			}
			out = append(out, buildRequestFollowupRow(req, communityID, tz, fireAt, offset))
		}
	}
	return out, nil
}

func buildRequestFollowupRow(req *models.Request, communityID, tz string, fireAt, offset int64) *models.ScheduledNotification {
	row := &models.ScheduledNotification{
		RecipientUserId: req.RequesterId,
		FireAtUnixSec:   fireAt,
		Item: &models.ScheduledNotification_Request{
			Request: &models.RequestNotification{
				RequestId:               req.Id,
				Purpose:                 models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT,
				OffsetSecondsFromAnchor: offset,
			},
		},
	}
	if communityID != "" {
		cid := communityID
		row.CommunityId = &cid
	}
	if tz != "" {
		tzCopy := tz
		row.QuietHoursTimezone = &tzCopy
	}
	return row
}

// RequestFollowupDispatcher renders the push payload for a
// request-anchored followup-prompt row at dispatch time.
type RequestFollowupDispatcher struct {
	world RequestWorld
}

// NewRequestFollowupDispatcher wires the dispatcher against real storage.
func NewRequestFollowupDispatcher(s *storage.ProtoSQLStorage) *RequestFollowupDispatcher {
	return &RequestFollowupDispatcher{world: &storageRequestWorld{storage: s}}
}

// Name returns the stable identifier used in structured logs.
func (d *RequestFollowupDispatcher) Name() string {
	return "request_followup_prompts"
}

// CanHandle reports whether the row's item variant is a request
// followup-prompt row this dispatcher owns.
func (d *RequestFollowupDispatcher) CanHandle(row *models.ScheduledNotification) bool {
	req := row.GetRequest()
	if req == nil {
		return false
	}
	return req.Purpose == models.RequestNotificationPurpose_REQUEST_NOTIFICATION_PURPOSE_FOLLOWUP_PROMPT
}

// QuietHoursPolicy returns Defer. The prompt is day-grained; sliding
// it to 07:00 local doesn't change its meaning.
func (d *RequestFollowupDispatcher) QuietHoursPolicy(_ *models.ScheduledNotification) QuietHoursPolicy {
	return QuietHoursPolicyDefer
}

// Render reads the current Request and builds the followup-prompt
// Notification. Returns (nil, nil) when the request is no longer
// open, is soft-deleted, its community is soft-deleted, or the
// requester has opted out. Does not emit a community event or story
// (the push is private to the requester).
func (d *RequestFollowupDispatcher) Render(ctx context.Context, row *models.ScheduledNotification) (*models.Notification, error) {
	reqItem := row.GetRequest()
	if reqItem == nil {
		return nil, fmt.Errorf("row missing request variant")
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationDispatch",
		"dispatcher", d.Name(),
		"target_request_id", reqItem.RequestId,
		"offset_seconds_from_anchor", reqItem.OffsetSecondsFromAnchor,
		"recipient_user_id", row.RecipientUserId,
	)

	request, err := d.world.GetRequest(ctx, reqItem.RequestId)
	if err != nil {
		logger.InfoContext(ctx, "anchor Request not loadable; skipping send", "error", err)
		return nil, nil
	}
	if request.GetDeleted() != nil {
		logger.InfoContext(ctx, "anchor Request soft-deleted; skipping send")
		return nil, nil
	}
	if !isRequestOpen(request.State) {
		logger.InfoContext(ctx, "request no longer open; skipping send",
			"request_state", request.State.String(),
		)
		return nil, nil
	}

	communityID := row.GetCommunityId()
	if communityID != "" {
		community, err := d.world.GetCommunity(ctx, communityID)
		if err != nil {
			logger.InfoContext(ctx, "community not loadable; skipping send", "error", err)
			return nil, nil
		}
		if community.GetDeleted() != nil {
			logger.InfoContext(ctx, "community soft-deleted; skipping send")
			return nil, nil
		}
	}

	// Per-community preference gate. Fail open on storage error so a
	// transient DB hiccup doesn't suppress a real prompt — mirrors the
	// ExperienceReminderDispatcher posture.
	if communityID != "" {
		prefs, err := d.world.GetCommunityNotificationPreferences(ctx, row.RecipientUserId, communityID)
		if err != nil {
			logger.WarnContext(ctx, "preference lookup failed; failing open and delivering",
				"community_id", communityID,
				"error", err,
			)
		} else if prefs != nil && prefs.NotifyRequestFollowupPrompts != nil && !*prefs.NotifyRequestFollowupPrompts {
			logger.InfoContext(ctx, "recipient has disabled request followup prompts; skipping",
				"community_id", communityID,
			)
			return nil, nil
		}
	}

	requestID := request.Id
	// The copy no longer states how many days ago the request was posted: the
	// day count lives only in the row's offset, and the payload has no field
	// for it, so the off-app renderers could not reproduce the sentence. One
	// wording that every surface can render beats a push-only wording that
	// leaves SMS and email with nothing (#2896).
	return notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		CommunityId:  communityID,
		EventType:    "REQUEST_FOLLOWUP_PROMPT",
		RequestId:    &requestID,
		RequestTitle: request.Title,
	}), nil
}

// isRequestOpen reports whether the given state is one where followup
// prompts should be sent. Only ACTIVE and OFFERS_RECEIVED qualify.
func isRequestOpen(state models.RequestState) bool {
	switch state {
	case models.RequestState_REQUEST_STATE_ACTIVE,
		models.RequestState_REQUEST_STATE_OFFERS_RECEIVED:
		return true
	}
	return false
}

// storageRequestWorld is the production RequestWorld backed by
// ProtoSQLStorage.
type storageRequestWorld struct {
	storage *storage.ProtoSQLStorage
}

func (w *storageRequestWorld) ListOpenRequests(ctx context.Context) ([]*models.Request, error) {
	active, err := w.storage.QueryByField(ctx, "state",
		fmt.Sprintf("%d", int(models.RequestState_REQUEST_STATE_ACTIVE)),
		&models.Request{})
	if err != nil {
		return nil, fmt.Errorf("query active requests: %w", err)
	}
	offersReceived, err := w.storage.QueryByField(ctx, "state",
		fmt.Sprintf("%d", int(models.RequestState_REQUEST_STATE_OFFERS_RECEIVED)),
		&models.Request{})
	if err != nil {
		return nil, fmt.Errorf("query offers_received requests: %w", err)
	}
	out := make([]*models.Request, 0, len(active)+len(offersReceived))
	for _, m := range active {
		out = append(out, m.(*models.Request))
	}
	for _, m := range offersReceived {
		out = append(out, m.(*models.Request))
	}
	return out, nil
}

func (w *storageRequestWorld) GetRequest(ctx context.Context, requestID string) (*models.Request, error) {
	r := &models.Request{}
	if err := w.storage.GetByID(ctx, requestID, r); err != nil {
		return nil, err
	}
	return r, nil
}

// GetPrimaryCommunityIDsForRequests fetches the earliest non-archived
// CommunityRequest community for each request in one query, avoiding
// N+1 per-request lookups. "Primary" is defined as the non-archived
// CommunityRequest with the smallest shared_at_unix_sec.
func (w *storageRequestWorld) GetPrimaryCommunityIDsForRequests(ctx context.Context, requestIDs []string) (map[string]string, error) {
	if len(requestIDs) == 0 {
		return map[string]string{}, nil
	}
	msgs, err := w.storage.QueryByFieldIn(ctx, "request_id", requestIDs, &models.CommunityRequest{})
	if err != nil {
		return nil, err
	}
	// For each request, pick the non-archived row with the earliest shared_at.
	type entry struct {
		communityID string
		sharedAt    int64
	}
	best := make(map[string]entry, len(requestIDs))
	for _, m := range msgs {
		cr := m.(*models.CommunityRequest)
		if cr.GetDeleted() != nil {
			continue
		}
		if cr.Archived {
			continue
		}
		prev, ok := best[cr.RequestId]
		if !ok || cr.SharedAtUnixSec < prev.sharedAt {
			best[cr.RequestId] = entry{communityID: cr.CommunityId, sharedAt: cr.SharedAtUnixSec}
		}
	}
	out := make(map[string]string, len(best))
	for rid, e := range best {
		out[rid] = e.communityID
	}
	return out, nil
}

func (w *storageRequestWorld) GetCommunity(ctx context.Context, communityID string) (*models.Community, error) {
	c := &models.Community{}
	if err := w.storage.GetByID(ctx, communityID, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (w *storageRequestWorld) GetUserTimezones(ctx context.Context, userIDs []string) map[string]string {
	if len(userIDs) == 0 {
		return map[string]string{}
	}
	userMap, err := w.storage.GetByIDs(ctx, userIDs, &models.User{})
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(userMap))
	for id, m := range userMap {
		out[id] = m.(*models.User).PreferredTimezone
	}
	return out
}

func (w *storageRequestWorld) GetCommunityNotificationPreferences(ctx context.Context, userID, communityID string) (*models.CommunityNotificationPreferences, error) {
	if userID == "" || communityID == "" {
		return nil, nil
	}
	prefsByUser, err := communitylib.FetchPreferencesForUsers(ctx, w.storage, communityID, []string{userID})
	if err != nil {
		return nil, err
	}
	return prefsByUser[userID], nil
}
