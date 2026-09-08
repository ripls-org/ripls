package momentum

import "testing"

func TestValidateLeverCopy_PassingExamples(t *testing.T) {
	cases := []string{
		"Schedule round 6",
		"Bring Sunday brunch back",
		"Invite Mike to host next",
		"Share the pressure washer",
		"Offer the drill to the circle",
		"Create an event for the group",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			if err := ValidateLeverCopy(tc); err != nil {
				t.Errorf("expected pass, got error: %v", err)
			}
		})
	}
}

func TestValidateLeverCopy_TooManyWords(t *testing.T) {
	tooLong := "Schedule the next round of the smoker series for Saturday morning"
	if err := ValidateLeverCopy(tooLong); err == nil {
		t.Errorf("expected too-many-words error for %q, got nil", tooLong)
	}
}

func TestValidateLeverCopy_EmptyFails(t *testing.T) {
	for _, tc := range []string{"", "   "} {
		if err := ValidateLeverCopy(tc); err == nil {
			t.Errorf("expected error for empty lever %q, got nil", tc)
		}
	}
}
