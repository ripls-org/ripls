package feed

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func workshopNudge(contextID string) *models.StoredNudge {
	n := &models.StoredNudge{
		Id:        "nudge-workshop",
		Headline:  "Schedule Wednesday morning run again",
		CtaAction: "schedule_repeat",
		Surface:   models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
	}
	if contextID != "" {
		n.ContextId = &contextID
	}
	return n
}

// TestNudgeAudience_FeedTakesOnlyFeedSurface — the browse feed is unchanged by
// the inbox widening: a host prompt written for the retired Workshop tab must
// not appear between two pieces of community content.
func TestNudgeAudience_FeedTakesOnlyFeedSurface(t *testing.T) {
	cases := map[string]struct {
		nudge *models.StoredNudge
		want  bool
	}{
		"feed surface": {
			&models.StoredNudge{Surface: models.NudgeSurface_NUDGE_SURFACE_FEED}, true,
		},
		"unspecified (pre-surface rows)": {
			&models.StoredNudge{Surface: models.NudgeSurface_NUDGE_SURFACE_UNSPECIFIED}, true,
		},
		"workshop with context":    {workshopNudge("exp-1"), false},
		"workshop without context": {workshopNudge(""), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := feedAudience.accepts(tc.nudge); got != tc.want {
				t.Errorf("feedAudience.accepts = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNudgeAudience_InboxTakesActionableHostPrompts — the inbox is where a
// "schedule this again" prompt gets a reader now that the Workshop tab is gone,
// but only when it names the event it acts on. Without a context_id the CTA
// dispatches to an empty create modal, which the feed's own plan_experience
// nudges already cover.
func TestNudgeAudience_InboxTakesActionableHostPrompts(t *testing.T) {
	if !inboxAudience.accepts(workshopNudge("exp-1")) {
		t.Error("inbox should accept a workshop nudge that names its entity")
	}
	if inboxAudience.accepts(workshopNudge("")) {
		t.Error("inbox should reject a workshop nudge with no entity to act on")
	}
	empty := ""
	if inboxAudience.accepts(&models.StoredNudge{
		Surface:   models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK,
		ContextId: &empty,
	}) {
		t.Error("an empty-string context_id is no context at all")
	}
	if !inboxAudience.accepts(&models.StoredNudge{
		Surface: models.NudgeSurface_NUDGE_SURFACE_FEED,
	}) {
		t.Error("inbox should still accept ordinary feed nudges")
	}
}

// TestNudgeAudience_MediaRequirement — a nudge with no background is invisible
// in the feed but renders fine on the inbox's solid card.
func TestNudgeAudience_MediaRequirement(t *testing.T) {
	if !feedAudience.requiresMedia() {
		t.Error("feed must hide nudges whose imagery has not arrived")
	}
	if inboxAudience.requiresMedia() {
		t.Error("inbox must accept text-only nudges")
	}
}

// TestNudgeToPayload_CarriesContextID — the id is what turns "schedule this
// again" into a pre-filled draft; dropping it silently lands the host in an
// empty create modal instead.
func TestNudgeToPayload_CarriesContextID(t *testing.T) {
	payload := nudgeToPayload(workshopNudge("exp-1"))
	if payload.ContextId == nil {
		t.Fatal("expected context_id on the payload")
	}
	if *payload.ContextId != "exp-1" {
		t.Errorf("context_id = %q, want exp-1", *payload.ContextId)
	}

	// Feed nudges dispatch by cta_action alone and carry none.
	plain := nudgeToPayload(&models.StoredNudge{CtaAction: "plan_experience"})
	if plain.ContextId != nil {
		t.Errorf("expected no context_id, got %q", *plain.ContextId)
	}
	empty := ""
	blank := nudgeToPayload(&models.StoredNudge{ContextId: &empty})
	if blank.ContextId != nil {
		t.Error("an empty-string context_id must not reach the client as set")
	}
}

// TestInboxNudge_SurfacesWorkshopHostPrompt — the whole point of widening the
// inbox pool: a "schedule {event} again" row written by the momentum engine has
// to come back from InboxNudge, carrying the id of the event it repeats. This
// goes through real storage because the surface + context_id round trip is
// exactly where it could silently drop.
func TestInboxNudge_SurfacesWorkshopHostPrompt(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	userID := setupTestUser(t, sqlStorage, "host@test.com", "Host")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Hump Day Milers")

	ctx := context.Background()
	contextID := "exp-42"
	nudge := &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
		NudgeVariant:     1,
		Headline:         "Schedule Wednesday morning run again",
		Description:      "Worth keeping the rhythm going.",
		CtaLabel:         "Schedule it again",
		CtaAction:        "schedule_repeat",
		ContextId:        &contextID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if err := insertNudge(ctx, sqlStorage, nudge); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	svc := &Service{sqlStorage: sqlStorage}
	got, err := svc.InboxNudge(ctx, userID, []string{communityID})
	if err != nil {
		t.Fatalf("InboxNudge: %v", err)
	}
	if got == nil {
		t.Fatal("expected the host prompt on the home view, got none")
	}
	if got.CtaAction != "schedule_repeat" {
		t.Errorf("cta_action = %q, want schedule_repeat", got.CtaAction)
	}
	if got.ContextId == nil || *got.ContextId != contextID {
		t.Errorf("context_id = %v, want %q", got.ContextId, contextID)
	}
}

// TestGetActiveNudgesForUser_StillExcludesWorkshop — the feed must not start
// showing host prompts between two pieces of community content.
func TestGetActiveNudgesForUser_StillExcludesWorkshop(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	userID := setupTestUser(t, sqlStorage, "feed@test.com", "Feed User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Feed Community")

	ctx := context.Background()
	mediaID := "media-1"
	contextID := "exp-42"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
		NudgeVariant:     1,
		Headline:         "Schedule it again",
		CtaLabel:         "Go",
		CtaAction:        "schedule_repeat",
		ContextId:        &contextID,
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insertNudge: %v", err)
	}

	fetched, err := getActiveNudgesForUser(ctx, sqlStorage, userID, communityID)
	if err != nil {
		t.Fatalf("getActiveNudgesForUser: %v", err)
	}
	if len(fetched) != 0 {
		t.Errorf("feed pool took %d workshop nudge(s), want 0", len(fetched))
	}
}

// TestInboxNudge_HostPromptOutranksAGenericNudgeWithImagery — the ranking bug
// the runclub reel caught. The momentum engine writes host prompts with no
// stock imagery, and the inbox used to prefer any nudge that had a picture, so
// a generic "plan something" card permanently buried "schedule your Wednesday
// run again" — the one that knows what the viewer actually did.
func TestInboxNudge_HostPromptOutranksAGenericNudgeWithImagery(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	userID := setupTestUser(t, sqlStorage, "ranked@test.com", "Ranked")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Ranked Community")

	ctx := context.Background()
	mediaID := "media-1"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          models.NudgeSurface_NUDGE_SURFACE_FEED,
		NudgeVariant:     1,
		Headline:         "Plan something",
		CtaLabel:         "Plan something",
		CtaAction:        "plan_experience",
		MediaId:          &mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert generic nudge: %v", err)
	}
	contextID := "exp-42"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD,
		NudgeVariant:     1,
		Headline:         "Schedule Wednesday morning run again",
		CtaLabel:         "Schedule it again",
		CtaAction:        "schedule_repeat",
		ContextId:        &contextID,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert host prompt: %v", err)
	}

	got, err := (&Service{sqlStorage: sqlStorage}).InboxNudge(ctx, userID, []string{communityID})
	if err != nil {
		t.Fatalf("InboxNudge: %v", err)
	}
	if got == nil {
		t.Fatal("expected a nudge")
	}
	if got.CtaAction != "schedule_repeat" {
		t.Errorf("cta_action = %q, want schedule_repeat (the host prompt)", got.CtaAction)
	}
}

// TestInboxNudge_GenericNudgesNeverSurface pins what #2936 changed: the inbox
// slot carries host prompts only.
//
// Generic nudges used to fill it, ranked by whether stock imagery had arrived.
// They were LLM-written prompts to do something — anything — and the Home zero
// state now offers Plan an event / Ask for help / Offer something
// unconditionally instead. Rows may still exist in the table (soft-deleted
// pools, historical data); none of them may reach a user.
func TestInboxNudge_GenericNudgesNeverSurface(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	userID := setupTestUser(t, sqlStorage, "imagery@test.com", "Imagery")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Imagery Community")

	ctx := context.Background()
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId: userID, CommunityId: communityID,
		Surface: models.NudgeSurface_NUDGE_SURFACE_FEED, NudgeVariant: 1,
		Headline: "Text only", CtaLabel: "Go", CtaAction: "plan_experience",
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert text-only: %v", err)
	}
	// The one that used to win: fully generated, imagery attached, fresh.
	mediaID := "media-1"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId: userID, CommunityId: communityID,
		Surface: models.NudgeSurface_NUDGE_SURFACE_FEED, NudgeVariant: 1,
		Headline: "With imagery", CtaLabel: "Go", CtaAction: "plan_experience",
		MediaId: &mediaID, CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert with-media: %v", err)
	}

	got, err := (&Service{sqlStorage: sqlStorage}).InboxNudge(ctx, userID, []string{communityID})
	if err != nil {
		t.Fatalf("InboxNudge: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil — no generic nudge may reach the inbox (#2936)", got)
	}
}

// TestInboxNudge_OneHostPromptWinsAcrossEveryCommunity pins the amplification
// this ranking carries, because it is not obvious from the preference list and
// it is what made #2892 land on every affected host's Home screen.
//
// The host-prompt tier is unconditional: the first one found in ANY of the
// viewer's communities is returned, ahead of a media-ready nudge in the
// community that sorts first. That is correct — a prompt about something the
// viewer actually did beats a generic "plan something" — but it means a
// detector that writes a bad host prompt does not degrade one circle's card,
// it takes the top slot everywhere. A new detector inherits that reach on day
// one, before anyone has seen its copy in production.
func TestInboxNudge_OneHostPromptWinsAcrossEveryCommunity(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	userID := setupTestUser(t, sqlStorage, "reach@test.com", "Reach")
	first := setupTestCommunity(t, sqlStorage, userID, "First Community")
	second := setupTestCommunity(t, sqlStorage, userID, "Second Community")

	ctx := context.Background()
	mediaID := "media-1"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId: userID, CommunityId: first,
		Surface: models.NudgeSurface_NUDGE_SURFACE_FEED, NudgeVariant: 1,
		Headline: "Plan something", CtaLabel: "Go", CtaAction: "plan_experience",
		MediaId: &mediaID, CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert generic nudge: %v", err)
	}
	contextID := "exp-42"
	if err := insertNudge(ctx, sqlStorage, &models.StoredNudge{
		UserId: userID, CommunityId: second,
		Surface: models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD, NudgeVariant: 1,
		Headline: "Schedule Wednesday morning run again", CtaLabel: "Schedule it",
		CtaAction: "schedule_repeat", ContextId: &contextID,
		CreatedAtUnixSec: time.Now().Unix(),
	}); err != nil {
		t.Fatalf("insert host prompt: %v", err)
	}

	got, err := (&Service{sqlStorage: sqlStorage}).InboxNudge(ctx, userID, []string{first, second})
	if err != nil {
		t.Fatalf("InboxNudge: %v", err)
	}
	if got == nil {
		t.Fatal("expected a nudge")
	}
	if got.Headline != "Schedule Wednesday morning run again" {
		t.Errorf("headline = %q, want the second community's host prompt — "+
			"the host-prompt tier reaches across every community, which is "+
			"exactly why a detector's copy has to be true before it ships",
			got.Headline)
	}
}
