package email

// DigestPeriod selects the cadence label (and therefore the subject and
// banner copy) for a digest. Daily and weekly digests share the same
// rendered sections; only the wrapper text changes.
type DigestPeriod int

const (
	// DigestPeriodDaily is the once-a-day "yesterday" digest.
	DigestPeriodDaily DigestPeriod = iota
	// DigestPeriodWeekly is the Monday-morning "last week" digest.
	DigestPeriodWeekly
)

// ActivityDigestInput is the rendered shape of the activity digest
// email. The aggregator in server/activity_digest fills this in and
// hands it to Service.SendActivityDigest. The struct intentionally
// surfaces raw user-identifying values (names, emails, intended-use
// strings) because the email body needs them to be actionable; callers
// must still mask the same values when writing log lines.
type ActivityDigestInput struct {
	// Period chooses the cadence label used in the subject and the
	// email banner. Defaults to DigestPeriodDaily (zero value).
	Period DigestPeriod

	// Window is the [start, end) UTC unix-second range the digest covers.
	WindowStartUnixSec int64
	WindowEndUnixSec   int64

	// TimezoneName is the IANA name used to compute the window
	// (e.g. "America/Denver"). Rendered in the email header so the
	// recipient knows what "yesterday" means.
	TimezoneName string

	// GeneratedAtUnixSec is when the digest was assembled.
	GeneratedAtUnixSec int64

	// Engagement (auth) section.
	NewUsers             []DigestNewUser
	NewUserAuthBreakdown map[string]int // auth method label -> count
	// InteractiveSignInCount is the distinct count of returning users who
	// completed an interactive sign-in (password / one-time-code /
	// identity-provider) in the window. Token rotations are excluded —
	// see RefreshTokenOrigin.
	InteractiveSignInCount int
	WaitlistSignups        []DigestWaitlistSignup
	PasswordResetCount     int

	// ActiveContributorCount is the distinct count of users who performed
	// a counted action or sent a chat message in the window. Derivable
	// for any historical window.
	ActiveContributorCount int
	// ActiveUserCount is the distinct count of authenticated users seen
	// by the API in the window, from the user_active_day stamp table.
	// Trustworthy only when ActiveUserDataCoversWindow — the table
	// accrues from the feature's deploy forward.
	ActiveUserCount            int
	ActiveUserDataCoversWindow bool
	// DailyActives is the weekly digest's per-local-day breakdown; empty
	// for daily digests.
	DailyActives []DigestDayActives

	// Activity by entity type — the Metabase-style "what happened" summary.
	ActivityTotals ActivityTotals

	// Chat by topic — engagement split per conversation kind.
	ChatByTopic ChatTopicSummary

	// Top communities, ranked by combined activity. Each entry is a
	// short ranking row; there is no per-community narrative anymore.
	// Communities beyond the top rows are summarized in the rollups.
	Communities []DigestCommunity

	// AdHocTailRollup summarizes ad-hoc communities that didn't make the
	// listed rows (nil when none); NamedTailRollup does the same for
	// named communities.
	AdHocTailRollup *DigestCommunityRollup
	NamedTailRollup *DigestCommunityRollup

	// Totals across the window. TotalMemberActionCount is the number of
	// counted CommunityEvent rows — retractions and bookkeeping signals
	// are excluded and reported separately below so the email reconciles
	// exactly with the fetched rows.
	TotalMemberActionCount int
	TotalUserMessageCount  int

	// Reconciliation counts for rows fetched but not counted as actions.
	UndoneEventCount   int // UNDONE retraction rows
	InternalEventCount int // bookkeeping signals (e.g. roster-refresh pings)

	// Prior holds the previous window's headline metrics (previous local
	// day for daily, previous week for weekly) so headline numbers can
	// carry deltas. Nil when unavailable.
	Prior *DigestPriorPeriod
}

// DigestPriorPeriod is the slim prior-window aggregate used only for
// delta rendering — no per-community or per-user detail.
type DigestPriorPeriod struct {
	// WindowLabel names the prior window, e.g. "2026-05-12" or
	// "week of 2026-05-04".
	WindowLabel string

	ActivityTotals         ActivityTotals
	TotalMemberActionCount int
	TotalUserMessageCount  int
	NewUserCount           int
	ActiveContributorCount int
	// ActiveUserCount is meaningful only when
	// ActiveUserDataCoversWindow (see ActivityDigestInput).
	ActiveUserCount            int
	ActiveUserDataCoversWindow bool
}

// DigestDayActives is one local day's active-user counts in the weekly
// digest's actives-by-day breakdown.
type DigestDayActives struct {
	// DateLabel is the rendered local date, e.g. "Mon 2026-06-29".
	DateLabel string
	// ActiveUsers is the distinct app-active count for the day, or -1
	// when the stamp table doesn't cover that day yet.
	ActiveUsers int
	// Contributors is the distinct count of users with a counted action
	// that day (chat senders are only unioned into the window total).
	Contributors int
}

// DigestNewUser describes a single account created in the window.
type DigestNewUser struct {
	Name                 string
	Email                string
	AuthMethodLabel      string
	PrimaryCommunityName string // empty if the user has no primary community yet
}

// DigestWaitlistSignup describes a single waitlist entry created in the window.
type DigestWaitlistSignup struct {
	Email       string
	Name        string
	IntendedUse string
}

// ActivityTotals captures aggregate per-entity-type counts of activity
// in the window. Each entity type (gear / transfer / request /
// experience / membership) has its own funnel-style fields so the email
// can show motion through the lifecycle at a glance.
type ActivityTotals struct {
	// Gear.
	GearShared   int
	GearUnshared int

	// Transfers — loans and giveaways tracked separately because they
	// have different lifecycles to ops.
	LoansStarted       int
	LoansCompleted     int
	LoansCancelled     int
	GiveawaysStarted   int
	GiveawaysCompleted int
	GiveawaysCancelled int

	// Transfer coordination steps shared by loans and giveaways — the
	// funnel motion between "shared" and "started".
	TransferInterestExpressed  int
	TransferInterestWithdrawn  int
	TransferRecipientsSelected int
	TransferPickupsProposed    int

	// Requests.
	RequestsPosted         int
	RequestOffersMade      int
	RequestOffersSelected  int
	RequestOffersWithdrawn int
	RequestsFulfilled      int
	RequestsCancelled      int

	// Experiences (events).
	ExperiencesPosted    int
	ExperiencesStarted   int
	ExperiencesUpdated   int
	ExperiencesCompleted int
	ExperiencesCancelled int
	RSVPYes              int
	RSVPMaybe            int
	RSVPNo               int

	// Planning — collaborative needs & contributions on experiences and
	// requests.
	PlanningNeedsAdded           int
	PlanningNeedsRemoved         int
	PlanningNeedsClaimed         int
	PlanningNeedsUpdated         int
	PlanningContributionsAdded   int
	PlanningContributionsRemoved int

	// Community lifecycle / membership.
	InvitationsRedeemed  int
	MembersLeft          int
	MembersRejoined      int
	CommunitiesCreated   int
	CommunitiesNamed     int
	CommunitiesDeleted   int
	CommunitiesRestored  int
	OwnershipTransferred int
}

// ChatTopicSummary breaks chat-message counts down by the entity type
// each conversation is scoped to.
type ChatTopicSummary struct {
	AboutTransfers   int
	AboutRequests    int
	AboutExperiences int
	AboutGear        int
	CommunityWide    int
}

// Total returns the sum across all topic types.
func (c ChatTopicSummary) Total() int {
	return c.AboutTransfers + c.AboutRequests + c.AboutExperiences + c.AboutGear + c.CommunityWide
}

// DigestCategoryCount is one reporting category's action count within a
// community row or rollup line — e.g. {Label: "loans", Count: 3}
// renders as "3 loans". The label arrives pre-pluralized.
type DigestCategoryCount struct {
	Label string
	Count int
}

// DigestCommunity is a compact per-community ranking row. The digest no
// longer prints individual events; the aggregate counts live on
// ActivityTotals.
type DigestCommunity struct {
	// Name is the community's own name; empty for an ad-hoc (nameless)
	// per-item community — the renderer then labels the row from IsAdHoc
	// + OriginLabel.
	Name string
	// IsAdHoc marks a nameless per-item community (#2492).
	IsAdHoc bool
	// OriginLabel describes what an ad-hoc community formed around, e.g.
	// `event "Board-game night"` or `gear "Canoe"`. Empty when the
	// origin item could not be resolved.
	OriginLabel string
	// Deleted marks a soft-deleted community that still had activity in
	// the window (e.g. it was deleted during it).
	Deleted bool
	// MemberCount is the current active-member count.
	MemberCount int
	// ActionCount is the number of counted member actions in the window.
	ActionCount int
	// UserMessageCount / DistinctSenderCount describe chat activity.
	UserMessageCount    int
	DistinctSenderCount int
	// TopCategories are the up-to-three largest action categories,
	// largest first.
	TopCategories []DigestCategoryCount
	ActivityScore int // ordering key
}

// DigestCommunityRollup summarizes the communities beyond the listed
// top rows so the section stays legible when ad-hoc groups are many.
type DigestCommunityRollup struct {
	CommunityCount int
	ActionCount    int
	MessageCount   int
	// KindCounts breaks an ad-hoc rollup down by origin item kind
	// ("around events", "around gear", ...); empty for the named rollup.
	KindCounts []DigestCategoryCount
}
