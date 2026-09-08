package email

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"sort"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/mailgun/mailgun-go/v5"

	"go.ripls.org/ripls/server/logging"
)

//go:embed activity_digest_template.html
var activityDigestTemplateHTML string

//go:embed activity_digest_template.txt
var activityDigestTemplateText string

// activityRow is one labelled row in the Activity-totals section.
type activityRow struct {
	// Key is the row's stable identity across windows (labels can flip
	// between e.g. "Loans started" and "Loan activity"); deltas match on
	// it.
	Key    string
	Label  string
	Count  int
	Detail string // e.g. "2 completed, 0 cancelled"
	Delta  string // e.g. "+4" vs the prior window; empty without prior data
}

// chatRow is one labelled row in the Chat-by-topic section.
type chatRow struct {
	Label string
	Count int
	Ratio string // e.g. "14 per loan"
}

// communityRowView is one rendered line in the Top-communities section.
type communityRowView struct {
	Title  string // e.g. `Maple Street (14 members)` or `Ad hoc · around event "X" (5 members)`
	Detail string // e.g. "3 loans and 1 request · 12 messages from 5 people"
}

// dayActivesRow is one local day in the weekly actives-by-day breakdown.
type dayActivesRow struct {
	Label        string // "Mon 2026-06-29"
	ActiveLabel  string // active-user count, or "n/a" pre-coverage
	Contributors int
}

// activityDigestTemplateData is the flat shape the templates iterate
// over. Computed once per render call from an ActivityDigestInput.
type activityDigestTemplateData struct {
	Title                  string
	WindowLabel            string
	TimezoneName           string
	GeneratedAtLabel       string
	PeriodWord             string // "today" or "this week"
	HasNewUsers            bool
	NewUsers               []DigestNewUser
	NewUserBreakdownLine   string
	InteractiveSignInCount int
	ActiveUserCount        int
	ActiveUsersKnown       bool
	ActiveContributorCount int
	DayActiveRows          []dayActivesRow
	HasDailyActives        bool
	HasWaitlistSignups     bool
	WaitlistSignups        []DigestWaitlistSignup
	PasswordResetCount     int
	ActivityRows           []activityRow
	HasActivity            bool
	ChatRows               []chatRow
	HasChatActivity        bool
	ChatTotal              int
	CommunityRows          []communityRowView
	AdHocRollupLine        string
	NamedRollupLine        string
	HasCommunities         bool
	TotalMemberActionCount int
	TotalUserMessageCount  int
	// ExcludedFooterLine reconciles the totals with the fetched rows —
	// e.g. "3 bookkeeping events excluded and 1 action undone". Empty
	// when nothing was excluded.
	ExcludedFooterLine string
	// Deltas vs the prior window; all empty when no prior data. The
	// active-users delta additionally requires stamp coverage of both
	// windows.
	TotalActionsDelta  string
	TotalMessagesDelta string
	NewUsersDelta      string
	ActiveUsersDelta   string
	ContributorsDelta  string
	PriorWindowLabel   string
	Colors             templateColors
}

// buildActivityDigestTemplateData prepares the rendering struct from the
// caller-provided input. Time formatting uses the requested IANA
// timezone; an invalid TimezoneName silently falls back to UTC.
func buildActivityDigestTemplateData(in ActivityDigestInput) activityDigestTemplateData {
	loc, err := time.LoadLocation(in.TimezoneName)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	start := time.Unix(in.WindowStartUnixSec, 0).In(loc)
	end := time.Unix(in.WindowEndUnixSec, 0).In(loc)
	generated := time.Unix(in.GeneratedAtUnixSec, 0).In(loc)

	data := activityDigestTemplateData{
		Title:                  digestTitle(in.Period),
		WindowLabel:            formatPeriodWindowLabel(in.Period, start, end),
		TimezoneName:           displayTimezoneName(in.TimezoneName, loc),
		PeriodWord:             periodWord(in.Period),
		GeneratedAtLabel:       generated.Format("2006-01-02 15:04 MST"),
		HasNewUsers:            len(in.NewUsers) > 0,
		NewUsers:               in.NewUsers,
		NewUserBreakdownLine:   formatAuthBreakdown(in.NewUserAuthBreakdown),
		InteractiveSignInCount: in.InteractiveSignInCount,
		ActiveUserCount:        in.ActiveUserCount,
		ActiveUsersKnown:       in.ActiveUserDataCoversWindow,
		ActiveContributorCount: in.ActiveContributorCount,
		DayActiveRows:          buildDayActiveRows(in.DailyActives),
		HasDailyActives:        len(in.DailyActives) > 0,
		HasWaitlistSignups:     len(in.WaitlistSignups) > 0,
		WaitlistSignups:        in.WaitlistSignups,
		PasswordResetCount:     in.PasswordResetCount,
		CommunityRows:          buildCommunityRowViews(in.Communities),
		AdHocRollupLine:        formatAdHocRollup(in.AdHocTailRollup),
		NamedRollupLine:        formatNamedRollup(in.NamedTailRollup),
		HasCommunities:         len(in.Communities) > 0 || in.AdHocTailRollup != nil || in.NamedTailRollup != nil,
		TotalMemberActionCount: in.TotalMemberActionCount,
		TotalUserMessageCount:  in.TotalUserMessageCount,
		ExcludedFooterLine:     formatExcludedFooter(in.InternalEventCount, in.UndoneEventCount),
		ChatTotal:              in.ChatByTopic.Total(),
	}

	data.ActivityRows = buildActivityRows(in.ActivityTotals)
	data.HasActivity = len(data.ActivityRows) > 0
	data.ChatRows = buildChatRows(in.ChatByTopic, in.ActivityTotals)
	data.HasChatActivity = data.ChatTotal > 0
	data.Colors = emailPalette()
	applyPriorDeltas(&data, in)

	return data
}

// applyPriorDeltas fills the delta strings from the prior-window
// headline metrics. No-op (all deltas stay empty) without prior data.
func applyPriorDeltas(data *activityDigestTemplateData, in ActivityDigestInput) {
	if in.Prior == nil {
		return
	}
	priorByKey := map[string]int{}
	for _, r := range buildActivityRows(in.Prior.ActivityTotals) {
		priorByKey[r.Key] = r.Count
	}
	for i := range data.ActivityRows {
		row := &data.ActivityRows[i]
		row.Delta = formatDelta(row.Count - priorByKey[row.Key])
	}
	data.TotalActionsDelta = formatDelta(in.TotalMemberActionCount - in.Prior.TotalMemberActionCount)
	data.TotalMessagesDelta = formatDelta(in.TotalUserMessageCount - in.Prior.TotalUserMessageCount)
	data.NewUsersDelta = formatDelta(len(in.NewUsers) - in.Prior.NewUserCount)
	data.ContributorsDelta = formatDelta(in.ActiveContributorCount - in.Prior.ActiveContributorCount)
	if in.ActiveUserDataCoversWindow && in.Prior.ActiveUserDataCoversWindow {
		data.ActiveUsersDelta = formatDelta(in.ActiveUserCount - in.Prior.ActiveUserCount)
	}
	data.PriorWindowLabel = in.Prior.WindowLabel
}

// formatDelta renders a signed change vs the prior window.
func formatDelta(d int) string {
	switch {
	case d > 0:
		return fmt.Sprintf("+%d", d)
	case d < 0:
		return fmt.Sprintf("%d", d)
	default:
		return "±0"
	}
}

// lifecycleRow picks the right primary count and label for a row whose
// primary action (e.g. "Loans started") may be zero while later-stage
// activity (completions, cancellations, RSVPs) is non-zero. In that
// case the row keeps surfacing — but it switches to a label like
// "Loan activity" with the count of the actually-happened later
// events, instead of misleadingly showing "0 Loans started".
func lifecycleRow(key string, primaryCount int, singularLabel, pluralLabelStr, fallbackLabel, detail string, fallbackCount int) activityRow {
	if primaryCount > 0 {
		return activityRow{
			Key:    key,
			Label:  pluralLabel(primaryCount, singularLabel, pluralLabelStr),
			Count:  primaryCount,
			Detail: detail,
		}
	}
	return activityRow{
		Key:    key,
		Label:  fallbackLabel,
		Count:  fallbackCount,
		Detail: detail,
	}
}

// buildActivityRows turns the per-entity-type counters into a list of
// rendered rows. Each row is only emitted if the underlying count is
// non-zero — quiet days produce a short section, not a wall of zeros.
func buildActivityRows(t ActivityTotals) []activityRow {
	rows := make([]activityRow, 0, 10)

	if t.GearShared > 0 || t.GearUnshared > 0 {
		detail := ""
		if t.GearUnshared > 0 {
			detail = fmt.Sprintf("%d unshared", t.GearUnshared)
		}
		rows = append(rows, activityRow{
			Key:    "gear",
			Label:  pluralLabel(t.GearShared, "Gear item shared", "Gear items shared"),
			Count:  t.GearShared,
			Detail: detail,
		})
	}
	if t.LoansStarted > 0 || t.LoansCompleted > 0 || t.LoansCancelled > 0 {
		rows = append(rows, lifecycleRow(
			"loans", t.LoansStarted, "Loan started", "Loans started", "Loan activity",
			funnelDetail("completed", t.LoansCompleted, "cancelled", t.LoansCancelled),
			t.LoansCompleted+t.LoansCancelled,
		))
	}
	if t.GiveawaysStarted > 0 || t.GiveawaysCompleted > 0 || t.GiveawaysCancelled > 0 {
		rows = append(rows, lifecycleRow(
			"giveaways", t.GiveawaysStarted, "Giveaway started", "Giveaways started", "Giveaway activity",
			funnelDetail("completed", t.GiveawaysCompleted, "cancelled", t.GiveawaysCancelled),
			t.GiveawaysCompleted+t.GiveawaysCancelled,
		))
	}
	coordinationTotal := t.TransferInterestExpressed + t.TransferInterestWithdrawn +
		t.TransferRecipientsSelected + t.TransferPickupsProposed
	if coordinationTotal > 0 {
		var parts []string
		if t.TransferInterestExpressed > 0 {
			parts = append(parts, fmt.Sprintf("%d interest expressed", t.TransferInterestExpressed))
		}
		if t.TransferInterestWithdrawn > 0 {
			parts = append(parts, fmt.Sprintf("%d interest withdrawn", t.TransferInterestWithdrawn))
		}
		if t.TransferRecipientsSelected > 0 {
			parts = append(parts, fmt.Sprintf("%d recipient%s selected", t.TransferRecipientsSelected, pluralS(t.TransferRecipientsSelected)))
		}
		if t.TransferPickupsProposed > 0 {
			parts = append(parts, fmt.Sprintf("%d pickup%s proposed", t.TransferPickupsProposed, pluralS(t.TransferPickupsProposed)))
		}
		rows = append(rows, activityRow{
			Key:    "coordination",
			Label:  "Transfer coordination",
			Count:  coordinationTotal,
			Detail: joinHumanList(parts),
		})
	}
	requestFollowUps := t.RequestOffersMade + t.RequestOffersSelected + t.RequestOffersWithdrawn +
		t.RequestsFulfilled + t.RequestsCancelled
	if t.RequestsPosted > 0 || requestFollowUps > 0 {
		var parts []string
		if t.RequestOffersMade > 0 {
			parts = append(parts, fmt.Sprintf("%d offers received", t.RequestOffersMade))
		}
		if t.RequestOffersSelected > 0 {
			parts = append(parts, fmt.Sprintf("%d selected", t.RequestOffersSelected))
		}
		if t.RequestOffersWithdrawn > 0 {
			parts = append(parts, fmt.Sprintf("%d withdrawn", t.RequestOffersWithdrawn))
		}
		if t.RequestsFulfilled > 0 {
			parts = append(parts, fmt.Sprintf("%d fulfilled", t.RequestsFulfilled))
		}
		if t.RequestsCancelled > 0 {
			parts = append(parts, fmt.Sprintf("%d cancelled", t.RequestsCancelled))
		}
		rows = append(rows, lifecycleRow(
			"requests", t.RequestsPosted, "Request posted", "Requests posted", "Request activity",
			joinHumanList(parts),
			requestFollowUps,
		))
	}
	experienceFollowUps := t.ExperiencesStarted + t.ExperiencesUpdated + t.ExperiencesCompleted +
		t.ExperiencesCancelled + t.RSVPYes + t.RSVPMaybe + t.RSVPNo
	if t.ExperiencesPosted > 0 || experienceFollowUps > 0 {
		var parts []string
		if t.ExperiencesStarted > 0 {
			parts = append(parts, fmt.Sprintf("%d started", t.ExperiencesStarted))
		}
		if t.ExperiencesCompleted > 0 {
			parts = append(parts, fmt.Sprintf("%d completed", t.ExperiencesCompleted))
		}
		if t.ExperiencesCancelled > 0 {
			parts = append(parts, fmt.Sprintf("%d cancelled", t.ExperiencesCancelled))
		}
		if t.ExperiencesUpdated > 0 {
			parts = append(parts, fmt.Sprintf("%d updated", t.ExperiencesUpdated))
		}
		detail := joinHumanList(parts)
		if rsvpDetail := formatRSVPDetail(t.RSVPYes, t.RSVPMaybe, t.RSVPNo); rsvpDetail != "" {
			if detail != "" {
				detail += "; "
			}
			detail += rsvpDetail
		}
		rows = append(rows, lifecycleRow(
			"events", t.ExperiencesPosted, "Event posted", "Events posted", "Event activity",
			detail,
			experienceFollowUps,
		))
	}
	planningTotal := t.PlanningNeedsAdded + t.PlanningNeedsRemoved + t.PlanningNeedsClaimed +
		t.PlanningNeedsUpdated + t.PlanningContributionsAdded + t.PlanningContributionsRemoved
	if planningTotal > 0 {
		var parts []string
		if t.PlanningNeedsAdded > 0 {
			parts = append(parts, fmt.Sprintf("%d need%s added", t.PlanningNeedsAdded, pluralS(t.PlanningNeedsAdded)))
		}
		if t.PlanningNeedsClaimed > 0 {
			parts = append(parts, fmt.Sprintf("%d claimed", t.PlanningNeedsClaimed))
		}
		if t.PlanningNeedsUpdated > 0 {
			parts = append(parts, fmt.Sprintf("%d updated", t.PlanningNeedsUpdated))
		}
		if t.PlanningNeedsRemoved > 0 {
			parts = append(parts, fmt.Sprintf("%d removed", t.PlanningNeedsRemoved))
		}
		if t.PlanningContributionsAdded > 0 {
			parts = append(parts, fmt.Sprintf("%d contribution%s added", t.PlanningContributionsAdded, pluralS(t.PlanningContributionsAdded)))
		}
		if t.PlanningContributionsRemoved > 0 {
			parts = append(parts, fmt.Sprintf("%d contribution%s removed", t.PlanningContributionsRemoved, pluralS(t.PlanningContributionsRemoved)))
		}
		rows = append(rows, activityRow{
			Key:    "planning",
			Label:  "Planning activity",
			Count:  planningTotal,
			Detail: joinHumanList(parts),
		})
	}
	if t.InvitationsRedeemed > 0 || t.MembersLeft > 0 || t.MembersRejoined > 0 {
		detail := ""
		if t.MembersLeft > 0 {
			detail = fmt.Sprintf("%d left", t.MembersLeft)
		}
		if t.MembersRejoined > 0 {
			if detail != "" {
				detail += ", "
			}
			detail += fmt.Sprintf("%d rejoined", t.MembersRejoined)
		}
		rows = append(rows, activityRow{
			Key:    "membership",
			Label:  pluralLabel(t.InvitationsRedeemed, "Member joined via invite", "Members joined via invite"),
			Count:  t.InvitationsRedeemed,
			Detail: detail,
		})
	}
	if t.CommunitiesCreated > 0 || t.CommunitiesNamed > 0 || t.CommunitiesDeleted > 0 || t.CommunitiesRestored > 0 || t.OwnershipTransferred > 0 {
		var parts []string
		if t.CommunitiesCreated > 0 {
			parts = append(parts, fmt.Sprintf("%d created", t.CommunitiesCreated))
		}
		if t.CommunitiesNamed > 0 {
			parts = append(parts, fmt.Sprintf("%d ad-hoc group%s named", t.CommunitiesNamed, pluralS(t.CommunitiesNamed)))
		}
		if t.CommunitiesDeleted > 0 {
			parts = append(parts, fmt.Sprintf("%d deleted", t.CommunitiesDeleted))
		}
		if t.CommunitiesRestored > 0 {
			parts = append(parts, fmt.Sprintf("%d restored", t.CommunitiesRestored))
		}
		if t.OwnershipTransferred > 0 {
			parts = append(parts, fmt.Sprintf("%d ownership transfer%s", t.OwnershipTransferred, pluralS(t.OwnershipTransferred)))
		}
		rows = append(rows, activityRow{
			Key:    "community",
			Label:  "Community lifecycle",
			Count:  t.CommunitiesCreated + t.CommunitiesNamed + t.CommunitiesDeleted + t.CommunitiesRestored + t.OwnershipTransferred,
			Detail: joinHumanList(parts),
		})
	}

	return rows
}

// buildDayActiveRows renders the weekly actives-by-day breakdown. Days
// before the stamp table's coverage render "n/a" instead of a
// misleading zero.
func buildDayActiveRows(days []DigestDayActives) []dayActivesRow {
	rows := make([]dayActivesRow, 0, len(days))
	for _, d := range days {
		activeLabel := "n/a"
		if d.ActiveUsers >= 0 {
			activeLabel = fmt.Sprintf("%d", d.ActiveUsers)
		}
		rows = append(rows, dayActivesRow{
			Label:        d.DateLabel,
			ActiveLabel:  activeLabel,
			Contributors: d.Contributors,
		})
	}
	return rows
}

// buildCommunityRowViews renders each DigestCommunity into a
// title/detail line pair for the Top-communities section.
func buildCommunityRowViews(communities []DigestCommunity) []communityRowView {
	views := make([]communityRowView, 0, len(communities))
	for _, c := range communities {
		views = append(views, communityRowView{
			Title:  communityRowTitle(c),
			Detail: communityRowDetail(c),
		})
	}
	return views
}

// communityRowTitle composes the display name with member-count and
// deletion annotations: `Maple Street (14 members)`,
// `Ad hoc · around event "X" (5 members)`, `Old Crew (deleted)`.
func communityRowTitle(c DigestCommunity) string {
	name := c.Name
	if c.IsAdHoc {
		if c.OriginLabel != "" {
			name = "Ad hoc · around " + c.OriginLabel
		} else {
			name = "Ad hoc group"
		}
	} else if name == "" {
		name = "Unknown community"
	}
	var annotations []string
	if c.MemberCount > 0 {
		annotations = append(annotations, fmt.Sprintf("%d member%s", c.MemberCount, pluralS(c.MemberCount)))
	}
	if c.Deleted {
		annotations = append(annotations, "deleted")
	}
	if len(annotations) > 0 {
		name += " (" + joinSemicolonList(annotations) + ")"
	}
	return name
}

// communityRowDetail composes the per-community activity breakdown:
// the top action categories plus chat volume with distinct senders.
func communityRowDetail(c DigestCommunity) string {
	var segments []string
	if len(c.TopCategories) > 0 {
		parts := make([]string, 0, len(c.TopCategories))
		for _, cat := range c.TopCategories {
			parts = append(parts, fmt.Sprintf("%d %s", cat.Count, cat.Label))
		}
		segments = append(segments, joinHumanList(parts))
	} else if c.ActionCount > 0 {
		segments = append(segments, fmt.Sprintf("%d action%s", c.ActionCount, pluralS(c.ActionCount)))
	}
	if c.UserMessageCount > 0 {
		msg := fmt.Sprintf("%d message%s", c.UserMessageCount, pluralS(c.UserMessageCount))
		if c.DistinctSenderCount > 0 {
			msg += fmt.Sprintf(" from %d %s", c.DistinctSenderCount, pluralPeople(c.DistinctSenderCount))
		}
		segments = append(segments, msg)
	}
	if len(segments) == 0 {
		return "no counted activity"
	}
	return joinMiddots(segments)
}

// formatAdHocRollup renders the ad-hoc tail summary line, e.g.
// "…and 7 more ad-hoc groups (10 actions, 3 messages): 4 around events,
// 2 around gear, and 1 unknown origin". Empty when there is no tail.
func formatAdHocRollup(r *DigestCommunityRollup) string {
	if r == nil || r.CommunityCount == 0 {
		return ""
	}
	line := fmt.Sprintf("…and %d more ad-hoc group%s (%d action%s, %d message%s)",
		r.CommunityCount, pluralS(r.CommunityCount),
		r.ActionCount, pluralS(r.ActionCount),
		r.MessageCount, pluralS(r.MessageCount))
	if len(r.KindCounts) > 0 {
		parts := make([]string, 0, len(r.KindCounts))
		for _, k := range r.KindCounts {
			parts = append(parts, fmt.Sprintf("%d %s", k.Count, k.Label))
		}
		line += ": " + joinHumanList(parts)
	}
	return line
}

// formatNamedRollup renders the named-communities tail summary line.
// Empty when there is no tail.
func formatNamedRollup(r *DigestCommunityRollup) string {
	if r == nil || r.CommunityCount == 0 {
		return ""
	}
	return fmt.Sprintf("…and %d more %s (%d action%s, %d message%s)",
		r.CommunityCount, pluralLabel(r.CommunityCount, "community", "communities"),
		r.ActionCount, pluralS(r.ActionCount),
		r.MessageCount, pluralS(r.MessageCount))
}

// joinMiddots joins segments with a middle-dot separator.
func joinMiddots(parts []string) string {
	return strings.Join(parts, " · ")
}

// joinSemicolonList joins annotation parts with "; ".
func joinSemicolonList(parts []string) string {
	return strings.Join(parts, "; ")
}

// pluralPeople returns "person" or "people".
func pluralPeople(n int) string {
	if n == 1 {
		return "person"
	}
	return "people"
}

// formatExcludedFooter renders the reconciliation line for rows fetched
// but not counted as member actions. Returns "" when nothing was
// excluded so quiet windows carry no footer noise.
func formatExcludedFooter(internalCount, undoneCount int) string {
	var parts []string
	if internalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d bookkeeping event%s excluded", internalCount, pluralS(internalCount)))
	}
	if undoneCount > 0 {
		parts = append(parts, fmt.Sprintf("%d action%s undone", undoneCount, pluralS(undoneCount)))
	}
	return joinHumanList(parts)
}

func buildChatRows(c ChatTopicSummary, t ActivityTotals) []chatRow {
	rows := make([]chatRow, 0, 5)
	if c.AboutTransfers > 0 {
		denom := t.LoansStarted + t.GiveawaysStarted
		rows = append(rows, chatRow{
			Label: "About transfers (loans + giveaways)",
			Count: c.AboutTransfers,
			Ratio: perItemRatio(c.AboutTransfers, denom, "transfer started"),
		})
	}
	if c.AboutRequests > 0 {
		rows = append(rows, chatRow{
			Label: "About requests",
			Count: c.AboutRequests,
			Ratio: perItemRatio(c.AboutRequests, t.RequestsPosted, "request posted"),
		})
	}
	if c.AboutExperiences > 0 {
		rows = append(rows, chatRow{
			Label: "About events",
			Count: c.AboutExperiences,
			Ratio: perItemRatio(c.AboutExperiences, t.ExperiencesPosted, "event posted"),
		})
	}
	if c.AboutGear > 0 {
		rows = append(rows, chatRow{
			Label: "About gear (perpetual)",
			Count: c.AboutGear,
		})
	}
	if c.CommunityWide > 0 {
		rows = append(rows, chatRow{
			Label: "Community-wide",
			Count: c.CommunityWide,
		})
	}
	return rows
}

// perItemRatio returns "X.Y messages per <noun>" when denom > 0; empty
// otherwise. Single decimal so 14.3 reads better than 14.27.
func perItemRatio(numerator, denom int, noun string) string {
	if denom <= 0 {
		return ""
	}
	ratio := float64(numerator) / float64(denom)
	return fmt.Sprintf("%.1f per %s", ratio, noun)
}

// funnelDetail joins two count/label pairs with commas, skipping zeros.
// Returns "" when both are zero.
func funnelDetail(label1 string, count1 int, label2 string, count2 int) string {
	var parts []string
	if count1 > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", count1, label1))
	}
	if count2 > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", count2, label2))
	}
	if len(parts) == 0 {
		return ""
	}
	return joinHumanList(parts)
}

func formatRSVPDetail(yes, maybe, no int) string {
	if yes == 0 && maybe == 0 && no == 0 {
		return ""
	}
	total := yes + maybe + no
	return fmt.Sprintf("%d RSVPs (%dy / %dm / %dn)", total, yes, maybe, no)
}

func pluralLabel(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// formatWindowLabel renders a window like "2026-05-13" when start and
// end land on the same local day, else a range.
func formatWindowLabel(start, end time.Time) string {
	endInclusive := end.Add(-time.Nanosecond)
	if start.Format("2006-01-02") == endInclusive.Format("2006-01-02") {
		return start.Format("2006-01-02")
	}
	return fmt.Sprintf("%s through %s", start.Format("2006-01-02"), endInclusive.Format("2006-01-02"))
}

func displayTimezoneName(configured string, loc *time.Location) string {
	if configured != "" {
		return configured
	}
	return loc.String()
}

// formatAuthBreakdown renders "1 Google, 1 Apple, 1 phone".
func formatAuthBreakdown(breakdown map[string]int) string {
	if len(breakdown) == 0 {
		return ""
	}
	type kv struct {
		Label string
		Count int
	}
	rows := make([]kv, 0, len(breakdown))
	for label, count := range breakdown {
		if count <= 0 {
			continue
		}
		rows = append(rows, kv{Label: label, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Label < rows[j].Label
	})
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%d %s", r.Count, r.Label))
	}
	return joinHumanList(parts)
}

func joinHumanList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	var b bytes.Buffer
	last := len(parts) - 1
	for i, p := range parts {
		if i == last {
			b.WriteString("and ")
			b.WriteString(p)
			continue
		}
		b.WriteString(p)
		b.WriteString(", ")
	}
	return b.String()
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// renderActivityDigestText executes the plain-text template.
func renderActivityDigestText(tmpl *texttemplate.Template, in ActivityDigestInput) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, buildActivityDigestTemplateData(in)); err != nil {
		return "", fmt.Errorf("execute activity digest text template: %w", err)
	}
	return buf.String(), nil
}

// renderActivityDigestHTML executes the HTML template.
func renderActivityDigestHTML(tmpl *template.Template, in ActivityDigestInput) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, buildActivityDigestTemplateData(in)); err != nil {
		return "", fmt.Errorf("execute activity digest HTML template: %w", err)
	}
	return buf.String(), nil
}

// RenderActivityDigest is a public alternative to SendActivityDigest
// that returns the rendered (htmlBody, textBody, subject) tuple
// without touching Mailgun. Intended for offline preview tooling
// (server/cmd/preview-activity-digest). The bodies are byte-identical
// to what SendActivityDigest would put on the wire.
func RenderActivityDigest(in ActivityDigestInput) (htmlBody, textBody, subject string, err error) {
	htmlTmpl, err := parseEmailHTML("activityDigestHTML", activityDigestTemplateHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("parse activity digest HTML template: %w", err)
	}
	textTmpl, err := texttemplate.New("activityDigestText").Parse(activityDigestTemplateText)
	if err != nil {
		return "", "", "", fmt.Errorf("parse activity digest text template: %w", err)
	}
	htmlBody, err = renderActivityDigestHTML(htmlTmpl, in)
	if err != nil {
		return "", "", "", err
	}
	textBody, err = renderActivityDigestText(textTmpl, in)
	if err != nil {
		return "", "", "", err
	}
	return htmlBody, textBody, activityDigestSubject(in), nil
}

// activityDigestSubject returns the subject line for the email. The
// cadence label and the date format depend on Period.
func activityDigestSubject(in ActivityDigestInput) string {
	loc, err := time.LoadLocation(in.TimezoneName)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	start := time.Unix(in.WindowStartUnixSec, 0).In(loc)
	switch in.Period {
	case DigestPeriodWeekly:
		return fmt.Sprintf("Ripls weekly activity digest — week of %s", start.Format("2006-01-02"))
	default:
		return fmt.Sprintf("Ripls daily activity digest — %s", start.Format("2006-01-02"))
	}
}

// periodWord returns the cadence-flavoured time phrase used inline
// in copy ("today" for daily, "this week" for weekly).
func periodWord(p DigestPeriod) string {
	if p == DigestPeriodWeekly {
		return "this week"
	}
	return "today"
}

// digestTitle returns the title used in the email banner. Mirrors the
// subject's cadence label so an inbox preview and the open email don't
// disagree.
func digestTitle(p DigestPeriod) string {
	if p == DigestPeriodWeekly {
		return "Ripls weekly activity digest"
	}
	return "Ripls daily activity digest"
}

// formatPeriodWindowLabel renders the period-appropriate window label.
// Daily collapses a same-day [start, end) to one date; weekly always
// shows a range and prefixes "week of".
func formatPeriodWindowLabel(p DigestPeriod, start, end time.Time) string {
	endInclusive := end.Add(-time.Nanosecond)
	if p == DigestPeriodWeekly {
		return fmt.Sprintf("week of %s through %s", start.Format("2006-01-02"), endInclusive.Format("2006-01-02"))
	}
	return formatWindowLabel(start, end)
}

// SendActivityDigest sends the daily activity digest email.
func (s *MailgunService) SendActivityDigest(ctx context.Context, toEmail string, in ActivityDigestInput) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "SendActivityDigest",
		"recipient_email", logging.MaskEmail(toEmail),
		"email_type", "activity_digest",
		"window_start_unix_sec", in.WindowStartUnixSec,
		"window_end_unix_sec", in.WindowEndUnixSec,
		"timezone", in.TimezoneName,
		"new_user_count", len(in.NewUsers),
		"interactive_signin_count", in.InteractiveSignInCount,
		"active_user_count", in.ActiveUserCount,
		"active_contributor_count", in.ActiveContributorCount,
		"waitlist_signup_count", len(in.WaitlistSignups),
		"password_reset_count", in.PasswordResetCount,
		"community_count", len(in.Communities),
		"total_community_event_count", in.TotalMemberActionCount,
		"total_user_message_count", in.TotalUserMessageCount,
	)
	logger.InfoContext(ctx, "sending activity digest email")

	htmlBody, err := renderActivityDigestHTML(s.activityDigestHTMLTemplate, in)
	if err != nil {
		logger.ErrorContext(ctx, "failed to render activity digest HTML",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}
	textBody, err := renderActivityDigestText(s.activityDigestTextTemplate, in)
	if err != nil {
		logger.ErrorContext(ctx, "failed to render activity digest text",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return err
	}

	message := mailgun.NewMessage(s.domain, s.fromAddress, activityDigestSubject(in), textBody, toEmail)
	message.SetHTML(htmlBody)
	s.prepareOutbound(ctx, message, "activity_digest")

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := s.mg.Send(sendCtx, message); err != nil {
		logger.ErrorContext(ctx, "failed to send activity digest via mailgun",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("send activity digest via Mailgun: %w", err)
	}

	logger.InfoContext(ctx, "activity digest sent",
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return nil
}
