// Package notification_content renders a notification's copy for every delivery
// surface (push, SMS, email) from one uniform, surface-agnostic descriptor
// (Content).
//
// # One sentence, adornments per surface
//
// Each kind has exactly one self-contained sentence — notif.offapp.{kind}.message
// — that names its entity inline ("Sam is attending Backyard BBQ"). Every
// surface renders that sentence. What differs is only what each one adds around
// it: push prepends a short category title (notif.community_event.{kind}.title,
// via PushCopy) because the OS shows two lines; SMS and email append a
// call-to-action on the link ("Manage RSVPs at https://ripls.app/go/…") because
// they have no app context to navigate from.
//
// This package previously held a second copy of that sentence per kind, and the
// two drifted: the same event was "wants to borrow your Tent" on push and "is
// interested in Tent" off-app, and several push bodies were fragments completed
// by their title ("is starting now") that made no sense on a surface with no
// title. Worse, a notification whose producer wrote its copy inline had a push
// sentence and no off-app one at all, so it fell through to the generic default
// line and texted people "Ripls:  posted an update in " (#2896).
//
// Two invariants keep that from recurring. Every event type that can reach a
// recipient maps to a kind — see registry.go, whose test fails the build
// otherwise. And no template slot can render blank: params substitutes a
// localized stand-in for any name a lookup failed to resolve.
//
// The renderers are pure (no DB) so they are trivially testable and drive the
// render-all-examples utility (cmd/notification_examples).
//
// It is a leaf package: imported by both notifications (off-app senders) and
// notifications/community_subscriber (push), it imports neither.
package notification_content

import (
	"context"
	"strings"
	"unicode"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// Mechanism is how an SMS is delivered, which changes its framing.
type Mechanism int

const (
	// Platform — the message is sent from the Ripls 10DLC long code, so it brands
	// itself ("Ripls:") on every message. The STOP/HELP opt-out footer is gated
	// by RenderSMS's includeOptOut: it rides the recipient's first platform SMS
	// and then only ~monthly (notifications.shouldDiscloseOptOut). The keywords
	// are honored centrally regardless of the rendered text, so the footer is a
	// copy/compliance-reminder decision, not a functional one. RCS will drop it
	// entirely (#2573).
	Platform Mechanism = iota
	// Relay — the host sends it from their own phone (host-relayed invite). It
	// reads as a personal text: no "Ripls:" prefix, no STOP/HELP.
	Relay
)

// Content is the uniform, surface-agnostic description of one notification.
// Off-app senders build it from the CommunityEventPayload (the subscriber
// resolved the names when the notification was built); the renderers compose
// each surface's copy from it.
type Content struct {
	Kind             string // a stable kind key (see kindForEventType); selects the copy
	ActorName        string
	ExperienceName   string
	GearName         string
	RequestTitle     string
	CommunityName    string
	PlanningItemName string // the planning need/contribution name (PLANNING_* events)

	// Entity ids for the off-app deep link (resolved to a /go short link by the
	// caller and passed back into the renderers).
	ExperienceID string
	GearID       string
	RequestID    string

	Mechanism Mechanism
}

// FromCommunityEvent builds Content from a community-event notification payload.
// The names were populated by the producer at build time, so this does no DB
// work. Mechanism defaults to Platform (the off-app dispatcher's case); the
// host-relay invite path sets Relay explicitly.
//
// ctx is used only for the unmapped-event-type warning (see kindForEventType).
func FromCommunityEvent(ctx context.Context, p *models.CommunityEventPayload) Content {
	kind := kindForEventType(ctx, p.GetEventType())
	// A gear share reads differently for a giveaway ("giving away" + Claim) vs a
	// loan ("lending out" + Borrow); split the kind on the resolved availability.
	if kind == "gear_shared" {
		if p.GetGearAvailability() == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			kind = "gear_shared_giveaway"
		} else {
			kind = "gear_shared_loan"
		}
	}
	// A transfer reads differently as a loan vs a giveaway ("your loan is now
	// active" vs "it's on its way to its new owner"), and a cancellation reads
	// differently again when the offer was simply no longer needed rather than
	// withdrawn. Push split these three ways from a Transfer read; the payload
	// now carries both distinctions so every surface splits the same way.
	if kind == "transfer_active" || kind == "transfer_cancelled" {
		switch {
		case kind == "transfer_cancelled" && p.GetTransferOfferCovered():
			kind = "transfer_cancelled_covered"
		case p.GetTransferType() == models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			kind += "_giveaway"
		default:
			kind += "_loan"
		}
	}
	// An experience update says what changed (time / location / both), from the
	// flags the publisher set; falls back to a generic line otherwise.
	if kind == "experience_updated" {
		tc, lc := p.GetTimeChanged(), p.GetLocationChanged()
		switch {
		case tc && lc:
			kind = "experience_updated_both"
		case tc:
			kind = "experience_updated_time"
		case lc:
			kind = "experience_updated_location"
		default:
			kind = "experience_updated_other"
		}
	}
	return Content{
		Kind:             kind,
		ActorName:        p.GetActorName(),
		ExperienceName:   p.GetExperienceName(),
		GearName:         p.GetGearName(),
		RequestTitle:     p.GetRequestTitle(),
		CommunityName:    p.GetCommunityName(),
		PlanningItemName: p.GetPlanningItemName(),
		ExperienceID:     p.GetExperienceId(),
		GearID:           p.GetGearId(),
		RequestID:        p.GetRequestId(),
		Mechanism:        Platform,
	}
}

// params builds the template substitution map, substituting a localized stand-in
// for every name that resolved empty.
//
// Names reach the renderers from storage lookups that can miss — the producers'
// helpers return "" on error by design (community_subscriber.getRequestTitle
// says so outright) — and a template slot that renders "" produces copy like
// "  is attending " with a doubled space and a dangling preposition. Standing in
// a generic noun keeps every sentence grammatical and truthful at the cost of
// being vague, which is the right trade for a notification. The stand-ins live
// in the catalog, not inline, because these surfaces render in the recipient's
// locale (l10n.T's stand-in for an unknown chat sender sets the same precedent).
//
// Callers that need to know whether a name was *really* present must read the
// Content field, not this map — Content.messageKind does exactly that.
func (c Content) params(ctx context.Context, loc *l10n.Localizer) map[string]any {
	standIn := func(value, key string) string {
		if value != "" {
			return value
		}
		return loc.T(ctx, "notif.standin."+key, nil)
	}
	gear := standIn(c.GearName, "gear")
	gearWithArticle := WithArticle(c.GearName)
	if c.GearName == "" {
		// The stand-in already carries its article ("an item"); WithArticle
		// would return "" here and strand the sentence without an object.
		gearWithArticle = gear
	}
	return map[string]any{
		"ActorName":        standIn(c.ActorName, "actor"),
		"ActorFirstName":   standIn(firstName(c.ActorName), "actor"),
		"ExperienceName":   standIn(c.ExperienceName, "experience"),
		"GearName":         gear,
		"GearWithArticle":  gearWithArticle,
		"RequestTitle":     standIn(c.RequestTitle, "request"),
		"CommunityName":    standIn(c.CommunityName, "community"),
		"PlanningItemName": standIn(c.PlanningItemName, "planning_item"),
	}
}

// messageKind returns the kind key that selects the message line. For every
// bespoke kind that is the kind itself; "default" degrades further by which
// names the payload genuinely carries.
//
// The degradation exists because the default line names an actor and a
// community, and the notifications that land on it most often have neither:
// system-generated reminders and nudges have no actor at all, and no producer
// sets community_name on them. Reading the raw Content fields (not params, whose
// stand-ins are never empty) is what distinguishes "the lookup missed" from
// "there was never an actor" (#2896).
func (c Content) messageKind() string {
	if c.Kind != KindDefault {
		return c.Kind
	}
	switch {
	case c.ActorName != "" && c.CommunityName != "":
		return KindDefault
	case c.ActorName != "":
		return "default_actor"
	case c.CommunityName != "":
		return "default_community"
	default:
		return "default_bare"
	}
}

// message is the self-contained, entity-inline sentence for this kind
// ("Sam is attending 'Backyard BBQ'"). Shared by every off-app surface.
func (c Content) message(ctx context.Context, loc *l10n.Localizer) string {
	return loc.T(ctx, "notif.offapp."+c.messageKind()+".message", c.params(ctx, loc))
}

// cta is the call-to-action verb phrase for this kind's link ("Manage RSVPs at").
// Keyed on the kind itself, not messageKind: the default line has four wordings
// but one call to action.
func (c Content) cta(ctx context.Context, loc *l10n.Localizer) string {
	return loc.T(ctx, "notif.offapp."+c.Kind+".cta", c.params(ctx, loc))
}

// RenderSMS composes the outbound text:
//
//	Ripls: {message}
//
//	Reply STOP to opt out, HELP for help.   (only when includeOptOut)
//
//	{cta} {link}
//
// The Relay mechanism (host-relayed invite) drops the "Ripls:" brand prefix and
// the STOP/HELP footer — it's a personal text from the host's own number.
//
// includeOptOut gates the STOP/HELP footer on a Platform message: the dispatcher
// passes true for the recipient's first platform SMS and then only ~monthly (see
// notifications.shouldDiscloseOptOut). The keywords are honored centrally
// regardless of the text, so the footer is a copy/compliance-reminder decision,
// not a functional one. Relay never carries it. link may be empty, in which case
// the CTA line is omitted.
func (c Content) RenderSMS(ctx context.Context, loc *l10n.Localizer, link string, includeOptOut bool) string {
	var b strings.Builder

	if c.Mechanism == Platform {
		b.WriteString(loc.T(ctx, "sms.brand_prefix", nil))
		b.WriteString(" ")
	}
	b.WriteString(c.message(ctx, loc))

	if c.Mechanism == Platform && includeOptOut {
		b.WriteString("\n\n")
		b.WriteString(loc.T(ctx, "sms.optout", nil))
	}

	if link != "" {
		b.WriteString("\n\n")
		b.WriteString(c.cta(ctx, loc))
		b.WriteString(" ")
		b.WriteString(link)
	}
	return b.String()
}

// EmailParts is the surface-specific copy for the off-app email — the subject,
// the heading/body sentence, and the CTA button label — all derived from the
// same Content. The caller wraps these in the branded shell.
type EmailParts struct {
	Subject  string
	Heading  string
	CTALabel string
}

// RenderEmail composes the email copy. Subject and heading are the self-contained
// message; the CTA button is labeled with the kind's action verb (the verb minus
// its trailing " at", e.g. "Manage RSVPs") so the button reads as an action, not
// a generic "Open Ripls".
func (c Content) RenderEmail(ctx context.Context, loc *l10n.Localizer) EmailParts {
	msg := c.message(ctx, loc)
	return EmailParts{
		Subject:  msg,
		Heading:  msg,
		CTALabel: strings.TrimSuffix(c.cta(ctx, loc), " at"),
	}
}

// PushCopy returns the push title and body for this kind from the shared
// catalog: the kind's own title line, and the same self-contained .message
// every other surface renders.
//
// This is the "one sentence, adornments per surface" shape — the sentence is
// written once and push adds a category title where SMS and email add a link
// CTA. Producers of system-generated notifications call it instead of composing
// their own fmt.Sprintf copy, which is what left those notifications with no
// off-app copy at all (#2896).
func (c Content) PushCopy(ctx context.Context, loc *l10n.Localizer) (title, body string) {
	if c.Kind == "invitation" {
		// The cold invite goes to someone who is not a user yet — there is no
		// device to push to, only SMS or email.
		return "", ""
	}
	p := c.params(ctx, loc)
	return loc.T(ctx, "notif.community_event."+c.Kind+".title", p), c.message(ctx, loc)
}

// ArticleFor returns the English indefinite article ("a"/"an") for name, or ""
// when name is empty. Item names are stored article-free and Title-Cased ("Lawn
// Mower"), so copy that introduces one fresh ("Sam is lending out ___") must
// supply the article itself. Heuristic: "an" before a leading vowel letter,
// else "a" — good enough for concrete item titles; the rare misses ("an hour",
// "a unicycle") don't occur in practice and aren't worth a pronunciation table.
func ArticleFor(name string) string {
	if name == "" {
		return ""
	}
	switch unicode.ToLower([]rune(name)[0]) {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	default:
		return "a"
	}
}

// WithArticle returns name prefixed with its indefinite article ("a Lawn Mower"
// / "an Umbrella"), or "" for an empty name. Exposed as the GearWithArticle
// template param for both push and off-app copy.
func WithArticle(name string) string {
	a := ArticleFor(name)
	if a == "" {
		return ""
	}
	return a + " " + name
}

// firstName returns the leading whitespace-separated token of name.
func firstName(name string) string {
	for i, r := range name {
		if r == ' ' || r == '\t' {
			return name[:i]
		}
	}
	return name
}
