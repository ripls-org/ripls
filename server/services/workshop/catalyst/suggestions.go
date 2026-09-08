package catalyst

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// RateLimitWindowDays is how far back we look for prior catalyst-pull
// messages to a recipient when enforcing the rate limit. Per the brief:
// 14 days.
const RateLimitWindowDays = 14

// RateLimitMaxPullsPerWindow is the cap on how many catalyst-pull
// messages a single recipient can receive across all hosts and circles
// inside the rate-limit window. Per the brief: 3.
const RateLimitMaxPullsPerWindow = 3

// DeclineCooldownDays is how long a declined catalyst-pull suppresses
// re-prompting for the same recurring rhythm. Per the brief: 30 days.
const DeclineCooldownDays = 30

// Suggestion is the result of running the full catalyst-pull pipeline
// for a (host, circle) pair: load detection + eligibility + rate limit +
// decline cooldown.
//
// Lopsided is the LoadSignal output. SuggestedUserID is the chosen
// catalyst recipient (empty when no eligible recipient passes the rate
// limit and cooldown gates). NoEligibleRecipient is set true when the
// load is lopsided but everyone is filtered out — the Workshop should
// surface a seed-the-catalyst nudge instead of a load overlay.
type Suggestion struct {
	LoadSignal          LoadSignal
	SuggestedUserID     string
	NoEligibleRecipient bool
}

// GetSuggestion runs the full catalyst-pull pipeline.
//
// Steps, in order:
//
//  1. DetectLoad: is the circle's hosting load lopsided?
//  2. If not lopsided, return a non-suggestion Suggestion (no overlay).
//  3. HostModeEligibleUserIDs: who in the circle has done an originating
//     action? (Excludes the host themself.)
//  4. Filter eligible candidates by rate limit + decline cooldown.
//  5. If no eligible candidate remains, set NoEligibleRecipient = true.
//
// The host parameter is the user viewing the Workshop (typically the
// over-loaded host themselves in the Case-B / host-as-Sarah variant).
func GetSuggestion(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityID, hostUserID string,
	now time.Time,
) (*Suggestion, error) {
	loadPtr, err := DetectLoad(ctx, store, communityID)
	if err != nil {
		return nil, fmt.Errorf("GetSuggestion: load detection: %w", err)
	}
	load := *loadPtr
	if !load.Lopsided {
		return &Suggestion{LoadSignal: load}, nil
	}

	candidates, err := HostModeEligibleUserIDs(ctx, store, communityID, hostUserID)
	if err != nil {
		return nil, fmt.Errorf("GetSuggestion: eligibility: %w", err)
	}
	if len(candidates) == 0 {
		return &Suggestion{LoadSignal: load, NoEligibleRecipient: true}, nil
	}

	for _, candidate := range candidates {
		rateLimited, err := IsRecipientRateLimited(ctx, store, candidate, now)
		if err != nil {
			continue // skip on error, try next candidate
		}
		if rateLimited {
			continue
		}
		cooldown, err := IsInDeclineCooldown(ctx, store, candidate, load.RhythmName, now)
		if err != nil {
			continue
		}
		if cooldown {
			continue
		}
		return &Suggestion{
			LoadSignal:      load,
			SuggestedUserID: candidate,
		}, nil
	}

	// All candidates filtered out — fall back to seed-the-catalyst.
	return &Suggestion{LoadSignal: load, NoEligibleRecipient: true}, nil
}

// CatalystPullStateActive is the recorded state for a catalyst-pull
// message that is in flight (sent but neither accepted nor declined).
const CatalystPullStateActive = "ACTIVE"

// CatalystPullStateAccepted is the recorded state for a catalyst-pull
// message the recipient accepted (they took on the host slot).
const CatalystPullStateAccepted = "ACCEPTED"

// CatalystPullStateDeclined is the recorded state for a catalyst-pull
// message the recipient declined.
const CatalystPullStateDeclined = "DECLINED"

// IsRecipientRateLimited returns true when the user has received
// RateLimitMaxPullsPerWindow or more catalyst-pull messages within the
// rate-limit window.
//
// v1 reads from a stub `CatalystPullRecord` storage type; the actual
// model proto is added when the catalyst-pull RPC plumbing lands. Today
// the function returns false (no rate limit applied) when the model is
// not yet registered, so it doesn't gate testing of the upstream
// suggestion logic.
func IsRecipientRateLimited(
	_ context.Context,
	_ *storage.ProtoSQLStorage,
	_ string,
	_ time.Time,
) (bool, error) {
	// Not yet implemented: the rate-limit guard needs a CatalystPullRecord
	// proto that does not exist yet. Once it does, query records where
	// recipient_user_id matches and sent_at_unix_sec > now -
	// RateLimitWindowDays, and return true when the count is >=
	// RateLimitMaxPullsPerWindow.
	//
	// For v1 we return false unconditionally so the upstream pipeline is
	// testable; the rate-limit guard comes online with the RPC.
	return false, nil
}

// IsInDeclineCooldown returns true when the user previously declined a
// catalyst-pull for the same rhythm within DeclineCooldownDays. Same v1
// stub treatment as IsRecipientRateLimited.
func IsInDeclineCooldown(
	_ context.Context,
	_ *storage.ProtoSQLStorage,
	_ string,
	_ string,
	_ time.Time,
) (bool, error) {
	// Not yet implemented, same v1 stub treatment as IsRecipientRateLimited.
	// Once the CatalystPullRecord proto exists, query records where
	// recipient_user_id matches AND rhythm_name matches AND state ==
	// DECLINED AND declined_at_unix_sec > now - DeclineCooldownDays, and
	// return true when any exists.
	return false, nil
}

// suppress unused-import warning for the models package; once the
// CatalystPullRecord proto exists, IsRecipientRateLimited and
// IsInDeclineCooldown will reference it directly.
var _ = (*models.User)(nil)
