package simulation

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

func testScenario() Scenario {
	return Scenario{
		Name:        "test-scenario",
		Description: "Small scenario for testing",
		Communities: []CommunityDef{
			{
				Name:        "Test Community",
				Description: "A test community",
				Region:      "Austin, TX",
				Members: []MemberDef{
					{Name: "Alice", Email: "alice@example.com", Persona: PersonaAlfred},
					{Name: "Bob", Email: "bob@example.com", Persona: PersonaDerek},
					{Name: "Carol", Email: "carol@example.com", Persona: PersonaGary},
					{Name: "Diana", Email: "diana@example.com", Persona: PersonaBetty},
					{Name: "Eve", Email: "eve@example.com", Persona: PersonaEmma},
				},
			},
		},
		StartTime: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestGenerateTimeline(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	scenario := testScenario()

	result := GenerateTimeline(rng, scenario)
	steps := result.Steps

	if len(steps) == 0 {
		t.Fatal("GenerateTimeline produced empty timeline")
	}

	t.Logf("Timeline has %d steps, %d gear templates", len(steps), len(result.GearTemplates))

	// Verify timeline is sorted by time.
	for i := 1; i < len(steps); i++ {
		if steps[i].Time.Before(steps[i-1].Time) {
			t.Errorf("Timeline not sorted: step[%d] at %v before step[%d] at %v",
				i, steps[i].Time, i-1, steps[i-1].Time)
			break
		}
	}

	// Verify gear templates are populated for all SaveGear steps.
	for _, step := range steps {
		if step.Action == ActionSaveGear {
			if _, ok := result.GearTemplates[step.Ref]; !ok {
				t.Errorf("SaveGear step ref %q has no gear template", step.Ref)
			}
		}
	}
}

func TestGenerateTimelineDeterministic(t *testing.T) {
	scenario := testScenario()

	r1 := GenerateTimeline(rand.New(rand.NewSource(42)), scenario)
	r2 := GenerateTimeline(rand.New(rand.NewSource(42)), scenario)

	if len(r1.Steps) != len(r2.Steps) {
		t.Fatalf("Same seed produced different lengths: %d vs %d", len(r1.Steps), len(r2.Steps))
	}
	for i := range r1.Steps {
		s1, s2 := r1.Steps[i], r2.Steps[i]
		if s1.Action != s2.Action || s1.Actor != s2.Actor || !s1.Time.Equal(s2.Time) {
			t.Errorf("Step[%d] differs: {%v, %v, %v} vs {%v, %v, %v}",
				i, s1.Action, s1.Actor, s1.Time,
				s2.Action, s2.Actor, s2.Time)
			break
		}
	}
}

func TestGenerateTimelineTimeBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	scenario := testScenario()

	steps := GenerateTimeline(rng, scenario).Steps

	// Backward construction (#1920) means a flow anchored in the first
	// week of the scenario may have its earliest step (e.g. ExpressInterest
	// for a loan that completes in week 1) up to ~30 days before
	// scenario.StartTime. That represents activity that started before
	// the simulation window. Allow up to maxFlowDuration of leeway on the
	// lower bound; the upper bound stays strict — future timestamps are
	// the bug we are fixing.
	const maxFlowDuration = 32 * 24 * time.Hour
	startSlack := scenario.StartTime.Add(-maxFlowDuration)

	for i, step := range steps {
		if step.Time.Before(startSlack) {
			t.Errorf("Step[%d] at %v is more than %v before StartTime %v",
				i, step.Time, maxFlowDuration, scenario.StartTime)
		}
		if step.Time.After(scenario.EndTime) {
			t.Errorf("Step[%d] at %v is after EndTime %v", i, step.Time, scenario.EndTime)
		}
	}
}

// TestTimelineNeverExceedsEndTime asserts that no scenario's generated
// timeline emits a step whose Time falls after scenario.EndTime. Issue
// #1920: forward-constructed lifecycle steps (StartLoan + CompleteLoan,
// experience completion, request fulfilment, etc.) used to extend up to
// ~30 days past EndTime, producing chat messages with future
// sent_at_unix_sec that sorted to the bottom of conversations.
func TestTimelineNeverExceedsEndTime(t *testing.T) {
	for _, scenario := range AllScenarios() {
		t.Run(scenario.Name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(42))
			steps := GenerateTimeline(rng, scenario).Steps

			var worstOffset time.Duration
			offenders := 0
			for _, step := range steps {
				if step.Time.After(scenario.EndTime) {
					offenders++
					if d := step.Time.Sub(scenario.EndTime); d > worstOffset {
						worstOffset = d
					}
				}
			}
			if offenders > 0 {
				t.Errorf("%d steps exceed scenario.EndTime; worst overshoot %v",
					offenders, worstOffset)
			}
		})
	}
}

// TestTimelineIncludesInProgressFlows asserts that across a large
// scenario, the generator produces a non-zero count of flows that
// terminate before reaching their final lifecycle step (loans started
// but never completed, requests submitted but never fulfilled,
// experiences shared but not yet started). This locks in the "snapshot
// at now" shape introduced in #1920 — a future tweak that re-flattens
// every flow to all-completed would silently regress simulator realism.
func TestTimelineIncludesInProgressFlows(t *testing.T) {
	// LoadTestLarge has enough activity to deflake the in-progress
	// sample counts across a single fixed seed.
	scenario := LoadTestLarge()
	steps := GenerateTimeline(rand.New(rand.NewSource(42)), scenario).Steps

	// Bucket steps by ref so we can ask "which lifecycle stages did
	// this flow emit?"
	byRef := make(map[string]map[Action]bool)
	for _, s := range steps {
		if byRef[s.Ref] == nil {
			byRef[s.Ref] = make(map[Action]bool)
		}
		byRef[s.Ref][s.Action] = true
	}

	var loansInProgress, requestsInProgress, experiencesInProgress int
	for _, actions := range byRef {
		switch {
		case actions[ActionStartLoan] && !actions[ActionCompleteLoan] && !actions[ActionCancelTransfer]:
			loansInProgress++
		case actions[ActionSubmitRequest] && !actions[ActionFulfillRequest] && !actions[ActionCancelRequest]:
			requestsInProgress++
		case actions[ActionShareExperience] && !actions[ActionStartExperience] && !actions[ActionCancelExperience] && !actions[ActionCompleteExperience]:
			experiencesInProgress++
		}
	}

	if loansInProgress == 0 {
		t.Error("no in-progress loans (StartLoan without CompleteLoan or CancelTransfer)")
	}
	if requestsInProgress == 0 {
		t.Error("no in-progress requests (SubmitRequest without FulfillRequest or CancelRequest)")
	}
	if experiencesInProgress == 0 {
		t.Error("no in-progress experiences (ShareExperience without Start/Cancel/Complete)")
	}
	t.Logf("in-progress flows: loans=%d requests=%d experiences=%d",
		loansInProgress, requestsInProgress, experiencesInProgress)
}

// TestChatSubFlowsRespectDeadline asserts that sendChatMessages bounds
// its msgTime advance by the deadline it is given. The current
// implementation advances forward unconditionally, walking past
// EndTime when called with a baseTime near it.
func TestChatSubFlowsRespectDeadline(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	deadline := baseTime.Add(10 * time.Minute) // tight window

	msgs := []chatMessage{
		{SenderEmail: "a@example.com", Text: "first"},
		{SenderEmail: "b@example.com", Text: "second"},
		{SenderEmail: "c@example.com", Text: "third"},
		{SenderEmail: "d@example.com", Text: "fourth"},
		{SenderEmail: "e@example.com", Text: "fifth"},
		{SenderEmail: "f@example.com", Text: "sixth"},
	}

	got := planChatMessageTimes(rng, msgs, baseTime, deadline)
	for i, ts := range got {
		if ts.After(deadline) {
			t.Errorf("planned msg[%d] at %v exceeds deadline %v", i, ts, deadline)
		}
	}
	if len(got) == 0 {
		t.Error("planChatMessageTimes returned no times; expected at least the first to fit")
	}
}

func TestGenerateTimelineHasAllActionTypes(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	scenario := testScenario()

	steps := GenerateTimeline(rng, scenario).Steps

	actionCounts := make(map[Action]int)
	for _, step := range steps {
		actionCounts[step.Action]++
	}

	// With 5 members over 3 months, we should see most action types.
	// Note: ActionSelectRecipient is not expected because loans use auto-select
	// (ExpressInterest transitions directly to RECIPIENT_SELECTED).
	expectedActions := []Action{
		ActionSaveGear,
		ActionExpressInterest,
		ActionStartLoan,
		ActionCompleteLoan,
		ActionSaveExperience,
		ActionShareExperience,
		ActionRSVP,
		ActionRSVPNo,
		ActionStartExperience,
		ActionCompleteExperience,
		ActionSubmitRequest,
	}

	for _, action := range expectedActions {
		if actionCounts[action] == 0 {
			t.Errorf("No steps with action %d found in timeline", action)
		}
	}

	t.Logf("Action distribution:")
	for action, count := range actionCounts {
		t.Logf("  Action %d: %d steps", action, count)
	}
}

func TestGenerateTimelineMultiUserPatterns(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	scenario := testScenario()

	steps := GenerateTimeline(rng, scenario).Steps

	// Check that some refs have multiple actors (multi-user interaction).
	refActors := make(map[string]map[string]bool)
	for _, step := range steps {
		if refActors[step.Ref] == nil {
			refActors[step.Ref] = make(map[string]bool)
		}
		refActors[step.Ref][step.Actor] = true
	}

	multiUserRefs := 0
	for _, actors := range refActors {
		if len(actors) > 1 {
			multiUserRefs++
		}
	}

	if multiUserRefs == 0 {
		t.Error("No multi-user activity sequences found")
	}
	t.Logf("Found %d multi-user activity sequences out of %d total refs", multiUserRefs, len(refActors))
}

func TestPoissonSample(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	n := 10000
	sum := 0
	for range n {
		sum += poissonSample(rng, 3.0)
	}
	mean := float64(sum) / float64(n)

	// Mean of Poisson(3.0) should be approximately 3.0.
	if mean < 2.5 || mean > 3.5 {
		t.Errorf("Poisson mean = %f, expected ~3.0", mean)
	}
}

func TestPoissonSampleZero(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	result := poissonSample(rng, 0)
	if result != 0 {
		t.Errorf("poissonSample(0) = %d, want 0", result)
	}
}

func TestJitterDuration(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	lo := 1 * time.Hour
	hi := 24 * time.Hour

	for range 100 {
		d := jitterDuration(rng, lo, hi)
		if d < lo || d >= hi {
			t.Errorf("jitterDuration out of range: %v not in [%v, %v)", d, lo, hi)
		}
	}
}

func TestJitterDurationMinEqualsMax(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	d := jitterDuration(rng, 5*time.Hour, 5*time.Hour)
	if d != 5*time.Hour {
		t.Errorf("jitterDuration(5h, 5h) = %v, want 5h", d)
	}
}

func TestGenerateTimelineUniqueRefs(t *testing.T) {
	// With multiple seeds and a longer scenario, ensure the same gear item
	// targeted in different weeks gets unique refs (no collisions).
	for _, seed := range []int64{42, 123, 999} {
		rng := rand.New(rand.NewSource(seed))
		scenario := testScenario()
		steps := GenerateTimeline(rng, scenario).Steps

		// Collect all first-action steps per ref to detect collisions.
		// Each ref should correspond to exactly one "initiating" step
		// (ExpressInterest, ExpressInterestGiveaway, SubmitRequest, SaveExperience).
		type refInfo struct {
			firstAction Action
			firstActor  string
			firstTime   time.Time
		}
		refs := make(map[string]refInfo)
		duplicates := 0

		for _, step := range steps {
			switch step.Action {
			case ActionExpressInterest, ActionExpressInterestGiveaway,
				ActionSubmitRequest, ActionSaveExperience, ActionSaveGear:
				if existing, ok := refs[step.Ref]; ok {
					// For transfers, multiple ExpressInterest on same ref is expected
					// (contention). But two initiating actions that create new entities
					// (SaveGear, SaveExperience, SubmitRequest) sharing a ref is a bug.
					if step.Action == ActionSaveGear || step.Action == ActionSaveExperience || step.Action == ActionSubmitRequest {
						t.Errorf("seed=%d: duplicate initiating ref %q: action=%d actor=%q time=%v vs existing action=%d actor=%q time=%v",
							seed, step.Ref, step.Action, step.Actor, step.Time,
							existing.firstAction, existing.firstActor, existing.firstTime)
						duplicates++
					}
				} else {
					refs[step.Ref] = refInfo{step.Action, step.Actor, step.Time}
				}
			}
		}

		t.Logf("seed=%d: %d unique refs, %d duplicates", seed, len(refs), duplicates)
	}
}

func TestLoanFlowTimingOrder(t *testing.T) {
	// Verify that within each loan flow, WithdrawInterest always happens
	// before SelectRecipient in the sorted timeline.
	rng := rand.New(rand.NewSource(42))
	scenario := testScenario()
	steps := GenerateTimeline(rng, scenario).Steps

	// Group steps by ref.
	refSteps := make(map[string][]ActivityStep)
	for _, step := range steps {
		refSteps[step.Ref] = append(refSteps[step.Ref], step)
	}

	for ref, flowSteps := range refSteps {
		var withdrawTime, selectTime time.Time
		hasWithdraw, hasSelect := false, false

		for _, step := range flowSteps {
			switch step.Action {
			case ActionWithdrawInterest:
				withdrawTime = step.Time
				hasWithdraw = true
			case ActionSelectRecipient:
				selectTime = step.Time
				hasSelect = true
			}
		}

		if hasWithdraw && hasSelect {
			if !withdrawTime.Before(selectTime) {
				t.Errorf("ref %q: WithdrawInterest at %v is not before SelectRecipient at %v",
					ref, withdrawTime, selectTime)
			}
		}
	}
}

func TestRequestFlowTimingOrder(t *testing.T) {
	// Verify that WithdrawOffer always happens before FulfillRequest,
	// and that the withdrawer has an OfferToFulfill step before the withdrawal.
	for _, seed := range []int64{42, 123, 999} {
		rng := rand.New(rand.NewSource(seed))
		scenario := testScenario()
		steps := GenerateTimeline(rng, scenario).Steps

		refSteps := make(map[string][]ActivityStep)
		for _, step := range steps {
			refSteps[step.Ref] = append(refSteps[step.Ref], step)
		}

		for ref, flowSteps := range refSteps {
			var withdrawTime, fulfillTime time.Time
			var withdrawActor string
			hasWithdraw, hasFulfill := false, false

			// Collect offer times per actor.
			offerTimes := make(map[string]time.Time)
			for _, step := range flowSteps {
				switch step.Action {
				case ActionOfferToFulfill:
					offerTimes[step.Actor] = step.Time
				case ActionWithdrawOffer:
					withdrawTime = step.Time
					withdrawActor = step.Actor
					hasWithdraw = true
				case ActionFulfillRequest:
					fulfillTime = step.Time
					hasFulfill = true
				}
			}

			if hasWithdraw {
				// Withdrawer must have a preceding offer.
				offerTime, offered := offerTimes[withdrawActor]
				if !offered {
					t.Errorf("seed=%d ref %q: WithdrawOffer by %q but no OfferToFulfill found",
						seed, ref, withdrawActor)
				} else if !offerTime.Before(withdrawTime) {
					t.Errorf("seed=%d ref %q: OfferToFulfill at %v is not before WithdrawOffer at %v for %q",
						seed, ref, offerTime, withdrawTime, withdrawActor)
				}
			}

			if hasWithdraw && hasFulfill {
				if !withdrawTime.Before(fulfillTime) {
					t.Errorf("seed=%d ref %q: WithdrawOffer at %v is not before FulfillRequest at %v",
						seed, ref, withdrawTime, fulfillTime)
				}
			}
		}
	}
}

func TestExtractGearRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "loan ref with flow ID",
			ref:  "gear-Test Community-5-loan-42",
			want: "gear-Test Community-5",
		},
		{
			name: "giveaway ref with flow ID",
			ref:  "gear-Test Community-12-giveaway-99",
			want: "gear-Test Community-12",
		},
		{
			name: "plain gear ref (no suffix)",
			ref:  "gear-Test Community-3",
			want: "gear-Test Community-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGearRef(tt.ref)
			if got != tt.want {
				t.Errorf("extractGearRef(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestGenerateTimelineNoOverlappingLoans(t *testing.T) {
	// Verify that no two loan flows on the same gear item overlap in time.
	for _, seed := range []int64{42, 123, 999} {
		rng := rand.New(rand.NewSource(seed))
		scenario := testScenario()
		steps := GenerateTimeline(rng, scenario).Steps

		type flowRange struct {
			ref   string
			start time.Time
			end   time.Time
		}

		// Find start (first ExpressInterest) and end (CompleteLoan/CancelTransfer)
		// for each loan flow.
		flowStart := make(map[string]time.Time)
		flowEnd := make(map[string]time.Time)
		for _, step := range steps {
			switch step.Action {
			case ActionExpressInterest:
				if _, ok := flowStart[step.Ref]; !ok {
					flowStart[step.Ref] = step.Time
				}
			case ActionCompleteLoan, ActionCancelTransfer:
				flowEnd[step.Ref] = step.Time
			}
		}

		// Group by gear ref and check for overlaps.
		gearFlows := make(map[string][]flowRange)
		for ref, start := range flowStart {
			gearRef := extractGearRef(ref)
			end, ok := flowEnd[ref]
			if !ok {
				continue
			}
			gearFlows[gearRef] = append(gearFlows[gearRef], flowRange{ref: ref, start: start, end: end})
		}

		for gearRef, flows := range gearFlows {
			sort.Slice(flows, func(i, j int) bool {
				return flows[i].start.Before(flows[j].start)
			})
			for i := 1; i < len(flows); i++ {
				if flows[i].start.Before(flows[i-1].end) {
					t.Errorf("seed=%d: overlapping loan flows for gear %q: %q ends at %v, %q starts at %v",
						seed, gearRef, flows[i-1].ref, flows[i-1].end, flows[i].ref, flows[i].start)
				}
			}
		}
	}
}

func TestGenerateTimelineRequestOfferTiming(t *testing.T) {
	// Verify that all offers arrive before fulfillment or cancellation.
	for _, seed := range []int64{42, 123, 999} {
		rng := rand.New(rand.NewSource(seed))
		scenario := testScenario()
		steps := GenerateTimeline(rng, scenario).Steps

		fulfillTimes := make(map[string]time.Time)
		cancelTimes := make(map[string]time.Time)
		offerTimes := make(map[string][]time.Time)

		for _, step := range steps {
			switch step.Action {
			case ActionFulfillRequest:
				fulfillTimes[step.Ref] = step.Time
			case ActionCancelRequest:
				cancelTimes[step.Ref] = step.Time
			case ActionOfferToFulfill:
				offerTimes[step.Ref] = append(offerTimes[step.Ref], step.Time)
			}
		}

		for ref, fulfillTime := range fulfillTimes {
			for _, offerTime := range offerTimes[ref] {
				if offerTime.After(fulfillTime) {
					t.Errorf("seed=%d: offer at %v after fulfillment at %v for request %q",
						seed, offerTime, fulfillTime, ref)
				}
			}
		}

		for ref, cancelTime := range cancelTimes {
			for _, offerTime := range offerTimes[ref] {
				if offerTime.After(cancelTime) {
					t.Errorf("seed=%d: offer at %v after cancellation at %v for request %q",
						seed, offerTime, cancelTime, ref)
				}
			}
		}
	}
}

func TestGenerateTimelineMonthlyDistribution(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	scenario := SuburbanNeighborhood()
	result := GenerateTimeline(rng, scenario)
	steps := result.Steps

	// Categorize actions into types for clearer reporting.
	type actionCategory int
	const (
		catGearSetup actionCategory = iota
		catLoan
		catGiveaway
		catExperience
		catRequest
		catOther
	)
	categorize := func(a Action) actionCategory {
		switch a {
		case ActionSaveGear, ActionShareGear:
			return catGearSetup
		case ActionExpressInterest, ActionSelectRecipient, ActionStartLoan, ActionCompleteLoan,
			ActionWithdrawInterest, ActionCancelTransfer:
			return catLoan
		case ActionExpressInterestGiveaway, ActionSelectGiveaway, ActionCompleteGiveaway:
			return catGiveaway
		case ActionSaveExperience, ActionShareExperience, ActionRSVP, ActionRSVPNo,
			ActionStartExperience, ActionCompleteExperience, ActionCancelExperience:
			return catExperience
		case ActionSubmitRequest, ActionOfferToFulfill, ActionFulfillRequest,
			ActionCancelRequest, ActionWithdrawOffer:
			return catRequest
		default:
			return catOther
		}
	}
	catNames := map[actionCategory]string{
		catGearSetup:  "gear-setup",
		catLoan:       "loan",
		catGiveaway:   "giveaway",
		catExperience: "experience",
		catRequest:    "request",
		catOther:      "other",
	}

	// Bucket steps by year-month.
	type monthKey struct {
		year  int
		month time.Month
	}
	monthCounts := make(map[monthKey]map[actionCategory]int)
	for _, step := range steps {
		mk := monthKey{step.Time.Year(), step.Time.Month()}
		if monthCounts[mk] == nil {
			monthCounts[mk] = make(map[actionCategory]int)
		}
		monthCounts[mk][categorize(step.Action)]++
	}

	// Collect and sort months.
	var months []monthKey
	for mk := range monthCounts {
		months = append(months, mk)
	}
	sort.Slice(months, func(i, j int) bool {
		if months[i].year != months[j].year {
			return months[i].year < months[j].year
		}
		return months[i].month < months[j].month
	})

	t.Logf("SuburbanNeighborhood timeline: %d steps, %s to %s",
		len(steps), scenario.StartTime.Format("2006-01"), scenario.EndTime.Format("2006-01"))
	t.Logf("")
	t.Logf("%-10s %8s %8s %8s %8s %8s %8s", "month", "setup", "loan", "giveaway", "exp", "request", "total")
	t.Logf("%-10s %8s %8s %8s %8s %8s %8s", "-----", "-----", "----", "--------", "---", "-------", "-----")

	for _, mk := range months {
		counts := monthCounts[mk]
		total := 0
		for _, c := range counts {
			total += c
		}
		t.Logf("%-10s %8d %8d %8d %8d %8d %8d",
			time.Date(mk.year, mk.month, 1, 0, 0, 0, 0, time.UTC).Format("2006-01"),
			counts[catGearSetup], counts[catLoan], counts[catGiveaway],
			counts[catExperience], counts[catRequest], total)
	}

	// Verify: activities should span at least 80% of the months between start and end.
	// Exclude the first month (gear setup) from the check.
	startMonth := monthKey{scenario.StartTime.Year(), scenario.StartTime.Month()}
	activityMonths := 0
	totalMonths := 0
	for _, mk := range months {
		if mk == startMonth {
			continue
		}
		totalMonths++
		counts := monthCounts[mk]
		nonSetup := counts[catLoan] + counts[catGiveaway] + counts[catExperience] + counts[catRequest]
		if nonSetup > 0 {
			activityMonths++
		}
	}

	if totalMonths > 0 {
		coverage := float64(activityMonths) / float64(totalMonths)
		t.Logf("\nActivity coverage: %d/%d months (%.0f%%)", activityMonths, totalMonths, coverage*100)
		if coverage < 0.8 {
			t.Errorf("Activity coverage %.0f%% is below 80%% — activities are not spread across all months", coverage*100)
		}
	}

	// Verify: loans, experiences, and requests should each appear in at least half the months.
	for _, cat := range []actionCategory{catLoan, catExperience, catRequest} {
		catMonths := 0
		for _, mk := range months {
			if mk == startMonth {
				continue
			}
			if monthCounts[mk][cat] > 0 {
				catMonths++
			}
		}
		if totalMonths > 0 && float64(catMonths)/float64(totalMonths) < 0.5 {
			t.Errorf("%s activity only appears in %d/%d months — should be in at least half",
				catNames[cat], catMonths, totalMonths)
		}
	}
}

// TestGenerateExperienceFlow_RespectsMaxParticipants is the regression test
// for issue #1062. It verifies that generateExperienceFlow never schedules
// more yes-RSVPs than the experience template's capacity allows. Effective
// non-host cap is MaxParticipants - 1, because the server auto-RSVPs the
// host (see server/services/experience/share.go). MaxParticipants == 0
// means unlimited.
func TestGenerateExperienceFlow_RespectsMaxParticipants(t *testing.T) {
	comm := CommunityDef{
		Name: "Test Community",
		Members: []MemberDef{
			{Name: "Host", Email: "host@example.com", Persona: PersonaAlfred},
			{Name: "A", Email: "a@example.com", Persona: PersonaAlfred},
			{Name: "B", Email: "b@example.com", Persona: PersonaAlfred},
			{Name: "C", Email: "c@example.com", Persona: PersonaAlfred},
			{Name: "D", Email: "d@example.com", Persona: PersonaAlfred},
			{Name: "E", Email: "e@example.com", Persona: PersonaAlfred},
			{Name: "F", Email: "f@example.com", Persona: PersonaAlfred},
			{Name: "G", Email: "g@example.com", Persona: PersonaAlfred},
			{Name: "H", Email: "h@example.com", Persona: PersonaAlfred},
		},
	}
	host := comm.Members[0]

	cases := []struct {
		name            string
		maxParticipants int
	}{
		{"cap of 3", 3},
		{"cap of 5", 5},
		{"unlimited", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := ExperienceTemplate{
				Name:            "Test Experience",
				Description:     "for-test",
				DurationMinutes: 60,
				MaxParticipants: tc.maxParticipants,
				Category:        "social",
				VenueCategory:   "park",
			}

			maxYes := -1 // -1 means unlimited
			if tc.maxParticipants > 0 {
				maxYes = tc.maxParticipants - 1
			}

			// Sweep many seeds: each flow produces different RSVP counts
			// because attend rolls are probabilistic. With no cap, unlucky
			// seeds would overshoot; under the cap, none should.
			for seed := int64(0); seed < 200; seed++ {
				rng := rand.New(rand.NewSource(seed))
				baseTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
				scen := Scenario{StartTime: baseTime.Add(-180 * 24 * time.Hour), EndTime: baseTime}
				steps := generateExperienceFlow(rng, scen, host, comm, []ExperienceTemplate{tmpl}, baseTime)

				yes := 0
				for _, s := range steps {
					if s.Action == ActionRSVP {
						yes++
					}
				}
				if maxYes >= 0 && yes > maxYes {
					t.Errorf("seed=%d cap=%d: generated %d yes-RSVPs, cap is %d",
						seed, tc.maxParticipants, yes, maxYes)
				}
				if s0 := steps[0]; s0.Action != ActionSaveExperience {
					t.Errorf("seed=%d: first step should be SaveExperience, got %v", seed, s0.Action)
				}
			}
		})
	}
}

// TestGenerateExperienceFlow_FillsCapWhenPossible spot-checks that the cap
// is actually reached for at least one seed when member count far exceeds
// capacity — i.e. the cap activates rather than the function simply never
// coming close.
func TestGenerateExperienceFlow_FillsCapWhenPossible(t *testing.T) {
	// Many members to maximize the chance of rolling above attend threshold.
	members := []MemberDef{{Name: "Host", Email: "host@example.com", Persona: PersonaAlfred}}
	for i := 0; i < 30; i++ {
		members = append(members, MemberDef{
			Name:    "M" + string(rune('a'+i%26)),
			Email:   "m" + string(rune('a'+i%26)) + "@example.com",
			Persona: PersonaAlfred,
		})
	}
	// Dedupe emails to keep the filter honest.
	seen := map[string]bool{}
	dedup := members[:0]
	for _, m := range members {
		if seen[m.Email] {
			continue
		}
		seen[m.Email] = true
		dedup = append(dedup, m)
	}
	comm := CommunityDef{Name: "Big Community", Members: dedup}
	host := comm.Members[0]

	tmpl := ExperienceTemplate{
		Name: "Small Cap", DurationMinutes: 60,
		MaxParticipants: 3, Category: "social", VenueCategory: "park",
	}

	hitCap := false
	for seed := int64(0); seed < 500; seed++ {
		rng := rand.New(rand.NewSource(seed))
		baseTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		scen := Scenario{StartTime: baseTime.Add(-180 * 24 * time.Hour), EndTime: baseTime}
		steps := generateExperienceFlow(rng, scen, host, comm, []ExperienceTemplate{tmpl}, baseTime)
		yes := 0
		for _, s := range steps {
			if s.Action == ActionRSVP {
				yes++
			}
		}
		if yes == tmpl.MaxParticipants-1 {
			hitCap = true
			break
		}
	}
	if !hitCap {
		t.Errorf("Expected at least one seed to fill the cap of %d; never hit it", tmpl.MaxParticipants-1)
	}
}

func TestGenerateTimelineExperienceRSVPTiming(t *testing.T) {
	// Verify that all RSVPs arrive before cancellation or start.
	for _, seed := range []int64{42, 123, 999} {
		rng := rand.New(rand.NewSource(seed))
		scenario := testScenario()
		steps := GenerateTimeline(rng, scenario).Steps

		cancelTimes := make(map[string]time.Time)
		startTimes := make(map[string]time.Time)
		rsvpTimes := make(map[string][]time.Time)

		for _, step := range steps {
			switch step.Action {
			case ActionCancelExperience:
				cancelTimes[step.Ref] = step.Time
			case ActionStartExperience:
				startTimes[step.Ref] = step.Time
			case ActionRSVP, ActionRSVPNo:
				rsvpTimes[step.Ref] = append(rsvpTimes[step.Ref], step.Time)
			}
		}

		for ref, cancelTime := range cancelTimes {
			for _, rsvpTime := range rsvpTimes[ref] {
				if rsvpTime.After(cancelTime) {
					t.Errorf("seed=%d: RSVP at %v after cancellation at %v for experience %q",
						seed, rsvpTime, cancelTime, ref)
				}
			}
		}

		for ref, startTime := range startTimes {
			for _, rsvpTime := range rsvpTimes[ref] {
				if rsvpTime.After(startTime) {
					t.Errorf("seed=%d: RSVP at %v after start at %v for experience %q",
						seed, rsvpTime, startTime, ref)
				}
			}
		}
	}
}
