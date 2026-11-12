# Issue #2638: Same user requests to borrow an item many times

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2638
**Filed:** 2026-07-04 | **Labels:** user-feedback
**Reporter context:** Android 0.26.0 (PROD), gear detail screen, "WHO'S USING IT" card

## UI Investigation

**Investigated:** 2026-07-04 on `ui_investigation_skill`@d2b8305 | **Environment:** hermetic e2e stack (`ripls-ui-investigate`, first run)
**Verdict:** Not tap-spam and not duplicate requests. Primary: **client-render / API-contract gap** — the "Who's using it" card renders every live loan transfer as an identical, undated person-row, and the ACTIVE loan is **double-listed** (once as the "Has it now" holder, once again as a queue row because `GetTransferContext` appends ACTIVE transfers to `pending_requests`). Secondary: **UX affordance gap** — the queue rows offer no way to distinguish or rescind one's own bookings.

### Reproduction

Seeded world: owner ("Marc Hall") + FOR_LOAN gear + borrower ("A. Maintainer")
in the gear's per-item community. All steps through the real Flutter Web
bundle:

1. Borrower opens `/gear/{id}`, taps **Book for borrowing**, taps a day
   (Jul 11), taps **Borrow it**. → one loan transfer (`RECIPIENT_SELECTED`,
   dated).
2. Repeats for a **disjoint** day (Jul 18). → a second live transfer. This is
   **by design**: bookings are loan transfers with date windows
   (`ClaimGearDays`, `server/services/gear/bookings.go`), merged only when
   ranges touch (±1 day) for the same recipient.
3. Who's-using-it card now shows **"A. Maintainer" twice** — identical rows,
   no dates, "Book for borrowing" CTA still active.
4. Owner starts the loan on the Jul-11 booking (`StartLoan`). Card now shows
   **three** Bryan rows — "HAS IT NOW" + 2 plain — while the database still
   holds exactly **2** transfers. The ACTIVE transfer is rendered twice.

The reporter's screenshot (1 × HAS IT NOW + 3 plain rows) is therefore
explained by as few as **2–3 real, legitimate reservations** — no duplicate
requests need to exist at all.

### Hypotheses tested

| # | Hypothesis | Probe | Observation | Verdict |
|---|---|---|---|---|
| H1 | Missing debounce lets accidental multi-tap create duplicate borrow requests (reporter's theory) | code + history | `ExpressInterest` has had an idempotent dedupe on (gear, recipient, community) since #2116 (v0.23.0 → in reporter's 0.26.0); `ClaimGearDays` merges same/adjacent-day rebooks into one transfer; the booking sheet guards on `isMutating` | **Refuted** |
| H2 | Multiple live loan transfers per (user, gear) exist by design (disjoint calendar bookings) | probe-01 (UI-only) | Two UI bookings on disjoint days → 2 `RECIPIENT_SELECTED` loan transfers in the DB (confirmed via psql: distinct pickup dates Jul 11 / Jul 18) | **Confirmed** |
| H3 | The who-card renders each live transfer as an identical bare name-row | probe-01 screenshot | "WHO'S USING IT — A. Maintainer / A. Maintainer", no dates, no grouping | **Confirmed** |
| H4 | The ACTIVE loan is double-listed (holder row + queue row) | probe-02 | `StartLoan` on booking 1 → card shows 3 rows from 2 DB transfers ("HAS IT NOW" + 2 plain) | **Confirmed** |
| H5 | No rescind affordance where the duplicates are shown | probe-01/02 | Queue rows are inert. A "Drop" affordance exists but only inside the booking calendar after tapping your own booked day. Giveaways get an inline "Withdraw" CTA; loans don't. | **Confirmed** |

### Evidence

- **DB ground truth after probe-01:** two `transfer` rows, both
  `state=RECIPIENT_SELECTED`, `transfer_type=LOAN`, same `recipient_id`,
  `estimated_pickup` 2026-07-11 and 2026-07-18.
- **Probe-02 screenshot vs DB:** card renders "A. Maintainer HAS IT NOW /
  A. Maintainer / A. Maintainer" while the DB holds exactly two transfers
  (one ACTIVE, one RECIPIENT_SELECTED) — one row is a phantom.
- **Code path:** `server/services/transfer/context.go` builds
  `pending_requests` by appending ALL transfers on the gear in
  `INTEREST_EXPRESSED`, then `RECIPIENT_SELECTED`, then `ACTIVE` states —
  no per-user aggregation, no exclusion of the transfer already surfaced as
  `gear.active_loan`, and `TransferRequest` carries only
  `{transfer_id, borrower, requested_at}` — no state, no booking window — so
  the client cannot render dates or filter the holder even if it wanted to.
- `app/lib/presentation/screens/gear/widgets/gear_who_card.dart` `_loan()`
  renders holder row + one bare `_personRow` per `pendingRequests` entry.

### Fix direction

1. **Server (`transfer/context.go`):** don't append the transfer that is the
   gear's active loan to `pending_requests` (it is already conveyed as
   `active_loan`) — kills the phantom row.
2. **API + server:** add the booking window (start/end unix sec) to
   `api.TransferRequest` so clients can render *which days* each pending
   booking covers.
3. **Client (`gear_who_card.dart`):** render queue rows with their date
   ranges (e.g. "Jul 18"), so two bookings by the same person read as two
   reservations, not a glitch; mark the viewer's own rows ("Your booking").
4. **UX (optional follow-up):** surface the existing Drop/withdraw affordance
   on the viewer's own queue rows instead of burying it in the calendar
   day-view; consider "Book more days" CTA label when the viewer already has
   a live booking.

**E2E spec:** not warranted per the harness cherry-pick criteria — the
double-listing is a server response-builder bug (server integration test
territory) and the undated rows are a widget-render concern (widget test
territory). No browser-only failure class remains once those two exist.

### Fix (verified in the same hermetic environment)

Implemented on this branch and re-verified by re-running the reproduction
probes against the rebuilt bundle + server:

1. `transfer/context.go` no longer appends ACTIVE **loans** to
   `pending_requests` (ACTIVE giveaways stay — their roster is driven by
   those entries). The probe-02 state now renders **2 rows for 2 transfers**:
   "A. Maintainer HAS IT NOW / A. Maintainer JUL 18".
2. `api.TransferRequest` gained `estimated_pickup_unix_sec` /
   `expected_return_unix_sec` (mirroring the storage model), populated by the
   context builder.
3. `gear_who_card.dart` + `gear_whos_using_panel.dart` render a compact date
   pill on each dated queue row (`JUL 11`, `JUL 18–20` via
   `DateTimeFormatter.formatBookingWindow`); undated interest requests render
   as before. Probe-01 now shows "A. Maintainer JUL 11 / A. Maintainer
   JUL 18" — two distinguishable reservations.

Tests: `TestGetGearTransferContext_ActiveLoanExcludedFromPendingRequests`
(flipped from the old inclusion test),
`TestGetGearTransferContext_BookingWindowsOnPendingRequests`, widget tests
for dated/undated queue rows, and `formatBookingWindow` unit tests. Full
server Go suite and full Flutter suite (4,926 tests) pass.

Deferred (follow-up candidates): inline withdraw/Drop affordance on the
viewer's own queue rows (today it lives only in the calendar day view), and
a "Book more days" CTA label when the viewer already has a booking. The
chat-side context builder (`chat/context.go`) never included ACTIVE
transfers, so it needed no change; its `TransferRequest`s don't carry the
booking window yet — populate via the same helper if the chat gear card
ever needs dates.

### Follow-up fixes (round 2, same branch)

The follow-up investigation's three defects are fixed and e2e-covered:

1. **Gear who-card CTA** flips to "Mark returned" when the viewer is the
   active-loan holder (wired to the existing complete-loan handler); booking
   more days stays on the card-body → calendar tap. New l10n strings
   (`gearMarkReturned` / `a11yGearMarkReturned`, en+es).
2. **Home NEEDS-YOU pill un-nested** (`home_editorial_row.dart`): the pill is
   now a sibling of the row `Tappable`, so it has its own semantics node —
   web clicks reach it and screen readers announce it.
3. **`ReleaseGearBooking` state validation**: only `INTEREST_EXPRESSED` /
   `RECIPIENT_SELECTED` bookings can be dropped; ACTIVE/terminal states get
   `FailedPrecondition` (`gear_booking_not_droppable`). The calendar
   day-dock also hides "Drop" for non-RESERVED bookings.

Tests: `TestReleaseGearBooking_RejectsStartedLoan`, widget tests for the CTA
swap and the pill's own semantics node, and
`e2e/tests/workflows/loan-return-mark-returned.spec.ts` — the Home-pill test
drives the real browser a11y tree, the layer where the swallow lived (a
widget test's gesture arena passes even when the web overlay eats the tap).

## Follow-up investigation: borrower return flow + calendar contents

**Investigated:** 2026-07-04 (same hermetic env, probes 03–07), prompted by two
questions: where does the "Has it now" borrower mark the loan returned, and why
can the who's-using calendar look empty while the card lists people?

### Where "mark returned" lives (and doesn't)

- **The only borrower affordance is on the Home tab:** the NEEDS YOU row's
  **"Mark returned" pill** (`home_editorial_row.dart` `onPill` →
  `runDecisionAction` → `_confirmTransferUpdate`: confirm sheet → typed
  `HOME_TRANSFER_ACTION_COMPLETE_LOAN` mutation → undo toast). The server
  correctly emits the decision (`portfolio/home_view.go` ACTIVE-loan branch,
  `acceptLabel: "Mark returned"`).
- **The gear screen has no borrower return affordance at all.** The who-card
  CTA stays "Book for borrowing"; the UP NEXT hero card and the NEEDS YOU row
  *body* both route to the gear screen — a dead end. The borrower checklist
  that contains "Confirm returned" (`GearMenuItems.buildBorrowerLoanItems`)
  is only mounted in `GearWhosUsingPanel`, which for loans is unreachable:
  `GearWhoCard._loan()` sends both card-body and CTA to the calendar, and
  only the giveaway branch calls `onExpandInterest`/`_expandWhosUsing`.
  Effectively dead code for loans.
- **Bug (web/a11y): the pill tap is swallowed by the row.** The row `Tappable`
  defaults to `excludeChildSemantics: true`, so the pill has no semantics
  node; on Flutter Web the `flt-semantics` overlay owns the whole row rect
  (verified: row node spans 0–390 × 64px and `elementFromPoint` at the pill
  center returns it), so any DOM-level click — every web user, and any
  mobile user with a screen reader — fires the ROW action (open gear screen)
  instead of the pill. Same failure class the who-card comment documents
  ("nesting made the card tap swallow the button tap"); fix likewise: don't
  nest, or set `excludeChildSemantics: false` so the pill keeps its node.
  Raw-hit-test mobile (no assistive tech) is unaffected, which is why the
  pill works on phones.
- **Bug (latent): "Drop" on an ACTIVE booking cancels an in-flight loan.**
  The calendar day-dock for your own booking offers only "Drop" + hand-off
  editing even while the loan is ACTIVE, and `ReleaseGearBooking` has **no
  state validation** — it sets CANCELLED unconditionally, skipping COMPLETED
  (and its impact accounting). Either hide Drop for ACTIVE bookings (show a
  return affordance instead) or make the server reject non-live states.

### Why the calendar can look empty

- **Dated bookings do render** on the calendar — borrower initials on their
  day cells (verified: AH on the active days, CF on Jul 14–15). The
  indicators are visual-only (day-cell aria labels carry no booking info).
- **Undated interest requests never appear.** `ListGearBookings` only returns
  transfers with a booking window (`isBookingTransfer`), so queue entries
  created via `ExpressInterest` (chat "request to borrow", web handoff) have
  no days to plot — a who-card full of names above a blank calendar is the
  expected result when the queue is undated-interest transfers. Post-fix
  these rows are distinguishable (no date pill vs. date pills).
- Related guard discovered: `ExpressInterest` is rejected outright
  ("currently unavailable") while the gear is out on loan, whereas
  `ClaimGearDays` still accepts future-dated bookings — the two request
  paths diverge in availability semantics.

### Residual notes

- `ExpressInterest`'s dedupe is keyed on (gear, recipient, community); gear
  shared into multiple communities could still yield one live transfer per
  community for the same user. Not exercised here; the per-item-community
  model makes it unlikely to be the reporter's case.
- Whether the reporter's 3 plain rows were 2 bookings + 1 phantom or 3 real
  bookings can't be determined from the screenshot — both are consistent
  with the confirmed mechanisms.
