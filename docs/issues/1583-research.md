# Issue #1583 — Research Notes (working file)

Internal scratch for the beta-1-month retrospective. Not published.
Pulled from prod analytics (`analytics.ripls.org`, database 2),
release notes 0.10.0 → 0.17.0, and post-0.17 PRs.

## Headline numbers (as of 2026-04-29)

Source: single SQL run against prod, cross-checked against schema.

| Metric                         | All-time | April only |
|--------------------------------|---------:|-----------:|
| Users (non-sim, non-deleted)   |       17 |         14 |
| Communities (non-sim)          |       12 |          3 |
| Memberships (community_user)   |       35 |          — |
| Gear listed                    |       62 |         20 |
| Requests opened                |       10 |          — |
| Requests fulfilled             |        2 |          — |
| Experiences                    |       29 |          — |
| Experiences completed          |        8 |          — |
| Transfers (total)              |       14 |          — |
| — Loans                        |       12 |          — |
| — Giveaways                    |        2 |          — |
| Transfers completed            |        4 |          — |
| — Loans completed              |        3 |          — |
| — Giveaways completed          |        1 |          — |
| Photos (raw `media` count)     |    1,354 |          — |
| — non-stock (excl. server fetches) |  671 |          — |
| — by real (non-sim) users      |       89 |          — |
| — by users who shared to a community | 87 |          — |

The page uses **87** for "photos uploaded onto shared items." The
1,354 raw figure inflates the user-facing story because it counts
server-side stock-imagery fetches (~683) plus uploads by simulation
users; filtering to (a) `source_stock_image_id IS NULL OR ''`,
(b) `user.simulation_id = ''`, and (c) the user appearing as
`gear.owner_id` / `experience.owner_id` / `request.requester_id` on
at least one row joined into `community_*` lands on 87. There is no
direct foreign key from `media` to gear/experience/request — the
linkage is encoded inside each parent's `binary_proto`, which is not
SQL-queryable, so this is the closest reproducible bound.
| Chat messages                  |      387 |          — |
| RSVPs                          |       56 |          — |

**N is small.** 17 total registered users since launch; 14 of them in
April. The retrospective tone has to honor that: this is a private
beta with hand-picked seed cohorts, not a launch story. The story is
"what's possible at small N" — coordination, sharing, real activity —
not scale.

## Aggregate impact (across completed activity)

Source: sum of `impact_estimate_*` columns on `state='4'` transfers
+ experiences + requests.

- **$552.50** money saved
- **6,300 g** (≈ 6.3 kg) CO₂e avoided
- **3,120 minutes** (≈ 52 hours) saved

These are conservative — only completed/fulfilled items count toward
the aggregate; in-flight activity is not included.

## Weekly activity (Apr 1 → Apr 27)

Five-week picture (week start = Sunday). Launch week is the week of
2026-03-30, which contains April 1.

| Week start | Users | Comms | Gear | Transfers | RSVPs | Chat msgs |
|------------|------:|------:|-----:|----------:|------:|----------:|
| 2026-03-30 |     6 |     1 |    1 |         1 |    10 |        65 |
| 2026-04-06 |     3 |     1 |   12 |         2 |    16 |       114 |
| 2026-04-13 |     4 |     0 |    5 |         3 |     5 |        65 |
| 2026-04-20 |     1 |     1 |    2 |         2 |     0 |        14 |
| 2026-04-27 |     0 |     0 |    0 |         0 |     3 |        24 |

Shape: launch-week onboarding burst; activity peak in week 2 (12 new
gear, 114 chat messages); mid-month tail; modest late-April pickup.

## Top gear categories (life-of-product)

| Category               | Count |
|------------------------|------:|
| Kitchen Appliances     |    10 |
| Electronics            |     7 |
| Books                  |     6 |
| Puzzles & Games        |     6 |
| Musical Instruments    |     4 |
| Sports Equipment       |     2 |
| Clothing               |     2 |
| Games & Puzzles        |     2 |
| Books and Media        |     2 |
| Bicycles               |     1 |
| Outdoor Power Equipment|     1 |
| Lawn & Garden          |     1 |
| (other singletons)     |     5 |

## Tester roster — confirmed (mapping resolved 2026-04-29)

Email source: in-app feedback bot submissions since 2026-04-01 +
"reported by &lt;handle&gt;" tags in release notes 0.10.0 → 0.16.0.

| Email                         | First name | Notes |
|-------------------------------|------------|-------|
| user4@example.org                | Eric       | external tester |
| user5@example.org              | Joanne     | external tester |
| user6@example.org            | Lisa       | external tester |
| user7@example.org          | Leslie     | external tester |
| user8@example.org         | Marc       | external tester |
| user9@example.org                | —          | test user, omit |
| user10@example.org              | —          | co-founder, omit |
| user11@example.org         | —          | engineer, omit |
| user2@example.org               | —          | project owner, omit |

Page roll-call (alphabetical): **Eric · Joanne · Leslie · Lisa · Marc**.

## Feature shortlist (for the visual tour)

Headlines worth a screenshot or short video, organized by theme.

### Coordination (core fabric)
- **Planning: needs & contributions (0.17)** — collaborative Plan tab
  on experiences and requests; slot counts; coalesced system messages
  in chat. PRs #1201, #1208.
- **Time polls (0.17)** — vote on proposed slots; chosen slot becomes
  the experience time; closed polls preserved. PRs #1219, #1245.
- **Generalized undo (0.17)** — reverse any state change from chat,
  story feed, or snackbar; cascade-aware. PRs #964, #1207, #1228.
- **Requests + offers on experiences (0.17)** — coordinate what
  attendees bring or need help with from the experience view. PR #1192.
- **Inline "Add to calendar" icon on Event tab (post-0.17)** — PR #1550.
- **Per-community notification preferences (post-0.17)** — PR #1553.

### Creation (host friction)
- **Streaming AI generation (0.17)** — gear, request, experience
  previews populate incrementally; ~78% faster time-to-first-content.
  PRs #1157, #1218, #1232, #1241, #1253, #1259, #1502, #1507.
- **Description mid-stream + skeleton coverage (post-0.17)** — PR #1502.
- **End-to-end community-creation streaming (post-0.17)** — PR #1567.
- **Gear-image and request-image streaming (post-0.17)** — PR #1554.
- **Structured event extraction from JSON-LD (0.14)** — 100% reliable
  date/time parsing for Eventbrite, Meetup, Luma, AllEvents.
- **Gear-from-URL improvements (0.10)** — UA fix, localized errors.

### Onboarding & access
- **Phone login (0.14)** — passwordless via Firebase, iOS + Android.
- **Deferred deep linking (0.13, 0.13.1)** — invite codes survive a
  Play Store / App Store install.
- **Watchlist & timeless inbox (0.16)** — owned, borrowed, RSVP'd,
  offered items auto-watched; access ring shows participants.
- **Unified content views (0.16)** — gear/request/experience screens
  share a swipeable layout; integrated actions.
- **Access circles (0.16)** — unified sharing sheet across content
  types.

### Real-time and chat
- **Real-time updates (0.12)** — three-layer hybrid (gRPC streaming +
  FCM + polling backstop). No more pull-to-refresh.
- **Multi-image chat picker (0.15)** — pending-attachment strip with
  preview.
- **Tappable links in descriptions (0.15)** — `LinkifiedText`.
- **Unified chat experience (0.15)** — removed standalone
  ConversationScreen; in-context chat everywhere.

### Reliability and platform
- **iOS notification fixes (0.10)** — three independent root causes.
- **Chat history refresh on resume + reconnect gap-fill
  (post-0.17)** — PR #1566.
- **Panic recovery on streaming goroutines (post-0.17)** — PR #1563.
- **Rate-limited auth endpoints (post-0.17)** — PR #1561.
- **JWT signing-secret hardening (post-0.17)** — PR #1572.
- **Always-on Cloud Run (0.11)** — `min_instances=1`, no cold starts.
- **App Links / Universal Links verification (0.12.2)** — served
  directly from the Go server.
- **Accessibility migration (post-0.17)** — full WCAG-A coverage with
  CI-enforced ratchet. PR #1489.

### Bug fixes worth a callout
Only included if the fix story is a useful "we hear you" beat.
- iOS notifications, three root causes (0.10) — matt
- Auto-capitalization on feedback sheet (0.10) — lesliehall
- Experience completion from JOINED state (0.12.2) — lesliehall
- Stale chat messages from notifications (0.15) — bryan
- Wrong location from event URLs (0.15)
- Borrow prompt on own items (0.16) — bryan
- Community member count showing 0 (0.16) — bryan
- Inbox showing stale loan status (0.16) — bryan
- Recipient withdrawal ending the giveaway (post-0.17, #1565)
- Chat doesn't show message from notification (post-0.17, #1566)

## Page outline (working)

Target ~1,500–2,000 words. Four sections.

1. **What ripls is, briefly** (~250 words). Latent host, small
   circles, AI does the chasing. Pull from `docs/users/priorities.md`
   and `docs/users/latent-host.md`. No marketing puffery.
2. **One month of fabric** (~500 words + 5–7 charts/tables). Headline
   numbers + weekly activity + top categories + aggregate impact.
   Acknowledge small N; lean into the "small and real" frame.
3. **What shipped** (~700 words + 6–8 short videos / screenshots).
   Group by theme (coordination / creation / onboarding /
   real-time / reliability). One paragraph per theme; 2–3 features
   per theme with a video each. End with an "and dozens of fixes"
   roll-up linking the release notes index.
4. **Thanks** (~150 words). First names alphabetically; brief shared
   thank-you paragraph.

## Visual asset inventory — PRs with attachments

Source: `gh pr view <n> --json body | grep user-attachments`. 21 PRs
with 47 total assets across the launch-to-now window. Sorted by
asset count.

| PR | Assets | Topic |
|---:|------:|-------|
| 1140 | 6 | Don't overwrite typed descriptions w/ AI (creation) |
| 1102 | 5 | Watchlist + portfolio + access-ring redesign |
| 1201 | 4 | Experience needs & contributions v2 (Plan tab) |
| 1258 | 3 | Sharing / RSVP / time-poll bug fixes |
| 1245 | 3 | Time polls on the Plan tab |
| 1228 | 3 | Generalized undo |
| 1192 | 3 | Requests & offers on experiences |
| 1553 | 2 | Per-community notification preferences |
| 1372 | 2 | Metric generation fixes |
| 1178 | 2 | RSVP issues + missing thumbnail |
| 1151 | 2 | Inbox → watchlist |
| 1138 | 2 | Stale chat from notification |
| 1114 | 2 | Small-screen creation modal overflow |
| 1550 | 1 | Add to calendar icon |
| 1473 | 1 | Progressive feed loading |
| 1208 | 1 | Plan tab — requests |
| 1173 | 1 | Community member count fix |
| 1170 | 1 | Inbox consistency w/ content view actions |
| 1168 | 1 | Swiping content animation |
| 1139 | 1 | Multi-photo chat |
| 1131 | 1 | Passwordless phone signup |

## Asset fetch path

GitHub user-attachments are session-locked. The fetch script lives
at `scripts/fetch_pr_media.sh`; it reads the `user_session` cookie
from `$HOME/.gh-user-session` (gitignored — never commit) and
downloads every `user-attachments/assets/<uuid>` referenced in a
PR's body to `/tmp/pr-media/<pr>/<uuid>.<ext>`.

Cookies expire ~every 14 days; refresh by re-pasting the cookie
value when downloads start 404'ing.

## SQL snippets (reproducibility)

Pasteable, run via `/api/dataset` with `database: 2`,
`type: "native"`. Dev DB is structurally identical; rerun against
`https://analytics-dev.ripls.org` if needed.

```sql
-- Headline counts (life-of-product + April-window)
-- See body for full query; uses unix-second timestamps and
-- string-int enum values (e.g. transfer_type = '1').
```

```sql
-- Weekly activity series
-- date_trunc('week', to_timestamp(<created_at>_unix_sec))
-- bucket = Sun-anchored.
```

```sql
-- Aggregate impact: sum impact_estimate_money_saved_value_usd_mean
-- across state='4' rows of transfer + experience + request,
-- non-deleted.
```

Window: `t0 = 1775347200` (2026-04-01), `t1 = 1777939200` (2026-04-30).

## Open input requests

- [ ] **Email → first-name mapping + consent** for the 7 external
      tester candidates above (jsyben, ledreher, mr.marchall, eric,
      lesliehall — plus matt and thomas if they want a public credit
      separate from being on the project domain).
- [ ] **Confirm publication target** — the plan recommends a single
      static page at `website/content/beta-1-month.html` with one
      footer link from the homepage. Confirm this matches Bryan's
      mental picture before HTML lands.
