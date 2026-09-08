package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// RunResult contains summary statistics and state from a simulation run.
type RunResult struct {
	SimulationID       string
	UsersCreated       int
	CommunitiesCreated int
	TimelineSteps      int
	Duration           time.Duration

	// State contains auth tokens and IDs for entities created during the run.
	State *State

	// Timeline is the generated activity timeline (for report generation).
	Timeline Timeline
}

// RunSimulation executes a scenario against a running server.
func RunSimulation(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	startTime := time.Now()
	scenario := cfg.Scenario
	simID := scenario.SimulationID()
	client := NewClient(cfg.ServerURL)
	//nolint:gosec // G404: seeded from cfg.Seed on purpose. The simulation harness
	// exists to replay deterministic timelines (docs/simulation.md); crypto/rand
	// would make a run unreproducible, which is the opposite of what it is for.
	rng := rand.New(rand.NewSource(cfg.Seed))

	slog.Info("starting simulation",
		"simulation_id", simID,
		"scenario", scenario.Name,
		"communities", len(scenario.Communities),
		"seed", cfg.Seed,
	)

	// Clean up any previous run with the same simulation ID.
	cleanupResp, err := client.Admin().CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: simID,
	}))
	if err != nil {
		return nil, fmt.Errorf("cleanup previous simulation: %w", err)
	}
	if cleanupResp.Msg.UsersDeleted > 0 || cleanupResp.Msg.CommunitiesDeleted > 0 {
		slog.Info("cleaned up previous simulation data",
			"simulation_id", simID,
			"users_deleted", cleanupResp.Msg.UsersDeleted,
			"communities_deleted", cleanupResp.Msg.CommunitiesDeleted,
			"records_deleted", cleanupResp.Msg.RecordsDeleted,
		)
	}

	// Set clock to scenario start time for initial setup.
	client.SetTimestamp(scenario.StartTime)

	// State tracks users, communities, gear, etc. created during simulation.
	state := NewState()

	// Step 1: Register users and create communities.
	for _, commDef := range scenario.Communities {
		if err := setupCommunity(ctx, client, state, commDef, simID); err != nil {
			return nil, fmt.Errorf("setup community %q: %w", commDef.Name, err)
		}
	}

	slog.Info("setup complete",
		"users", len(state.UserTokens),
		"communities", len(state.CommunityIDs),
	)

	// Upload media assets if available (non-fatal if directory doesn't exist).
	// Authenticate as the first registered user for media uploads.
	if cfg.AssetsDir != "" {
		for _, token := range state.UserTokens {
			client.AsUser(token)
			break
		}
		if err := uploadAssetsForRun(ctx, client, state, cfg.AssetsDir); err != nil {
			slog.Warn("media upload failed, continuing without images", "error", err)
		} else {
			slog.Info("media assets uploaded", "count", len(state.MediaIDs))
		}

		// Attach profile images to users.
		if err := setUserProfileImages(ctx, client, state); err != nil {
			slog.Warn("failed to set profile images", "error", err)
		}

		// Attach community images.
		if err := setCommunityImages(ctx, client, state, scenario); err != nil {
			slog.Warn("failed to set community images", "error", err)
		}
	}

	// Step 2: Generate deterministic activity timeline.
	tlResult := GenerateTimelineWithOptions(rng, scenario, TimelineOptions{
		ReadTraffic:    cfg.ReadTraffic,
		ReadMultiplier: cfg.ReadMultiplier,
	})

	// Populate gear templates in state so the executor can find them.
	for ref, tmpl := range tlResult.GearTemplates {
		state.GearTemplates[ref] = tmpl
	}

	slog.Info("timeline generated",
		"steps", len(tlResult.Steps),
		"gear_templates", len(tlResult.GearTemplates),
	)

	// Step 3: Execute the timeline.
	if cfg.Concurrency > 0 {
		if err := executeConcurrent(ctx, cfg.ServerURL, state, tlResult.Steps, rng, cfg.ReadTraffic, cfg.Concurrency); err != nil {
			return nil, fmt.Errorf("execute timeline (concurrent): %w", err)
		}
	} else {
		if err := executeTimeline(ctx, cfg.ServerURL, state, tlResult.Steps, rng, cfg.ReadTraffic); err != nil {
			return nil, fmt.Errorf("execute timeline: %w", err)
		}
	}

	duration := time.Since(startTime)
	result := &RunResult{
		SimulationID:       simID,
		UsersCreated:       len(state.UserTokens),
		CommunitiesCreated: len(state.CommunityIDs),
		TimelineSteps:      len(tlResult.Steps),
		Duration:           duration,
		State:              state,
		Timeline:           tlResult.Steps,
	}

	slog.Info("simulation complete",
		"simulation_id", simID,
		"users_created", result.UsersCreated,
		"communities_created", result.CommunitiesCreated,
		"timeline_steps", result.TimelineSteps,
		"gear_created", state.GearCreated,
		"transfers_completed", state.TransfersCompleted,
		"requests_fulfilled", state.RequestsFulfilled,
		"transfers_cancelled", state.TransfersCancelled,
		"interest_withdrawn", state.InterestWithdrawn,
		"requests_cancelled", state.RequestsCancelled,
		"offers_withdrawn", state.OffersWithdrawn,
		"experiences_completed", state.ExperiencesCompleted,
		"experiences_cancelled", state.ExperiencesCancelled,
		"rsvps_yes", state.RSVPYesCount,
		"rsvps_no", state.RSVPNoCount,
		"chat_messages", state.ChatMessagesSent,
		"reads_executed", state.ReadsExecuted,
		"steps_skipped", state.StepsSkipped,
		"steps_failed", state.StepsFailed,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

// setupCommunity registers all members, creates a community, and invites members.
func setupCommunity(ctx context.Context, client *Client, state *State, commDef CommunityDef, simID string) error {
	if len(commDef.Members) == 0 {
		return fmt.Errorf("community has no members")
	}

	// Register the first member (community creator).
	creator := commDef.Members[0]
	if err := registerUser(ctx, client, state, creator, simID); err != nil {
		return fmt.Errorf("register creator %q: %w", creator.Email, err)
	}

	// Create community as the creator.
	client.AsUser(state.UserTokens[creator.Email])
	commResp, err := client.Community().CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:         commDef.Name,
		Description:  commDef.Description,
		SimulationId: &simID,
	}))
	if err != nil {
		return fmt.Errorf("create community: %w", err)
	}
	communityID := commResp.Msg.Id
	state.CommunityIDs[commDef.Name] = communityID

	// Track community members for chat.
	var memberEmails []string
	for _, m := range commDef.Members {
		memberEmails = append(memberEmails, m.Email)
		state.UserNames[m.Email] = m.Name
	}
	state.CommunityMembers[commDef.Name] = memberEmails

	slog.Info("created community",
		"community_name", commDef.Name,
		"community_id", communityID,
		"creator", creator.Email,
	)

	// Get an invitation link for this community.
	inviteResp, err := client.Community().GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		return fmt.Errorf("get invite link: %w", err)
	}
	shortCode := inviteResp.Msg.ShortCode

	// Register remaining members via invitation.
	for _, member := range commDef.Members[1:] {
		if err := registerAndJoin(ctx, client, state, member, shortCode, simID); err != nil {
			return fmt.Errorf("register member %q: %w", member.Email, err)
		}
	}

	return nil
}

// proveSimulatedEmail runs the mailed-code loop for a simulated persona and
// returns the proof token EmailRegister needs since #2571.
//
// The code comes back on RequestEmailCodeResponse.dev_code, which the server
// populates only in dev mode. Simulation already requires a dev-mode server
// (it registers without invitation codes), so that is not a new constraint —
// and it is why the allowlist of fixed test accounts is not the right seam
// here: a run mints a fresh persona per member and cannot be enumerated ahead
// of time.
func proveSimulatedEmail(ctx context.Context, client *Client, email string) (string, error) {
	codeResp, err := client.Login().RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: email,
	}))
	if err != nil {
		return "", fmt.Errorf("request email code: %w", err)
	}
	if codeResp.Msg.GetDevCode() == "" {
		return "", fmt.Errorf("no dev code returned for %s: simulation requires a dev-mode server", email)
	}

	verifyResp, err := client.Login().VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: email,
		Code:  codeResp.Msg.GetDevCode(),
	}))
	if err != nil {
		return "", fmt.Errorf("verify email code: %w", err)
	}
	return verifyResp.Msg.EmailProofToken, nil
}

// registerUser registers the first user (bootstrap — no invite needed).
func registerUser(ctx context.Context, client *Client, state *State, member MemberDef, simID string) error {
	if _, exists := state.UserTokens[member.Email]; exists {
		return nil
	}

	client.AsUser("")
	proofToken, err := proveSimulatedEmail(ctx, client, member.Email)
	if err != nil {
		return err
	}
	resp, err := client.Login().EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
		Email:           member.Email,
		Name:            member.Name,
		EmailProofToken: proofToken,
		SimulationId:    &simID,
	}))
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}

	state.UserTokens[member.Email] = resp.Msg.Tokens.AccessToken
	state.UserIDs[member.Email] = resp.Msg.User.Id

	slog.Info("registered user",
		"email", member.Email,
		"user_id", resp.Msg.User.Id,
		"persona", member.Persona.String(),
	)

	return nil
}

// uploadAssetsForRun uploads images from each asset subdirectory (gear,
// experiences, requests, profiles) and populates state.MediaIDs.
func uploadAssetsForRun(ctx context.Context, client *Client, state *State, assetsDir string) error {
	subdirs := []string{"gear", "experiences", "requests", "profiles", "communities"}
	for _, sub := range subdirs {
		dir := assetsDir + "/" + sub
		if _, err := UploadMediaDir(ctx, client, state, dir); err != nil {
			return fmt.Errorf("upload %s: %w", sub, err)
		}
	}
	return nil
}

// setUserProfileImages attaches uploaded profile images to each user via SaveUser.
// The email→index mapping must match AllUniqueEmails() ordering used by the fetcher.
func setUserProfileImages(ctx context.Context, client *Client, state *State) error {
	emails := AllUniqueEmails()
	attached := 0
	for i, email := range emails {
		key := "profiles/" + ProfileAssetFilename(i)
		mediaID, ok := state.MediaIDs[key]
		if !ok {
			continue
		}

		token, ok := state.UserTokens[email]
		if !ok {
			continue
		}
		userID, ok := state.UserIDs[email]
		if !ok {
			continue
		}

		client.AsUser(token)
		_, err := client.User().SaveUser(ctx, connect.NewRequest(&api.SaveUserRequest{
			UserId:  userID,
			MediaId: &mediaID,
		}))
		if err != nil {
			slog.Warn("failed to set profile image",
				"email", email,
				"error", err,
			)
			continue
		}
		attached++
	}

	slog.Info("profile images attached", "count", attached)
	return nil
}

// setCommunityImages attaches uploaded community images via UpdateCommunity.
// The community→index mapping must match AllUniqueCommunities() ordering used
// by the fetcher.
func setCommunityImages(ctx context.Context, client *Client, state *State, scenario Scenario) error {
	// Build community name → index mapping from AllUniqueCommunities.
	allComms := AllUniqueCommunities()
	commIndex := make(map[string]int, len(allComms))
	for i, c := range allComms {
		commIndex[c.Name] = i
	}

	attached := 0
	for _, commDef := range scenario.Communities {
		idx, ok := commIndex[commDef.Name]
		if !ok {
			continue
		}

		key := "communities/" + CommunityAssetFilename(idx)
		mediaID, ok := state.MediaIDs[key]
		if !ok {
			continue
		}

		communityID, ok := state.CommunityIDs[commDef.Name]
		if !ok {
			continue
		}

		// Auth as the community creator (first member).
		creator := commDef.Members[0]
		token, ok := state.UserTokens[creator.Email]
		if !ok {
			continue
		}

		client.AsUser(token)
		_, err := client.Community().UpdateCommunity(ctx, connect.NewRequest(&api.UpdateCommunityRequest{
			Id:       communityID,
			MediaIds: []string{mediaID},
		}))
		if err != nil {
			slog.Warn("failed to set community image",
				"community", commDef.Name,
				"error", err,
			)
			continue
		}
		attached++
	}

	slog.Info("community images attached", "count", attached)
	return nil
}

// registerAndJoin registers a user via invitation and joins the community.
func registerAndJoin(ctx context.Context, client *Client, state *State, member MemberDef, shortCode, simID string) error {
	if _, exists := state.UserTokens[member.Email]; exists {
		return nil
	}

	client.AsUser("")
	proofToken, err := proveSimulatedEmail(ctx, client, member.Email)
	if err != nil {
		return err
	}
	resp, err := client.Login().EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
		Email:           member.Email,
		Name:            member.Name,
		EmailProofToken: proofToken,
		ShortCode:       shortCode,
		SimulationId:    &simID,
	}))
	if err != nil {
		return fmt.Errorf("register with invite: %w", err)
	}

	state.UserTokens[member.Email] = resp.Msg.Tokens.AccessToken
	state.UserIDs[member.Email] = resp.Msg.User.Id

	slog.Info("registered and joined user",
		"email", member.Email,
		"user_id", resp.Msg.User.Id,
		"persona", member.Persona.String(),
	)

	return nil
}
