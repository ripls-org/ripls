package ai

import (
	"fmt"
	"strings"
)

// buildConversationSummaryPrompt creates the prompt for conversation summarization.
// When requestTitle and requestDescription are provided, they are included as context
// to help the AI generate a more accurate summary.
// The style parameter determines whether to generate a progress (third-person) or completion (first-person) summary.
func buildConversationSummaryPrompt(messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) string {
	if style == SummaryStyleCompletion {
		return buildCompletionSummaryPrompt(messages, requestTitle, requestDescription)
	}
	return buildProgressSummaryPrompt(messages, requestTitle, requestDescription)
}

// buildProgressSummaryPrompt creates a third-person, objective summary of a conversation.
// Used during active request phases to show what's happening without assuming resolution.
func buildProgressSummaryPrompt(messages []ConversationMessage, requestTitle, requestDescription string) string {
	// Build conversation history
	var conversationText strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&conversationText, "%s: %s\n", msg.SenderName, msg.Text)
	}

	// Build context section if request details are provided
	contextSection := ""
	if requestTitle != "" || requestDescription != "" {
		contextSection = "REQUEST CONTEXT:\n"
		if requestTitle != "" {
			contextSection += fmt.Sprintf("The request is for: \"%s\"\n", requestTitle)
		}
		if requestDescription != "" {
			contextSection += fmt.Sprintf("\nDetails: %s\n", requestDescription)
		}
		contextSection += "\n"
	}

	return fmt.Sprintf(`You are an expert at summarizing conversations concisely and capturing key points.

Your task is to create a brief, objective summary of the following conversation about a help request.

%s`+toneAndStyle+`
- Write in third person (not as the requester)
- Be factual and objective
- Do NOT assume the request has been resolved unless explicitly stated
- Do NOT include thank-yous or gratitude (that comes later)

CONVERSATION:
%s

REQUIREMENTS:
1. Create a 2-3 sentence summary that captures:
   - The current state of the conversation
   - Who has offered to help and what they've offered
   - Any logistics being discussed (timing, location, etc.)

2. Focus on facts:
   - What has been discussed or agreed upon?
   - What's still being figured out?

3. Be neutral and objective:
   - Don't editorialize or interpret emotions
   - Don't assume outcomes that haven't been stated
   - Report only what's in the conversation

4. Keep it concise:
   - Target: 2-3 sentences
   - Focus on the most relevant current state

EXAMPLE SUMMARIES:
- "Mike offered to lend his bike. They're working out a pickup time this weekend."
- "Sarah and Tom both volunteered to help with the move. Still discussing which day works best."
- "Three people have offered suggestions for dog sitters. The requester is reviewing options."

Return a JSON object with a single "summary" field containing your 2-3 sentence summary.`, contextSection, conversationText.String())
}

// buildCompletionSummaryPrompt creates a first-person, solution-focused summary.
// Used during wrap-up to thank helpers and highlight the resolution.
func buildCompletionSummaryPrompt(messages []ConversationMessage, requestTitle, requestDescription string) string {
	// Build conversation history
	var conversationText strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&conversationText, "%s: %s\n", msg.SenderName, msg.Text)
	}

	// Build context section if request details are provided
	contextSection := ""
	if requestTitle != "" || requestDescription != "" {
		contextSection = "REQUEST CONTEXT:\n"
		if requestTitle != "" {
			contextSection += fmt.Sprintf("The user requested help with: \"%s\"\n", requestTitle)
		}
		if requestDescription != "" {
			contextSection += fmt.Sprintf("\nDetails: %s\n", requestDescription)
		}
		contextSection += "\n"
	}

	return fmt.Sprintf(`You are an expert at summarizing conversations concisely and capturing key points.

Your task is to create a brief, appreciative summary of the following conversation in which someone requested help with something from their friends.

%s`+toneAndStyle+`
- Write in first person as if you are the person making the request
- Sound warm, friendly, and appreciative
- Show gratitude ("I really appreciate it!", "Thank you all!")

CONVERSATION:
%s

REQUIREMENTS:
1. Create a 2-3 sentence summary that captures:
   - How the request was resolved (if clear)
   - Mention a few of the helpers by name (first name only) who contributed to the resolution

2. Focus on substance over details:
   - What decisions or agreements have been made?
   - Who helped or offered assistance?

3. Be warm and thankful:
   - Express gratitude to those who helped
   - Focus on the positive outcome
   - Keep it informative so everyone knows the final outcome

4. Keep it concise:
   - Target: 2-3 sentences
   - Focus on what's most helpful for understanding the solution

EXAMPLE SUMMARIES:
- "Mike is lending his extra bike. Thank you everyone!"
- "Jane convinced us. We are going to join them at the Outcomes summer camp. We're still discussing potential dates and carpooling options but looking forward to seeing everyone there!"
- "Deandre, Mary, and Jon all volunteered to help paint the fence. You guys are amazing!"

Return a JSON object with a single "summary" field containing your 2-3 sentence summary.`, contextSection, conversationText.String())
}

// buildExperienceCompletionSummaryPrompt creates the prompt for experience completion summarization.
// Includes experience context (title, description) and attendee names.
// messages may be empty; the summary will be generated from experience details and attendees.
func buildExperienceCompletionSummaryPrompt(messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) string {
	// Build conversation section (may be empty)
	var conversationSection string
	if len(messages) > 0 {
		var conversationText strings.Builder
		for _, msg := range messages {
			fmt.Fprintf(&conversationText, "%s: %s\n", msg.SenderName, msg.Text)
		}
		conversationSection = fmt.Sprintf(`
CONVERSATION (from the planning phase, before the event took place):
%s`, conversationText.String())
	}

	// Build attendee section
	attendeeSection := ""
	if len(attendeeNames) > 0 {
		attendeeSection = fmt.Sprintf("\nATTENDEES: %s", strings.Join(attendeeNames, ", "))
	}

	return fmt.Sprintf(`You are an expert at summarizing experiences and group activities.

The group organized an experience called "%s" (original description: "%s") and it has already taken place.
%s%s

CRITICAL RULES:
- This experience has ALREADY HAPPENED. Write ENTIRELY in PAST TENSE.
- Do NOT copy or paraphrase the description. The description was written BEFORE the event to invite people. Write your own original summary of how it went AFTER the fact.
- Do NOT use future-tense language like "let's", "anyone want to", "we can", "come join".
- Mention attendees by first name.

`+toneAndStyle+`
- Write in first person as if you are the person who hosted/organized the experience
- Sound warm, friendly, and appreciative
- Use past tense throughout ("had a great time", "everyone enjoyed", "we went")
- Show gratitude to attendees ("Thanks everyone for coming!", "You all were amazing!")

REQUIREMENTS:
1. Create a 2-3 sentence summary that captures:
   - How the experience went (past tense)
   - Key highlights or memorable moments
   - Mention attendees by first name

2. Focus on the experience itself:
   - What happened during the activity
   - What made it enjoyable or memorable

3. Be positive and celebratory:
   - Focus on what went well
   - Create a sense of community and shared accomplishment

4. Keep it concise:
   - Target: 2-3 sentences

EXAMPLE SUMMARIES:
- "Great hike at Mount Tam! Alice, Bob, and Dana all made it to the summit. Thanks for coming everyone!"
- "Had an awesome pasta-making class with Tony and Sarah. Everyone learned to make fresh fettuccine and we all enjoyed a delicious meal together."
- "Fun game night with Chris, Dana, Emma, and Frank. We played several rounds of Settlers of Catan and everyone stayed until midnight!"

Return a JSON object with a single "summary" field containing your 2-3 sentence summary.`, experienceTitle, experienceDescription, attendeeSection, conversationSection)
}

// buildRequestImageAnalysisPrompt creates the prompt for analyzing images to generate request content.
