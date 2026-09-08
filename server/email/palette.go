package email

// templateColors is the palette a template renders against.
//
// The light values are embedded directly, so `{{.Colors.TextPrimary}}` keeps
// resolving as it always has and every color still arrives inline — the only
// form that survives clients which strip <style>. The dark values ride
// alongside under Dark, feeding the prefers-color-scheme override in the
// shared head partial.
//
// Both palettes travel together because a dark rule is only correct relative
// to the light one it overrides, and because the alternative — a second data
// field threaded through every render site — is the kind of parallel plumbing
// that drifts.
type templateColors struct {
	BrandColors
	Dark BrandColors
}

// emailPalette returns the palette every email renders with: the generated
// light and dark token sets from design/tokens.json.
//
// Roles invert between them rather than mapping one-to-one. In light, the page
// is the tinted Surface and a card sits on it in white Background; in dark the
// page is the deepest Background and the card lifts off it in Surface. An
// inset inside that card (the sign-in code chip, the waitlist panel) reads as
// Surface against white, but in dark it has to clear the card it sits on, so
// it takes Border. Mapping each token to its own name in both themes would
// flatten the card into the page.
func emailPalette() templateColors {
	return templateColors{BrandColors: LightColors, Dark: DarkColors}
}
