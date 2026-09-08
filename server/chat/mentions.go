package chat

import (
	"regexp"
)

// MentionType represents the type of entity being mentioned.
type MentionType string

const (
	MentionTypeUser       MentionType = "user"
	MentionTypeLoan       MentionType = "loan"
	MentionTypeGiveaway   MentionType = "giveaway"
	MentionTypeRequest    MentionType = "request"
	MentionTypeExperience MentionType = "experience"
)

// ParsedMention represents a parsed @-mention from message text.
type ParsedMention struct {
	Type        MentionType
	ID          string
	DisplayName string
	StartIndex  int
	EndIndex    int
}

// mentionPattern matches encoded mentions in the format @[type:id:display_name]
// Types: user, loan, giveaway, request, experience.
var mentionPattern = regexp.MustCompile(`@\[(user|loan|giveaway|request|experience):([a-zA-Z0-9_-]+):([^\]]+)\]`)

// ParseMentions extracts all @-mentions from the given text.
// Returns a slice of ParsedMention objects sorted by their position in the text.
func ParseMentions(text string) []ParsedMention {
	matches := mentionPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	mentions := make([]ParsedMention, 0, len(matches))
	for _, match := range matches {
		// match indices: [full start, full end, type start, type end, id start, id end, name start, name end]
		if len(match) < 8 {
			continue
		}

		mentionType := text[match[2]:match[3]]
		id := text[match[4]:match[5]]
		displayName := text[match[6]:match[7]]

		mentions = append(mentions, ParsedMention{
			Type:        MentionType(mentionType),
			ID:          id,
			DisplayName: displayName,
			StartIndex:  match[0],
			EndIndex:    match[1],
		})
	}

	return mentions
}

// ParseUserMentions extracts only user @-mentions from the given text.
// Returns a slice of user IDs that were mentioned.
func ParseUserMentions(text string) []string {
	mentions := ParseMentions(text)
	if len(mentions) == 0 {
		return nil
	}

	userIDs := make([]string, 0)
	for _, mention := range mentions {
		if mention.Type == MentionTypeUser {
			userIDs = append(userIDs, mention.ID)
		}
	}

	return userIDs
}

// HasMentions returns true if the text contains any @-mentions.
func HasMentions(text string) bool {
	return mentionPattern.MatchString(text)
}

// AddedViaMentionText returns the display text for an added-via-mention action.
func AddedViaMentionText(addedUserName, mentionerName string) string {
	return addedUserName + " was added by " + mentionerName
}

// DecodeMentions converts encoded mentions to human-readable format.
// Replaces @[type:id:display_name] with @display_name.
func DecodeMentions(text string) string {
	return mentionPattern.ReplaceAllString(text, "@$3")
}
