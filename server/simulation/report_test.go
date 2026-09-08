package simulation

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestActionString(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionSaveGear, "SaveGear"},
		{ActionShareGear, "ShareGear"},
		{ActionExpressInterest, "ExpressInterest"},
		{ActionSelectRecipient, "SelectRecipient"},
		{ActionStartLoan, "StartLoan"},
		{ActionCompleteLoan, "CompleteLoan"},
		{ActionExpressInterestGiveaway, "ExpressInterestGiveaway"},
		{ActionSelectGiveaway, "SelectGiveaway"},
		{ActionCompleteGiveaway, "CompleteGiveaway"},
		{ActionSubmitRequest, "SubmitRequest"},
		{ActionOfferToFulfill, "OfferToFulfill"},
		{ActionFulfillRequest, "FulfillRequest"},
		{ActionSaveExperience, "SaveExperience"},
		{ActionShareExperience, "ShareExperience"},
		{ActionRSVP, "RSVP"},
		{ActionRSVPNo, "RSVPNo"},
		{ActionStartExperience, "StartExperience"},
		{ActionCompleteExperience, "CompleteExperience"},
		{ActionCancelExperience, "CancelExperience"},
		{ActionWithdrawInterest, "WithdrawInterest"},
		{ActionCancelTransfer, "CancelTransfer"},
		{ActionCancelRequest, "CancelRequest"},
		{ActionWithdrawOffer, "WithdrawOffer"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.action.String(); got != tt.want {
				t.Errorf("Action(%d).String() = %q, want %q", tt.action, got, tt.want)
			}
		})
	}

	// Unknown action returns "Unknown(N)".
	unknown := Action(999)
	if got := unknown.String(); got != "Unknown(999)" {
		t.Errorf("unknown action String() = %q, want %q", got, "Unknown(999)")
	}
}

func TestActionCategory(t *testing.T) {
	tests := []struct {
		action Action
		want   reportCategory
	}{
		{ActionSaveGear, "gear"},
		{ActionShareGear, "gear"},
		{ActionExpressInterest, "loans"},
		{ActionStartLoan, "loans"},
		{ActionCompleteLoan, "loans"},
		{ActionExpressInterestGiveaway, "giveaways"},
		{ActionCompleteGiveaway, "giveaways"},
		{ActionSubmitRequest, "requests"},
		{ActionFulfillRequest, "requests"},
		{ActionSaveExperience, "experiences"},
		{ActionCompleteExperience, "experiences"},
		{ActionRSVP, "experiences"},
		{ActionCancelTransfer, "cancellations"},
		{ActionCancelRequest, "cancellations"},
		{ActionCancelExperience, "cancellations"},
		{ActionWithdrawInterest, "cancellations"},
		{ActionWithdrawOffer, "cancellations"},
	}

	for _, tt := range tests {
		t.Run(tt.action.String(), func(t *testing.T) {
			if got := actionCategory(tt.action); got != tt.want {
				t.Errorf("actionCategory(%s) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestBuildWeeklyBuckets(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 22, 0, 0, 0, 0, time.UTC) // 3 weeks

	timeline := Timeline{
		// Week 1.
		{Time: start.Add(1 * 24 * time.Hour), Action: ActionSaveGear, Actor: "a@example.com", CommunityName: "c"},
		{Time: start.Add(2 * 24 * time.Hour), Action: ActionSaveGear, Actor: "b@example.com", CommunityName: "c"},
		{Time: start.Add(3 * 24 * time.Hour), Action: ActionExpressInterest, Actor: "a@example.com", CommunityName: "c"},
		// Week 2.
		{Time: start.Add(8 * 24 * time.Hour), Action: ActionStartLoan, Actor: "a@example.com", CommunityName: "c"},
		{Time: start.Add(9 * 24 * time.Hour), Action: ActionSubmitRequest, Actor: "b@example.com", CommunityName: "c"},
		{Time: start.Add(10 * 24 * time.Hour), Action: ActionCancelTransfer, Actor: "a@example.com", CommunityName: "c"},
		// Week 3.
		{Time: start.Add(15 * 24 * time.Hour), Action: ActionSaveExperience, Actor: "a@example.com", CommunityName: "c"},
		{Time: start.Add(16 * 24 * time.Hour), Action: ActionCompleteGiveaway, Actor: "b@example.com", CommunityName: "c"},
	}

	buckets := buildWeeklyBuckets(timeline, start, end)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 weekly buckets, got %d", len(buckets))
	}

	// Week 1: 2 gear + 1 loan.
	if buckets[0].Gear != 2 {
		t.Errorf("week 1 gear = %d, want 2", buckets[0].Gear)
	}
	if buckets[0].Loans != 1 {
		t.Errorf("week 1 loans = %d, want 1", buckets[0].Loans)
	}

	// Week 2: 1 loan + 1 request + 1 cancellation.
	if buckets[1].Loans != 1 {
		t.Errorf("week 2 loans = %d, want 1", buckets[1].Loans)
	}
	if buckets[1].Requests != 1 {
		t.Errorf("week 2 requests = %d, want 1", buckets[1].Requests)
	}
	if buckets[1].Cancellations != 1 {
		t.Errorf("week 2 cancellations = %d, want 1", buckets[1].Cancellations)
	}

	// Week 3: 1 experience + 1 giveaway.
	if buckets[2].Experiences != 1 {
		t.Errorf("week 3 experiences = %d, want 1", buckets[2].Experiences)
	}
	if buckets[2].Giveaways != 1 {
		t.Errorf("week 3 giveaways = %d, want 1", buckets[2].Giveaways)
	}
}

func TestBuildWeeklyBucketsEmpty(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC)

	buckets := buildWeeklyBuckets(nil, start, end)
	if buckets != nil {
		t.Errorf("expected nil buckets for empty timeline, got %v", buckets)
	}
}

func TestBuildUserActivity(t *testing.T) {
	lookup := map[string]memberInfo{
		"a@example.com": {Name: "Alice", Persona: "Alfred", Community: "TestComm"},
		"b@example.com": {Name: "Bob", Persona: "Gary", Community: "TestComm"},
	}

	timeline := Timeline{
		{Actor: "a@example.com", Action: ActionSaveGear},
		{Actor: "a@example.com", Action: ActionSaveGear},
		{Actor: "a@example.com", Action: ActionExpressInterest},
		{Actor: "a@example.com", Action: ActionSaveExperience},
		{Actor: "b@example.com", Action: ActionSubmitRequest},
		{Actor: "b@example.com", Action: ActionCancelRequest},
	}

	rows := buildUserActivity(timeline, lookup)

	if len(rows) != 2 {
		t.Fatalf("expected 2 user rows, got %d", len(rows))
	}

	// Alice should be first (higher total: 2+1+1 = 4).
	if rows[0].Email != "a@example.com" {
		t.Errorf("expected Alice first, got %s", rows[0].Email)
	}
	if rows[0].Gear != 2 {
		t.Errorf("Alice gear = %d, want 2", rows[0].Gear)
	}
	if rows[0].Loans != 1 {
		t.Errorf("Alice loans = %d, want 1", rows[0].Loans)
	}
	if rows[0].Total != 4 {
		t.Errorf("Alice total = %d, want 4", rows[0].Total)
	}
	if rows[0].Persona != "Alfred" {
		t.Errorf("Alice persona = %q, want %q", rows[0].Persona, "Alfred")
	}

	// Bob: 1 request + 1 cancellation = 2.
	if rows[1].Email != "b@example.com" {
		t.Errorf("expected Bob second, got %s", rows[1].Email)
	}
	if rows[1].Requests != 1 {
		t.Errorf("Bob requests = %d, want 1", rows[1].Requests)
	}
	if rows[1].Cancellations != 1 {
		t.Errorf("Bob cancellations = %d, want 1", rows[1].Cancellations)
	}
	if rows[1].Total != 2 {
		t.Errorf("Bob total = %d, want 2", rows[1].Total)
	}
}

func TestBuildCommunityBreakdown(t *testing.T) {
	scenario := Scenario{
		Communities: []CommunityDef{
			{Name: "Comm A", Members: []MemberDef{
				{Name: "Alice", Email: "a@example.com", Persona: PersonaAlfred},
				{Name: "Bob", Email: "b@example.com", Persona: PersonaGary},
			}},
			{Name: "Comm B", Members: []MemberDef{
				{Name: "Charlie", Email: "c@example.com", Persona: PersonaDerek},
			}},
		},
	}

	timeline := Timeline{
		{Actor: "a@example.com", CommunityName: "Comm A", Action: ActionSaveGear},
		{Actor: "a@example.com", CommunityName: "Comm A", Action: ActionSaveGear},
		{Actor: "b@example.com", CommunityName: "Comm A", Action: ActionExpressInterest},
		{Actor: "c@example.com", CommunityName: "Comm B", Action: ActionSubmitRequest},
	}

	rows := buildCommunityBreakdown(timeline, scenario)

	if len(rows) != 2 {
		t.Fatalf("expected 2 community rows, got %d", len(rows))
	}

	// Comm A should be first (higher total: 2+1 = 3).
	if rows[0].Name != "Comm A" {
		t.Errorf("expected Comm A first, got %s", rows[0].Name)
	}
	if rows[0].Members != 2 {
		t.Errorf("Comm A members = %d, want 2", rows[0].Members)
	}
	if rows[0].Gear != 2 {
		t.Errorf("Comm A gear = %d, want 2", rows[0].Gear)
	}
	if rows[0].Total != 3 {
		t.Errorf("Comm A total = %d, want 3", rows[0].Total)
	}

	// Comm B.
	if rows[1].Name != "Comm B" {
		t.Errorf("expected Comm B second, got %s", rows[1].Name)
	}
	if rows[1].Members != 1 {
		t.Errorf("Comm B members = %d, want 1", rows[1].Members)
	}
	if rows[1].Requests != 1 {
		t.Errorf("Comm B requests = %d, want 1", rows[1].Requests)
	}
}

func TestBuildReport(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	scenario := Scenario{
		Name:        "test-scenario",
		Description: "A test scenario",
		StartTime:   start,
		EndTime:     end,
		Communities: []CommunityDef{
			{Name: "Test Community", Members: []MemberDef{
				{Name: "Alice", Email: "a@example.com", Persona: PersonaAlfred},
				{Name: "Bob", Email: "b@example.com", Persona: PersonaGary},
			}},
		},
	}

	timeline := Timeline{
		{Time: start.Add(1 * 24 * time.Hour), Actor: "a@example.com", CommunityName: "Test Community", Action: ActionSaveGear},
		{Time: start.Add(2 * 24 * time.Hour), Actor: "b@example.com", CommunityName: "Test Community", Action: ActionExpressInterest},
		{Time: start.Add(8 * 24 * time.Hour), Actor: "a@example.com", CommunityName: "Test Community", Action: ActionCancelTransfer},
	}

	state := NewState()
	state.GearCreated = 5
	state.TransfersCompleted = 2
	state.ExperiencesCreated = 1

	result := &RunResult{
		SimulationID:       "sim-test",
		UsersCreated:       2,
		CommunitiesCreated: 1,
		TimelineSteps:      3,
		Duration:           5 * time.Second,
		State:              state,
		Timeline:           timeline,
	}

	report := BuildReport(result, scenario, 42)

	if report.ScenarioName != "test-scenario" {
		t.Errorf("ScenarioName = %q, want %q", report.ScenarioName, "test-scenario")
	}
	if report.Seed != 42 {
		t.Errorf("Seed = %d, want 42", report.Seed)
	}
	if report.Summary.Users != 2 {
		t.Errorf("Summary.Users = %d, want 2", report.Summary.Users)
	}
	if report.Summary.Gear != 5 {
		t.Errorf("Summary.Gear = %d, want 5", report.Summary.Gear)
	}
	if report.Summary.TransfersCompleted != 2 {
		t.Errorf("Summary.TransfersCompleted = %d, want 2", report.Summary.TransfersCompleted)
	}
	if report.Summary.TimelineSteps != 3 {
		t.Errorf("Summary.TimelineSteps = %d, want 3", report.Summary.TimelineSteps)
	}
	if len(report.WeeklyBuckets) != 2 {
		t.Errorf("WeeklyBuckets length = %d, want 2", len(report.WeeklyBuckets))
	}
	if len(report.UserActivity) != 2 {
		t.Errorf("UserActivity length = %d, want 2", len(report.UserActivity))
	}
	if len(report.CommunityBreakdown) != 1 {
		t.Errorf("CommunityBreakdown length = %d, want 1", len(report.CommunityBreakdown))
	}
	if len(report.Credentials) != 2 {
		t.Errorf("Credentials length = %d, want 2", len(report.Credentials))
	}
	if report.Credentials[0].Email != "a@example.com" {
		t.Errorf("Credentials[0].Email = %q, want %q", report.Credentials[0].Email, "a@example.com")
	}
}

func TestBuildWeeklyBucketsBoundarySteps(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC) // 2 weeks
	week := 7 * 24 * time.Hour

	timeline := Timeline{
		// Exactly at start — should land in week 1.
		{Time: start, Action: ActionSaveGear, Actor: "a@example.com", CommunityName: "c"},
		// Exactly at week boundary — should land in week 2, not week 1.
		{Time: start.Add(week), Action: ActionSubmitRequest, Actor: "a@example.com", CommunityName: "c"},
		// Before start — should be excluded from all buckets.
		{Time: start.Add(-1 * time.Hour), Action: ActionSaveGear, Actor: "a@example.com", CommunityName: "c"},
		// At end — should be excluded (end is exclusive).
		{Time: end, Action: ActionSaveGear, Actor: "a@example.com", CommunityName: "c"},
	}

	buckets := buildWeeklyBuckets(timeline, start, end)

	if len(buckets) != 2 {
		t.Fatalf("expected 2 weekly buckets, got %d", len(buckets))
	}

	if buckets[0].WeekStart != "2025-01-01" {
		t.Errorf("bucket 0 WeekStart = %q, want %q", buckets[0].WeekStart, "2025-01-01")
	}
	if buckets[1].WeekStart != "2025-01-08" {
		t.Errorf("bucket 1 WeekStart = %q, want %q", buckets[1].WeekStart, "2025-01-08")
	}

	// Week 1: only the step at exactly start.
	if buckets[0].Gear != 1 {
		t.Errorf("week 1 gear = %d, want 1 (step at start boundary)", buckets[0].Gear)
	}

	// Week 2: only the step at exactly week boundary.
	if buckets[1].Requests != 1 {
		t.Errorf("week 2 requests = %d, want 1 (step at week boundary)", buckets[1].Requests)
	}
	if buckets[1].Gear != 0 {
		t.Errorf("week 2 gear = %d, want 0 (before-start and at-end excluded)", buckets[1].Gear)
	}
}

func TestBuildUserActivityAllCategories(t *testing.T) {
	lookup := map[string]memberInfo{
		"a@example.com": {Name: "Alice", Persona: "Alfred", Community: "C"},
	}

	timeline := Timeline{
		{Actor: "a@example.com", Action: ActionSaveGear},
		{Actor: "a@example.com", Action: ActionStartLoan},
		{Actor: "a@example.com", Action: ActionCompleteGiveaway},
		{Actor: "a@example.com", Action: ActionSubmitRequest},
		{Actor: "a@example.com", Action: ActionSaveExperience},
		{Actor: "a@example.com", Action: ActionCancelTransfer},
	}

	rows := buildUserActivity(timeline, lookup)

	if len(rows) != 1 {
		t.Fatalf("expected 1 user row, got %d", len(rows))
	}

	r := rows[0]
	if r.Gear != 1 {
		t.Errorf("gear = %d, want 1", r.Gear)
	}
	if r.Loans != 1 {
		t.Errorf("loans = %d, want 1", r.Loans)
	}
	if r.Giveaways != 1 {
		t.Errorf("giveaways = %d, want 1", r.Giveaways)
	}
	if r.Requests != 1 {
		t.Errorf("requests = %d, want 1", r.Requests)
	}
	if r.Experiences != 1 {
		t.Errorf("experiences = %d, want 1", r.Experiences)
	}
	if r.Cancellations != 1 {
		t.Errorf("cancellations = %d, want 1", r.Cancellations)
	}
	if r.Total != 6 {
		t.Errorf("total = %d, want 6", r.Total)
	}
}

func TestWriteReportHTML(t *testing.T) {
	report := &Report{
		ScenarioName: "test-scenario",
		Description:  "Test description",
		StartDate:    "2025-01-01",
		EndDate:      "2025-01-15",
		DurationMS:   5000,
		Seed:         42,
		Summary: ReportSummary{
			Users:       2,
			Communities: 1,
			Gear:        5,
		},
		WeeklyBuckets: []WeekBucket{
			{WeekStart: "2025-01-01", Gear: 3, Loans: 1},
			{WeekStart: "2025-01-08", Requests: 2},
		},
		UserActivity: []UserRow{
			{Email: "a@example.com", Name: "Alice", Persona: "Alfred", Community: "C", Gear: 3, Total: 3},
		},
		Credentials: []CredentialRow{
			{Name: "Alice", Email: "a@example.com", Persona: "Alfred", Community: "C"},
		},
	}

	var buf bytes.Buffer
	err := WriteReportHTML(&buf, report)
	if err != nil {
		t.Fatalf("WriteReportHTML failed: %v", err)
	}

	html := buf.String()

	// Should be a complete HTML document.
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("missing closing html tag")
	}

	// Should contain embedded report data.
	if !strings.Contains(html, "test-scenario") {
		t.Error("missing scenario name in HTML")
	}
	if !strings.Contains(html, "a@example.com") {
		t.Error("missing user email in HTML")
	}

	// Should contain the JSON data in a script tag.
	if !strings.Contains(html, `"seed":42`) {
		t.Error("missing seed in embedded JSON")
	}
	if !strings.Contains(html, `"gear":5`) {
		t.Error("missing gear count in embedded JSON")
	}

	// Should contain key UI elements.
	if !strings.Contains(html, "Simulation Report") {
		t.Error("missing report title")
	}
	if !strings.Contains(html, "RequestEmailCode") {
		t.Error("missing password in credentials section")
	}
}

func TestWriteReportHTMLEmpty(t *testing.T) {
	report := &Report{
		ScenarioName: "empty",
		Summary:      ReportSummary{},
	}

	var buf bytes.Buffer
	err := WriteReportHTML(&buf, report)
	if err != nil {
		t.Fatalf("WriteReportHTML with empty report failed: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE for empty report")
	}
	if !strings.Contains(html, "empty") {
		t.Error("missing scenario name for empty report")
	}
}

func TestWriteReportHTMLSpecialCharacters(t *testing.T) {
	report := &Report{
		ScenarioName: `test <script>alert("xss")</script>`,
		Description:  `Description with "quotes" & <tags>`,
		Summary:      ReportSummary{},
	}

	var buf bytes.Buffer
	err := WriteReportHTML(&buf, report)
	if err != nil {
		t.Fatalf("WriteReportHTML with special chars failed: %v", err)
	}

	html := buf.String()

	// The JSON is embedded via template.JS so it won't be HTML-escaped,
	// but it will be JSON-escaped (quotes become \"). Verify the template
	// renders without error and produces valid structure.
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("missing closing html tag")
	}
}
