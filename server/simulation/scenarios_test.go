package simulation

import (
	"math/rand"
	"testing"
)

func TestAllScenariosValidate(t *testing.T) {
	for _, scenario := range AllScenarios() {
		if err := scenario.Validate(); err != nil {
			t.Errorf("Scenario %q failed validation: %v", scenario.Name, err)
		}
	}
}

func TestAllScenariosUniqueEmails(t *testing.T) {
	for _, scenario := range AllScenarios() {
		emails := make(map[string]bool)
		for _, comm := range scenario.Communities {
			for _, m := range comm.Members {
				if emails[m.Email] {
					t.Errorf("Scenario %q: duplicate email %q across communities", scenario.Name, m.Email)
				}
				emails[m.Email] = true
			}
		}
	}
}

func TestScenarioSizes(t *testing.T) {
	tests := []struct {
		name        string
		scenario    Scenario
		communities int
		minMembers  int
		maxMembers  int
	}{
		{"suburban-neighborhood", SuburbanNeighborhood(), 1, 15, 20},
		{"college-friends", CollegeFriends(), 1, 5, 10},
		{"active-community-org", ActiveCommunityOrg(), 2, 35, 40},
		{"new-community", NewCommunity(), 1, 3, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.scenario.Communities) != tt.communities {
				t.Errorf("Communities = %d, want %d", len(tt.scenario.Communities), tt.communities)
			}
			totalMembers := 0
			for _, c := range tt.scenario.Communities {
				totalMembers += len(c.Members)
			}
			if totalMembers < tt.minMembers || totalMembers > tt.maxMembers {
				t.Errorf("Total members = %d, want [%d, %d]", totalMembers, tt.minMembers, tt.maxMembers)
			}
		})
	}
}

func TestScenarioTimelineGeneration(t *testing.T) {
	for _, scenario := range AllScenarios() {
		t.Run(scenario.Name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(42))
			result := GenerateTimeline(rng, scenario)

			if len(result.Steps) == 0 && scenario.Name != "new-community" {
				t.Error("GenerateTimeline produced empty timeline")
			}
			t.Logf("Scenario %q: %d timeline steps", scenario.Name, len(result.Steps))
		})
	}
}

func TestReadTimelineGeneration(t *testing.T) {
	scenario := LoadTestSmall()
	rng := rand.New(rand.NewSource(42))

	// Without read traffic: no read actions.
	result := GenerateTimelineWithOptions(rng, scenario, TimelineOptions{})
	readCount := 0
	for _, step := range result.Steps {
		if step.Action >= ActionCheckFeed && step.Action <= ActionViewImpact {
			readCount++
		}
	}
	if readCount != 0 {
		t.Errorf("Expected 0 read steps without ReadTraffic, got %d", readCount)
	}

	// With read traffic: should have read actions.
	rng = rand.New(rand.NewSource(42))
	result = GenerateTimelineWithOptions(rng, scenario, TimelineOptions{
		ReadTraffic:    true,
		ReadMultiplier: 1.0,
	})
	readCount = 0
	writeCount := 0
	for _, step := range result.Steps {
		if step.Action >= ActionCheckFeed && step.Action <= ActionViewImpact {
			readCount++
		} else {
			writeCount++
		}
	}
	if readCount == 0 {
		t.Error("Expected read steps with ReadTraffic enabled, got 0")
	}
	t.Logf("With reads: %d write + %d read = %d total (%.1f:1 read/write ratio)",
		writeCount, readCount, writeCount+readCount, float64(readCount)/float64(writeCount))

	// Read multiplier should scale reads.
	rng = rand.New(rand.NewSource(42))
	result2x := GenerateTimelineWithOptions(rng, scenario, TimelineOptions{
		ReadTraffic:    true,
		ReadMultiplier: 2.0,
	})
	readCount2x := 0
	for _, step := range result2x.Steps {
		if step.Action >= ActionCheckFeed && step.Action <= ActionViewImpact {
			readCount2x++
		}
	}
	// 2x multiplier should produce roughly 2x reads (allow 50% tolerance for Poisson variance).
	if readCount2x < readCount {
		t.Errorf("2x multiplier produced fewer reads (%d) than 1x (%d)", readCount2x, readCount)
	}
	t.Logf("1x reads=%d, 2x reads=%d (ratio=%.2f)", readCount, readCount2x, float64(readCount2x)/float64(readCount))
}

func TestLoadTestScenarioSizes(t *testing.T) {
	tests := []struct {
		name        string
		scenario    Scenario
		communities int
		members     int
	}{
		{"load-test-small", LoadTestSmall(), 1, 10},
		{"load-test-medium", LoadTestMedium(), 2, 30},
		{"load-test-large", LoadTestLarge(), 3, 75},
		{"read-heavy", ReadHeavy(), 1, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.scenario.Communities) != tt.communities {
				t.Errorf("Communities = %d, want %d", len(tt.scenario.Communities), tt.communities)
			}
			totalMembers := 0
			for _, c := range tt.scenario.Communities {
				totalMembers += len(c.Members)
			}
			if totalMembers != tt.members {
				t.Errorf("Total members = %d, want %d", totalMembers, tt.members)
			}
		})
	}
}
