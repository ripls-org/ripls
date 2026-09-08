package portfolio

import (
	"testing"
)

func TestDefaultGoals(t *testing.T) {
	// Verify the system default constants are reasonable.
	if defaultSocialTimeGoalMinutes <= 0 {
		t.Errorf("defaultSocialTimeGoalMinutes = %d, want > 0", defaultSocialTimeGoalMinutes)
	}
	if defaultMoneySavedGoalCents <= 0 {
		t.Errorf("defaultMoneySavedGoalCents = %d, want > 0", defaultMoneySavedGoalCents)
	}
	if defaultCO2AvoidedGoalGrams <= 0 {
		t.Errorf("defaultCO2AvoidedGoalGrams = %d, want > 0", defaultCO2AvoidedGoalGrams)
	}
	// Social time goal should be 360 minutes (6 hours) per week.
	if defaultSocialTimeGoalMinutes != 360 {
		t.Errorf("defaultSocialTimeGoalMinutes = %d, want 360", defaultSocialTimeGoalMinutes)
	}
	// Money saved default: $500 = 50000 cents.
	if defaultMoneySavedGoalCents != 50_000 {
		t.Errorf("defaultMoneySavedGoalCents = %d, want 50000", defaultMoneySavedGoalCents)
	}
	// CO2 default: 30 kg = 30000 g.
	if defaultCO2AvoidedGoalGrams != 30_000 {
		t.Errorf("defaultCO2AvoidedGoalGrams = %d, want 30000", defaultCO2AvoidedGoalGrams)
	}
}
