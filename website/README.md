# Website

The Ripls marketing site and static web surfaces (landing page, community
invite / event deep-link pages, legal docs, release notes). Plain static HTML +
CSS + vanilla JS — no build step, no framework. Served by nginx; a handful of
stylesheets and assets are also embedded into the Go server for the pages it
renders dynamically.

## Layout

```
website/
├── config/default.conf      # nginx server config (deployed alongside content/)
├── config/locations.include # shared routing rules (prod + every staging preview)
├── smoke_test.cjs           # Playwright smoke test for the landing page (npm run test:website)
├── templates/
│   └── release-notes.html   # Pandoc template for release-notes pages
└── content/                 # the document root nginx serves (rsynced as-is)
    ├── home/                # the Ripls Foundation home page, served at /
    │   ├── index.html       #   (see "Home page")
    │   └── content.md       # HUMAN-WRITTEN copy source — never edited by an agent
    ├── homescreen/          # an audience landing variant, served at /homescreen/
    │   ├── index.html
    │   ├── copy.md          # editable copy source (see "Editing copy")
    │   ├── background/      # full-bleed section photos (01–05.jpg)
    │   └── screens/         # phone-frame mockups (video + gif)
    ├── crew/                # the "crew" landing variant, served at /crew/
    │   ├── index.html
    │   ├── copy.md
    │   ├── background/
    │   └── screens/
    ├── moms/                # the "moms" landing variant, served at /moms/
    │   ├── index.html       #   (same model + an extra TL;DR section)
    │   ├── copy.md
    │   ├── background/
    │   └── screens/
    ├── css/ -> ../../server/services/web/static/css   # symlink (#2953); see "Styling"
    ├── js/                  # waitlist.js — shared signup handler for all 3 pages
    ├── assets/              # shared images (logo, etc.)
    ├── legal/               # privacy-policy.html
    ├── sms-terms.html       # SMS terms (A2P 10DLC), served at /sms-terms.html
    ├── sms-opt-in.html      # SMS opt-in / consent proof (linked in the 10DLC campaign registration)
    ├── release-notes/       # per-release notes (Pandoc-rendered)
    ├── invite.html          # community invite card, served at /go/<code>
    ├── reset-password.html  # password-reset landing
    ├── beta-1-month.html    # private-beta retrospective
    └── favicon*, site.webmanifest, .well-known/  # icons + deep-link association files
```

## Home page

`content/home/index.html` is the public home page, served at the site root `/`.
It introduces the **organization** — mission, principles, story — and is aimed
at collaborators first, users second (#2895). It is deliberately *not* a
conversion funnel: no waitlist form, no phone mockups, no scroll animation.

It lives in its own folder but is served at `/` by a rule in the shared routing
snippet (`config/locations.include`):

```nginx
location = / { try_files /home/index.html /index.html =404; }
location = /home/ { return 301 /; }   # canonical: folder URL -> /
```

The `/index.html` fallback keeps the rule safe on the shared staging server,
where each branch preview has its own root — a branch without a `/home/` page
still serves its own `/index.html`.

Because it is served at `/` but its files live under `/home/`, the page uses
**root-absolute** asset paths (`/css/home.css`) rather than a `<base>` tag — a
`<base>` would break in-page fragment links.

Unlike the marketing pages, it builds on `css/gen/tokens.gen.css` (the generated
Ink & Sage tokens) plus `css/home.css`. It is **light only** — it consumes the
fixed `--light-*` tokens, which never follow the OS preference, the same way
`/walkthroughs/` does. Do not add a `prefers-color-scheme: dark` block; the
smoke test asserts there isn't one. Sage tints are derived with `color-mix()`
off `--light-primary` rather than hand-picked hex, so the page stays inside the
`lint:web:colors` gate. See "Styling".

### The copy is human-written — never edit it

**`content/home/content.md` is 100% human-written. No LLM or agent may edit
it**, and the same goes for every word rendered on the home page. The only
permitted direction of travel is one-way, verbatim injection:

```
content.md  ──(verbatim)──▶  index.html
```

An agent may change markup, classes, CSS, and layout freely. It may **not**
paraphrase, tighten, expand, reorder, correct (including typos), or invent any
user-visible string — headings, link labels, `<title>`, `<meta description>`.
If a section of `content.md` is empty or a placeholder, the page renders it as
a visible stub; the agent does not draft the missing prose. Raise gaps with a
human instead.

This is enforced, not just documented: `smoke_test.cjs` parses `content.md` and
fails if any of its strings is missing from the rendered page, so a paraphrase
or a drifted heading breaks CI. Keep the enforcement there — the page's own
HTML stays clean of build-process commentary.

The one escape hatch is `HOME_STRUCTURAL_HEADINGS` in the test: headings that
name a slot rather than supply rendered copy (today just `## Tagline`, whose
text is the page's `<h1>`). Keep that list minimal.

## Audience landing pages

`homescreen/` (served at `/homescreen/`), `crew/`, `moms/`, `builders/`,
`green/`, `kids/`, `simple/`, and `social/` are audience-specific landing
variants that pitch the app. They share the same look and signup flow but keep
their own copy and a few page-specific components. `homescreen/` used to be the
site root; it now self-canonicals to `/homescreen/` and is kept out of
`sitemap.xml`.

Their signup form POSTs to the waitlist RPC (`WaitlistService/JoinWaitlist`);
the email field maps to `email` and the "what are you keeping alive?" textarea
to `intendedUse`. It shows loading / success / already-registered / error states
inline.

## Editing copy

Each marketing page keeps its prose in an editable `copy.md` next to its
`index.html` (`homescreen/copy.md`, `crew/copy.md`, `moms/copy.md`). Edit the text under each
`## key` heading, then ask Claude to inject the changes back into `index.html`
— there is no automated generator; injection is a deliberate manual step.
Inline tags (`<em>`, `<br>`, `<strong>`, `<span class="accent">`) are preserved
verbatim. Photo credits live next to each image in the HTML, not in `copy.md`.

These `copy.md` files may be edited by an agent. **`home/content.md` may not** —
it is a stricter class of file (see "Home page" above).

Per repo convention, user-facing copy says **"Event"**, never "Experience".

## The marketing-page model (read this before adding/changing one)

The audience pages (`homescreen/`, `crew/`, `moms/`, `builders/`, `green/`,
`kids/`, `simple/`, `social/`) are instances of **one model**. Keeping them
uniform is the whole point — they share CSS and JS, and differ only in copy,
imagery, and a few page-specific components. Each folder is self-contained:

`home/` is **not** an instance of this model — different audience, different
stylesheet, no waitlist form. Don't copy an audience page as its template.

```
<page>/
├── index.html   # full-bleed photo sections; links /css/marketing.css + /js/waitlist.js
├── copy.md      # editable copy source (manual inject — see "Editing copy")
├── background/  # section photos, loaded by data-bg index
└── screens/     # phone-frame mockups (video + gif)
```

### To add a new audience page

1. `mkdir content/<name>/{background,screens}`.
2. Copy `crew/index.html` as the starting point (it's the **subpath** model — see
   the path gotcha below) and rewrite the copy + `data-bg`/`screens` references.
   Keep the shared `<link rel="stylesheet" href="/css/marketing.css">` and
   `<script src="/js/waitlist.js"></script>`; keep only genuinely page-specific
   rules in the inline `<style>`.
3. Write `content/<name>/copy.md` mirroring the HTML (same `## key` convention).
4. Import images **optimized**: backgrounds ≤3200px / ~1 MB (source pitch images
   are often 4–10 MB — downscale, e.g. `sips -Z 3200 -s formatOptions 65 in.jpg
   --out background/NN.jpg`).
5. Add a render + waitlist block to `website/smoke_test.cjs` and run
   `npm run test:website`.
6. No nginx change is needed for a subpath page (`index index.html` serves it).
   **But** if it gets a new public hostname, add that origin to the server CORS
   allowlist (see gotchas).

### Gotchas (the non-obvious traps)

- **Asset paths depend on where the page is served.** Only `home/` is served at
  the site **root** `/`, so it is the one page that must use **root-absolute**
  paths (`/css/...`) and carry **no `<base>` tag** (a `<base>` rewrites in-page
  fragment links). Every audience page — `homescreen/` included, since it stopped
  being the root — is served at its own subpath and uses **relative** paths
  (`background/`, `screens/`). `homescreen/` still uses root-absolute
  `/homescreen/...` paths for its `data-bg` loader, which works either way. Copy
  the right page as your template.
- **`copy.md` ↔ `index.html` is a manual sync.** There is no generator; "inject"
  means a human/agent hand-applies `copy.md` edits to the HTML. They can silently
  drift — after editing one, reconcile the other. `copy.md` is the source of truth.
  (The home page is the exception: its `content.md` → HTML sync **is** enforced,
  by `smoke_test.cjs`. See "Home page".)
- **The signup form reads fields by `name`, not `form.<field>`.** `/js/waitlist.js`
  uses `form.querySelector('[name="..."]')` on purpose: `form.name` returns the
  form's own name attribute (an HTMLFormElement quirk), so `form.name` would NOT
  give you the name input. Fields: `email` (required) → `email`, optional `name`
  → `name`, optional `keeping_alive` textarea → `intendedUse`.
- **`#formSuccess` holds the confirmation copy.** `waitlist.js` captures its text
  as the success fallback, then clears it so the `aria-live` region announces on
  submit. Edit the confirmation in `copy.md` (`signup.form-success`), not the JS.
- **CORS: the form POSTs cross-origin** to `server.ripls.org/...JoinWaitlist`. The
  page's origin must be in the server's CORS allowlist
  (`terraform/environments/prod/main.tf` → `cors_allowed_origins`, matched by
  `corsOriginValidator`). A new public marketing hostname won't work until it's
  added there and a server release ships. `*.staging.ripls.org` previews are
  covered by a subdomain wildcard.
- **Deploys don't reload nginx.** Files rsync into place; the mimir-side watcher
  (`infra/web-nginx-reload/`) reloads within ~15s, gated on `nginx -t`. Routing
  rules are authored once in `config/locations.include`; prod includes it
  directly and each staging preview includes its own deployed copy.
- **`marketing.css` is a separate design surface from `tokens.css`.** Don't merge
  them; the marketing look (dark photo overlays + Libre Baskerville) is
  deliberately distinct from the app-aligned palette.
- **Brand mark is the shared SVG dot logo.** Imported pitches sometimes use a
  text-`ripls`+`<span class="dot">`; replace it with the SVG `.brand` markup so it
  matches `marketing.css`.
- **Missing assets.** If a section references an asset that doesn't exist (e.g. the
  moms pitch's `re-engage.gif`), omit the section rather than ship a broken image,
  and note it at the top of that page's `copy.md`.

## Styling

| Stylesheet | Used by | Notes |
|------------|---------|-------|
| `css/home.css` | `home/` | The Ripls Foundation home page. Built on `css/gen/tokens.gen.css`, **not** `marketing.css`. Light only (fixed `--light-*` tokens). A centered broadside: big serif section titles in deep sage, sage-tinted full-bleed bands via `color-mix()`, serif for text meant to be read and sans for chrome. No boxes, no card borders, no shadows, no motion, no photography, and no small-caps "eyebrow" labels above headings. |
| `css/marketing.css` | `homescreen/`, `crew/`, `moms/`, and the other audience pages | Shared foundation for the full-bleed marketing pages: dark photo-overlay sections, Libre Baskerville/Public Sans type scale, brand wordmark, phone frames, signup-form visuals. Each page adds a slim inline `<style>` for its own components. |
| `css/tokens.css` | the Go-rendered invite/event pages | Derived aliases, type/spacing/radius scales, and shadows layered over the generated tokens. `css/gen/tokens.gen.css` is generated — edit `design/tokens.json` and run `npm run generate:design-tokens`. |
| `css/deeplink.css`, `css/event.css` | invite / event landing pages | Build on `tokens.css`. |
| `css/shared.css` | older static pages | Legacy shared styles. |
| `css/privacy-policy.css` | legal pages | |

The home page's principle icons are **Material Icons glyphs inlined as SVG
paths** — no CDN request and no extra font to host. They were extracted from the
`MaterialIcons-Regular.otf` that Flutter already bundles
(`server/services/web/app_assets/assets/fonts/`, Apache 2.0), scaled onto a
24×24 grid and flipped (font glyphs have y pointing up). Note that font is
Flutter's **tree-shaken subset**: it only contains glyphs the Ripls app itself
uses, so many icons (`code`, `public`, `hub`, …) simply aren't in it. Codepoints
come from the Flutter SDK's `packages/flutter/lib/src/material/icons.dart`.

`marketing.css` is intentionally a **separate design surface** from
`tokens.css`: the audience pages use a distinct brand look (dark overlays +
Libre Baskerville display serif + cream/sage), while `tokens.css` mirrors the
in-app palette. Don't merge the two without a design decision.

The home page picks the generated-token side of that split on purpose: it is the
org's front door, so it should look like the product's design system, and it
stays inside the generated-token contrast gate (`npm run lint:design-tokens`,
#2441) instead of becoming a third ungated surface. It deliberately does *not*
take the OS dark mode that comes with the auto-switching tokens — it is a
light-only page and pins the fixed `--light-*` set.

## Serving: nginx vs. the Go server

Most pages here are **static**, served straight from `content/` by nginx.
`config/default.conf` is a thin server block; all the routing rules (the home
page at `/`, the deep-link routes `/go/<code>` → `invite.html`, `/reset-password`, and
the `.well-known/` Apple/Android association files) live in
`config/locations.include`, which the production server block `include`s.
Keeping the rules in one file is why prod and staging can't drift.

On staging every preview `include`s its **own copy** of that file. A PR deploy
writes `config/locations-<slug>.include` next to a generated
`config/site-<slug>.conf` server block, so a PR that changes routing is exercised
on its own preview and nowhere else — see
[`infra/staging-web/README.md`](../infra/staging-web/README.md). Before #2895 the
snippet went to one global path shared by every preview, and the last website
deploy to run silently set routing for all of them.

**Deploys don't reload nginx.** The deploy SSH endpoint is a restricted rsync
container with no docker socket, so it can only write files — a config or content
change doesn't take effect until nginx is reloaded. A host-side watcher on mimir
(`infra/web-nginx-reload/`) closes that gap: it reloads each container within
~15s of a change, gated on `nginx -t` so a broken config is never applied. (This
gap, before the watcher existed, took the site down once when a deploy removed
the root `index.html` without a reload.)

Stylesheets and assets are **embedded into the Go server** so it can render the
invite/event pages with consistent styling. Since #2953 they live in
`server/services/web/static/`, which `content/css`, `content/fonts`, and
`content/.well-known` symlink into, and `server/services/web/static.go` embeds
the whole `static/css` directory — so a new stylesheet dropped there is embedded
whether or not the server renders anything that uses it. `home.css` and
`marketing.css` are in that bucket: nginx-static in practice, embedded as a side
effect of sharing the directory. Adding one needs no code change either way.

## Canonical host (SEO)

The single canonical host is **`https://ripls.org`** (non-www) — it matches the
app domain `ripls.app` and the non-www URLs the server already emits (email logos
`https://ripls.org/assets/logo.png`, the `RiplsBot` UA, the invite-page privacy
link). The site is also reachable as `www.ripls.org`, `type2fun.org`, and
`www.type2fun.org`: Nginx-Proxy-Manager (on mimir) terminates TLS and proxies all
four hostnames to the same container with the original `Host` preserved.

Two layers keep that from becoming duplicate-content:

1. **301 redirect** — `config/default.conf` (prod only) sends the three alias
   hosts to `https://ripls.org$request_uri`. This is the strong signal; it lives
   in `default.conf` and **not** in `locations.include` because every staging
   preview (`*.staging.ripls.org`) includes a copy of that file and must keep
   serving its own branch host. Without it, all four hosts answered `200` for
   identical content and
   Search Console reported the homepage as *"Duplicate without user-selected
   canonical."*
2. **`<link rel="canonical">`** — each page self-references its
   `https://ripls.org/...` URL: `home/` canonicals to `/` (it is served there),
   and every audience page to its own subpath (`/homescreen/`, `/crew/`,
   `/moms/`). Keep new pages non-www and self-canonical. When a page stops being
   the root, its canonical has to move with it — that is why `homescreen/`
   canonicals to `/homescreen/` now.

`content/robots.txt` and `content/sitemap.xml` declare the canonical host and the
indexable page set; add a `<url>` to the sitemap when you add a public page.
(Staging stays out of the index via its `X-Robots-Tag: noindex` header, set in
`infra/staging-web/config/default.conf`.)

## Testing

```bash
npm run test:website     # Playwright smoke test (website/smoke_test.cjs)
```

It serves `content/` with the same routing as `config/locations.include` and
checks that the home page renders at `/` with **every string from
`home/content.md` present verbatim** (the copy invariant, #2895), that the
audience pages render with their backgrounds and a labelled form, and that the
waitlist submit handles success / already-registered / server-error. It serves
the static files itself and mocks the waitlist endpoint, so it needs no backend.

## Deploying

Pushing to `main` under `website/**` triggers `.github/workflows/publish_website.yaml`,
which rsyncs `website/config` and `website/content` to the web host (see
`.github/actions/deploy-website`). A staging pipeline
(`deploy_staging_website.yaml`, `infra/staging-web/`) serves branch previews for
visual review before changes reach production `/`.
