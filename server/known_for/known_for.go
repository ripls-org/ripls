package known_for

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// CategoryThreshold is the minimum weight a single category (per
// source) must clear before surfacing as a tag. Each shared item
// contributes +1 to its category bucket; items in a completion
// state contribute +1 more. A threshold of 2 therefore surfaces a
// category when either:
//
//   - two distinct items share the same category, or
//   - one item shares the category *and* sits in a completion state
//     (e.g. an experience that has run, a request that was filled,
//     a piece of gear that has been loaned and returned).
const CategoryThreshold = 2

// MaxTags caps the merged tag list returned by [Derive]. The chip
// widget surfaces only the first two rows on screen and reveals the
// rest behind an expansion affordance, so a higher cap is safe: the
// user always sees the strongest tags up front and can opt into the
// long tail.
const MaxTags = 20

// Mode selects between per-user and per-community aggregation.
type Mode int

const (
	// ModePerUser counts events whose owner / requester matches
	// [Options.OwnerID] across the given communities. Tags describe
	// what one person is known for.
	ModePerUser Mode = iota

	// ModePerCommunity counts events by any owner / requester across
	// the given communities. Tags describe what the community (or
	// aggregate across communities) is known for.
	ModePerCommunity
)

// Options carries the parameters that vary by call site.
type Options struct {
	// Mode picks per-user vs per-community aggregation. Required.
	Mode Mode

	// OwnerID is the owner the per-user mode counts events for —
	// gear-owner for loans, host for experiences, requester for
	// requests. Required when Mode == ModePerUser; ignored otherwise.
	OwnerID string

	// CommunityIDs is the scope of communities considered. An empty
	// slice always returns an empty result regardless of Mode.
	CommunityIDs []string

	// SuppressedKeys is the set of normalized category keys that
	// should be dropped from the result. Callers that want
	// "suppressed_known_for"-aware behaviour load the set from
	// storage and pass it in here; callers that don't can leave it
	// nil to keep the legacy behaviour.
	SuppressedKeys map[string]struct{}
}

// rankedTag pairs a formatted display string with its repeat-count,
// used internally to sort merged candidates across sources.
type rankedTag struct {
	display string
	count   int
}

// Derive returns capability tags drawn from three sources, each
// counted across every item shared with the community scope (not
// just the items in a completion state):
//
//   - gear listed in scope                       → category from Gear
//   - experiences shared into scope              → category from Experience
//   - requests shared into scope                 → category from Request
//
// Every shared item adds +1 to its category bucket and every item
// that has reached its completion state adds an extra +1 —
// completed loans, COMPLETED experiences, FULFILLED requests. All
// three sources accumulate into a single counts map keyed on the
// (normalized) category string, so "Power Tools" gear listings,
// completed loans, and fulfilled requests collapse into one chip
// whose weight is the sum across sources. The final result is
// sorted by weight descending (alphabetic tie-break) and capped at
// [MaxTags].
//
// Returns an empty slice (never nil) when the community scope is
// empty, when no shared items exist, or when no category clears
// the threshold.
//
// Query budget: 7 reads when all three sources contribute (3 for
// gear / loans, 2 for experiences, 2 for requests). Tests should
// wrap calls in [storage.AssertMaxQueries] with max=7.
func Derive(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
) ([]string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeriveKnownFor",
		"community_id_count", len(opts.CommunityIDs),
		"mode", modeName(opts.Mode),
	)

	if len(opts.CommunityIDs) == 0 {
		return []string{}, nil
	}
	if opts.Mode == ModePerUser && opts.OwnerID == "" {
		return nil, fmt.Errorf("derive known_for: per-user mode requires OwnerID")
	}

	sharedSet := make(map[string]struct{}, len(opts.CommunityIDs))
	for _, id := range opts.CommunityIDs {
		sharedSet[id] = struct{}{}
	}

	counts := newCategoryCounts()
	if err := addLoanCounts(ctx, s, opts, sharedSet, counts); err != nil {
		return nil, fmt.Errorf("loan counts: %w", err)
	}
	if err := addHostCounts(ctx, s, opts, sharedSet, counts); err != nil {
		return nil, fmt.Errorf("host counts: %w", err)
	}
	if err := addAskerCounts(ctx, s, opts, sharedSet, counts); err != nil {
		return nil, fmt.Errorf("asker counts: %w", err)
	}

	merged := counts.aboveThreshold()
	// Drop suppressed categories before ranking. Suppression is keyed
	// on the same normalized form the counter uses.
	if len(opts.SuppressedKeys) > 0 {
		kept := merged[:0]
		for _, t := range merged {
			if _, hit := opts.SuppressedKeys[normalizeCategoryKey(t.display)]; hit {
				continue
			}
			kept = append(kept, t)
		}
		merged = kept
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].count != merged[j].count {
			return merged[i].count > merged[j].count
		}
		return merged[i].display < merged[j].display
	})
	if len(merged) > MaxTags {
		merged = merged[:MaxTags]
	}

	tags := make([]string, 0, len(merged))
	for _, t := range merged {
		tags = append(tags, t.display)
	}
	logger.DebugContext(ctx, "known_for: derived tags",
		"tag_count", len(tags),
	)
	return tags, nil
}

// categoryCounts groups events by their normalized category key
// while preserving the display-case of the first-seen category
// label. Used by every source-specific candidate builder.
type categoryCounts map[string]*categoryEntry

// categoryEntry is the inner record stored by [categoryCounts] —
// the display label seen on first occurrence and the running count.
type categoryEntry struct {
	display string
	count   int
}

// newCategoryCounts allocates an empty tally.
func newCategoryCounts() categoryCounts {
	return make(categoryCounts)
}

// add bumps the count for the given raw category string. An empty
// or whitespace-only category is silently skipped — items with no
// category can't contribute to any tag.
func (c categoryCounts) add(raw string) {
	key := normalizeCategoryKey(raw)
	if key == "" {
		return
	}
	entry, ok := c[key]
	if !ok {
		entry = &categoryEntry{display: titleCase(raw)}
		c[key] = entry
	}
	entry.count++
}

// aboveThreshold returns the entries whose count >= [CategoryThreshold]
// as [rankedTag]s using the bare category label as the display
// string (no role suffix). Ordering of the returned slice is
// unspecified — the orchestrator sorts the merged set.
func (c categoryCounts) aboveThreshold() []rankedTag {
	out := make([]rankedTag, 0, len(c))
	for _, e := range c {
		if e.count < CategoryThreshold {
			continue
		}
		out = append(out, rankedTag{
			display: e.display,
			count:   e.count,
		})
	}
	return out
}

func modeName(m Mode) string {
	if m == ModePerUser {
		return "per_user"
	}
	return "per_community"
}

// normalizeCategoryKey lower-cases and collapses whitespace so
// "Power Tools" and "power  tools" deduplicate to the same key.
func normalizeCategoryKey(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	prevSpace := false
	for _, r := range lower {
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}

// titleCase applies Title-Case to the first letter of each
// whitespace-separated word and lowercases the rest. ASCII-English
// only; non-Latin scripts pass through unchanged.
func titleCase(raw string) string {
	if raw == "" {
		return ""
	}
	words := strings.Fields(strings.TrimSpace(raw))
	for i, w := range words {
		runes := []rune(strings.ToLower(w))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
