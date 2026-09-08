package portfolio

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestJoinNames(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"Mia"}, "Mia"},
		{[]string{"Mia", "Ben"}, "Mia & Ben"},
		{[]string{"Mia", "Ben", "Sam"}, "Mia, Ben & Sam"},
	}
	for _, c := range cases {
		if got := joinNames(c.in); got != c.want {
			t.Errorf("joinNames(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCaseHelpers(t *testing.T) {
	if capitalizeFirst("hike") != "Hike" {
		t.Error("capitalizeFirst")
	}
	if lowerFirst("Hike") != "hike" {
		t.Error("lowerFirst")
	}
}

func TestViewerActivitiesRanksAndCountsCrew(t *testing.T) {
	const me = "u-me"
	d := &fetchedData{
		expMap: map[string]proto.Message{
			"e1": &models.Experience{Id: "e1", OwnerId: me, Name: "Flatirons hike"},
			"e2": &models.Experience{Id: "e2", OwnerId: "u-other", Name: "Flatirons hike"},
			"e3": &models.Experience{Id: "e3", OwnerId: me, Name: "Lookout picnic"},
			"e4": &models.Experience{Id: "e4", OwnerId: "u-other", Name: "Not mine"},
		},
		expAttendeesMap: map[string][]string{
			"e1": {me, "u-mia", "u-ben"},
			"e2": {me, "u-mia"},
			"e3": {me, "u-mia"},
			"e4": {"u-other"}, // viewer not involved
		},
		userMap: map[string]*api.User{},
	}
	acts := viewerActivities(d, me, time.UTC)
	if len(acts) != 2 {
		t.Fatalf("got %d activities, want 2 (excludes the one the viewer wasn't in)", len(acts))
	}
	if acts[0].label != "Flatirons hike" || acts[0].count != 2 {
		t.Errorf("top activity = %q×%d, want Flatirons hike×2", acts[0].label, acts[0].count)
	}
	// Mia attended both hikes with the viewer.
	if acts[0].crew["u-mia"] != 2 {
		t.Errorf("crew[u-mia] = %d, want 2", acts[0].crew["u-mia"])
	}
}

func TestAssembleOpenDaySuggestionsSkipsBusyAndPast(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, tz)
	today := truncDayLocal(now, tz)

	// Two "Trail run" experiences so the activity clears the ≥2 threshold
	// (#2674) via exact-name fallback (no embedding groups set here).
	d := &fetchedData{
		expMap: map[string]proto.Message{
			"e1": &models.Experience{Id: "e1", OwnerId: "u-me", Name: "Trail run"},
			"e2": &models.Experience{Id: "e2", OwnerId: "u-me", Name: "Trail run"},
		},
		expAttendeesMap: map[string][]string{
			"e1": {"u-me", "u-sam"},
			"e2": {"u-me", "u-sam"},
		},
		userMap: map[string]*api.User{
			"u-sam": {Id: "u-sam", Name: "Sam Rivera"},
		},
	}

	day0 := today                        // open
	day1 := today.AddDate(0, 0, 1)       // busy
	dayTypical := today.AddDate(0, 0, 2) // open, typical
	forecast := []*api.DayForecast{
		{DateUnixSec: day0.Unix(), Summary: "Clear and 72°"},
		{DateUnixSec: day1.Unix(), Summary: "Mild and 64°"},
		{DateUnixSec: dayTypical.Unix(), Summary: "Mild and 64°", IsTypical: true},
	}
	calendar := []*api.HomeUpNextEntry{
		{TimeUnixSec: day1.Add(3 * time.Hour).Unix()}, // makes day1 busy
	}

	got := assembleOpenDaySuggestions(d, "u-me", calendar, forecast, tz, now)
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want 2 (day0 + dayTypical; day1 busy)", len(got))
	}
	if got[0].GetDateUnixSec() != day0.Unix() {
		t.Errorf("first suggestion day = %d, want %d", got[0].GetDateUnixSec(), day0.Unix())
	}
	if got[0].GetTitle() != "Trail run?" {
		t.Errorf("title = %q, want %q", got[0].GetTitle(), "Trail run?")
	}
	// The reason weaves the day's weather into the history clause — the
	// client renders no separate weather text on the suggestion card. The
	// fixture holds two trail runs (the ≥2 suggestion threshold, #2674),
	// so the history clause counts 2×.
	wantReason := "Clear and 72° — you've done trail run 2× before, mostly on days like this."
	if got[0].GetReason() != wantReason {
		t.Errorf("reason = %q, want %q", got[0].GetReason(), wantReason)
	}
	wantTypical := "Usually mild and 64° this time of year — you've done trail run 2× before, mostly on days like this."
	if got[1].GetReason() != wantTypical {
		t.Errorf("typical-day reason = %q, want %q", got[1].GetReason(), wantTypical)
	}
	if len(got[0].GetPeople()) != 1 || got[0].GetPeople()[0].GetUserId() != "u-sam" {
		t.Errorf("people = %v, want [u-sam]", got[0].GetPeople())
	}
	// The typical day's suggestion is flagged low confidence.
	if !got[1].GetIsLowConfidence() {
		t.Error("typical-day suggestion should be low confidence")
	}
}

func TestSuggestionReason(t *testing.T) {
	lead := activityStat{label: "Flatirons hike", count: 3}
	cases := []struct {
		name string
		fc   *api.DayForecast
		want string
	}{
		{
			name: "real forecast leads with the weather",
			fc:   &api.DayForecast{Summary: "Clear and 72°"},
			want: "Clear and 72° — you've done flatirons hike 3× before, mostly on days like this.",
		},
		{
			name: "climate normal softens to usually-this-time-of-year",
			fc:   &api.DayForecast{Summary: "Showers on and off, 63°", IsTypical: true},
			want: "Usually showers on and off, 63° this time of year — you've done flatirons hike 3× before, mostly on days like this.",
		},
		{
			name: "missing summary falls back to history alone",
			fc:   &api.DayForecast{},
			want: "You've done flatirons hike 3× before, mostly on days like this.",
		},
	}
	for _, c := range cases {
		if got := suggestionReason(lead, c.fc); got != c.want {
			t.Errorf("%s: suggestionReason = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAssembleOpenDaySuggestionsNoHistoryOrWeather(t *testing.T) {
	tz := time.UTC
	now := time.Now()
	empty := &fetchedData{expMap: map[string]proto.Message{}, userMap: map[string]*api.User{}}
	if got := assembleOpenDaySuggestions(empty, "u", nil, nil, tz, now); got != nil {
		t.Error("no weather → nil suggestions")
	}
	fc := []*api.DayForecast{{DateUnixSec: truncDayLocal(now, tz).Unix(), Summary: "Clear"}}
	if got := assembleOpenDaySuggestions(empty, "u", nil, fc, tz, now); got != nil {
		t.Error("no history → nil suggestions")
	}
}

// TestAssembleOpenDaySuggestionsRequiresTwoSimilar is the core #2674 guard:
// two differently-named events count as one activity only when the embedding
// layer groups them, and a lone activity (one group per event) is suppressed.
func TestAssembleOpenDaySuggestionsRequiresTwoSimilar(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, tz)
	fc := []*api.DayForecast{{DateUnixSec: truncDayLocal(now, tz).Unix(), Summary: "Clear"}}

	base := func() *fetchedData {
		return &fetchedData{
			expMap: map[string]proto.Message{
				"e1": &models.Experience{Id: "e1", OwnerId: "u-me", Name: "Flatirons hike"},
				"e2": &models.Experience{Id: "e2", OwnerId: "u-me", Name: "Sunday hike"},
			},
			expAttendeesMap: map[string][]string{"e1": {"u-me"}, "e2": {"u-me"}},
			userMap:         map[string]*api.User{},
		}
	}

	// Each event its own group → two one-off activities → nothing suggested.
	dSingles := base()
	dSingles.expActivityGroups = [][]string{{"e1"}, {"e2"}}
	if got := assembleOpenDaySuggestions(dSingles, "u-me", nil, fc, tz, now); got != nil {
		t.Errorf("two one-off activities should suppress suggestions, got %d", len(got))
	}

	// Both events in one group → one recurring activity (count 2) → suggested.
	dGrouped := base()
	dGrouped.expActivityGroups = [][]string{{"e1", "e2"}}
	got := assembleOpenDaySuggestions(dGrouped, "u-me", nil, fc, tz, now)
	if len(got) == 0 {
		t.Fatal("two similar events should produce a suggestion")
	}
	// Representative label = most-frequent name, earliest on a tie → "Flatirons hike".
	if got[0].GetTitle() != "Flatirons hike?" {
		t.Errorf("title = %q, want %q", got[0].GetTitle(), "Flatirons hike?")
	}
	if !strings.Contains(got[0].GetReason(), "2×") {
		t.Errorf("reason should cite the 2× count, got %q", got[0].GetReason())
	}
}

// TestAssembleOpenDaySuggestionsFallbackMergesExactNames verifies the
// no-embedding fallback: identical names still merge to clear the threshold,
// and a genuine one-off stays suppressed.
func TestAssembleOpenDaySuggestionsFallbackMergesExactNames(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 6, 18, 9, 0, 0, 0, tz)
	fc := []*api.DayForecast{{DateUnixSec: truncDayLocal(now, tz).Unix(), Summary: "Clear"}}

	// No expActivityGroups → exact-name fallback path.
	d := &fetchedData{
		expMap: map[string]proto.Message{
			"e1": &models.Experience{Id: "e1", OwnerId: "u-me", Name: "Taco night"},
			"e2": &models.Experience{Id: "e2", OwnerId: "u-me", Name: "Taco night"},
			"e3": &models.Experience{Id: "e3", OwnerId: "u-me", Name: "One-off gala"},
		},
		expAttendeesMap: map[string][]string{},
		userMap:         map[string]*api.User{},
	}
	got := assembleOpenDaySuggestions(d, "u-me", nil, fc, tz, now)
	if len(got) == 0 {
		t.Fatal("two identical events should suggest via fallback")
	}
	if got[0].GetTitle() != "Taco night?" {
		t.Errorf("title = %q, want %q (the ×2 activity, not the one-off)", got[0].GetTitle(), "Taco night?")
	}
}
