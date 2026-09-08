package simulation

import (
	"fmt"
	"math/rand"
	"time"
)

// ReadBehavior defines how often a persona performs read actions per simulated week.
// Values derived from production traffic analysis (docs/ai/production_traffic_analysis.md).
type ReadBehavior struct {
	FeedChecksPerWeek         float64
	SearchesPerWeek           float64
	GearBrowsesPerWeek        float64
	ProfileViewsPerWeek       float64
	StoryViewsPerWeek         float64
	ConversationChecksPerWeek float64
	ImpactViewsPerWeek        float64
}

// ReadConfig returns the read behavior configuration for each persona type.
// Weights are calibrated from UX flow analysis and production traffic patterns.
func ReadConfig() map[PersonaType]ReadBehavior {
	return map[PersonaType]ReadBehavior{
		PersonaAlfred: { // Stuff × Giving — lots of gear, generous lender
			FeedChecksPerWeek:         7,
			SearchesPerWeek:           1,
			GearBrowsesPerWeek:        4,
			ProfileViewsPerWeek:       1,
			StoryViewsPerWeek:         3,
			ConversationChecksPerWeek: 5,
			ImpactViewsPerWeek:        1,
		},
		PersonaDerek: { // Stuff × Exchanging — selective, lends & borrows
			FeedChecksPerWeek:         10,
			SearchesPerWeek:           3,
			GearBrowsesPerWeek:        6,
			ProfileViewsPerWeek:       2,
			StoryViewsPerWeek:         3,
			ConversationChecksPerWeek: 7,
			ImpactViewsPerWeek:        1,
		},
		PersonaGary: { // Stuff × Seeking — borrows a lot, little to share
			FeedChecksPerWeek:         14,
			SearchesPerWeek:           5,
			GearBrowsesPerWeek:        10,
			ProfileViewsPerWeek:       2,
			StoryViewsPerWeek:         3,
			ConversationChecksPerWeek: 7,
			ImpactViewsPerWeek:        0.5,
		},
		PersonaBetty: { // Time × Giving — hosts many experiences
			FeedChecksPerWeek:         7,
			SearchesPerWeek:           1,
			GearBrowsesPerWeek:        2,
			ProfileViewsPerWeek:       1,
			StoryViewsPerWeek:         5,
			ConversationChecksPerWeek: 5,
			ImpactViewsPerWeek:        2,
		},
		PersonaEmma: { // Time × Exchanging — occasional participant
			FeedChecksPerWeek:         10,
			SearchesPerWeek:           2,
			GearBrowsesPerWeek:        4,
			ProfileViewsPerWeek:       2,
			StoryViewsPerWeek:         4,
			ConversationChecksPerWeek: 7,
			ImpactViewsPerWeek:        1,
		},
		PersonaHenry: { // Time × Seeking — makes requests for help
			FeedChecksPerWeek:         14,
			SearchesPerWeek:           4,
			GearBrowsesPerWeek:        6,
			ProfileViewsPerWeek:       1,
			StoryViewsPerWeek:         3,
			ConversationChecksPerWeek: 7,
			ImpactViewsPerWeek:        0.5,
		},
	}
}

// readActionEntry pairs a read action with its per-week frequency.
type readActionEntry struct {
	action  Action
	perWeek float64
}

// generateReadTimeline generates periodic read steps for all members across
// all communities, using Poisson-sampled frequencies per persona per week.
// The multiplier scales all read frequencies (e.g., 2.0 = double the reads).
func generateReadTimeline(rng *rand.Rand, scenario Scenario, multiplier float64) Timeline {
	readCfg := ReadConfig()
	var timeline Timeline
	refCounter := 0

	weekDuration := 7 * 24 * time.Hour
	// Start reads after the first week (gear setup period).
	readStart := scenario.StartTime.Add(weekDuration)

	for _, comm := range scenario.Communities {
		for _, member := range comm.Members {
			behavior := readCfg[member.Persona]
			actions := []readActionEntry{
				{ActionCheckFeed, behavior.FeedChecksPerWeek},
				{ActionSearchCommunity, behavior.SearchesPerWeek},
				{ActionBrowseGear, behavior.GearBrowsesPerWeek},
				{ActionViewProfile, behavior.ProfileViewsPerWeek},
				{ActionCheckStories, behavior.StoryViewsPerWeek},
				{ActionCheckConversations, behavior.ConversationChecksPerWeek},
				{ActionViewImpact, behavior.ImpactViewsPerWeek},
			}

			currentWeek := readStart
			for currentWeek.Before(scenario.EndTime) {
				weekEnd := currentWeek.Add(weekDuration)
				if weekEnd.After(scenario.EndTime) {
					weekEnd = scenario.EndTime
				}

				for _, entry := range actions {
					scaledRate := entry.perWeek * multiplier
					count := poissonSample(rng, scaledRate)
					for range count {
						refCounter++
						t := jitterTime(rng, currentWeek, 0, weekEnd.Sub(currentWeek))
						if t.After(scenario.EndTime) {
							continue
						}
						timeline = append(timeline, ActivityStep{
							Time:          t,
							Actor:         member.Email,
							CommunityName: comm.Name,
							Action:        entry.action,
							Ref:           fmt.Sprintf("read-%s-%d", comm.Name, refCounter),
						})
					}
				}

				currentWeek = weekEnd
			}
		}
	}

	return timeline
}

// SearchQueryPool returns a pool of realistic search terms drawn from gear
// catalog names, categories, and common keywords.
func SearchQueryPool() []string {
	// Build from gear catalog.
	var queries []string
	seen := make(map[string]bool)

	for _, g := range AllGear() {
		if !seen[g.Name] {
			queries = append(queries, g.Name)
			seen[g.Name] = true
		}
		if !seen[g.Category] {
			queries = append(queries, g.Category)
			seen[g.Category] = true
		}
		if g.Brand != "" && !seen[g.Brand] {
			queries = append(queries, g.Brand)
			seen[g.Brand] = true
		}
	}

	// Add common search terms.
	extras := []string{
		"drill", "saw", "ladder", "tent", "cooler", "projector",
		"blender", "mixer", "grill", "kayak", "bike", "camera",
		"speaker", "table", "chairs", "generator", "pressure washer",
		"help moving", "ride to airport", "dog sitting",
		"community BBQ", "workshop", "yoga", "gardening",
	}
	for _, q := range extras {
		if !seen[q] {
			queries = append(queries, q)
			seen[q] = true
		}
	}

	return queries
}
