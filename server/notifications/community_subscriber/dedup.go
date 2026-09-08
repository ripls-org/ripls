package community_subscriber

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// dedupTTL bounds how long a (request_id, event_type, item_id, user_id) key is
// retained after a push is sent. The per-action fan-out burst that this guard
// collapses completes within ~1-2 seconds in production, so the TTL exists only
// to cap memory — never for correctness, since a request_id is unique per user
// action and can never recur for a genuinely separate action.
const dedupTTL = 5 * time.Minute

// dedupPruneThreshold is the entry count above which seenOrAdd sweeps expired
// keys. Below it, the map is small enough that pruning would cost more than it
// saves.
const dedupPruneThreshold = 256

// dedupSet is a thread-safe, TTL-bounded set of string keys used to suppress
// duplicate push notifications that arise when one user action fans out into
// multiple per-community CommunityEvents (see #2088). The bus dispatches each
// sibling event on its own goroutine, so all access is mutex-guarded and the
// check-and-reserve is atomic.
type dedupSet struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]time.Time // key -> expiry instant
}

// newDedupSet returns an empty dedupSet with the given TTL and a wall-clock
// time source. Tests inject a fake clock via the now field.
func newDedupSet(ttl time.Duration) *dedupSet {
	return &dedupSet{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]time.Time),
	}
}

// seenOrAdd atomically reports whether key is already present and unexpired. If
// it is, seenOrAdd returns true (the caller is a duplicate and must suppress).
// Otherwise it reserves key with a fresh expiry and returns false. Reserving at
// the decision point — rather than after the send completes — closes the race
// where two sibling events both pass the check before either records. Callers
// that fail to deliver must call remove so a sibling can retry.
func (d *dedupSet) seenOrAdd(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	if exp, ok := d.entries[key]; ok && exp.After(now) {
		return true
	}
	d.entries[key] = now.Add(d.ttl)
	d.pruneLocked(now)
	return false
}

// remove drops key from the set. Called when a reserved send ultimately fails,
// so a sibling event for the same action can attempt delivery instead of being
// silently suppressed.
func (d *dedupSet) remove(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, key)
}

// pruneLocked sweeps expired entries when the map has grown past the threshold.
// The caller must hold d.mu.
func (d *dedupSet) pruneLocked(now time.Time) {
	if len(d.entries) < dedupPruneThreshold {
		return
	}
	for k, exp := range d.entries {
		if !exp.After(now) {
			delete(d.entries, k)
		}
	}
}

// dedupKey builds the per-recipient duplicate-suppression key for an event, or
// "" when the event can't (or needn't) be deduped. It returns "" when there is
// no propagated request_id to scope the user action — background-job publishers
// and any non-RPC path then notify exactly as before — and when the event
// carries no item id, since those event types are single-community and cannot
// fan out into duplicates. Including event_type and item_id keeps genuinely
// distinct notifications emitted by the same RPC from collapsing into one.
func dedupKey(ctx context.Context, event *models.CommunityEvent, userID string) string {
	rid := logging.RequestIDFromContext(ctx)
	if rid == "" {
		return ""
	}
	itemID := eventItemID(event)
	if itemID == "" {
		return ""
	}
	return strings.Join([]string{rid, event.EventType.String(), itemID, userID}, "|")
}

// eventItemID returns the kind-prefixed entity id an event is about, preferring
// experience, then request, then gear — the same priority the client uses to
// route a notification tap. Returns "" when the event references no such item.
func eventItemID(event *models.CommunityEvent) string {
	if id := event.GetExperienceId(); id != "" {
		return "experience:" + id
	}
	if id := event.GetRequestId(); id != "" {
		return "request:" + id
	}
	if event.GearId != "" {
		return "gear:" + event.GearId
	}
	return ""
}
