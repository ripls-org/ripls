package simulation

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

// ActivityStep represents a single RPC call in a multi-step activity sequence.
type ActivityStep struct {
	// Time is when this step should execute.
	Time time.Time

	// Actor is the email of the user performing this action.
	Actor string

	// CommunityName identifies which community this occurs in.
	CommunityName string

	// Action describes what RPC to call.
	Action Action

	// Ref is an opaque key linking related steps (e.g., same gear loan).
	Ref string

	// NextSiblingTime, when non-zero, is the Time of the next ActivityStep
	// in the same Ref. Chat sub-flows anchored at this step must not advance
	// past it. Populated by GenerateTimeline after sorting; zero on steps
	// that have no later sibling. Issue #1920.
	NextSiblingTime time.Time
}

// Action is the specific RPC operation to perform.
type Action int

const (
	// Gear setup actions.
	ActionSaveGear  Action = iota // SaveGear + ShareGear
	ActionShareGear               // ShareGear only (re-share after return)

	// Loan flow.
	ActionExpressInterest // TransferService.ExpressInterest
	ActionSelectRecipient // TransferService.SelectRecipient
	ActionStartLoan       // TransferService.StartLoan
	ActionCompleteLoan    // TransferService.CompleteTransfer (loan)

	// Giveaway flow.
	ActionExpressInterestGiveaway // TransferService.ExpressInterest (giveaway)
	ActionSelectGiveaway          // TransferService.SelectRecipient (giveaway)
	ActionCompleteGiveaway        // TransferService.CompleteTransfer (giveaway)

	// Request flow.
	ActionSubmitRequest  // RequestService.SubmitRequest
	ActionOfferToFulfill // RequestService.OfferToFulfill
	ActionFulfillRequest // RequestService.MarkRequestFulfilled

	// Experience flow.
	ActionSaveExperience     // ExperienceService.SaveExperience
	ActionShareExperience    // ExperienceService.ShareExperience
	ActionRSVP               // ExperienceService.RSVPToExperience (YES or MAYBE)
	ActionRSVPNo             // ExperienceService.RSVPToExperience (NO)
	ActionStartExperience    // ExperienceService.MarkExperienceInProcess
	ActionCompleteExperience // ExperienceService.CompleteExperience
	ActionCancelExperience   // ExperienceService.CancelExperience

	// Negative / withdrawal actions.
	ActionWithdrawInterest // TransferService.WithdrawInterest
	ActionCancelTransfer   // TransferService.CancelTransfer (owner cancels)
	ActionCancelRequest    // RequestService.CancelRequest
	ActionWithdrawOffer    // RequestService.WithdrawOffer

	// Read-heavy actions (browsing, searching, checking).
	ActionCheckFeed          // FeedService.GetFeed + MarkFeedItemsViewed
	ActionSearchCommunity    // SearchService.Search + follow-up detail views
	ActionBrowseGear         // CommunityService.ListCommunityGear + GetGear + GetGearPeople
	ActionViewProfile        // UserService.GetUser + GetUserStats
	ActionCheckStories       // FeedService.ListStories
	ActionCheckConversations // ChatService.ListConversations + GetUnreadCounts + GetConversationHistory
	ActionViewImpact         // ImpactService.GetCommunityImpactMetrics + GetUserImpactMetrics
)

// String returns a human-readable name for the action.
func (a Action) String() string {
	switch a {
	case ActionSaveGear:
		return "SaveGear"
	case ActionShareGear:
		return "ShareGear"
	case ActionExpressInterest:
		return "ExpressInterest"
	case ActionSelectRecipient:
		return "SelectRecipient"
	case ActionStartLoan:
		return "StartLoan"
	case ActionCompleteLoan:
		return "CompleteLoan"
	case ActionExpressInterestGiveaway:
		return "ExpressInterestGiveaway"
	case ActionSelectGiveaway:
		return "SelectGiveaway"
	case ActionCompleteGiveaway:
		return "CompleteGiveaway"
	case ActionSubmitRequest:
		return "SubmitRequest"
	case ActionOfferToFulfill:
		return "OfferToFulfill"
	case ActionFulfillRequest:
		return "FulfillRequest"
	case ActionSaveExperience:
		return "SaveExperience"
	case ActionShareExperience:
		return "ShareExperience"
	case ActionRSVP:
		return "RSVP"
	case ActionRSVPNo:
		return "RSVPNo"
	case ActionStartExperience:
		return "StartExperience"
	case ActionCompleteExperience:
		return "CompleteExperience"
	case ActionCancelExperience:
		return "CancelExperience"
	case ActionWithdrawInterest:
		return "WithdrawInterest"
	case ActionCancelTransfer:
		return "CancelTransfer"
	case ActionCancelRequest:
		return "CancelRequest"
	case ActionWithdrawOffer:
		return "WithdrawOffer"
	case ActionCheckFeed:
		return "CheckFeed"
	case ActionSearchCommunity:
		return "SearchCommunity"
	case ActionBrowseGear:
		return "BrowseGear"
	case ActionViewProfile:
		return "ViewProfile"
	case ActionCheckStories:
		return "CheckStories"
	case ActionCheckConversations:
		return "CheckConversations"
	case ActionViewImpact:
		return "ViewImpact"
	default:
		return fmt.Sprintf("Unknown(%d)", int(a))
	}
}

// gearRecord tracks a gear item's owner and reference key within the timeline generator.
type gearRecord struct {
	owner    string
	template GearTemplate
	refKey   string
}

// Timeline is an ordered sequence of activity steps.
type Timeline []ActivityStep

// TimelineResult contains the generated timeline and associated metadata.
type TimelineResult struct {
	Steps         Timeline
	GearTemplates map[string]GearTemplate // ref key → gear template
}

// TimelineOptions configures timeline generation behavior.
type TimelineOptions struct {
	// ReadTraffic enables periodic read actions (feed checks, browsing, etc.).
	ReadTraffic bool

	// ReadMultiplier scales read frequencies (1.0 = normal, 2.0 = double).
	ReadMultiplier float64
}

// GenerateTimeline builds a deterministic activity timeline for a scenario.
func GenerateTimeline(rng *rand.Rand, scenario Scenario) TimelineResult {
	return GenerateTimelineWithOptions(rng, scenario, TimelineOptions{})
}

// GenerateTimelineWithOptions builds a deterministic activity timeline with
// configurable read traffic generation.
func GenerateTimelineWithOptions(rng *rand.Rand, scenario Scenario, opts TimelineOptions) TimelineResult {
	config := PersonaConfig()
	var allSteps Timeline
	gearTemplates := make(map[string]GearTemplate)

	for _, comm := range scenario.Communities {
		result := generateCommunityTimeline(rng, comm, scenario, config)
		allSteps = append(allSteps, result.Steps...)
		for k, v := range result.GearTemplates {
			gearTemplates[k] = v
		}
	}

	// Generate read traffic if enabled.
	if opts.ReadTraffic {
		multiplier := opts.ReadMultiplier
		if multiplier <= 0 {
			multiplier = 1.0
		}
		readSteps := generateReadTimeline(rng, scenario, multiplier)
		allSteps = append(allSteps, readSteps...)
	}

	// Sort all steps by time for sequential execution.
	sort.Slice(allSteps, func(i, j int) bool {
		return allSteps[i].Time.Before(allSteps[j].Time)
	})

	// Populate NextSiblingTime so chat sub-flows know their forward bound.
	// Walk the sorted timeline once, tracking the most-recently-seen step
	// per ref; when we see a later step on the same ref, fill in the
	// previous step's NextSiblingTime. The final step in each ref gets
	// scenario.EndTime as a fallback bound so terminal-step chat
	// injection (e.g. StartLoan in an in-progress loan) still has a
	// deadline. Issue #1920.
	type prevInfo struct{ idx int }
	lastIdx := make(map[string]prevInfo, len(allSteps))
	for i := range allSteps {
		if prev, ok := lastIdx[allSteps[i].Ref]; ok {
			allSteps[prev.idx].NextSiblingTime = allSteps[i].Time
		}
		lastIdx[allSteps[i].Ref] = prevInfo{idx: i}
	}
	for _, info := range lastIdx {
		if allSteps[info.idx].NextSiblingTime.IsZero() {
			allSteps[info.idx].NextSiblingTime = scenario.EndTime
		}
	}

	return TimelineResult{
		Steps:         allSteps,
		GearTemplates: gearTemplates,
	}
}

// generateCommunityTimeline builds the activity timeline for one community.
func generateCommunityTimeline(rng *rand.Rand, comm CommunityDef, scenario Scenario, config map[PersonaType]PersonaWeights) TimelineResult {
	var timeline Timeline
	gearTemplates := make(map[string]GearTemplate)
	var communityGear []gearRecord

	// Step 1: Initial gear setup at StartTime.
	refCounter := 0
	nextRef := func(prefix string) string {
		refCounter++
		return fmt.Sprintf("%s-%s-%d", prefix, comm.Name, refCounter)
	}

	for _, member := range comm.Members {
		gear := GearForPersona(rng, member.Persona)
		for _, g := range gear {
			ref := nextRef("gear")
			timeline = append(timeline, ActivityStep{
				Time:          jitterTime(rng, scenario.StartTime, 0, 7*24*time.Hour),
				Actor:         member.Email,
				CommunityName: comm.Name,
				Action:        ActionSaveGear,
				Ref:           ref,
			})
			communityGear = append(communityGear, gearRecord{
				owner:    member.Email,
				template: g,
				refKey:   ref,
			})
			gearTemplates[ref] = g
		}
	}

	// Step 2: Generate activities week by week over the simulation period.
	experiences, _ := LoadExperienceTemplates()
	requests, _ := LoadRequestTemplates()

	weekDuration := 7 * 24 * time.Hour
	currentWeek := scenario.StartTime.Add(weekDuration) // Start activity after gear setup week.

	// Track gear availability to prevent overlapping loans/giveaways on the same item.
	gearBusyUntil := make(map[string]time.Time)

	for currentWeek.Before(scenario.EndTime) {
		weekEnd := currentWeek.Add(weekDuration)
		if weekEnd.After(scenario.EndTime) {
			weekEnd = scenario.EndTime
		}

		for _, member := range comm.Members {
			pw := config[member.Persona]
			numActivities := poissonSample(rng, pw.BaseActivitiesPerWeek)

			for range numActivities {
				activity := SelectActivity(rng, pw.Weights)
				baseTime := jitterTime(rng, currentWeek, 0, weekDuration)
				if baseTime.After(scenario.EndTime) {
					continue
				}

				switch activity {
				case ActivityShareGear:
					// Already handled in initial setup.
					continue

				case ActivityBorrow:
					refCounter++
					steps := generateLoanFlow(rng, scenario, member, comm, communityGear, baseTime, refCounter, gearBusyUntil)
					timeline = append(timeline, steps...)

				case ActivityGiveAway:
					refCounter++
					steps := generateGiveawayFlow(rng, scenario, member, comm, communityGear, baseTime, refCounter, gearBusyUntil)
					timeline = append(timeline, steps...)

				case ActivityHostExperience:
					if len(experiences) > 0 {
						steps := generateExperienceFlow(rng, scenario, member, comm, experiences, baseTime)
						timeline = append(timeline, steps...)
					}

				case ActivityAttendExperience:
					// RSVPs are generated as part of experience flows.
					continue

				case ActivityMakeRequest:
					if len(requests) > 0 {
						steps := generateRequestFlow(rng, scenario, member, comm, requests, baseTime)
						timeline = append(timeline, steps...)
					}

				case ActivityOfferHelp:
					// Offers are generated as part of request flows.
					continue
				}
			}
		}

		currentWeek = weekEnd
	}

	return TimelineResult{
		Steps:         timeline,
		GearTemplates: gearTemplates,
	}
}

// jitterTime returns a time offset from base by a random duration in [minOffset, maxOffset].
func jitterTime(rng *rand.Rand, base time.Time, minOffset, maxOffset time.Duration) time.Time {
	return base.Add(jitterDuration(rng, minOffset, maxOffset))
}

// pickAnchorAfter returns a uniformly-distributed time inside
// [earliest, scenario.EndTime]. Used for flows whose internal duration
// constraints require the anchor to land at or after a known earlier
// time (e.g., a loan completion must be at least minDuration after
// scenario.StartTime to leave room for ExpressInterest + StartLoan).
// If earliest is at or after scenario.EndTime, returns scenario.EndTime.
func pickAnchorAfter(rng *rand.Rand, scenario Scenario, earliest time.Time) time.Time {
	if !earliest.Before(scenario.EndTime) {
		return scenario.EndTime
	}
	span := scenario.EndTime.Sub(earliest)
	offset := time.Duration(rng.Int63n(int64(span)))
	return earliest.Add(offset)
}

// commitGearRange records that gear refKey is occupied across [start, end]
// (inclusive) by a now-committed flow. With backward-constructed flows
// (#1920), a later flow's anchor can fall after this end while its actual
// range still extends backward into the occupied window. To prevent that
// without tracking every committed range, the recorded busyUntil
// conservatively reserves the gear for maxFlowLookback past end so a
// subsequent eligibility check against the new flow's anchor catches
// any overlap. Callers should compare candidate anchor.Before(busyUntil)
// and reject if true.
func commitGearRange(busyUntil map[string]time.Time, refKey string, _, end time.Time) {
	busyUntil[refKey] = end.Add(maxFlowLookback)
}

// maxFlowLookback is the worst-case distance any backward-constructed flow
// reaches from its anchor to its earliest step. Driven by the loan flow's
// 7-30 day completion window + 1-3 day interest-to-start gap, rounded up.
const maxFlowLookback = 35 * 24 * time.Hour

// jitterDuration returns a random duration in [min, max].
func jitterDuration(rng *rand.Rand, lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(rng.Int63n(int64(hi-lo)))
}

// poissonSample returns a Poisson-distributed random number with the given mean.
// Uses the Knuth algorithm for small means.
func poissonSample(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}

	// Precompute threshold: e^(-lambda).
	threshold := expNegLambda(lambda)

	p := 1.0
	for k := range 20 { // Cap at 20 to avoid runaway.
		p *= rng.Float64()
		if p < threshold {
			return k
		}
	}
	return int(lambda) // Fallback to mean.
}

// expNegLambda computes e^(-lambda) using a simple approximation.
func expNegLambda(lambda float64) float64 {
	result := 1.0
	for range 100 {
		result *= (1 - lambda/100)
	}
	if result < 0 {
		return 0
	}
	return result
}
