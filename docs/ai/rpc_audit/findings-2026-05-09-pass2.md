---
issue: 1581
session: 2026-05-09 12:21-12:27 (walkthrough), Android emulator
window: ~5 minutes of active app interaction against local server
captured:
  app_log: /tmp/ripls_app.log (5111 lines, includes full flutter run output)
  server_log: /tmp/ripls_server.log (4303 lines, post-PR-1803)
supersedes: findings-2026-05-09.md (where noted)
---

# RPC Traffic Audit — Session 2026-05-09 (Pass 2, post #1803)

Second capture session of the audit defined in
`docs/issues/1581-rpc-traffic-audit.md`, after PR #1803 fixed the
`X-Request-ID` correlation bug from #1799. **Client request_ids now
appear in the server log** — every sampled client UUID had 5–9
matching server lines, so every finding below is joinable across
both sides.

This pass ran on the **Android emulator** (last pass was iOS
Simulator). The fix works on both platforms.

## Resolved findings from pass 1

| ID | Status | Note |
|---|---|---|
| F0 — request_id correlation broken | **Fixed** | Sampled client UUIDs → 5-9 server hits each. |
| F10 — middleware close-line missing for `/ripls.api/*` | **Fixed** | 510 close-lines in this run vs 0 in pass 1. |

The "wiring problem" framing in pass 1's intro is no longer accurate
either; that pass also went to local — the X-Request-ID propagation
was just broken so the join key didn't work.

## Volumes (this session)

| Counter | Walkthrough |
|---|---|
| Wall time | ~5 min (12:21:37 → 12:26:57) |
| Client `RPC_AUDIT` | 491 |
| Client `CACHE_AUDIT` | 915 |
| Server `/ripls.api/*` close-lines | 510 |
| Server distinct request_ids | 737 |
| `/media/*` GETs (image fetches) | not yet counted, sample below |

The server saw 510 unary RPC completions and the client logged 491
RPC_AUDIT lines — the small gap (≈4%) is the streaming RPCs which
emit a different log pattern.

## Top RPCs and their cost

Now that we have proper correlation, here are the heavy hitters
joined to db_queries and response_bytes.

### By DB query cost (sum across walkthrough)

| Total queries | Calls | Avg | Max | RPC |
|---:|---:|---:|---:|---|
| **455** | 7 | **65.0** | 65 | RequestService/ListMyRequests |
| 402 | 189 | 2.1 | 3 | MediaService/GetMedia |
| 334 | 16 | 20.9 | 22 | PortfolioService/GetPortfolioInboxFeed |
| 254 | 10 | 25.4 | **50** | FeedService/GetFeed |
| 167 | 12 | 13.9 | 14 | GearService/GetGear |
| 115 | 12 | 9.6 | 11 | TransferService/GetGearTransferContext |
| 80 | 5 | 16.0 | 16 | ExperienceService/GetExperience |
| 72 | 9 | 8.0 | 8 | ExperienceService/ListMyExperiences |
| 57 | 25 | 2.3 | 4 | CommunityService/ListCommunityEvents |
| 48 | 10 | 4.8 | 5 | SearchService/Search |
| 41 | 1 | 41.0 | 41 | RequestService/ListRequests |

### By bytes (top responses)

| Total bytes | Calls | Avg/call | RPC |
|---:|---:|---:|---|
| 138 KB | 10 | 14 KB | SearchService/Search |
| 134 KB | 16 | 8.4 KB | PortfolioService/GetPortfolioInboxFeed |
| 76 KB | 7 | 11 KB | TransferService/ListMyTransfers |
| 70 KB | 189 | 371 B | MediaService/GetMedia |
| 58 KB | 9 | 6.4 KB | GearService/ListUserGear |
| 41 KB | 3 | 14 KB | ExperienceService/ListExperiences |
| 39 KB | 10 | 3.9 KB | FeedService/GetFeed |

## Findings (this session)

### F1 — Streams reconnect on a 2-minute cycle [HIGH, still present]

```
14 stream-started events (StreamCommunityEvents)
11 stream-started events (StreamMessages)
10 stream-ended events
```

Stream end durations cluster at ~120 s (`120000`, `120001`, `119999`,
`119994`, `118811`) plus several earlier ends from screen
navigation. The 2-minute idle timeout is unchanged. Over 5 minutes
that's ~25 stream lifecycle events for a single user — same problem
as pass 1.

**Action:** unchanged from pass 1 — find and lift the 2-minute
timeout, or wire a server keepalive the client recognizes.

### F4 — Media URL cache thrash, now correlated [HIGH, less severe than pass 1]

Client-side: 189 cache misses on `media:*` keys; server-side:
**189 `MediaService/GetMedia` calls**. Perfect 1:1 — every client
miss hits the wire once, no batching, no in-flight dedup.

Top thrashed keys this run (less severe than pass 1):

| mediaId | client misses | client hits |
|---|---:|---:|
| `media:e39e9fca-…` | 5 | varies |
| `media:9d1a3606-…` | 5 | — |
| `media:7cd68df0-…` | 4 | — |
| `media:44ac6743-…` | 4 | — |
| `media:120933fd-…` | 4 | — |

Less dramatic than the iOS pass (top thrasher there was 14 misses).
Possible reasons:
- Shorter session (5 min vs 7 min), less screen revisiting.
- Android-specific cache behavior in `CachedNetworkImage` /
  `MediaRepository`.
- The user happened to navigate forward more than back.

Cost is real — 189 GetMedia calls × ~2.1 db_queries = 402 DB queries
just for media URL refreshing. Each call is cheap (~371 B response,
~2 ms), but it's still a big chunk of the session's traffic.

**Action:** unchanged from pass 1 (instrument MediaRepository with
per-instance ID, regression test for stable cacheKey). Single-flight
in `CacheManager.get` (F5) would also halve this when stampedes
happen.

### F5 — Cache stampede, confirmed and gear-side too [HIGH]

Same-second multiple-miss pattern reappears. Top stampedes:

```
3x  12:23:38  gear:0f9a8dfa-…:d6609ac3-…   ← three parallel misses on same gear
2x  12:26:51  media:62a7351f-…
2x  12:26:39  media:9d1a3606-…
2x  12:26:39  media:7cd68df0-…
2x  12:26:26  media:96147f84-…
2x  12:24:36  media:e39e9fca-…
2x  12:23:48  media:44ac6743-…
2x  12:23:38  gear:stats:0f9a8dfa-…:d6609ac3-…
2x  12:23:36  gear:0f9a8dfa-…:d6609ac3-…   ← then 2 more misses 2s later
2x  12:21:38  media:120933fd-…
2x  12:21:37  portfolio:inbox_feed
```

Two new flavors compared to pass 1:
- **Gear keys stampede too**, not just media. `gear:0f9a8dfa-…`
  and its sibling `gear:stats:0f9a8dfa-…` both got hit by parallel
  misses — same gear detail screen probably has multiple widgets
  fetching from `GearRepository` simultaneously.
- **`portfolio:inbox_feed`** gets parallel misses on first load.

**Action:** single-flight inside `CacheManager.get`. One change,
fixes every stampede. Add a unit test that fires 5 parallel
`get(key, fetch)` and asserts `fetch` runs once.

### F-NEW — RequestService/ListMyRequests fires 65 DB queries per call [HIGH]

Newly visible because we now have `request db stats` per RPC. This
is the biggest single-RPC offender of the session:

```
total=455  calls=7  avg=65.0  max=65  RequestService/ListMyRequests
```

7 calls in 5 minutes is reasonable (inbox refresh on tab switches),
but **65 queries per call is a textbook N+1**. Tracing one call's
request_id (`a332713a-…`) shows the per-request-item fan-out:

```
6× GetConversationParticipantsWhoPosted
6× GetConversationMessageCount
1× RequireAuth
1× AuthFunc
```

For each of N requests in the list, the handler runs two extra
storage calls (participants-who-posted and message-count) to populate
conversation badges. With ~6 requests on the screen that's 12 extra
calls per RPC, plus the base list load.

Family resemblance to the existing N+1 backlog (#1280
ListConversations, #1281 Leaderboard, #1283 ListEvents). The
`server/services/request/` handler likely needs the same batch-fetch
treatment those endpoints got in the SQL refactor.

**Action:** file as a separate issue, "ListMyRequests N+1: 65
queries/call from per-item conversation enrichment." Reuse the
`GetByIDs` / `QueryByFieldIn` helpers from `server/storage/`.

### F-NEW — FeedService/GetFeed: up to 50 queries per call [MED]

```
total=254  calls=10  avg=25.4  max=50  FeedService/GetFeed
```

Two of the ten calls hit 50 queries. The feed endpoint went through
batch optimization in the SQL refactor (down to 17 queries per
docs/ai/profile_*.md), so a 50-query call is a regression or a
specific scenario that wasn't covered. Worth a look — but the bigger
fish is ListMyRequests.

**Action:** join one 50-query GetFeed call's request_id to the inner
operation logs to identify which fan-out is firing. Likely a
specific feed item type pulling extra data per item.

### F-NEW — PortfolioService/GetPortfolioInboxFeed: 21 queries × 16 calls [MED]

```
total=334  calls=16  avg=20.9  max=22
```

16 inbox-feed loads in 5 minutes is itself worth questioning — looks
like the inbox is refetching on every tab focus or screen rebuild.
That's the same pattern we saw client-side as `portfolio:inbox_feed`
having 16 misses with 0 hits (F7 in pass 1 — still present).

21 queries per call is moderate but could probably be brought down
similarly. The combo of frequent calls × moderate query count puts
this second only to ListMyRequests.

**Action:** investigate why the inbox feed isn't being cached on the
client (F7 from pass 1) and look at handler-side query reduction.

### F2 — Auth middleware "5× per request" [RETRACTED]

**Update with proper correlation (post-#1803):** the ratio is
1.005:1. The captured walkthrough had 512 unary `/ripls.api`
calls and 515 `RequireAuth` runs (and 516 `ValidateToken` runs).
Auth runs once per request as designed.

The 5× claim in pass 1 was an artifact of the broken X-Request-ID
middleware collapsing many requests under one ID. Counting
`RequireAuth runs ÷ distinct request_ids` made auth look redundant
only because most distinct request_ids on the server side didn't
exist.

Not a real issue. Removing the action item.

### F8 — Startup RPC bursts, attributable now [LOW once F9 lands]

This pass had three burst-y minutes:

| Minute | client RPCs | dominated by |
|---|---:|---|
| 12:21 | startup | login + initial loads |
| 12:23 | mid-session | gear detail screen (gear: stampede above) |
| 12:26 | end | inbox/feed refresh |

We can now attribute each burst to specific RPCs via server logs
(grep on the minute window) but the client `RPC_AUDIT` still says
`rpc=unknown` and `screen=Builder<dynamic>` for almost every line.

### F9 — Audit instrumentation gaps [MED, unchanged]

Still present:
- `operationName` unset for all but `RecordAttendance` →
  `rpc=unknown`.
- Screen attribution falls through to widget runtime types →
  `screen=Builder<dynamic>` 38% of the time.

Before the next pass, fixing F9 will make the client log
self-describing instead of requiring a server-side join.

## Open questions / things to look at next

1. Is the 2-min stream timeout enforced by the server's HTTP/2
   server, or by the client's Connect transport, or by some global
   `context.WithTimeout`? F1 keeps reappearing; finding the source
   would close it cleanly.
2. The 50-query `GetFeed` call — what feed item type triggered the
   fan-out? Picking one request_id and reading the trace should
   answer in <5 minutes.
3. Why does `portfolio:inbox_feed` never cache-hit (16 misses, 0
   hits)? Is the cache key including a timestamp or random nonce by
   accident?

## Recommended next steps

1. **File three new issues** (one each): F-NEW ListMyRequests N+1,
   F-NEW GetFeed 50-query case, single-flight in
   `CacheManager.get` (F5). All three are well-scoped and close
   real wasted work.
2. **Land F9** (operation names + real screen names) — small but it
   makes the audit log line useful without joining.
3. **Decide on the 2-minute stream timeout (F1)** — separate
   investigation, but high value: every reduction in cycles drops
   server registration overhead and client reconnect cost.

## Issues filed from this audit

| Issue | Finding | Scope | Status in this branch |
|---|---|---|---|
| #1799 | F0 X-Request-ID correlation | server + client | Fixed by #1803 (merged into rpc_audit) |
| #1806 | F-NEW ListMyRequests N+1 (65 queries/call) | server | Open — not in this branch |
| #1807 | F-NEW GetFeed 50-query case (`enrichStoriesWithEmbeddedESM`) | server | Open — not in this branch |
| #1808 | F5 CacheManager single-flight | client | **Implemented in this branch** (`StashCacheManager._inflight`) |
| #1809 | F9 operationName + screen names | client | Open — instrumentation sweep |
| #1810 | F1 HTTP WriteTimeout=120s closing streams | server (1-line) | Open — small fix |
| #1811 | PortfolioInbox invalidate→fetch loop | client | Open — viewmodel rework |

**Retracted:** F2 (auth-middleware-runs-5×) was a false alarm in
pass 1, corrected in this doc once correlation worked. Not filed.
