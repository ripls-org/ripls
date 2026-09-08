package ai

import "testing"

func TestDescriptionWithoutTitleClause(t *testing.T) {
	tests := []struct {
		name        string
		description string
		title       string
		want        string
	}{
		{
			name:        "strips the duplicated opening clause",
			description: "Mother's Day Brunch. At our place — the kids are cooking, bring the whole crew!",
			title:       "Mother's Day Brunch",
			want:        "At our place — the kids are cooking, bring the whole crew!",
		},
		{
			name:        "case-insensitive match",
			description: "mother's day brunch — at our place, the kids are cooking!",
			title:       "Mother's Day Brunch",
			want:        "at our place, the kids are cooking!",
		},
		{
			name:        "title the AI invented rather than quoted is left alone",
			description: "Ours died halfway through the front yard. One afternoon is all I need.",
			title:       "A lawn mower for the weekend",
			want:        "Ours died halfway through the front yard. One afternoon is all I need.",
		},
		{
			name:        "a title that is only a word prefix does not truncate the sentence",
			description: "Mower repair help — the pull cord snapped and I am out of ideas.",
			title:       "Mower",
			want:        "Mower repair help — the pull cord snapped and I am out of ideas.",
		},
		{
			name:        "keeps the prompt when stripping would leave nothing useful",
			description: "Board game night. Friday!",
			title:       "Board game night",
			want:        "Board game night. Friday!",
		},
		{
			name:        "description equal to the title is untouched",
			description: "Board game night",
			title:       "Board game night",
			want:        "Board game night",
		},
		{
			name:        "empty title is a no-op",
			description: "Anything at all here",
			title:       "",
			want:        "Anything at all here",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DescriptionWithoutTitleClause(tc.description, tc.title); got != tc.want {
				t.Errorf("DescriptionWithoutTitleClause(%q, %q) = %q, want %q",
					tc.description, tc.title, got, tc.want)
			}
		})
	}
}
