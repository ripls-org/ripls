package simulation

import "math/rand"

// ActivityType represents a kind of action a persona can take.
type ActivityType int

const (
	ActivityShareGear        ActivityType = iota // List gear in community
	ActivityBorrow                               // Express interest in borrowing gear
	ActivityGiveAway                             // Give away gear permanently
	ActivityHostExperience                       // Create and host an experience
	ActivityAttendExperience                     // RSVP to someone else's experience
	ActivityMakeRequest                          // Submit a request for help/items
	ActivityOfferHelp                            // Offer to fulfill a request
)

// PersonaWeights defines the activity probability distribution for a persona.
type PersonaWeights struct {
	// Persona is which persona this applies to.
	Persona PersonaType

	// Weights maps activity type to relative probability (0.0-1.0).
	Weights map[ActivityType]float64

	// BaseActivitiesPerWeek is the average number of activities per simulated week.
	BaseActivitiesPerWeek float64

	// ContentionProbability is how likely the persona is to express interest
	// in gear that already has other interested users (0.0-1.0).
	ContentionProbability float64

	// GearCategories lists gear catalog categories this persona draws from.
	GearCategories []string

	// GearCountMin is the minimum number of gear items this persona starts with.
	GearCountMin int

	// GearCountMax is the maximum number of gear items this persona starts with.
	GearCountMax int
}

// PersonaConfig returns the behavior configuration for each persona type.
func PersonaConfig() map[PersonaType]PersonaWeights {
	return map[PersonaType]PersonaWeights{
		PersonaAlfred: {
			Persona: PersonaAlfred,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.30,
				ActivityBorrow:           0.05,
				ActivityGiveAway:         0.05,
				ActivityHostExperience:   0.05,
				ActivityAttendExperience: 0.15,
				ActivityMakeRequest:      0.05,
				ActivityOfferHelp:        0.15,
			},
			BaseActivitiesPerWeek: 4,
			ContentionProbability: 0.15,
			GearCategories:        []string{"Tools", "Outdoor", "Garden"},
			GearCountMin:          12,
			GearCountMax:          20,
		},
		PersonaDerek: {
			Persona: PersonaDerek,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.15,
				ActivityBorrow:           0.20,
				ActivityGiveAway:         0.05,
				ActivityHostExperience:   0.05,
				ActivityAttendExperience: 0.15,
				ActivityMakeRequest:      0.10,
				ActivityOfferHelp:        0.10,
			},
			BaseActivitiesPerWeek: 3,
			ContentionProbability: 0.25,
			GearCategories:        []string{"Kitchen", "Outdoor", "Electronics"},
			GearCountMin:          5,
			GearCountMax:          10,
		},
		PersonaGary: {
			Persona: PersonaGary,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.05,
				ActivityBorrow:           0.35,
				ActivityGiveAway:         0.00,
				ActivityHostExperience:   0.00,
				ActivityAttendExperience: 0.20,
				ActivityMakeRequest:      0.20,
				ActivityOfferHelp:        0.05,
			},
			BaseActivitiesPerWeek: 2,
			ContentionProbability: 0.40,
			GearCategories:        []string{"Kitchen", "Party"},
			GearCountMin:          1,
			GearCountMax:          3,
		},
		PersonaBetty: {
			Persona: PersonaBetty,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.05,
				ActivityBorrow:           0.05,
				ActivityGiveAway:         0.00,
				ActivityHostExperience:   0.35,
				ActivityAttendExperience: 0.15,
				ActivityMakeRequest:      0.05,
				ActivityOfferHelp:        0.20,
			},
			BaseActivitiesPerWeek: 4,
			ContentionProbability: 0.20,
			GearCategories:        []string{"Party", "Kitchen"},
			GearCountMin:          2,
			GearCountMax:          5,
		},
		PersonaEmma: {
			Persona: PersonaEmma,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.05,
				ActivityBorrow:           0.10,
				ActivityGiveAway:         0.00,
				ActivityHostExperience:   0.10,
				ActivityAttendExperience: 0.30,
				ActivityMakeRequest:      0.10,
				ActivityOfferHelp:        0.15,
			},
			BaseActivitiesPerWeek: 3,
			ContentionProbability: 0.30,
			GearCategories:        []string{"Outdoor", "Kitchen", "Electronics"},
			GearCountMin:          3,
			GearCountMax:          6,
		},
		PersonaHenry: {
			Persona: PersonaHenry,
			Weights: map[ActivityType]float64{
				ActivityShareGear:        0.02,
				ActivityBorrow:           0.10,
				ActivityGiveAway:         0.00,
				ActivityHostExperience:   0.03,
				ActivityAttendExperience: 0.10,
				ActivityMakeRequest:      0.40,
				ActivityOfferHelp:        0.05,
			},
			BaseActivitiesPerWeek: 2,
			ContentionProbability: 0.35,
			GearCategories:        []string{"Tools"},
			GearCountMin:          0,
			GearCountMax:          2,
		},
	}
}

// allActivityTypes defines a fixed iteration order for deterministic selection.
var allActivityTypes = []ActivityType{
	ActivityShareGear,
	ActivityBorrow,
	ActivityGiveAway,
	ActivityHostExperience,
	ActivityAttendExperience,
	ActivityMakeRequest,
	ActivityOfferHelp,
}

// SelectActivity picks an activity type based on persona weights using the PRNG.
func SelectActivity(rng *rand.Rand, weights map[ActivityType]float64) ActivityType {
	total := 0.0
	for _, at := range allActivityTypes {
		total += weights[at]
	}

	r := rng.Float64() * total
	cumulative := 0.0
	for _, at := range allActivityTypes {
		cumulative += weights[at]
		if r < cumulative {
			return at
		}
	}

	// Fallback — should not happen with valid weights.
	return ActivityAttendExperience
}

// GearForPersona selects gear items from the catalog for a persona based on
// their category preferences and count range.
func GearForPersona(rng *rand.Rand, persona PersonaType) []GearTemplate {
	config := PersonaConfig()[persona]

	// Collect eligible gear from preferred categories.
	var eligible []GearTemplate
	categorySet := make(map[string]bool)
	for _, c := range config.GearCategories {
		categorySet[c] = true
	}
	for _, g := range AllGear() {
		if categorySet[g.Category] {
			eligible = append(eligible, g)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Determine count within persona's range.
	count := config.GearCountMin
	if config.GearCountMax > config.GearCountMin {
		count += rng.Intn(config.GearCountMax - config.GearCountMin + 1)
	}

	// Cap at available items.
	if count > len(eligible) {
		count = len(eligible)
	}

	// Shuffle and pick.
	rng.Shuffle(len(eligible), func(i, j int) {
		eligible[i], eligible[j] = eligible[j], eligible[i]
	})

	return eligible[:count]
}
