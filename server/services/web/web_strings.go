package web

import (
	"context"

	"go.ripls.org/ripls/server/l10n"
)

// This file holds the per-page localized "static label" builders for the SSR
// landings (#2090). Copy that is derived from item/event data (headlines,
// roles, CTAs) is built alongside that data in the *_page.go files; the
// chrome that's the same for every render of a page — button labels, aria
// labels, the at-capacity banner, the open-in-app hint, JS strings — lives
// here so the templates stay free of inline English. Each build* helper takes
// the request's single localizer (Accept-Language) and returns a struct the
// template reads field-by-field.

// capacityStrings is the localized "community is full" banner, shown when a
// landing's hosting community is at the member cap. bodyKey selects the verb
// ("RSVP" on the event, "respond" on the item landings).
type capacityStrings struct {
	Title string
	Body  string
	Ask   string
}

func buildCapacityStrings(ctx context.Context, loc *l10n.Localizer, bodyKey string, num, hi int32) capacityStrings {
	return capacityStrings{
		Title: loc.T(ctx, "web.capacity.title", nil),
		Body:  loc.T(ctx, bodyKey, map[string]any{"Num": num, "Max": hi}),
		Ask:   loc.T(ctx, "web.capacity.ask_inviter", nil),
	}
}

// eventStrings holds the localized static labels for event.html.
type eventStrings struct {
	OpenInApp     string
	MetaAria      string
	IsHosting     string
	RSVPAria      string
	CTAIn         string
	CTAInAria     string
	CTAMaybe      string
	CTAMaybeAria  string
	CTAOut        string
	CTAOutAria    string
	DeclineThanks string
	DeclineFailed string
	Capacity      capacityStrings
}

func buildEventStrings(ctx context.Context, loc *l10n.Localizer, numMembers, maxMembers int32) eventStrings {
	return eventStrings{
		OpenInApp:     loc.T(ctx, "web.common.open_in_app", nil),
		MetaAria:      loc.T(ctx, "web.event.meta_aria", nil),
		IsHosting:     loc.T(ctx, "web.event.is_hosting", nil),
		RSVPAria:      loc.T(ctx, "web.event.rsvp_aria", nil),
		CTAIn:         loc.T(ctx, "web.event.cta_in", nil),
		CTAInAria:     loc.T(ctx, "web.event.cta_in_aria", nil),
		CTAMaybe:      loc.T(ctx, "web.event.cta_maybe", nil),
		CTAMaybeAria:  loc.T(ctx, "web.event.cta_maybe_aria", nil),
		CTAOut:        loc.T(ctx, "web.event.cta_out", nil),
		CTAOutAria:    loc.T(ctx, "web.event.cta_out_aria", nil),
		DeclineThanks: loc.T(ctx, "web.event.decline_thanks", nil),
		DeclineFailed: loc.T(ctx, "web.event.decline_failed", nil),
		Capacity:      buildCapacityStrings(ctx, loc, "web.capacity.body_rsvp", numMembers, maxMembers),
	}
}

// itemStrings holds the localized static labels shared by the gear and request
// landings (the open-in-app hint and the at-capacity banner; their headline /
// role / CTA copy is data-derived and built in the *_page.go files).
type itemStrings struct {
	OpenInApp string
	Capacity  capacityStrings
}

func buildItemStrings(ctx context.Context, loc *l10n.Localizer, numMembers, maxMembers int32) itemStrings {
	return itemStrings{
		OpenInApp: loc.T(ctx, "web.common.open_in_app", nil),
		Capacity:  buildCapacityStrings(ctx, loc, "web.capacity.body_respond", numMembers, maxMembers),
	}
}

// buildCommunityStrings builds the same label set for the community
// landing. It differs from buildItemStrings only in the at-capacity verb:
// on an item page the blocked action is "respond", on a community invite
// it is "join".
func buildCommunityStrings(ctx context.Context, loc *l10n.Localizer, numMembers, maxMembers int32) itemStrings {
	return itemStrings{
		OpenInApp: loc.T(ctx, "web.common.open_in_app", nil),
		Capacity:  buildCapacityStrings(ctx, loc, "web.capacity.body_join", numMembers, maxMembers),
	}
}

// titleOnRipls wraps an already-localized headline in the localized
// "{Title} on Ripls" frame used for the gear/request <title> and og:title.
func titleOnRipls(ctx context.Context, loc *l10n.Localizer, headline string) string {
	return loc.T(ctx, "web.common.on_ripls_title", map[string]any{"Title": headline})
}

// inviteStrings holds the localized labels + OG/title copy for invite.html.
// Since #2875 that template is only the shared error surface — every valid
// share link renders its own landing — so this is the "this link didn't
// work" label set plus the install/manual-code recovery affordances a
// stranded recipient still needs.
type inviteStrings struct {
	Title         string
	OGTitle       string
	OGDescription string

	LogoAlt        string
	InvalidHeading string
	ErrorHelp      string
	TestFlightLbl  string
	PlayLbl        string
	Reopen         string
	CodeLabel      string
	CodeHint       string
	PrivacyPolicy  string
}

// buildInviteStrings builds the invite.html error-card label set.
func buildInviteStrings(ctx context.Context, loc *l10n.Localizer) inviteStrings {
	return inviteStrings{
		Title:          loc.T(ctx, "web.invite.title_invalid", nil),
		OGTitle:        loc.T(ctx, "web.invite.og_title_invalid", nil),
		OGDescription:  loc.T(ctx, "web.invite.og_desc_invalid", nil),
		LogoAlt:        loc.T(ctx, "web.invite.logo_alt", nil),
		InvalidHeading: loc.T(ctx, "web.invite.invalid_heading", nil),
		ErrorHelp:      loc.T(ctx, "web.invite.error_help", nil),
		TestFlightLbl:  loc.T(ctx, "web.invite.store_testflight_label", nil),
		PlayLbl:        loc.T(ctx, "web.invite.store_play_label", nil),
		Reopen:         loc.T(ctx, "web.invite.reopen", nil),
		CodeLabel:      loc.T(ctx, "web.invite.code_label", nil),
		CodeHint:       loc.T(ctx, "web.invite.code_hint", nil),
		PrivacyPolicy:  loc.T(ctx, "web.invite.privacy_policy", nil),
	}
}
