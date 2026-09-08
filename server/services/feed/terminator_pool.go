package feed

// TerminatorEntry is one entry in the daily-rotating feed-terminator pool.
type TerminatorEntry struct {
	// Headline shown on the terminator card.
	Headline string
	// Description shown below the headline. Must be coherent with the
	// "Plan something" CTA — about being caught up, going outside, or
	// connecting in person. Never about sharing gear or returning favors.
	Description string
	// StockQuery used to fetch background imagery.
	StockQuery string
}

// terminatorPool is a static pool of "go outside" terminator messages.
// They rotate by day-of-year so every user sees the same message on a given day.
// Every entry pairs with a "Plan something" CTA that opens experience creation.
var terminatorPool = []TerminatorEntry{
	{
		Headline:    "You're all caught up",
		Description: "Nothing new in the feed. Close the app, go outside, call a friend, or plan something worth doing.",
		StockQuery:  "morning trail hike sunrise friends",
	},
	{
		Headline:    "Nothing left to see",
		Description: "Your community is thriving. The only thing left is to be part of it in person.",
		StockQuery:  "neighborhood friends gathering outdoors",
	},
	{
		Headline:    "You've seen it all",
		Description: "Now go make something worth remembering. The best adventures start with a plan.",
		StockQuery:  "adventure outdoor nature group",
	},
	{
		Headline:    "All quiet here",
		Description: "The best memories aren't on a screen. Get outside and make one.",
		StockQuery:  "campfire friends evening outdoors",
	},
	{
		Headline:    "That's the whole feed",
		Description: "Take everything you just saw and turn it into a plan. Your crew is ready.",
		StockQuery:  "hiking group mountain trail",
	},
	{
		Headline:    "End of the line",
		Description: "Go outside. Seriously. We'll still be here when you get back.",
		StockQuery:  "forest path sunlight trees",
	},
	{
		Headline:    "You made it",
		Description: "Now close this and go have an actual adventure. Pick somewhere new.",
		StockQuery:  "mountain summit view landscape",
	},
	{
		Headline:    "Scroll no more",
		Description: "Your friends are out there. Go find them — or give them a reason to come to you.",
		StockQuery:  "neighborhood park community friends",
	},
	{
		Headline:    "The feed is yours",
		Description: "Everything you just saw started with one person making a plan. Be that person.",
		StockQuery:  "outdoor action sports community",
	},
	{
		Headline:    "Well done",
		Description: "You read everything. Now go live it.",
		StockQuery:  "lake kayak morning light",
	},
	{
		Headline:    "All caught up",
		Description: "The only thing left to do is something worth talking about later.",
		StockQuery:  "rock climbing adventure friends",
	},
	{
		Headline:    "That's everything",
		Description: "Time to log off and log some miles.",
		StockQuery:  "running trail morning nature",
	},
	{
		Headline:    "You're current",
		Description: "Your community is waiting for you — offline. Pick a time and a place.",
		StockQuery:  "neighborhood meetup friends coffee",
	},
	{
		Headline:    "See you out there",
		Description: "The best things happening in your community aren't on this app. Go find them.",
		StockQuery:  "sunset outdoor friends silhouette",
	},
	{
		Headline:    "You're done here",
		Description: "Go somewhere new. Bring someone. Make it a story worth telling.",
		StockQuery:  "bicycle urban neighborhood street",
	},
	{
		Headline:    "Fresh out of feed",
		Description: "That's your cue. Get outside, make a plan, and give people something to look forward to.",
		StockQuery:  "outdoor morning light adventure path",
	},
	{
		Headline:    "Up to date",
		Description: "Everything's been seen. Now go do something worth posting about.",
		StockQuery:  "outdoor sport action friends",
	},
	{
		Headline:    "Feed complete",
		Description: "The next great thing in this community starts with someone making a move. Go first.",
		StockQuery:  "outdoor market community gathering",
	},
	{
		Headline:    "You're ahead of the feed",
		Description: "Nothing new yet. Close the app, get some air, and check back later.",
		StockQuery:  "open sky clouds walking path",
	},
	{
		Headline:    "That's a wrap",
		Description: "Close the app. Go outside. Call someone. Plan something. In that order.",
		StockQuery:  "friends laughing outdoor evening",
	},
}

// TerminatorsPerDay controls how many distinct terminator nudges are generated
// and persisted each day. All are pre-fetched with imagery; one is chosen
// randomly per feed request. Increase to reduce repetition within a day.
const TerminatorsPerDay = 5

var terminatorFallback = TerminatorEntry{
	Headline:    "You're all caught up",
	Description: "Nothing new in the feed. Close the app, go outside, or plan something worth doing.",
	StockQuery:  "outdoor nature hiking",
}

// GetTerminatorForSlot returns the terminator entry for a specific slot within
// a day. Using both day and slot ensures that each of the TerminatorsPerDay
// entries is distinct and that the set rotates across days.
func GetTerminatorForSlot(day, slot int) TerminatorEntry {
	if len(terminatorPool) == 0 {
		return terminatorFallback
	}
	idx := ((day-1)*TerminatorsPerDay + slot) % len(terminatorPool)
	if idx < 0 {
		idx = 0
	}
	return terminatorPool[idx]
}
