package chat

import (
	"testing"
)

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []ParsedMention
	}{
		{
			name:     "empty text",
			text:     "",
			expected: nil,
		},
		{
			name:     "no mentions",
			text:     "Hello world!",
			expected: nil,
		},
		{
			name: "single user mention",
			text: "Hello @[user:abc123:John Doe]!",
			expected: []ParsedMention{
				{Type: MentionTypeUser, ID: "abc123", DisplayName: "John Doe", StartIndex: 6, EndIndex: 29},
			},
		},
		{
			name: "single loan mention",
			text: "Check out @[loan:xyz789:Camping Tent]",
			expected: []ParsedMention{
				{Type: MentionTypeLoan, ID: "xyz789", DisplayName: "Camping Tent", StartIndex: 10, EndIndex: 37},
			},
		},
		{
			name: "single giveaway mention",
			text: "Free @[giveaway:def456:Old Bike]!",
			expected: []ParsedMention{
				{Type: MentionTypeGiveaway, ID: "def456", DisplayName: "Old Bike", StartIndex: 5, EndIndex: 32},
			},
		},
		{
			name: "single request mention",
			text: "See @[request:req123:Need a ladder]",
			expected: []ParsedMention{
				{Type: MentionTypeRequest, ID: "req123", DisplayName: "Need a ladder", StartIndex: 4, EndIndex: 35},
			},
		},
		{
			name: "single experience mention",
			text: "Join @[experience:exp456:Weekend Hike]!",
			expected: []ParsedMention{
				{Type: MentionTypeExperience, ID: "exp456", DisplayName: "Weekend Hike", StartIndex: 5, EndIndex: 38},
			},
		},
		{
			name: "multiple mentions",
			text: "Hey @[user:u1:Alice] and @[user:u2:Bob], check @[loan:l1:Tent]",
			expected: []ParsedMention{
				{Type: MentionTypeUser, ID: "u1", DisplayName: "Alice", StartIndex: 4, EndIndex: 20},
				{Type: MentionTypeUser, ID: "u2", DisplayName: "Bob", StartIndex: 25, EndIndex: 39},
				{Type: MentionTypeLoan, ID: "l1", DisplayName: "Tent", StartIndex: 47, EndIndex: 62},
			},
		},
		{
			name: "mention with hyphen in ID",
			text: "@[user:user-123-abc:John]",
			expected: []ParsedMention{
				{Type: MentionTypeUser, ID: "user-123-abc", DisplayName: "John", StartIndex: 0, EndIndex: 25},
			},
		},
		{
			name: "mention with underscore in ID",
			text: "@[user:user_123_abc:Jane]",
			expected: []ParsedMention{
				{Type: MentionTypeUser, ID: "user_123_abc", DisplayName: "Jane", StartIndex: 0, EndIndex: 25},
			},
		},
		{
			name:     "invalid mention type",
			text:     "@[invalid:id:name]",
			expected: nil,
		},
		{
			name:     "malformed mention - missing bracket",
			text:     "@[user:id:name",
			expected: nil,
		},
		{
			name:     "malformed mention - missing colon",
			text:     "@[user:idname]",
			expected: nil,
		},
		{
			name: "mention with special chars in display name",
			text: "@[user:u1:John O'Brien-Smith]",
			expected: []ParsedMention{
				{Type: MentionTypeUser, ID: "u1", DisplayName: "John O'Brien-Smith", StartIndex: 0, EndIndex: 29},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMentions(tt.text)

			if len(got) != len(tt.expected) {
				t.Errorf("ParseMentions() returned %d mentions, want %d", len(got), len(tt.expected))
				return
			}

			for i, mention := range got {
				exp := tt.expected[i]
				if mention.Type != exp.Type {
					t.Errorf("mention[%d].Type = %q, want %q", i, mention.Type, exp.Type)
				}
				if mention.ID != exp.ID {
					t.Errorf("mention[%d].ID = %q, want %q", i, mention.ID, exp.ID)
				}
				if mention.DisplayName != exp.DisplayName {
					t.Errorf("mention[%d].DisplayName = %q, want %q", i, mention.DisplayName, exp.DisplayName)
				}
				if mention.StartIndex != exp.StartIndex {
					t.Errorf("mention[%d].StartIndex = %d, want %d", i, mention.StartIndex, exp.StartIndex)
				}
				if mention.EndIndex != exp.EndIndex {
					t.Errorf("mention[%d].EndIndex = %d, want %d", i, mention.EndIndex, exp.EndIndex)
				}
			}
		})
	}
}

func TestParseUserMentions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "empty text",
			text:     "",
			expected: nil,
		},
		{
			name:     "no mentions",
			text:     "Hello world!",
			expected: nil,
		},
		{
			name:     "single user mention",
			text:     "Hello @[user:abc123:John]!",
			expected: []string{"abc123"},
		},
		{
			name:     "multiple user mentions",
			text:     "Hey @[user:u1:Alice] and @[user:u2:Bob]!",
			expected: []string{"u1", "u2"},
		},
		{
			name:     "mixed mentions - only returns users",
			text:     "Hey @[user:u1:Alice], check @[loan:l1:Tent]",
			expected: []string{"u1"},
		},
		{
			name:     "no user mentions - only entities",
			text:     "Check @[loan:l1:Tent] and @[request:r1:Ladder]",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUserMentions(tt.text)

			if len(got) != len(tt.expected) {
				t.Errorf("ParseUserMentions() returned %d IDs, want %d", len(got), len(tt.expected))
				return
			}

			for i, id := range got {
				if id != tt.expected[i] {
					t.Errorf("ParseUserMentions()[%d] = %q, want %q", i, id, tt.expected[i])
				}
			}
		})
	}
}

func TestHasMentions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{name: "empty text", text: "", expected: false},
		{name: "no mentions", text: "Hello world!", expected: false},
		{name: "has user mention", text: "Hello @[user:u1:John]", expected: true},
		{name: "has loan mention", text: "Check @[loan:l1:Tent]", expected: true},
		{name: "has giveaway mention", text: "Free @[giveaway:g1:Bike]", expected: true},
		{name: "has request mention", text: "See @[request:r1:Ladder]", expected: true},
		{name: "has experience mention", text: "Join @[experience:e1:Hike]", expected: true},
		{name: "invalid mention", text: "@[invalid:id:name]", expected: false},
		{name: "plain @ symbol", text: "email@example.com", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasMentions(tt.text)
			if got != tt.expected {
				t.Errorf("HasMentions(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

func TestAddedViaMentionText(t *testing.T) {
	text := AddedViaMentionText("Alice", "Bob")
	expected := "Alice was added by Bob"
	if text != expected {
		t.Errorf("AddedViaMentionText() = %q, want %q", text, expected)
	}
}

func TestDecodeMentions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "empty text",
			text:     "",
			expected: "",
		},
		{
			name:     "no mentions",
			text:     "Hello world!",
			expected: "Hello world!",
		},
		{
			name:     "single user mention",
			text:     "Hello @[user:abc123:John Doe]!",
			expected: "Hello @John Doe!",
		},
		{
			name:     "single loan mention",
			text:     "Check out @[loan:xyz789:Camping Tent]",
			expected: "Check out @Camping Tent",
		},
		{
			name:     "multiple mentions",
			text:     "Hey @[user:u1:Alice] and @[user:u2:Bob], check @[loan:l1:Tent]",
			expected: "Hey @Alice and @Bob, check @Tent",
		},
		{
			name:     "mention at start",
			text:     "@[user:u1:John] said hello",
			expected: "@John said hello",
		},
		{
			name:     "mention at end",
			text:     "Hello @[user:u1:John]",
			expected: "Hello @John",
		},
		{
			name:     "only mention",
			text:     "@[user:u1:John]",
			expected: "@John",
		},
		{
			name:     "mention with special chars in name",
			text:     "@[user:u1:John O'Brien-Smith]",
			expected: "@John O'Brien-Smith",
		},
		{
			name:     "invalid mention unchanged",
			text:     "@[invalid:id:name] stays",
			expected: "@[invalid:id:name] stays",
		},
		{
			name:     "plain @ symbol unchanged",
			text:     "email@example.com",
			expected: "email@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeMentions(tt.text)
			if got != tt.expected {
				t.Errorf("DecodeMentions(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}
