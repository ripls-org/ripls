# Issue #1583: Create 1-month retrospective / update page for private beta

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1583
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** none | **Assignees:** maintainer
**Context docs:** none (issue references `docs/users/` priorities and `analytics.ripls.org` implicitly)

## Audit Findings (2026-04-29)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability,web}.md`. Most do not apply —
this deliverable is a static marketing/retrospective page in
`website/content/`, not a Flutter feature, not a Go RPC, not a piece of
the data layer. The audit findings that *do* apply are folded into the
plan below; the highlights:

- **Web accessibility basics are not yet called out and should be.** The
  existing pages under `website/content/` set the precedent:
  `<html lang="en">`, alt-text on every image (`alt="Ripls Logo"`),
  proper `<h1>/<h2>/…` hierarchy, real `<header>/<section>` landmarks,
  and a single shared CSS palette. The retrospective must keep these
  invariants and add per-image alt text describing the chart or feature
  shown ("Weekly new communities, April 2026" — not "Chart 1").
- **GIFs as the recommended visual format is the wrong call.** Use
  `<video autoplay muted loop playsinline>` with `.webm`/`.mp4`
  sources instead. Reasons: (a) substantially smaller files than GIFs
  at the same visual quality; (b) `prefers-reduced-motion` users can
  get a static poster frame for free via the `poster` attribute and a
  CSS media query that pauses autoplay; (c) `aria-label` provides a
  description that GIFs cannot. Phase 3 is updated accordingly.
- **No i18n integration.** The Flutter app's ARB system
  (`docs/client/i18n.md`) is irrelevant here — the `website/content/`
  site is English-only and that's a deliberate, current state. We
  should *not* introduce ARB into the static site for this one page.
  If a Spanish version is wanted later, the right move is a sibling
  `beta-1-month-es.html`, not a localization framework.
- **No server observability work.** Option A is static; no handler is
  added, so `docs/server/observability.md` does not apply. If we ever
  switch to Option C, the structured-logging conventions
  (`logging.LoggerWithContext`, masked PII, request IDs) become
  load-bearing — flagged here so a future maintainer doesn't add a
  `log.Printf` and ship it.
- **Tester-credit consent is missing from the plan.** Even
  first-name-only credits go on a public page — at least one beta
  tester will reasonably prefer not to be named. Phase 1 is updated
  to include an explicit consent pass.
- **Small-N privacy is missing from the plan.** Aggregate charts
  must suppress buckets where N is small enough to identify
  individuals (e.g., a community with one active member, or a metro
  with two communities). Phase 2 is updated to require a small-N
  suppression policy before any chart is exported.
- **No website-level lint/test in CI.** `npm run build` runs Go lint
  and `go build`. The website does not currently have its own
  lint/HTML-validation step. The plan now calls out manual link/image
  verification and a recommendation to run an HTML validator (e.g.
  `tidy -e -q`) and a Lighthouse pass before merge.

The rest of the plan is structurally sound for a deliverable of this
shape.

## Problem Statement

One month after the 1 April 2026 private beta launch (release 0.10.0), we want
a single, visually rich retrospective webpage — roughly 2,000 words — that
recaps what's happened since launch. The page has four jobs:

1. **Anchor.** Remind the reader what ripls is and who it's for, drawing on
   `docs/users/priorities.md` and `docs/users/latent-host.md` so the page
   stands on its own for someone who only loosely remembers the launch.
2. **Tell the production story.** Show — with charts, graphs, and tables —
   what real circles have actually done in the last month: communities
   created, gear listed, requests opened, experiences run, loans completed,
   giveaways, rallies, photos shared. Data comes from `analytics.ripls.org`
   (Metabase, accessible via the `ripls-analytics` skill).
3. **Tour the work.** Walk through the major features shipped and bugs
   fixed across releases 0.11.0 → 0.17.0 (eight releases in four weeks),
   leaning on screenshots, videos, and GIFs already attached to the
   relevant PRs.
4. **Thank the testers.** Call out, by first name, the beta testers who
   filed bugs and tested fixes. The mapping of email → first name will be
   provided by Bryan; the issue/PR comments and `reported by <handle>`
   tags in release notes are the source of who-did-what.

The page lives in the existing static website (`website/content/`) and
should match the established sunset-coral / warm-beige / cream aesthetic
with Libre Baskerville headings.

## Context from Comments

The issue has **zero comments** — it's a fresh ask from Bryan filed today.
Two implications:

- The shape of the page is whatever we make it; no one else has weighed in
  on tone, structure, or scope.
- The "I can provide login info" (analytics) and "I can provide a mapping
  of emails to first names" notes mean two inputs will arrive from Bryan
  during execution. The plan needs to account for blocking on those.

The branch `beta_update_page` already exists but currently has no commits
ahead of `main` — it's a placeholder waiting for this plan to land before
work begins.

## Codebase reality check

- **Releases since beta:** 0.10.0 (launch), 0.11.0, 0.12.0, 0.12.1, 0.12.2,
  0.13.0, 0.13.1, 0.14.0, 0.15.0, 0.16.0, 0.17.0 — eleven release notes
  files, the most recent dated yesterday. Each has both an `.html` long
  form and a `.txt` short form already linkable as receipts.
- **Website conventions:** Static HTML pages live in `website/content/`,
  served via `website/embed.go`. CSS in `css/shared.css` plus per-page
  stylesheets. Existing `release-notes.html` template (in
  `website/templates/`) is the closest stylistic precedent.
- **Tester credit precedent already exists.** Release notes in
  `website/content/release-notes/*.txt` already credit reporters by
  handle (e.g. "reported by matt", "reported by thomas", "reported by
  lesliehall"). We can mine these directly.
- **Analytics surface.** The `ripls-analytics` skill already documents
  the full access path — API keys live in GCP Secret Manager
  (`metabase-admin-api-key` in `ripls-dev` / `ripls-prod`), pulled via
  `gcloud secrets versions access latest` and sent as `x-api-key` against
  `analytics-dev.ripls.org` / `analytics.ripls.org`. The skill also
  covers discovering table/field IDs, creating saved questions
  (database ID `2`, dashboard ID `2`), and adding cards to a dashboard.
  **No external login is needed from Bryan.** There is no existing
  retrospective dashboard — we'll create one.
- **PR media.** GitHub PR descriptions in this repo regularly include
  screenshots and short screen recordings (the streaming-AI work, the
  time-poll UI, the planning Plan tab, the calendar icon, etc.). These
  are the raw material for the visual tour.

## Potential Solutions

### Option A: Single static HTML page in `website/content/`
- **Approach:** Add `website/content/beta-retrospective.html` (or
  `1-month-update.html`) following the existing release-notes styling.
  Embed analytics charts as static PNG/SVG exports stored under
  `website/content/assets/retrospective/`. GIFs of new features live in
  the same directory. Hand-write the prose. Link from the homepage's
  footer or a new "Updates" section.
- **Pros:**
  - Matches every other page on the site — one file, one review, one
    deploy. No new infrastructure.
  - Static charts mean the page is a fixed snapshot of the 30-day window,
    which is exactly what a retrospective should be — it shouldn't
    silently update three months later.
  - Works offline / through CDN / from email clients that fetch images.
- **Cons:**
  - Hand-curating 2,000 words plus a dozen-plus visuals is real work.
  - Chart images must be regenerated and re-uploaded if the underlying
    data is corrected before publish.
- **Effort:** Medium (~1-2 days of focused work, gated on analytics +
  name mapping inputs from Bryan).

### Option B: Same static page, but generated from a Markdown source
- **Approach:** Author the retrospective in
  `website/content/retrospective/2026-05-beta-1-month.md` and add a
  small Go-template build step (or one-time script) that renders it to
  HTML. Charts and GIFs still live alongside as binary assets.
- **Pros:**
  - Markdown is easier to draft, review, and diff than raw HTML.
  - If we plan to do this monthly or quarterly, the template pays off
    after the second one.
- **Cons:**
  - Introduces a new build step the site doesn't currently need. The
    rest of the site is hand-authored HTML; one Markdown file mid-stream
    is an inconsistency that future contributors will trip on.
  - Premature given we have one of these to write, not ten.
- **Effort:** Medium-high (page + small templating system + integrating
  it with `embed.go` / serving). Not justified for a one-shot.

### Option C: Server-rendered page that pulls live analytics
- **Approach:** Add a Go handler that queries Metabase (or our own DB
  read replica) at request time, rendering current numbers into a
  template. Visual tour + tester credits stay static.
- **Pros:**
  - Numbers are always current.
  - Could become the foundation for a public "stats" page later.
- **Cons:**
  - A retrospective for a *specific window* shouldn't drift after publish.
    Live data defeats the purpose.
  - Adds a Metabase dependency, auth handling, caching, error states, and
    rate-limit risk against analytics.ripls.org.
  - Significantly more code than the deliverable warrants.
- **Effort:** Large. Wrong fit for a snapshot retrospective.

## Recommendation

**Option A.** Static HTML page with embedded chart images and GIFs, written
by hand, matching the existing site aesthetic. A one-month retrospective is
inherently a snapshot — the value is in the editorial framing, not in the
data being live. Option A also costs the least, ships the soonest, and
doesn't introduce a Markdown→HTML build step the site doesn't otherwise
need.

The work splits cleanly into research (mine PRs + analytics + tester list),
asset production (capture/convert short videos, export charts), and
writing/layout. Research and asset production can run unblocked —
analytics access already lives in the `ripls-analytics` skill. The only
external input needed from Bryan is the email→first-name mapping (with
consent decisions) for the tester-credits section, which arrives **after**
a draft tester list is assembled rather than at kickoff.

## Open Questions

- **Where does this page live in the site?** *Recommendation:* Add it as
  `website/content/beta-1-month.html` and add a single inline link from
  `index.html` (footer or hero sub-link) plus a link in the next release
  notes. A new "Updates" section is overkill for one page.
- **Public or gated?** *Recommendation:* Public, indexable. The beta is
  private but the *story* of the beta is excellent fundraising and
  recruiting material. Anything sensitive (specific user names beyond
  first-name credits, raw user counts if low, individual community
  details) gets aggregated or omitted. We are not naming circles.
- **What counts as a "major feature" worth touring?** *Recommendation:*
  Anything that landed end-user-visible UI or notification copy in
  releases 0.11–0.17. From a quick scan of the release notes that's
  roughly: streaming AI generation, the planning Plan tab,
  needs/contributions, time polls, generalized undo, requests/offers
  on experiences, per-community notification preferences, the inline
  "Add to calendar" icon, single-player mode polish, Gear-from-URL
  improvements, chat resume / reconnect gap-fill, accessibility
  pass. Aim for ~6-8 features with screenshots/GIFs and a one-line
  description each — the rest get a "and a hundred other fixes" line
  with a link to the release notes index.
- **Tester credits — how, exactly?** *Recommendation:* A single
  "Thanks to" section near the end. List first names alphabetically.
  Don't tie any individual person to any individual bug — that's both
  invasive and unnecessary. If Bryan wants higher-fidelity callouts
  ("Matt caught the chat deep-link bug"), make it opt-in: the default
  is the alphabetical roll-call.
- **Word count.** *Recommendation:* 2,000 words is the issue's number.
  Treat it as a soft target. The page is going to feel image-heavy
  regardless, and a retrospective that's mostly visuals + 1,500 words
  of taut copy beats one that's mostly visuals + 2,400 words of filler.
- **Which charts are the headline charts?** *Recommendation:*
  - Communities created over time (cumulative + weekly new).
  - Active circles and members week-over-week.
  - Gear listed cumulative, by community.
  - Requests opened / fulfilled / rate.
  - Experiences scheduled / completed / RSVPs.
  - Loans + giveaways completed (the Tier-1 atomic-offer evidence).
  - Aggregate impact: hours saved / kg CO₂e / dollars saved (already
    computed by the impact estimator — see `MEMORY.md` notes; pulls
    from production transactions).
  Skip anything where N is too small to be honest.
- **Tone and voice.** *Recommendation:* Match `priorities.md` —
  declarative, warm, specific, no marketing puffery. The product
  speaks plainly about "the parent who would host the potluck"; the
  retrospective should speak the same way. No stock photos, no
  emoji, no "amazing journey" language.
- **Visuals: GIFs vs. videos?** *Recommendation (revised by audit):*
  Use `<video autoplay muted loop playsinline preload="metadata"
  poster="…">` with `.webm` (VP9 or AV1) primary source and `.mp4`
  (H.264) fallback. Smaller files than GIF at the same quality, every
  modern browser autoplays muted videos, and `prefers-reduced-motion`
  can be honored via CSS (`@media (prefers-reduced-motion: reduce) {
  video { animation: none; }` plus a JS hook that pauses autoplay,
  leaving the poster frame visible). Each `<video>` gets an
  `aria-label` describing the feature shown. Hosted under
  `website/content/assets/retrospective/`. Convert from PR videos with
  `ffmpeg`; target ~500 KB per asset and a height of 480–600 px.

## Implementation Plan

### Phase 1: Research and asset gathering
**Goal:** Have every input collected before we write a single paragraph.
**Tasks:**
- [ ] Read every release notes file from 0.11.0 through 0.17.0; build a
      shortlist of 6–8 features worth visual tours plus a "rest of the
      change log" bucket.
- [ ] For each shortlisted feature, locate the originating PR(s), pull
      screenshots/videos, and stage them under
      `website/content/assets/retrospective/raw/`.
- [ ] Build a candidate list of beta testers from `reported by <handle>`
      tags in release notes plus issue commenters between 2026-04-01
      and today.
- [ ] **Consent pass for tester credits.** For everyone on the
      candidate list, send a single short opt-in message ("we'd like
      to thank you by first name on a public retrospective page —
      reply NO if you'd rather we didn't"). Default is *not to
      include* anyone who hasn't been reached. Bryan owns this step
      since he holds the email→first-name mapping; the plan only
      includes confirmed names.
- [ ] Once a draft tester list is assembled (handles + commit/issue
      attribution), hand it to Bryan and request: (a) the email →
      first-name mapping for the people on that list; (b) consent
      decisions per (a). Analytics access is **not** a request —
      the `ripls-analytics` skill already provides it.
**Validates:** That we have enough material, before sinking time into
prose. If the answer is "no — not enough activity to fill 2,000 words
honestly," we surface that to Bryan immediately.

### Phase 2: Build the analytics charts
**Goal:** Produce the static chart images for the production-story
section.
**Tasks:**
- [ ] Use the `ripls-analytics` skill to create or clone the headline
      questions in Metabase: communities, members, gear, requests,
      experiences, loans, giveaways, aggregate impact, all bounded to
      2026-04-01 → 2026-04-30.
- [ ] Group them into a "1-month beta retrospective" dashboard so the
      raw cuts are reproducible later.
- [ ] **Define and apply a small-N suppression policy** before
      exporting any chart: any bucket (week, metro, community) with
      fewer than a threshold (proposed: 5) users either rolls up into
      a larger bucket or is omitted. Run the policy through every
      candidate chart and discard any chart that survives only because
      it leaks small-N detail.
- [ ] Export each chart as PNG (or SVG where it renders cleanly) at
      retina resolution; store under
      `website/content/assets/retrospective/charts/`. Each export
      gets a descriptive filename (`weekly-new-communities.png`, not
      `chart-1.png`) that doubles as the basis for the page's
      `alt` text.
- [ ] Spot-check every number against a second source (production DB
      directly, or another Metabase question) so we don't publish wrong
      counts.
**Validates:** That the analytics story is publishable — sometimes the
honest answer is a chart looks too sparse to include, in which case it
gets cut here, not after the page is laid out.

### Phase 3: Convert and prepare visual assets
**Goal:** All inline visuals are final, sized, lightweight, and
accessible.
**Tasks:**
- [ ] Convert each PR video to a 3–6s `.webm` (VP9 or AV1) primary
      plus `.mp4` (H.264) fallback at target ~500 KB and consistent
      height (480–600 px). Capture a representative still as the
      `poster` frame.
- [ ] Crop and pad screenshots to a consistent device frame where it
      reads better in-page.
- [ ] Optimize all PNGs/JPGs (`oxipng`, `cwebp`) so the page weight
      stays modest.
- [ ] Draft `alt` text for every chart and screenshot, and
      `aria-label` for every video. Labels describe content
      ("Weekly new communities, April 2026"), not position.
**Validates:** The page can be embedded in an email or opened on a
slow connection without falling over, and a screen reader can describe
every visual.

### Phase 4: Write and lay out the page
**Goal:** Ship `website/content/beta-1-month.html`.
**Tasks:**
- [ ] Author the four sections (Anchor / Production story / Feature
      tour / Thanks) in a working draft alongside the assets. Aim
      ~1,500–2,000 words.
- [ ] Match the existing static-page invariants: `<html lang="en">`,
      single `<h1>` with a sensible `<h2>/<h3>` hierarchy, real
      landmarks (`<header>`, `<main>`, `<section>`), every
      `<img>` carrying meaningful `alt` text (decorative images use
      `alt=""`), every `<video>` carrying an `aria-label`. Add a
      `prefers-reduced-motion` media query that pauses autoplay
      videos and falls back to the poster frame.
- [ ] Add a per-page CSS file (`css/retrospective.css`) only if needed;
      otherwise reuse `css/shared.css` and any styles already present
      on `release-notes.html`. Verify color contrast against the
      sunset-coral / warm-beige / cream palette meets WCAG AA for body
      text and headings.
- [ ] Link from `index.html` (single inline link, footer or hero
      sub-link).
- [ ] Add a `.txt` companion under `website/content/release-notes/` if
      we want a plain-text equivalent for plain-text email — *optional;
      probably skip*.
- [ ] Local preview by running the website embed/server, click every
      link and image, watch every video, on both desktop and mobile.
- [ ] Run `npm run build` to confirm the Go build still passes (the
      website embed list in `website/embed.go` may need a new entry
      if the page references new shared assets).
- [ ] Run `tidy -e -q website/content/beta-1-month.html` (or an
      equivalent HTML validator) and fix any errors. There is no
      website-level lint in CI; this is the manual substitute.
- [ ] Run a Lighthouse pass (Performance, Accessibility, Best
      Practices, SEO); target ≥ 95 on Accessibility.
**Validates:** The page renders, reads cleanly, and matches site
aesthetic on phone + laptop.

### Phase 5: Review and ship
**Goal:** Bryan approves; page goes live.
**Tasks:**
- [ ] Push branch, open a PR, link the rendered preview (or attach
      screenshots if no preview environment exists).
- [ ] Iterate on copy with Bryan.
- [ ] Merge, deploy, and share the URL with the beta cohort.
**Validates:** The retrospective is live, accurate, and on-brand.

## Related Issues

- None directly. The closest neighbors are individual feature/bug PRs
  the page will reference (e.g. #1553 notification preferences, #1567
  community streaming, #1550 calendar icon, #1207/#1228 generalized
  undo, #1157 streaming AI generation), but the retrospective is a
  net-new piece of work without a parent epic.

---

## Regeneration Runbook

This section is a self-contained recipe for rebuilding the page at a
later checkpoint (e.g. "the 6-month retrospective" or "after the next
public release"). Every step has been validated by the work that
produced the v1 page on 2026-04-29.

The page itself lives at `website/content/beta-1-month.html`. For a
new retrospective, copy it to `website/content/beta-<period>.html`
and follow the steps below. All four content surfaces — numbers,
visuals, feature copy, tester credits — are independent and can be
refreshed in any order.

### Prerequisites

Tools (all already used in v1):

- `gh` CLI authenticated against the `Type2Fun-Foundation/ripls` repo.
- `gcloud` CLI authenticated for the `ripls-prod` project (used to
  pull the Metabase API key from Secret Manager).
- `ffmpeg` 8.x with `libvpx-vp9` and `libx264` (Homebrew default).
- `sips` (built-in on macOS) for PNG resizing.
- `jq`, `curl`, `xargs`, `file`, standard POSIX tools.

GitHub session cookie (for fetching PR media — the REST API does
*not* expose user-attachments and a token alone won't work):

1. Open `github.com` in a browser tab where you're logged in.
2. DevTools → Application/Storage → Cookies → `https://github.com`.
3. Copy the value of the `user_session` cookie (long alphanumeric).
4. Save it to `~/.gh-user-session` as a single line.

The cookie file is in `.gitignore`; never commit it. It expires
about every 14 days — refresh by re-pasting if `fetch_pr_media.sh`
starts returning HTTP 404.

### Step 1 — Refresh production numbers

Pull all the headline numbers in a single SQL pass against the prod
Metabase database (id `2`). The window endpoints are unix seconds —
**not milliseconds, not microseconds.** Compute them with
`date -u -j -f "%Y-%m-%d %H:%M:%S" "<date>" +%s`.

```bash
METABASE_PROD_KEY=$(gcloud secrets versions access latest \
  --secret=metabase-admin-api-key --project=ripls-prod)

# Window: edit these two lines for the new period.
T0=1775347200    # 2026-04-01 00:00:00 UTC
T1=1777939200    # 2026-04-30 00:00:00 UTC

# Headline counts (life-of-product totals + window-only deltas).
cat > /tmp/q.sql <<EOF
SELECT
  (SELECT COUNT(*) FROM "user" WHERE simulation_id = '' AND deleted_deleted_at_unix_sec IS NULL) AS users_total,
  (SELECT COUNT(*) FROM "user" WHERE created_at BETWEEN $T0 AND $T1 AND simulation_id = '' AND deleted_deleted_at_unix_sec IS NULL) AS users_window,
  (SELECT COUNT(*) FROM community WHERE (simulation_id = '' OR simulation_id IS NULL) AND deleted_deleted_at_unix_sec IS NULL) AS communities_total,
  (SELECT COUNT(*) FROM gear WHERE deleted_deleted_at_unix_sec IS NULL) AS gear_total,
  (SELECT COUNT(*) FROM experience WHERE deleted_deleted_at_unix_sec IS NULL) AS experiences_total,
  (SELECT COUNT(*) FROM experience WHERE state = '4' AND deleted_deleted_at_unix_sec IS NULL) AS experiences_completed,
  (SELECT COUNT(*) FROM transfer WHERE deleted_deleted_at_unix_sec IS NULL) AS transfers_total,
  (SELECT COUNT(*) FROM transfer WHERE transfer_type = '1' AND state = '4' AND deleted_deleted_at_unix_sec IS NULL) AS loans_completed,
  (SELECT COUNT(*) FROM transfer WHERE transfer_type = '2' AND state = '4' AND deleted_deleted_at_unix_sec IS NULL) AS giveaways_completed,
  (SELECT COUNT(*) FROM request WHERE deleted_deleted_at_unix_sec IS NULL) AS requests_total,
  (SELECT COUNT(*) FROM request WHERE state = '4' AND deleted_deleted_at_unix_sec IS NULL) AS requests_fulfilled,
  (SELECT COUNT(*) FROM chat_message) AS chat_messages,
  (SELECT COUNT(*) FROM experience_rsvp) AS rsvps,
  (SELECT COUNT(DISTINCT m.id) FROM media m
    JOIN "user" u ON u.id = m.user_id
    WHERE m.deleted_deleted_at_unix_sec IS NULL
      AND (m.source_stock_image_id IS NULL OR m.source_stock_image_id = '')
      AND u.simulation_id = ''
      AND u.deleted_deleted_at_unix_sec IS NULL
      AND m.user_id IN (
        SELECT g.owner_id FROM gear g JOIN community_gear cg ON cg.gear_id = g.id
          WHERE g.deleted_deleted_at_unix_sec IS NULL AND cg.deleted_deleted_at_unix_sec IS NULL
        UNION SELECT e.owner_id FROM experience e JOIN community_experience ce ON ce.experience_id = e.id
          WHERE e.deleted_deleted_at_unix_sec IS NULL AND ce.deleted_deleted_at_unix_sec IS NULL
        UNION SELECT r.requester_id FROM request r JOIN community_request cr ON cr.request_id = r.id
          WHERE r.deleted_deleted_at_unix_sec IS NULL AND cr.deleted_deleted_at_unix_sec IS NULL
      )) AS photos_attached_to_shared_items
EOF
QRY=$(jq -Rs . < /tmp/q.sql)
curl -s -X POST "https://analytics.ripls.org/api/dataset" \
  -H "x-api-key: $METABASE_PROD_KEY" -H "Content-Type: application/json" \
  -d "{\"database\":2,\"type\":\"native\",\"native\":{\"query\":$QRY}}" | jq
```

Aggregate impact across all completed activity:

```bash
cat > /tmp/q.sql <<'EOF'
SELECT
  (SELECT COALESCE(SUM(impact_estimate_money_saved_value_usd_mean),0) FROM transfer  WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_money_saved_value_usd_mean),0) FROM experience WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_money_saved_value_usd_mean),0) FROM request    WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) AS money_saved_usd,
  (SELECT COALESCE(SUM(impact_estimate_emissions_prevented_manufacture_avoide_b33440e2),0) FROM transfer  WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_emissions_prevented_manufacture_avoide_b33440e2),0) FROM experience WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_emissions_prevented_manufacture_avoide_b33440e2),0) FROM request    WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) AS co2_grams_avoided,
  (SELECT COALESCE(SUM(impact_estimate_time_saved_minutes_mean),0) FROM transfer  WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_time_saved_minutes_mean),0) FROM experience WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) +
  (SELECT COALESCE(SUM(impact_estimate_time_saved_minutes_mean),0) FROM request    WHERE state='4' AND deleted_deleted_at_unix_sec IS NULL) AS minutes_saved
EOF
```

**Schema gotchas (validated, easy to forget):**

- All `*_unix_sec` columns are unix **seconds**. Don't multiply by 1000.
- Protobuf-enum columns (`state`, `transfer_type`) are stored as **text
  representations of integers** — `'1'`, `'2'`, `'4'`, etc. Bare integers
  give `operator does not exist: text = integer`. Mapping for the enums
  used above: `TransferType` LOAN=1, GIVEAWAY=2; `TransferState`/
  `RequestState`/`ExperienceState` ACTIVE=1/3, COMPLETED=4, CANCELLED=5
  (see `proto/ripls/models/{transfer,request,experience}.proto`).
- `user.simulation_id = ''` filters out load-test users; `community`,
  `gear`, etc. need `(simulation_id = '' OR simulation_id IS NULL)`.
- `community_user`, `community_gear`, `community_experience`,
  `community_request`, etc. all carry `deleted_deleted_at_unix_sec` —
  always filter NULL.
- `media` has **no foreign key** to gear/experience/request. The
  linkage lives inside each parent entity's `binary_proto`. Use the
  uploader-was-an-active-sharer approximation above; the alternative
  ("decode every binary_proto") is not SQL-queryable.
- Raw `COUNT(*) FROM media` returns ~5× the truthful "photos shared"
  number because it includes server-fetched stock imagery
  (`source_stock_image_id` set) and simulation uploads.

Drop the resulting numbers into the four places they appear in
`beta-<period>.html`:

- The inline-stats sentence in the "What it added up to" section.
- The `.impact-row` (three cards: $, kg CO₂e, hrs).
- Any prose claims that cite specific numbers.

### Step 2 — Inventory PR media

Build a sorted list of merged PRs in the window that carry image or
video attachments in their bodies. The xargs parallelism matters —
one-call-per-PR sequentially times out around 200 PRs.

```bash
gh pr list --state merged --base main --limit 400 \
  --search "merged:>=<window-start>" \
  --json number --jq '.[].number' > /tmp/prs.txt

cat > /tmp/scan_one.sh <<'EOF'
#!/bin/bash
pr=$1
body=$(gh pr view $pr --json body,title --jq '.title + "|" + .body' 2>/dev/null)
title=$(echo "$body" | head -1 | cut -d'|' -f1)
count=$(echo "$body" | grep -oE 'user-attachments/assets/[a-f0-9-]+' | wc -l | tr -d ' ')
[ "$count" -gt 0 ] && printf "%s\t%s\t%s\n" "$pr" "$count" "$title"
EOF
chmod +x /tmp/scan_one.sh

> /tmp/pr_media.tsv
xargs -P 12 -I {} /tmp/scan_one.sh {} < /tmp/prs.txt >> /tmp/pr_media.tsv
sort -t$'\t' -k2 -nr /tmp/pr_media.tsv | head -30
```

Top PRs by attachment count are the obvious sizzle candidates. v1
shipped 13 visuals across 11 features pulling from PRs #1131, #1192,
#1201, #1245, #1228, #1140, #1139, #1473, #1168, #1102, #1553, and
#1550.

### Step 3 — Fetch the assets

```bash
# Top N PRs from the inventory above (or pass an explicit list).
awk -F'\t' '{print $1}' /tmp/pr_media.tsv | xargs scripts/fetch_pr_media.sh
```

This downloads to `/tmp/pr-media/<pr>/<uuid>.<ext>`. Source files are
typically 5–80 MB each (raw screen captures), so total disk use is in
the 1–5 GB range. The script writes nothing under git-tracked paths.

### Step 4 — Convert to web-friendly variants

```bash
scripts/convert_pr_media.sh
# → /tmp/pr-media-web/<pr>/<uuid>.{webm,mp4,jpg,png}
```

Each video produces three files: `.webm` (VP9, ~600 KB each),
`.mp4` (H.264 fallback, similar), and `.jpg` poster from the first
frame. Each PNG is resized to ≤720 px wide via `sips`. The `format=yuv420p`
filter is required — libvpx-vp9 rejects GIF's native `gbrap` pixel
format, and H.264 needs `yuv420p` for broad device support. Total
output is ~50 MB across all PRs.

### Step 5 — Curate visually

You can't pick the best representatives without looking at them — PR
titles do not always match what's actually shown. The poster JPG from
each video is the fast preview. With Claude Code's Read tool:

```
Read /tmp/pr-media-web/<pr>/<uuid>.jpg
```

…or open them in Finder. Aim for ~12–18 visuals total, one per
headline feature, picking whichever clip *demonstrates* the feature
(claim mechanics > generic event card; "Agree on Time" Poll badge >
plain Plan tab).

When picked, stage to the page's media folder:

```bash
DST=website/content/assets/beta-<period>
mkdir -p "$DST"
# For each pick, copy the .webm + .mp4 + .jpg (videos) or .png
# (still images) using descriptive filenames:
#   plan-tab-claim, time-poll, undo-giveaway, streaming-create,
#   calendar-icon, watchlist, notif-banner, notif-prefs,
#   chat-in-context, invite, progressive-feed, swipe-content, etc.
```

### Step 6 — Refresh tester credits

External testers come from two places:

```bash
# (a) In-app feedback bot submissions — body has a "Submitted By" email.
gh issue list --state all --search "author:app/ripls-feedback-bot created:>=<window-start>" \
  --limit 200 --json body \
  --jq '.[] | .body | capture("Submitted By\\s*\\n\\n(?<e>[^\\s]+)").e' | sort -u

# (b) "reported by <handle>" tags in release notes.
grep -hoE "reported by [a-z0-9.@_-]+" website/content/release-notes/*.txt | sort -u
```

Map each email/handle to a first name — only Bryan can do that.
Default to *omitting* anyone whose first-name mapping or consent is
unclear. Internal test users (`ripls.org` addresses), co-founders, and
engineers are excluded from the public roll-call.

### Step 7 — Update prose

Decide on headline features for the new period (lift the high-asset
PRs from Step 2 plus anything notable from the release notes). For
each, write a `<h2>` + a `.feature` block in `beta-<period>.html`:

- Alternate `<div class="feature">` and `<div class="feature flip">`
  to alternate left/right placement on desktop. The 720 px breakpoint
  collapses to a single column on mobile.
- Videos are `<video autoplay muted loop playsinline preload="metadata">`
  with both `.webm` and `.mp4` `<source>` children and a `poster=`
  pointing at the JPG. Always include an `aria-label`.
- Still images use `<img class="feature-media" loading="lazy">` with
  meaningful `alt` text describing the screen, not the position.
- The release-credit footer line uses `<p class="release">…</p>`.

Bug section: pull from `git log` + release notes. Attribute by first
name where the reporter is in the consent list; otherwise just cite
the release/PR number.

### Step 8 — Verify and ship

```bash
# Sanity build (the website embed list lives in website/embed.go;
# unchanged unless the static page was renamed).
go build ./website/

# Word-count check.
awk '/<body>/,/<\/body>/' website/content/beta-<period>.html \
  | sed -E 's/<[^>]+>//g; s/&[a-z]+;/ /g' | tr -s ' \t\n' ' ' | wc -w

# Open the page in a browser. There is no website-level CI lint or
# automatic preview; manual verification is the substitute.
open website/content/beta-<period>.html
```

Add a single inline link from `website/content/index.html` (footer)
to the new page. v1 also keeps the prior link as a "previous
retrospective" entry.

### Files to look at before starting

- `docs/issues/1583-research.md` — running notes from v1: schema
  gotchas, the layered photo-count derivation, the v1 tester roster,
  the v1 PR-media inventory.
- `scripts/fetch_pr_media.sh` — the cookie-based downloader.
- `scripts/convert_pr_media.sh` — the ffmpeg/sips conversion pipeline.
- `website/content/beta-1-month.html` — the v1 page; copy as a
  starting template.
- `website/content/css/shared.css` — palette and base styles. The
  page-local CSS lives inline in the `<style>` block at the top of
  the HTML.

### Estimated session cost

For the v1 page, end-to-end:

- Numbers refresh: ~5 min (one SQL pass + cross-check).
- Inventory: ~3 min (200 PRs at xargs -P 12 ≈ 30 s + sort).
- Fetch: ~5 min (47 assets at 1–2 s each + cookie setup if needed).
- Convert: ~10 min (libvpx-vp9 at CRF 35 is the slow step; 11 videos
  × ~30–60 s each).
- Curate + write copy: ~30–60 min (the human-judgment piece).
- Total: a focused half-day for a fully refreshed page.
