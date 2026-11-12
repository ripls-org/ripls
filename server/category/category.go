package category

import (
	"strings"
)

// Categorize returns a coarse category label derived from [title]
// and [description] via the keyword dictionary defined in
// [rules]. Returns the empty string when no rule matches — callers
// should treat that as "uncategorized" and skip downstream chip
// generation.
//
// The match is case-insensitive substring over the concatenation of
// title and description. Rules are evaluated in order, so more
// specific categories appear earlier in the list and more general
// fallbacks later.
func Categorize(title, description string) string {
	haystack := strings.ToLower(title + " " + description)
	if strings.TrimSpace(haystack) == "" {
		return ""
	}
	for _, rule := range rules {
		for _, kw := range rule.keywords {
			if strings.Contains(haystack, kw) {
				return rule.category
			}
		}
	}
	return ""
}

// rule pairs a category display label with the set of lowercase
// keywords that triggers it.
type rule struct {
	category string
	keywords []string
}

// rules is the ordered keyword dictionary. Order matters: the first
// matching rule wins, so put specific buckets ahead of broader ones
// ("Hiking" before "Outdoors") to keep the chip text meaningful.
//
// Keep keywords short and lowercased — the matcher does a simple
// substring contains, not a word-boundary regex, so "drill" will
// also catch "drilling".
//
// Known limitations: the dictionary is in code, so a keyword change
// needs a server redeploy; and it is English-only — keywords are
// lowercase ASCII English, so non-English content (Spanish "comida",
// French "cuisine") returns the empty string and gets no chip.
var rules = []rule{
	{"Cooking", []string{"cook", "recipe", "meal", "kitchen", "bake", "potluck", "supper", "brunch", "dinner", "lunch", "breakfast", "chef"}},
	{"Bonfires", []string{"bonfire", "smoker", "bbq", "barbecue", "grill"}},
	{"Hiking", []string{"hike", "trail", "trek"}},
	{"Camping", []string{"camp", "tent", "campfire", "backpack"}},
	{"Biking", []string{"bike", "bicycle", "cycling"}},
	{"Climbing", []string{"climb", "bouldering"}},
	{"Paddling", []string{"kayak", "canoe", "paddle", "raft"}},
	{"Skiing", []string{"ski", "snowboard"}},
	{"Books", []string{"book", "novel", "library", "reading", "author"}},
	{"Music", []string{"music", "concert", "band", "song", "jam"}},
	{"Games", []string{"board game", "trivia", "puzzle", "tabletop"}},
	{"Fitness", []string{"workout", "yoga", "gym", "fitness", "pilates"}},
	{"Running", []string{" run ", "running", "jog"}},
	{"Pets", []string{"dog", "cat", "puppy", "pet "}},
	{"Kids", []string{"kid", "child", "playdate", "stroller"}},
	{"Home Repair", []string{"repair", "fix", "leaky", "handyman"}},
	{"Power Tools", []string{"drill", "saw", "wrench", "hammer", "power tool"}},
	{"Gardening", []string{"garden", "plant", "compost", "soil", "seedling", "harvest"}},
	{"Carpooling", []string{"carpool", "rideshare"}},
	{"Volunteering", []string{"volunteer", "service day", "clean-up"}},
	{"Photography", []string{"photo", "camera", "darkroom"}},
	{"Movies", []string{"movie", "film", "screening"}},
	{"Crafts", []string{"craft", "paint", "draw", "sew", "knit", "pottery"}},
	{"Outdoors", []string{"outdoor", "nature", "park"}},
}
