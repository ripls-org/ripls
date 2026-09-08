# branding

The instance's user-facing identity: the product name, the origin that outbound
links point at, the legal details outbound email needs, and the store listings.

## Why this is a package and not a set of literals

These values used to be string literals spread across the email templates, the
server-rendered web landings, and a handful of services. Two consequences, both
of which #2953 had to fix:

1. **A second deployment could not exist.** Every link, badge, and footer named
   one specific instance.
2. **A latent bug.** `RequestPasswordReset` built its link from a hardcoded
   host, so a reset mailed by a development server sent the recipient to
   production. Deriving `AppBaseURL` from the same flag every other link uses
   (`--invite-link-hostname`) makes that class of drift impossible.

## Key files

| File | What it holds |
|---|---|
| `branding.go` | The `Config` value type, its accessors (`AppURL`, `DeepLink`, `BotUserAgent`), and `Validate` |
| `branding_test.go` | Table tests for URL derivation, the User-Agent fallback, and validation |

Flag declaration lives in `server/config/branding_flags.go`, not here — this
package is the value, not the parsing.

## Defaults, and why they differ

Two kinds of field live in `Config`, and which kind a field is decides its
default:

- **The product's identity** (`Name`, `DeepLinkScheme`) describes the software.
  These default to real values, so a fork that doesn't care about renaming
  doesn't have to configure anything.
- **The deployment's identity** (every URL, `LegalEntityName`, `PostalAddress`,
  `SupportEmail`, the store listings, `AndroidPackageID`) describes *whose*
  instance this is. These default to empty, and consumers render nothing when
  they are empty.

That asymmetry is the point. An unconfigured store badge that renders nothing is
a missing feature; one that defaults to *our* listing sends a fork's users to
our app. Empty is the failure mode worth having.

## When to add a field here

Add one when a user-visible string identifies *the operator* rather than *the
software* — a URL someone will click, an address a regulator requires, an
identifier a store issued. Do not add general server tuning; that belongs in
`server/config`.

Adding a field means giving it a default per the rule above, a flag in
`server/config/branding_flags.go`, and a consumer that behaves sensibly when it
is empty.
