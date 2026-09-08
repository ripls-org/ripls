// Command notification_examples renders the off-app notification copy for every
// notification kind, in every locale, as a self-contained HTML review page —
// phone mockups (push card + SMS bubble) alongside the REAL rendered email HTML
// — so the copy can be reviewed for marketing quality in a realistic context.
//
//	go run ./cmd/notification_examples > /tmp/notifications.html && open /tmp/notifications.html
package main

import (
	"context"
	"fmt"
	"html"
	"os"
	"sort"
	"strings"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/notifications/notification_content"
)

// Sample values for the rendered preview. Deliberately fictional: this tool
// writes an HTML page of every notification, and a preview that carried a real
// operator's address or a real person's name would put both wherever the page
// is shared.
const (
	goLink    = "https://app.example.com/go/Xk9mN2pQ"
	unsubLink = "https://app.example.com/email/unsubscribe?u=u_8f31&token=a1b2c3d4e5"
	postal    = "Example Org · 100 Example Street, Springfield, IL 62701"
)

// kindInvitation is the example kind whose copy uses the cold-contact
// (not-yet-a-member) SMS and email variants.
const kindInvitation = "invitation"

func samplePayload(eventType string) *models.CommunityEventPayload {
	return &models.CommunityEventPayload{
		EventType:        eventType,
		ActorName:        "Sam",
		ExperienceName:   "Backyard BBQ",
		GearName:         "Power Drill",
		RequestTitle:     "Folding Table",
		CommunityName:    "Block Party Crew",
		PlanningItemName: "drinks",
	}
}

// heroSet holds one representative hero image URL per entity kind, used to show
// the rich email card in a realistic context on the review page.
type heroSet struct {
	event, gear, request string
}

// loadHeroes fetches a topical Unsplash photo per category when an
// UNSPLASH_ACCESS_KEY is in the environment (the project's standard secret),
// falling back to deterministic picsum.photos placeholders so the page always
// renders. These sample images are review-only — production heroes are the
// item's own media, embedded inline (see notifications.buildEmailExtras).
func loadHeroes(ctx context.Context) heroSet {
	placeholder := func(seed string) string {
		return "https://picsum.photos/seed/" + seed + "/640/280"
	}
	key := os.Getenv("UNSPLASH_ACCESS_KEY")
	if key == "" {
		return heroSet{event: placeholder("bbq"), gear: placeholder("drill"), request: placeholder("table")}
	}
	client := mediapkg.NewUnsplashClient(key)
	pick := func(query, seed string) string {
		resp, err := client.SearchPhotos(ctx, query, 1)
		if err == nil && len(resp.Results) > 0 && resp.Results[0].URLs.Regular != "" {
			return resp.Results[0].URLs.Regular
		}
		return placeholder(seed)
	}
	return heroSet{
		event:   pick("backyard barbecue party", "bbq"),
		gear:    pick("cordless power drill tool", "drill"),
		request: pick("folding table furniture", "table"),
	}
}

// sampleEmailExtras returns a representative summary card + hero URL for a
// notification kind, localized via loc — the review-page stand-in for what
// notifications.buildEmailExtras fetches from storage in production. Kinds with
// no underlying item (community lifecycle, default) return (nil, "").
func sampleEmailExtras(ctx context.Context, loc *l10n.Localizer, kind string, h heroSet) (*email.ItemSummary, string) {
	t := func(id string, data map[string]any) string { return loc.T(ctx, id, data) }
	switch {
	case strings.HasPrefix(kind, "experience_") || kind == kindInvitation || strings.HasPrefix(kind, "planning_"):
		return &email.ItemSummary{
			Eyebrow:     t("email.summary.eyebrow.event", nil),
			Name:        "Backyard BBQ",
			OwnerLine:   t("email.summary.hosted_by", map[string]any{"Name": "Sam"}),
			Date:        "June 27",
			Going:       t("email.summary.going", map[string]any{"Count": 12}),
			Description: "Burgers, dogs, and lawn games — bring a side and your crew. Rain plan is the garage.",
		}, h.event
	case kind == "gear_shared_giveaway":
		return &email.ItemSummary{
			Eyebrow:     t("email.summary.eyebrow.giveaway", nil),
			Name:        "Power Drill",
			OwnerLine:   t("email.summary.shared_by", map[string]any{"Name": "Sam"}),
			Description: "20V cordless drill with two batteries and a charger. Works great — just upgraded.",
		}, h.gear
	case kind == "gear_shared_loan" || strings.HasPrefix(kind, "transfer_"):
		return &email.ItemSummary{
			Eyebrow:     t("email.summary.eyebrow.loan", nil),
			Name:        "Power Drill",
			OwnerLine:   t("email.summary.shared_by", map[string]any{"Name": "Sam"}),
			Description: "20V cordless drill with two batteries and a charger. Happy to lend for a weekend.",
		}, h.gear
	case strings.HasPrefix(kind, "request_"):
		return &email.ItemSummary{
			Eyebrow:     t("email.summary.eyebrow.request", nil),
			Name:        "Folding Table",
			OwnerLine:   t("email.summary.requested_by", map[string]any{"Name": "Sam"}),
			Description: "Looking to borrow a 6-foot folding table for a party this weekend.",
		}, h.request
	default:
		return nil, ""
	}
}

func invitationContent(m notification_content.Mechanism) notification_content.Content {
	return notification_content.Content{
		Kind:           kindInvitation,
		ActorName:      "Alex Rivera",
		ExperienceName: "Backyard BBQ",
		Mechanism:      m,
	}
}

// sortedEventTypes returns every event type that can reach a recipient, in a
// stable order: the CommunityEventType enum members first (by number), then the
// synthetic strings the system-generated producers mint.
//
// It reads notification_content.NotificationEventTypes rather than walking the
// enum, because the enum is not the whole story — reminders and nudges are not
// enum members, so a page built from the enum alone showed none of them. That
// blind spot is why the copy in #2896 went unreviewed for months.
func sortedEventTypes() []string {
	byNumber := make(map[string]int32, len(models.CommunityEventType_name))
	for num, name := range models.CommunityEventType_name {
		byNumber[name] = num
	}
	all := notification_content.NotificationEventTypes()
	sort.Slice(all, func(i, j int) bool {
		ni, iOK := byNumber[all[i]]
		nj, jOK := byNumber[all[j]]
		if iOK != jOK {
			return iOK // enum members before the synthetic strings
		}
		if !iOK {
			return all[i] < all[j]
		}
		return ni < nj
	})
	return all
}

// isSystemEventType reports whether an event type is one of the synthetic
// system strings, which have no CommunityEventType and so cannot be run through
// the bus delivery gate.
func isSystemEventType(name string) bool {
	for _, s := range notification_content.SystemEventTypes {
		if s == name {
			return true
		}
	}
	return false
}

func main() {
	// stdout carries the HTML; route logs (e.g. the Unsplash client's) to stderr
	// so they don't leak into the page when redirected with `> file`.
	logging.SetDefault(logging.NewLogger(logging.Options{Output: os.Stderr}))
	fmt.Print(renderHTML())
}

func renderHTML() string {
	ctx := context.Background()
	mg, err := email.NewMailgunService("mail.example.com", "dummy", "Example <noreply@example.com>")
	if err != nil {
		panic(err)
	}
	mg.SetPostalAddress(postal)

	heroes := loadHeroes(ctx)

	var b strings.Builder
	b.WriteString(htmlHead)

	// Partition the enum by the real delivery gate (community_subscriber.ShouldNotify)
	// so the page stays in sync with the notify logic — no hardcoded list.
	var delivered, omitted []string
	for _, et := range sortedEventTypes() {
		// System-generated types are delivered by their own producers, not by
		// the bus gate; they are always live.
		if isSystemEventType(et) || commsub.ShouldNotify(eventType(et)) {
			delivered = append(delivered, et)
		} else {
			omitted = append(omitted, et)
		}
	}

	for _, locale := range []struct {
		label string
		tag   language.Tag
		lang  string
	}{{"English", language.English, "en"}, {"Spanish", language.Spanish, "es"}} {
		loc, lErr := l10n.NewLocalizer(locale.tag)
		if lErr != nil {
			panic(lErr)
		}
		fmt.Fprintf(&b, `<h1 class="locale">%s</h1>`, html.EscapeString(locale.label))

		// Cold-contact invitation: the funnel's first touch. Platform card shows
		// all three surfaces; the host-relay variant is SMS-only (personal text).
		b.WriteString(card(ctx, mg, loc, locale.lang, "invitation — cold contact (platform)", "An invited contact with no Ripls account yet", invitationContent(notification_content.Platform), heroes))
		b.WriteString(smsOnlyCard(ctx, loc, "invitation — host-relay (sent from the host's own phone)", invitationContent(notification_content.Relay)))

		for _, et := range delivered {
			audience := commsub.RecipientAudienceLabel(eventType(et))
			// Gear shares read differently for a giveaway vs a loan — show both.
			if et == "COMMUNITY_EVENT_TYPE_GEAR_SHARED" {
				for _, av := range []struct {
					label string
					avail models.Availability
				}{
					{"giveaway", models.Availability_AVAILABILITY_FOR_GIVEAWAY},
					{"loan", models.Availability_AVAILABILITY_FOR_LOAN},
				} {
					p := samplePayload(et)
					p.GearAvailability = av.avail
					b.WriteString(card(ctx, mg, loc, locale.lang, et+" ("+av.label+")", audience, notification_content.FromCommunityEvent(ctx, p), heroes))
				}
				continue
			}
			b.WriteString(card(ctx, mg, loc, locale.lang, et, audience, notification_content.FromCommunityEvent(ctx, samplePayload(et)), heroes))
		}
	}

	// Omitted: event types that fire no notification (ShouldNotify == false).
	b.WriteString(`<h1 class="locale">Omitted — no notification delivered</h1><section class="omitted"><ul>`)
	for _, et := range omitted {
		fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(et))
	}
	b.WriteString(`</ul></section>`)

	b.WriteString(htmlFoot)
	return b.String()
}

// eventType resolves a CommunityEventType enum name to its typed value.
func eventType(name string) models.CommunityEventType {
	return models.CommunityEventType(models.CommunityEventType_value[name])
}

// card renders one notification across all three surfaces: a push card, an SMS
// bubble, and the real email HTML in an isolated iframe. audience is the
// who-receives-it label (from community_subscriber.RecipientAudienceLabel).
func card(ctx context.Context, mg *email.MailgunService, loc *l10n.Localizer, lang, label, audience string, c notification_content.Content, heroes heroSet) string {
	pushTitle, pushBody := c.PushCopy(ctx, loc)
	// The cold-contact invitation is the recipient's first platform SMS, so it
	// carries the STOP/HELP footer; steady-state kinds (already disclosed) omit
	// it — mirroring the first-message-plus-monthly cadence in production.
	sms := c.RenderSMS(ctx, loc, goLink, c.Kind == kindInvitation)
	parts := c.RenderEmail(ctx, loc)
	summary, heroURL := sampleEmailExtras(ctx, loc, c.Kind, heroes)
	emailHTML, _, _, _, err := mg.RenderNotification(ctx, email.NotificationEmailInput{
		PreferredLanguage: lang,
		Title:             parts.Heading,
		CTALabel:          parts.CTALabel,
		ActionURL:         goLink,
		UnsubscribeURL:    unsubLink,
		ColdInvite:        c.Kind == kindInvitation, // cold-contact footer variant
		HeroImageURL:      heroURL,
		Summary:           summary,
	})
	if err != nil {
		emailHTML = "render error: " + err.Error()
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<section class="card"><h2>%s</h2><div class="audience">Delivered to: %s</div><div class="surfaces">`,
		html.EscapeString(label), html.EscapeString(audience))

	// Push
	b.WriteString(`<div class="surface"><div class="surface-label">Push</div>`)
	if pushTitle == "" && pushBody == "" {
		b.WriteString(`<div class="push push-na">not a push notification</div>`)
	} else {
		fmt.Fprintf(&b, `<div class="push"><div class="push-app"><span class="dot"></span>Ripls</div><div class="push-title">%s</div><div class="push-body">%s</div></div>`,
			html.EscapeString(pushTitle), html.EscapeString(pushBody))
	}
	b.WriteString(`</div>`)

	// SMS
	fmt.Fprintf(&b, `<div class="surface"><div class="surface-label">SMS</div><div class="bubble">%s</div></div>`,
		html.EscapeString(sms))

	// Email (real HTML, isolated in an iframe)
	fmt.Fprintf(&b, `<div class="surface surface-email"><div class="surface-label">Email — subject: <em>%s</em></div><iframe sandbox srcdoc="%s"></iframe></div>`,
		html.EscapeString(parts.Subject), html.EscapeString(emailHTML))

	b.WriteString(`</div></section>`)
	return b.String()
}

// smsOnlyCard renders a single SMS bubble (for the host-relay invite variant).
func smsOnlyCard(ctx context.Context, loc *l10n.Localizer, label string, c notification_content.Content) string {
	sms := c.RenderSMS(ctx, loc, goLink, false) // Relay (host-relay) never carries the footer
	return fmt.Sprintf(`<section class="card"><h2>%s</h2><div class="surfaces"><div class="surface"><div class="surface-label">SMS</div><div class="bubble">%s</div></div></div></section>`,
		html.EscapeString(label), html.EscapeString(sms))
}

const htmlHead = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Ripls — off-app notification copy</title>
<style>
  :root { --ink:#2a211c; --muted:#6b6259; --line:#e7e0d8; --bg:#f4f0ea; --sage:#7d9b76; }
  body { margin:0; background:var(--bg); color:var(--ink); font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  h1.locale { position:sticky; top:0; margin:0; padding:18px 24px; background:var(--ink); color:#fff; z-index:2; }
  .card { padding:20px 24px; border-bottom:1px solid var(--line); }
  .card h2 { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:13px; color:var(--muted); margin:0 0 2px; }
  .audience { font-size:12px; color:var(--sage); margin:0 0 12px; font-weight:600; }
  .omitted { padding:16px 24px 40px; }
  .omitted ul { columns:2; font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:12px; color:var(--muted); }
  .surfaces { display:grid; grid-template-columns:280px 280px 1fr; gap:20px; align-items:start; }
  .surface-label { font-size:11px; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); margin-bottom:6px; }
  .surface-label em { text-transform:none; letter-spacing:0; color:var(--ink); font-style:normal; }
  /* Push card */
  .push { background:#fff; border-radius:14px; padding:12px 14px; box-shadow:0 1px 4px rgba(0,0,0,.08); }
  .push-app { display:flex; align-items:center; gap:6px; font-size:11px; color:var(--muted); text-transform:uppercase; letter-spacing:.06em; margin-bottom:4px; }
  .push-app .dot { width:14px; height:14px; border-radius:4px; background:var(--sage); display:inline-block; }
  .push-title { font-weight:650; font-size:14px; }
  .push-body { font-size:13px; color:#3c342e; margin-top:2px; }
  .push-na { color:var(--muted); font-style:italic; box-shadow:none; background:transparent; padding:12px 0; }
  /* SMS bubble */
  .bubble { background:#e6e6eb; border-radius:18px; border-bottom-left-radius:5px; padding:10px 14px; font-size:14px; line-height:1.45; white-space:pre-wrap; max-width:260px; }
  /* Email */
  .surface-email iframe { width:100%; height:720px; border:1px solid var(--line); border-radius:10px; background:#fff; }
  @media (max-width:1100px){ .surfaces{ grid-template-columns:1fr; } .surface-email iframe{ height:760px; } }
</style></head><body>
`

const htmlFoot = `</body></html>`
