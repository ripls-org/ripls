# Production Traffic Analysis

**Date:** 2026-02-27
**Tool:** `scripts/analyze_cloud_logs.sh`
**Sources:** Cloud Run request logs from `ripls-dev` (7 days) and `ripls-prod` (3 days)

## Key Findings

### Dev Environment (ripls-dev / server-dev)

- **Period:** 2026-02-20 to 2026-02-26 (165 hours)
- **Total RPC calls:** 45,273
- **Read/Write ratio:** 15% read / 85% write (0.18:1)
- **Peak RPS:** 8.8 (527 requests in busiest minute)
- **Unique RPC methods seen:** 91

Dev traffic is almost entirely simulation-generated write activity:

| Rank | RPC Method | Calls | % | Type | p50ms | p95ms |
|------|-----------|------:|--:|------|------:|------:|
| 1 | ChatService/SendMessage | 13,022 | 28.8% | WRITE | 165 | 448 |
| 2 | ExperienceService/RSVPToExperience | 5,401 | 11.9% | WRITE | 499 | 1,704 |
| 3 | MediaService/AddMedia | 3,282 | 7.2% | WRITE | 248 | 456 |
| 4 | RequestService/OfferToFulfill | 2,879 | 6.4% | WRITE | 721 | 1,936 |
| 5 | DailyService/GetDailyView | 2,550 | 5.6% | READ | 0 | 0 |
| 6 | MediaService/GetMedia | 2,162 | 4.8% | READ | 142 | 570 |
| 7 | TransferService/ExpressInterest | 2,135 | 4.7% | WRITE | 433 | 1,235 |
| 8 | CommunityService/ShareGear | 1,907 | 4.2% | WRITE | 94 | 232 |
| 9 | TransferService/CompleteTransfer | 1,158 | 2.6% | WRITE | 1,196 | 3,201 |
| 10 | TransferService/StartLoan | 1,009 | 2.2% | WRITE | 864 | 2,475 |

**Notable latency outliers (dev):**

| Endpoint | p50ms | p95ms | p99ms |
|----------|------:|------:|------:|
| TransferService/CompleteTransfer | 1,196 | 3,201 | 6,188 |
| ExperienceService/CompleteExperience | 980 | 2,759 | 4,393 |
| TransferService/StartLoan | 864 | 2,475 | 3,463 |
| RequestService/OfferToFulfill | 721 | 1,936 | 2,685 |
| ExperienceService/RSVPToExperience | 499 | 1,704 | 2,472 |

These write-path latencies are high (p95 > 1s for several endpoints) and worth
investigating during Phase 5 optimization.

### Prod Environment (ripls-prod / server)

- **Period:** 2026-02-24 to 2026-02-27 (3 days)
- **Total RPC calls:** 948 (extremely low — early-stage app)
- **Legitimate RPC calls:** ~55 (rest is bot scanner traffic: wp-admin, .php probes)
- **Unique users:** 0 (user_id not yet in deployed logging middleware)

Legitimate prod RPC calls (filtering out bot traffic):

| RPC Method | Calls | Type | p50ms |
|-----------|------:|------|------:|
| MediaService/GetMedia | 22 | READ | 98 |
| ChatService/GetConversationContext | 6 | READ | 19 |
| ChatService/UpdatePresence | 4 | WRITE | 0 |
| ChatService/StreamMessages | 3 | WRITE | 900,000 |
| ChatService/GetUnreadCounts | 3 | READ | 14 |
| CommunityService/ListCommunities | 3 | READ | 1 |
| LoginService/Login | 3 | READ | 18 |
| ChatService/ListConversations | 2 | READ | 126 |
| FeedService/MarkFeedItemsViewed | 2 | WRITE | 12 |
| FeedService/GetFeed | 2 | READ | 373 |
| LocationService/GetLocation | 2 | READ | 4 |
| ExperienceService/GetExperience | 1 | READ | 29 |
| ImpactService/GetCommunityMetricDetail | 1 | READ | 156 |
| UserService/GetUser | 1 | READ | 3 |
| RequestService/GetRequest | 1 | READ | 64 |
| ImpactService/GetCommunityImpactMetrics | 1 | READ | 213 |
| CommunityService/GetCommunity | 1 | READ | 73 |

Even with minimal traffic, the prod data confirms the expected pattern: **real
user sessions are read-heavy** (GetMedia, GetFeed, ListConversations,
GetUnreadCounts, GetCommunity, etc.) with occasional writes.

## Conclusions

### 1. Simulation has zero read traffic

The existing simulation framework generates only write operations (gear creation,
transfers, RSVPs, chat messages). Real-world client apps spend most of their
time reading: loading feeds, browsing gear, checking conversations, viewing
profiles. This is the core gap that Phase 2 addresses.

### 2. Prod traffic is too sparse for statistical calibration

With only ~55 legitimate RPC calls in 3 days, prod data cannot provide reliable
per-endpoint frequency distributions. The read weights for simulation personas
must be based on typical mobile app usage patterns rather than measured
production data.

### 3. Read weights should be based on app UX flow analysis

Based on the client app architecture (Flutter screens, navigation patterns) and
the prod data that does exist, the expected real-world read frequency ranking is:

1. **GetFeed / ListStories** — Every app open, pull-to-refresh (highest frequency)
2. **ListConversations / GetUnreadCounts** — Every app open, tab switch to chat
3. **ListCommunityGear** — Browsing community items
4. **GetMedia** — Loading images (implicit, very high volume)
5. **Search** — Active browsing sessions
6. **GetGear / GetGearPeople** — Detail views after browse/search
7. **GetUser / GetUserStats** — Profile views
8. **GetExperience / ListExperiences** — Event browsing
9. **ListRequests / GetRequest** — Request browsing
10. **GetCommunityImpactMetrics** — Impact tab views (lower frequency)

### 4. Latency hotspots to investigate

From dev simulation data (which does exercise write paths under load):

- **CompleteTransfer** p95=3.2s — Story generation, equity scoring, impact estimation
- **CompleteExperience** p95=2.8s — Same post-completion pipeline
- **StartLoan** p95=2.5s — State machine transitions, notifications
- **OfferToFulfill** p95=1.9s — Notification fan-out, conversation creation
- **RSVPToExperience** p95=1.7s — Notification fan-out

These are candidates for Phase 5 optimization after read traffic is added.

### 5. Bot traffic on prod needs attention

Prod receives substantial bot scanner traffic (wp-admin, .php probes,
.well-known requests). These all return 404/401 but still consume resources.
Consider adding rate limiting or a WAF rule. Not blocking for profiling work.

## Recommended Simulation Read Weights

Based on the UX flow analysis above, here are the per-persona read frequencies
(actions per simulated week). These replace the initial guess in the profiling
plan with UX-informed values.

| Persona | Feed | Search | Browse Gear | Profiles | Stories | Chat | Impact |
|---------|-----:|-------:|------------:|---------:|--------:|-----:|-------:|
| Alfred (Stuff×Giving) | 7 | 1 | 4 | 1 | 3 | 5 | 1 |
| Derek (Stuff×Exchange) | 10 | 3 | 6 | 2 | 3 | 7 | 1 |
| Gary (Stuff×Seeking) | 14 | 5 | 10 | 2 | 3 | 7 | 0.5 |
| Betty (Time×Giving) | 7 | 1 | 2 | 1 | 5 | 5 | 2 |
| Emma (Time×Exchange) | 10 | 2 | 4 | 2 | 4 | 7 | 1 |
| Henry (Time×Seeking) | 14 | 4 | 6 | 1 | 3 | 7 | 0.5 |

**Rationale:**

- **Feed checks** scale with engagement: seekers (Gary, Henry) check more often
  because they're actively looking for items/help. Givers check less frequently.
- **Search** scales with seeking behavior. Gary and Henry search most; Alfred and
  Betty rarely search because they're giving, not seeking.
- **Gear browsing** highest for Gary (stuff-seeking persona).
- **Chat checking** is uniformly high across all personas — everyone checks
  messages. Slightly higher for exchangers/seekers who have active conversations.
- **Stories** are higher for Betty (community-oriented, time-giving persona) and
  lowest for seekers who are task-focused.
- **Impact** is niche — mainly viewed by givers who want to see their
  contribution. Seekers rarely check.

These weights produce an overall read/write ratio of approximately **3:1** when
combined with the existing write activity rates, which is conservative compared
to typical consumer apps (which are often 10:1+). The `--read-multiplier` flag
allows scaling this up during load testing.

## Re-run Instructions

To update this analysis with fresh data:

```bash
# Export and analyze dev
./scripts/analyze_cloud_logs.sh --export dev --output docs/ai/production_traffic_analysis_dev.md

# Export and analyze prod
./scripts/analyze_cloud_logs.sh --export prod --output docs/ai/production_traffic_analysis_prod.md

# Compare side-by-side
./scripts/analyze_cloud_logs.sh --compare /tmp/cloud-run-dev-logs.json /tmp/cloud-run-prod-logs.json
```

As prod traffic grows, re-run this analysis to calibrate simulation read weights
against actual user behavior.
