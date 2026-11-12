# Issue #2927: Email OTP has mis-aligned logo at the top in Apple Mail

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2927
**Filed:** 2026-08-15 | **Last activity:** 2026-08-15
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (index) · `docs/server/architecture.md` (⚓) ·
`docs/server/conventions.md` (⚓ — Go comments, server testing, build
verification) · `docs/server/l10n.md` (governs `server/email/**`) ·
`docs/users/priorities.md` + `docs/users/personas.md` (⚓ product) ·
`docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/accessibility_review_prompt.md` (engaged dimensions) ·
`server/email/README.md`

**Status:** Implemented on `logo_align`, **PR #2930**. Verified on live sends of
the sign-in code and waitlist welcome emails (2026-08-15): the `align="center"`
table renders centered in **both Apple Mail and Gmail**, and the declared dark
palette renders correctly in Apple Mail with light unchanged. Option B was the
right call — centering no longer depends on client CSS handling.

Both follow-ups this plan deferred were then folded in at the reviewer's
direction, rather than filed:

- **Option D (shared partials) shipped after all.** The document head and both
  logo lockups moved to `partials.html`, parsed into every template by
  `parseEmailHTML`. Only balanced fragments are shareable — html/template's
  contextual escaper rejects a define that ends mid-element — so per-template
  copy stays put. This also converted the off-app notification brand bar to the
  same table idiom, which settles the open Phase 1 question about whether its
  `inline-block` logo mis-rendered.
- **Dark mode shipped.** Not optional once the head became shared: the digest
  and every other template now inherit the `color-scheme` declaration, and
  declaring it without supplying dark rules would hand dark-mode readers a
  full-brightness email. See the dark-mode section below.

Phase 3 (offline `.eml` preview) not built — the live-send test added in its
place covers the verification need at lower cost.

## Problem Statement

The Ripls logo at the top of the email one-time sign-in code is flush against
the left edge of the card in Apple Mail on macOS. Every other element in the
same header — the `<h1>`, the body copy, the code chip, the footer — is
correctly centered. The same message renders with the logo centered in Gmail
web, which is what the reporter used as the control.

The screenshots confirm the failure is centering-only, not sizing: measured
against the card width (a known 560 CSS px), the logo renders at ~70 CSS px in
**both** clients. Apple Mail is applying the image's `width`/`height`/`display`
declarations and is honoring `text-align: center` on the surrounding text — it
is specifically not honoring `margin: 0 auto` on the block-level `<img>`.

The offending markup is identical in six templates (`server/email/`):

```html
<div style="text-align: center; margin-bottom: 30px">
  <img
    src="cid:logo.png"
    alt="Ripls Logo"
    style="width: 80px; height: 80px; margin: 0 auto 20px; display: block"
  />
```

- `email_code_template_en.html` / `_es.html` — the reported email
- `password_reset_template_en.html` / `_es.html`
- `waitlist_welcome_template_en.html` / `_es.html`

`display: block` opts the image out of the parent's `text-align: center`, and
the fallback centering mechanism (`margin: … auto …`) is the one Apple Mail
drops. The result is a block-level image laid out at its containing block's
left content edge — exactly what the screenshot shows. **This is not
OTP-specific**: password reset and waitlist welcome are broken the same way and
simply haven't been reported.

Notably, `notification_template_en.html` / `_es.html` already use the
email-safe idiom for the same logo — `display: inline-block` inside a
`text-align: center` parent (line 32-37) — and have never been reported. The
repo already contains both the bug and its counter-example.

Two things in the report are **not** the bug and should not be conflated with
it:

1. **"This is also dark theme."** The email renders a dark card in Apple Mail
   because Apple Mail auto-inverts messages that don't declare a color scheme.
   Every email is rendered with `LightColors` (`server/email/mailgun.go:436,
   472, 654, 753, 850`; `activity_digest_render.go:149`); the generated
   `DarkColors` palette in `design_tokens_gen.go` is currently **unused by
   email**. Apple Mail's inversion is off-brand but is a separate, larger
   change (see Open Questions and Related Issues). It is not what moves the
   logo.
2. **"Platform: Web / MacIntel"** in the device context is where the *feedback
   was filed from*, not where the email was viewed. The rendering client is
   Apple Mail on macOS.

### Why `ripls-ui-investigate` was not run

The skill reproduces UI bugs by driving the real Flutter Web bundle in a
hermetic stack. This defect lives in Apple Mail's rendering of a
server-generated MIME message; there is no app surface involved and no code
path the web bundle can exercise. It falls squarely in the skill's documented
carve-out. The equivalent reproduction step — render the template and open it
in the failing client — is folded into Phase 1 below.

## Context from Comments

**There are no comments on this issue.** It is a `user-feedback` auto-filed
report from the in-app feedback system, submitted by user2@example.org against
PROD v0.30.0.

The absence of discussion is not a blocker here: the two attached screenshots
are an unusually good bug report (failing client and working client, same
message), and the defect is fully determined by six lines of checked-in
template markup. What the issue does *not* say — and what only the reporter can
answer — is whether the logo also mis-renders on iOS Mail and in Outlook, and
whether the auto-inverted dark rendering is itself considered a defect. Both
are captured in Open Questions.

## Potential Solutions

### Option A: Match the working template — `display: inline-block`
- **Approach:** In all six templates, change the logo's inline style from
  `margin: 0 auto 20px; display: block` to `margin: 0 0 20px;
  display: inline-block; vertical-align: middle`. Centering then comes solely
  from the parent `text-align: center` — the mechanism Apple Mail demonstrably
  honors in this exact email. This makes all eight logo-bearing templates use
  one idiom.
- **Pros:** Smallest possible diff (one declaration list per template). Zero
  new markup. Provably consistent with `notification_template_*.html`, which
  is already in production. Directly targets the mechanism the screenshots
  implicate.
- **Cons:** Relies on a CSS property (`text-align` inheritance onto an
  inline-level replaced element). If Apple Mail's quirk turns out to be
  specific to `cid:`-embedded inline attachments — i.e. Mail substitutes its
  own attachment view and forces `display: block` on it — this fix does
  nothing and we burn a verification round-trip. Inline-block images also sit
  on the text baseline, adding ~4px of descender space below the logo unless
  `vertical-align` is set.
- **Effort:** Small.

### Option B: Bulletproof table-centered header
- **Approach:** Wrap the logo in a shrink-to-fit presentation table centered by
  the HTML `align="center"` attribute, keeping the parent `text-align: center`
  as a second mechanism:

  ```html
  <div style="text-align: center; margin-bottom: 30px">
    <table role="presentation" align="center" border="0" cellpadding="0" cellspacing="0" style="margin: 0 auto">
      <tr>
        <td style="padding: 0 0 20px; line-height: 0; font-size: 0">
          <img src="cid:logo.png" alt="Ripls" width="80" height="80"
               style="width: 80px; height: 80px; display: block; border: 0" />
        </td>
      </tr>
    </table>
    <h1 …>…</h1>
  </div>
  ```

  The element that must be centered is now a `<table>`, not an `<img>`, and it
  is centered by a presentational HTML attribute rather than by CSS the client
  can override. `line-height: 0; font-size: 0` on the cell removes the
  descender gap; `width`/`height` attributes back up the CSS dimensions for
  Outlook's Word engine.
- **Pros:** The industry-standard email centering idiom, and the only one that
  survives every client including Outlook 2016+ and Apple Mail. Immune to both
  competing hypotheses about the Apple Mail quirk — it does not depend on
  `margin: auto` on an image, nor on `text-align` reaching a replaced element.
  Fixes it in one pass whether or not our diagnosis of the mechanism is exact.
- **Cons:** ~6 lines of table boilerplate per template × 6 templates, in files
  that are otherwise div-based and readable. Diverges from
  `notification_template_*.html` unless that is converted too (an
  eighth/ninth file of churn on a template that isn't broken).
- **Effort:** Small–medium.

### Option D: Factor a shared header partial, then fix it once
- **Approach:** The reason this bug appears six times is that the header block
  is copy-pasted across six files with no shared source. Go's `html/template`
  supports `{{define "logo_header"}}` / `{{template "logo_header" .}}`; parsing
  a shared base alongside each locale template (`s.emailCode.pick(tag)` and
  friends in `server/email/mailgun.go`) would collapse the header — and the
  footer, which is similarly duplicated — to one definition, and this fix to
  one line.
- **Pros:** Addresses the actual root cause. The code-quality rubric's own
  threshold ("a pattern reused 3 or more times should be factored") is met
  three times over. The next header change costs one edit instead of six, and
  can't be half-applied across the en/es pair.
- **Cons:** Touches template *parsing* in `mailgun.go` (928/1000 lines against
  the Go file-size gate) for every email type, which is a materially larger
  blast radius than the defect warrants. A parsing regression would break
  sign-in emails outright — strictly worse than a mis-centered logo. It also
  entangles the fix with a refactor, so a revert can't undo one without the
  other.
- **Effort:** Medium.

### Option C: Keep block, longhand the margin
- **Approach:** Replace the three-value `margin: 0 auto 20px` with
  `margin-left: auto; margin-right: auto; margin-bottom: 20px`, on the
  hypothesis that Apple Mail's style normalizer chokes on the three-value
  shorthand (a valid but less-common form) while handling longhands.
- **Pros:** One-line change, preserves the existing block layout exactly, and
  would confirm a specific parser hypothesis.
- **Cons:** Purely speculative — nothing in the evidence distinguishes "drops
  three-value shorthand" from "overrides image margins wholesale". If wrong,
  it looks identical to doing nothing. Leaves the codebase depending on
  `margin: auto` centering in email, which is fragile regardless of this
  client.
- **Effort:** Trivial.

## Recommendation

**Option B**, applied to the six broken templates.

The deciding factor is the cost asymmetry. Verification requires a human to
open a real message in Apple Mail — there is no CI harness for email client
rendering, and there cannot be one. A fix that is *probably* right (Option A)
costs a round-trip through someone's inbox if it isn't, and this is a defect
on the sign-in path, which is the highest-stakes email we send: it is the only
thing standing between a new user and their first session, and it is the first
brand impression for the phone-first/latent-host audience
(`docs/users/priorities.md`). Option B costs six lines of extremely
well-understood boilerplate and is correct under every hypothesis on the table,
including ones we haven't thought of, because it stops depending on client CSS
handling for the centering at all.

Option A is the sympathetic choice — it is smaller and it unifies the codebase
on one idiom — and I'd take it if we could test cheaply. Phase 1 makes testing
cheap *after the fact*, but the first verification still happens once, on
whatever we ship. Ship the one that can't be wrong.

Do **not** convert `notification_template_*.html` in this change. It is not
broken, it is a different header (40px brand bar, no `<h1>`), and rewriting a
working production template to satisfy consistency is churn on the
highest-volume email we send. Consistency is served instead by the guardrail
test in Phase 2, which accepts either safe idiom and rejects only the broken
one.

**Option D is right about the root cause and still shouldn't ship here.** The
duplication is real and it is why one CSS mistake became six defects — but the
fix for a mis-centered logo should not carry a risk of breaking template
parsing on the sign-in path. Fix the defect, land the guardrail, and file the
partial-extraction refactor as its own issue where it gets its own review and
its own revert. Recorded in Related Issues.

Reduced scope if the reviewer disagrees on markup weight: Option A plus the
same Phase 2 guardrail, with the Phase 1 Apple Mail check treated as
mandatory-before-merge rather than confirmatory.

## Open Questions

- **Does the same defect appear in iOS Mail and Outlook?** — Recommendation:
  assume yes for iOS Mail (same WebKit-based Mail rendering stack) and check it
  in the Phase 1 pass, since it costs one extra device. Don't block on Outlook;
  Option B is the idiom that satisfies it anyway, so a negative result changes
  nothing about the fix.
- **Should Apple Mail's automatic dark inversion be fixed too?** —
  Recommendation: **no, not here.** Declaring `<meta name="color-scheme"
  content="light dark">` plus a `@media (prefers-color-scheme: dark)` block
  means threading the already-generated `DarkColors` palette through every
  template and every `Render*` call site, re-checking contrast for eight
  templates in two locales, and re-verifying in each client. That is its own
  issue with its own review surface. File it as a follow-up
  (see Related Issues) and keep this PR to the alignment defect.
- **Is `password_reset_template_*.html` still worth fixing?** —
  Recommendation: yes. Prod has ~22 password accounts and the passwordless
  migration (#2571) is not finished, so the template is live; more importantly
  the fix is the same six lines and leaving one template on the broken idiom
  defeats the guardrail test.
- **Is the current 80px logo the right size given it renders at ~70 CSS px?** —
  Recommendation: leave it. The ~70px measurement is consistent across both
  clients and is explained by transparent padding in `logo.png`, not by a
  rendering bug. Resizing is a design call, not part of this fix, and would
  muddy the before/after comparison.
- **`alt="Ripls Logo"` vs `alt="Ripls"`** — Recommendation: standardize on
  `alt="Ripls"` while the line is already being edited. A screen reader
  announces the element type itself, so "Logo" is redundant, and
  `notification_template_*.html` already uses the shorter form.

## Implementation Plan

### Phase 1: Fix the header centering in all six templates
**Goal:** The logo is centered in Apple Mail, and stays centered in Gmail,
across `email_code`, `password_reset`, and `waitlist_welcome` in both locales.

**Tasks:**
- [ ] **Reproduce first.** Before editing anything, run the server locally
      (`npm run start:server`, which preflights both `gcloud auth login` and
      `gcloud auth application-default login` and prints the exact reauth
      command on failure) and request an email sign-in code to a mailbox read
      in Apple Mail on macOS. Confirm the live left-aligned logo. This is the
      before-shot and it proves the local send path reproduces what PROD sent.
      Keep test sends to a handful: dev and prod share the one real Mailgun
      domain (`ripls.app`) and therefore share the sending quota.
- [ ] Replace the header logo block with the Option B table wrapper in
      `email_code_template_en.html`, `email_code_template_es.html`,
      `password_reset_template_en.html`, `password_reset_template_es.html`,
      `waitlist_welcome_template_en.html`, `waitlist_welcome_template_es.html`.
      The `_es` files are byte-identical in this block — the en/es pair must
      move together (`docs/server/l10n.md`; `lint:go:l10n-parity` covers the
      TOML catalog, **not** template markup, so nothing will catch a
      half-applied change).
- [ ] Set `alt="Ripls"` and add the `width="80" height="80"` attributes
      alongside the CSS dimensions.
- [ ] Confirm no Go changes are needed: the templates are `//go:embed`-ed and
      rendered through the existing `Render*` methods; nothing parses the
      header markup.
- [ ] `npm run build` (the Go gate CI runs — `lint:go` then `go build ./...`)
      and `go test ./server/email/...`. Existing
      `TestTemplates_EmbedLogoRatherThanHotlinking` and the `l10n_test.go`
      render tests must stay green (they assert on copy and the absence of
      hotlinks, not on layout, so they should be unaffected).
- [ ] Re-send after the fix and confirm the logo is centered in Apple Mail.
      Repeat once for `password_reset` or `waitlist_welcome` — a second
      template proves the change is the idiom and not a one-off. Keep the
      Gmail control open throughout; attach before/after screenshots for both
      clients to the PR.
- [ ] While in Apple Mail, spot-check the off-app notification email's 40px
      brand bar. If its `display: inline-block` logo is *also* left-aligned,
      the mechanism is `cid:`-attachment-specific rather than
      `margin: auto`-specific, Option A was never viable, and
      `notification_template_*.html` joins the fix (this is the one finding
      that would widen scope — nothing else here does).

**Validates:** That the diagnosis is right — that the defect is centering
mechanism, not asset, sizing, or MIME encoding — and that the fix holds in the
client that reported it without regressing the client that worked.

### Phase 2: Guardrail so the broken idiom can't come back
**Goal:** A future template (or a copy-paste of an old one) cannot reintroduce
`margin: auto` centering on a block image.

**Tasks:**
- [ ] Lift the template map currently inlined in
      `TestTemplates_EmbedLogoRatherThanHotlinking` into one package-level
      `var allHTMLTemplates = map[string]string{…}` in `mailgun_test.go`, and
      have both tests range over it. Otherwise a ninth template gets added to
      one map and silently escapes the other — which is the same
      copy-paste failure mode that produced this bug.
- [ ] Add the new test next to the hotlink test: fail any template whose
      `<img>` style carries both `display: block` and a `margin` shorthand or
      longhand resolving to `auto`. Comment it with *why* (Apple Mail does not
      honor `margin: auto` centering on images — #2927), per the
      "explain why, not what" rule in `docs/server/conventions.md`, with no
      ephemeral phase markers (`npm run lint:ephemeral-comments`, part of
      `npm run lint`).
- [ ] Assert positively as well: each logo-bearing template's header must
      contain one of the two safe idioms (`align="center"` table wrapper, or
      `display: inline-block` under a centered parent), so a template that
      simply drops the centering entirely also fails. Assert against the raw
      embedded template strings, matching the hotlink test's precedent — the
      header contains no `{{…}}` actions, so a rendered-output assertion would
      buy nothing and would need a `MailgunService` per case.
- [ ] Add a short "Centering in email" note to `server/email/README.md`
      recording the constraint and both accepted idioms. The README is
      currently stale — it lists `password_reset_template.html` and friends
      without their `_en`/`_es` suffixes and omits `email_code`, `notification`,
      and `activity_digest` — so refresh the key-files list in the same pass.
      This is a package README, not a durable doc under `docs/`: it carries no
      `context:` frontmatter and needs no
      `npm run generate:context-map` run.

**Validates:** That the fix is durable rather than a one-time patch, and that
the next person adding an email template inherits the constraint instead of
rediscovering it in a user-feedback issue.

### Phase 3 (NOT BUILT — superseded): offline email preview
**Superseded by** `server/email/mailgun_live_test.go` +
`npm run test:server:integration:email:live`, added during Phase 1. It mirrors
the existing Twilio live-send precedent, self-skips unless `MAILGUN_LIVE_TO` is
set, and tags its delivery variables `local` so the status webhook drops the
events instead of folding manual checks into dev or prod metrics. It sends real
mail rather than writing `.eml` files, which costs quota but removes the
question the `.eml` harness would have raised — whether an offline render is
faithful to what Mailgun actually transmits. Revisit the `.eml` route only if
quota or turnaround becomes the constraint.

**Original goal:** Make "check this in Apple Mail" a local, zero-send
operation, so the next email change doesn't cost an inbox round-trip.

**Tasks:**
- [ ] Add `server/cmd/preview-emails` that renders every template (both
      locales) via the existing `Render*` methods against a `MailgunService`
      built with dummy credentials — `NewMailgunService` needs no network, as
      `TestNewMailgunService` shows — and writes one `multipart/related` `.eml`
      per template to an output directory, with `logo.png` attached inline as
      `cid:logo.png` exactly as `prepareOutbound` does.
- [ ] `open` the `.eml` files directly in Apple Mail to review the real
      rendering path (inline `cid:` attachment included) with no message sent.
- [ ] Follow the existing preview-command precedent
      (`server/cmd/preview-activity-digest`, `server/cmd/notification_examples`)
      for structure and the usage line in the package comment. Single-file
      `main.go` with a `// Command preview-emails …` comment satisfies
      `revive.package-comments`; a single-file cmd directory needs no
      `README.md` (same as `preview-activity-digest`).
- [ ] Keep the new code in `server/cmd/preview-emails/`, **not** in
      `server/email/mailgun.go` — that file is at 928 lines against the
      1,000-line `lint:go:size` gate, and it is not where a dev tool belongs.
- [ ] The command must have no send path at all: render and write files only,
      never construct a Mailgun send. A preview tool that can accidentally mail
      a real inbox is a footgun, and its sample codes are fabricated strings
      that must never look like a delivered OTP.
- [ ] Document it in `server/email/README.md`.

**Validates:** Whether the Apple Mail rendering can be reproduced faithfully
offline. If the `.eml` reproduces the *original* bug (check by rendering from
a pre-fix revision), the harness is trustworthy for future email work; if it
doesn't, that itself is the finding — it would mean the quirk depends on
something Mailgun adds in transit, and the harness should be dropped rather
than trusted.

## Related Issues

## Dark mode (shipped with this change)

Apple Mail invents a dark rendering for any message that does not declare one,
by inverting the light styles — that inversion is why the bug report's
screenshot came back dark, and it is a separate thing from the alignment
defect. Every email rendered with `LightColors` only; the generated
`DarkColors` palette existed but no email used it.

The design constraint that shapes the implementation: declaring `color-scheme`
switches the inversion *off*, so the declaration and the replacement rules must
land together. Shipping the declaration alone would be a regression — the
auto-inverted rendering in the original screenshot is imperfect but readable,
and a full-brightness email in a dark client is worse.

- The light palette stays **inline on every element** — the only form that
  survives clients which strip `<style>`.
- The shared head partial emits `<meta name="color-scheme">` plus a
  `@media (prefers-color-scheme: dark)` block generated from `DarkColors`.
  Apple Mail honors it; Gmail largely does not and keeps the light rendering it
  has always had, which is the correct fallback.
- `!important` is required throughout, since inline styles otherwise win.
- **Roles invert rather than mapping token-for-token.** In light the page is
  the tinted `Surface` with a white `Background` card on it; in dark the page
  is the deepest `Background` and the card lifts off it in `Surface`. An inset
  inside that card (the code chip, the waitlist panel) reads as `Surface`
  against white, but in dark it must clear the card it sits on, so it takes
  `Border`. Mapping each token to its own name in both themes would flatten the
  card into the page.
- Because the dark rules arrive in a stylesheet while the light values are
  inline, an element is only themed if it carries the class the rules key on.
  `TestTemplates_DeclareDarkModeForEveryColoredElement` asserts against
  *rendered* output that every element painted with a theme-varying token names
  an `r-` class — verified to fail by removing one class.

- ~~New follow-up (dark mode)~~ — **done in this change**, see above. Note
  `design_tokens_gen.go` is **generated**; the source of truth is
  `design/tokens.json` (#2441) via `npm run generate:design-tokens`, with
  `npm run lint:design-tokens` failing CI on drift.
- ~~New follow-up (duplication)~~ — **done in this change** as shared head and
  logo partials. Footers were *not* shared: their copy differs per template and
  html/template cannot express an open/close wrapper pair.
- #2571 — Passwordless email-OTP. Owns the email sign-in code flow this bug
  appears in; relevant to whether `password_reset_template_*` still matters.
- #2441 — Design tokens. Source of the `LightColors`/`DarkColors` palettes
  inlined into these templates.
- #2492 — Phone-first pivot. Introduced `notification_template_*.html`, whose
  `display: inline-block` header is the working counter-example this plan
  cites.

## Audit Notes

Audited against the documentation context map. Engaged dimensions:
`code_quality`, `testing`, `tech_debt`, `accessibility` (alt text),
`security` (light — Phase 3 tooling only). Not engaged: `api_design` (no
proto/RPC change), `performance`, `observability`, `dependency`,
`error_handling` (markup-only change; no new error paths).

Changes the audit made to the plan:

1. **Added Option D** — the code-quality rubric's DRY threshold ("a pattern
   reused 3+ times should be factored") is met by the copy-pasted header, and
   the original plan fixed the symptom in six places without naming the cause.
   Recommendation still defers it, with the reasoning made explicit and a
   follow-up issue recorded.
2. **Added a reproduce-first step to Phase 1** — the plan verified the fix but
   never established the baseline in the failing client. Also added the
   shared-Mailgun-quota caution (dev and prod both send from `ripls.app`).
3. **Added the notification-template spot-check** — the one observation that
   would invalidate the diagnosis (a `cid:`-attachment-specific quirk rather
   than a `margin: auto` one) now has an explicit, cheap check attached to the
   verification pass instead of being left implicit.
4. **Phase 2 now factors the template map** rather than duplicating it, so a
   future template can't be covered by one guard and miss the other — the
   exact failure mode under audit. Added the rationale for asserting on raw
   template strings rather than rendered output.
5. **Phase 3 hardened** — new code lands in `server/cmd/preview-emails/`, not
   in `mailgun.go` (928/1,000 lines against `lint:go:size`); the command gets
   no send path; single-file cmd needs a `// Command …` comment for
   `revive.package-comments` but no `README.md`, per the
   `preview-activity-digest` precedent.
6. **Corrected the dark-mode follow-up** — `design_tokens_gen.go` is generated
   and must not be hand-edited; `design/tokens.json` (#2441) is the source of
   truth.
7. **Build/lint commands made concrete** — `npm run build` (the same Go gate
   CI runs) and `npm run lint:ephemeral-comments`; both script names verified
   against `package.json`.

Compliance checks that passed without change: templates are `//go:embed`-ed
and rendered through existing `Render*` methods, so no service-boundary or
architecture-layer change is involved; the en/es pair requirement from
`docs/server/l10n.md` was already called out, along with the fact that
`lint:go:l10n-parity` covers the TOML catalog and not template markup; no
TODOs are introduced, so the `TODO(#NNNN)` policy is not engaged; the change
makes no claim in `docs/server/l10n.md` (which globs `server/email/**`) stale,
though the nightly `claude_refresh_docs` ratchet may still visit that doc.

Docs consulted: `docs/llms.txt`, `docs/server/architecture.md`,
`docs/server/conventions.md`, `docs/server/l10n.md`, `docs/client/design.md`
(design-token source of truth), `docs/users/priorities.md`,
`docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`,
`docs/reviews/accessibility_review_prompt.md`, `server/email/README.md`,
`package.json` (script names and gate composition).
