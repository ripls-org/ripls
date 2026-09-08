package simulation

import (
	"sort"
	"time"
)

// Report contains visualization-friendly data aggregated from a simulation run.
type Report struct {
	// Scenario metadata.
	ScenarioName string `json:"scenario_name"`
	Description  string `json:"description"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	DurationMS   int64  `json:"duration_ms"`
	Seed         int64  `json:"seed"`

	// Summary counters.
	Summary ReportSummary `json:"summary"`

	// WeeklyBuckets contains activity counts per week by category.
	WeeklyBuckets []WeekBucket `json:"weekly_buckets"`

	// UserActivity shows per-user action counts.
	UserActivity []UserRow `json:"user_activity"`

	// CommunityBreakdown shows per-community totals.
	CommunityBreakdown []CommunityRow `json:"community_breakdown"`

	// Credentials lists login info for each simulated user.
	Credentials []CredentialRow `json:"credentials"`
}

// ReportSummary holds top-level counters.
type ReportSummary struct {
	Users                int `json:"users"`
	Communities          int `json:"communities"`
	Gear                 int `json:"gear"`
	TransfersStarted     int `json:"transfers_started"`
	TransfersCompleted   int `json:"transfers_completed"`
	TransfersCancelled   int `json:"transfers_cancelled"`
	RequestsSubmitted    int `json:"requests_submitted"`
	RequestsFulfilled    int `json:"requests_fulfilled"`
	RequestsCancelled    int `json:"requests_cancelled"`
	ExperiencesCreated   int `json:"experiences_created"`
	ExperiencesCompleted int `json:"experiences_completed"`
	ExperiencesCancelled int `json:"experiences_cancelled"`
	RSVPYes              int `json:"rsvp_yes"`
	RSVPNo               int `json:"rsvp_no"`
	ChatMessagesSent     int `json:"chat_messages_sent"`
	StepsSkipped         int `json:"steps_skipped"`
	StepsFailed          int `json:"steps_failed"`
	TimelineSteps        int `json:"timeline_steps"`
}

// WeekBucket holds activity counts for a single week.
type WeekBucket struct {
	WeekStart     string `json:"week_start"`
	Gear          int    `json:"gear"`
	Loans         int    `json:"loans"`
	Giveaways     int    `json:"giveaways"`
	Requests      int    `json:"requests"`
	Experiences   int    `json:"experiences"`
	Cancellations int    `json:"cancellations"`
}

// UserRow holds per-user activity counts.
type UserRow struct {
	Email         string `json:"email"`
	Name          string `json:"name"`
	Persona       string `json:"persona"`
	Community     string `json:"community"`
	Gear          int    `json:"gear"`
	Loans         int    `json:"loans"`
	Giveaways     int    `json:"giveaways"`
	Requests      int    `json:"requests"`
	Experiences   int    `json:"experiences"`
	Cancellations int    `json:"cancellations"`
	Total         int    `json:"total"`
}

// CommunityRow holds per-community activity totals.
type CommunityRow struct {
	Name          string `json:"name"`
	Members       int    `json:"members"`
	Gear          int    `json:"gear"`
	Loans         int    `json:"loans"`
	Giveaways     int    `json:"giveaways"`
	Requests      int    `json:"requests"`
	Experiences   int    `json:"experiences"`
	Cancellations int    `json:"cancellations"`
	Total         int    `json:"total"`
}

// CredentialRow holds login info for a single simulated user.
type CredentialRow struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Persona   string `json:"persona"`
	Community string `json:"community"`
}

// memberInfo holds name/persona/community for a user, keyed by email.
type memberInfo struct {
	Name      string
	Persona   string
	Community string
}

// reportCategory is the bucket an [Action] rolls up into across every report
// breakdown (weekly, per-user, per-community). The string values double as the
// JSON keys on WeekBucket/UserRow/CommunityRow, whose struct tags must repeat
// them literally — Go requires tags be literals — so a rename here means
// renaming the tags too.
type reportCategory string

const (
	categoryGear          reportCategory = "gear"
	categoryLoans         reportCategory = "loans"
	categoryGiveaways     reportCategory = "giveaways"
	categoryRequests      reportCategory = "requests"
	categoryExperiences   reportCategory = "experiences"
	categoryCancellations reportCategory = "cancellations"
	categoryOther         reportCategory = "other"
)

// actionCategory classifies an action into a report category.
func actionCategory(a Action) reportCategory {
	switch a {
	case ActionSaveGear, ActionShareGear:
		return categoryGear
	case ActionExpressInterest, ActionSelectRecipient, ActionStartLoan, ActionCompleteLoan:
		return categoryLoans
	case ActionExpressInterestGiveaway, ActionSelectGiveaway, ActionCompleteGiveaway:
		return categoryGiveaways
	case ActionSubmitRequest, ActionOfferToFulfill, ActionFulfillRequest:
		return categoryRequests
	case ActionSaveExperience, ActionShareExperience, ActionRSVP, ActionRSVPNo,
		ActionStartExperience, ActionCompleteExperience:
		return categoryExperiences
	case ActionCancelExperience, ActionCancelTransfer, ActionCancelRequest,
		ActionWithdrawInterest, ActionWithdrawOffer:
		return categoryCancellations
	default:
		return categoryOther
	}
}

// BuildReport processes raw simulation data into a visualization-friendly report.
func BuildReport(result *RunResult, scenario Scenario, seed int64) *Report {
	r := &Report{
		ScenarioName: scenario.Name,
		Description:  scenario.Description,
		StartDate:    scenario.StartTime.Format("2006-01-02"),
		EndDate:      scenario.EndTime.Format("2006-01-02"),
		DurationMS:   result.Duration.Milliseconds(),
		Seed:         seed,
	}

	// Summary from state counters.
	r.Summary = ReportSummary{
		Users:                result.UsersCreated,
		Communities:          result.CommunitiesCreated,
		Gear:                 result.State.GearCreated,
		TransfersStarted:     result.State.TransfersStarted,
		TransfersCompleted:   result.State.TransfersCompleted,
		TransfersCancelled:   result.State.TransfersCancelled,
		RequestsSubmitted:    result.State.RequestsSubmitted,
		RequestsFulfilled:    result.State.RequestsFulfilled,
		RequestsCancelled:    result.State.RequestsCancelled,
		ExperiencesCreated:   result.State.ExperiencesCreated,
		ExperiencesCompleted: result.State.ExperiencesCompleted,
		ExperiencesCancelled: result.State.ExperiencesCancelled,
		RSVPYes:              result.State.RSVPYesCount,
		RSVPNo:               result.State.RSVPNoCount,
		ChatMessagesSent:     result.State.ChatMessagesSent,
		StepsSkipped:         result.State.StepsSkipped,
		StepsFailed:          result.State.StepsFailed,
		TimelineSteps:        result.TimelineSteps,
	}

	// Build member lookup for name/persona/community.
	memberLookup := make(map[string]memberInfo)
	for _, comm := range scenario.Communities {
		for _, m := range comm.Members {
			memberLookup[m.Email] = memberInfo{
				Name:      m.Name,
				Persona:   m.Persona.String(),
				Community: comm.Name,
			}
		}
	}

	// Weekly buckets.
	r.WeeklyBuckets = buildWeeklyBuckets(result.Timeline, scenario.StartTime, scenario.EndTime)

	// Per-user activity.
	r.UserActivity = buildUserActivity(result.Timeline, memberLookup)

	// Per-community breakdown.
	r.CommunityBreakdown = buildCommunityBreakdown(result.Timeline, scenario)

	// Credentials.
	for _, comm := range scenario.Communities {
		for _, m := range comm.Members {
			r.Credentials = append(r.Credentials, CredentialRow{
				Name:      m.Name,
				Email:     m.Email,
				Persona:   m.Persona.String(),
				Community: comm.Name,
			})
		}
	}

	return r
}

// buildWeeklyBuckets aggregates timeline steps into weekly buckets.
func buildWeeklyBuckets(timeline Timeline, start, end time.Time) []WeekBucket {
	if len(timeline) == 0 {
		return nil
	}

	week := 7 * 24 * time.Hour
	var buckets []WeekBucket

	for ws := start; ws.Before(end); ws = ws.Add(week) {
		we := ws.Add(week)
		if we.After(end) {
			we = end
		}

		b := WeekBucket{
			WeekStart: ws.Format("2006-01-02"),
		}

		for _, step := range timeline {
			if step.Time.Before(ws) || !step.Time.Before(we) {
				continue
			}
			switch actionCategory(step.Action) {
			case categoryGear:
				b.Gear++
			case categoryLoans:
				b.Loans++
			case categoryGiveaways:
				b.Giveaways++
			case categoryRequests:
				b.Requests++
			case categoryExperiences:
				b.Experiences++
			case categoryCancellations:
				b.Cancellations++
			}
		}

		buckets = append(buckets, b)
	}

	return buckets
}

// buildUserActivity aggregates per-user action counts from the timeline.
func buildUserActivity(timeline Timeline, lookup map[string]memberInfo) []UserRow {
	counts := make(map[string]*UserRow)
	for _, step := range timeline {
		row, ok := counts[step.Actor]
		if !ok {
			info := lookup[step.Actor]
			row = &UserRow{
				Email:     step.Actor,
				Name:      info.Name,
				Persona:   info.Persona,
				Community: info.Community,
			}
			counts[step.Actor] = row
		}

		switch actionCategory(step.Action) {
		case categoryGear:
			row.Gear++
		case categoryLoans:
			row.Loans++
		case categoryGiveaways:
			row.Giveaways++
		case categoryRequests:
			row.Requests++
		case categoryExperiences:
			row.Experiences++
		case categoryCancellations:
			row.Cancellations++
		}
	}

	var rows []UserRow
	for _, row := range counts {
		row.Total = row.Gear + row.Loans + row.Giveaways + row.Requests + row.Experiences + row.Cancellations
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Total > rows[j].Total
	})

	return rows
}

// buildCommunityBreakdown aggregates per-community totals from the timeline.
func buildCommunityBreakdown(timeline Timeline, scenario Scenario) []CommunityRow {
	counts := make(map[string]*CommunityRow)

	// Initialize with community metadata.
	for _, comm := range scenario.Communities {
		counts[comm.Name] = &CommunityRow{
			Name:    comm.Name,
			Members: len(comm.Members),
		}
	}

	for _, step := range timeline {
		commName := step.CommunityName
		row, ok := counts[commName]
		if !ok {
			row = &CommunityRow{Name: commName}
			counts[commName] = row
		}

		switch actionCategory(step.Action) {
		case categoryGear:
			row.Gear++
		case categoryLoans:
			row.Loans++
		case categoryGiveaways:
			row.Giveaways++
		case categoryRequests:
			row.Requests++
		case categoryExperiences:
			row.Experiences++
		case categoryCancellations:
			row.Cancellations++
		}
	}

	var rows []CommunityRow
	for _, row := range counts {
		row.Total = row.Gear + row.Loans + row.Giveaways + row.Requests + row.Experiences + row.Cancellations
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Total > rows[j].Total
	})

	return rows
}
