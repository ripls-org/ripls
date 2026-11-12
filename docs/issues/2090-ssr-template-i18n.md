# Issue #2090: Server-side i18n for SSR templates (invite.html, event.html)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2090
**Filed:** 2026-05-21 | **Last activity:** 2026-06-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/server/l10n.md` (⚓ server l10n boundary + catalog), `server/services/web/README.md` (SSR landing routing), `server/l10n/README.md`, `docs/issues/2049-web-event-landing.md` (Compliance B — the English-only v1 that filed this issue), `CLAUDE.md` (server conventions: file-size gate, TODO policy, l10n render boundary)

## Problem Statement

The four SSR landing templates under `server/services/web/templates/`
(`invite.html`, `event.html`, `gear.html`, `request.html`) and the Go
handlers that feed them render **English-only** copy regardless of the
visitor's `Accept-Language`. A Spanish-speaking neighbor who taps a
`/go/{code}` share link sees English CTAs, error states, host/role lines,
and date labels.

The original issue framed step 1 as "pick a Go-side i18n strategy," but
**that decision is already made and shipped.** The maintainer comment
(2026-06-20, from #2520) records that the SSR landing now has the l10n
plumbing in place:

- `server/l10n` is a `nicksnyder/go-i18n` v2 catalog (TOML source of
  truth, `en`/`es`, CI parity gate) with a `Localizer.T(ctx, key, params)`
  renderer, missing-key fallback, and rate-limited warn logging.
- The `/go/` route is wrapped in `middleware.AcceptLanguage`
  (`server/routes.go:88`), so every SSR request already carries the
  visitor's `Accept-Language` in context, and `l10n.LocaleFromContext`
  resolves it (explicit → Accept-Language → stored pref → `en`).
- One string is already localized end-to-end through this path:
  `communityDisplayName()` renders `common.adhoc_community_name` for
  nameless communities (`service.go:62`).

So the library-selection, header-parsing, and catalog-format questions in
the issue body are **resolved**. What remains is the mechanical (but
non-trivial) work the comment describes: **migrate every remaining
hardcoded English string** — in both the Go handlers and the four
templates — into the `server/l10n` catalog and render it through the
localizer.

Two things have also changed since the issue was filed:

1. **Scope grew.** The issue lists `invite.html` + `event.html`. There are
   now four templates (`gear.html`, `request.html` were added for the
   #2492 phone-first landings). All four are English-only and in scope.
2. **The hard parts are now visible.** A lot of the copy is not static
   labels — it's word-order-sensitive composed sentences built with Go
   string concatenation (`ownerName + " is lending " + itemName`), plural
   forms (`+ N others going`, `N items · 1 still open`), and
   `time.Format`-produced English weekday/month names. These can't be
   translated by swapping a string; they need parameterized catalog keys,
   plural support, and (for dates) a locale-aware assembly step.

## Context from Comments

There is exactly one comment, and it is load-bearing (maintainer,
2026-06-20):

> Update from #2520 (merged 2026-06-20): the SSR landing now has the l10n
> plumbing in place — `/go/` routes pass through
> `middleware.AcceptLanguage`, and the nameless-community label is
> localized via the server `l10n` catalog (`common.adhoc_community_name`,
> en+es). The **remaining** work here is mechanical: localize the rest of
> `invite.html`/`event.html` template strings ("Date TBD", "Open in the
> Ripls app", RSVP CTAs, OG copy) by adding catalog keys + rendering via
> the localizer.

Takeaways that reshape the plan:

- **Do not re-litigate the library choice.** `go-i18n` + TOML is the
  committed strategy (`docs/server/l10n.md` → "Source format choice"). The
  ARB-mirroring option from the issue body is explicitly rejected there
  ("ARB-as-shared-source is **not** adopted") because the client and
  server message sets are disjoint.
- **The plumbing tasks (2 & 3 in the issue) are done.** Only "mirror the
  v1 string set into the catalog" and "update tests for both locales"
  remain — plus the composed-copy/plural/date subtleties the comment
  understates as "mechanical."

## Codebase findings (what the migration actually touches)

### Strings to migrate — Go-generated (handlers)

| Site | Strings | Note |
|---|---|---|
| `service.go` (invite errors) | "No invitation code provided.", "Something went wrong. Please try again.", "This invitation link is not valid.", "This invitation link has been revoked." | passed as `ErrorMessage` |
| `event_page.go` `renderEventError` callers | "This event is no longer available.", "Something went wrong. Please try again." | |
| `gear_page.go` / `request_page.go` error callers | "This item is no longer available.", "This request is no longer available.", "This invitation link is not valid." | |
| `event_page.go` `formatEventWhen` | "Soon", "Date TBD" + **English weekday/month names** from `m.Time.Format("Monday" / "Mon, Jan 2" / "3:04 PM")` | date-name localization is the hard part |
| `event_page.go` `fetchPlaceName` | "Location TBD" | |
| `gear_page.go` / `request_page.go` | "Someone" (owner fallback) | |
| `gear_page.go` `buildGearPageData` | `Headline`, `Kicker` ("Free"/"Borrow"), `OwnerLine`, `CtaLabel` ("I want this"/"Ask to borrow"), `CtaAria` — **built with `+` concatenation** | word-order-sensitive |
| `request_page.go` `buildRequestPageData` | `Headline`, `RequesterLine`, `CtaLabel` ("Offer to help"), `CtaAria` — concatenated | word-order-sensitive |
| `request_page.go` `formatNeedsSummary` | "N items"/"1 item" (**plural**), "· all covered", "· N still open" (**plural**) | needs plural support |

### Strings to migrate — template-inline

- **event.html:** title/OG `Join {Host} for {Event}[ in {Community}]`,
  "Open in the Ripls app" (+ `aria-label`), "is hosting",
  `+ {N} other[s] going` (**plural** + word order), the at-capacity block
  ("This community is full", "Can't RSVP because the hosting community is
  full ({n} of {max} members).", "Ask your inviter for options."), CTA
  labels "I'm out"/"Maybe"/"I'm in" + their `aria-label`s, and the two JS
  strings ("Thanks — we'll let the host know.", "Couldn't reach the
  server. Your reply may not have been recorded.").
- **invite.html:** title/OG copy, "You're Invited!", "{Inviter} invited
  you to join", "Open Ripls App", "Invalid Invitation", the
  expired/revoked help paragraph, store-button copy ("Join Beta on" /
  "TestFlight", "Get it on" / "Google Play"), "Already installed? Tap here
  to open in Ripls", invite-code labels, footer "Privacy Policy", and JS
  status strings ("Opening app…", "Don't have the app? Download it
  below:", "To accept this invitation, download the Ripls app:").
- **gear.html:** the at-capacity block, "Open in the Ripls app" (+aria),
  and the visible role line rendered via `{{if eq .Kicker "Free"}}is
  giving this away{{else}}is lending this{{end}}` — see gotcha below.
- **request.html:** "Wanted" kicker, "is looking for this", "Open in the
  Ripls app" (+aria), the at-capacity block.

### Established pattern to follow

Localize copy in Go, keep the template dumb. This is the existing
convention — `gear_page.go` literally comments "precomputed in Go so the
template stays dumb," and the off-app **email rich-card summary** already
does exactly this: `server/email/invite.go:69-76` resolves a locale,
builds a `Localizer`, and renders parameterized copy
(`loc.T(ctx, "email.summary.…", data)`) into struct fields before handing
a dumb template the finished strings. The SSR landing is the browser
analogue of that email card; it should render the same way, driven off
`l10n.NewLocalizerForContext(ctx)` (Accept-Language) instead of the
recipient's stored preference.

### Guardrails already in place

- `npm run lint:go:l10n-parity` (`scripts/check_l10n_toml_parity.js`)
  fails CI if `es.toml` is missing any key in `en.toml`.
- `npm run lint:go:no-inline-user-strings`
  (`scripts/check_no_inline_user_strings.js`) blocks inline
  `fmt.Sprintf("Capitalized …")` user copy — but **only in its
  `SCANNED_FILES` list, which does not yet include the four web handler
  files**, and its regex only catches `fmt.Sprintf`, not bare string
  literals passed as function args (e.g.
  `renderEventError(w, code, "This event is no longer available.")`).
  Extending both is part of this work so the migration can't silently
  regress.

## Non-obvious findings (must-address)

These are the things the issue/comment don't mention that will bite if
missed:

1. **`Cache-Control` + localization = wrong-language cache poisoning.**
   `renderEventPage`/`renderGearPage`/`renderRequestPage` set
   `Cache-Control: public, max-age=30`. Once the body varies by locale, a
   shared cache/CDN keyed only on URL will serve one visitor's Spanish
   HTML to the next English visitor (and vice-versa). **Add
   `Vary: Accept-Language`** to every localized render path. (There is
   already a `TODO(#2051)` in `event_page.go` about `Vary: Cookie` for
   future personalization — same class of bug, address the language axis
   now.)

2. **The `{{if eq .Kicker "Free"}}` sentinel breaks the moment `Kicker`
   is localized.** `gear.html:52` derives the visible role line by
   string-comparing the display kicker to the literal English "Free."
   Translate `Kicker` to "Gratis" and the comparison silently falls
   through to the loan branch ("is lending this") for a giveaway. Fix:
   branch on a boolean/flavor field (e.g. `IsGiveaway`), or — cleaner —
   render the role line from the already-existing Go-computed `OwnerLine`
   field instead of re-deriving it inline. Never branch template logic on
   a user-facing string.

3. **Composed sentences must become parameterized keys, not concatenated
   strings.** Spanish reorders subject/verb/object ("Dana presta un
   taladro", not "Dana is lending a drill"). The Go `+`-concatenation in
   `buildGearPageData`/`buildRequestPageData` produces ungrammatical
   output if each fragment is translated in isolation. Each composed line
   becomes one catalog value with `{{.Owner}}`/`{{.Item}}` placeholders.

4. **Plurals need go-i18n's `PluralCount`, which `Localizer.T` doesn't
   expose yet.** `T` sets only `MessageID` + `TemplateData`, so it always
   renders the `other` form. "+ N others going", "N items"/"1 item", and
   "N still open" need `one`/`other` selection. Add a plural-aware entry
   point (e.g. `Localizer.TCount(ctx, id, count, data)` that sets
   `LocalizeConfig.PluralCount`); `docs/server/l10n.md` already documents
   `one`/`other` as the intended mechanism, the renderer just hasn't
   surfaced it.

5. **`<html lang="en">` is hardcoded** in all four templates; set it to
   the resolved locale for correct screen-reader pronunciation and browser
   translate behavior (accessibility review dimension).

6. **OG/crawler requests carry no `Accept-Language`.** Facebook/Slack/
   iMessage unfurlers won't send the header, so link-preview cards
   correctly fall back to English via the resolver default — acceptable
   and expected. Optionally emit `<meta property="og:locale">`; not
   required for acceptance.

7. **Build one `Localizer` per request, thread it down.** `T` is cheap
   per call, but `communityDisplayName()` today constructs a fresh
   `l10n.NewLocalizerForContext(ctx)` on every invocation. Replicating
   that at each of the ~15 string sites per render is wasteful and
   scatters error handling. Construct the localizer once at the top of
   each `handle*Landing`/`HandleInvitePage` and pass it into
   `build*PageData` / `build*Strings`. (This also lets
   `communityDisplayName` take a `*Localizer` param instead of
   re-resolving.)

8. **DRY: several strings are duplicated across three templates —
   consolidate, don't triplicate the catalog.** The at-capacity block
   ("This community is full" + "Can't {RSVP|respond} because the hosting
   community is full ({n} of {max} members)." + "Ask your inviter for
   options.") appears verbatim (bar the verb) in `event.html`,
   `gear.html`, and `request.html`; "Open in the Ripls app" (+ its
   `aria-label`) appears in all three item templates. Give these shared
   `web.capacity.*` / `web.common.open_in_app` keys (parameterize the
   RSVP-vs-respond verb, or use two keys) rather than minting three
   near-identical entries per locale.

9. **Localized-string composition stays injection-safe — keep the final
   render through `{{.Field}}`.** go-i18n fills `{{.Param}}` placeholders
   with `text/template` (no HTML escaping), then hands the finished string
   to the field; `html/template` escapes it when rendered as `{{.Field}}`
   (HTML **and** JS-string contexts). This is the same double-templating
   the existing `CtaLabel`/`OwnerLine` fields already rely on and is safe
   — but only because the output goes through plain `{{.Field}}`. Do not
   mark any localized field `template.HTML`/pipe it through a `safeHTML`
   func; all these strings are plain text and must stay escaped.

10. **Mind the 1000-line Go file-size gate.** `event_page.go` is already
    ~600 lines and the date-label rework (Phase 4) grows it. Put the new
    per-page `build*Strings` helpers and the date-assembly logic in
    dedicated sibling files (e.g. `web_strings.go`, `event_when.go`)
    rather than bloating the handler files past the CI cap
    (`npm run lint:go:size`).

## Potential Solutions

The only genuine design fork left is **how a template obtains its
localized strings** (the library and header-parsing are settled). Two
viable approaches; a third for date handling is covered under Open
Questions.

### Option A: Precompute in Go, dumb templates (recommended)

- **Approach:** Build a `Localizer` once per request from
  `l10n.NewLocalizerForContext(ctx)`, resolve every string in the Go
  handler, and pass finished strings to the template as struct fields —
  grouping the label set into a small per-page `Str` sub-struct (e.g.
  `data.Str.ImIn`, `data.Str.IsHosting`, `data.Str.OpenInApp`) so the
  top-level data struct doesn't balloon. Parameterized/plural/date copy is
  assembled in Go where word order and counts are trivial to handle.
- **Pros:** Matches the codebase convention verbatim ("template stays
  dumb"), directly mirrors the shipped email-summary pattern, keeps all
  translation logic (plurals, word order, dates) in testable Go, and
  sidesteps `html/template`'s parse-time `FuncMap` constraint. Auto
  JS-escaping of `{{.Str.X}}` inside `<script>` handles the JS-string
  cases for free.
- **Cons:** More struct fields; the `.html` files lose their inline
  human-readable English (that copy now lives in TOML + Go). Slightly more
  Go per page.
- **Effort:** Medium.

### Option B: `T` template function / method on the data struct

- **Approach:** Expose a `T` callable in templates (`{{ .T "web.event.cta.im_in" }}`),
  backed by a per-request localizer either via a method on the data struct
  or a per-request `template.Clone()` with a `FuncMap` closure.
- **Pros:** Template keeps its shape; adding a label is a one-line
  template edit.
- **Cons:** `html/template` binds `FuncMap` at parse time, so per-request
  binding means either a `.Clone()` per render (cost) or a method that
  reaches back into a localizer field. Passing params/plural counts from
  template syntax is clumsy, and word-order-sensitive sentences still have
  to be assembled somewhere — pushing logic into templates fights the
  existing "dumb template" convention.
- **Effort:** Medium-high (more moving parts for a worse fit).

### Option C: Third-party locale-aware date formatter (dates only)

- **Approach:** Pull a library (e.g. `goodsign/monday`) to format
  localized weekday/month names instead of cataloging them.
- **Pros:** Correct CLDR date formatting without hand-maintaining month
  tables.
- **Cons:** New dependency for two locales, against the repo's "minimal
  dependencies" preference; still needs per-locale ordering/format-string
  choices. `golang.org/x/text` (already vendored) has strong
  number/plural support but weak calendar-date formatting, so it's not a
  drop-in either.
- **Effort:** Small-medium, but adds a dep.

## Recommendation

**Option A** for the rendering approach — it is the path of least
resistance because it's already the house style and there's a shipped
precedent to copy (email summary). Group each page's static labels into a
`Str` sub-struct built by a small `build*Strings(ctx, loc)` helper;
keep composed/plural/date copy assembled in Go.

For dates (Option C vs. cataloging), **catalog the weekday and month
names in TOML** and assemble the label in Go from parts + a per-locale
format template. No new dependency, consistent with the TOML-everywhere
catalog, and only 7 + 12 short strings per locale. Scope it as its own
phase (it's the largest single chunk) so the rest can ship first.

Construct the localizer **once per request** and thread it into the
`build*` helpers (see finding 7); consolidate the strings shared across
item templates into `web.capacity.*` / `web.common.*` keys (finding 8).
Add `Vary: Accept-Language` and the `<html lang>` fix as part of the very
first phase — they are cheap and are correctness bugs the moment any
string is localized.

Namespace the new keys `web.<page>.<role>` (`web.event.cta.im_in`,
`web.invite.youre_invited`, `web.error.event_unavailable`,
`web.common.open_in_app`, `web.when.month.jan`, …), extending the existing
`notif.*`/`email.*`/`sms.*`/`common.*` families documented in
`docs/server/l10n.md`.

## Open Questions

- **Do we localize weekday/month names (dates) in this issue, or defer to
  a follow-up?** — Recommendation: **include it, as the final phase.** es
  parity is the stated acceptance bar and an English "Saturday, May 24" on
  an otherwise-Spanish page is jarring. But it's cleanly separable (fixed
  strings ship without it), so if scope pressure hits, split it into a
  child issue rather than blocking the CTA/error localization. Assemble
  from cataloged month/weekday names in Go; do **not** add a date library.

- **How should both-locale rendering be tested without doubling the golden
  files?** — Recommendation: keep the existing English golden-file tests
  as-is, and add **assertion-based `es` tests** that render with
  `Accept-Language: es` and assert the key Spanish substrings appear (CTA
  labels, error text, role line) plus that no target English string
  leaks. A second full golden set per locale is high-maintenance for low
  marginal coverage; the parity gate already guarantees key completeness.

- **Plural API shape on the `Localizer`.** — Recommendation: add
  `TCount(ctx, id, count, data)` that sets `LocalizeConfig.PluralCount`
  (leaving `T` untouched for the no-plural majority). Minimal surface, and
  it's the mechanism `docs/server/l10n.md` already promises.

- **Should the inline-string guard also scan `.html` templates?** —
  Recommendation: for now, extend `check_no_inline_user_strings.js` to
  cover the four web **Go** handler files and broaden its regex to catch
  bare string-literal args (not just `fmt.Sprintf`). A template-scanning
  guard is a nice-to-have follow-up; the golden/es tests plus code review
  are adequate backstop for the `.html` files initially.

- **`preferred_language` for the SSR page?** — The visitor is pre-auth
  (no user row), so Accept-Language is the only real signal and the
  resolver already handles it. No action; noting it so a reviewer doesn't
  expect stored-preference resolution here.

## Implementation Plan

Each phase is independently shippable and leaves the surface fully
functional (English unchanged, newly-migrated strings localized).

### Phase 1: Error/fallback strings + caching & lang correctness
**Goal:** Establish the Go→localizer→dumb-template pattern for the web
surface on the lowest-risk strings, and fix the two correctness bugs that
localization introduces.
**Tasks:**
- [ ] Add `web.error.*` keys (invalid/revoked/not-found/generic-500,
  event/item/request-unavailable) and `web.common.*` fallbacks
  ("Location TBD" → `web.event.location_tbd`, "Someone" →
  `web.common.someone`) to `en.toml` + `es.toml`.
- [ ] Add a one-per-request localizer: construct
  `l10n.NewLocalizerForContext(ctx)` once at the top of each
  `handle*Landing`/`HandleInvitePage` and thread it into the build/render
  helpers (make `communityDisplayName` take the `*Localizer`).
- [ ] Route every error/fallback render site in `service.go`,
  `event_page.go`, `gear_page.go`, `request_page.go`, `item_preview.go`
  through `loc.T(...)`.
- [ ] Add `Vary: Accept-Language` to `renderInvitePage`,
  `renderEventPage`, `renderGearPage`, `renderRequestPage`.
- [ ] Set `<html lang="{{.Lang}}">` from the resolved locale in all four
  templates.
- [ ] Extend `scripts/check_no_inline_user_strings.js`: add the four web
  Go files to `SCANNED_FILES` and broaden the pattern to catch bare
  string-literal user copy.
- [ ] Add `es` assertion tests for at least the invite-error and
  event-unavailable pages, **plus a test asserting the
  `Vary: Accept-Language` response header** on each render path.
**Validates:** the end-to-end Accept-Language → localizer → template-data
path on real render sites; the `Vary`/`lang` fixes; the guard extension.

### Phase 2: Composed copy, roles, CTAs, and plural support
**Goal:** Correctly localize the word-order-sensitive and pluralized copy
that concatenation/`fmt.Sprintf` can't handle.
**Tasks:**
- [ ] Add `Localizer.TCount(ctx, id, count, data)` (sets
  `LocalizeConfig.PluralCount`) + unit tests for `one`/`other` selection.
- [ ] Convert `Headline`/`OwnerLine`/`RequesterLine`/`Kicker`/`CtaLabel`/
  `CtaAria` in `buildGearPageData`/`buildRequestPageData` from `+`
  concatenation to parameterized `web.gear.*`/`web.request.*` keys.
- [ ] Fix the `{{if eq .Kicker "Free"}}` sentinel in `gear.html` — branch
  on a boolean/flavor field or render from the Go-computed role line.
- [ ] Migrate `formatNeedsSummary` (`N items`/`1 item`, `all covered`,
  `N still open`) to plural catalog keys via `TCount`.
- [ ] Migrate event.html's `+ N others going` (plural + word order) into a
  Go-computed field.
- [ ] `es` translations for all of the above + both-locale tests
  (gear giveaway/loan, request, event overflow count).
**Validates:** word-order-correct parameterized rendering and the plural
infrastructure across all three item flavors.

### Phase 3: Template-inline static labels
**Goal:** Remove every remaining hardcoded English string from the four
`.html` files.
**Tasks:**
- [ ] Introduce a per-page `Str` sub-struct + `build*Strings(ctx, loc)`
  helper in a dedicated file (e.g. `web_strings.go`) to respect the
  1000-line Go file-size gate; populate it in each `build*PageData`.
- [ ] Consolidate the strings shared across the three item templates into
  `web.capacity.*` (RSVP-vs-respond verb parameterized) and
  `web.common.open_in_app` (+aria), rather than per-template copies.
- [ ] Migrate event.html: "Open in the Ripls app" (+aria), "is hosting",
  at-capacity block, "I'm out"/"Maybe"/"I'm in" (+arias), the two JS
  strings, and the title/OG `Join … for … [in …]` copy.
- [ ] Migrate invite.html: full visible copy (heading, inviter line,
  buttons, error help paragraph, store buttons, invite-code labels,
  footer) + JS status strings + title/OG copy.
- [ ] Migrate gear.html/request.html remaining labels ("Wanted",
  role lines, "Open in the Ripls app", at-capacity).
- [ ] `es` translations + both-locale render tests asserting no English
  target strings leak.
**Validates:** the visible SSR surface is fully localized; templates are
purely structural.

### Phase 4: Locale-aware date & time labels
**Goal:** Localize the `time.Format`-produced weekday/month names and the
"Soon"/"Date TBD" labels so a Spanish page shows a Spanish date.
**Tasks:**
- [ ] Add `web.when.weekday.*` (7) and `web.when.month.*` (12) keys +
  `web.when.date_tbd`/`web.when.soon` to both catalogs.
- [ ] Rework `formatEventWhen` to assemble `DayLabel`/`WhenShort`/
  `WhenSummary`/`TimeFormatted` from cataloged names + a per-locale
  format/ordering template (Spanish: "sábado, 24 de mayo"), instead of
  `time.Format` English output. No date library.
- [ ] Both-locale tests for TBD, all-day (no time), and full
  date-with-time variants.
- [ ] Update the docs whose globs cover the touched code and advance each
  one's `freshness` stamp: `docs/server/l10n.md` (new `web.*` catalog
  family + date approach), `server/services/web/README.md`, and
  `docs/invitations.md` (globs `server/services/web/**`; its
  §"Error Handling" copy table and §4 `communityDisplayName` note now
  describe localized rendering). The nightly `claude_refresh_docs` ratchet
  will otherwise flag them.
**Validates:** full es date parity; closes the acceptance criteria
(`Accept-Language: es` → Spanish copy, English unchanged, parity gate
green). *Separable:* if scope pressure hits, split into a child issue —
Phases 1-3 satisfy most of the acceptance bar without it.

## Related Issues
- **#2049** — filed this issue (its "Compliance B" is the English-only v1
  decision for the event landing); the parent SSR-landing work.
- **#1195** — grandparent (web guest RSVP); also flags the English-only v1.
- **#2520** — shipped the l10n plumbing (`middleware.AcceptLanguage` on
  `/go/`, `common.adhoc_community_name`) this issue builds on.
- **#1904** — the server l10n system (`server/l10n`, `docs/server/l10n.md`)
  whose catalog + resolver this extends.
- **#2492** — phone-first pivot; added `gear.html`/`request.html`, which
  grew this issue's scope beyond the two templates originally named.
- **#2051** — future SSR personalization; shares the `Vary` cache concern
  (Cookie axis) with this issue's `Vary: Accept-Language` fix.

---

## Audit notes (ripls-audit)

**Scope:** server-only (`server/services/web/**`, `server/l10n/**`); no
proto/RPC or client change. Engaged dimensions: error_handling,
observability, performance (caching/allocation), testing, code_quality,
accessibility. `api_design` not engaged (no proto/RPC); `security` low
(SSR of already-first-name-only PII; auto-escaping preserved).

**Verdict:** Compliant with the loaded standards. The plan already
respects the committed l10n architecture (go-i18n + TOML, disjoint
client/server catalogs, `common.*`/`web.*` namespacing, missing-key
fallback + rate-limited warn logging) and the SSR "dumb template"
convention. It correctly does **not** invent a new library or
Accept-Language parser (both already shipped via #2520/#1904).

**Fixes applied during audit:**
- Added finding 7 (one localizer per request, threaded) — performance +
  code_quality; folded into the recommendation and Phase 1 tasks.
- Added finding 8 (DRY: shared `web.capacity.*` / `web.common.open_in_app`
  keys instead of triplicated copy) — folded into Phase 3.
- Added finding 9 (double-templating stays injection-safe only via
  `{{.Field}}`; never `safeHTML`) — security.
- Added finding 10 + dedicated-file guidance (`web_strings.go`,
  `event_when.go`) for the 1000-line Go file-size gate — code_quality.
- Phase 1: added a `Vary: Accept-Language` header-assertion test —
  testing.
- Phase 4: expanded the doc-update task to include `docs/invitations.md`
  and to advance the `freshness` stamp on every touched doc (the nightly
  `claude_refresh_docs` ratchet governs `server/services/web/**`).

**Standards checked against `docs/llms.txt` selection:** anchors
`server/architecture.md`, `server/conventions.md` (§Error Handling,
§Structured Logging, §Server Testing, §Go Server Build Verification),
`server/observability.md`, `testing/architecture.md`; leaf docs
`server/l10n.md`, `server/services/web/README.md`, `server/l10n/README.md`,
`docs/invitations.md`; review rubrics accessibility, performance,
error_handling, testing, code_quality (+ security, observability).
Confirmed conformance with CLAUDE.md conventions (Dart/Go file-size gates,
TODO(#NNNN) policy, l10n render boundary, no-vendor-names-in-proto — N/A
here, README/doc vendor names allowed).
