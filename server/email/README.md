# Email

The `email` package provides transactional email delivery via Mailgun. It defines the `Service` interface and a Mailgun-backed production implementation, plus embedded HTML and plain-text templates for each email type.

## Key files

- `mailgun.go` — production `Service` implementation using the Mailgun API; renders and sends the email one-time sign-in code, password reset, waitlist welcome, waitlist notification, and off-app notification emails, and attaches the inline logo plus delivery-attribution variables via `prepareOutbound`.
- `activity_digest.go` / `activity_digest_render.go` — the internal daily/weekly activity digest (English only), rendered from `ActivityDigestInput`.
- `locale.go` — `localeForRecipient`, which resolves the recipient's locale (their stored `preferred_language` beats the request's `Accept-Language`; see `docs/server/l10n.md`).
- `invite.go`, `undeliverable.go` — host-relayed invitations and the undeliverable-address path.
- `templates.go` / `partials.html` — the shared HTML fragments (document head, logo lockups) and the `parseEmailHTML` helper that makes them callable from every template.
- `mock.go` — in-memory mock for use in tests.
- `design_tokens_gen.go` — **generated** brand palette (`LightColors` / `DarkColors`) inlined into templates at render time. Edit `design/tokens.json` and run `npm run generate:design-tokens`; never edit this file.
- `logo.png` — the logo embedded in every message as `cid:logo.png`.
- Templates, each as an HTML/text pair and (except the two internal ones) per locale:
  - `email_code_template_{en,es}.{html,txt}`
  - `password_reset_template_{en,es}.{html,txt}`
  - `waitlist_welcome_template_{en,es}.{html,txt}`
  - `notification_template_{en,es}.{html,txt}` — off-app notification
  - `activity_digest_template.{html,txt}` — internal, English only
  - `waitlist_notification_template.{html,txt}` — internal, English only

## Writing templates

Email clients are not browsers, and the rules that hold in the app do not hold here.

- **Shared structure lives in `partials.html`.** The document head and both logo lockups are `{{define}}` blocks parsed into every template by `parseEmailHTML`; call them with `{{template "head" .}}`, `{{template "logo" .}}`, `{{template "logo_brandbar" .}}`. Only balanced, self-contained fragments can go there — html/template's contextual escaper rejects a define that ends mid-element, so "open wrapper"/"close wrapper" pairs are not expressible and per-template copy stays per-template.
- **Never center an image with `margin: auto`.** Apple Mail honors `width`, `height` and `display` on a block-level `<img>` but drops its auto margins, which silently left-aligns the logo while every centered *text* element around it stays centered (#2927). Use one of the two idioms that work everywhere: wrap the image in an `align="center"` presentation table (the header logo), or make it `display: inline-block` inside a `text-align: center` parent (the notification brand bar). `TestTemplates_CenterTheLogoWithoutAutoMargins` enforces this.
- **Never hotlink the logo.** A remote `<img>` is a read receipt and renders as a broken box wherever remote content is blocked. Reference the embedded attachment as `cid:logo.png`; `TestTemplates_EmbedLogoRatherThanHotlinking` enforces this.
- **Colors come from the generated palette**, as `{{.Colors.X}}` — email clients cannot use CSS custom properties, so the light tokens are inlined at render time.
- **Every colored element needs its dark-mode class.** Apple Mail invents a dark rendering for any message that does not declare one, by inverting the light styles; the `color-scheme` declaration in the head partial switches that off, so we owe dark-mode readers a real palette in return. The `@media (prefers-color-scheme: dark)` rules key on `r-` classes — `r-page`, `r-card`, `r-inset`, `r-chip`, `r-h`, `r-t`, `r-f`, `r-link`, `r-rule`, `r-bd`, `r-num`, `r-num-alt` — so an element painted with a token but missing its class stays light while everything around it goes dark. `TestTemplates_DeclareDarkModeForEveryColoredElement` enforces this against rendered output. Roles invert rather than mapping token-for-token; see `emailPalette` in `palette.go`.
- **Localized templates move in pairs.** The `_en` and `_es` variants of a template are structurally identical; a change to one belongs in the other. `npm run lint:go:l10n-parity` covers the TOML catalog, **not** template markup, so nothing will catch a half-applied edit.
- Both bodies are required: every HTML template has a `.txt` counterpart, and both are rendered and sent.

## When to add code here vs. elsewhere

Add new email types here: define the method on the `Service` interface, add the template files, and implement the method in `mailgun.go` and `mock.go`. Triggering logic (e.g. "send the reset email when a user requests it") belongs in the service that handles the user action, typically `server/services/login`.
