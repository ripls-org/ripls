# Performance Profile

- **Branch:** load_and_profile
- **Commit:** 24110789
- **Date:** 2026-02-27T14:42:58Z
- **Scenario:** load-test-small (seed=42, concurrency=0)
- **Read traffic:** enabled (multiplier: 1.0)
- **Total HTTP requests:** 3373
- **Requests with DB:** 3371
- **Total SQL queries:** 137621
- **Total DB time:** 70487ms
- **Errors:** 2
- **Wall-clock duration:** 83s

## Per-Endpoint Latency

| RPC Method | Calls | P50ms | P95ms | P99ms | Max ms | Errors |
|------------|------:|------:|------:|------:|-------:|-------:|
| FeedService/GetFeed | 446 | 49 | 62 | 67 | 76 | 0 |
| FeedService/MarkFeedItemsViewed | 446 | 20 | 23 | 25 | 27 | 0 |
| MediaService/AddMedia | 324 | 25 | 34 | 36 | 41 | 0 |
| ChatService/GetUnreadCounts | 258 | 5 | 13 | 15 | 16 | 0 |
| ChatService/ListConversations | 258 | 12 | 28 | 34 | 40 | 0 |
| CommunityService/ListCommunityGear | 218 | 113 | 123 | 127 | 132 | 0 |
| ChatService/SendMessage | 186 | 2 | 4 | 5 | 5 | 0 |
| GearService/GetGear | 137 | 3 | 4 | 5 | 6 | 0 |
| FeedService/ListStories | 131 | 1 | 2 | 3 | 4 | 0 |
| SearchService/Search | 120 | 12 | 15 | 16 | 20 | 0 |
| ChatService/GetConversationHistory | 101 | 4 | 9 | 10 | 11 | 0 |
| ExperienceService/GetExperience | 67 | 4 | 5 | 6 | 6 | 0 |
| ExperienceService/GetExperiencePeople | 67 | 2 | 4 | 5 | 6 | 0 |
| ExperienceService/RSVPToExperience | 67 | 17 | 23 | 24 | 24 | 0 |
| UserService/GetUser | 63 | 0 | 0 | 2 | 3 | 0 |
| CommunityService/ShareGear | 54 | 12 | 15 | 17 | 17 | 0 |
| GearService/SaveGear | 52 | 0 | 0 | 1 | 2 | 0 |
| ImpactService/GetCommunityImpactMetrics | 38 | 72 | 81 | 84 | 87 | 0 |
| GearService/GetGearPeople | 30 | 1 | 2 | 2 | 2 | 0 |
| RequestService/GetRequest | 27 | 4 | 6 | 6 | 7 | 0 |
| RequestService/OfferToFulfill | 26 | 20 | 24 | 25 | 26 | 0 |
| TransferService/GetTransfer | 23 | 2 | 3 | 3 | 5 | 0 |
| UserService/GetUserStats | 22 | 2 | 3 | 3 | 4 | 0 |
| TransferService/ExpressInterest | 20 | 11 | 15 | 15 | 15 | 2 |
| LocationService/SaveLocation | 17 | 1 | 4 | 4 | 6 | 0 |
| RequestService/GetRequestPeople | 17 | 1 | 3 | 3 | 3 | 0 |
| RequestService/SubmitRequest | 17 | 12 | 15 | 15 | 15 | 0 |
| TransferService/CompleteTransfer | 17 | 21 | 22 | 22 | 26 | 0 |
| TransferService/StartLoan | 17 | 19 | 23 | 23 | 23 | 0 |
| ExperienceService/SaveExperience | 14 | 4 | 5 | 5 | 6 | 0 |
| ExperienceService/ShareExperience | 14 | 10 | 11 | 11 | 15 | 0 |
| RequestService/MarkRequestFulfilled | 14 | 29 | 31 | 31 | 32 | 0 |
| ChatService/GetConversationForTransfer | 12 | 3 | 4 | 4 | 4 | 0 |
| ExperienceService/CompleteExperience | 11 | 19 | 21 | 21 | 22 | 0 |
| ExperienceService/MarkExperienceInProcess | 11 | 6 | 6 | 6 | 8 | 0 |
| LoginService/Register | 10 | 185 | 187 | 187 | 191 | 0 |
| ImpactService/GetUserImpactMetrics | 9 | 6 | 12 | 12 | 15 | 0 |
| ExperienceService/CancelExperience | 3 | 6 | 6 | 6 | 8 | 0 |
| AdminService/CleanupSimulation | 2 | 46 | 1 | 1 | 46 | 0 |
| AdminService/ListSimulations | 1 | 6 | 6 | 6 | 6 | 0 |
| CommunityService/CreateCommunity | 1 | 13 | 13 | 13 | 13 | 0 |
| CommunityService/GetOrCreateInviteLink | 1 | 3 | 3 | 3 | 3 | 0 |
| RequestService/CancelRequest | 1 | 14 | 14 | 14 | 14 | 0 |
| RequestService/WithdrawOffer | 1 | 8 | 8 | 8 | 8 | 0 |
| TransferService/CancelTransfer | 1 | 9 | 9 | 9 | 9 | 0 |

## Per-Endpoint DB Queries

| Endpoint | Calls | Total Q | Min | Avg | P50 | P95 | Max |
|----------|------:|--------:|----:|----:|----:|----:|----:|
| /ripls.api.CommunityService/ListCommunityGear | 218 | 47275 | 211 | 216.9 | 218 | 220 | 220 |
| /ripls.api.FeedService/GetFeed | 446 | 42733 | 61 | 95.8 | 98 | 117 | 122 |
| /ripls.api.FeedService/MarkFeedItemsViewed | 446 | 18286 | 41 | 41 | 41 | 41 | 41 |
| /ripls.api.ChatService/ListConversations | 258 | 7027 | 1 | 27.2 | 27 | 54 | 66 |
| /ripls.api.ImpactService/GetCommunityImpactMetrics | 38 | 5283 | 118 | 139 | 139 | 161 | 163 |
| /ripls.api.ChatService/GetUnreadCounts | 258 | 3083 | 1 | 11.9 | 11 | 25 | 31 |
| /ripls.api.ExperienceService/RSVPToExperience | 67 | 1902 | 19 | 28.4 | 31 | 32 | 32 |
| /ripls.api.ChatService/SendMessage | 186 | 1026 | 5 | 5.5 | 5 | 6 | 10 |
| /ripls.api.SearchService/Search | 120 | 1008 | 8 | 8.4 | 8 | 10 | 10 |
| /ripls.api.CommunityService/ShareGear | 54 | 998 | 5 | 18.5 | 19 | 19 | 19 |
| /ripls.api.ChatService/GetConversationHistory | 101 | 932 | 3 | 9.2 | 10 | 19 | 20 |
| /ripls.api.GearService/GetGear | 137 | 822 | 6 | 6 | 6 | 6 | 6 |
| /ripls.api.RequestService/OfferToFulfill | 26 | 804 | 30 | 30.9 | 31 | 31 | 31 |
| /ripls.api.MediaService/AddMedia | 324 | 648 | 2 | 2 | 2 | 2 | 2 |
| /ripls.api.TransferService/CompleteTransfer | 17 | 634 | 36 | 37.3 | 38 | 39 | 39 |
| /ripls.api.RequestService/MarkRequestFulfilled | 14 | 623 | 43 | 44.5 | 44 | 46 | 46 |
| /ripls.api.TransferService/StartLoan | 17 | 528 | 31 | 31.1 | 31 | 31 | 32 |
| /ripls.api.ExperienceService/GetExperience | 67 | 524 | 7 | 7.8 | 8 | 8 | 8 |
| /ripls.api.TransferService/ExpressInterest | 20 | 434 | 1 | 21.7 | 24 | 24 | 24 |
| /ripls.api.FeedService/ListStories | 131 | 384 | 2 | 2.9 | 3 | 3 | 3 |
| /ripls.api.ExperienceService/GetExperiencePeople | 67 | 374 | 3 | 5.6 | 6 | 8 | 10 |
| /ripls.api.ExperienceService/CompleteExperience | 11 | 365 | 29 | 33.2 | 34 | 35 | 35 |
| /ripls.api.RequestService/SubmitRequest | 17 | 272 | 16 | 16 | 16 | 16 | 16 |
| /ripls.api.RequestService/GetRequest | 27 | 250 | 8 | 9.3 | 10 | 10 | 10 |
| /ripls.api.ExperienceService/ShareExperience | 14 | 238 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.ImpactService/GetUserImpactMetrics | 9 | 142 | 10 | 15.8 | 13 | 20 | 30 |
| /ripls.api.TransferService/GetTransfer | 23 | 115 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.UserService/GetUserStats | 22 | 110 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.ChatService/GetConversationForTransfer | 12 | 96 | 8 | 8 | 8 | 8 | 8 |
| /ripls.api.GearService/GetGearPeople | 30 | 91 | 3 | 3 | 3 | 3 | 4 |
| /ripls.api.ExperienceService/MarkExperienceInProcess | 11 | 77 | 7 | 7 | 7 | 7 | 7 |
| /ripls.api.AdminService/CleanupSimulation | 1 | 75 | 75 | 75 | 75 | 75 | 75 |
| /ripls.api.ExperienceService/SaveExperience | 14 | 70 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.LoginService/Register | 10 | 64 | 1 | 6.4 | 7 | 7 | 7 |
| /ripls.api.UserService/GetUser | 63 | 63 | 1 | 1 | 1 | 1 | 1 |
| /ripls.api.RequestService/GetRequestPeople | 17 | 60 | 3 | 3.5 | 3 | 4 | 5 |
| /ripls.api.GearService/SaveGear | 52 | 52 | 1 | 1 | 1 | 1 | 1 |
| /ripls.api.LocationService/SaveLocation | 17 | 51 | 3 | 3 | 3 | 3 | 3 |
| /ripls.api.ExperienceService/CancelExperience | 3 | 27 | 9 | 9 | 9 | 9 | 9 |
| /ripls.api.RequestService/WithdrawOffer | 1 | 20 | 20 | 20 | 20 | 20 | 20 |
| /ripls.api.RequestService/CancelRequest | 1 | 17 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.TransferService/CancelTransfer | 1 | 17 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.CommunityService/CreateCommunity | 1 | 11 | 11 | 11 | 11 | 11 | 11 |
| /ripls.api.CommunityService/GetOrCreateInviteLink | 1 | 6 | 6 | 6 | 6 | 6 | 6 |
| /ripls.api.AdminService/ListSimulations | 1 | 4 | 4 | 4 | 4 | 4 | 4 |

## Top 10 Latency Offenders (p95 x calls)

| Rank | RPC Method | Impact | Calls | P95ms |
|-----:|------------|-------:|------:|------:|
| 1 | FeedService/GetFeed | 27652 | 446 | 62 |
| 2 | CommunityService/ListCommunityGear | 26814 | 218 | 123 |
| 3 | MediaService/AddMedia | 11016 | 324 | 34 |
| 4 | FeedService/MarkFeedItemsViewed | 10258 | 446 | 23 |
| 5 | ChatService/ListConversations | 7224 | 258 | 28 |
| 6 | ChatService/GetUnreadCounts | 3354 | 258 | 13 |
| 7 | ImpactService/GetCommunityImpactMetrics | 3078 | 38 | 81 |
| 8 | LoginService/Register | 1870 | 10 | 187 |
| 9 | SearchService/Search | 1800 | 120 | 15 |
| 10 | ExperienceService/RSVPToExperience | 1541 | 67 | 23 |

## Top 10 Query Offenders

| Rank | Endpoint | Total Q | Calls | Avg/Call |
|-----:|----------|--------:|------:|--------:|
| 1 | /ripls.api.CommunityService/ListCommunityGear | 47275 | 218 | 216.9 |
| 2 | /ripls.api.FeedService/GetFeed | 42733 | 446 | 95.8 |
| 3 | /ripls.api.FeedService/MarkFeedItemsViewed | 18286 | 446 | 41 |
| 4 | /ripls.api.ChatService/ListConversations | 7027 | 258 | 27.2 |
| 5 | /ripls.api.ImpactService/GetCommunityImpactMetrics | 5283 | 38 | 139 |
| 6 | /ripls.api.ChatService/GetUnreadCounts | 3083 | 258 | 11.9 |
| 7 | /ripls.api.ExperienceService/RSVPToExperience | 1902 | 67 | 28.4 |
| 8 | /ripls.api.ChatService/SendMessage | 1026 | 186 | 5.5 |
| 9 | /ripls.api.SearchService/Search | 1008 | 120 | 8.4 |
| 10 | /ripls.api.CommunityService/ShareGear | 998 | 54 | 18.5 |
