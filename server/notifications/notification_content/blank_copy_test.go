package notification_content

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

// splitKinds are the intermediate values in offAppKinds that FromCommunityEvent
// always resolves to a more specific kind before any render. They keep their
// off-app catalog entries but are unreachable, so they are not asserted.
var splitKinds = map[string]bool{
	"gear_shared":           true, // → gear_shared_giveaway | gear_shared_loan
	"item_shared_with_user": true, // → item_shared_with_user_{gear,experience,request}
	"experience_updated":    true, // → experience_updated_{time,location,both,other}
	"transfer_active":       true, // → transfer_active_{loan,giveaway}
	"transfer_cancelled":    true, // → transfer_cancelled_{loan,giveaway,covered}
}

// pushExempt are kinds that never become a push, so they have no
// notif.community_event.* entry and PushCopy is not asserted for them.
var pushExempt = map[string]bool{
	"invitation":         true, // cold contact is not yet a user; SMS/email only
	"experience_rsvp_no": true, // community_subscriber.ShouldNotify excludes it
}

// renderKinds returns every kind key a render can actually reach: the
// offAppKinds values minus the split intermediates, the kinds
// FromCommunityEvent derives from those splits, and the cold-invite kind that
// has no event type at all. The default line's wordings are exercised through
// Content shapes instead (see blankContents) because they are message variants
// of one kind, not kinds.
func renderKinds() []string {
	seen := map[string]bool{}
	var out []string
	add := func(kinds ...string) {
		for _, k := range kinds {
			if !seen[k] && !splitKinds[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	for _, k := range offAppKinds {
		add(k)
	}
	add("gear_shared_giveaway", "gear_shared_loan")
	add("item_shared_with_user_gear", "item_shared_with_user_experience",
		"item_shared_with_user_request")
	add("transfer_active_loan", "transfer_active_giveaway")
	add("transfer_cancelled_loan", "transfer_cancelled_giveaway", "transfer_cancelled_covered")
	add("experience_updated_time", "experience_updated_location",
		"experience_updated_both", "experience_updated_other")
	add("invitation")
	return out
}

// assertWellFormed fails when a rendered string shows any of the tells of a
// template slot that resolved to nothing: a doubled space, a line that leads or
// trails with whitespace, a leftover template delimiter, Go's "<no value>", or —
// because l10n.T falls back to the message id when a key is missing — a literal
// catalog key in the output.
func assertWellFormed(t *testing.T, label, out string) {
	t.Helper()
	if out == "" {
		t.Errorf("%s: rendered empty", label)
		return
	}
	for _, bad := range []string{"  ", "{{", "}}", "<no value>", "notif.", "sms.", "email."} {
		if strings.Contains(out, bad) {
			t.Errorf("%s: contains %q — a name resolved empty or a catalog key is missing:\n%s",
				label, bad, out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if line != strings.TrimSpace(line) {
			t.Errorf("%s: line %d has leading/trailing whitespace: %q", label, i, line)
		}
	}
}

// TestCopyNeverRendersBlanks is the #2896 regression gate. Every kind, on every
// surface, in every locale, rendered from a Content with nothing filled in —
// which is what a system-generated notification or a missed storage lookup
// actually produces. Before the stand-ins this emitted "Ripls:  posted an update
// in " to real phones.
func TestCopyNeverRendersBlanks(t *testing.T) {
	for _, tag := range []language.Tag{language.English, language.Spanish} {
		loc, err := l10n.NewLocalizer(tag)
		if err != nil {
			t.Fatalf("l10n.NewLocalizer(%s): %v", tag, err)
		}
		ctx := context.Background()
		for _, kind := range renderKinds() {
			c := Content{Kind: kind, Mechanism: Platform}

			sms := c.RenderSMS(ctx, loc, "https://example.com/go/abc", true)
			// The link is the one legitimate source of "//" and of a token that
			// looks like a key; check the copy around it instead.
			assertWellFormed(t, tag.String()+"/"+kind+"/sms",
				strings.ReplaceAll(sms, "https://example.com/go/abc", "LINK"))

			parts := c.RenderEmail(ctx, loc)
			assertWellFormed(t, tag.String()+"/"+kind+"/email.subject", parts.Subject)
			assertWellFormed(t, tag.String()+"/"+kind+"/email.cta", parts.CTALabel)

			if pushExempt[kind] {
				continue
			}
			title, body := c.PushCopy(ctx, loc)
			assertWellFormed(t, tag.String()+"/"+kind+"/push.title", title)
			assertWellFormed(t, tag.String()+"/"+kind+"/push.body", body)
		}

		// The default kind's four wordings, exercised as the Content shapes that
		// select them rather than as kinds. "no names" is the #2896 payload: a
		// system-generated notification with no actor and no community.
		for _, shape := range []struct {
			label     string
			actor     string
			community string
		}{
			{"both names", "Sam Rivera", "The Block"},
			{"actor only", "Sam Rivera", ""},
			{"community only", "", "The Block"},
			{"no names", "", ""},
		} {
			c := Content{Kind: "default", ActorName: shape.actor, CommunityName: shape.community}
			sms := c.RenderSMS(ctx, loc, "https://example.com/go/abc", false)
			assertWellFormed(t, tag.String()+"/default/"+shape.label+"/sms",
				strings.ReplaceAll(sms, "https://example.com/go/abc", "LINK"))
			assertWellFormed(t, tag.String()+"/default/"+shape.label+"/email.subject",
				c.RenderEmail(ctx, loc).Subject)
		}
	}
}

// TestDefaultKindDegradesByAvailableNames asserts the default line picks its
// wording from the names the payload genuinely carries, rather than
// interpolating blanks. The no-actor/no-community case is the one #2896 hit:
// system-generated notifications have no actor and no producer sets
// community_name on them.
func TestDefaultKindDegradesByAvailableNames(t *testing.T) {
	loc := enLoc(t)
	ctx := context.Background()

	tests := []struct {
		name          string
		actor         string
		community     string
		wantKind      string
		wantSubstring string
	}{
		{"both names", "Sam Rivera", "The Block", "default", "Sam Rivera posted an update in The Block"},
		{"actor only", "Sam Rivera", "", "default_actor", "Sam Rivera posted an update"},
		{"community only", "", "The Block", "default_community", "There's an update in The Block"},
		{"neither", "", "", "default_bare", "You have an update on Ripls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Content{Kind: "default", ActorName: tt.actor, CommunityName: tt.community}
			if got := c.messageKind(); got != tt.wantKind {
				t.Errorf("messageKind() = %q, want %q", got, tt.wantKind)
			}
			if got := c.message(ctx, loc); got != tt.wantSubstring {
				t.Errorf("message() = %q, want %q", got, tt.wantSubstring)
			}
			// All four wordings share one call to action.
			if got := c.cta(ctx, loc); got != "View at" {
				t.Errorf("cta() = %q, want %q", got, "View at")
			}
		})
	}
}

// TestStandInsFillEmptyNames asserts each empty name is replaced by its
// localized stand-in rather than an empty string, including the gear article
// case where WithArticle would otherwise strand the sentence without an object.
func TestStandInsFillEmptyNames(t *testing.T) {
	loc := enLoc(t)
	p := Content{}.params(context.Background(), loc)

	want := map[string]string{
		"ActorName":        "Someone",
		"ActorFirstName":   "Someone",
		"ExperienceName":   "an event",
		"GearName":         "an item",
		"GearWithArticle":  "an item",
		"RequestTitle":     "a request",
		"CommunityName":    "your community",
		"PlanningItemName": "something",
	}
	for k, w := range want {
		if got := p[k]; got != w {
			t.Errorf("params[%q] = %q, want %q", k, got, w)
		}
	}

	// A present name is never overwritten.
	filled := Content{GearName: "Tent"}.params(context.Background(), loc)
	if filled["GearName"] != "Tent" || filled["GearWithArticle"] != "a Tent" {
		t.Errorf("present name should pass through; got %q / %q",
			filled["GearName"], filled["GearWithArticle"])
	}
}

// TestUnmappedEventTypeWarnsOnce asserts the fallthrough is observable. #2896
// was invisible for months because an event type with no copy silently produced
// the default line; the warning is what makes the next one alertable.
func TestUnmappedEventTypeWarnsOnce(t *testing.T) {
	ResetUnmappedLogForTest()
	ctx := context.Background()

	if got := kindForEventType(ctx, "SOME_TYPE_NOBODY_MAPPED"); got != "default" {
		t.Errorf("unmapped type should fall back to default, got %q", got)
	}
	if _, logged := unmappedLog.Load("SOME_TYPE_NOBODY_MAPPED"); !logged {
		t.Error("fallthrough should record the event type in the warn-once gate")
	}
	// A mapped type must not trip the gate.
	if _, logged := unmappedLog.Load("COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES"); logged {
		t.Error("a mapped type must not be recorded as unmapped")
	}
	kindForEventType(ctx, "COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES")
	if _, logged := unmappedLog.Load("COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES"); logged {
		t.Error("a mapped type must not be recorded as unmapped")
	}
}
