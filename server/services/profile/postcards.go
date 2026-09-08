package profile

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/itemkind"
	"go.ripls.org/ripls/server/storage"
)

// Postcard kinds carried on BriefCTARow.cta_action. The client uses
// these tokens to route taps to the right detail screen.
const (
	postcardKindGear       = "gear"
	postcardKindExperience = "experience"
	postcardKindRequest    = "request"
)

// populateRowItem attaches the underlying [api.Item] to a postcard row.
// postcardKind is the entity discriminator; rows whose kind is not an entity
// (none exist in postcards today, but the shape is enforced for safety) get a
// nil item. The entity id and media id are passed in rather than read back off
// the row, which is why the row no longer carries them at top level.
func populateRowItem(row *api.BriefCTARow, postcardKind, contextID, mediaID string) {
	kind, isEntity := itemkind.FromCTAActionString(postcardKind)
	if !isEntity {
		return
	}
	item := &api.Item{
		Kind:      kind,
		Title:     row.Headline,
		ContextId: contextID,
	}
	if row.AtmosphereLine != nil {
		a := *row.AtmosphereLine
		item.Subtitle = &a
	}
	if mediaID != "" {
		m := mediaID
		item.MediaId = &m
	}
	row.Item = item
}

// generatePostcards builds the three deterministic viewer-facing
// postcards (gear, experience, help) for the target user, scoped to
// communities the viewer and target both belong to. Cards are emitted
// only when underlying data exists; an empty slice means the postcard
// section is hidden on the client. The shared-community set is the
// privacy boundary — no postcard ever references an entity inside a
// community the viewer is not in.
func (s *Service) generatePostcards(
	ctx context.Context,
	targetID, targetFirstName string,
	sharedCommunityIDs []string,
) ([]*api.BriefCTARow, error) {
	if len(sharedCommunityIDs) == 0 {
		return nil, nil
	}

	var rows []*api.BriefCTARow

	if row, err := s.buildGearPostcard(ctx, targetID, targetFirstName, sharedCommunityIDs); err != nil {
		return nil, fmt.Errorf("gear postcard: %w", err)
	} else if row != nil {
		rows = append(rows, row)
	}

	if row, err := s.buildExperiencePostcard(ctx, targetID, targetFirstName, sharedCommunityIDs); err != nil {
		return nil, fmt.Errorf("experience postcard: %w", err)
	} else if row != nil {
		rows = append(rows, row)
	}

	if row, err := s.buildHelpPostcard(ctx, targetID, targetFirstName, sharedCommunityIDs); err != nil {
		return nil, fmt.Errorf("help postcard: %w", err)
	} else if row != nil {
		rows = append(rows, row)
	}

	return rows, nil
}

// buildGearPostcard picks the target user's most-loaned, currently
// available item that is shared with at least one community the viewer
// is in. Returns nil when the target has no qualifying gear.
func (s *Service) buildGearPostcard(
	ctx context.Context,
	targetID, targetFirstName string,
	sharedCommunityIDs []string,
) (*api.BriefCTARow, error) {
	allOwned, err := storage.QueryByField[*models.Gear](s.storage, ctx, "owner_id", targetID)
	if err != nil {
		return nil, fmt.Errorf("query owned gear: %w", err)
	}
	if len(allOwned) == 0 {
		return nil, nil
	}

	ownedByID := make(map[string]*models.Gear, len(allOwned))
	gearIDs := make([]string, 0, len(allOwned))
	for _, g := range allOwned {
		if g.Deleted != nil {
			continue
		}
		if g.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
			continue
		}
		ownedByID[g.Id] = g
		gearIDs = append(gearIDs, g.Id)
	}
	if len(gearIDs) == 0 {
		return nil, nil
	}

	commGearRows, err := storage.QueryByFieldIn[*models.CommunityGear](
		s.storage, ctx, "gear_id", gearIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community gear: %w", err)
	}

	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, id := range sharedCommunityIDs {
		sharedSet[id] = struct{}{}
	}

	// Map each visible gear id to one shared community id where it lives
	// (preferring the most-recently-shared row, so the postcard's circle
	// chip stays stable across reads).
	type visibleGear struct {
		gear        *models.Gear
		communityID string
		sharedAt    int64
	}
	visible := make(map[string]*visibleGear)
	for _, cg := range commGearRows {
		if cg.Deleted != nil {
			continue
		}
		if _, ok := sharedSet[cg.CommunityId]; !ok {
			continue
		}
		g, ok := ownedByID[cg.GearId]
		if !ok {
			continue
		}
		if existing, ok := visible[cg.GearId]; ok && existing.sharedAt >= cg.CreatedAtUnixSec {
			continue
		}
		visible[cg.GearId] = &visibleGear{
			gear:        g,
			communityID: cg.CommunityId,
			sharedAt:    cg.CreatedAtUnixSec,
		}
	}
	if len(visible) == 0 {
		return nil, nil
	}

	visibleIDs := make([]string, 0, len(visible))
	for id := range visible {
		visibleIDs = append(visibleIDs, id)
	}

	// Count completed loans per visible gear for ordering and copy.
	loans, err := storage.QueryByFieldIn[*models.Transfer](s.storage, ctx, "gear_id", visibleIDs)
	if err != nil {
		return nil, fmt.Errorf("query loans: %w", err)
	}

	loanCount := make(map[string]int, len(visibleIDs))
	activeOnGear := make(map[string]bool, len(visibleIDs))
	for _, t := range loans {
		if t.Deleted != nil {
			continue
		}
		switch t.State {
		case models.TransferState_TRANSFER_STATE_COMPLETED:
			if t.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
				loanCount[t.GearId]++
			}
		case models.TransferState_TRANSFER_STATE_ACTIVE,
			models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
			activeOnGear[t.GearId] = true
		}
	}

	type candidate struct {
		gear      *visibleGear
		loanCount int
	}
	candidates := make([]candidate, 0, len(visibleIDs))
	for _, id := range visibleIDs {
		if activeOnGear[id] {
			continue
		}
		candidates = append(candidates, candidate{
			gear:      visible[id],
			loanCount: loanCount[id],
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].loanCount != candidates[j].loanCount {
			return candidates[i].loanCount > candidates[j].loanCount
		}
		// Stable tiebreaker by id so test output is deterministic.
		return candidates[i].gear.gear.Id < candidates[j].gear.gear.Id
	})

	pick := candidates[0]
	row := &api.BriefCTARow{
		Headline: fmt.Sprintf("Borrow %s's %s", targetFirstName, pick.gear.gear.Name),
	}
	cid := pick.gear.communityID
	row.CommunityId = &cid
	if pick.loanCount > 0 {
		atmosphere := fmt.Sprintf("Already lent %d times", pick.loanCount)
		row.AtmosphereLine = &atmosphere
	}
	mediaID := ""
	if len(pick.gear.gear.MediaIds) > 0 && pick.gear.gear.MediaIds[0] != "" {
		mediaID = pick.gear.gear.MediaIds[0]
	}
	populateRowItem(row, postcardKindGear, pick.gear.gear.Id, mediaID)
	return row, nil
}

// buildExperiencePostcard surfaces the earliest upcoming experience
// owned by the target user inside any shared community. Returns nil
// when the target has no upcoming events visible to the viewer.
func (s *Service) buildExperiencePostcard(
	ctx context.Context,
	targetID, targetFirstName string,
	sharedCommunityIDs []string,
) (*api.BriefCTARow, error) {
	owned, err := storage.QueryByField[*models.Experience](s.storage, ctx, "owner_id", targetID)
	if err != nil {
		return nil, fmt.Errorf("query owned experiences: %w", err)
	}
	if len(owned) == 0 {
		return nil, nil
	}

	byID := make(map[string]*models.Experience, len(owned))
	ids := make([]string, 0, len(owned))
	for _, e := range owned {
		if e.Deleted != nil {
			continue
		}
		switch e.State {
		case models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
			models.ExperienceState_EXPERIENCE_STATE_CANCELLED:
			continue
		}
		byID[e.Id] = e
		ids = append(ids, e.Id)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := storage.QueryByFieldIn[*models.CommunityExperience](s.storage, ctx, "experience_id", ids)
	if err != nil {
		return nil, fmt.Errorf("query community experiences: %w", err)
	}

	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, id := range sharedCommunityIDs {
		sharedSet[id] = struct{}{}
	}

	type candidate struct {
		exp          *models.Experience
		communityID  string
		startUnixSec int64
	}
	now := time.Now().Unix()
	seen := make(map[string]struct{})
	candidates := make([]candidate, 0, len(rows))
	for _, ce := range rows {
		if ce.Deleted != nil || ce.Archived {
			continue
		}
		if _, ok := sharedSet[ce.CommunityId]; !ok {
			continue
		}
		e, ok := byID[ce.ExperienceId]
		if !ok {
			continue
		}
		// One candidate per experience — first shared community wins.
		if _, dup := seen[ce.ExperienceId]; dup {
			continue
		}
		var start int64
		if e.Time != nil {
			if sp := e.Time.GetSpecific(); sp != nil {
				start = sp.UnixTimestampSec
			} else if r := e.Time.GetRange(); r != nil {
				start = r.StartUnixSec
			}
		}
		if start < now {
			continue
		}
		seen[ce.ExperienceId] = struct{}{}
		candidates = append(candidates, candidate{
			exp:          e,
			communityID:  ce.CommunityId,
			startUnixSec: start,
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].startUnixSec != candidates[j].startUnixSec {
			return candidates[i].startUnixSec < candidates[j].startUnixSec
		}
		return candidates[i].exp.Id < candidates[j].exp.Id
	})
	pick := candidates[0]
	_ = targetFirstName // not required in the headline; experience name is the hook

	row := &api.BriefCTARow{
		Headline: fmt.Sprintf("Join %s", pick.exp.Name),
	}
	cid := pick.communityID
	row.CommunityId = &cid
	mediaID := ""
	if len(pick.exp.MediaIds) > 0 && pick.exp.MediaIds[0] != "" {
		mediaID = pick.exp.MediaIds[0]
	}
	populateRowItem(row, postcardKindExperience, pick.exp.Id, mediaID)
	return row, nil
}

// buildHelpPostcard surfaces the number of times the target user has
// been a confirmed helper on a fulfilled request inside a shared
// community. Returns nil when there is no help history visible to the
// viewer. Unlike gear and experience cards, this card does not point
// at a single entity — the action opens a help-request creation flow.
func (s *Service) buildHelpPostcard(
	ctx context.Context,
	targetID, targetFirstName string,
	sharedCommunityIDs []string,
) (*api.BriefCTARow, error) {
	// Find fulfilled requests where the target was confirmed as a helper.
	allFulfilled, err := storage.QueryByField[*models.Request](
		s.storage, ctx, "state", int32(models.RequestState_REQUEST_STATE_FULFILLED),
	)
	if err != nil {
		return nil, fmt.Errorf("query fulfilled requests: %w", err)
	}
	if len(allFulfilled) == 0 {
		return nil, nil
	}

	helperRequestIDs := make([]string, 0)
	for _, r := range allFulfilled {
		if r.Deleted != nil {
			continue
		}
		for _, h := range r.ConfirmedHelperIds {
			if h == targetID {
				helperRequestIDs = append(helperRequestIDs, r.Id)
				break
			}
		}
	}
	if len(helperRequestIDs) == 0 {
		return nil, nil
	}

	commReqRows, err := storage.QueryByFieldIn[*models.CommunityRequest](
		s.storage, ctx, "request_id", helperRequestIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query community requests: %w", err)
	}
	sharedSet := make(map[string]struct{}, len(sharedCommunityIDs))
	for _, id := range sharedCommunityIDs {
		sharedSet[id] = struct{}{}
	}
	visibleRequests := make(map[string]string) // request_id -> community_id
	for _, cr := range commReqRows {
		if cr.Deleted != nil {
			continue
		}
		if _, ok := sharedSet[cr.CommunityId]; !ok {
			continue
		}
		if _, dup := visibleRequests[cr.RequestId]; dup {
			continue
		}
		visibleRequests[cr.RequestId] = cr.CommunityId
	}
	if len(visibleRequests) == 0 {
		return nil, nil
	}

	count := len(visibleRequests)
	row := &api.BriefCTARow{
		Headline: fmt.Sprintf("Ask %s for a hand", targetFirstName),
	}
	atmosphere := fmt.Sprintf("Offered help %s across your crews", pluralize(count, "time", "times"))
	row.AtmosphereLine = &atmosphere
	// Link to the most recent shared request as the context entity so
	// the client's tap dispatcher has somewhere to land. Pick the first
	// in stable id order for determinism.
	pickedRequestID := ""
	pickedCommunityID := ""
	for rid, cid := range visibleRequests {
		if pickedRequestID == "" || rid < pickedRequestID {
			pickedRequestID = rid
			pickedCommunityID = cid
		}
	}
	row.CommunityId = &pickedCommunityID
	populateRowItem(row, postcardKindRequest, pickedRequestID, "")
	return row, nil
}

// firstName returns the first whitespace-delimited token of name, or
// the full name if no whitespace is present. Used to keep postcard
// headlines short ("Borrow Liam's drill" rather than "Liam Pemberton's
// drill"). Returns "them" when the name is blank.
func firstName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "them"
	}
	if i := strings.IndexAny(trimmed, " \t"); i > 0 {
		return trimmed[:i]
	}
	return trimmed
}

// pluralize returns "1 thing" or "N things" depending on count.
func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
