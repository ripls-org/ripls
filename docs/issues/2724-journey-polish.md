# #2724 — Journey polish from the walkthrough reel review

**Goal:** make the five critical user journeys — as filmed by the walkthrough
reels — excellent, intuitive, and consistent across screens. This plan is the
deduplicated output of a frame-by-frame review of all five reels
(`onboarding`, `events`/brunch, `gear`/kitchen, `requests`/mower,
`classroom`), watched from a first-time user's point of view, plus a
cross-reel pass over the surfaces every journey shares (impact/wrap-up,
share sheets, phone-OTP join, unified create, chat).

**Method.** Seven independent reviewers each watched one reel segment as
stills (1 frame/2 s, with 3–4 fps fine slices around suspect transitions;
~280 stills total), quoting exact on-screen text and citing frames. The
orchestrating pass compared the same surface across reels. Findings were
deduplicated into the workstreams below. Severity: **P0** = looks broken /
factually wrong on screen, **P1** = confusing or unintuitive, **P2** =
polish. Evidence citations are `reel/scene` (clips live under
`e2e/videos-walkthroughs/<reel>/`, stills in the session scratchpad).

**Verification loop (applies to every workstream).** The reels are the
review harness: after each fix batch, `npm run build:e2e` +
`e2e/scripts/run_walkthrough.sh <reel>` for affected reels, then read
`frames/`. State-correctness fixes (workstreams A, B) also get a
correctness twin in `e2e/tests/workflows/` — reels are showcase, not gate
(`docs/walkthroughs.md`). Copy edits touch `app/lib/l10n/app_en.arb` **and**
`app_es.arb`.

---

## P0 — visibly broken on camera (fix first)

Each of these appears in a rendered reel today and reads as a bug to a
viewer. Every one gets its own small PR (or two grouped) with a workflow
twin where server state is involved.

- **P0.1 Impact preview ≠ final at commit.** The Mark Fulfilled /
  Mark Completed modal computes impact **excluding the transfers that
  complete on commit**, so the preview contradicts the post-commit screen:
  classroom modal `$0 Saved / 49 mins / 5.0kg CO₂` → fulfilled request
  `$80 / 36 mins / 19kg` (all three numbers change, time goes *down*);
  events completion `237 mins` re-renders to `211 mins` under the user's
  finger mid-tap; events preview shows `$0 Saved / 0.0kg CO₂` although a
  folding-table loan is attached to the very event on camera
  (classroom/s04, events/s07). This is the #2702 `$0`-class bug resurfacing
  in the *preview* path. Fix in `server/services/experience/preview.go` /
  `stats.go` (and the request-fulfill equivalent under
  `server/services/request/`): preview must use the same inputs as the
  final computation, including confirmed gear offers / child transfers that
  will complete on commit; freeze the previewed figures through submit.
  Acceptance: modal numbers == post-commit numbers in a workflow test; no
  visible number change during commit in re-rendered classroom + events
  reels.
- **P0.2 Guest RSVPs, then reads "You haven't replied yet."** After
  `Finish RSVP` (phone-OTP registration path) the event page renders the
  viewer row as "You — You haven't replied yet" with a dashed avatar and a
  bottom "RSVP — are you in? ›" CTA, while the header count already
  includes her ("4 in · 6 invited") — the RSVP recorded during registration
  never hydrates the "You" row (events/s02 closing beat). Fix: re-fetch or
  patch the viewer RSVP row after auth completes. Acceptance: workflow twin
  asserts the rendered "You" row is *Going* immediately after registration;
  events reel scene 2 ends on "You — Going ✓".
- **P0.3 "N interested" off-by-one after recipient selection.** Spaghetti:
  "3 interested" over two rows (Dana SELECTED, Marcus INTERESTED); bread:
  "2 interested" over one row (Priya SELECTED). Pre-selection counts are
  correct — selection double-counts the recipient (gear/s06). Fix the count
  source (distinct requesters); assert count == rows in the gear workflow
  twin.
- **P0.4 Blank sparkle row on the create/preview sheet.** Between the
  Lend|Give toggle and the location row sits a row that is only `✦` + a
  chevron — no text — on every gear create (gear/s01–02, events/s05 link-gear
  create, requests/s02 item create). It occupies the slot where the item
  value/details belong. Fix: label it (e.g. "Estimated value · $12") or hide
  the row when empty.
- **P0.5 Empty "FULFILLMENT HISTORY".** The impact receipt — the closing
  frame of both request reels — ends with the header over blank space on a
  request fulfilled seconds earlier (requests/s05, classroom/s05). Fix:
  populate (fulfillment event + confirmed helpers + date) or suppress the
  header when empty. `app/lib/presentation/screens/item/item_metrics_screen.dart`
  area.
- **P0.6 Home "NEEDS YOU" row targets the superseded claimant.** After Dana
  is picked, Home still shows "Spaghetti (2 boxes) / **Marcus** / [Give it]"
  — the one person *not* getting it (gear/s06). Fix: action rows follow the
  selected transfer ("Hand off to Dana"); a superseded claim never drives
  the CTA.
- **P0.7 Request fulfillment is invisible on the request page.** After
  Mark Fulfilled, the only change is a new unlabeled chip row; the needs row
  still reads "ACCEPTED ✓ Theo", the card "1 of 1 claimed", no
  badge/toast/system line (requests/s04 — the "closes itself" scene shows
  no closing). Fix: fulfilled state morph — status chip on the hero (as
  events gets "WRAPPED"), needs rows → "HANDED OFF ✓ / GIVEN ✓", toast.
- **P0.8 Glyph/emoji fallout.** Emoji in *loaded chat history* render as
  tofu `□` while live-arriving messages render color emoji (two text paths;
  events/s06); "CO▯" subscript + tofu search icon during modal pop-in
  (classroom/s04); weather icon first paint "▯ 69°" (onboarding). Fix: one
  emoji/icon fallback path for history + live; preload the icon font.

---

## Workstream A — impact & wrap-up coherence

The wrap-up is the emotional payoff of every journey and currently the
least trustworthy screen. Beyond P0.1/P0.5:

- **A1. Label the impact chips.** Request detail shows bare `$210 · 37 mins
  · 40kg` (icon-only chips) while the events completion sheet labels its
  tiles — a first-time user can't tell $-of-what/kg-of-what
  (requests/s04–05, classroom/s04–05, my xreel pass). One shared component:
  value + short label ("$210 saved · 40 kg CO₂ · 2 h back").
- **A2. One time metric, or explain both.** `8h RECOVERED` vs `37 mins
  QUALITY TIME` side-by-side with no definitions (requests/s05,
  classroom/s05); the request chips show only one of them. Merge or caption
  (nod to #2019's tier-ladder work; don't build parallel metaphors here).
- **A3. Humanize durations.** `237 mins` should read `~4 h` (the narrative
  literally says "four hours"; events/s07).
- **A4. Suppress zero tiles.** Never headline `$0 Saved` / `0.0kg CO₂`;
  hide zero tiles or replace with the non-zero story (events/s07 preview —
  largely falls out of P0.1, but keep the guard for genuinely-zero events).
- **A5. De-duplicate the receipt.** Impact view shows the same four numbers
  twice (hero strip + tile grid), with two tiles arbitrarily ~60 % dimmer —
  the dim pair reads as disabled (requests/s05, classroom/s05). One
  presentation; consistent emphasis and tile order.
- **A6. Requester-framed receipt copy.** June (the *requester*) reads "Each
  time a friend borrows **this** instead of buying one, your circle saves:" —
  owner-framed copy on her request's receipt (requests/s05); same class as
  the #2703 framing fix. Frame by viewer role.
- **A7. Surface "View Impact".** Post-fulfillment it hides inside the
  edit/manage menu (classroom/s05, requests/s05); put an inline entry point
  on the fulfilled request/event page.

## Workstream B — live-state truth & count reconciliation

The product's differentiator on camera is that things update live; every
place that doesn't (or disagrees with itself) reads as broken. Beyond
P0.2/P0.3/P0.6:

- **B1. Needs strip + claim pills must live-update in the open "Who's In"
  pane.** RSVP rows update live, but Eric's dessert claim and Marc's
  "Mimosas!" never appear in Leslie's open pane — the scene-5 payoff is a
  static list (events/s05; scene 7 proves both landed). Subscribe needs +
  claim pills to the same stream RSVPs use.
- **B2. Reconcile "in / invited" math.** "Invited 2 people" toast, then
  "1 in · 3 invited" (host counted as invited); "4 in · 6 invited" only
  reconciles if "invited" includes everyone going (events/s01–02). Partition
  as "N going · M invited (no reply)". Host excluded.
- **B3. One collapsed-row count.** Same 5-need list shows "1 more ›" to
  Maya and "2 more ›" to Rob with identical "4 of 5 claimed" (classroom
  s02/s03). Collapsed count = total − visible, identical for all viewers.
- **B4. Reconcile share counts.** "SHARED WITH 4 PEOPLE" card vs
  "SHARED · NO REPLY · 2" panel with no explanation (requests/s02–03); the
  card is also mostly empty space with an uninspectable number (all reels).
  One count basis + a facepile/community chips so the number is legible.
- **B5. Post-handoff/post-completion panes must reflect the terminal
  state.** Gear: recipient row stays "SELECTED" after completion (badge
  should flip to "RECEIVED IT" / "GIVEN ✓") and the recipient's completed
  view still headlines "Giving this away for free" above "GIVEAWAY
  COMPLETED" (gear/s06–07). Requests: post-fulfill, GIVING tags vanish while
  "LENDING Bea" persists — add a "GIVEN ✓ / DELIVERED" tag so rows stay
  parallel (classroom/s04–05).
- **B6. Freeze affordances on wrapped events.** Completed event roster
  still offers "We need / I'll bring / Invite", an actionable open-need
  chip, and editable "Going ⌄" dropdowns (events/s07). Read-only after
  wrap (with an explicit "reopen" path if needed).

## Workstream C — boot, hydration & transition quality

Every reel films at least one loading seam; several film three. Real users
hit the same seams.

- **C1. Dark boot, one loader.** The web bootstrap first-paints a
  full-screen **white flash** (brutal mid-journey in dark mode), then the
  splash, then sometimes a skeleton — three stacked treatments
  (onboarding ~3–6 s, gear/s02+s06 mid-scene, events/s01). Style the
  bootstrap page with the app's dark surface; collapse to one branded
  loading state.
- **C2. Kill developer captions.** "Checking authentication..."
  (`splash_view_model.dart`; filmed in 4 reels) → "One moment…" or nothing;
  "Loading communities..." lies when opening an item → neutral "Loading…".
- **C3. Skeletons, not placeholder facts.** During hydration the item view
  shows `WHERE / TBD`, an empty flat-green avatar circle, a black hero
  (gear/s01+s06), and — worse — **false empty states**: "No one's raised
  their hand yet." on a claimed item, "Be the first to message" under
  existing history (gear/s06, gear/s04). Empty-state copy must never render
  while the query is in flight.
- **C4. Image placeholders.** Incoming chat photos paint as large blank
  white cards ~2 s (brightest object on a dark screen, events/s06);
  blurhash/dominant-color + fade-in. Avatars flash as initials/blank disks
  before photos across panes (events/s07 roster goes *all* initials ~2 s
  post-completion; completion modal disks; claim sheet) — reuse cached
  avatar bytes across panes.
- **C5. Don't clear content under progress.** "Crafting your story…"
  renders as a near-black screen with a stray gray square (events/s07);
  dim the sheet content instead. "Link copied" snackbar renders *behind*
  the open share sheet — invisible feedback, only an orange glow bleeds
  through (classroom/s01) — anchor confirmation above sheets or flip the
  button label to "Copied ✓".
- **C6. Create-sheet detection state.** While the classifier streams, the
  sheet shows the **Request tab preselected**, an enabled-looking Save, and
  "Add a description…" before flipping to Item ~1 s later (gear/s02,
  events/s05). No preselected tab (or dimmed form) + disabled Save until
  detection resolves.
- **C7. Web-platform copy leak.** "Live camera is mobile-only. You can
  still create from text or URL on the other tabs." inside the phone-frame
  flagship beat (gear/s01). Form-factor-aware copy; keep "Pick a photo to
  get started".

## Workstream D — one vocabulary

Terminology sprawl is the single biggest *consistency* finding. One sweep
through `app_en.arb`/`app_es.arb` + the server-rendered system lines, with
a small glossary in `docs/client/conventions.md` so it sticks:

- **D1. Entity nouns.** "Experience" never reaches users ("Experience
  marked complete" toast → "Event wrapped up"); pick **event**. Pick
  **item** over "gear" in user copy ("Link gear / Add new gear / Search
  your gear" vs "Add a new item / No items in your library yet." on one
  sheet — requests/s02, events/s05). Pick **needs** over "pieces"
  ("2 PIECES" / "Share list · 2 pieces" vs "NEEDED · 4 OPEN" vs "added a
  need:" — events/s03). Reconcile "crew / communities / circles" (home CTA
  "Ask the crew", sheets "Invite communities", narrative "circles").
- **D2. One claim-state ladder per domain, one register.** Gear giveaway:
  "I'm interested" button / "WHO WANTS IT" header / "Raise your hand"
  helper / "submitted interest" system line / badges
  INTERESTED→SELECTED→RECEIVED IT / "REQUESTED" badge in chat (request-
  domain word in a giveaway) / "Signed up · Just now" row subtitle (reads
  as account registration). Requests: "I'll do it" → "Offered" →
  "Accepted ✓" → "ACCEPTED ✓ Theo", while the section header counts
  "claimed" and the give/lend distinction collapses to a bare "Offered" on
  the requester's own sheet (classroom/s02 — the scene *about* Giving/
  Lending tags). Choose the warm register (raise-a-hand family; keep
  Giving/Lending pills visible to the requester alongside state).
- **D3. One completion verb family.** Menu "Mark Completed" → progress
  "Crafting your story…" → toast "Experience marked complete" → chip
  "WRAPPED · 0m ago" → sheet eyebrow "COMPLETED" (events/s07); requests use
  "fulfilled". Pick per-domain verbs (events *wrap up*; requests *fulfill*)
  and use them in menu, modal, toast, chip.
- **D4. "transfer" never reaches users.** Chat: "This transfer has been
  completed" (gear/s07) → "Handed off ✓ / Given to Dana".
- **D5. Toast grammar.** One visual (green + check) and one voice —
  outcome-first, named: "Shared to communities" after sharing to *one*
  community → "Shared to Cedar Court Neighbors"; cold "Recipient selected"
  → "It's going to Dana"; "Account created" after `Finish RSVP` → lead with
  the outcome ("You're in — RSVP sent") with account creation as the
  secondary line (onboarding, requests, events).
- **D6. Composer + create copy.** "Type '@' to mention things" → "Message…
  (@ to mention people or items)"; "Generate" → "Create"; "URL" tab →
  "Link"; "Replace background" before any photo exists → "Add a photo";
  "Anyone with link can join" on items/requests → "Anyone with the link can
  see this item/request" (join is for events/communities); share sheet
  "Invite community" vs "Share to communities" (one verb); "Search members
  or people…" (`markCompletedAddPersonSearch`, `pastTransferSearchHint`) →
  "Search people…"; "CLAIM A NEED" eyebrow → softer ("Pitch in").

## Workstream E — formatting primitives

Small shared utilities; each currently wrong in 2+ reels:

- **E1. Phone formatting.** As-you-type mask; formatted echo on the OTP
  screen (raw `+15551234576` filmed in four reels next to a formatted
  placeholder).
- **E2. Relative time.** "last 0m" → "just now"; "64d 4h ago" → humanized
  ("2 months ago"); "0m ago" chips. One formatter.
- **E3. Dates.** "May 10 10:30AM" → "May 10, 10:30 AM"; align SSR
  ("Sun, May 10 · 10:30 AM") and in-app formats; fix the SSR info card's
  dangling "·" when the location wraps (onboarding, events).
- **E4. Pluralization.** Stepper "− 1 + items" → "1 item"; "See all 1 ›";
  "1 OF 5 STILL NEED SOMEONE" → "NEEDS SOMEONE".
- **E5. Truncation.** Word-boundary ellipsis ("Foldi…", "ro…"); "+1 more"
  over mid-word cuts; close the opening quote on truncated quote cards;
  don't truncate addresses that fit ("1608 Alta Vista …").

## Workstream F — the description / thread-preview card

The quote card at the top of every event/request/item is a conversation
carousel, and it's confusing in four of five reels:

- **F1. Never rotate superseded text back in.** The card cycles between
  the polished description and the raw create prompt (or the latest chat
  message), with visible crossfade ghosting and artifacts like
  "last 0m0m" (classroom all scenes, requests/s03–04, events/s07). Pin the
  description; if a latest-message preview is wanted, make it a separate,
  labeled line ("💬 latest: …").
- **F2. Metadata honesty.** Fresh entity shows "1 reply · last 0m" because
  the description counts as a reply (all reels). Count replies excluding
  the seed message.
- **F3. Don't let the generator duplicate the title into the description**
  ("Mother's Day Brunch. At our place…" under the heading "Mother's Day
  Brunch"; events/s01) — strip at source. Keep description text consistent
  across event view / chat seed / SSR.

## Workstream G — guest landing & phone-first auth

The app-less path is the pivot's crown jewel; it's 90 % right:

- **G1. Landing completeness.** Request landing shows no description and no
  supply list — Rob offers help blind, and a large empty band sits where
  the pitch belongs; requester avatar is a gray initial though a photo
  exists (requests/s02, classroom/s03). Render description, needs summary
  ("5 items · 1 still open"), real avatar.
- **G2. Outcome-first completion** (with P0.2): after Finish RSVP /
  Finish, confirm the thing the user did ("You're in — see you May 10!"),
  not just "Account created" (onboarding/s01, requests/s02).
- **G3. Pre-fill the invited name.** Guest invited by name retypes it
  (onboarding); provisional-user name should pre-fill, editable.
- **G4. Auth-option consistency.** "Continue with Google" (plain-text "G"
  → proper mark; button) vs "Sign up with Email" (bare row) vs "Already
  have an account?" — one pattern, consistent verbs (onboarding, events,
  classroom).
- **G5. Consent copy per entry point.** SMS consent leads with "event
  invites and community updates" on a *help-request* landing
  (requests/s02); parameterize the first noun by context. Keep the A2P
  compliance body.
- **G6. Roster warmth after join.** SSR landing shows real faces; the
  in-app WHO'S IN renders identical green check circles (onboarding) —
  avatar + small status badge instead. Fix "5 in · 8 invited / 5 rows /
  '1 more ›'" contradiction by labeling overflow by status ("+1 maybe").

## Workstream H — share & invite sheets

- **H1. Share over the thing being shared.** Post-save share sheet floats
  over a black "Search nearby" void (events/s01 — dismissing strands the
  user with no "Event created" confirmation), over a *different item's*
  full-bleed photo (gear/s02 ×2 — parses as sharing the wrong item), and
  over a feed row reading "June Park — Created **community** · now" for a
  request (requests/s01). Open it over the just-created entity's own view;
  toast the creation.
- **H2. One sheet at a time.** Who's In → share-sheet remnant (gray QR
  sliver) → "Invite communities" stack three layers (events/s01); dismiss
  the share sheet before pushing pickers.
- **H3. Don't offer the host to herself** in the Invite people picker
  (events/s01).
- **H4. Share nudge accuracy.** "Reach a bit further…" shows when there are
  no unshared circles left (gear/s05).
- **H5. Add Media sheet dark-mode pass** — white halo glow, light-theme
  component in a dark app (events/s01).

## Workstream I — chat surface

- **I1. Header scrim/inset.** Transcript scrolls flush under the floating
  ‹ / ✕ controls; first frame shows text jammed under both (events/s06,
  events/s04, gear chat). Add scrim + top inset; drop the double dismiss
  (dimmed chevron *and* ✕).
- **I2. The entity chip is not an avatar.** Bottom-left of the composer, an
  item/event photo captioned with a *person's* name ("Leslie" under a plate
  of food; "June Park" under the mower; "Maya" under bread) — reads as a
  wrong-avatar bug in every reel with chat. Caption with the entity name,
  or restyle as an obvious context chip.
- **I3. System-line economy.** One action → up to three lines ("offered to
  help" / "will lend their Honda…" / "is bringing: Lawn mower"), while the
  *accept* — the emotional beat — emits nothing (requests/s03, events/s06);
  "updated time →" no-op lines on freshly created events (events/s04);
  needs logged out of entry order; "Lisa will lend **their** …" for a named
  person. Collapse per action, emit accepts, suppress no-ops, order by
  creation, fix the pronoun (they/them default is fine for unknown people —
  the awkwardness is the *duplication*, not the pronoun).
- **I4. Badge economy.** "ATTENDING" chip on every message including the
  host's (events/s06) — badge the exceptions (HOST, or non-attendee), not
  the rule. "REQUESTED" badge in giveaway chat → D2's ladder.
- **I5. Emoji parity** (P0.8) and photo placeholders (C4) land here too;
  label the under-composer thumbnail rail ("Photos from this event") or
  relocate it above the composer (events/s06).

## Workstream J — completion ceremony semantics

- **J1. Preview ≠ achieved.** Modal eyebrow reads "COMPLETED"/"FULFILLED"
  while the CTA still says Complete/Mark Fulfilled (events/s07,
  classroom/s04). Preview eyebrow → "WRAP UP" / "READY TO FULFILL";
  reserve the past-tense for after.
- **J2. Confirm-list affordance.** "TAP TO CONFIRM" instructs a no-op on a
  fully pre-checked list, and tapping a tile silently *removes* someone
  (the #2703 gotcha). Copy → "Confirm who came / who helped — tap to
  remove"; consider an explicit per-row ✓/✕ toggle visual.
- **J3. Consistent completion feedback**: status chip on the hero (events
  has "WRAPPED · 0m ago"; requests/gear get parallel chips per P0.7/B5),
  one toast voice (D3/D5).

## Workstream K — reel plumbing & narrative sync (no app code)

The reels themselves have defects that make good UI look broken; fix so
future renders judge the app, not the harness:

- **K1. No mid-scene `page.goto` reboots** — navigate in-app; two filmed
  full reboots in gear/s02+s06, plus cold-open black frames (all reels) and
  static tails (gear/s07 ~55 % freeze, events/s04 ~8 s). Trim heads/tails
  in export; settle to network-idle before filming hydrating pages.
- **K2. Seed dates in the future / backdate listings.** "May 10 · 64d 4h
  **ago**" for an upcoming-brunch story; "Listed 1m ago" against a
  "two days out" narrative. Seed relative-to-now.
- **K3. Make the two no-op wheel beats actually scroll** (classroom/s02,
  s05 — pixel-identical frames; the fifth needs row never appears on
  camera).
- **K4. Fixture hygiene.** Theo's "snapped photo" is pixel-identical to
  June's request hero (same stock image — a careful viewer notices);
  Marc's scripted reply claims a guest is "hosting"; OTP beat flashes by in
  <1 s (slow the paced typing so the verification reads).
- **K5. Narrative ↔ UI sync.** kitchen `walkthrough.md` §06 promises "the
  app totals what didn't hit the trash" — no impact surface is filmed; §07
  quotes "Gone to a new home" — the UI says "GIVEAWAY COMPLETED /
  RECEIVED IT". `docs/walkthroughs.md` catalog says events scene 7 includes
  "invitee revisits impact" — the spec doesn't script it. Either film the
  promised beats (preferred once A-workstream lands) or fix copy + catalog.
- **K6. On-camera copy mutation.** The off-camera title/description tidy
  swaps "Setting up Room 7…" → "Back-to-school supplies for Room 7" while
  the share sheet still shows the old title (classroom/s01, requests/s01↔s03).
  Seed the polished copy *before* the share sheet opens, or let the
  generated copy stand.

---

## Sequencing

Delivered as one PR, committed phase by phase (the phases below are the
commit series, not separate PRs):

| Phase | Contents | Status |
|---|---|---|
| 1 | P0.1–P0.8, each with a twin test | done |
| 2 | A (impact coherence) + J (ceremony) | done |
| 3 | B (live-state + counts) | done |
| 4 | D + E + F (vocabulary, primitives, quote card) | done |
| 5 | C + H + I (loading, sheets, chat) | done |
| 6 | G (guest landing/auth) | done |
| 7 | K (reel plumbing) + re-render all five reels + walkthrough-copy/catalog sync + deploy pages | done |

Phase 7 is the acceptance test for everything above: all five reels
re-rendered, frames reviewed, and the walkthrough pages recommitted.

### What the acceptance pass caught

Re-rendering and re-reading every frame is what made this plan honest, and it
found things the code review could not:

- **The impact fix was still incomplete after phase 1 shipped.** Preview and
  commit shared one computation, but the commit alone could consult the LLM
  when an entity had no stored hints — so the number still moved at the
  instant of commitment. The parity tests missed it because they nil-ed the
  AI provider, the same blind spot the earlier consistency test had. Both
  paths now use stored hints only, and the tests run with a live provider.
- Four defects that only exist on screen: a details card echoing its own
  label, a CTA rename colliding with the FAB's accessible name, labeled pills
  truncating, and the share sheet opening over a heroless entity.

**#2727 is now closed too** — see the last entry below. The events scene ends
on "You — Going" with no RSVP prompt.

### Root causes worth remembering

Five findings turned out to be one bug each, not the class they looked
like from the frames:

- **Impact preview ≠ commit** (P0.1) was not a rounding or a $0 edge case:
  preview and commit ran *entirely separate computations*. The request
  preview was value-based and never queried the gear offers; the commit
  adopted the transfers but never rebuilt Quality Time for the group size.
  Both sides now route through one `requestFulfillmentImpact` /
  `adoptChildTransferImpact`, and the parity tests assert exact equality —
  no tolerance band — so any future drift fails loudly.
- **Needs claims not live-updating** (B1) was not a client subscription
  gap: the six `PLANNING_*` event types existed in the models enum but not
  the API enum, so `ModelEventTypeToAPI` degraded them to `UNSPECIFIED`
  and every client dropped them silently.
- **Emoji tofu in chat history** (P0.8) was not two render paths: skwasm
  downloads Noto fallbacks lazily on first *use* and already-painted
  paragraphs don't re-shape, so history painted before the font arrived
  kept its boxes forever. Warming the fallbacks at splash fixes it.
- **The description repeating the title** (F3) was not the mock echoing the
  prompt: all three text-create streams emit the prompt verbatim as the
  description *by design*, before the AI has returned a title, so the client
  can paint on the first frame. The title then arrives as that same prompt's
  opening clause. The fix re-emits a trimmed description once the title is
  known rather than delaying the first paint.
- **The guest's stale RSVP row** (#2727) was not the refetch gap it looked
  like — the client *did* refetch, and the notifier *did* hold the right
  reply. `loadExperienceDetails` publishes the roster, then fetches the owner
  profile and location, then publishes the viewer's reply in a second write
  that never re-checked `_loadGeneration` the way its first one does. The
  hand-off RSVP lands in that window, so the refresh published the reply and
  the superseded load reverted it ~60ms later, from a roster read before the
  RSVP existed. Instrumentation settled this after inspection had produced
  three wrong diagnoses in a row: the probe showed `refresh.write
  intention=YES gen=2/2` followed by `load.write2 intention=null gen=1/2`.
  The reply is now derived from the roster, so one fetch publishes both and
  no writer can revert it.

### Found while re-rendering, not in the frame review

The reels are the review harness, so re-rendering them surfaced defects the
still-by-still pass had missed — each is fixed here:

- The **item-details card echoed its own label** ("ITEM DETAILS / Item
  Details") for anything without a brand, which is every food item. It now
  summarizes brand · model / category / ~$value.
- **"Create" collided with the Create FAB.** Renaming the create-drawer CTA
  from "Generate" gave two different buttons the same accessible name — and
  was inaccurate besides: that button drafts, **Save** creates. It reads
  "Draft it".
- **Labeled impact pills truncated** to "37 min to…" because the row forced
  equal thirds.
- **The share sheet opened over a black view** in the request reels: the
  H1 fix puts it over the new entity, but a text-created request has no hero
  until production's stock-imagery branch supplies one, and the e2e stack has
  no such provider. The reels now photograph the subject on camera (as the
  events reel already did), which is also the better beat.

## Out of scope / adjacent tracked work

- Metric metaphor redesign (tier ladders): #2019 — A2 only labels what
  exists; don't build new metaphors here.
- Workshop v4.1 polish follow-ups: #2489 (separate surface).
- Post-completion gear-view 3-faced bug: #2695 (already tracked; B5
  touches the same screens — coordinate, don't duplicate).
- URL-mode create issues: #2037/#2039.

## Do not regress (what the review praised)

Contextual auth headings ("Confirm your phone to RSVP to …"); disabled→mint
CTA discipline with in-button spinners; the one-sentence→structured-entity
create beat; one-tap claim with "Raise your hand · Maya picks who it goes
to."; "THE OTHERS — we'll let them know kindly."; claim→link-gear→photo
escalation with "Starts a loan offer — they'll give it back."; live RSVP
pill flips and chat streaming; linked-need 🔗 chips in chat; toast causality
ladder ("Account created" → "You offered to help — the requester will see
it" → "Loan offer sent — …"); "Name this group…" as the next-gathering
hook; serif-over-photo visual language throughout.
