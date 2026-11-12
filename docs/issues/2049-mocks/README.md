# Issue #2049 — SSR event landing mocks

Static HTML mock variants for the event-flavored `/go/{code}` SSR
landing page. Built during Phase 0 of `docs/issues/2049-web-event-landing.md`
to lock visual direction before any Go template work.

## How to review

Open each `.html` file directly in a browser — no server needed.

```
open mock-a-editorial.html
open mock-b-cardstack.html
...
```

Toggle your OS appearance (System Settings → Appearance) to verify the
dark-mode variant of each mock. All mocks consume the shared
`../../../website/content/css/tokens.css` color and type tokens
mirrored from `app/lib/core/theme/app_colors.dart` and `app_theme.dart`.

Each mock shows three states stacked vertically:

1. **Happy path** — normal RSVP-able event with 6 attendees.
2. **At-capacity variant** — community at 32/32. **All three CTA
   buttons are hidden entirely** and replaced with: "Can't RSVP
   because the hosting community is full (32 of 32 members). Ask
   your inviter for options." No greyed-out controls.
3. **Empty-attendee variant** — what the page looks like before
   anyone RSVPs.

In the happy path, CTAs are visually disabled (`aria-label="Coming
soon — RSVP available shortly"`) because RSVP wiring is sub-issue
#2050's scope.

## Sample imagery

Hero photo and attendee portraits are pulled from
`server/simulation/assets/` (the simulator's stock asset set —
public-domain photography). The mock event uses `exp_004.jpg`
for the hero (a community potluck scene) and
`profile_001/005/010/012/018/025/030.jpg` for the host and
attendees.

## The variants

| File | Direction | Voice |
|------|-----------|-------|
| `mock-a-editorial.html` | Editorial — full-bleed hero, sticky footer CTAs | National-Geographic feature article |
| `mock-b-cardstack.html` | Floating card — centered card on cream | Apple invitation |
| `mock-c-compact.html` | Compact list — dense one-column scan | Mailchimp summary |
| `mock-d-postcard.html` | Postcard — handmade invitation with stamp | Field Notes, vintage |
| `mock-e-minimal.html` | Modern minimal — text-led, sans-serif | Linear / Notion |
| `mock-f-conversational.html` | Handwritten note — Emma's voice, serif prose | A personal letter |
| `mock-g-botanical.html` | Botanical bulletin — community board, color-distinct CTAs | Public garden notice |
| `mock-h-hybrid.html` | **Hybrid (A + C)** — full-bleed hero gradient-fading into a compact body, sticky footer with "I'm in / Maybe / I'm out" | Synthesis after first review pass |

## Sample data

All mocks use the same data so visual differences are apples-to-apples:

- **Event:** Community Potluck Dinner
- **Host:** Emma R.
- **Community:** Two Oaks Block (18 of 32 members)
- **When:** Saturday, May 24 · 6:00 PM
- **Where:** Two Oaks Community Center
- **Attendees:** Marcus, Henry, Ian, Betty, Tom, Sarah (6 RSVPed)

## What this is not

**Reference-only artifacts.** Once Phase 1 ships the SSR template,
these mocks aren't kept in sync with the production `event.html`.
They're a record of the design conversation, not living docs. Future
visual changes belong in `server/services/web/templates/event.html`
directly.

## Related

- Plan: [`../2049-web-event-landing.md`](../2049-web-event-landing.md)
- Parent: [`../1195-web-guest-rsvp.md`](../1195-web-guest-rsvp.md)
- Tokens source of truth: `app/lib/core/theme/app_colors.dart`,
  `app/lib/core/theme/app_theme.dart`
- Tokens CSS mirror: `website/content/css/tokens.css`
