package email

import (
	"context"
	"html"
	"strings"
	"testing"
	"time"
)

// fixtureInput returns a baseline ActivityDigestInput covering 2026-05-13
// in America/Denver. Tests mutate the returned value to vary specific
// sections.
func fixtureInput() ActivityDigestInput {
	loc, _ := time.LoadLocation("America/Denver")
	start := time.Date(2026, 5, 13, 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	return ActivityDigestInput{
		WindowStartUnixSec:   start.Unix(),
		WindowEndUnixSec:     end.Unix(),
		TimezoneName:         "America/Denver",
		GeneratedAtUnixSec:   end.Unix(),
		NewUserAuthBreakdown: map[string]int{},
	}
}

func renderBoth(t *testing.T, in ActivityDigestInput) (htmlBody, textBody string) {
	t.Helper()
	svc, err := NewMailgunService("example.com", "test-key", "from@example.com")
	if err != nil {
		t.Fatalf("NewMailgunService: %v", err)
	}
	htmlBody, err = renderActivityDigestHTML(svc.activityDigestHTMLTemplate, in)
	if err != nil {
		t.Fatalf("renderActivityDigestHTML: %v", err)
	}
	textBody, err = renderActivityDigestText(svc.activityDigestTextTemplate, in)
	if err != nil {
		t.Fatalf("renderActivityDigestText: %v", err)
	}
	return htmlBody, textBody
}

func TestActivityDigest_ZeroActivity(t *testing.T) {
	in := fixtureInput()
	htmlBody, textBody := renderBoth(t, in)

	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "No new users today") {
			t.Errorf("zero-activity body missing 'No new users today': %s", body)
		}
		if !strings.Contains(body, "No returning interactive sign-ins today") {
			t.Errorf("zero-activity body missing 'No returning interactive sign-ins today': %s", body)
		}
		if !strings.Contains(body, "App-active tracking does not cover this window yet") {
			t.Errorf("zero-activity body missing actives-coverage note: %s", body)
		}
		if !strings.Contains(body, "No tracked activity in the window") {
			t.Errorf("zero-activity body missing activity placeholder: %s", body)
		}
		if !strings.Contains(body, "No chat messages in the window") {
			t.Errorf("zero-activity body missing chat placeholder: %s", body)
		}
	}
}

func TestActivityDigest_EngagementOnly(t *testing.T) {
	in := fixtureInput()
	in.NewUsers = []DigestNewUser{
		{Name: "Alice", Email: "alice@example.com", AuthMethodLabel: "Google", PrimaryCommunityName: "Backyard Tinkerers"},
		{Name: "Bob", Email: "bob@example.com", AuthMethodLabel: "Apple"},
	}
	in.NewUserAuthBreakdown = map[string]int{"Google": 1, "Apple": 1}
	in.InteractiveSignInCount = 17
	in.ActiveUserCount = 23
	in.ActiveUserDataCoversWindow = true
	in.ActiveContributorCount = 9
	in.WaitlistSignups = []DigestWaitlistSignup{
		{Email: "carol@example.com", Name: "Carol", IntendedUse: "borrow tools"},
	}
	in.PasswordResetCount = 1

	htmlBody, textBody := renderBoth(t, in)

	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "alice@example.com") {
			t.Errorf("body missing alice email: %s", body)
		}
		if !strings.Contains(body, "Backyard Tinkerers") {
			t.Errorf("body missing primary community: %s", body)
		}
		if !strings.Contains(body, "17") {
			t.Errorf("body missing interactive sign-in count: %s", body)
		}
		if !strings.Contains(body, "23") || !strings.Contains(body, "9") {
			t.Errorf("body missing active-user / contributor counts: %s", body)
		}
		if strings.Contains(body, "App-active tracking does not cover") {
			t.Errorf("covered window should not carry the coverage note: %s", body)
		}
		if !strings.Contains(body, "carol@example.com") {
			t.Errorf("body missing waitlist email: %s", body)
		}
		if !strings.Contains(body, "borrow tools") {
			t.Errorf("body missing intended use: %s", body)
		}
		if !strings.Contains(body, "1 Google") || !strings.Contains(body, "1 Apple") {
			t.Errorf("body missing auth breakdown: %s", body)
		}
		if !strings.Contains(body, "password reset") {
			t.Errorf("body missing password reset line: %s", body)
		}
	}
}

func TestActivityDigest_ActivityTotalsRendered(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals = ActivityTotals{
		GearShared:           5,
		LoansStarted:         3,
		LoansCompleted:       1,
		GiveawaysStarted:     2,
		RequestsPosted:       4,
		RequestOffersMade:    2,
		RequestsFulfilled:    1,
		ExperiencesPosted:    3,
		ExperiencesCompleted: 2,
		RSVPYes:              8,
		RSVPMaybe:            3,
		RSVPNo:               1,
		InvitationsRedeemed:  2,
	}

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "Gear items shared") {
			t.Errorf("body missing gear-shared row: %s", body)
		}
		if !strings.Contains(body, "Loans started") {
			t.Errorf("body missing loans row: %s", body)
		}
		if !strings.Contains(body, "Giveaways started") {
			t.Errorf("body missing giveaways row: %s", body)
		}
		if !strings.Contains(body, "Requests posted") {
			t.Errorf("body missing requests row: %s", body)
		}
		if !strings.Contains(body, "Events posted") {
			t.Errorf("body missing events row: %s", body)
		}
		if !strings.Contains(body, "12 RSVPs (8y / 3m / 1n)") {
			t.Errorf("body missing RSVP breakdown: %s", body)
		}
		if !strings.Contains(body, "1 completed") {
			t.Errorf("body missing loan-completed funnel detail: %s", body)
		}
		if !strings.Contains(body, "Members joined via invite") {
			t.Errorf("body missing invitations row: %s", body)
		}
	}
}

func TestActivityDigest_NewCategoryRowsRendered(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals = ActivityTotals{
		TransferInterestExpressed:  3,
		TransferRecipientsSelected: 1,
		TransferPickupsProposed:    2,
		RequestOffersSelected:      1,
		RequestOffersWithdrawn:     1,
		ExperiencesStarted:         2,
		ExperiencesUpdated:         1,
		PlanningNeedsAdded:         4,
		PlanningNeedsClaimed:       2,
		PlanningContributionsAdded: 3,
		CommunitiesNamed:           1,
	}
	in.TotalMemberActionCount = 21
	in.UndoneEventCount = 2
	in.InternalEventCount = 3

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "Transfer coordination") {
			t.Errorf("body missing transfer coordination row: %s", body)
		}
		if !strings.Contains(body, "3 interest expressed") {
			t.Errorf("body missing interest-expressed detail: %s", body)
		}
		if !strings.Contains(body, "2 pickups proposed") {
			t.Errorf("body missing pickup detail: %s", body)
		}
		if !strings.Contains(body, "Planning activity") {
			t.Errorf("body missing planning row: %s", body)
		}
		if !strings.Contains(body, "4 needs added") {
			t.Errorf("body missing needs-added detail: %s", body)
		}
		if !strings.Contains(body, "1 ad-hoc group named") {
			t.Errorf("body missing communities-named detail: %s", body)
		}
		if !strings.Contains(body, "Member actions") {
			t.Errorf("body missing member-actions total label: %s", body)
		}
		if strings.Contains(body, "Community events:") {
			t.Errorf("body still contains legacy 'Community events:' label: %s", body)
		}
		if !strings.Contains(body, "3 bookkeeping events excluded and 2 actions undone") {
			t.Errorf("body missing reconciliation footer: %s", body)
		}
	}
}

func TestActivityDigest_WeeklyDailyActivesTable(t *testing.T) {
	in := fixtureInput()
	in.Period = DigestPeriodWeekly
	in.DailyActives = []DigestDayActives{
		{DateLabel: "Mon 2026-05-11", ActiveUsers: 14, Contributors: 9},
		{DateLabel: "Tue 2026-05-12", ActiveUsers: -1, Contributors: 3},
	}

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "Actives by day") {
			t.Errorf("body missing actives-by-day section: %s", body)
		}
		if !strings.Contains(body, "Mon 2026-05-11") || !strings.Contains(body, "14 active / 9 contributing") {
			t.Errorf("body missing Monday actives row: %s", body)
		}
		if !strings.Contains(body, "n/a active / 3 contributing") {
			t.Errorf("body missing pre-coverage n/a row: %s", body)
		}
	}
}

func TestActivityDigest_PriorPeriodDeltas(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals = ActivityTotals{LoansStarted: 12, RequestsPosted: 2}
	in.TotalMemberActionCount = 14
	in.TotalUserMessageCount = 30
	in.NewUsers = []DigestNewUser{{Name: "A", Email: "a@x.com", AuthMethodLabel: "phone"}}
	in.ActiveContributorCount = 6
	in.ActiveUserCount = 10
	in.ActiveUserDataCoversWindow = true
	in.Prior = &DigestPriorPeriod{
		WindowLabel:            "2026-05-12",
		ActivityTotals:         ActivityTotals{LoansStarted: 8, RequestsPosted: 3},
		TotalMemberActionCount: 11,
		TotalUserMessageCount:  35,
		NewUserCount:           1,
		ActiveContributorCount: 6,
		// Prior window predates stamp coverage — active-users delta must
		// be suppressed even though the current window is covered.
		ActiveUserCount:            0,
		ActiveUserDataCoversWindow: false,
	}

	htmlBody, textBody := renderBoth(t, in)
	// html/template entity-escapes "+" and "±"; unescape before asserting.
	for _, body := range []string{html.UnescapeString(htmlBody), textBody} {
		if !strings.Contains(body, "(+4)") {
			t.Errorf("body missing loans-row delta (+4): %s", body)
		}
		if !strings.Contains(body, "(-1)") {
			t.Errorf("body missing requests-row delta (-1): %s", body)
		}
		if !strings.Contains(body, "(+3)") {
			t.Errorf("body missing member-actions delta (+3): %s", body)
		}
		if !strings.Contains(body, "(-5)") {
			t.Errorf("body missing messages delta (-5): %s", body)
		}
		if !strings.Contains(body, "(±0)") {
			t.Errorf("body missing contributors zero delta (±0): %s", body)
		}
		if !strings.Contains(body, "Deltas compare to 2026-05-12") {
			t.Errorf("body missing delta footnote: %s", body)
		}
	}
	// Active-users delta suppressed: 10 vs prior 0 would render "(+10)".
	if strings.Contains(textBody, "(+10)") {
		t.Errorf("active-users delta should be suppressed without prior coverage: %s", textBody)
	}
}

func TestActivityDigest_NoDeltasWithoutPrior(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals = ActivityTotals{LoansStarted: 12}
	in.TotalMemberActionCount = 12

	_, textBody := renderBoth(t, in)
	if strings.Contains(textBody, "Deltas compare") {
		t.Errorf("no-prior render should carry no delta footnote: %s", textBody)
	}
	if strings.Contains(textBody, "(+") || strings.Contains(textBody, "(±") {
		t.Errorf("no-prior render should carry no delta annotations: %s", textBody)
	}
}

func TestActivityDigest_NoExcludedFooterWhenClean(t *testing.T) {
	in := fixtureInput()
	in.TotalMemberActionCount = 5
	_, textBody := renderBoth(t, in)
	if strings.Contains(textBody, "excluded") || strings.Contains(textBody, "undone") {
		t.Errorf("clean window should carry no reconciliation footer: %s", textBody)
	}
}

func TestActivityDigest_ChatBreakdownWithRatios(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals = ActivityTotals{
		LoansStarted:      3,
		GiveawaysStarted:  2,
		RequestsPosted:    4,
		ExperiencesPosted: 3,
	}
	in.ChatByTopic = ChatTopicSummary{
		AboutTransfers:   25,
		AboutRequests:    16,
		AboutExperiences: 9,
		AboutGear:        4,
		CommunityWide:    2,
	}

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "About transfers") {
			t.Errorf("body missing transfers chat row: %s", body)
		}
		// 25 messages across 5 transfers = 5.0 per transfer started.
		if !strings.Contains(body, "5.0 per transfer started") {
			t.Errorf("body missing transfers ratio: %s", body)
		}
		// 16 / 4 = 4.0 per request posted.
		if !strings.Contains(body, "4.0 per request posted") {
			t.Errorf("body missing requests ratio: %s", body)
		}
		// 9 / 3 = 3.0 per event posted.
		if !strings.Contains(body, "3.0 per event posted") {
			t.Errorf("body missing events ratio: %s", body)
		}
		if !strings.Contains(body, "Community-wide") {
			t.Errorf("body missing community-wide chat row: %s", body)
		}
	}
}

func TestActivityDigest_TopCommunitiesRanking(t *testing.T) {
	in := fixtureInput()
	in.Communities = []DigestCommunity{
		{
			Name: "Alpha", MemberCount: 14, ActionCount: 10, UserMessageCount: 30,
			DistinctSenderCount: 5, ActivityScore: 50,
			TopCategories: []DigestCategoryCount{{Label: "loans", Count: 7}, {Label: "requests", Count: 3}},
		},
		{Name: "Bravo", ActionCount: 5, UserMessageCount: 5, ActivityScore: 15},
	}
	in.TotalMemberActionCount = 15
	in.TotalUserMessageCount = 35

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, "Alpha (14 members)") {
			t.Errorf("body missing Alpha title with member count: %s", body)
		}
		if !strings.Contains(body, "7 loans and 3 requests") {
			t.Errorf("body missing Alpha category breakdown: %s", body)
		}
		if !strings.Contains(body, "30 messages from 5 people") {
			t.Errorf("body missing Alpha chat detail: %s", body)
		}
		if !strings.Contains(body, "Bravo") {
			t.Errorf("body missing Bravo row: %s", body)
		}
	}
}

func TestActivityDigest_AdHocAndDeletedCommunityRows(t *testing.T) {
	in := fixtureInput()
	in.Communities = []DigestCommunity{
		{
			IsAdHoc: true, OriginLabel: `event "Board-game night"`,
			MemberCount: 5, ActionCount: 4, UserMessageCount: 6,
			DistinctSenderCount: 3, ActivityScore: 14,
			TopCategories: []DigestCategoryCount{{Label: "events", Count: 4}},
		},
		{
			Name: "Old Crew", Deleted: true, ActionCount: 1, ActivityScore: 2,
			TopCategories: []DigestCategoryCount{{Label: "community", Count: 1}},
		},
		{IsAdHoc: true, MemberCount: 2, ActionCount: 1, ActivityScore: 2},
	}
	in.AdHocTailRollup = &DigestCommunityRollup{
		CommunityCount: 7, ActionCount: 10, MessageCount: 3,
		KindCounts: []DigestCategoryCount{
			{Label: "around events", Count: 4},
			{Label: "around gear", Count: 2},
			{Label: "unknown origin", Count: 1},
		},
	}
	in.NamedTailRollup = &DigestCommunityRollup{CommunityCount: 3, ActionCount: 5, MessageCount: 2}

	htmlBody, textBody := renderBoth(t, in)
	for _, body := range []string{htmlBody, textBody} {
		if !strings.Contains(body, `Ad hoc · around event &#34;Board-game night&#34; (5 members)`) &&
			!strings.Contains(body, `Ad hoc · around event "Board-game night" (5 members)`) {
			t.Errorf("body missing labeled ad-hoc row: %s", body)
		}
		if !strings.Contains(body, "Old Crew (deleted)") {
			t.Errorf("body missing deleted annotation: %s", body)
		}
		if !strings.Contains(body, "Ad hoc group (2 members)") {
			t.Errorf("body missing unresolved ad-hoc fallback: %s", body)
		}
		if strings.Contains(body, "Unknown community") {
			t.Errorf("body still contains 'Unknown community': %s", body)
		}
		if !strings.Contains(body, "…and 7 more ad-hoc groups (10 actions, 3 messages): 4 around events, 2 around gear, and 1 unknown origin") {
			t.Errorf("body missing ad-hoc rollup line: %s", body)
		}
		if !strings.Contains(body, "…and 3 more communities (5 actions, 2 messages)") {
			t.Errorf("body missing named rollup line: %s", body)
		}
	}
}

func TestActivityDigest_MockServiceRecords(t *testing.T) {
	mock := &MockEmailService{}
	in := fixtureInput()
	in.InteractiveSignInCount = 5

	if err := mock.SendActivityDigest(context.Background(), "ops@example.com", in); err != nil {
		t.Fatalf("SendActivityDigest: %v", err)
	}
	if len(mock.ActivityDigests) != 1 {
		t.Fatalf("expected 1 recorded digest, got %d", len(mock.ActivityDigests))
	}
	got := mock.ActivityDigests[0]
	if got.ToEmail != "ops@example.com" {
		t.Errorf("ToEmail = %q, want ops@example.com", got.ToEmail)
	}
	if got.Input.InteractiveSignInCount != 5 {
		t.Errorf("InteractiveSignInCount = %d, want 5", got.Input.InteractiveSignInCount)
	}

	mock.Reset()
	if len(mock.ActivityDigests) != 0 {
		t.Errorf("Reset did not clear ActivityDigests")
	}
}

func TestActivityDigest_SubjectIncludesDate(t *testing.T) {
	in := fixtureInput()
	got := activityDigestSubject(in)
	if !strings.Contains(got, "2026-05-13") {
		t.Errorf("subject %q missing window date", got)
	}
}

func TestActivityDigest_InvalidTimezoneFallsBackToUTC(t *testing.T) {
	in := fixtureInput()
	in.TimezoneName = "Not/AReal_Zone"
	_, textBody := renderBoth(t, in)
	if textBody == "" {
		t.Error("invalid timezone produced empty body")
	}
}

func TestActivityDigest_RenderHelperMatchesSendBody(t *testing.T) {
	in := fixtureInput()
	in.ActivityTotals.GearShared = 1
	html, text, subj, err := RenderActivityDigest(in)
	if err != nil {
		t.Fatalf("RenderActivityDigest: %v", err)
	}
	if !strings.Contains(html, "Gear item shared") {
		t.Errorf("HTML missing gear row: %s", html)
	}
	if !strings.Contains(text, "Gear item shared") {
		t.Errorf("text missing gear row: %s", text)
	}
	if !strings.Contains(subj, "2026-05-13") {
		t.Errorf("subject missing date: %s", subj)
	}
}
