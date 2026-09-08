package simulation

import (
	"context"
	"fmt"
	"math/rand"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// executeCheckFeed simulates a user checking their community feed.
// Calls GetFeed, then optionally MarkFeedItemsViewed.
func executeCheckFeed(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	resp, err := client.Feed().GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	}))
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	// Mark some items as viewed.
	var itemIDs []string
	for _, item := range resp.Msg.Items {
		if item.Id != "" {
			itemIDs = append(itemIDs, item.Id)
		}
	}
	if len(itemIDs) > 0 {
		_, err = client.Feed().MarkFeedItemsViewed(ctx, connect.NewRequest(&api.MarkFeedItemsViewedRequest{
			CommunityId: communityID,
			ItemIds:     itemIDs,
		}))
		if err != nil {
			// Non-fatal: marking as viewed is a side effect.
			return fmt.Errorf("mark feed items viewed: %w", err)
		}
	}

	// 20% chance: request page 2.
	if rng.Float64() < 0.20 && resp.Msg.NextPageToken != "" {
		_, err = client.Feed().GetFeed(ctx, connect.NewRequest(&api.GetFeedRequest{
			CommunityIds: []string{communityID},
			PageSize:     20,
			PageToken:    resp.Msg.NextPageToken,
		}))
		if err != nil {
			return fmt.Errorf("get feed page 2: %w", err)
		}
	}

	return nil
}

// executeSearchCommunity simulates a user searching within their community.
// Calls Search, then optionally follows up with a detail view on a result.
func executeSearchCommunity(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand, queryPool []string) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	if len(queryPool) == 0 {
		return fmt.Errorf("empty search query pool")
	}
	query := queryPool[rng.Intn(len(queryPool))]

	resp, err := client.Search().Search(ctx, connect.NewRequest(&api.SearchRequest{
		Query:        query,
		CommunityIds: []string{communityID},
	}))
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}

	// 30% chance: follow up with a detail view on a result.
	if rng.Float64() < 0.30 && len(resp.Msg.Results) > 0 {
		result := resp.Msg.Results[rng.Intn(len(resp.Msg.Results))]
		if gear := result.GetGear(); gear != nil && gear.Id != "" {
			_, err = client.Gear().GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
				Id:          gear.Id,
				CommunityId: communityID,
			}))
			if err != nil {
				return fmt.Errorf("get gear from search result: %w", err)
			}
		}
	}

	return nil
}

// executeBrowseGear simulates browsing community gear listings.
// Calls ListCommunityGear, then optionally GetGear and GetGearPeople.
func executeBrowseGear(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	resp, err := client.Community().ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		return fmt.Errorf("list community gear: %w", err)
	}

	// 40% chance: view a random gear item detail.
	if rng.Float64() < 0.40 && len(resp.Msg.GearItems) > 0 {
		gear := resp.Msg.GearItems[rng.Intn(len(resp.Msg.GearItems))]
		_, err = client.Gear().GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id:          gear.Id,
			CommunityId: communityID,
		}))
		if err != nil {
			return fmt.Errorf("get gear detail: %w", err)
		}

		// 20% chance: also view gear people.
		if rng.Float64() < 0.20 {
			_, err = client.Gear().GetGearPeople(ctx, connect.NewRequest(&api.GetGearPeopleRequest{
				GearId:      gear.Id,
				CommunityId: communityID,
			}))
			if err != nil {
				return fmt.Errorf("get gear people: %w", err)
			}
		}
	}

	return nil
}

// executeViewProfile simulates viewing another user's profile.
// Calls GetUser, then optionally GetUserStats.
func executeViewProfile(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) error {
	// Pick a random community member (not self).
	members := state.CommunityMembers[step.CommunityName]
	if len(members) < 2 {
		return fmt.Errorf("not enough members to view a profile")
	}

	var targetEmail string
	for _, idx := range rng.Perm(len(members)) {
		if members[idx] != step.Actor {
			targetEmail = members[idx]
			break
		}
	}
	if targetEmail == "" {
		return fmt.Errorf("no other member found")
	}

	targetID := state.UserIDs[targetEmail]
	if targetID == "" {
		return fmt.Errorf("no user ID for %q", targetEmail)
	}

	_, err := client.User().GetUser(ctx, connect.NewRequest(&api.GetUserRequest{
		UserId: targetID,
	}))
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// 30% chance: also view user stats.
	if rng.Float64() < 0.30 {
		_, err = client.User().GetUserStats(ctx, connect.NewRequest(&api.GetUserStatsRequest{
			UserId: targetID,
		}))
		if err != nil {
			return fmt.Errorf("get user stats: %w", err)
		}
	}

	return nil
}

// executeCheckStories simulates viewing community stories.
// Calls ListStories.
func executeCheckStories(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	_, err := client.Feed().ListStories(ctx, connect.NewRequest(&api.ListStoriesRequest{
		CommunityId: communityID,
		Limit:       20,
	}))
	if err != nil {
		return fmt.Errorf("list stories: %w", err)
	}

	return nil
}

// executeCheckConversations simulates checking chat conversations.
// Calls ListConversations, GetUnreadCounts, and optionally GetConversationHistory.
func executeCheckConversations(ctx context.Context, client *Client, rng *rand.Rand) error {
	listResp, err := client.Chat().ListConversations(ctx, connect.NewRequest(&api.ListConversationsRequest{}))
	if err != nil {
		return fmt.Errorf("list conversations: %w", err)
	}

	_, err = client.Chat().GetUnreadCounts(ctx, connect.NewRequest(&api.GetUnreadCountsRequest{}))
	if err != nil {
		return fmt.Errorf("get unread counts: %w", err)
	}

	// If there are conversations, view a random one.
	if rng.Float64() < 0.40 && len(listResp.Msg.Conversations) > 0 {
		conv := listResp.Msg.Conversations[rng.Intn(len(listResp.Msg.Conversations))]
		if conv.ConversationId != "" {
			_, err = client.Chat().GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
				ConversationId: conv.ConversationId,
				MaxMessages:    50,
			}))
			if err != nil {
				return fmt.Errorf("get conversation history: %w", err)
			}
		}
	}

	return nil
}

// executeViewImpact simulates viewing community impact metrics.
// Calls GetCommunityImpactMetrics, then optionally GetUserImpactMetrics.
func executeViewImpact(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	_, err := client.Impact().GetCommunityImpactMetrics(ctx, connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		return fmt.Errorf("get community impact metrics: %w", err)
	}

	// 20% chance: also view own impact metrics.
	if rng.Float64() < 0.20 {
		userID := state.UserIDs[step.Actor]
		if userID != "" {
			_, err = client.Impact().GetUserImpactMetrics(ctx, connect.NewRequest(&api.GetUserImpactMetricsRequest{
				UserId: userID,
			}))
			if err != nil {
				return fmt.Errorf("get user impact metrics: %w", err)
			}
		}
	}

	return nil
}

// injectContextualReads simulates the read RPCs a user would make before
// performing a write action (e.g., viewing a gear item before expressing
// interest). Returns the number of reads executed; errors are non-fatal.
func injectContextualReads(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) int {
	reads := 0

	switch step.Action {
	case ActionExpressInterest, ActionExpressInterestGiveaway:
		// User views the gear item before expressing interest.
		if rng.Float64() < 0.80 {
			gearRef := extractGearRef(step.Ref)
			gearID := state.GearIDs[gearRef]
			communityID := state.CommunityIDs[step.CommunityName]
			if gearID != "" && communityID != "" {
				_, err := client.Gear().GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
					Id:          gearID,
					CommunityId: communityID,
				}))
				if err == nil {
					reads++
					// Also view gear people.
					_, err = client.Gear().GetGearPeople(ctx, connect.NewRequest(&api.GetGearPeopleRequest{
						GearId:      gearID,
						CommunityId: communityID,
					}))
					if err == nil {
						reads++
					}
				}
			}
		}

	case ActionOfferToFulfill:
		// User views the request before offering help.
		if rng.Float64() < 0.80 {
			requestID := state.RequestIDs[step.Ref]
			communityID := state.CommunityIDs[step.CommunityName]
			if requestID != "" {
				_, err := client.Request().GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
					RequestId: requestID,
				}))
				if err == nil {
					reads++
				}
				if communityID != "" {
					_, err = client.Request().GetRequestPeople(ctx, connect.NewRequest(&api.GetRequestPeopleRequest{
						RequestId:   requestID,
						CommunityId: communityID,
					}))
					if err == nil {
						reads++
					}
				}
			}
		}

	case ActionRSVP, ActionRSVPNo:
		// User views the experience before RSVPing.
		if rng.Float64() < 0.80 {
			experienceID := state.ExperienceIDs[step.Ref]
			communityID := state.CommunityIDs[step.CommunityName]
			if experienceID != "" {
				_, err := client.Experience().GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
					Id:          experienceID,
					CommunityId: communityID,
				}))
				if err == nil {
					reads++
				}
				if communityID != "" {
					_, err = client.Experience().GetExperiencePeople(ctx, connect.NewRequest(&api.GetExperiencePeopleRequest{
						ExperienceId: experienceID,
						CommunityId:  communityID,
					}))
					if err == nil {
						reads++
					}
				}
			}
		}

	case ActionStartLoan:
		// User checks transfer and conversation before starting loan.
		if rng.Float64() < 0.60 {
			transferID := state.TransferIDs[step.Ref]
			if transferID != "" {
				_, err := client.Transfer().GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{
					TransferId: transferID,
				}))
				if err == nil {
					reads++
				}
				// View conversation for the transfer.
				convResp, err := client.Chat().GetConversationForTransfer(ctx, connect.NewRequest(&api.GetConversationForTransferRequest{
					TransferId: transferID,
				}))
				if err == nil {
					reads++
					if convResp.Msg.Conversation != nil && convResp.Msg.Conversation.ConversationId != "" {
						_, err = client.Chat().GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
							ConversationId: convResp.Msg.Conversation.ConversationId,
							MaxMessages:    50,
						}))
						if err == nil {
							reads++
						}
					}
				}
			}
		}

	case ActionCompleteLoan, ActionCompleteGiveaway:
		// User checks transfer and gear before completing.
		if rng.Float64() < 0.60 {
			transferID := state.TransferIDs[step.Ref]
			if transferID != "" {
				_, err := client.Transfer().GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{
					TransferId: transferID,
				}))
				if err == nil {
					reads++
				}
			}
			gearRef := extractGearRef(step.Ref)
			gearID := state.GearIDs[gearRef]
			communityID := state.CommunityIDs[step.CommunityName]
			if gearID != "" && communityID != "" {
				_, err := client.Gear().GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
					Id:          gearID,
					CommunityId: communityID,
				}))
				if err == nil {
					reads++
				}
			}
		}

	case ActionFulfillRequest:
		// User checks request before marking fulfilled.
		if rng.Float64() < 0.60 {
			requestID := state.RequestIDs[step.Ref]
			if requestID != "" {
				_, err := client.Request().GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
					RequestId: requestID,
				}))
				if err == nil {
					reads++
				}
			}
		}

	case ActionCompleteExperience:
		// Host checks experience and attendees before completing.
		if rng.Float64() < 0.60 {
			experienceID := state.ExperienceIDs[step.Ref]
			communityID := state.CommunityIDs[step.CommunityName]
			if experienceID != "" {
				_, err := client.Experience().GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{
					Id:          experienceID,
					CommunityId: communityID,
				}))
				if err == nil {
					reads++
				}
				if communityID != "" {
					_, err = client.Experience().GetExperiencePeople(ctx, connect.NewRequest(&api.GetExperiencePeopleRequest{
						ExperienceId: experienceID,
						CommunityId:  communityID,
					}))
					if err == nil {
						reads++
					}
				}
			}
		}
	}

	return reads
}
