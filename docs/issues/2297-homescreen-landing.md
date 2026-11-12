# Issue #2297: Website: promote homescreen.html to landing page, adopt crew copy/CSS workflow, fix README drift

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2297
**Filed:** 2026-06-05 | **Last activity:** 2026-06-05
**Labels:** documentation, enhancement | **Assignees:** unassigned
**Context docs:** none

## Architecture Audit (ripls-audit)

This plan was run through `/ripls-audit` against the standard reference docs
(`docs/client/{architecture,caching,i18n,accessibility,testing}.md`,
`docs/server/{architecture,observability}.md`). **Applicability is the headline
finding:** every one of those references governs the **Flutter app** (MVVM /
Riverpod / Stash / Freezed / Flutter a11y primitives) or the **Go server**
(Connect-Go services, `slog` structured logging, health checks). Issue #2297
touches **neither** — it is a static, nginx-served marketing site (HTML + CSS +
vanilla JS) that merely POSTs to an already-deployed `JoinWaitlist` endpoint. So
the MVVM/Riverpod, repository-caching, server-service, and structured-logging
conventions do not apply and are intentionally not invoked below.

Three principles *do* transfer, and the audit added concrete tasks for each:

- **"No new functionality without test coverage"** (server/architecture testing
  philosophy). The repo already has a top-level **Playwright `e2e/`** harness
  (`e2e/tests/*.spec.ts`, e.g. `ported-smoke.spec.ts`) that drives the built web
  bundle. The plan originally validated the waitlist form — the only real
  behavior on the page — with a *manual* staging test. **Fix:** Phase 3 now adds
  an automated Playwright smoke test for the landing page + waitlist submission
  (success and already-registered paths).
- **Accessibility as a default, not an afterthought** (client/accessibility.md;
  the Flutter primitives don't apply, but the intent does). The current
  `index.html` waitlist form uses **placeholder-only inputs with no `<label>`s** —
  a real a11y defect that Option A would propagate verbatim into the new `/`.
  **Fix:** Phase 3 now requires proper (optionally visually-hidden) `<label>`s,
  Phase 2 adds an explicit contrast/decorative-image a11y pass for the dark photo
  overlays and the iMessage/phone mockups.
- **"Event", not "Experience" in user-facing strings** (CLAUDE.md). `homescreen.html`
  line ~1015 contains the phrase **"invitee experience."** The marketing site is
  user-facing, so this is in scope. **Fix:** Phase 1's copy extraction now
  includes a terminology audit to catch/replace "experience" usage.

Two scope clarifications the audit recorded (no action, stated to preempt review
questions):

- **L10n boundary.** `docs/client/i18n.md` and the established render-boundary
  (client renders in-app via ARB, server renders push/email) describe two
  catalogs. The marketing site is a **third surface**: intentionally English-only,
  outside both the ARB catalog and the server l10n system. No drift expectation —
  do **not** wire marketing copy into `app_*.arb`. (Captured as an Open Question.)
- **Observability / fail-fast.** The waitlist POST is client-side JS against an
  endpoint that already logs server-side; no server change. The existing form
  surfaces errors to the user (no silent-ignore) — Phase 3 must preserve that
  behavior when porting, which it already specifies.

## Problem Statement

The marketing site at `website/content/` has accumulated two parallel landing
experiences and an inconsistent authoring story. We want to make
`homescreen.html` the public `/` page and bring it up to the standard already
established by the `crew/` page.

Concretely, five things are wrong or missing today:

1. **Two competing landing pages.** `index.html` is the live `/` page (minimal
   copy, but a *working* waitlist form wired to the `JoinWaitlist` RPC).
   `homescreen.html` is the richer 6-section narrative page we actually want to
   ship, but it has a **dead Formspree placeholder** form
   (`action="https://formspree.io/f/YOUR_FORM_ID"`) and is linked from nowhere.
2. **Copy is locked inside HTML.** `crew/` has an editable `copy.md` source with
   a `## key` convention that gets injected back into `crew/index.html`.
   `homescreen.html` has no such source — iterating on its copy means hand-editing
   1,157 lines of HTML.
3. **CSS is duplicated, not shared.** `crew/index.html` and `homescreen.html`
   each embed a ~near-identical `<style>` block (same `:root` tokens, same
   `.section` system, same Fraunces/Inter fonts). Crew has a handful of style
   tweaks (e.g. `--overlay-mobile: 0.68` vs `0.70`, refined `.section.light`,
   dropped `--overlay-soft`) that should be unified into a shared stylesheet
   rather than living as a fork.
4. **Background imagery may not match revised copy.** `homescreen-images/01–05.jpg`
   were chosen for the current copy; if copy changes during extraction we need to
   re-confirm fit. Crew uses a more flexible data-driven `data-bg="NN"` loader
   that homescreen could adopt.
5. **README drift.** `website/README.md` documents `drafts/` and `www/`
   directories that do not exist (real root is `content/`) and a design language
   (Libre Baskerville, coral/beige/cream) that the marketing pages no longer use.

**Ambiguity to resolve up front (see Open Questions):** the issue says to align
the shared marketing CSS "with `tokens.css`." But the two marketing pages use a
*different* design system than `tokens.css`. `tokens.css` is Libre Baskerville +
sage/coral and is hand-mirrored from the Flutter app theme
(`app/lib/core/theme/`). Crew/homescreen are Fraunces + Inter + dark-overlay +
cream/rust. These are not the same system, and forcing them together is a much
larger (and riskier) effort than the issue's core ask. The plan below treats
"share CSS between crew and homescreen" as the primary deliverable and
"reconcile with `tokens.css`" as an explicitly deferred, separate question.

## Context from Comments

The issue has **no comments** — it was filed and immediately handed to planning.
All context comes from the issue body (which I authored after exploring the
directory) plus the codebase research below. No maintainer has weighed in on the
key forks (canonical-page strategy, whether to unify the two design systems), so
those are surfaced as Open Questions rather than assumed.

## Codebase Research Findings

These findings materially shape the plan:

- **Serving is static nginx, not the Go server.** `website/config/default.conf`
  uses `root /usr/share/nginx/html; index index.html;` with `try_files`. `/`
  resolves to `index.html`. So "promote homescreen to `/`" means making
  `homescreen.html`'s content *be* `index.html` (rename/replace), not a server
  routing change.
- **`embed.go` does NOT need to change for the marketing pages.** It embeds
  `shared.css`, `deeplink.css`, `tokens.css`, `event.css`, `logo.png`, and the
  `.well-known/` files — these are consumed by the **Go server** when it renders
  dynamic pages (event/deeplink/invite). The static marketing pages
  (index/homescreen/crew) are served by nginx straight off disk and are not
  embedded. A new `marketing.css` shared only by nginx-served pages therefore
  does **not** belong in `embed.go`. (The issue's "update embed.go if needed" is
  conditional — and the condition is not met unless we also make the Go server
  render a marketing page, which we are not.)
- **There is no build/generation step for the website.** `find website` shows no
  scripts, Makefile, or npm targets. `publish_website.yaml` triggers on push to
  `main` under `website/**` and runs `./.github/actions/deploy-website` (rsync
  over SSH of static files). The `crew/copy.md → crew/index.html` "inject"
  workflow is **entirely manual / Claude-driven** — there is no tool that does
  it. This is important: "establish the same workflow" for homescreen means
  replicating the same *manual* convention (a documented `copy.md` with `## key`
  headings that a human asks Claude to inject), unless we decide to invest in a
  real generator (see Open Questions).
- **The waitlist form contract is known.** `index.html` POSTs JSON
  `{ email, name?, referralSource?, intendedUse? }` to
  `https://server.ripls.org/ripls.api.WaitlistService/JoinWaitlist`, handles
  `data.alreadyRegistered`, and renders loading/success/error states. This is the
  exact integration that must replace the Formspree placeholder in homescreen.
- **Crew ≈ homescreen CSS.** The `:root` token blocks are nearly byte-identical;
  crew is a light fork. Extraction risk is low.
- **A staging website pipeline exists** (`deploy_staging_website.yaml`,
  `cleanup_staging_website.yaml`, `infra/staging-web/`), so changes can be
  validated on a staging host before they hit production `/`.

## Potential Solutions

The issue is really five loosely-coupled workstreams. The meaningful design
choice is in two of them: **how to promote** (page strategy) and **how to share
CSS** (one design system or two). I present options for those; the rest is
execution.

### Promotion strategy

**DECIDED (per maintainer, 2026-06-05):** the homescreen lives in a **sibling
folder** `content/homescreen/` (mirroring `crew/`) and is **served at `/`** via
nginx. Option A below is the chosen mechanism; B and C are recorded as the
rejected alternatives.

#### Option A: Sibling folder `content/homescreen/`, served at `/` (CHOSEN)
- **Approach:** Author the page as a true sibling of crew —
  `content/homescreen/{index.html, copy.md, background/, screens/}`. Serve it at
  the homepage with a single nginx block:
  ```nginx
  # in website/config/default.conf, before the generic `location /`
  location = / { try_files /homescreen/index.html =404; }
  ```
  All other paths (`/homescreen/background/…`, `/css/…`, `/crew/`, `/legal/`,
  `/.well-known/`, `/go/`) fall through to the existing `location /` and serve
  from disk unchanged.
- **Asset paths — use root-absolute, NOT `<base>`:** reference assets as
  `/homescreen/background/01.jpg`, `/homescreen/screens/…`, `/css/marketing.css`.
  A `<base href="/homescreen/">` was considered and **rejected**: the page uses
  in-page fragment anchors (`href="#signup"` hero CTA + `href="#"` brand logos,
  7 total), and a `<base>` would resolve `#signup` to `/homescreen/#signup`,
  navigating the URL away from `/`. With root-absolute asset paths and no
  `<base>`, fragments stay on the current page (`/#signup` ✓). Cross-site links
  are already absolute (`www.ripls.org`, Unsplash, Google Fonts, mailto), so
  nothing else needs touching. The shared `data-bg` loader takes a `/homescreen/`
  base constant (crew passes its own relative base), keeping one loader for both.
- **SEO hygiene:** add `<link rel="canonical" href="https://www.ripls.org/">` and
  an optional `location = /homescreen/ { return 301 /; }` so the folder URL
  doesn't become a duplicate-content twin of `/`.
- **Pros:** Repo layout == served layout (no deploy-time copy/drift); homescreen
  and crew are structural siblings; one canonical page at `/`; page is directly
  testable at `/homescreen/` on staging; one-line nginx change.
- **Cons:** Asset paths encode the `/homescreen/` prefix (deliberate, single
  convention); homescreen's asset refs are absolute while crew's are relative — a
  minor, justified divergence (homescreen is dual-located: file at `/homescreen/`,
  served at `/`).
- **Effort:** Small.

#### Option B (rejected): Replace the root `index.html` with homescreen content
- Flattens the page back to the top level — abandons the sibling-folder layout
  the maintainer asked for, and re-creates the "everything at root" sprawl.

#### Option C (rejected): Keep `homescreen.html` at root, redirect/`index` to it
- Two index-like files persist (the drift we're removing); a redirect adds an
  SEO/perf hop on the most important page.

### CSS-sharing strategy

#### Option D: Extract a shared `marketing.css` for crew + homescreen (recommended)
- **Approach:** Pull the common `<style>` block into
  `content/css/marketing.css`, reconcile the small crew/homescreen deltas into
  one canonical set of values, and have both pages `<link>` it (plus a tiny
  page-specific block only where they genuinely differ). Leave `tokens.css` and
  the app-aligned pages untouched.
- **Pros:** Directly satisfies the core ask ("share CSS between crew and
  homescreen"). Low risk — the two blocks are already ~identical. No impact on
  the app-design-system pages.
- **Cons:** Leaves two design systems on the site (marketing vs. app-aligned).
  That's a real inconsistency, but a *contained* and intentional one.
- **Effort:** Medium.

#### Option E: Reconcile the marketing pages onto `tokens.css`
- **Approach:** Rework crew + homescreen to use `tokens.css` variables, fonts,
  and palette.
- **Pros:** One design system site-wide; satisfies the issue's literal "align
  with tokens.css."
- **Cons:** Large redesign. The marketing pages' whole aesthetic (dark
  full-bleed photo overlays, Fraunces display serif, cream/rust) is *different by
  intent* from the app's in-product palette. This is a design decision, not a
  refactor, and risks regressing a deliberately-crafted look. High effort, needs
  design sign-off.
- **Effort:** Large.

## Recommendation

**Promotion: Option A** (sibling folder `content/homescreen/`, served at `/` via a
one-line nginx `try_files`, root-absolute asset paths, no `<base>`). This is the
maintainer's decision and keeps repo layout == served layout while making
homescreen a structural sibling of crew. Port the proven `JoinWaitlist` form
verbatim.

**CSS: Option D** (shared `content/css/marketing.css` for crew + homescreen).
The two style blocks are already near-identical, so this is low-risk and delivers
the acceptance criterion ("styles shared, not duplicated in embedded blocks").
Do **not** attempt Option E in this issue — record "unify marketing aesthetic
with `tokens.css`" as a separate, design-led follow-up. No `embed.go` change is
needed because these pages are nginx-served, not Go-rendered.

For the copy workflow, **replicate the crew convention as-is** (a documented
`copy.md` with `## key` headings, injected manually) rather than building a
generator now — it matches the existing, accepted pattern and keeps scope tight.
If we want to remove the manual step, that's a separate tooling investment worth
its own issue (see Open Questions).

Sequence the work so each phase is independently shippable and the riskiest
content change (the live `/` swap) lands last, after it can be eyeballed on
staging.

## Open Questions

- **Should the marketing pages eventually share one design system with
  `tokens.css`, or stay deliberately distinct?** — Recommendation: keep them
  distinct for now. The marketing aesthetic (dark photo overlays + Fraunces) is a
  separate intentional brand surface from the in-app palette. File a follow-up
  design issue rather than forcing it here; this issue ships a *shared marketing*
  stylesheet, not a *site-wide* one.
- **How is the homescreen served at `/` from a sibling folder?** — **RESOLVED:**
  `content/homescreen/index.html` served via nginx `location = / { try_files
  /homescreen/index.html =404; }`, with root-absolute asset paths (`/homescreen/…`,
  `/css/…`) rather than a `<base>` tag (which the page's `#signup`/`#` fragment
  anchors would break). Add a canonical tag → `/` and an optional 301 from
  `/homescreen/` → `/`. See Promotion Option A for the full mechanism.
- **Build a real `copy.md` → HTML injector, or keep it manual like crew?** —
  Recommendation: keep it manual for parity and scope control. If we do invest in
  a generator, it should serve *both* crew and homescreen and live behind an npm
  script + CI check — that's its own issue, not a rider on this one.
- **Adopt crew's `data-bg` data-driven background loader for homescreen?** —
  Recommendation: yes, as part of the shared CSS/JS extraction — it removes
  inline `style="background-image:…"` URLs, tolerates `.jpg/.webp` swaps without
  HTML edits, and makes the two pages structurally consistent. Low cost once the
  CSS is already being unified.
- **Where does homescreen's `copy.md` live?** — **RESOLVED:** `content/homescreen/`
  mirrors `crew/`'s layout exactly (`index.html`, `copy.md`, `background/`,
  `screens/`). No deploy-time copy is needed because the page is served *in place*
  from `/homescreen/` at the `/` URL (see the Promotion decision) — repo layout
  stays identical to served layout.
- **Is the marketing site expected to localize (ES), or stay English-only?** —
  Recommendation: English-only, and explicitly *out* of the app's ARB catalog and
  the server l10n system. The marketing site is a separate surface from the
  in-app strings (`app_*.arb`) and server-rendered push/email; wiring marketing
  copy into either would create a parallel catalog that drifts. If localized
  marketing is ever wanted, it's a separate effort (per-locale `copy.md` +
  rendered pages), not a rider here. Recorded so a reviewer doesn't ask why the
  new copy isn't in `app_es.arb`.
- **README scope — minimal fix or full rewrite?** — Recommendation: full rewrite.
  It's short (883 bytes) and substantially wrong; a from-scratch rewrite
  reflecting `content/`, the two marketing pages, the css/ layout, the embed.go
  boundary, and the nginx/deploy pipeline is cheaper than patching.

## Implementation Plan

### Phase 1: Settle structure + extract homescreen copy to markdown
**Goal:** A single, agreed-upon file layout for the homescreen authoring source,
and all homescreen copy living in an editable `copy.md`.
**Tasks:**
- [ ] Create `content/homescreen/` mirroring `crew/`'s layout
      (`index.html`, `copy.md`, `background/`, `screens/`); `git mv` the page and
      its media in from `homescreen.html` / `homescreen-images/` to preserve
      history. (Structure decision is settled — see Promotion Option A.)
- [ ] Create the homescreen `copy.md` with the same editor header and `## key`
      convention as `crew/copy.md` (section keys aligned to the background slots;
      closing section keyed `signup`).
- [ ] Extract all 6 sections of copy from `homescreen.html` into `copy.md`,
      preserving inline `<em>/<br>/<strong>` tags verbatim.
- [ ] Verify the HTML still renders identically after re-injecting from `copy.md`
      (no copy lost in extraction).
- [ ] **Terminology audit (CLAUDE.md "Event" not "Experience"):** while copy is in
      markdown, grep for "experience" and replace user-facing uses with "Event".
      Known hit: `homescreen.html` ~line 1015 ("invitee experience"). Re-check the
      crew copy too while the convention is fresh.
**Validates:** That the crew authoring convention transfers cleanly to the
longer homescreen page, pins down the directory structure everything else
depends on, and that no user-facing "experience" wording ships.

### Phase 2: Shared marketing CSS + data-driven backgrounds
**Goal:** Crew and homescreen consume one stylesheet; no duplicated embedded
`<style>` blocks.
**Tasks:**
- [ ] Diff the crew vs. homescreen `<style>` blocks; produce one reconciled set
      of token values and rules.
- [ ] Create `content/css/marketing.css` with the shared rules.
- [ ] Convert `crew/index.html` and the homescreen page to `<link>`
      `marketing.css`, keeping only genuinely page-specific rules inline/minimal.
- [ ] (Per Open Question) adopt crew's `data-bg="NN"` + `loadBackground()` JS in
      homescreen, replacing inline `background-image` URLs.
- [ ] Confirm `embed.go` needs no change (marketing pages are nginx-served);
      note this explicitly in the README/PR.
- [ ] Visual-diff both pages before/after on the staging website to confirm no
      regression.
- [ ] **Web a11y pass on the shared styles:** confirm the dark-overlay text meets
      WCAG AA contrast against the photo backgrounds (the `--overlay*` values
      exist for exactly this — verify, don't assume); mark purely decorative
      imagery (phone frames, iMessage mock, background photos) with `alt=""` /
      `aria-hidden="true"` so screen readers skip them; give the logo meaningful
      `alt` text.
**Validates:** The two near-identical style blocks really do unify without visual
regression, the shared-CSS acceptance criterion holds, and the marketing pages
are screen-reader-sane.

### Phase 3: Wire the real waitlist form + promote to `/`
**Goal:** `/` serves the homescreen content with a working signup; the Formspree
placeholder is gone.
**Tasks:**
- [ ] Port the `JoinWaitlist` form integration from `index.html` (the JS handler,
      payload shape, loading/success/`alreadyRegistered`/error states) into the
      homescreen signup section, replacing the Formspree `action`. **Preserve the
      user-visible error surfacing** (no silent-ignore) when porting.
- [ ] **Fix the form a11y while porting (do not copy the defect):** the current
      `index.html` inputs are placeholder-only with no `<label>`. Add a `<label>`
      per input (visually-hidden is fine), associate via `for`/`id`, and keep the
      placeholder only as a supplement, not the sole accessible name.
- [ ] Preserve footer links currently unique to `index.html` (beta retrospective,
      privacy policy).
- [ ] Convert the homescreen's asset references to root-absolute paths
      (`/homescreen/background/…`, `/homescreen/screens/…`, `/css/marketing.css`)
      so the page works when served at `/`; thread a `/homescreen/` base constant
      into the shared `data-bg` loader. Add `<link rel="canonical"
      href="https://www.ripls.org/">`. Do **not** add a `<base>` tag (breaks the
      `#signup`/`#` fragment anchors).
- [ ] Execute the promotion (Option A): add `location = / { try_files
      /homescreen/index.html =404; }` to `website/config/default.conf` (before the
      generic `location /`), plus optional `location = /homescreen/ { return 301
      /; }`. Retire the old root `index.html` and confirm nothing links to a
      now-dead path.
- [ ] **Add a Playwright e2e smoke test** in the existing `e2e/` harness for the
      landing page: it renders, the form is present and labelled, and a submit
      exercises the success and already-registered branches (stub/intercept the
      `JoinWaitlist` response so the test doesn't depend on live backend state).
      Mirror the structure of `e2e/tests/ported-smoke.spec.ts`.
- [ ] Deploy to staging website and run the smoke test (plus one real manual
      submit against the dev waitlist endpoint) before promoting to production `/`.
**Validates:** The most important page works for a real user (signup actually
hits the backend), is accessible, and is covered by an automated test — not just
a one-off manual check — before it goes live at `/`.

### Phase 4: Background-image fit + structural unification
**Goal:** Imagery matches the (possibly revised) copy; the two marketing pages
are structurally consistent.
**Tasks:**
- [ ] Review each homescreen section's background against its final copy; swap or
      re-source any that no longer fit.
- [ ] Reconcile divergent media/asset layouts where reasonable
      (`homescreen-images/` vs `crew/background/`+`crew/screens/`), per the chosen
      Phase 1 structure.
- [ ] Remove any now-dead files (old placeholder form remnants, unused images).
**Validates:** The page reads coherently (copy + imagery aligned) and the
content tree is tidy.

### Phase 5: README rewrite
**Goal:** `README.md` matches reality.
**Tasks:**
- [ ] Rewrite `website/README.md`: real `content/` structure, the canonical
      landing page, the crew + homescreen `copy.md` authoring convention, the
      `css/` layout and the `marketing.css` vs `tokens.css` boundary, the
      `embed.go` (Go-served) vs nginx-served distinction, and the
      publish/staging deploy pipeline.
- [ ] Remove references to non-existent `drafts/`/`www/` and the stale
      Libre-Baskerville/coral design notes; document the *actual* current design
      language(s), noting the intentional marketing-vs-app split.
- [ ] If a `tokens.css`-unification follow-up is filed, link it from the README.
**Validates:** A new contributor can read the README and correctly find/edit copy
and styles without spelunking.

## Related Issues
- _(none found referencing the website landing/copy workflow directly)_ — if a
  follow-up is filed for unifying the marketing aesthetic with `tokens.css` or
  for a real `copy.md` injector/generator, link them here.
