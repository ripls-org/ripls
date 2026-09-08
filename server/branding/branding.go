package branding

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Config is the instance's user-facing identity: the name that appears in
// emails and page titles, the URLs a recipient is sent to, and the legal
// details a jurisdiction requires on outbound mail.
//
// It exists because these values used to be string literals scattered across
// the email templates, the SSR landings, and a handful of services — which
// meant a second deployment could not exist, and a password-reset link pointed
// at production no matter which environment sent it (#2953).
//
// Two kinds of field live here, and the difference decides the default:
//
//   - The **product's** identity — Name, DeepLinkScheme. These describe the
//     software and default to sensible values. A fork that wants its own name
//     overrides them; one that doesn't, doesn't have to.
//   - The **deployment's** identity — every URL, the legal entity, the store
//     listings, the support address. These describe *whose* instance this is
//     and default to empty. An unset value renders as nothing rather than as
//     somebody else's, which is the failure mode worth having.
type Config struct {
	// Name is the product name shown to users ("Ripls"). Note that the name and
	// logo are trademarks even where the code is permissively licensed — see
	// TRADEMARK.md before shipping a build under them.
	Name string

	// AppBaseURL is the canonical https origin for user-facing links, derived
	// from -invite-link-hostname so there is exactly one source of truth for
	// "where do links point". Empty only if that flag is empty, which startup
	// validation already rejects.
	AppBaseURL string

	// LegalEntityName is the operator of this instance, rendered in the invite
	// landing footer. Empty omits the footer line.
	LegalEntityName string

	// PostalAddress is the physical address CAN-SPAM requires in commercial
	// email. Empty gates the off-app email channel rather than sending mail
	// without it.
	PostalAddress string

	// SupportEmail receives user-initiated contact. Empty hides the contact
	// affordance rather than rendering a dead link.
	SupportEmail string

	// PrivacyPolicyURL and TermsURL are linked from consent surfaces. Empty
	// omits the link.
	PrivacyPolicyURL string
	TermsURL         string

	// AppStoreURL and PlayStoreURL point at this instance's published builds.
	// Empty omits the badge — a fork's users must not be sent to our listing.
	AppStoreURL  string
	PlayStoreURL string

	// AndroidPackageID is the applicationId of the published Android build,
	// used to build intent:// fallbacks on the SSR landings. Empty disables the
	// intent fallback and leaves the plain web link.
	AndroidPackageID string

	// DeepLinkScheme is the custom URL scheme the app registers ("ripls" →
	// ripls://). Two instances cannot claim the same scheme on one device, so a
	// fork shipping to users should override it.
	DeepLinkScheme string

	// UserAgentOverride replaces the User-Agent sent on server-initiated
	// outbound fetches. Empty derives one from Name and AppBaseURL — see
	// BotUserAgent.
	UserAgentOverride string
}

// BaseURLFromHostname turns a bare hostname into an https origin. A hostname
// that already carries a scheme is passed through, and a local host keeps its
// port. Returns "" for an empty hostname so callers can test one field.
func BaseURLFromHostname(hostname string) string {
	h := strings.TrimSpace(hostname)
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
		return strings.TrimSuffix(h, "/")
	}
	// Dev and e2e run against host:port targets (10.0.2.2:8080, localhost:8080)
	// that have no TLS in front of them; https there would be unreachable.
	if isLocalHost(h) {
		return "http://" + strings.TrimSuffix(h, "/")
	}
	return "https://" + strings.TrimSuffix(h, "/")
}

// isLocalHost reports whether a hostname is one nothing terminates TLS in
// front of: "localhost", any bare IP (dev servers, the Android emulator's
// 10.0.2.2, a LAN address), or the Docker host alias.
//
// Any IP counts, not a fixed list. server/services/web.BaseURL has always used
// that rule, and the two must agree: they decide the scheme for the same
// deployment from opposite ends — one for mailed links, one for the rendered
// page — so a disagreement means an email pointing at https where the page
// lives on http.
func isLocalHost(hostname string) bool {
	host := hostname
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host == "localhost" || host == "host.docker.internal" || net.ParseIP(host) != nil
}

// BotUserAgent returns the User-Agent for server-initiated outbound fetches.
// An explicit --bot-user-agent wins; otherwise it is derived from the brand
// name and app URL so an operator gets an identifiable, contactable agent
// string without configuring one.
func (b Config) BotUserAgent() string {
	if b.UserAgentOverride != "" {
		return b.UserAgentOverride
	}
	name := strings.ReplaceAll(b.Name, " ", "")
	if name == "" {
		name = "App"
	}
	if b.AppBaseURL == "" {
		return name + "Bot/1.0"
	}
	return fmt.Sprintf("%sBot/1.0 (+%s)", name, b.AppBaseURL)
}

// AppURL joins a path onto AppBaseURL. Returns "" when no base URL is
// configured, so a caller rendering a link can omit it rather than emit a
// relative URL into an email, where it would be unclickable.
func (b Config) AppURL(path string) string {
	if b.AppBaseURL == "" {
		return ""
	}
	return strings.TrimSuffix(b.AppBaseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}

// AppHost returns AppBaseURL's host, port included, with the scheme stripped —
// the form the landing pages compare `window.location.host` against when
// deciding whether they are running on the app's own domain (where Android App
// Links auto-verify) or somewhere else (where the custom scheme is the only
// reliable way into the app). Empty when no base URL is configured.
func (b Config) AppHost() string {
	if b.AppBaseURL == "" {
		return ""
	}
	u, err := url.Parse(b.AppBaseURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// DeepLink builds a custom-scheme URL (scheme://path?query).
func (b Config) DeepLink(path string) string {
	scheme := b.DeepLinkScheme
	if scheme == "" {
		return ""
	}
	return scheme + "://" + strings.TrimPrefix(path, "/")
}

// Validate rejects branding that would render broken output. It deliberately
// checks shape, not presence: an operator who has not configured a store
// listing gets no badge, which is correct, but one who typed a bare hostname
// into --privacy-policy-url gets told rather than shipping a dead link.
func (b Config) Validate() error {
	for _, f := range []struct {
		flag  string
		value string
	}{
		{"--privacy-policy-url", b.PrivacyPolicyURL},
		{"--terms-url", b.TermsURL},
		{"--app-store-url", b.AppStoreURL},
		{"--play-store-url", b.PlayStoreURL},
	} {
		if f.value == "" {
			continue
		}
		u, err := url.Parse(f.value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("%s must be an absolute URL (got %q)", f.flag, f.value)
		}
	}
	if s := b.DeepLinkScheme; s != "" && strings.ContainsAny(s, ":/") {
		return fmt.Errorf("--deep-link-scheme must be a bare scheme without ':' or '/' (got %q)", s)
	}
	return nil
}
