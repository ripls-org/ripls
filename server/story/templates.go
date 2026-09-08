package story

import (
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// storyTemplate contains template strings for generating story content.
type storyTemplate struct {
	// TitleTemplate is a format string for generating the title.
	TitleTemplate string

	// DescriptionTemplate is a format string for generating the description.
	DescriptionTemplate string
}

// getTemplateForStoryType returns the appropriate template for a given story type.
// These are simple fallback templates used when LLM generation is unavailable or fails.
func getTemplateForStoryType(storyType string) storyTemplate {
	switch storyType {
	case ai.StoryTypeLoanCompleted:
		return storyTemplate{
			TitleTemplate:       "Loan Completed Successfully",
			DescriptionTemplate: "%s borrowed %s from %s and returned it. Another successful share in the community!",
		}

	case ai.StoryTypeGiveawayCompleted:
		return storyTemplate{
			TitleTemplate:       "Item Given to Community Member",
			DescriptionTemplate: "%s generously gave %s to %s. What a wonderful act of giving!",
		}

	case ai.StoryTypeExperienceConcluded:
		return storyTemplate{
			TitleTemplate:       "Event Completed",
			DescriptionTemplate: "%s hosted %s with %d participants. Great time shared together!",
		}

	case ai.StoryTypeRequestFulfilled:
		return storyTemplate{
			TitleTemplate:       "Request Fulfilled",
			DescriptionTemplate: "%s's request for %s was fulfilled by %s. The community came through!",
		}

	case ai.StoryTypeNewMemberWelcome:
		return storyTemplate{
			TitleTemplate:       "Welcome to the Community",
			DescriptionTemplate: "Everyone, let's welcome %s to %s! We're excited to have you here.",
		}

	default:
		return storyTemplate{
			TitleTemplate:       "Community Story",
			DescriptionTemplate: "Something great happened in the community!",
		}
	}
}

// templateResult bundles the English fallback text with the
// structured-template payload Phase 4b clients use to render the
// story in the viewer's locale. Both halves are populated together;
// the legacy text fields stay on Story for backward compatibility
// (TODO(#2159): retire them after historical rows age out).
type templateResult struct {
	Title       string
	Description string
	TemplateKey string // empty when no structured payload (AI-generated)
	Params      map[string]string
}

// generateFromTemplate creates story content from a template using
// the request data. Returns both the English fallback strings (used
// by old clients and by the legacy Story.title/description fields)
// and the structured template_key + params that new clients
// resolve against their ARB catalog.
func generateFromTemplate(req CreateStoryRequest) templateResult {
	template := getTemplateForStoryType(req.StoryType)
	res := templateResult{Params: map[string]string{}}

	// res.Title is the English fallback persisted on Story.title for
	// older clients and for any surface that displays the literal
	// title field directly. Locale-aware clients (Phase 4b onward)
	// resolve the title from template_key via story_text_resolver.dart;
	// the literal string here is never shown when a template_key is
	// also emitted alongside.
	if req.StoryType == ai.StoryTypeLoanCompleted && req.RelatedEntityName != "" {
		res.Title = req.RelatedEntityName + " Lent"
	} else {
		res.Title = template.TitleTemplate
	}

	// Description varies based on story type
	switch req.StoryType {
	case ai.StoryTypeLoanCompleted:
		if len(req.ParticipantNames) >= 2 {
			res.Description = fmt.Sprintf(template.DescriptionTemplate,
				req.ParticipantNames[0], // borrower
				req.RelatedEntityName,   // gear name
				req.ParticipantNames[1], // lender
			)
			res.TemplateKey = "story.loan_completed"
			res.Params["borrowerName"] = req.ParticipantNames[0]
			res.Params["gearName"] = req.RelatedEntityName
			res.Params["lenderName"] = req.ParticipantNames[1]
		} else {
			res.Description = fmt.Sprintf("A loan of %s was completed successfully.", req.RelatedEntityName)
			res.TemplateKey = "story.loan_completed_simple"
			res.Params["gearName"] = req.RelatedEntityName
		}

	case ai.StoryTypeGiveawayCompleted:
		if len(req.ParticipantNames) >= 2 {
			res.Description = fmt.Sprintf(template.DescriptionTemplate,
				req.ParticipantNames[0], // giver
				req.RelatedEntityName,   // gear name
				req.ParticipantNames[1], // receiver
			)
			res.TemplateKey = "story.giveaway_completed"
			res.Params["giverName"] = req.ParticipantNames[0]
			res.Params["gearName"] = req.RelatedEntityName
			res.Params["receiverName"] = req.ParticipantNames[1]
		} else {
			res.Description = fmt.Sprintf("%s was given to a community member.", req.RelatedEntityName)
			res.TemplateKey = "story.giveaway_completed_simple"
			res.Params["gearName"] = req.RelatedEntityName
		}

	case ai.StoryTypeExperienceConcluded:
		if len(req.ParticipantNames) >= 1 {
			// Count participants (creator + attendees)
			participantCount := len(req.ParticipantIDs)
			res.Description = fmt.Sprintf(template.DescriptionTemplate,
				req.ParticipantNames[0], // creator
				req.RelatedEntityName,   // experience name
				participantCount,
			)
			res.TemplateKey = "story.experience_concluded"
			res.Params["hostName"] = req.ParticipantNames[0]
			res.Params["eventName"] = req.RelatedEntityName
			res.Params["participantCount"] = fmt.Sprintf("%d", participantCount)
		} else {
			res.Description = fmt.Sprintf("The event %s was completed.", req.RelatedEntityName)
			res.TemplateKey = "story.experience_concluded_simple"
			res.Params["eventName"] = req.RelatedEntityName
		}

	case ai.StoryTypeRequestFulfilled:
		if len(req.ParticipantNames) >= 2 {
			res.Description = fmt.Sprintf(template.DescriptionTemplate,
				req.ParticipantNames[0], // requester
				req.RelatedEntityName,   // request title
				req.ParticipantNames[1], // fulfiller
			)
			res.TemplateKey = "story.request_fulfilled"
			res.Params["requesterName"] = req.ParticipantNames[0]
			res.Params["requestTitle"] = req.RelatedEntityName
			res.Params["fulfillerName"] = req.ParticipantNames[1]
		} else {
			res.Description = fmt.Sprintf("A request for %s was fulfilled.", req.RelatedEntityName)
			res.TemplateKey = "story.request_fulfilled_simple"
			res.Params["requestTitle"] = req.RelatedEntityName
		}

	case ai.StoryTypeNewMemberWelcome:
		if len(req.ParticipantNames) >= 1 {
			res.Description = fmt.Sprintf(template.DescriptionTemplate,
				req.ParticipantNames[0], // new member name
				req.CommunityName,       // community name
			)
			res.TemplateKey = "story.new_member_welcome"
			res.Params["memberName"] = req.ParticipantNames[0]
			res.Params["communityName"] = req.CommunityName
		} else {
			res.Description = fmt.Sprintf("A new member joined %s!", req.CommunityName)
			res.TemplateKey = "story.new_member_welcome_simple"
			res.Params["communityName"] = req.CommunityName
		}

	default:
		res.Description = template.DescriptionTemplate
		// No template_key for the default branch — client falls
		// back to the literal description.
	}

	return res
}

// formatParticipantNames formats a list of participant names into a natural language string.
// Examples:
// - ["Alice"] -> "Alice"
// - ["Alice", "Bob"] -> "Alice and Bob"
// - ["Alice", "Bob", "Carol"] -> "Alice, Bob, and Carol".
func formatParticipantNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return fmt.Sprintf("%s and %s", names[0], names[1])
	default:
		// Join all but last with commas, then add "and" before last
		allButLast := strings.Join(names[:len(names)-1], ", ")
		return fmt.Sprintf("%s, and %s", allButLast, names[len(names)-1])
	}
}
