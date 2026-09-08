package simulation

import "sync"

// State tracks authentication credentials and entity IDs created during a
// simulation run.
type State struct {
	// mu protects all fields for concurrent access. The sequential executor
	// path does not use the mutex, so there is zero overhead in that case.
	mu sync.RWMutex
	// UserTokens maps email → auth token.
	UserTokens map[string]string

	// UserIDs maps email → user ID.
	UserIDs map[string]string

	// CommunityIDs maps community name → ID.
	CommunityIDs map[string]string

	// GearIDs maps ref key → gear ID.
	GearIDs map[string]string

	// GearTemplates maps ref key → the GearTemplate used to create it.
	GearTemplates map[string]GearTemplate

	// GearOwners maps gear ref key → owner email (for re-sharing).
	GearOwners map[string]string

	// LocationIDs maps a location key → location ID (for reuse).
	LocationIDs map[string]string

	// TransferIDs maps ref key → transfer ID.
	TransferIDs map[string]string

	// RequestIDs maps ref key → request ID.
	RequestIDs map[string]string

	// ExperienceIDs maps ref key → experience ID.
	ExperienceIDs map[string]string

	// InterestedUsers maps ref key → list of user emails who expressed interest.
	InterestedUsers map[string][]string

	// HelperUsers maps ref key → list of user emails who offered help.
	HelperUsers map[string][]string

	// RequestOwners maps request ref key → requester email.
	RequestOwners map[string]string

	// ExperienceOwners maps experience ref key → host email.
	ExperienceOwners map[string]string

	// MediaIDs maps asset filename → uploaded media ID.
	MediaIDs map[string]string

	// ConversationIDs maps ref key → conversation ID.
	ConversationIDs map[string]string

	// UserNames maps email → display name.
	UserNames map[string]string

	// CommunityMembers maps community name → list of member emails.
	CommunityMembers map[string][]string

	// CompletedRefs tracks refs whose terminal action succeeded (e.g., transfer
	// completed, giveaway completed, request fulfilled/cancelled, experience
	// completed/cancelled). Used to skip late-arriving steps that target
	// already-finished flows.
	CompletedRefs map[string]bool

	// FailedRefs tracks refs whose initiating action failed (e.g., ExpressInterest
	// returned an error). Subsequent steps for these refs are skipped.
	FailedRefs map[string]bool

	// Counters for summary reporting.
	ReadsExecuted        int
	GearCreated          int
	TransfersStarted     int
	TransfersCompleted   int
	TransfersCancelled   int
	InterestWithdrawn    int
	RequestsSubmitted    int
	RequestsFulfilled    int
	RequestsCancelled    int
	OffersWithdrawn      int
	ExperiencesCreated   int
	ExperiencesCompleted int
	ExperiencesCancelled int
	RSVPYesCount         int
	RSVPNoCount          int
	ChatMessagesSent     int
	StepsSkipped         int
	StepsFailed          int
}

// NewState creates a new empty simulation state.
func NewState() *State {
	return &State{
		UserTokens:       make(map[string]string),
		UserIDs:          make(map[string]string),
		CommunityIDs:     make(map[string]string),
		GearIDs:          make(map[string]string),
		GearTemplates:    make(map[string]GearTemplate),
		GearOwners:       make(map[string]string),
		LocationIDs:      make(map[string]string),
		TransferIDs:      make(map[string]string),
		RequestIDs:       make(map[string]string),
		ExperienceIDs:    make(map[string]string),
		InterestedUsers:  make(map[string][]string),
		HelperUsers:      make(map[string][]string),
		RequestOwners:    make(map[string]string),
		ExperienceOwners: make(map[string]string),
		MediaIDs:         make(map[string]string),
		ConversationIDs:  make(map[string]string),
		UserNames:        make(map[string]string),
		CommunityMembers: make(map[string][]string),
		CompletedRefs:    make(map[string]bool),
		FailedRefs:       make(map[string]bool),
	}
}

// rlock runs fn while holding the read lock.
func (s *State) rlock(fn func()) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn()
}

// wlock runs fn while holding the write lock.
func (s *State) wlock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}
