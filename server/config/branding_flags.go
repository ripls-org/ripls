package config

import (
	"flag"

	"go.ripls.org/ripls/server/branding"
)

// brandingFlags holds the flag pointers between declaration and flag.Parse.
type brandingFlags struct {
	name              *string
	legalEntityName   *string
	supportEmail      *string
	privacyPolicyURL  *string
	termsURL          *string
	appStoreURL       *string
	playStoreURL      *string
	androidPackageID  *string
	deepLinkScheme    *string
	userAgentOverride *string
}

// declareBrandingFlags registers the branding flags. Call before flag.Parse.
//
// Which values get a real default and which get "" is not arbitrary — see
// server/branding/README.md. In short: the product's identity (name, scheme)
// defaults to something usable; the deployment's identity (URLs, legal entity,
// store listings) defaults to empty, so an unconfigured instance renders
// nothing rather than pointing at somebody else's.
func declareBrandingFlags() *brandingFlags {
	return &brandingFlags{
		name: flag.String("brand-name", "Ripls",
			"Product name shown to users in emails, push notifications, and page titles. The name and logo are trademarks — see TRADEMARK.md."),
		legalEntityName: flag.String("legal-entity-name", "",
			"Legal name of the organization operating this instance, rendered in the invite landing footer. Empty omits the line."),
		supportEmail: flag.String("support-email", "",
			"Address users are directed to for help. Empty hides the contact affordance."),
		privacyPolicyURL: flag.String("privacy-policy-url", "",
			"Absolute URL of this instance's privacy policy, linked from consent surfaces. Empty omits the link."),
		termsURL: flag.String("terms-url", "",
			"Absolute URL of this instance's terms of service, linked from consent surfaces. Empty omits the link."),
		appStoreURL: flag.String("app-store-url", "",
			"Absolute URL of this instance's iOS listing (App Store or TestFlight). Empty omits the badge."),
		playStoreURL: flag.String("play-store-url", "",
			"Absolute URL of this instance's Google Play listing. Empty omits the badge."),
		androidPackageID: flag.String("android-package-id", "",
			"applicationId of the published Android build, used for intent:// fallbacks on the web landings. Empty leaves the plain web link."),
		deepLinkScheme: flag.String("deep-link-scheme", "ripls",
			"Custom URL scheme the app registers (scheme://...). Two instances cannot share one scheme on a device, so a fork shipping to users should override it."),
		userAgentOverride: flag.String("bot-user-agent", "",
			"User-Agent for server-initiated outbound fetches. Empty derives one from --brand-name and the app base URL."),
	}
}

// resolve builds the branding value. inviteLinkHostname and postalAddress come
// from flags declared in Parse, so this stays the single place that knows how
// those map onto user-facing identity — in particular, that every mailed link
// and every landing share one origin.
func (f *brandingFlags) resolve(inviteLinkHostname, postalAddress string) branding.Config {
	return branding.Config{
		Name:              *f.name,
		AppBaseURL:        branding.BaseURLFromHostname(inviteLinkHostname),
		LegalEntityName:   *f.legalEntityName,
		PostalAddress:     postalAddress,
		SupportEmail:      *f.supportEmail,
		PrivacyPolicyURL:  *f.privacyPolicyURL,
		TermsURL:          *f.termsURL,
		AppStoreURL:       *f.appStoreURL,
		PlayStoreURL:      *f.playStoreURL,
		AndroidPackageID:  *f.androidPackageID,
		DeepLinkScheme:    *f.deepLinkScheme,
		UserAgentOverride: *f.userAgentOverride,
	}
}
