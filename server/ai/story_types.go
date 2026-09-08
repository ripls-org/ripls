package ai

// Story-type discriminators. These are the values carried by
// models.Story.story_type (a bare string) and GenerateStoryRequest.StoryType,
// and they match the api.StoryType enum's value names one-for-one so the feed
// can map storage → API without a translation table.
//
// Nothing about these is AI-specific — story copy is rendered from the
// template catalog in server/story/templates.go (#2936). They stay in this
// package because server/story already imports it and the reverse would be an
// import cycle. Writers (server/story/subscriber) and readers (services/feed,
// services/user, server/story) all reference these rather than repeating the
// literals.
const (
	StoryTypeLoanCompleted       = "STORY_TYPE_LOAN_COMPLETED"
	StoryTypeGiveawayCompleted   = "STORY_TYPE_GIVEAWAY_COMPLETED"
	StoryTypeExperienceConcluded = "STORY_TYPE_EXPERIENCE_CONCLUDED"
	StoryTypeRequestFulfilled    = "STORY_TYPE_REQUEST_FULFILLED"
	StoryTypeNewMemberWelcome    = "STORY_TYPE_NEW_MEMBER_WELCOME"
	StoryTypeExperienceRSVP      = "STORY_TYPE_EXPERIENCE_RSVP"
)
