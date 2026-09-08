package email

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenderActivityDigest_FixturePreview renders a fully-populated
// weekly digest and writes both bodies to DIGEST_PREVIEW_DIR when set —
// an offline stand-in for server/cmd/preview-activity-digest that needs
// no database. Always asserts the render succeeds with every section
// populated simultaneously (sections can interact in template
// whitespace handling).
func TestRenderActivityDigest_FixturePreview(t *testing.T) {
	loc, _ := time.LoadLocation("America/Denver")
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 7)

	in := ActivityDigestInput{
		Period:             DigestPeriodWeekly,
		WindowStartUnixSec: start.Unix(),
		WindowEndUnixSec:   end.Unix(),
		TimezoneName:       "America/Denver",
		GeneratedAtUnixSec: end.Unix(),
		NewUsers: []DigestNewUser{
			{Name: "Carol", Email: "carol@example.com", AuthMethodLabel: "phone", PrimaryCommunityName: `ad hoc group around event "Board-game night"`},
			{Name: "Dave", Email: "dave@example.com", AuthMethodLabel: "Google", PrimaryCommunityName: "Maple Street"},
		},
		NewUserAuthBreakdown:       map[string]int{"phone": 1, "Google": 1},
		InteractiveSignInCount:     4,
		WaitlistSignups:            []DigestWaitlistSignup{{Email: "wait@example.com", Name: "Wanda", IntendedUse: "borrow tools"}},
		PasswordResetCount:         1,
		ActiveContributorCount:     14,
		ActiveUserCount:            21,
		ActiveUserDataCoversWindow: true,
		DailyActives: []DigestDayActives{
			{DateLabel: "Mon 2026-06-29", ActiveUsers: 12, Contributors: 7},
			{DateLabel: "Tue 2026-06-30", ActiveUsers: 9, Contributors: 4},
			{DateLabel: "Wed 2026-07-01", ActiveUsers: -1, Contributors: 5},
		},
		ActivityTotals: ActivityTotals{
			GearShared: 5, GearUnshared: 1,
			LoansStarted: 3, LoansCompleted: 2, LoansCancelled: 1,
			GiveawaysStarted: 2, GiveawaysCompleted: 1,
			TransferInterestExpressed: 6, TransferInterestWithdrawn: 1,
			TransferRecipientsSelected: 2, TransferPickupsProposed: 3,
			RequestsPosted: 4, RequestOffersMade: 5, RequestOffersSelected: 2,
			RequestOffersWithdrawn: 1, RequestsFulfilled: 2, RequestsCancelled: 1,
			ExperiencesPosted: 3, ExperiencesStarted: 2, ExperiencesUpdated: 1,
			ExperiencesCompleted: 2, ExperiencesCancelled: 1,
			RSVPYes: 12, RSVPMaybe: 4, RSVPNo: 2,
			PlanningNeedsAdded: 6, PlanningNeedsClaimed: 4, PlanningNeedsUpdated: 1,
			PlanningNeedsRemoved: 1, PlanningContributionsAdded: 5, PlanningContributionsRemoved: 1,
			InvitationsRedeemed: 7, MembersLeft: 1, MembersRejoined: 1,
			CommunitiesCreated: 2, CommunitiesNamed: 1, CommunitiesDeleted: 1,
			CommunitiesRestored: 1, OwnershipTransferred: 1,
		},
		ChatByTopic: ChatTopicSummary{AboutTransfers: 25, AboutRequests: 16, AboutExperiences: 9, AboutGear: 4, CommunityWide: 2},
		Communities: []DigestCommunity{
			{
				Name: "Maple Street", MemberCount: 14, ActionCount: 22, UserMessageCount: 30,
				DistinctSenderCount: 8, ActivityScore: 74,
				TopCategories: []DigestCategoryCount{{Label: "loans", Count: 9}, {Label: "events", Count: 7}, {Label: "requests", Count: 4}},
			},
			{
				IsAdHoc: true, OriginLabel: `event "Board-game night"`, MemberCount: 5,
				ActionCount: 9, UserMessageCount: 12, DistinctSenderCount: 4, ActivityScore: 30,
				TopCategories: []DigestCategoryCount{{Label: "events", Count: 8}, {Label: "planning", Count: 1}},
			},
			{
				Name: "Old Crew", Deleted: true, MemberCount: 3, ActionCount: 1, ActivityScore: 2,
				TopCategories: []DigestCategoryCount{{Label: "community", Count: 1}},
			},
		},
		AdHocTailRollup: &DigestCommunityRollup{
			CommunityCount: 6, ActionCount: 9, MessageCount: 4,
			KindCounts: []DigestCategoryCount{{Label: "around events", Count: 4}, {Label: "around gear", Count: 2}},
		},
		NamedTailRollup:        &DigestCommunityRollup{CommunityCount: 2, ActionCount: 3, MessageCount: 1},
		TotalMemberActionCount: 96,
		TotalUserMessageCount:  56,
		UndoneEventCount:       2,
		InternalEventCount:     5,
		Prior: &DigestPriorPeriod{
			WindowLabel:                "week of 2026-06-22",
			ActivityTotals:             ActivityTotals{LoansStarted: 5, RequestsPosted: 2, ExperiencesPosted: 4},
			TotalMemberActionCount:     70,
			TotalUserMessageCount:      61,
			NewUserCount:               1,
			ActiveContributorCount:     11,
			ActiveUserCount:            18,
			ActiveUserDataCoversWindow: true,
		},
	}

	htmlBody, textBody, subject, err := RenderActivityDigest(in)
	if err != nil {
		t.Fatalf("RenderActivityDigest: %v", err)
	}
	if subject == "" || htmlBody == "" || textBody == "" {
		t.Fatal("empty render output")
	}

	if dir := os.Getenv("DIGEST_PREVIEW_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "digest_fixture.html"), []byte(htmlBody), 0o644); err != nil {
			t.Fatalf("write html: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "digest_fixture.txt"), []byte(textBody), 0o644); err != nil {
			t.Fatalf("write text: %v", err)
		}
	}
}
