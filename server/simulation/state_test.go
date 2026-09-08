package simulation

import "testing"

func TestNewState(t *testing.T) {
	state := NewState()

	if state.UserTokens == nil {
		t.Error("UserTokens should be initialized")
	}
	if state.UserIDs == nil {
		t.Error("UserIDs should be initialized")
	}
	if state.CommunityIDs == nil {
		t.Error("CommunityIDs should be initialized")
	}

	// Verify all maps are initialized by writing to each one.
	state.UserTokens["k"] = "v"
	state.UserIDs["k"] = "v"
	state.CommunityIDs["k"] = "v"
	state.GearIDs["k"] = "v"
	state.GearTemplates["k"] = GearTemplate{Name: "test"}
	state.GearOwners["k"] = "v"
	state.LocationIDs["k"] = "v"
	state.TransferIDs["k"] = "v"
	state.RequestIDs["k"] = "v"
	state.ExperienceIDs["k"] = "v"
	state.InterestedUsers["k"] = []string{"a"}
	state.HelperUsers["k"] = []string{"b"}
	state.RequestOwners["k"] = "v"
	state.ExperienceOwners["k"] = "v"
	state.MediaIDs["k"] = "v"
	state.ConversationIDs["k"] = "v"
	state.UserNames["k"] = "v"
	state.CommunityMembers["k"] = []string{"c"}
	state.CompletedRefs["k"] = true
	state.FailedRefs["k"] = true
}
