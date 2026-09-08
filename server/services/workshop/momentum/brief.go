package momentum

// BriefPriority is a single ranked host priority for the daily brief.
// Populated from a StoredNudge row by the brief assembler in
// server/services/workshop.
type BriefPriority struct {
	// Slot is the cascade priority label (e.g., "active_quest",
	// "esm_repeat_signal", "calendar_gap").
	Slot string

	// EventName is the name of the experience or gear this priority
	// relates to. May be empty.
	EventName string

	// KickerLabel is the detector's template kicker copy.
	KickerLabel string

	// Headline is the detector's template headline.
	Headline string

	// Description is the detector's template description.
	Description string

	// AtmosphereLine is the detector's template atmosphere copy.
	AtmosphereLine string

	// CtaLabel is the detector's suggested lever label.
	CtaLabel string

	// CtaAction is the action vocabulary value (e.g., "schedule_repeat").
	CtaAction string

	// ContextID is the entity id the action applies to.
	ContextID string

	// CommunityID is the community this priority belongs to. Surfaced
	// to the wire on BriefCTARow rows so the client can render a
	// per-row circle chip.
	CommunityID string

	// CommunityName is the circle display name for multi-circle context.
	CommunityName string

	// AffirmCount and RespondentCount carry ESM aggregation when
	// Slot is esm_repeat_signal. Both 0 otherwise.
	AffirmCount     int
	RespondentCount int

	// RankScore is the ordering score assigned by BriefRanker.
	// Higher is more important. Not exposed over the wire. int64 so
	// the interest-tier boost can dominate the unix-second-range
	// recency tier without overflow.
	RankScore int64
}

// BriefSignal is the aggregated input the BriefProvider sees when
// generating the daily brief. Carries ranked priorities and season
// metrics.
type BriefSignal struct {
	// Priorities are the top 2-3 ranked host priorities, most important
	// first.
	Priorities []BriefPriority

	// TotalEventCount is the season-to-date completed-event count.
	TotalEventCount int

	// TotalReplacedCostUSD is the season-to-date replaced cost in USD.
	TotalReplacedCostUSD float64

	// TotalHoursTogether is the season-to-date together-time in hours.
	TotalHoursTogether float64

	// TotalCO2KeptOffRoadGrams is the season-to-date CO₂ avoided in
	// grams (matches `impact_metrics.ImpactSavingsResult.CarbonSavings`).
	// 0 when not available.
	TotalCO2KeptOffRoadGrams float64

	// TotalActsCount is the season-to-date count of community acts
	// across the requested scope. An act is one community participation
	// row tallied by `impact_metrics.Calculator.CountActs`.
	TotalActsCount int

	// TotalProblemsSolved is the "problems handled" numerator (X) across the
	// requested scope: completed loans + fulfilled requests + claimed needs.
	TotalProblemsSolved int

	// TotalProblemsPotential is the "problems handled" denominator (Y) across
	// the requested scope: all non-cancelled loan transfers + all non-cancelled
	// requests + all posted needs. Always >= TotalProblemsSolved.
	TotalProblemsPotential int

	// UniquePeopleCount is the number of distinct people across every
	// community in the requested scope. Someone who belongs to two
	// included communities is counted once.
	UniquePeopleCount int

	// KickerLabel is the server-authored kicker for the panel.
	KickerLabel string
}

// BriefCTARow is a single action postcard rendered on the Workshop home.
type BriefCTARow struct {
	Headline       string
	AtmosphereLine string
	CtaAction      string
	ContextID      string
	// CommunityID is the community this row belongs to. Surfaced over
	// the wire so the client can render a right-aligned circle chip
	// with the community color and short name.
	CommunityID string
	// ThumbnailMediaID is the media id of the thumbnail to render on
	// the postcard's leading edge. Resolved from the entity referenced
	// by ContextID. Empty when no media is available.
	ThumbnailMediaID string
}

// Brief is the assembled Workshop home brief — a list of action
// postcards built from the ranked momentum priorities. Hero-stat
// totals are carried separately on BriefSignal.
type Brief struct {
	CTARows []BriefCTARow
}
