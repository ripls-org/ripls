package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// executeTimeline runs all activity steps in order against the server.
// Steps whose refs are already completed or failed are skipped to avoid
// nonsense actions (e.g., selecting a recipient for a finished giveaway).
//
// Each step is dispatched on the per-user client from the pool, so there
// is no shared mutable auth state — cross-user side operations like the
// giveaway's owner-side re-share cannot leak their identity into the
// step's primary RPC.
func executeTimeline(ctx context.Context, serverURL string, state *State, timeline Timeline, rng *rand.Rand, readTraffic bool) error {
	experiences, _ := LoadExperienceTemplates()
	requests, _ := LoadRequestTemplates()
	venues, _ := LoadVenues()
	chatTemplates, _ := LoadChatTemplates()
	queryPool := SearchQueryPool()

	expIdx := 0
	reqIdx := 0

	totalSteps := len(timeline)
	executed := 0

	contextExpired := false

	pool := NewClientPool(serverURL, state.UserTokens)
	getClient := pool.For

	for i, step := range timeline {
		// If the parent context has expired (timeout or Ctrl+C), skip remaining steps.
		if ctx.Err() != nil {
			if !contextExpired {
				contextExpired = true
				remaining := totalSteps - i
				slog.Warn("context expired, skipping remaining steps",
					"reason", ctx.Err(),
					"remaining", remaining,
					"executed", executed,
					"failed", state.StepsFailed,
				)
			}
			state.StepsSkipped++
			continue
		}

		// Skip steps for flows that already completed or whose initiating step failed.
		if step.Action != ActionSaveGear {
			if state.CompletedRefs[step.Ref] || state.FailedRefs[step.Ref] {
				state.StepsSkipped++
				continue
			}
		}

		// Use a per-step context so a single slow request doesn't exhaust the
		// parent timeout and cause all subsequent steps to fail immediately.
		stepCtx, stepCancel := context.WithTimeout(ctx, 30*time.Second)

		// Every client in the pool carries the same simulated timestamp —
		// cross-user side operations within a single step (owner-side re-share,
		// chat injection) must all appear at step.Time on the wire.
		pool.SetTimestamp(step.Time)
		client := pool.For(step.Actor)
		if client == nil {
			slog.Warn("no client for step actor",
				"step", i,
				"actor", step.Actor,
			)
			stepCancel()
			state.StepsSkipped++
			continue
		}

		// Inject contextual reads before write actions (simulates user viewing
		// the item detail page before acting on it).
		if readTraffic {
			reads := injectContextualReads(stepCtx, client, state, step, rng)
			state.ReadsExecuted += reads
		}

		var err error
		switch step.Action {
		case ActionSaveGear:
			err = executeSaveGear(stepCtx, client, state, step, rng)
		case ActionExpressInterest:
			err = executeExpressInterest(stepCtx, client, state, step, false, getClient)
		case ActionSelectRecipient:
			err = executeSelectRecipient(stepCtx, client, state, step)
		case ActionStartLoan:
			err = executeStartLoan(stepCtx, client, state, step)
		case ActionCompleteLoan:
			err = executeCompleteTransfer(stepCtx, client, state, step)
		case ActionExpressInterestGiveaway:
			err = executeExpressInterest(stepCtx, client, state, step, true, getClient)
		case ActionSelectGiveaway:
			err = executeSelectRecipient(stepCtx, client, state, step)
		case ActionCompleteGiveaway:
			err = executeCompleteTransfer(stepCtx, client, state, step)
		case ActionSubmitRequest:
			err = executeSubmitRequest(stepCtx, client, state, step, requests, &reqIdx)
		case ActionOfferToFulfill:
			err = executeOfferToFulfill(stepCtx, client, state, step)
		case ActionFulfillRequest:
			err = executeFulfillRequest(stepCtx, client, state, step)
		case ActionSaveExperience:
			err = executeSaveExperience(stepCtx, client, state, step, experiences, &expIdx, rng, venues)
		case ActionShareExperience:
			err = executeShareExperience(stepCtx, client, state, step)
		case ActionRSVP:
			err = executeRSVP(stepCtx, client, state, step, false)
		case ActionRSVPNo:
			err = executeRSVP(stepCtx, client, state, step, true)
		case ActionStartExperience:
			err = executeStartExperience(stepCtx, client, state, step)
		case ActionCompleteExperience:
			err = executeCompleteExperience(stepCtx, client, state, step)
		case ActionCancelExperience:
			err = executeCancelExperience(stepCtx, client, state, step)
		case ActionWithdrawInterest:
			err = executeWithdrawInterest(stepCtx, client, state, step)
		case ActionCancelTransfer:
			err = executeCancelTransfer(stepCtx, client, state, step)
		case ActionCancelRequest:
			err = executeCancelRequest(stepCtx, client, state, step)
		case ActionWithdrawOffer:
			err = executeWithdrawOffer(stepCtx, client, state, step)
		case ActionCheckFeed:
			err = executeCheckFeed(stepCtx, client, state, step, rng)
		case ActionSearchCommunity:
			err = executeSearchCommunity(stepCtx, client, state, step, rng, queryPool)
		case ActionBrowseGear:
			err = executeBrowseGear(stepCtx, client, state, step, rng)
		case ActionViewProfile:
			err = executeViewProfile(stepCtx, client, state, step, rng)
		case ActionCheckStories:
			err = executeCheckStories(stepCtx, client, state, step)
		case ActionCheckConversations:
			err = executeCheckConversations(stepCtx, client, rng)
		case ActionViewImpact:
			err = executeViewImpact(stepCtx, client, state, step, rng)
		}

		// Increment read counter for standalone read actions.
		if err == nil && isReadAction(step.Action) {
			state.ReadsExecuted++
		}

		if err != nil {
			state.StepsFailed++
			// Mark the ref as failed for critical steps (flow-blocking failures).
			// Non-critical steps like RSVPs and offers can fail independently
			// without poisoning the entire flow.
			if isCriticalAction(step.Action) {
				state.FailedRefs[step.Ref] = true
			}
			slog.Warn("activity step failed",
				"step", i,
				"action", step.Action,
				"actor", step.Actor,
				"ref", step.Ref,
				"time", step.Time.Format("2006-01-02"),
				"error", err,
			)
		}

		// Send supplemental chat messages after successful actions.
		if err == nil && chatTemplates != nil {
			injectChat(stepCtx, getClient, state, rng, chatTemplates, step)
		}
		stepCancel()

		executed++
		if executed%100 == 0 {
			slog.Info("simulation progress",
				"step", i+1,
				"total", totalSteps,
				"executed", executed,
				"skipped", state.StepsSkipped,
				"failed", state.StepsFailed,
				"sim_date", step.Time.Format("2006-01-02"),
			)
		}

		// Mark terminal actions so late-arriving steps are skipped.
		if isTerminalAction(step.Action) && err == nil {
			state.CompletedRefs[step.Ref] = true
		}
	}

	return nil
}

// injectChat sends supplemental chat messages after a successful executor step.
// Probability thresholds control how often chat is sent for each action type.
// The getClient function returns a client authenticated as the given user.
func injectChat(ctx context.Context, getClient func(email string) *Client, state *State, rng *rand.Rand, templates *ChatTemplateSet, step ActivityStep) {
	deadline := step.NextSiblingTime
	switch step.Action {
	case ActionExpressInterest:
		if rng.Float64() < 0.7 {
			sendLoanChat(ctx, getClient, state, rng, templates, step.Ref, []string{"interest", "owner_reply"}, step.Time, deadline)
		}
	case ActionStartLoan:
		if rng.Float64() < 0.5 {
			sendLoanChat(ctx, getClient, state, rng, templates, step.Ref, []string{"logistics", "logistics_confirm", "thanks"}, step.Time, deadline)
		}
	case ActionExpressInterestGiveaway:
		if rng.Float64() < 0.7 {
			sendGiveawayChat(ctx, getClient, state, rng, templates, step.Ref, []string{"interest", "owner_reply"}, step.Time, deadline)
		}
	case ActionSelectGiveaway:
		if rng.Float64() < 0.6 {
			sendGiveawayChat(ctx, getClient, state, rng, templates, step.Ref, []string{"logistics", "thanks"}, step.Time, deadline)
		}
	case ActionOfferToFulfill:
		if rng.Float64() < 0.6 {
			requester := state.RequestOwners[step.Ref]
			sendRequestChat(ctx, getClient, state, rng, templates, step.Ref, requester, []string{"clarification", "requester_reply", "offer"}, step.Time, deadline)
		}
	case ActionFulfillRequest:
		if rng.Float64() < 0.7 {
			requester := state.RequestOwners[step.Ref]
			sendRequestChat(ctx, getClient, state, rng, templates, step.Ref, requester, []string{"planning", "requester_thanks"}, step.Time, deadline)
		}
	case ActionRSVP:
		if rng.Float64() < 0.6 {
			host := state.ExperienceOwners[step.Ref]
			phases := []string{"rsvp_excitement"}
			if rng.Float64() < 0.3 {
				phases = append(phases, "question", "host_reply")
			}
			if rng.Float64() < 0.15 {
				phases = append(phases, "mention_suggestion")
			}
			sendExperienceChat(ctx, getClient, state, rng, templates, step.Ref, host, step.Actor, step.CommunityName, phases, step.Time, deadline)
		}
	case ActionShareExperience:
		if rng.Float64() < 0.3 {
			host := state.ExperienceOwners[step.Ref]
			// Pick a random community member as the "questioner".
			var attendeeEmail string
			if members, ok := state.CommunityMembers[step.CommunityName]; ok {
				for _, tries := range rng.Perm(len(members)) {
					email := members[tries]
					if email != host {
						attendeeEmail = email
						break
					}
				}
			}
			if attendeeEmail != "" {
				sendExperienceChat(ctx, getClient, state, rng, templates, step.Ref, host, attendeeEmail, step.CommunityName, []string{"question", "host_reply"}, step.Time, deadline)
			}
		}
	}
}

// isCriticalAction returns true for actions whose failure should prevent all
// subsequent steps on the same ref. Non-critical actions like RSVPs and offers
// can fail independently without blocking the rest of the flow.
func isCriticalAction(a Action) bool {
	switch a {
	case ActionRSVP, ActionRSVPNo, ActionOfferToFulfill,
		ActionWithdrawInterest, ActionWithdrawOffer,
		ActionCheckFeed, ActionSearchCommunity, ActionBrowseGear,
		ActionViewProfile, ActionCheckStories, ActionCheckConversations,
		ActionViewImpact:
		return false
	}
	return true
}

// isReadAction returns true for standalone read actions whose read counter
// must be incremented by the caller.
func isReadAction(a Action) bool {
	switch a {
	case ActionCheckFeed, ActionSearchCommunity, ActionBrowseGear,
		ActionViewProfile, ActionCheckStories, ActionCheckConversations,
		ActionViewImpact:
		return true
	}
	return false
}

// isTerminalAction returns true for actions that complete a flow.
// After these succeed, late-arriving steps for the same ref should be skipped.
func isTerminalAction(a Action) bool {
	switch a {
	case ActionCompleteLoan, ActionCompleteGiveaway,
		ActionCancelTransfer, ActionFulfillRequest,
		ActionCancelRequest, ActionCompleteExperience,
		ActionCancelExperience:
		return true
	}
	return false
}

// executeSaveGear creates a gear item and shares it in the community.
func executeSaveGear(ctx context.Context, client *Client, state *State, step ActivityStep, rng *rand.Rand) error {
	// Get the gear template assigned during timeline generation.
	tmpl, ok := state.GearTemplates[step.Ref]
	if !ok {
		return fmt.Errorf("no gear template for ref %q", step.Ref)
	}

	// Create a location for the gear (user's home).
	locationID, err := getOrCreateLocation(ctx, client, state, step.Actor, step.CommunityName, rng)
	if err != nil {
		return fmt.Errorf("create location: %w", err)
	}

	// Look up media for this gear item if available.
	var mediaIDs []string
	assetFile := GearAssetFilename(tmpl)
	if assetFile != "" {
		if mediaID, ok := state.MediaIDs["gear/"+assetFile]; ok {
			mediaIDs = append(mediaIDs, mediaID)
		}
	}

	// Save the gear with USER provenance on all tracked fields so the
	// server's backfill job won't attempt expensive GenAI enrichment.
	userProv := &api.Provenance{Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER}
	resp, err := client.Gear().SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        &tmpl.Name,
		Description: &tmpl.Description,
		LocationId:  &locationID,
		MediaIds:    mediaIDs,
		Metadata: &api.GearMetadata{
			Category:         &api.TrackedString{Value: tmpl.Category, Provenance: userProv},
			Brand:            &api.TrackedString{Value: tmpl.Brand, Provenance: userProv},
			Model:            &api.TrackedString{Value: tmpl.Model, Provenance: userProv},
			MaterialCategory: &api.TrackedMaterialCategory{Value: materialCategoryFromString(tmpl.MaterialCategory), Provenance: userProv},
			WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: float32(tmpl.WeightGrams), Stddev: float32(tmpl.WeightGrams) * 0.1}, Provenance: userProv},
			ValueEstimate:    &api.ValueEstimate{EstimatedValueUsd: float32(tmpl.ValueUSD), Provenance: userProv},
		},
	}))
	if err != nil {
		return fmt.Errorf("save gear %q: %w", tmpl.Name, err)
	}

	gearID := resp.Msg.Id
	state.GearIDs[step.Ref] = gearID
	state.GearOwners[step.Ref] = step.Actor
	state.GearCreated++

	// Share the gear in the community.
	communityID := state.CommunityIDs[step.CommunityName]
	// Gear is FOR_LOAN by default and availability is item-wide (#2492/#2687),
	// so the share carries no Lend/Give argument; executeExpressInterest
	// re-points it with SetGearAvailability when a giveaway is wanted.
	_, err = client.Community().ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_GearId{GearId: gearID},
		ShareToCommunityIds: []string{communityID},
	}))
	if err != nil {
		return fmt.Errorf("share gear %q: %w", tmpl.Name, err)
	}

	return nil
}

// executeExpressInterest expresses interest in a gear item.
// For giveaways, the gear must first be switched to FOR_GIVEAWAY since all gear
// starts as FOR_LOAN. This changes the transfer type the server creates.
// The getClient function returns a client authenticated as the given user,
// used to set availability as the gear owner during giveaway flows.
func executeExpressInterest(ctx context.Context, client *Client, state *State, step ActivityStep, isGiveaway bool, getClient func(email string) *Client) error {
	// Extract the base gear ref from the loan/giveaway ref.
	gearRef := extractGearRef(step.Ref)
	gearID, ok := state.GearIDs[gearRef]
	if !ok {
		return fmt.Errorf("no gear ID for ref %q (from %q)", gearRef, step.Ref)
	}

	// For giveaways, switch the gear to FOR_GIVEAWAY before expressing interest.
	// All gear starts as FOR_LOAN; giveaways require FOR_GIVEAWAY for the correct
	// transfer state machine (INTEREST_EXPRESSED instead of auto-RECIPIENT_SELECTED).
	// Availability is item-wide (#2492/#2687), so this applies across every
	// community the gear is shared with.
	if isGiveaway {
		ownerEmail, ok := state.GearOwners[gearRef]
		if !ok {
			return fmt.Errorf("no gear owner for ref %q", gearRef)
		}

		ownerClient := getClient(ownerEmail)
		if ownerClient == nil {
			return fmt.Errorf("no client for gear owner %q", ownerEmail)
		}

		_, err := ownerClient.Community().SetGearAvailability(ctx, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId:       gearID,
			Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		}))
		if err != nil {
			return fmt.Errorf("set gear availability to giveaway: %w", err)
		}
	}

	resp, err := client.Transfer().ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		return fmt.Errorf("express interest: %w", err)
	}

	// Store the transfer ID (first interest creates the transfer).
	if _, exists := state.TransferIDs[step.Ref]; !exists {
		state.TransferIDs[step.Ref] = resp.Msg.Transfer.Id
	}

	// Capture the conversation ID for chat messages.
	if resp.Msg.Transfer.ConversationId != nil && *resp.Msg.Transfer.ConversationId != "" {
		state.ConversationIDs[step.Ref] = *resp.Msg.Transfer.ConversationId
	}

	// Track interested users for recipient selection.
	state.InterestedUsers[step.Ref] = append(state.InterestedUsers[step.Ref], step.Actor)

	return nil
}

// executeSelectRecipient selects a recipient for a transfer.
func executeSelectRecipient(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	transferID, ok := state.TransferIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no transfer ID for ref %q", step.Ref)
	}

	// Select the first interested user as recipient.
	interested := state.InterestedUsers[step.Ref]
	if len(interested) == 0 {
		return fmt.Errorf("no interested users for ref %q", step.Ref)
	}

	recipientEmail := interested[0]
	recipientID, ok := state.UserIDs[recipientEmail]
	if !ok {
		return fmt.Errorf("no user ID for %q", recipientEmail)
	}

	resp, err := client.Transfer().SelectRecipient(ctx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipientID,
	}))
	if err != nil {
		return fmt.Errorf("select recipient: %w", err)
	}

	// Capture the 1:1 conversation ID (may differ from the gear conversation).
	if resp.Msg.ConversationId != "" {
		state.ConversationIDs[step.Ref] = resp.Msg.ConversationId
	}

	return nil
}

// executeStartLoan starts an active loan.
func executeStartLoan(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	transferID, ok := state.TransferIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no transfer ID for ref %q", step.Ref)
	}

	_, err := client.Transfer().StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{
		TransferId: transferID,
	}))
	if err != nil {
		return fmt.Errorf("start loan: %w", err)
	}

	state.TransfersStarted++
	return nil
}

// executeCompleteTransfer completes a transfer (loan return or giveaway).
func executeCompleteTransfer(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	transferID, ok := state.TransferIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no transfer ID for ref %q", step.Ref)
	}

	_, err := client.Transfer().CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transferID,
	}))
	if err != nil {
		return fmt.Errorf("complete transfer: %w", err)
	}

	state.TransfersCompleted++
	return nil
}

// executeSubmitRequest submits a community request.
func executeSubmitRequest(ctx context.Context, client *Client, state *State, step ActivityStep, templates []RequestTemplate, idx *int) error {
	communityID := state.CommunityIDs[step.CommunityName]
	if communityID == "" {
		return fmt.Errorf("no community ID for %q", step.CommunityName)
	}

	templateIdx := *idx % len(templates)
	tmpl := templates[templateIdx]
	*idx++

	// Look up media for this request if available.
	var mediaIDs []string
	assetFile := RequestAssetFilename(templateIdx)
	if mediaID, ok := state.MediaIDs["requests/"+assetFile]; ok {
		mediaIDs = append(mediaIDs, mediaID)
	}

	resp, err := client.Request().SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:       tmpl.Title,
		Description: tmpl.Description,
		MediaIds:    mediaIDs,
	}))
	if err != nil {
		return fmt.Errorf("submit request %q: %w", tmpl.Title, err)
	}

	// Share the request into the step's community (#2529: SubmitRequest no
	// longer takes a community; it lands only in its own per-item community).
	if _, err := client.Community().ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_RequestId{RequestId: resp.Msg.RequestId},
		ShareToCommunityIds: []string{communityID},
	})); err != nil {
		return fmt.Errorf("share request %q into %q: %w", tmpl.Title, step.CommunityName, err)
	}

	state.RequestIDs[step.Ref] = resp.Msg.RequestId
	state.RequestOwners[step.Ref] = step.Actor
	state.RequestsSubmitted++
	return nil
}

// executeOfferToFulfill offers to fulfill a request.
func executeOfferToFulfill(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	requestID, ok := state.RequestIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no request ID for ref %q", step.Ref)
	}

	communityID := state.CommunityIDs[step.CommunityName]
	resp, err := client.Request().OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		return fmt.Errorf("offer to fulfill: %w", err)
	}

	// Capture the conversation ID from the request object.
	if resp.Msg.Request != nil && resp.Msg.Request.ConversationId != nil && *resp.Msg.Request.ConversationId != "" {
		state.ConversationIDs[step.Ref] = *resp.Msg.Request.ConversationId
	}

	state.HelperUsers[step.Ref] = append(state.HelperUsers[step.Ref], step.Actor)
	return nil
}

// executeFulfillRequest marks a request as fulfilled.
func executeFulfillRequest(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	requestID, ok := state.RequestIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no request ID for ref %q", step.Ref)
	}

	// Confirm all helpers who offered.
	var helperIDs []string
	for _, email := range state.HelperUsers[step.Ref] {
		if id, ok := state.UserIDs[email]; ok {
			helperIDs = append(helperIDs, id)
		}
	}

	_, err := client.Request().MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: helperIDs,
	}))
	if err != nil {
		return fmt.Errorf("fulfill request: %w", err)
	}

	state.RequestsFulfilled++
	return nil
}

// executeSaveExperience creates a new experience.
func executeSaveExperience(ctx context.Context, client *Client, state *State, step ActivityStep, templates []ExperienceTemplate, idx *int, rng *rand.Rand, venues map[string][]Venue) error {
	templateIdx := *idx % len(templates)
	tmpl := templates[templateIdx]
	*idx++

	// Find a venue matching the experience's venue category.
	locationID, err := getOrCreateVenueLocation(ctx, client, state, step.CommunityName, tmpl.VenueCategory, rng, venues)
	if err != nil {
		return fmt.Errorf("create venue location: %w", err)
	}

	// Look up media for this experience if available.
	var mediaIDs []string
	assetFile := ExperienceAssetFilename(templateIdx)
	if mediaID, ok := state.MediaIDs["experiences/"+assetFile]; ok {
		mediaIDs = append(mediaIDs, mediaID)
	}

	resp, err := client.Experience().SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:            tmpl.Name,
		Description:     tmpl.Description,
		LocationId:      locationID,
		MaxParticipants: int32(tmpl.MaxParticipants),
		MediaIds:        mediaIDs,
	}))
	if err != nil {
		return fmt.Errorf("save experience %q: %w", tmpl.Name, err)
	}

	state.ExperienceIDs[step.Ref] = resp.Msg.Experience.Id
	state.ExperienceOwners[step.Ref] = step.Actor
	state.ExperiencesCreated++
	return nil
}

// executeShareExperience shares an experience with a community.
func executeShareExperience(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	experienceID, ok := state.ExperienceIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no experience ID for ref %q", step.Ref)
	}

	communityID := state.CommunityIDs[step.CommunityName]
	_, err := client.Community().ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_ExperienceId{ExperienceId: experienceID},
		ShareToCommunityIds: []string{communityID},
	}))
	if err != nil {
		return fmt.Errorf("share experience: %w", err)
	}

	return nil
}

// executeRSVP RSVPs to an experience with either YES or NO intention.
func executeRSVP(ctx context.Context, client *Client, state *State, step ActivityStep, decline bool) error {
	experienceID, ok := state.ExperienceIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no experience ID for ref %q", step.Ref)
	}

	intention := api.RSVPIntention_RSVP_INTENTION_YES
	if decline {
		intention = api.RSVPIntention_RSVP_INTENTION_NO
	}

	communityID := state.CommunityIDs[step.CommunityName]
	resp, err := client.Experience().RSVPToExperience(ctx, connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: experienceID,
		CommunityId:  communityID,
		Intention:    intention,
	}))
	if err != nil {
		return fmt.Errorf("RSVP: %w", err)
	}

	// Capture the conversation ID from the experience object.
	if resp.Msg.Experience != nil && resp.Msg.Experience.ConversationId != nil && *resp.Msg.Experience.ConversationId != "" {
		state.ConversationIDs[step.Ref] = *resp.Msg.Experience.ConversationId
	}

	if decline {
		state.RSVPNoCount++
	} else {
		state.RSVPYesCount++
	}
	return nil
}

// executeStartExperience marks an experience as in-process.
func executeStartExperience(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	experienceID, ok := state.ExperienceIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no experience ID for ref %q", step.Ref)
	}

	_, err := client.Experience().MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: experienceID,
	}))
	if err != nil {
		return fmt.Errorf("start experience: %w", err)
	}

	return nil
}

// executeCompleteExperience completes an experience.
func executeCompleteExperience(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	experienceID, ok := state.ExperienceIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no experience ID for ref %q", step.Ref)
	}

	_, err := client.Experience().CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: experienceID,
	}))
	if err != nil {
		return fmt.Errorf("complete experience: %w", err)
	}

	state.ExperiencesCompleted++
	return nil
}

// executeWithdrawInterest withdraws interest in a transfer.
func executeWithdrawInterest(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	transferID, ok := state.TransferIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no transfer ID for ref %q", step.Ref)
	}

	_, err := client.Transfer().WithdrawInterest(ctx, connect.NewRequest(&api.WithdrawInterestRequest{
		TransferId: transferID,
	}))
	if err != nil {
		return fmt.Errorf("withdraw interest: %w", err)
	}

	// Remove the withdrawn user from the interested list so SelectRecipient
	// won't try to select someone who already withdrew.
	interested := state.InterestedUsers[step.Ref]
	for i, email := range interested {
		if email == step.Actor {
			state.InterestedUsers[step.Ref] = append(interested[:i], interested[i+1:]...)
			break
		}
	}

	state.InterestWithdrawn++
	return nil
}

// executeCancelTransfer cancels a transfer (owner action).
func executeCancelTransfer(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	transferID, ok := state.TransferIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no transfer ID for ref %q", step.Ref)
	}

	_, err := client.Transfer().CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: transferID,
	}))
	if err != nil {
		return fmt.Errorf("cancel transfer: %w", err)
	}

	state.TransfersCancelled++
	return nil
}

// executeCancelRequest cancels a request (requester action).
func executeCancelRequest(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	requestID, ok := state.RequestIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no request ID for ref %q", step.Ref)
	}

	_, err := client.Request().CancelRequest(ctx, connect.NewRequest(&api.CancelRequestRequest{
		RequestId: requestID,
	}))
	if err != nil {
		return fmt.Errorf("cancel request: %w", err)
	}

	state.RequestsCancelled++
	return nil
}

// executeWithdrawOffer withdraws an offer to fulfill a request.
func executeWithdrawOffer(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	requestID, ok := state.RequestIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no request ID for ref %q", step.Ref)
	}

	communityID := state.CommunityIDs[step.CommunityName]
	_, err := client.Request().WithdrawOffer(ctx, connect.NewRequest(&api.WithdrawOfferRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		return fmt.Errorf("withdraw offer: %w", err)
	}

	// Remove the withdrawn user from the helper list so FulfillRequest
	// won't include someone who already withdrew.
	helpers := state.HelperUsers[step.Ref]
	for i, email := range helpers {
		if email == step.Actor {
			state.HelperUsers[step.Ref] = append(helpers[:i], helpers[i+1:]...)
			break
		}
	}

	state.OffersWithdrawn++
	return nil
}

// executeCancelExperience cancels an experience (host action).
func executeCancelExperience(ctx context.Context, client *Client, state *State, step ActivityStep) error {
	experienceID, ok := state.ExperienceIDs[step.Ref]
	if !ok {
		return fmt.Errorf("no experience ID for ref %q", step.Ref)
	}

	_, err := client.Experience().CancelExperience(ctx, connect.NewRequest(&api.CancelExperienceRequest{
		ExperienceId: experienceID,
	}))
	if err != nil {
		return fmt.Errorf("cancel experience: %w", err)
	}

	state.ExperiencesCancelled++
	return nil
}

// getOrCreateLocation creates a residential location for a user in a city.
func getOrCreateLocation(ctx context.Context, client *Client, state *State, email, communityName string, rng *rand.Rand) (string, error) {
	locationKey := "home:" + email
	if id, ok := state.LocationIDs[locationKey]; ok {
		return id, nil
	}

	// Determine city from community region.
	city, err := cityForCommunity(communityName, state)
	if err != nil {
		return "", err
	}

	addr := GenerateResidentialAddress(rng, city)
	resp, err := client.Location().SaveLocation(ctx, connect.NewRequest(&api.SaveLocationRequest{
		LatitudeDeg:  addr.Coordinates.Latitude,
		LongitudeDeg: addr.Coordinates.Longitude,
		RegionCode:   "US",
		PostalCode:   addr.ZipCode,
		Locality:     addr.City,
		AddressLines: []string{addr.Street},
	}))
	if err != nil {
		return "", fmt.Errorf("save location: %w", err)
	}

	state.LocationIDs[locationKey] = resp.Msg.Id
	return resp.Msg.Id, nil
}

// getOrCreateVenueLocation finds a venue matching the category and creates a location for it.
func getOrCreateVenueLocation(ctx context.Context, client *Client, state *State, communityName, venueCategory string, rng *rand.Rand, allVenues map[string][]Venue) (string, error) {
	city, err := cityForCommunity(communityName, state)
	if err != nil {
		return "", err
	}

	cityVenues := VenuesForCity(allVenues, city.Name)

	// Filter by category.
	var matching []Venue
	for _, v := range cityVenues {
		if v.Category == venueCategory {
			matching = append(matching, v)
		}
	}

	// Fallback to any venue if no category match.
	if len(matching) == 0 {
		matching = cityVenues
	}
	if len(matching) == 0 {
		// No venues at all; generate a residential address as fallback.
		return getOrCreateLocation(ctx, client, state, "venue-"+venueCategory, communityName, rng)
	}

	venue := matching[rng.Intn(len(matching))]
	locationKey := "venue:" + venue.Name + ":" + venue.City

	if id, ok := state.LocationIDs[locationKey]; ok {
		return id, nil
	}

	resp, err := client.Location().SaveLocation(ctx, connect.NewRequest(&api.SaveLocationRequest{
		LatitudeDeg:  venue.Coordinates.Latitude,
		LongitudeDeg: venue.Coordinates.Longitude,
		RegionCode:   "US",
		Locality:     venue.City,
		AddressLines: []string{venue.FullAddress},
		Name:         venue.Name,
	}))
	if err != nil {
		return "", fmt.Errorf("save venue location %q: %w", venue.Name, err)
	}

	state.LocationIDs[locationKey] = resp.Msg.Id
	return resp.Msg.Id, nil
}

// cityForCommunity resolves a community name to a City definition.
// It first checks if the scenario has a region, then falls back to the first city.
func cityForCommunity(communityName string, _ *State) (City, error) {
	// Map known scenario community names to cities.
	communityCity := map[string]string{
		"Oakwood Heights Neighbors": "Austin",
		"Campus Crew":               "Boulder",
		"Eastside Tool Library":     "Minneapolis",
		"Westside Mutual Aid":       "Minneapolis",
		"New Friends Sharing":       "Baton Rouge",
		"Test Community":            "Austin",
		"LT-Small Community":        "Austin",
		"LT-Medium Alpha":           "Austin",
		"LT-Medium Beta":            "Boulder",
		"LT-Large Alpha":            "Austin",
		"LT-Large Beta":             "Minneapolis",
		"LT-Large Gamma":            "Boulder",
		"LT-Read Heavy":             "Austin",
	}

	if cityName, ok := communityCity[communityName]; ok {
		return CityByName(cityName)
	}

	// Default to first city.
	if len(Cities) > 0 {
		return Cities[0], nil
	}
	return City{}, fmt.Errorf("no cities available")
}

// extractGearRef extracts the base gear ref from a loan/giveaway ref.
// e.g., "gear-Community-1-loan-42" → "gear-Community-1".
func extractGearRef(ref string) string {
	if idx := strings.LastIndex(ref, "-loan"); idx != -1 {
		return ref[:idx]
	}
	if idx := strings.LastIndex(ref, "-giveaway"); idx != -1 {
		return ref[:idx]
	}
	return ref
}

// materialCategoryFromString converts a material category string to the proto enum.
func materialCategoryFromString(s string) api.MaterialCategory {
	switch s {
	case "metal":
		return api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL
	case "plastic":
		return api.MaterialCategory_MATERIAL_CATEGORY_SOLID_PLASTIC
	case "mixed_plastic_metal":
		return api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL
	case "wood":
		return api.MaterialCategory_MATERIAL_CATEGORY_WOOD
	case "textile":
		return api.MaterialCategory_MATERIAL_CATEGORY_FABRIC
	case "carbon_fiber":
		return api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL
	case "mixed_wood_metal":
		return api.MaterialCategory_MATERIAL_CATEGORY_MIXED_WOOD_METAL
	default:
		return api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED
	}
}
