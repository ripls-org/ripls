package activity_digest

import (
	"context"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// Builder assembles an email.ActivityDigestInput from raw storage
// queries. The zero value is not usable — construct via New.
type Builder struct {
	storage *storage.ProtoSQLStorage
	now     func() time.Time
}

// New returns a Builder bound to the given storage handle. now is used
// to stamp the GeneratedAt field on the output; pass nil for
// time.Now.
func New(s *storage.ProtoSQLStorage, now func() time.Time) *Builder {
	if now == nil {
		now = time.Now
	}
	return &Builder{storage: s, now: now}
}

// Build returns an ActivityDigestInput covering the local day that
// contains localDate, interpreted in the timezone localDate was
// constructed in.
//
// "Local day" means: the window starts at 00:00 in the timezone of
// localDate and ends at 00:00 the next calendar day in that same
// timezone. localDate's time component is ignored.
func (b *Builder) Build(ctx context.Context, localDate time.Time) (email.ActivityDigestInput, error) {
	loc := localDate.Location()
	dayStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	return b.BuildRange(ctx, dayStart, dayEnd, email.DigestPeriodDaily)
}

// BuildRange aggregates over an arbitrary [start, end) range — used by
// the weekly job (and any future custom windows). period controls the
// banner copy and subject; nothing about the aggregation pipeline
// itself changes between daily and weekly.
func (b *Builder) BuildRange(
	ctx context.Context, start, end time.Time, period email.DigestPeriod,
) (email.ActivityDigestInput, error) {
	loc := start.Location()
	startUnix := start.Unix()
	endUnix := end.Unix()

	// Parallel fan-out: nine independent storage queries.
	var (
		events           []*models.CommunityEvent
		chatRows         []storage.ChatActivityRow
		chatTopics       storage.ChatTopicCounts
		chatSenderIDs    []string
		newUsers         []*models.User
		waitlist         []*models.WaitlistEntry
		passwordResets   int
		activeSpans      []storage.UserActiveSpan
		earliestActivity int64
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		events, err = b.storage.FindEventsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		chatRows, err = b.storage.CountUserMessagesByCommunity(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		chatTopics, err = b.storage.CountUserMessagesByTopicType(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		newUsers, err = b.storage.FindUsersCreatedInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		waitlist, err = b.storage.FindWaitlistSignupsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		passwordResets, err = b.storage.CountPasswordResetsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		chatSenderIDs, err = b.storage.FindDistinctChatSenderIDsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		activeSpans, err = b.storage.FindUserActiveSpansInWindow(gctx, startUnix, endUnix)
		if err != nil {
			return err
		}
		earliestActivity, err = b.storage.EarliestUserActivityUnixSec(gctx)
		return err
	})
	if err := g.Wait(); err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: fan-out queries: %w", err)
	}

	newUserIDs := make([]string, 0, len(newUsers))
	for _, u := range newUsers {
		newUserIDs = append(newUserIDs, u.Id)
	}
	interactiveSignIns, err := b.storage.CountDistinctInteractiveSignInsInWindow(ctx, startUnix, endUnix, newUserIDs)
	if err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: count interactive sign-ins: %w", err)
	}

	// Aggregate events into ActivityTotals + per-community counts via the
	// classification registry. Retractions and bookkeeping signals are
	// excluded from all action counts but tallied so the email footer can
	// reconcile counted + excluded == fetched.
	totals := email.ActivityTotals{}
	perCommunityEventCount := map[string]int{}
	perCommunityCategory := map[string]map[Category]int{}
	countedEvents := 0
	undoneEvents := 0
	internalEvents := 0
	communityIDSet := map[string]struct{}{}
	warnedTypes := map[models.CommunityEventType]struct{}{}
	actorSet := map[string]struct{}{}
	// Per-local-day actor sets feed the weekly digest's actives-by-day
	// breakdown; skipped for daily windows.
	var actorsByDay map[string]map[string]struct{}
	if period == email.DigestPeriodWeekly {
		actorsByDay = map[string]map[string]struct{}{}
	}
	for _, ev := range events {
		entry := Registry[ev.EventType]
		switch entry.Disposition {
		case DispositionExcludedRetraction:
			undoneEvents++
			continue
		case DispositionExcludedInternal:
			internalEvents++
			continue
		case DispositionCounted:
			// Counted below.
		default:
			// Unreachable while TestEveryDigestEventTypeClassified passes;
			// count the row anyway so a drifted binary over-reports into
			// "member actions" rather than silently dropping activity.
			if _, seen := warnedTypes[ev.EventType]; !seen {
				warnedTypes[ev.EventType] = struct{}{}
				logging.LoggerWithContext(ctx).WarnContext(ctx,
					"activity digest: unclassified community event type",
					"event_type", ev.EventType.String(),
				)
			}
		}
		countedEvents++
		if ev.ActorId != "" {
			actorSet[ev.ActorId] = struct{}{}
			if actorsByDay != nil {
				day := time.Unix(ev.OccurredAtUnixSec, 0).In(loc).Format("2006-01-02")
				if actorsByDay[day] == nil {
					actorsByDay[day] = map[string]struct{}{}
				}
				actorsByDay[day][ev.ActorId] = struct{}{}
			}
		}
		if ev.CommunityId != "" {
			perCommunityEventCount[ev.CommunityId]++
			communityIDSet[ev.CommunityId] = struct{}{}
			cat := CategoryUnspecified
			if entry.CategoryOf != nil {
				cat = entry.CategoryOf(ev)
			}
			if perCommunityCategory[ev.CommunityId] == nil {
				perCommunityCategory[ev.CommunityId] = map[Category]int{}
			}
			perCommunityCategory[ev.CommunityId][cat]++
		}
		if entry.Apply != nil {
			entry.Apply(ev, &totals)
		}
	}
	// Active contributors: distinct users who performed a counted action
	// or sent a chat message. Computable for any historical window.
	contributorSet := make(map[string]struct{}, len(actorSet)+len(chatSenderIDs))
	for id := range actorSet {
		contributorSet[id] = struct{}{}
	}
	for _, id := range chatSenderIDs {
		contributorSet[id] = struct{}{}
	}

	// App-active users from the user_active_day stamp table. The table
	// only accrues from the feature's deploy forward, so the count is
	// only trustworthy when the oldest stamp predates the window.
	activesCovered := earliestActivity > 0 && earliestActivity <= startUnix
	activeUserSet := map[string]struct{}{}
	for _, span := range activeSpans {
		activeUserSet[span.UserID] = struct{}{}
	}
	var dailyActives []email.DigestDayActives
	if period == email.DigestPeriodWeekly {
		for dayStart := start; dayStart.Before(end); dayStart = dayStart.AddDate(0, 0, 1) {
			dayEnd := dayStart.AddDate(0, 0, 1)
			active := -1
			if earliestActivity > 0 && earliestActivity <= dayStart.Unix() {
				dayUsers := map[string]struct{}{}
				for _, span := range activeSpans {
					if span.LastSeenUnixSec >= dayStart.Unix() && span.FirstSeenUnixSec < dayEnd.Unix() {
						dayUsers[span.UserID] = struct{}{}
					}
				}
				active = len(dayUsers)
			}
			dailyActives = append(dailyActives, email.DigestDayActives{
				DateLabel:    dayStart.Format("Mon 2006-01-02"),
				ActiveUsers:  active,
				Contributors: len(actorsByDay[dayStart.Format("2006-01-02")]),
			})
		}
	}
	for _, row := range chatRows {
		if row.CommunityID != "" {
			communityIDSet[row.CommunityID] = struct{}{}
		}
	}

	// New users' earliest ("primary") communities — resolved in the same
	// community fetch below so each new-user line can name where they
	// landed.
	primaryCommunityByUser, err := b.storage.FindEarliestCommunityForUsers(ctx, newUserIDs)
	if err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: find new-user communities: %w", err)
	}
	for _, cid := range primaryCommunityByUser {
		communityIDSet[cid] = struct{}{}
	}

	// IncludeDeleted: a community soft-deleted during the window still
	// owns activity the digest counts (its COMMUNITY_DELETED event, at
	// minimum) — without the flag it would render as an unknown row.
	communityIDs := setToSlice(communityIDSet)
	communityMap, err := storage.GetByIDs[*models.Community](b.storage, ctx, communityIDs,
		storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: fetch communities: %w", err)
	}
	memberCounts, err := b.storage.CountMembersByCommunity(ctx, communityIDs)
	if err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: count members: %w", err)
	}
	// Origin item names label ad-hoc (nameless) communities; named ones
	// are skipped inside the resolver.
	originNames := communitylib.ResolveOriginItemNames(ctx, b.storage, communityMap)

	// Build per-community ranking rows.
	type communityAgg struct {
		eventCount int
		msgCount   int
		senders    int
	}
	aggByCommunity := map[string]*communityAgg{}
	for cid, count := range perCommunityEventCount {
		aggByCommunity[cid] = &communityAgg{eventCount: count}
	}
	var totalUserMessages int
	for _, row := range chatRows {
		a := aggByCommunity[row.CommunityID]
		if a == nil {
			a = &communityAgg{}
			aggByCommunity[row.CommunityID] = a
		}
		a.msgCount = row.MessageCount
		a.senders = row.DistinctSenderCount
		totalUserMessages += row.MessageCount
	}

	// rowWithKind carries the ad-hoc origin kind alongside the row so the
	// tail rollup can group by it after sorting.
	type rowWithKind struct {
		row  email.DigestCommunity
		kind string
	}
	rows := make([]rowWithKind, 0, len(aggByCommunity))
	for cid, a := range aggByCommunity {
		row := email.DigestCommunity{
			ActionCount:         a.eventCount,
			UserMessageCount:    a.msgCount,
			DistinctSenderCount: a.senders,
			MemberCount:         memberCounts[cid],
			TopCategories:       topCategories(perCommunityCategory[cid], 3),
			ActivityScore:       a.eventCount*2 + a.msgCount,
		}
		kind := ""
		if c := communityMap[cid]; c != nil {
			row.Name = c.Name
			row.IsAdHoc = c.Name == ""
			row.Deleted = c.GetDeleted() != nil
			if row.IsAdHoc {
				kind = originKind(c)
				row.OriginLabel = originLabel(kind, originNames[cid])
			}
		} else {
			// No row even with IncludeDeleted — hard-deleted or corrupt
			// reference. Keep the fallback so activity is never dropped.
			row.Name = "Unknown community"
		}
		rows = append(rows, rowWithKind{row: row, kind: kind})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].row.ActivityScore != rows[j].row.ActivityScore {
			return rows[i].row.ActivityScore > rows[j].row.ActivityScore
		}
		if rows[i].row.Name != rows[j].row.Name {
			return rows[i].row.Name < rows[j].row.Name
		}
		return rows[i].row.OriginLabel < rows[j].row.OriginLabel
	})

	// List the top rows; roll up the tail so a burst of per-item ad-hoc
	// groups can't bury the named communities section.
	var adHocTail, namedTail *email.DigestCommunityRollup
	adHocKindCounts := map[string]int{}
	listed := rows
	if len(rows) > maxCommunityRows {
		listed = rows[:maxCommunityRows]
		for _, r := range rows[maxCommunityRows:] {
			if r.row.IsAdHoc {
				if adHocTail == nil {
					adHocTail = &email.DigestCommunityRollup{}
				}
				adHocTail.CommunityCount++
				adHocTail.ActionCount += r.row.ActionCount
				adHocTail.MessageCount += r.row.UserMessageCount
				adHocKindCounts[r.kind]++
			} else {
				if namedTail == nil {
					namedTail = &email.DigestCommunityRollup{}
				}
				namedTail.CommunityCount++
				namedTail.ActionCount += r.row.ActionCount
				namedTail.MessageCount += r.row.UserMessageCount
			}
		}
	}
	if adHocTail != nil {
		adHocTail.KindCounts = adHocKindRollup(adHocKindCounts)
	}
	digestCommunities := make([]email.DigestCommunity, 0, len(listed))
	for _, r := range listed {
		digestCommunities = append(digestCommunities, r.row)
	}

	// Engagement section.
	digestNewUsers := make([]email.DigestNewUser, 0, len(newUsers))
	authBreakdown := map[string]int{}
	for _, u := range newUsers {
		label := AuthMethodLabel(u.AuthMethod)
		authBreakdown[label]++
		primaryName := ""
		if cid := primaryCommunityByUser[u.Id]; cid != "" {
			primaryName = primaryCommunityLabel(communityMap[cid], originNames[cid])
		}
		digestNewUsers = append(digestNewUsers, email.DigestNewUser{
			Name:                 u.Name,
			Email:                u.Email,
			AuthMethodLabel:      label,
			PrimaryCommunityName: primaryName,
		})
	}

	digestWaitlist := make([]email.DigestWaitlistSignup, 0, len(waitlist))
	for _, w := range waitlist {
		digestWaitlist = append(digestWaitlist, email.DigestWaitlistSignup{
			Email:       w.Email,
			Name:        w.GetName(),
			IntendedUse: w.GetIntendedUse(),
		})
	}

	// Prior-window headline metrics for delta rendering: the same number
	// of calendar days immediately preceding the window (previous local
	// day for daily, previous ISO week for weekly).
	priorStart := start.AddDate(0, 0, -calendarDays(start, end))
	prior, err := b.buildPriorHeadline(ctx, priorStart, start, period, earliestActivity)
	if err != nil {
		return email.ActivityDigestInput{}, fmt.Errorf("activity_digest: prior-window metrics: %w", err)
	}

	return email.ActivityDigestInput{
		Period:                     period,
		WindowStartUnixSec:         startUnix,
		WindowEndUnixSec:           endUnix,
		TimezoneName:               loc.String(),
		GeneratedAtUnixSec:         b.now().Unix(),
		NewUsers:                   digestNewUsers,
		NewUserAuthBreakdown:       authBreakdown,
		InteractiveSignInCount:     interactiveSignIns,
		ActiveContributorCount:     len(contributorSet),
		ActiveUserCount:            len(activeUserSet),
		ActiveUserDataCoversWindow: activesCovered,
		DailyActives:               dailyActives,
		WaitlistSignups:            digestWaitlist,
		PasswordResetCount:         passwordResets,
		ActivityTotals:             totals,
		ChatByTopic: email.ChatTopicSummary{
			AboutTransfers:   chatTopics.Transfer,
			AboutRequests:    chatTopics.Request,
			AboutExperiences: chatTopics.Experience,
			AboutGear:        chatTopics.Gear,
			CommunityWide:    chatTopics.Community,
		},
		Communities:            digestCommunities,
		AdHocTailRollup:        adHocTail,
		NamedTailRollup:        namedTail,
		TotalMemberActionCount: countedEvents,
		TotalUserMessageCount:  totalUserMessages,
		UndoneEventCount:       undoneEvents,
		InternalEventCount:     internalEvents,
		Prior:                  prior,
	}, nil
}

// buildPriorHeadline aggregates the slim headline metrics for the prior
// window — enough for delta rendering, with no per-community or
// per-user detail. earliestActivity is reused from the main pass so
// coverage of the stamp table costs no extra query.
func (b *Builder) buildPriorHeadline(
	ctx context.Context, start, end time.Time, period email.DigestPeriod, earliestActivity int64,
) (*email.DigestPriorPeriod, error) {
	startUnix := start.Unix()
	endUnix := end.Unix()

	var (
		events    []*models.CommunityEvent
		chatRows  []storage.ChatActivityRow
		senderIDs []string
		newUsers  int
		spans     []storage.UserActiveSpan
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		events, err = b.storage.FindEventsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		chatRows, err = b.storage.CountUserMessagesByCommunity(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		senderIDs, err = b.storage.FindDistinctChatSenderIDsInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		newUsers, err = b.storage.CountUsersCreatedInWindow(gctx, startUnix, endUnix)
		return err
	})
	g.Go(func() error {
		var err error
		spans, err = b.storage.FindUserActiveSpansInWindow(gctx, startUnix, endUnix)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	prior := &email.DigestPriorPeriod{
		WindowLabel:  formatPriorWindowLabel(period, start, end),
		NewUserCount: newUsers,
	}
	contributorSet := map[string]struct{}{}
	for _, ev := range events {
		entry := Registry[ev.EventType]
		switch entry.Disposition {
		case DispositionExcludedRetraction, DispositionExcludedInternal:
			continue
		}
		prior.TotalMemberActionCount++
		if ev.ActorId != "" {
			contributorSet[ev.ActorId] = struct{}{}
		}
		if entry.Apply != nil {
			entry.Apply(ev, &prior.ActivityTotals)
		}
	}
	for _, id := range senderIDs {
		contributorSet[id] = struct{}{}
	}
	prior.ActiveContributorCount = len(contributorSet)
	for _, row := range chatRows {
		prior.TotalUserMessageCount += row.MessageCount
	}
	prior.ActiveUserDataCoversWindow = earliestActivity > 0 && earliestActivity <= startUnix
	activeSet := map[string]struct{}{}
	for _, span := range spans {
		activeSet[span.UserID] = struct{}{}
	}
	prior.ActiveUserCount = len(activeSet)
	return prior, nil
}

// formatPriorWindowLabel names the prior window for the delta footnote.
func formatPriorWindowLabel(period email.DigestPeriod, start, end time.Time) string {
	if period == email.DigestPeriodWeekly {
		return "week of " + start.Format("2006-01-02")
	}
	endInclusive := end.Add(-time.Nanosecond)
	if start.Format("2006-01-02") == endInclusive.Format("2006-01-02") {
		return start.Format("2006-01-02")
	}
	return start.Format("2006-01-02") + " through " + endInclusive.Format("2006-01-02")
}

// calendarDays counts the local calendar days between start and end —
// wall-clock aware, so a DST-crossing week still counts 7.
func calendarDays(start, end time.Time) int {
	days := 0
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		days++
	}
	return days
}

// maxCommunityRows caps the individually-listed community rows; the
// remainder folds into the ad-hoc / named tail rollups.
const maxCommunityRows = 8

func setToSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// topCategories returns the up-to-limit largest reporting categories
// for one community, largest first (ties broken by category order so
// output is deterministic).
func topCategories(counts map[Category]int, limit int) []email.DigestCategoryCount {
	if len(counts) == 0 {
		return nil
	}
	type kv struct {
		cat Category
		n   int
	}
	sorted := make([]kv, 0, len(counts))
	for c, n := range counts {
		sorted = append(sorted, kv{cat: c, n: n})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].n != sorted[j].n {
			return sorted[i].n > sorted[j].n
		}
		return sorted[i].cat < sorted[j].cat
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	out := make([]email.DigestCategoryCount, 0, len(sorted))
	for _, e := range sorted {
		out = append(out, email.DigestCategoryCount{Label: e.cat.Label(e.n), Count: e.n})
	}
	return out
}

// originKind names the kind of item an ad-hoc community formed around,
// for both row labels and the tail rollup. Empty when the community has
// no origin_item (e.g. created via the direct flow, then never named).
func originKind(c *models.Community) string {
	switch {
	case c.GetOriginExperienceId() != "":
		return "event"
	case c.GetOriginGearId() != "":
		return "gear"
	case c.GetOriginRequestId() != "":
		return "request"
	case c.GetOriginTransferId() != "":
		return "shared gear"
	default:
		return ""
	}
}

// originLabel composes the display label for what an ad-hoc community
// formed around: `event "Board-game night"`, or just the kind when the
// item's name couldn't be resolved. Empty when there is no origin item.
func originLabel(kind, itemName string) string {
	if kind == "" {
		return ""
	}
	if itemName == "" {
		return kind
	}
	return fmt.Sprintf("%s %q", kind, itemName)
}

// adHocKindRollup converts the tail's per-kind tallies into ordered
// label/count pairs ("around events", "unknown origin"), largest first.
func adHocKindRollup(kindCounts map[string]int) []email.DigestCategoryCount {
	type kv struct {
		kind string
		n    int
	}
	sorted := make([]kv, 0, len(kindCounts))
	for k, n := range kindCounts {
		sorted = append(sorted, kv{kind: k, n: n})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].n != sorted[j].n {
			return sorted[i].n > sorted[j].n
		}
		return sorted[i].kind < sorted[j].kind
	})
	out := make([]email.DigestCategoryCount, 0, len(sorted))
	for _, e := range sorted {
		label := "around " + e.kind + "s"
		switch e.kind {
		case "":
			label = "unknown origin"
		case "gear", "shared gear":
			label = "around " + e.kind
		}
		out = append(out, email.DigestCategoryCount{Label: label, Count: e.n})
	}
	return out
}

// primaryCommunityLabel names a new user's earliest community for the
// engagement section: the community name, or a short ad-hoc description
// when it is nameless.
func primaryCommunityLabel(c *models.Community, originItemName string) string {
	if c == nil {
		return ""
	}
	if c.Name != "" {
		return c.Name
	}
	if label := originLabel(originKind(c), originItemName); label != "" {
		return "ad hoc group around " + label
	}
	return "ad hoc group"
}
