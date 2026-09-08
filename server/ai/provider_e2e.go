package ai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.ripls.org/ripls/server/clock"
)

// NewE2EDeterministicProvider returns a MockProvider configured to emit
// deterministic, immediately-saveable content for the unified-create flow, so
// end-to-end tests can drive the REAL create UI (prompt → classify → stream →
// preview → Save) without a live LLM.
//
// It differs from the bare NewMockProvider in two ways the create UI needs:
//   - the unified-create classifier picks a type from a simple keyword heuristic
//     (the bare mock always returns "gear"); and
//   - generated content has a non-empty title AND description (the bare mock's
//     default experience generation leaves the description empty, which the
//     client rejects as not-yet-saveable).
//
// Wired into the server via --mock-ai-provider. e2e/dev only — never production.
func NewE2EDeterministicProvider() *MockProvider {
	m := NewMockProvider()
	m.NameValue = "e2e-deterministic"

	m.ClassifyUnifiedCreateFunc = func(_ context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
		// Image mode sends no prompt text ("exactly one input"), so the
		// keyword heuristic can't run — and a photographed object is an
		// item, matching the real classifier's strong prior (#2687).
		if in.Image != nil {
			return &UnifiedCreateClassification{Type: UnifiedCreateContentTypeGear}, nil
		}
		return &UnifiedCreateClassification{Type: classifyE2EText(in.Text)}, nil
	}

	// Image-mode gear detection keyed on the uploaded filename, so the
	// walkthrough reels (#2687) get deterministic, on-camera-worthy
	// listings for the committed food fixtures instead of "Mock Gear
	// Item". Unknown filenames keep the bare mock's default content.
	m.DetectGearFunc = func(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
		if d := e2eGearDetectionForFilename(req.Filename); d != nil {
			return d, nil
		}
		return defaultDetectGear(ctx, req)
	}
	m.GenerateExperienceFromTextFunc = func(ctx context.Context, prompt, _, _ string) (*ExperienceGeneration, error) {
		// Deterministic date/time extraction so the create preview shows the
		// real detection smarts on camera (walkthrough reels, #2684): a prompt
		// like "… Sunday May 10 at 10:30am …" pre-fills the time card. An
		// empty LocationQuery lets the server fall back to the host's primary
		// residence ("at our place") without a geocoding provider.
		//
		// Resolve relative dates ("this Sunday") against the caller's clock,
		// not the wall clock, so a caller filming a fixed date gets the day
		// that date implies rather than today's.
		date := e2eExtractDate(prompt, clock.Now(ctx))
		clock := e2eExtractClock(prompt)
		confidence := "UNKNOWN"
		if date != "" {
			confidence = "EXPLICIT"
		}
		return &ExperienceGeneration{
			Title:          e2eTitle(prompt, "E2E Event"),
			Description:    e2eDescription(prompt),
			Confidence:     0.95,
			SearchKeywords: []string{"event", "gathering"},
			Date:           date,
			Time:           clock,
			TimeConfidence: confidence,
		}, nil
	}

	// Text-mode request generation echoes the request (title = first sentence,
	// description = the whole prompt) so the create preview reads like a
	// real request on camera (requests walkthrough, #2686) — the bare
	// mock's "Mock Request" placeholder would sink the scene. An empty
	// LocationQuery keeps the server's primary-residence fallback, matching
	// the experience override above.
	m.GenerateRequestFunc = func(_ context.Context, prompt, _ string) (*RequestGeneration, error) {
		return &RequestGeneration{
			Title:          e2eTitle(prompt, "E2E Request"),
			Description:    e2eDescription(prompt),
			Confidence:     0.95,
			SearchKeywords: []string{"request", "help", "neighbors"},
			ValueEstimate: &ValueEstimate{
				EstimatedValueUSD: 75,
				Confidence:        0.7,
				Reasoning:         "Typical replacement cost for an item like this",
				Sources:           []string{"AI estimation"},
			},
		}, nil
	}

	// The seeded need (#2702) lands on the request card in the walkthrough
	// reels, so its label must read like the asked-for thing — the bare
	// mock's "The thing" placeholder (or the full title fallback) would sink
	// the claim scene. Deterministic keyword scan over the request's text.
	m.GenerateRequestSuggestionsFunc = func(_ context.Context, title, description, _ string) (*RequestSuggestionResult, error) {
		return &RequestSuggestionResult{
			AdditionalAsks:  []string{"Extra hands", "Fuel topped up"},
			BreakdownPieces: []string{"Pick up", "Return run"},
			OfferIdeas:      []string{"Lend yours", "Offer a ride"},
			SeedNeeds:       e2eSeedNeeds(title + " " + description),
		}, nil
	}

	return m
}

// e2eGearDetectionForFilename returns canned, food-appropriate gear content
// for the committed walkthrough food fixtures (e2e/fixtures/walkthroughs/
// food-*.jpg), keyed on a substring of the uploaded filename. Content reads
// like a real photo-only generation: no brand/model, plausible weight and
// replacement value, no material category (food has none — the impact
// estimator falls back). Returns nil for unknown filenames.
func e2eGearDetectionForFilename(filename string) *GearDetection {
	f := strings.ToLower(filename)
	value := func(usd float32) *ValueEstimate {
		return &ValueEstimate{
			EstimatedValueUSD: usd,
			Confidence:        0.8,
			Reasoning:         "Typical grocery replacement cost",
			Sources:           []string{"AI estimation"},
		}
	}
	switch {
	case strings.Contains(f, "mower"):
		return &GearDetection{
			Title:       "Honda Self-Propelled Mower",
			Description: "Gas mower, starts on the first pull. Fresh blade this spring.",
			Category:    "Yard & Garden",
			WeightGrams: 38000,
			Confidence:  0.97,
			ValueEstimate: &ValueEstimate{
				EstimatedValueUSD: 420,
				Confidence:        0.8,
				Reasoning:         "Typical replacement cost for a self-propelled gas mower",
				Sources:           []string{"AI estimation"},
			},
		}
	case strings.Contains(f, "table"):
		// A lend-appropriate item for the event child-transfer flow (#2708):
		// a folding table brought to (and lent for) a gathering. Matches the
		// wooden folding tray table in folding-table.jpg.
		return &GearDetection{
			Title:       "Wooden Folding Table",
			Description: "Foldable wooden side table with a tray top. Packs flat and carries easily.",
			Category:    "Home & Furniture",
			WeightGrams: 6000,
			Confidence:  0.96,
			ValueEstimate: &ValueEstimate{
				EstimatedValueUSD: 45,
				Confidence:        0.8,
				Reasoning:         "Typical replacement cost for a wooden folding table",
				Sources:           []string{"AI estimation"},
			},
		}
	case strings.Contains(f, "spaghetti"):
		return &GearDetection{
			Title:         "Spaghetti (2 boxes)",
			Description:   "Two unopened boxes of dried spaghetti, straight from the pantry.",
			Category:      "Food",
			WeightGrams:   900,
			Confidence:    0.97,
			ValueEstimate: value(6),
		}
	case strings.Contains(f, "bread"):
		return &GearDetection{
			Title:         "Sourdough Loaf",
			Description:   "A whole crusty sourdough loaf, uncut and bakery-fresh.",
			Category:      "Food",
			WeightGrams:   700,
			Confidence:    0.96,
			ValueEstimate: value(7),
		}
	case strings.Contains(f, "broccoli"):
		return &GearDetection{
			Title:         "Fresh Broccoli",
			Description:   "Two crisp broccoli crowns, ready to steam or roast.",
			Category:      "Food",
			WeightGrams:   500,
			Confidence:    0.96,
			ValueEstimate: value(4),
		}
	}
	return nil
}

// classifyE2EText maps a prompt to a unified-create content type by keyword,
// defaulting to event. Mirrors the rough intent of the real classifier well
// enough for deterministic e2e runs.
func classifyE2EText(text string) UnifiedCreateContentType {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "lend"), strings.Contains(t, "borrow"),
		strings.Contains(t, "give away"), strings.Contains(t, "giveaway"),
		strings.Contains(t, "loan"):
		return UnifiedCreateContentTypeGear
	case strings.Contains(t, "need"), strings.Contains(t, "looking for"),
		strings.Contains(t, "anyone have"), strings.Contains(t, "request"):
		return UnifiedCreateContentTypeRequest
	default:
		return UnifiedCreateContentTypeEvent
	}
}

var (
	e2eDateRe = regexp.MustCompile(
		`(?i)\b(january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{1,2})\b`,
	)
	e2eWeekdayRe = regexp.MustCompile(
		`(?i)\b(sunday|monday|tuesday|wednesday|thursday|friday|saturday)\b`,
	)
	e2eWeekdays = map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
	}
	e2eClockRe = regexp.MustCompile(`(?i)\b(\d{1,2})(?::(\d{2}))?\s*(am|pm)\b`)

	e2eMonths = map[string]time.Month{
		"january": time.January, "february": time.February, "march": time.March,
		"april": time.April, "may": time.May, "june": time.June, "july": time.July,
		"august": time.August, "september": time.September, "october": time.October,
		"november": time.November, "december": time.December,
	}
)

// e2eExtractDate finds the first "<month name> <day>" in the prompt and
// returns it as YYYY-MM-DD in now's year (prompts name a real calendar date;
// the year is implicit). With no month-day, a bare weekday name ("this
// Sunday") resolves to its NEXT occurrence after now — walkthrough prompts
// use this so re-renders always film an upcoming event instead of one that
// drifts into the past (#2724). Empty when the prompt names no date.
func e2eExtractDate(prompt string, now time.Time) string {
	m := e2eDateRe.FindStringSubmatch(prompt)
	if m == nil {
		return e2eExtractWeekday(prompt, now)
	}
	month := e2eMonths[strings.ToLower(m[1])]
	var day int
	if _, err := fmt.Sscanf(m[2], "%d", &day); err != nil || day < 1 || day > 31 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", now.Year(), int(month), day)
}

// e2eExtractWeekday resolves the first weekday name in the prompt to its next
// occurrence strictly after now (a "Sunday" prompt on a Sunday means next
// week), formatted YYYY-MM-DD. Empty when the prompt names no weekday.
func e2eExtractWeekday(prompt string, now time.Time) string {
	m := e2eWeekdayRe.FindStringSubmatch(prompt)
	if m == nil {
		return ""
	}
	target := e2eWeekdays[strings.ToLower(m[1])]
	daysAhead := (int(target) - int(now.Weekday()) + 7) % 7
	if daysAhead == 0 {
		daysAhead = 7
	}
	d := now.AddDate(0, 0, daysAhead)
	return fmt.Sprintf("%04d-%02d-%02d", d.Year(), int(d.Month()), d.Day())
}

// e2eExtractClock finds the first "h[:mm]am/pm" in the prompt and returns it
// as 24h HH:MM. Empty when the prompt names no clock time.
func e2eExtractClock(prompt string) string {
	m := e2eClockRe.FindStringSubmatch(prompt)
	if m == nil {
		return ""
	}
	var hour, minute int
	if _, err := fmt.Sscanf(m[1], "%d", &hour); err != nil || hour < 1 || hour > 12 {
		return ""
	}
	if m[2] != "" {
		if _, err := fmt.Sscanf(m[2], "%d", &minute); err != nil || minute > 59 {
			return ""
		}
	}
	if strings.EqualFold(m[3], "pm") && hour != 12 {
		hour += 12
	}
	if strings.EqualFold(m[3], "am") && hour == 12 {
		hour = 0
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// e2eTitle derives a short, non-empty title from the prompt (first clause,
// capped at 60 chars), falling back to fallback when the prompt is empty.
// e2eSeedNeeds extracts every claimable thing a request's text plainly names,
// via a deterministic keyword scan, mirroring what the real LLM's seed_needs
// field produces (["Lawn mower"] from "Looking for a lawn mower this weekend";
// ["Picture books", "Whiteboard", …] from a classroom supply list). Matches are
// returned in the order they appear in the text — most-important-first only to
// the extent the requester wrote them that way — so the first entry seeds the
// request's primary need. Returns an empty slice when the text names nothing it
// recognizes, which leaves the request with an empty needs list for its author
// to fill (#2731).
func e2eSeedNeeds(text string) []string {
	lower := strings.ToLower(text)
	vocab := []string{
		"lawn mower", "pressure washer", "wheelbarrow", "folding table",
		"ladder", "pickup truck", "tent", "drill",
		// Classroom-supply-drive reel (#2703 / #2731): a back-to-school request
		// that enumerates its supply list is born with one need per item.
		"picture books", "whiteboard", "storage bins", "art supplies",
		"construction paper",
	}
	// Collect (position, label) for every vocab item present, then order by
	// first appearance in the text so the emitted list reads as written.
	type hit struct {
		pos   int
		label string
	}
	var hits []hit
	for _, item := range vocab {
		if pos := strings.Index(lower, item); pos >= 0 {
			hits = append(hits, hit{pos: pos, label: strings.ToUpper(item[:1]) + item[1:]})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	needs := make([]string, 0, len(hits))
	for _, h := range hits {
		needs = append(needs, h.label)
	}
	return needs
}

func e2eTitle(prompt, fallback string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return fallback
	}
	if i := strings.IndexAny(p, ".\n"); i > 0 {
		p = strings.TrimSpace(p[:i])
	}
	if len(p) > 60 {
		p = strings.TrimSpace(p[:60])
	}
	return p
}

// e2eDescription returns a saveable (≥10 char) description derived from the
// prompt, capped at 400 chars. The prompt's leading clause becomes the title
// (e2eTitle), so it is stripped here when a usable remainder follows — a
// description that re-opens with its own heading reads as a glitch on the
// create preview (#2724).
func e2eDescription(prompt string) string {
	p := strings.TrimSpace(prompt)
	if i := strings.IndexAny(p, ".\n"); i > 0 {
		if rest := strings.TrimLeft(strings.TrimSpace(p[i+1:]), "—–- "); len(rest) >= 10 {
			p = rest
		}
	}
	if len(p) < 10 {
		return "Created via the e2e unified-create flow."
	}
	if len(p) > 400 {
		p = strings.TrimSpace(p[:400])
	}
	return p
}
