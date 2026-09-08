package simulation

import (
	"fmt"
	"time"
)

// PersonaType maps to the product's persona framework: each value is one
// resource-type × capacity combination that the simulation can act out.
type PersonaType int

const (
	PersonaAlfred PersonaType = iota // Stuff × Giving — lots of gear, generous lender
	PersonaDerek                     // Stuff × Exchanging — selective, lends & borrows
	PersonaGary                      // Stuff × Seeking — borrows a lot, little to share
	PersonaBetty                     // Time × Giving — hosts many experiences
	PersonaEmma                      // Time × Exchanging — occasional participant
	PersonaHenry                     // Time × Seeking — makes requests for help
)

// String returns the persona name.
func (p PersonaType) String() string {
	switch p {
	case PersonaAlfred:
		return "Alfred"
	case PersonaDerek:
		return "Derek"
	case PersonaGary:
		return "Gary"
	case PersonaBetty:
		return "Betty"
	case PersonaEmma:
		return "Emma"
	case PersonaHenry:
		return "Henry"
	default:
		return fmt.Sprintf("Unknown(%d)", int(p))
	}
}

// Scenario defines a complete community simulation.
type Scenario struct {
	// Name is a short identifier for the scenario (e.g., "suburban-neighborhood").
	Name string

	// Description explains what this scenario demonstrates.
	Description string

	// Communities to create in this simulation.
	Communities []CommunityDef

	// StartTime is the simulated start of activity (e.g., 6 months ago).
	StartTime time.Time

	// EndTime is the simulated end of activity (e.g., now).
	EndTime time.Time
}

// SimulationID returns the formatted simulation ID for this scenario.
// Format: sim-scenario-name (date-independent for idempotent reruns).
func (s Scenario) SimulationID() string {
	return fmt.Sprintf("sim-%s", s.Name)
}

// CommunityDef defines a community and its members.
type CommunityDef struct {
	// Name of the community (e.g., "Portland Tool Library").
	Name string

	// Description of the community.
	Description string

	// Region for the community (e.g., "Portland, OR").
	Region string

	// Members defines users and their personas.
	Members []MemberDef
}

// MemberDef defines a user and their persona.
type MemberDef struct {
	// Name is the display name for the user.
	Name string

	// Email is the user's email (used for dev-mode auth).
	Email string

	// Persona determines the user's behavior patterns.
	Persona PersonaType
}

// GearTemplate defines a gear item to be created during simulation.
type GearTemplate struct {
	// Name of the gear item.
	Name string

	// Description of the gear item.
	Description string

	// Category (e.g., "Tools", "Kitchen", "Outdoor").
	Category string

	// Brand of the gear.
	Brand string

	// Model of the gear.
	Model string

	// MaterialCategory for carbon estimation (e.g., "metal", "plastic").
	MaterialCategory string

	// WeightGrams estimated weight.
	WeightGrams float64

	// ValueUSD estimated value in dollars.
	ValueUSD float64

	// AssetFilename is the image file in assets/gear/ to use (optional).
	AssetFilename string
}

// ExperienceTemplate defines an experience to be created during simulation.
type ExperienceTemplate struct {
	// Name of the experience (e.g., "Community BBQ").
	Name string `json:"name"`

	// Description of the experience.
	Description string `json:"description"`

	// DurationMinutes is how long the experience typically lasts.
	DurationMinutes int `json:"duration_minutes"`

	// MaxParticipants is the suggested max attendees (0 = unlimited).
	MaxParticipants int `json:"max_participants"`

	// Category for grouping (e.g., "social", "workshop", "outdoor", "sports").
	Category string `json:"category"`

	// VenueCategory is the type of venue where this experience should be held.
	// Matches venue categories from the venue data (e.g., "park", "library",
	// "cafe", "community_center", "gym", "restaurant", "church", "brewery").
	VenueCategory string `json:"venue_category"`
}

// RequestTemplate defines a request to be created during simulation.
type RequestTemplate struct {
	// Title of the request (e.g., "Need help moving furniture").
	Title string `json:"title"`

	// Description of the request with details.
	Description string `json:"description"`

	// Category for grouping (e.g., "borrow", "help", "advice").
	Category string `json:"category"`
}

// ChatTemplate holds a single message template with placeholder text.
type ChatTemplate struct {
	// Text is the message body with placeholders like {item_name}, {owner_name}.
	Text string `json:"text"`
}

// ChatPhase groups messages by conversation phase (e.g., "interest", "logistics").
type ChatPhase struct {
	// Phase is the conversation phase name.
	Phase string `json:"phase"`

	// Messages is the pool of templates to pick from for this phase.
	Messages []ChatTemplate `json:"messages"`
}

// ChatTemplateSet holds all chat templates organized by transaction type.
type ChatTemplateSet struct {
	Loan       []ChatPhase `json:"loan"`
	Giveaway   []ChatPhase `json:"giveaway"`
	Request    []ChatPhase `json:"request"`
	Experience []ChatPhase `json:"experience"`
}

// RunConfig configures a simulation run.
type RunConfig struct {
	// ServerURL is the base URL of the server to run against.
	ServerURL string

	// Scenario defines what to simulate.
	Scenario Scenario

	// Seed for the PRNG to ensure reproducible simulations.
	Seed int64

	// AssetsDir is the path to simulation image assets (optional).
	// When set, images are uploaded via MediaService and attached to gear,
	// experiences, requests, and user profiles during simulation.
	AssetsDir string

	// ReadTraffic enables periodic read actions (feed checks, browsing, etc.).
	ReadTraffic bool

	// ReadMultiplier scales read frequencies (1.0 = normal, 2.0 = double).
	ReadMultiplier float64

	// Concurrency sets the number of concurrent user goroutines for timeline
	// execution. 0 means sequential (existing behavior).
	Concurrency int
}

// Validate checks that the scenario and run config are well-formed.
func (s Scenario) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("scenario name is required")
	}
	if len(s.Communities) == 0 {
		return fmt.Errorf("scenario must have at least one community")
	}
	if s.StartTime.IsZero() {
		return fmt.Errorf("scenario start time is required")
	}
	if s.EndTime.IsZero() {
		return fmt.Errorf("scenario end time is required")
	}
	if !s.EndTime.After(s.StartTime) {
		return fmt.Errorf("scenario end time must be after start time")
	}
	for i, c := range s.Communities {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("community[%d]: %w", i, err)
		}
	}
	return nil
}

// MaxCommunityMembers is the maximum number of members allowed per community.
const MaxCommunityMembers = 32

// Validate checks that the community definition is well-formed.
func (c CommunityDef) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("community name is required")
	}
	if len(c.Members) == 0 {
		return fmt.Errorf("community must have at least one member")
	}
	if len(c.Members) > MaxCommunityMembers {
		return fmt.Errorf("community %q has %d members, max is %d", c.Name, len(c.Members), MaxCommunityMembers)
	}
	emails := make(map[string]bool)
	for i, m := range c.Members {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("member[%d]: %w", i, err)
		}
		if emails[m.Email] {
			return fmt.Errorf("member[%d]: duplicate email %q", i, m.Email)
		}
		emails[m.Email] = true
	}
	return nil
}

// Validate checks that the member definition is well-formed.
func (m MemberDef) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("member name is required")
	}
	if m.Email == "" {
		return fmt.Errorf("member email is required")
	}
	if m.Persona < PersonaAlfred || m.Persona > PersonaHenry {
		return fmt.Errorf("invalid persona type: %d", m.Persona)
	}
	return nil
}

// Validate checks that the run config is well-formed.
func (rc RunConfig) Validate() error {
	if rc.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}
	return rc.Scenario.Validate()
}
