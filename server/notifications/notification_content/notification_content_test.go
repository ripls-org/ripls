package notification_content

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// enLoc returns an English Localizer; a failure here means the embedded l10n
// catalog failed to load, which every assertion below depends on.
func enLoc(t *testing.T) *l10n.Localizer {
	t.Helper()
	loc, err := l10n.NewLocalizer(language.English)
	if err != nil {
		t.Fatalf("l10n.NewLocalizer(en): %v", err)
	}
	return loc
}

// TestFromCommunityEvent_KindMapping covers the plain enum→kind lookup and the
// "default" fallback for an unmapped event type, plus name propagation.
func TestFromCommunityEvent_KindMapping(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		wantKind  string
	}{
		{"rsvp yes", "COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES", "experience_rsvp_yes"},
		{"request offer made", "COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE", "request_offer_made"},
		{"unmapped falls back to default", "COMMUNITY_EVENT_TYPE_UNSPECIFIED", "default"},
		{"empty falls back to default", "", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromCommunityEvent(context.Background(), &models.CommunityEventPayload{
				EventType:      tt.eventType,
				ActorName:      "Sam Rivera",
				ExperienceName: "Backyard BBQ",
			})
			if c.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", c.Kind, tt.wantKind)
			}
			if c.ActorName != "Sam Rivera" || c.ExperienceName != "Backyard BBQ" {
				t.Errorf("names not propagated: %+v", c)
			}
			if c.Mechanism != Platform {
				t.Errorf("Mechanism = %v, want Platform (default)", c.Mechanism)
			}
		})
	}
}

// TestFromCommunityEvent_GearShareSplit asserts a GEAR_SHARED event resolves to
// the giveaway or loan kind based on the gear's availability.
func TestFromCommunityEvent_GearShareSplit(t *testing.T) {
	tests := []struct {
		name     string
		avail    models.Availability
		wantKind string
	}{
		{"giveaway", models.Availability_AVAILABILITY_FOR_GIVEAWAY, "gear_shared_giveaway"},
		{"loan", models.Availability_AVAILABILITY_FOR_LOAN, "gear_shared_loan"},
		{"unspecified defaults to loan", models.Availability_AVAILABILITY_UNSPECIFIED, "gear_shared_loan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromCommunityEvent(context.Background(), &models.CommunityEventPayload{
				EventType:        "COMMUNITY_EVENT_TYPE_GEAR_SHARED",
				GearAvailability: tt.avail,
			})
			if c.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", c.Kind, tt.wantKind)
			}
		})
	}
}

// TestFromCommunityEvent_ExperienceUpdatedSplit asserts EXPERIENCE_UPDATED picks
// the variant matching which fields changed.
func TestFromCommunityEvent_ExperienceUpdatedSplit(t *testing.T) {
	tests := []struct {
		name             string
		timeC, locationC bool
		wantKind         string
	}{
		{"both", true, true, "experience_updated_both"},
		{"time only", true, false, "experience_updated_time"},
		{"location only", false, true, "experience_updated_location"},
		{"neither", false, false, "experience_updated_other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeC, locationC := tt.timeC, tt.locationC
			c := FromCommunityEvent(context.Background(), &models.CommunityEventPayload{
				EventType:       "COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED",
				TimeChanged:     &timeC,
				LocationChanged: &locationC,
			})
			if c.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", c.Kind, tt.wantKind)
			}
		})
	}
}

// TestRenderSMS_Platform asserts the platform SMS is branded, carries the
// STOP/HELP footer, and ends with the CTA + link.
func TestRenderSMS_Platform(t *testing.T) {
	loc := enLoc(t)
	c := Content{
		Kind:           "experience_rsvp_yes",
		ActorName:      "Sam",
		ExperienceName: "Backyard BBQ",
		Mechanism:      Platform,
	}
	out := c.RenderSMS(context.Background(), loc, "https://example.com/go/abc", true)

	if !strings.HasPrefix(out, "Ripls:") {
		t.Errorf("platform SMS must start with brand prefix; got:\n%s", out)
	}
	for _, want := range []string{"Sam", "Backyard BBQ", "Reply STOP", "https://example.com/go/abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("platform SMS missing %q; got:\n%s", want, out)
		}
	}
	// CTA + link is the last line.
	if !strings.HasSuffix(out, "https://example.com/go/abc") {
		t.Errorf("platform SMS must end with the link; got:\n%s", out)
	}
}

// TestRenderSMS_OptOutGating asserts the STOP/HELP footer is present only when
// includeOptOut is true (the first message and then ~monthly), but the brand
// prefix, message, and link are always present.
func TestRenderSMS_OptOutGating(t *testing.T) {
	loc := enLoc(t)
	c := Content{Kind: "experience_rsvp_yes", ActorName: "Sam", ExperienceName: "Backyard BBQ", Mechanism: Platform}

	with := c.RenderSMS(context.Background(), loc, "https://example.com/go/abc", true)
	without := c.RenderSMS(context.Background(), loc, "https://example.com/go/abc", false)

	if !strings.Contains(with, "Reply STOP") {
		t.Errorf("includeOptOut=true should carry the footer; got:\n%s", with)
	}
	if strings.Contains(without, "STOP") {
		t.Errorf("includeOptOut=false must omit the footer; got:\n%s", without)
	}
	// Brand prefix, entity, and link survive regardless of the footer.
	for _, out := range []string{with, without} {
		for _, want := range []string{"Ripls:", "Backyard BBQ", "https://example.com/go/abc"} {
			if !strings.Contains(out, want) {
				t.Errorf("SMS missing %q; got:\n%s", want, out)
			}
		}
	}
}

// TestRenderSMS_Relay asserts the host-relayed text drops the brand prefix and
// the STOP/HELP footer — it reads as a personal message.
func TestRenderSMS_Relay(t *testing.T) {
	loc := enLoc(t)
	c := Content{
		Kind:           "invitation",
		ActorName:      "Sam",
		ExperienceName: "Backyard BBQ",
		Mechanism:      Relay,
	}
	// Even with includeOptOut=true, Relay drops the footer (and brand prefix).
	out := c.RenderSMS(context.Background(), loc, "https://example.com/go/abc", true)

	if strings.HasPrefix(out, "Ripls:") {
		t.Errorf("relay SMS must NOT carry the brand prefix; got:\n%s", out)
	}
	if strings.Contains(out, "STOP") {
		t.Errorf("relay SMS must NOT carry the STOP/HELP footer; got:\n%s", out)
	}
	if !strings.Contains(out, "https://example.com/go/abc") {
		t.Errorf("relay SMS should still carry the link; got:\n%s", out)
	}
}

// TestRenderSMS_NoLink asserts an empty link omits the CTA line entirely.
func TestRenderSMS_NoLink(t *testing.T) {
	loc := enLoc(t)
	c := Content{Kind: "default", ActorName: "Sam", CommunityName: "The Block", Mechanism: Platform}
	out := c.RenderSMS(context.Background(), loc, "", true)

	if strings.Contains(out, "http") {
		t.Errorf("SMS with no link must not contain a URL; got:\n%s", out)
	}
	// The default CTA verb ("View at") must not dangle without a link.
	if strings.Contains(out, "View at") {
		t.Errorf("SMS with no link must omit the CTA verb; got:\n%s", out)
	}
}

// TestRenderEmail asserts subject/heading are the self-contained message and the
// CTA button label drops the trailing " at".
func TestRenderEmail(t *testing.T) {
	loc := enLoc(t)
	c := Content{Kind: "experience_rsvp_yes", ActorName: "Sam", ExperienceName: "Backyard BBQ"}
	parts := c.RenderEmail(context.Background(), loc)

	if parts.Subject == "" || parts.Subject != parts.Heading {
		t.Errorf("Subject and Heading should be the same non-empty message; got %+v", parts)
	}
	if !strings.Contains(parts.Subject, "Backyard BBQ") {
		t.Errorf("Subject should name the entity; got %q", parts.Subject)
	}
	if strings.HasSuffix(parts.CTALabel, " at") || parts.CTALabel == "" {
		t.Errorf("CTALabel should be the action verb without a trailing %q; got %q", " at", parts.CTALabel)
	}
}

// TestPushCopy asserts the push shape: the cold invitation is never a push, and
// the giveaway/loan split renders distinct, unambiguous copy ("giving away" vs
// "lending out") with the article supplied. The body is the same self-contained
// sentence the off-app surfaces render — push adds only the title.
func TestPushCopy(t *testing.T) {
	loc := enLoc(t)

	invite := Content{Kind: "invitation", ActorName: "Sam"}
	if title, body := invite.PushCopy(context.Background(), loc); title != "" || body != "" {
		t.Errorf("invitation push should be empty; got (%q, %q)", title, body)
	}

	give := Content{Kind: "gear_shared_giveaway", ActorName: "Sam", GearName: "Tent"}
	loan := Content{Kind: "gear_shared_loan", ActorName: "Sam", GearName: "Tent"}
	gt, gb := give.PushCopy(context.Background(), loc)
	lt, lb := loan.PushCopy(context.Background(), loc)
	if gt == lt && gb == lb {
		t.Errorf("giveaway and loan push should be distinct; both = (%q, %q)", gt, gb)
	}
	if gt == "" || gb == "" || lt == "" || lb == "" {
		t.Errorf("gear_shared push should be non-empty; give=(%q,%q) loan=(%q,%q)", gt, gb, lt, lb)
	}
	if !strings.Contains(gb, "giving away") || !strings.Contains(gb, "a Tent") {
		t.Errorf("giveaway body should say 'giving away a Tent'; got %q", gb)
	}
	if !strings.Contains(lb, "lending out") || !strings.Contains(lb, "a Tent") {
		t.Errorf("loan body should say 'lending out a Tent'; got %q", lb)
	}
	// One sentence: the push body IS the off-app message.
	if msg := give.message(context.Background(), loc); gb != msg {
		t.Errorf("push body %q should be the shared message %q", gb, msg)
	}
}

func TestFirstName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Sam Rivera", "Sam"},
		{"Sam", "Sam"},
		{"", ""},
		{"Sam\tRivera", "Sam"},
		{" Leading", ""},
	}
	for _, tt := range tests {
		if got := firstName(tt.in); got != tt.want {
			t.Errorf("firstName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
