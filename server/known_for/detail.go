package known_for

import (
	"context"
	"fmt"
	"sort"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// MaxDetailItems caps the number of items the Detail response carries.
// The detail screen renders a single column; ten is enough to convey
// "what this scope is doing in this category" without padding the
// screen with dim recency-only entries.
const MaxDetailItems = 10

// MaxDetailMembers caps the number of de-duplicated member IDs the
// Detail response carries. Twenty-four fits roughly three rows of
// circular avatars without spilling.
const MaxDetailMembers = 24

// DetailKind discriminates the entity backing a DetailItem so the
// service-layer wiring can pick the right title / media id and emit
// the correct [api.ItemKind] without re-reflecting on the type.
type DetailKind int

const (
	// DetailKindGear is a gear listing.
	DetailKindGear DetailKind = iota
	// DetailKindExperience is an experience (community event).
	DetailKindExperience
	// DetailKindRequest is a help request.
	DetailKindRequest
)

// DetailOptions extends [Options] with the category filter and the
// pre-loaded suppression key set. Mode / OwnerID / CommunityIDs carry
// the same semantics as for [Derive].
type DetailOptions struct {
	// Mode picks per-user vs per-community aggregation. Required.
	Mode Mode

	// OwnerID is the target user for ModePerUser. Required when
	// Mode == ModePerUser; ignored otherwise.
	OwnerID string

	// CommunityIDs scopes the result. Empty slice yields an empty
	// result.
	CommunityIDs []string

	// Category is the chip's display string or normalized key. The
	// filter compares the normalized form so display-case and
	// whitespace differences don't drop matches.
	Category string

	// SuppressedKeys is the set of normalized category keys the caller
	// has resolved as suppressed for the requested scope. Callers that
	// have not yet wired suppression pass an empty map; callers backed
	// by the suppressed_known_for table populate it. When the requested
	// Category's normalized form is in the set, [Detail] returns
	// Suppressed: true without doing any further work.
	SuppressedKeys map[string]struct{}
}

// DetailItem is one entity entry in the result. The fields are the
// minimum the workshop-service wiring needs to build an [api.Item]:
// the entity itself (so the caller can pull title / media), a
// discriminator, and the recency timestamp used to rank.
type DetailItem struct {
	Kind        DetailKind
	Gear        *models.Gear
	Experience  *models.Experience
	Request     *models.Request
	RecencySecs int64
}

// DetailResult is the in-package return shape for [Detail]. Items are
// already sorted newest-first and capped at [MaxDetailItems];
// MemberIDs are de-duplicated and capped at [MaxDetailMembers].
type DetailResult struct {
	Items      []DetailItem
	MemberIDs  []string
	Suppressed bool
}

// Detail returns the representative items and the people behind them
// for a single capability tag. Reuses the same scoped loaders as
// [Derive] so the chip query and the detail query stay in lock-step.
//
// Returns an empty (non-nil) [DetailResult] when the scope is empty.
// Returns DetailResult{Suppressed: true} when the requested category
// is in [DetailOptions.SuppressedKeys] — no items, no members.
//
// Query budget: matches the chip-derivation ceiling at 7 reads.
func Detail(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts DetailOptions,
) (DetailResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DetailKnownFor",
		"community_id_count", len(opts.CommunityIDs),
		"mode", modeName(opts.Mode),
		"category", opts.Category,
	)

	if len(opts.CommunityIDs) == 0 {
		return DetailResult{Items: []DetailItem{}, MemberIDs: []string{}}, nil
	}
	if opts.Mode == ModePerUser && opts.OwnerID == "" {
		return DetailResult{}, fmt.Errorf("detail known_for: per-user mode requires OwnerID")
	}

	targetKey := normalizeCategoryKey(opts.Category)
	if targetKey == "" {
		return DetailResult{Items: []DetailItem{}, MemberIDs: []string{}}, nil
	}
	if _, hit := opts.SuppressedKeys[targetKey]; hit {
		logger.DebugContext(ctx, "detail: category suppressed for scope")
		return DetailResult{
			Items:      []DetailItem{},
			MemberIDs:  []string{},
			Suppressed: true,
		}, nil
	}

	sharedSet := make(map[string]struct{}, len(opts.CommunityIDs))
	for _, id := range opts.CommunityIDs {
		sharedSet[id] = struct{}{}
	}

	derive := Options{
		Mode:         opts.Mode,
		OwnerID:      opts.OwnerID,
		CommunityIDs: opts.CommunityIDs,
	}

	items := make([]DetailItem, 0, MaxDetailItems)
	memberSet := make(map[string]struct{})

	gearItems, gearMembers, err := collectGearDetail(ctx, s, derive, sharedSet, targetKey)
	if err != nil {
		return DetailResult{}, fmt.Errorf("collect gear: %w", err)
	}
	items = append(items, gearItems...)
	for _, m := range gearMembers {
		memberSet[m] = struct{}{}
	}

	expItems, expMembers, err := collectExperienceDetail(ctx, s, derive, sharedSet, targetKey)
	if err != nil {
		return DetailResult{}, fmt.Errorf("collect experiences: %w", err)
	}
	items = append(items, expItems...)
	for _, m := range expMembers {
		memberSet[m] = struct{}{}
	}

	reqItems, reqMembers, err := collectRequestDetail(ctx, s, derive, sharedSet, targetKey)
	if err != nil {
		return DetailResult{}, fmt.Errorf("collect requests: %w", err)
	}
	items = append(items, reqItems...)
	for _, m := range reqMembers {
		memberSet[m] = struct{}{}
	}

	// On per-user mode the target's own id is never a "person they
	// shared with"; their avatar already lives in the surrounding
	// profile screen.
	if opts.Mode == ModePerUser {
		delete(memberSet, opts.OwnerID)
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].RecencySecs > items[j].RecencySecs
	})
	if len(items) > MaxDetailItems {
		items = items[:MaxDetailItems]
	}

	members := make([]string, 0, len(memberSet))
	for id := range memberSet {
		members = append(members, id)
	}
	sort.Strings(members)
	if len(members) > MaxDetailMembers {
		members = members[:MaxDetailMembers]
	}

	logger.DebugContext(ctx, "detail: collected",
		"item_count", len(items),
		"member_count", len(members),
	)
	return DetailResult{Items: items, MemberIDs: members}, nil
}

// collectGearDetail walks the same loanInputs the chip derivation
// uses, picks gear whose category matches [targetKey], and returns
// the qualifying items plus the contributing member IDs. Gear
// recency is the gear's CreatedAtUnixSec.
func collectGearDetail(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	targetKey string,
) ([]DetailItem, []string, error) {
	inputs, err := loadLoanInputs(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, nil, err
	}
	if len(inputs.gearByID) == 0 {
		return nil, nil, nil
	}

	seen := make(map[string]struct{}, len(inputs.sharedGearIDs))
	items := make([]DetailItem, 0, len(inputs.sharedGearIDs))
	members := make([]string, 0)

	addGear := func(g *models.Gear) {
		if g == nil {
			return
		}
		if _, ok := seen[g.Id]; ok {
			return
		}
		if normalizeCategoryKey(gearCategoryString(g)) != targetKey {
			return
		}
		seen[g.Id] = struct{}{}
		items = append(items, DetailItem{
			Kind:        DetailKindGear,
			Gear:        g,
			RecencySecs: g.CreatedAtUnixSec,
		})
		if g.OwnerId != "" {
			members = append(members, g.OwnerId)
		}
	}

	for _, id := range inputs.sharedGearIDs {
		addGear(inputs.gearByID[id])
	}
	for _, t := range inputs.loanTransfers {
		addGear(inputs.gearByID[t.GearId])
		// Borrowers (loan recipients) are intentionally NOT added to
		// members. Borrowing isn't "showing up for" the category in
		// the same sense that owning, hosting, or helping is; the
		// per-user chip derivation never counts borrowing either, so
		// surfacing borrowers here would create a contributor whose
		// own profile carries no matching chip.
	}

	return items, members, nil
}

// collectExperienceDetail filters experiences in scope to those
// matching the requested category. On per-user mode the result
// includes both experiences the target hosted and experiences the
// target attended — symmetric with the chip-derivation counting.
// Cancelled experiences are skipped. Members for each matched item
// include the host plus every attendee (RSVP.attended == "YES").
//
// Recency picks the most recent of completed_at / started_at /
// scheduled start time.
func collectExperienceDetail(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	targetKey string,
) ([]DetailItem, []string, error) {
	experiences, err := loadScopedExperiences(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, nil, err
	}
	pool := make(map[string]*models.Experience, len(experiences))
	for id, e := range experiences {
		pool[id] = e
	}
	if opts.Mode == ModePerUser {
		attended, err := loadAttendedScopedExperiencesByUser(ctx, s, opts.OwnerID, sharedSet)
		if err != nil {
			return nil, nil, err
		}
		for id, e := range attended {
			if _, dup := pool[id]; dup {
				continue
			}
			pool[id] = e
		}
	}

	items := make([]DetailItem, 0)
	members := make([]string, 0)
	matchedIDs := make([]string, 0)
	for _, e := range pool {
		if e == nil {
			continue
		}
		if e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			continue
		}
		if normalizeCategoryKey(e.Category) != targetKey {
			continue
		}
		items = append(items, DetailItem{
			Kind:        DetailKindExperience,
			Experience:  e,
			RecencySecs: experienceRecency(e),
		})
		matchedIDs = append(matchedIDs, e.Id)
		if e.OwnerId != "" {
			members = append(members, e.OwnerId)
		}
	}

	attendeesByExp, err := loadAttendeesByExperience(ctx, s, matchedIDs, sharedSet)
	if err != nil {
		return nil, nil, err
	}
	for _, attendees := range attendeesByExp {
		members = append(members, attendees...)
	}

	return items, members, nil
}

// collectRequestDetail filters requests in scope to those matching
// the requested category. On per-user mode the result includes both
// requests the target filed and requests the target helped fulfill —
// symmetric with the chip-derivation counting. Cancelled requests
// are skipped. Members for each matched item include the requester
// plus every confirmed helper.
//
// Recency picks the fulfilled timestamp when present, otherwise
// zero — live requests sort newest-first on the storage's natural
// order via the stable sort.
func collectRequestDetail(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	targetKey string,
) ([]DetailItem, []string, error) {
	requests, err := loadScopedRequests(ctx, s, opts, sharedSet)
	if err != nil {
		return nil, nil, err
	}
	pool := make(map[string]*models.Request, len(requests))
	for id, r := range requests {
		pool[id] = r
	}
	if opts.Mode == ModePerUser {
		helped, err := loadHelpedScopedRequestsByUser(ctx, s, opts.OwnerID, opts, sharedSet)
		if err != nil {
			return nil, nil, err
		}
		for id, r := range helped {
			if _, dup := pool[id]; dup {
				continue
			}
			pool[id] = r
		}
	}

	items := make([]DetailItem, 0)
	members := make([]string, 0)
	for _, r := range pool {
		if r == nil {
			continue
		}
		if r.State == models.RequestState_REQUEST_STATE_CANCELLED {
			continue
		}
		if normalizeCategoryKey(r.Category) != targetKey {
			continue
		}
		items = append(items, DetailItem{
			Kind:        DetailKindRequest,
			Request:     r,
			RecencySecs: requestRecency(r),
		})
		if r.RequesterId != "" {
			members = append(members, r.RequesterId)
		}
		for _, helper := range r.ConfirmedHelperIds {
			if helper != "" {
				members = append(members, helper)
			}
		}
	}
	return items, members, nil
}

// experienceRecency picks the most recent meaningful timestamp on an
// experience. Falls back to 0 when the experience has no lifecycle
// timestamps and no scheduled start.
func experienceRecency(e *models.Experience) int64 {
	var best int64
	if e.CompletedAtUnixSec != nil && *e.CompletedAtUnixSec > best {
		best = *e.CompletedAtUnixSec
	}
	if e.StartedAtUnixSec != nil && *e.StartedAtUnixSec > best {
		best = *e.StartedAtUnixSec
	}
	if e.Time != nil {
		if sp := e.Time.GetSpecific(); sp != nil && sp.UnixTimestampSec > best {
			best = sp.UnixTimestampSec
		}
		if rg := e.Time.GetRange(); rg != nil && rg.StartUnixSec > best {
			best = rg.StartUnixSec
		}
	}
	return best
}

// requestRecency picks the fulfilled timestamp when present. Live
// requests carry no creation timestamp on the entity, so we treat
// them as 0 — stable sort keeps the storage order, which is typically
// recent-first.
func requestRecency(r *models.Request) int64 {
	if r.FulfilledAtUnixSec != nil {
		return *r.FulfilledAtUnixSec
	}
	return 0
}
