package simulation

import (
	"math/rand"
	"strings"
	"testing"
)

func TestFirstName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alice Johnson", "Alice"},
		{"Bob", "Bob"},
		{"Mary Jane Watson", "Mary"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firstName(tt.input); got != tt.want {
			t.Errorf("firstName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExpandTemplate(t *testing.T) {
	cc := chatContext{
		ItemName:        "Cordless Drill",
		OwnerName:       "Alice",
		BorrowerName:    "Bob",
		Duration:        "a week or so",
		MentionUserID:   "user-123",
		MentionUserName: "Charlie",
	}

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "item name",
			input:  "Can I borrow your {item_name}?",
			expect: "Can I borrow your Cordless Drill?",
		},
		{
			name:   "owner name",
			input:  "Hey {owner_name}!",
			expect: "Hey Alice!",
		},
		{
			name:   "borrower name",
			input:  "Sure {borrower_name}, come get it.",
			expect: "Sure Bob, come get it.",
		},
		{
			name:   "duration",
			input:  "I'll return it in {duration}.",
			expect: "I'll return it in a week or so.",
		},
		{
			name:   "mention format",
			input:  "Hey @[user:{mention_user_id}:{mention_user_name}] check this out!",
			expect: "Hey @[user:user-123:Charlie] check this out!",
		},
		{
			name:   "multiple placeholders",
			input:  "{owner_name}'s {item_name} for {borrower_name}",
			expect: "Alice's Cordless Drill for Bob",
		},
		{
			name:   "no placeholders",
			input:  "Thanks so much!",
			expect: "Thanks so much!",
		},
		{
			name:   "empty context fields",
			input:  "Hey {owner_name}!",
			expect: "Hey !",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the "empty context" test, use an empty context.
			ctx := cc
			if tt.name == "empty context fields" {
				ctx = chatContext{}
			}
			got := expandTemplate(tt.input, ctx)
			if got != tt.expect {
				t.Errorf("expandTemplate(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestBuildConversationDeterministic(t *testing.T) {
	phases := []ChatPhase{
		{
			Phase: "interest",
			Messages: []ChatTemplate{
				{Text: "Can I borrow {item_name}?"},
				{Text: "Hey, is {item_name} available?"},
				{Text: "I'd love to use your {item_name}."},
			},
		},
		{
			Phase: "reply",
			Messages: []ChatTemplate{
				{Text: "Sure, it's available!"},
				{Text: "Yes, come grab it."},
				{Text: "Absolutely, when do you need it?"},
			},
		},
	}

	cc := chatContext{ItemName: "Drill"}
	senderRoles := map[string]string{
		"interest": "bob@example.com",
		"reply":    "alice@example.com",
	}

	// Same seed should produce same results.
	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))

	msgs1 := buildConversation(rng1, phases, []string{"interest", "reply"}, cc, senderRoles, 4)
	msgs2 := buildConversation(rng2, phases, []string{"interest", "reply"}, cc, senderRoles, 4)

	if len(msgs1) != len(msgs2) {
		t.Fatalf("Different message counts: %d vs %d", len(msgs1), len(msgs2))
	}
	for i := range msgs1 {
		if msgs1[i].Text != msgs2[i].Text {
			t.Errorf("Message[%d] differs: %q vs %q", i, msgs1[i].Text, msgs2[i].Text)
		}
		if msgs1[i].SenderEmail != msgs2[i].SenderEmail {
			t.Errorf("Sender[%d] differs: %q vs %q", i, msgs1[i].SenderEmail, msgs2[i].SenderEmail)
		}
	}
}

func TestBuildConversationMaxMessages(t *testing.T) {
	phases := []ChatPhase{
		{
			Phase: "a",
			Messages: []ChatTemplate{
				{Text: "msg a1"},
				{Text: "msg a2"},
				{Text: "msg a3"},
			},
		},
		{
			Phase: "b",
			Messages: []ChatTemplate{
				{Text: "msg b1"},
				{Text: "msg b2"},
				{Text: "msg b3"},
			},
		},
		{
			Phase: "c",
			Messages: []ChatTemplate{
				{Text: "msg c1"},
				{Text: "msg c2"},
				{Text: "msg c3"},
			},
		},
	}

	rng := rand.New(rand.NewSource(99))
	senderRoles := map[string]string{
		"a": "user@example.com",
		"b": "user@example.com",
		"c": "user@example.com",
	}

	msgs := buildConversation(rng, phases, []string{"a", "b", "c"}, chatContext{}, senderRoles, 2)
	if len(msgs) > 2 {
		t.Errorf("Expected at most 2 messages, got %d", len(msgs))
	}
}

func TestBuildConversationMissingSender(t *testing.T) {
	phases := []ChatPhase{
		{
			Phase:    "test",
			Messages: []ChatTemplate{{Text: "hello"}},
		},
	}

	rng := rand.New(rand.NewSource(1))
	// Empty sender role should skip the phase.
	senderRoles := map[string]string{
		"test": "",
	}

	msgs := buildConversation(rng, phases, []string{"test"}, chatContext{}, senderRoles, 4)
	if len(msgs) != 0 {
		t.Errorf("Expected 0 messages with empty sender, got %d", len(msgs))
	}
}

func TestBuildConversationMissingPhase(t *testing.T) {
	phases := []ChatPhase{
		{
			Phase:    "exists",
			Messages: []ChatTemplate{{Text: "hello"}},
		},
	}

	rng := rand.New(rand.NewSource(1))
	senderRoles := map[string]string{
		"missing": "user@example.com",
		"exists":  "user@example.com",
	}

	msgs := buildConversation(rng, phases, []string{"missing", "exists"}, chatContext{}, senderRoles, 4)
	// Should skip the missing phase and include the existing one.
	if len(msgs) == 0 {
		t.Error("Expected at least 1 message from the 'exists' phase")
	}
	for _, msg := range msgs {
		if strings.Contains(msg.Text, "missing") {
			t.Error("Got message from missing phase")
		}
	}
}

func TestBuildConversationPlaceholdersExpanded(t *testing.T) {
	phases := []ChatPhase{
		{
			Phase: "test",
			Messages: []ChatTemplate{
				{Text: "Hi {owner_name}, can I borrow {item_name}?"},
			},
		},
	}

	cc := chatContext{
		ItemName:  "Table Saw",
		OwnerName: "Frank",
	}
	senderRoles := map[string]string{"test": "bob@example.com"}

	rng := rand.New(rand.NewSource(1))
	msgs := buildConversation(rng, phases, []string{"test"}, cc, senderRoles, 4)

	if len(msgs) == 0 {
		t.Fatal("Expected at least 1 message")
	}
	if msgs[0].Text != "Hi Frank, can I borrow Table Saw?" {
		t.Errorf("Template not expanded: %q", msgs[0].Text)
	}
}
