package portfolio

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/storage"
)

// This file is an exploration harness (#2674), not a regression guard. It
// compares candidate strategies for deciding whether two experiences are the
// "same" recurring activity, so we pick the most discriminative one before
// leaning on it for suggestions. Run it manually:
//
//	go test ./services/portfolio/ -run TestClusteringStrategyComparison -v
//
// It requires the local ONNX model (skips otherwise). Output is a table of
// AUC + recall-at-high-precision per strategy; it asserts only that the best
// strategy clears a sane bar so a model regression is still noticed.

// activitySample is one labeled experience: a kind (ground truth), plus the
// name and description a user might actually write — deliberately sprinkled with
// filler (days, times, "with friends", "get-together") and with some kinds kept
// intentionally close (running vs cycling, hiking vs camping vs picnic) to test
// how picky each strategy can be.
type activitySample struct {
	kind string
	name string
	desc string
}

var activityDataset = []activitySample{
	// hiking
	{"hiking", "Flatirons hike", "Morning hike up the first Flatiron with a few folks."},
	{"hiking", "Sunday morning hike", "Easy loop trail, met at the trailhead at 8am."},
	{"hiking", "Hike up Green Mountain", "Steep out-and-back, brought trekking poles."},
	{"hiking", "Quick trail hike after work", "Short evening hike to catch the sunset."},
	{"hiking", "Chautauqua hike with friends", "Wandered the meadow trails, lots of photos."},
	{"hiking", "Saturday summit hike", "Long day hike to the ridge, packed lunch."},

	// running
	{"running", "Trail run", "5 miles on the dirt path along the creek."},
	{"running", "Morning 5k run", "Easy pace loop around the neighborhood before work."},
	{"running", "Evening jog with the crew", "Casual social run, stopped for water halfway."},
	{"running", "Long run Sunday", "Half-marathon training, 10 miles."},
	{"running", "Track intervals", "Speed work, 8x400m at the high school track."},
	{"running", "Quick lunch run", "Three quick miles to clear my head."},

	// cycling (intentionally near running/hiking to test pickiness)
	{"cycling", "Bike ride along the creek path", "Flat cruise on the paved trail, easy gears."},
	{"cycling", "Sunday cycling loop", "Road ride out to the reservoir and back."},
	{"cycling", "Mountain biking trip", "Rocky singletrack, muddy after the rain."},
	{"cycling", "Evening bike ride with friends", "Slow social spin around town at dusk."},
	{"cycling", "Gravel ride", "Long gravel route through the farm roads."},
	{"cycling", "Quick commute ride", "Rode the bike to the office and back."},

	// dinner / potluck
	{"dinner", "Taco Tuesday dinner", "Homemade tacos and margaritas at my place."},
	{"dinner", "Neighborhood potluck", "Everyone brought a dish, big shared table."},
	{"dinner", "Homemade pasta night", "Fresh pasta and red sauce, wine on the porch."},
	{"dinner", "Pizza night with friends", "Made dough from scratch, topped our own pies."},
	{"dinner", "Sunday roast dinner", "Slow-cooked roast, the whole crew came over."},
	{"dinner", "Backyard barbecue", "Grilled burgers and corn, ate outside."},

	// board games
	{"boardgames", "Board game night", "Played Catan and Ticket to Ride until late."},
	{"boardgames", "Weekly game night", "Our usual crew, cards and a couple of board games."},
	{"boardgames", "Board games with friends", "Learned a new co-op game, lots of laughs."},
	{"boardgames", "Catan evening", "Three rounds of Settlers, close finish."},
	{"boardgames", "Game night get-together", "Party games and snacks, everyone joined."},
	{"boardgames", "Sunday board games", "Quiet afternoon of strategy games and tea."},

	// yoga
	{"yoga", "Morning yoga session", "Gentle vinyasa flow in the living room."},
	{"yoga", "Yoga in the park", "Outdoor class on the grass, sunny and warm."},
	{"yoga", "Evening restorative yoga", "Slow stretches and breathing to wind down."},
	{"yoga", "Sunday yoga with friends", "Followed an online class together."},
	{"yoga", "Hot yoga class", "Sweaty 60-minute heated flow."},
	{"yoga", "Quick lunchtime yoga", "Twenty minutes of stretching at midday."},

	// swimming
	{"swimming", "Lap swim at the pool", "Forty laps freestyle, easy pace."},
	{"swimming", "Morning swim", "Cold water, quick dip before work."},
	{"swimming", "Lake swim with friends", "Open-water swim across the cove."},
	{"swimming", "Pool afternoon", "Splashing around and a few laps."},
	{"swimming", "Evening swim workout", "Intervals in the pool, kickboard sets."},
	{"swimming", "Weekend swim", "Long relaxed swim at the rec center."},

	// book club (text-heavy, tests desc noise)
	{"bookclub", "Book club meeting", "Discussed the new novel over coffee."},
	{"bookclub", "Monthly book club", "Everyone finished the book this time, good chat."},
	{"bookclub", "Book club with wine", "Talked plot twists late into the evening."},
	{"bookclub", "Reading group get-together", "Short story collection, split opinions."},
	{"bookclub", "Sunday book club", "Cozy afternoon discussing the memoir."},
	{"bookclub", "Book club at the cafe", "Met downtown to review the mystery pick."},

	// camping (near hiking/picnic to test pickiness)
	{"camping", "Weekend camping trip", "Two nights at the state park, campfire and tents."},
	{"camping", "Backcountry camping", "Hiked in and camped by the alpine lake."},
	{"camping", "Car camping with friends", "Drove up, set up camp, roasted marshmallows."},
	{"camping", "Overnight camp", "One night under the stars, cooked over the fire."},
	{"camping", "Family camping weekend", "Big group site, kids and s'mores."},
	{"camping", "Lakeside camping", "Pitched tents by the water, morning coffee outside."},

	// movie night (near board games / dinner as "night at home")
	{"movienight", "Movie night", "Watched a classic with popcorn on the couch."},
	{"movienight", "Friday movie night", "Double feature, everyone brought snacks."},
	{"movienight", "Backyard movie night", "Projector on the fence, blankets out."},
	{"movienight", "Movie marathon with friends", "Three films back to back, lots of snacks."},
	{"movienight", "Sunday movie night", "Cozy film and takeout at home."},
	{"movienight", "Documentary night", "Watched a nature doc and chatted after."},
}

// fillerWords are cross-kind noise: days, times, generic social/eventy words,
// articles, and qualifiers. Stripping them should sharpen kind separation if the
// hypothesis holds.
var fillerWords = map[string]bool{}

//nolint:gochecknoinits // test-only setup for the clustering strategy table.
func init() {
	for _, w := range strings.Fields(
		"a an the of to for with and or at in on my our your their his her us we " +
			"monday tuesday wednesday thursday friday saturday sunday " +
			"morning afternoon evening night noon midday tonight today tomorrow " +
			"weekly monthly daily annual weekend weeknight " +
			"friends folks everyone crew group gang buddies neighbors family " +
			"event outing session meetup meet hangout hang gettogether together get " +
			"quick little big small fun casual first second new usual quiet cozy " +
			"class time day",
	) {
		fillerWords[w] = true
	}
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// stripFiller lowercases, splits on non-alphanumerics, drops filler words, and
// rejoins. "Sunday morning hike with friends" -> "hike".
func stripFiller(text string) string {
	toks := nonWord.Split(strings.ToLower(text), -1)
	kept := make([]string, 0, len(toks))
	for _, t := range toks {
		if t == "" || fillerWords[t] {
			continue
		}
		kept = append(kept, t)
	}
	return strings.Join(kept, " ")
}

func TestClusteringStrategyComparison(t *testing.T) {
	embedder := storage.SetupTestEmbedder(t)
	if embedder == nil {
		return
	}
	ctx := context.Background()

	text := func(s activitySample, useDesc, strip bool) string {
		txt := s.name
		if useDesc {
			txt = s.name + " " + s.desc
		}
		if strip {
			txt = stripFiller(txt)
		}
		return txt
	}

	type strategy struct {
		name    string
		useDesc bool
		strip   bool
	}
	strategies := []strategy{
		{"name-only, raw", false, false},
		{"name-only, filler-stripped", false, true},
		{"name+desc, raw", true, false},
		{"name+desc, filler-stripped", true, true},
	}

	t.Logf("dataset: %d samples across kinds", len(activityDataset))
	t.Logf("%-32s  %6s  %10s  %8s", "strategy", "AUC", "rec@P>=.98", "thresh")

	var bestAUC float64
	var winnerSame, winnerCross []float64 // name-only, raw — for the PR curve below
	for _, st := range strategies {
		vecs := make([][]float32, len(activityDataset))
		for i, s := range activityDataset {
			v, err := embedder.Generate(ctx, text(s, st.useDesc, st.strip))
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			vecs[i] = v
		}

		var same, cross []float64
		for i := 0; i < len(activityDataset); i++ {
			for j := i + 1; j < len(activityDataset); j++ {
				sim := float64(embedding.CosineSimilarity(vecs[i], vecs[j]))
				if activityDataset[i].kind == activityDataset[j].kind {
					same = append(same, sim)
				} else {
					cross = append(cross, sim)
				}
			}
		}

		auc := rocAUC(same, cross)
		rec, thr := recallAtPrecision(same, cross, 0.98)
		if auc > bestAUC {
			bestAUC = auc
		}
		if !st.useDesc && !st.strip {
			winnerSame, winnerCross = same, cross
		}
		t.Logf("%-32s  %.4f  %9.1f%%  %8.3f", st.name, auc, rec*100, thr)
	}

	// Precision/recall operating points for the winning strategy (name-only,
	// raw), so the production threshold is chosen deliberately for a picky
	// feature where a wrong merge is worse than a missed one.
	t.Logf("--- name-only,raw precision/recall (pick the threshold here) ---")
	t.Logf("%8s  %8s  %8s", "P>=", "recall", "thresh")
	for _, target := range []float64{0.95, 0.98, 0.99, 1.00} {
		rec, thr := recallAtPrecision(winnerSame, winnerCross, target)
		t.Logf("%7.0f%%  %7.1f%%  %8.3f", target*100, rec*100, thr)
	}

	// Sanity floor: the best strategy should be strongly discriminative, else the
	// model or dataset regressed.
	if bestAUC < 0.90 {
		t.Errorf("best AUC %.4f < 0.90 — clustering signal too weak to trust", bestAUC)
	}
}

// rocAUC = P(same-kind similarity > cross-kind similarity), ties counted as 0.5.
func rocAUC(same, cross []float64) float64 {
	if len(same) == 0 || len(cross) == 0 {
		return 0
	}
	sorted := append([]float64(nil), cross...)
	sort.Float64s(sorted)
	var wins float64
	for _, s := range same {
		lo := sort.SearchFloat64s(sorted, s)    // # cross strictly < s
		hi := upperBound(sorted, s)             // # cross <= s
		ties := hi - lo                         // # cross == s
		wins += float64(lo) + 0.5*float64(ties) // < counts 1, == counts 0.5
	}
	return wins / float64(len(same)*len(cross))
}

func upperBound(sorted []float64, v float64) int {
	return sort.Search(len(sorted), func(i int) bool { return sorted[i] > v })
}

// recallAtPrecision finds the lowest threshold whose precision >= target and
// returns the recall achieved there (the pickiest useful operating point). If no
// threshold reaches the target precision, returns (0, 1.0).
func recallAtPrecision(same, cross []float64, target float64) (recall, threshold float64) {
	cand := append(append([]float64(nil), same...), cross...)
	sort.Float64s(cand)
	bestRecall, bestThresh := 0.0, 1.0
	found := false
	for _, thr := range cand {
		tp, fp := 0, 0
		for _, s := range same {
			if s >= thr {
				tp++
			}
		}
		for _, c := range cross {
			if c >= thr {
				fp++
			}
		}
		if tp+fp == 0 {
			continue
		}
		prec := float64(tp) / float64(tp+fp)
		if prec >= target {
			r := float64(tp) / float64(len(same))
			if r > bestRecall {
				bestRecall, bestThresh, found = r, thr, true
			}
		}
	}
	if !found {
		return 0, 1.0
	}
	return bestRecall, bestThresh
}
