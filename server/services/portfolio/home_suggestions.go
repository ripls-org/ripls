package portfolio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Open-day suggestion tuning.
const (
	// maxOpenDaySuggestions caps how many open days get a suggestion — the
	// nearest few, so the engine stays a gentle prompt rather than filling every
	// blank square.
	maxOpenDaySuggestions = 8
	// maxSuggestionAlternatives is how many "or" chips accompany the lead.
	maxSuggestionAlternatives = 2
	// suggestionCrewSize is how many regulars to name/face per suggestion.
	suggestionCrewSize = 3
	// defaultStartMins is the recommended start time-of-day (minutes past local
	// midnight) when the viewer has no scheduled-time history for an activity.
	defaultStartMins = 9 * 60 // 9:00 AM
	// minActivityOccurrences is how many similar past events an activity needs
	// before it can seed an open-day suggestion. Below this the engine stays
	// quiet and the client falls back to its generic empty-day nudge, so a
	// single one-off event never produces a confident "you've done this" prompt
	// (#2674). "Similar" is decided by embedding clustering (see fetchAll), so
	// near-but-not-identical titles still accrue toward the threshold.
	minActivityOccurrences = 2
	// activityClusterMinSimilarity is the cosine-similarity floor for treating
	// two past experiences as the same recurring activity when grouping for
	// suggestions (#2674). Deliberately much stricter than search's
	// storage.SemanticSearchMinSimilarity (0.5): search optimizes recall, but a
	// wrong merge here produces a misleading "you've done this N× before" prompt,
	// and suggestions must be good, so grouping optimizes precision. Calibrated
	// on real ripls-minilm-v1 embeddings of experience *names* over a labeled
	// dataset (clustering_strategy_test.go): at 0.83 the pairwise decision hits
	// ~100% precision (zero false merges) with ~46% recall — we'd rather miss a
	// merge (fewer suggestions) than make a wrong one. Passing 1.0 would recover
	// exact-name-only grouping.
	activityClusterMinSimilarity = 0.83
)

// activityStat is one thing the viewer has done before, with how often, the
// people who tend to come, and the time of day it usually happens.
type activityStat struct {
	label string
	count int
	crew  map[string]int // co-attendee user_id → times together
	// startMinsSum / startMinsCount accumulate the local time-of-day (minutes
	// past midnight) of past occurrences, for a recommended start time.
	startMinsSum   int
	startMinsCount int
}

// recommendedMins is the activity's typical start time-of-day (minutes past
// local midnight), or a sensible default when there's no history.
func (a activityStat) recommendedMins() int {
	if a.startMinsCount == 0 {
		return defaultStartMins
	}
	return a.startMinsSum / a.startMinsCount
}

// assembleOpenDaySuggestions proposes something to do on the calendar's open
// days, built from the viewer's past activities (which experiences they've done,
// and with whom) fitted to each day's weather. This is a heuristic v1: it keys
// "activity" on the experience name and "crew" on recurring co-attendees, rather
// than a learned model. Beyond-horizon ("typical") days are flagged low
// confidence. Returns nil when the viewer has no history to draw on — the client
// then falls back to its generic empty-day nudge. Pure function over pre-fetched
// data.
func assembleOpenDaySuggestions(
	d *fetchedData,
	userID string,
	calendar []*api.HomeUpNextEntry,
	forecast []*api.DayForecast,
	tz *time.Location,
	now time.Time,
) []*api.OpenDaySuggestion {
	if len(forecast) == 0 {
		// Without weather there is no "fit"; suggestions stay off until a
		// forecast is wired for the viewer.
		return nil
	}
	activities := eligibleActivities(viewerActivities(d, userID, tz))
	if len(activities) == 0 {
		return nil
	}

	busy := busyDays(calendar, tz)
	today := truncDayLocal(now, tz).Unix()

	out := make([]*api.OpenDaySuggestion, 0, maxOpenDaySuggestions)
	dayOrdinal := 0
	for _, fc := range forecast {
		if len(out) >= maxOpenDaySuggestions {
			break
		}
		day := fc.GetDateUnixSec()
		if day < today || busy[day] {
			continue
		}
		// Rotate the lead activity by open-day ordinal so consecutive open days
		// don't all propose the identical thing.
		lead := activities[dayOrdinal%len(activities)]
		dayOrdinal++

		out = append(out, &api.OpenDaySuggestion{
			DateUnixSec:             day,
			Title:                   capitalizeFirst(lead.label) + "?",
			Reason:                  suggestionReason(lead, fc),
			People:                  crewPeople(d, lead.crew),
			PeopleReason:            crewReason(d, lead.crew),
			Alternatives:            alternativesExcept(activities, lead.label),
			IsLowConfidence:         fc.GetIsTypical(),
			RecommendedStartUnixSec: day + int64(lead.recommendedMins()*60),
			RecommendedStartMinutes: int32(lead.recommendedMins()),
		})
	}
	return out
}

// viewerActivities ranks the things the viewer has done, most frequent first,
// each carrying the co-attendees they did it with and the time of day it tends
// to happen (in tz). When the fetch layer supplied semantic activity groups
// (d.expActivityGroups), near-but-not-identical events (e.g. "Flatirons hike" +
// "Sunday hike") are counted as one activity; otherwise it falls back to
// grouping by exact experience name.
func viewerActivities(d *fetchedData, userID string, tz *time.Location) []activityStat {
	if len(d.expActivityGroups) > 0 {
		return activitiesFromGroups(d, userID, tz)
	}
	return activitiesByExactName(d, userID, tz)
}

// activitiesFromGroups builds one activityStat per semantic group, counting
// only the experiences in each group the viewer was actually part of. The
// group's human-facing label is its most-frequent (earliest as tiebreak) exact
// name.
func activitiesFromGroups(d *fetchedData, userID string, tz *time.Location) []activityStat {
	stats := make([]activityStat, 0, len(d.expActivityGroups))
	for _, group := range d.expActivityGroups {
		stat := activityStat{crew: map[string]int{}}
		nameCounts := map[string]int{}
		for _, expID := range group {
			exp, attendees, ok := involvedExperience(d, expID, userID)
			if !ok {
				continue
			}
			label := strings.TrimSpace(exp.GetName())
			if label == "" {
				continue
			}
			nameCounts[label]++
			accumulateActivity(&stat, exp, attendees, userID, tz)
		}
		if stat.count == 0 {
			continue
		}
		stat.label = representativeLabel(nameCounts)
		stats = append(stats, stat)
	}
	sortActivities(stats)
	return stats
}

// activitiesByExactName is the fallback keying used when no embedding groups are
// available: one activityStat per distinct trimmed experience name.
func activitiesByExactName(d *fetchedData, userID string, tz *time.Location) []activityStat {
	byLabel := map[string]*activityStat{}
	for expID := range d.expMap {
		exp, attendees, ok := involvedExperience(d, expID, userID)
		if !ok {
			continue
		}
		label := strings.TrimSpace(exp.GetName())
		if label == "" {
			continue
		}
		stat := byLabel[label]
		if stat == nil {
			stat = &activityStat{label: label, crew: map[string]int{}}
			byLabel[label] = stat
		}
		accumulateActivity(stat, exp, attendees, userID, tz)
	}

	stats := make([]activityStat, 0, len(byLabel))
	for _, s := range byLabel {
		stats = append(stats, *s)
	}
	sortActivities(stats)
	return stats
}

// involvedExperience returns the Experience for expID and its attendees, plus
// whether the viewer was part of it (owner or a YES/MAYBE attendee). Experiences
// the viewer wasn't part of don't count toward their activity history.
func involvedExperience(d *fetchedData, expID, userID string) (*models.Experience, []string, bool) {
	msg, ok := d.expMap[expID]
	if !ok {
		return nil, nil, false
	}
	exp, ok := msg.(*models.Experience)
	if !ok {
		return nil, nil, false
	}
	attendees := d.expAttendeesMap[expID]
	return exp, attendees, isViewerInvolved(exp, attendees, userID)
}

// isViewerInvolved reports whether the viewer took part in an experience — its
// owner, or a YES/MAYBE attendee. Shared by fetchAll (to pick which experiences
// to cluster) and involvedExperience (to build activity stats).
func isViewerInvolved(exp *models.Experience, attendees []string, userID string) bool {
	if exp.GetOwnerId() == userID {
		return true
	}
	for _, a := range attendees {
		if a == userID {
			return true
		}
	}
	return false
}

// accumulateActivity folds one experience into a stat: bumps the count, adds its
// local time-of-day toward the recommended start time, and tallies co-attendees.
func accumulateActivity(stat *activityStat, exp *models.Experience, attendees []string, userID string, tz *time.Location) {
	stat.count++
	if st := experienceScheduledTime(exp); st > 0 {
		lt := time.Unix(st, 0).In(tz)
		stat.startMinsSum += lt.Hour()*60 + lt.Minute()
		stat.startMinsCount++
	}
	for _, a := range attendees {
		if a != userID {
			stat.crew[a]++
		}
	}
}

// representativeLabel picks the most-frequent name in a group (earliest
// alphabetically as a deterministic tiebreak) as the group's display label.
func representativeLabel(nameCounts map[string]int) string {
	best := ""
	bestCount := -1
	for name, c := range nameCounts {
		if c > bestCount || (c == bestCount && name < best) {
			best, bestCount = name, c
		}
	}
	return best
}

// sortActivities orders stats most-done first, with a stable label tiebreak so
// output is deterministic.
func sortActivities(stats []activityStat) {
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].count != stats[j].count {
			return stats[i].count > stats[j].count
		}
		return stats[i].label < stats[j].label
	})
}

// eligibleActivities keeps only activities the viewer has done at least
// minActivityOccurrences times — the #2674 suppression rule. Similar events are
// already folded into one activity upstream, so the count reflects semantic
// repetition, not identical-title repetition.
func eligibleActivities(activities []activityStat) []activityStat {
	out := make([]activityStat, 0, len(activities))
	for _, a := range activities {
		if a.count >= minActivityOccurrences {
			out = append(out, a)
		}
	}
	return out
}

// busyDays returns the set of local-day Unix seconds that already have a planned
// calendar entry, so suggestions only land on genuinely open days.
func busyDays(calendar []*api.HomeUpNextEntry, tz *time.Location) map[int64]bool {
	busy := make(map[int64]bool, len(calendar))
	for _, e := range calendar {
		day := truncDayLocal(time.Unix(e.GetTimeUnixSec(), 0), tz).Unix()
		busy[day] = true
	}
	return busy
}

// suggestionReason is the editorial "why" line, weaving the day's weather into
// the history clause so the suggestion reads as one thought — the client shows
// no separate weather text on the card. E.g. "Clear and 72° — you've done
// flatirons hike 3× before, mostly on days like this." A climate-normal day
// (is_typical) softens the weather clause to "usually … this time of year";
// a missing summary falls back to the history clause alone.
func suggestionReason(a activityStat, fc *api.DayForecast) string {
	history := fmt.Sprintf("you've done %s %d× before, mostly on days like this", lowerFirst(a.label), a.count)
	weather := fc.GetSummary()
	if weather == "" {
		return capitalizeFirst(history) + "."
	}
	if fc.GetIsTypical() {
		return fmt.Sprintf("Usually %s this time of year — %s.", lowerFirst(weather), history)
	}
	return fmt.Sprintf("%s — %s.", capitalizeFirst(weather), history)
}

// crewPeople returns up to suggestionCrewSize regulars as DailyPersons, most
// frequent companions first. Only people in userMap (whom the viewer can see)
// are included.
func crewPeople(d *fetchedData, crew map[string]int) []*api.DailyPerson {
	type pair struct {
		id    string
		count int
	}
	pairs := make([]pair, 0, len(crew))
	for id, c := range crew {
		pairs = append(pairs, pair{id, c})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].id < pairs[j].id
	})
	people := make([]*api.DailyPerson, 0, suggestionCrewSize)
	for _, p := range pairs {
		if len(people) >= suggestionCrewSize {
			break
		}
		if u, ok := d.userMap[p.id]; ok {
			people = append(people, dailyPersonFor(u))
		}
	}
	return people
}

func crewReason(d *fetchedData, crew map[string]int) string {
	people := crewPeople(d, crew)
	if len(people) == 0 {
		return ""
	}
	names := make([]string, 0, len(people))
	for _, p := range people {
		if p.DisplayName != "" {
			names = append(names, firstName(p.DisplayName))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return joinNames(names) + " usually come along"
}

// alternativesExcept returns up to maxSuggestionAlternatives activities other
// than the lead, best-fit (most-done) first.
func alternativesExcept(activities []activityStat, leadLabel string) []*api.OpenDaySuggestionAlternative {
	alts := make([]*api.OpenDaySuggestionAlternative, 0, maxSuggestionAlternatives)
	for _, a := range activities {
		if len(alts) >= maxSuggestionAlternatives {
			break
		}
		if a.label == leadLabel {
			continue
		}
		alts = append(alts, &api.OpenDaySuggestionAlternative{
			Id:    a.label,
			Label: a.label,
			Glyph: "", // the client maps a glyph if it has one
		})
	}
	return alts
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// joinNames renders ["Mia"] → "Mia", ["Mia","Ben"] → "Mia & Ben",
// ["Mia","Ben","Sam"] → "Mia, Ben & Sam".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " & " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " & " + names[len(names)-1]
	}
}
