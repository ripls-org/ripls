# Performance Profile

- **Branch:** load_and_profile
- **Commit:** 710c0388
- **Date:** 2026-02-27T19:43:22Z
- **Scenario:** load-test-small (seed=42, concurrency=0)
- **Read traffic:** enabled (multiplier: 1.0)
- **Total HTTP requests:** 3373
- **Requests with DB:** 3371
- **Total SQL queries:** 43643
- **Total DB time:** 22710ms
- **Errors:** 2
- **Wall-clock duration:** 36s

## Per-Endpoint Latency

| RPC Method | Calls | P50ms | P95ms | P99ms | Max ms | Errors |
|------------|------:|------:|------:|------:|-------:|-------:|
| FeedService/GetFeed | 446 | 9 | 12 | 13 | 18 | 0 |
| FeedService/MarkFeedItemsViewed | 446 | 11 | 13 | 15 | 19 | 0 |
| MediaService/AddMedia | 324 | 26 | 36 | 39 | 43 | 0 |
| ChatService/GetUnreadCounts | 258 | 5 | 13 | 15 | 17 | 0 |
| ChatService/ListConversations | 258 | 13 | 28 | 32 | 34 | 0 |
| CommunityService/ListCommunityGear | 218 | 4 | 6 | 7 | 7 | 0 |
| ChatService/SendMessage | 186 | 2 | 5 | 6 | 8 | 0 |
| GearService/GetGear | 137 | 3 | 3 | 4 | 4 | 0 |
| FeedService/ListStories | 131 | 1 | 2 | 2 | 2 | 0 |
| SearchService/Search | 120 | 13 | 15 | 15 | 16 | 0 |
| ChatService/GetConversationHistory | 101 | 4 | 9 | 10 | 11 | 0 |
| ExperienceService/GetExperience | 67 | 4 | 6 | 6 | 8 | 0 |
| ExperienceService/GetExperiencePeople | 67 | 2 | 4 | 4 | 4 | 0 |
| ExperienceService/RSVPToExperience | 67 | 17 | 22 | 24 | 27 | 0 |
| UserService/GetUser | 63 | 0 | 0 | 2 | 2 | 0 |
| CommunityService/ShareGear | 54 | 12 | 15 | 16 | 18 | 0 |
| GearService/SaveGear | 52 | 0 | 0 | 2 | 2 | 0 |
| ImpactService/GetCommunityImpactMetrics | 38 | 10 | 11 | 12 | 12 | 0 |
| GearService/GetGearPeople | 30 | 1 | 2 | 2 | 2 | 0 |
| RequestService/GetRequest | 27 | 4 | 6 | 6 | 6 | 0 |
| RequestService/OfferToFulfill | 26 | 21 | 22 | 22 | 25 | 0 |
| TransferService/GetTransfer | 23 | 2 | 3 | 3 | 3 | 0 |
| UserService/GetUserStats | 22 | 2 | 3 | 3 | 4 | 0 |
| TransferService/ExpressInterest | 20 | 11 | 14 | 14 | 15 | 2 |
| LocationService/SaveLocation | 17 | 1 | 2 | 2 | 5 | 0 |
| RequestService/GetRequestPeople | 17 | 1 | 2 | 2 | 3 | 0 |
| RequestService/SubmitRequest | 17 | 14 | 16 | 16 | 18 | 0 |
| TransferService/CompleteTransfer | 17 | 20 | 22 | 22 | 25 | 0 |
| TransferService/StartLoan | 17 | 17 | 20 | 20 | 23 | 0 |
| ExperienceService/SaveExperience | 14 | 3 | 4 | 4 | 6 | 0 |
| ExperienceService/ShareExperience | 14 | 11 | 11 | 11 | 13 | 0 |
| RequestService/MarkRequestFulfilled | 14 | 30 | 34 | 34 | 37 | 0 |
| ChatService/GetConversationForTransfer | 12 | 4 | 4 | 4 | 6 | 0 |
| ExperienceService/CompleteExperience | 11 | 19 | 21 | 21 | 21 | 0 |
| ExperienceService/MarkExperienceInProcess | 11 | 6 | 7 | 7 | 8 | 0 |
| LoginService/Register | 10 | 190 | 200 | 200 | 202 | 0 |
| ImpactService/GetUserImpactMetrics | 9 | 6 | 14 | 14 | 16 | 0 |
| ExperienceService/CancelExperience | 3 | 7 | 7 | 7 | 9 | 0 |
| AdminService/CleanupSimulation | 2 | 68 | 3 | 3 | 68 | 0 |
| AdminService/ListSimulations | 1 | 5 | 5 | 5 | 5 | 0 |
| CommunityService/CreateCommunity | 1 | 11 | 11 | 11 | 11 | 0 |
| CommunityService/GetOrCreateInviteLink | 1 | 3 | 3 | 3 | 3 | 0 |
| RequestService/CancelRequest | 1 | 10 | 10 | 10 | 10 | 0 |
| RequestService/WithdrawOffer | 1 | 12 | 12 | 12 | 12 | 0 |
| TransferService/CancelTransfer | 1 | 7 | 7 | 7 | 7 | 0 |

## Per-Endpoint DB Queries

| Endpoint | Calls | Total Q | Min | Avg | P50 | P95 | Max |
|----------|------:|--------:|----:|----:|----:|----:|----:|
| /ripls.api.FeedService/MarkFeedItemsViewed | 446 | 9433 | 3 | 21.2 | 22 | 22 | 22 |
| /ripls.api.FeedService/GetFeed | 446 | 7531 | 10 | 16.9 | 17 | 17 | 17 |
| /ripls.api.ChatService/ListConversations | 258 | 7189 | 1 | 27.9 | 28 | 57 | 69 |
| /ripls.api.ChatService/GetUnreadCounts | 258 | 3083 | 1 | 11.9 | 11 | 25 | 31 |
| /ripls.api.ExperienceService/RSVPToExperience | 67 | 1902 | 19 | 28.4 | 31 | 32 | 32 |
| /ripls.api.CommunityService/ListCommunityGear | 218 | 1718 | 7 | 7.9 | 8 | 8 | 8 |
| /ripls.api.ChatService/SendMessage | 186 | 1026 | 5 | 5.5 | 5 | 6 | 10 |
| /ripls.api.SearchService/Search | 120 | 1010 | 8 | 8.4 | 8 | 10 | 10 |
| /ripls.api.CommunityService/ShareGear | 54 | 998 | 5 | 18.5 | 19 | 19 | 19 |
| /ripls.api.ChatService/GetConversationHistory | 101 | 920 | 3 | 9.1 | 9 | 15 | 20 |
| /ripls.api.GearService/GetGear | 137 | 822 | 6 | 6 | 6 | 6 | 6 |
| /ripls.api.RequestService/OfferToFulfill | 26 | 804 | 30 | 30.9 | 31 | 31 | 31 |
| /ripls.api.ImpactService/GetCommunityImpactMetrics | 38 | 679 | 16 | 17.9 | 18 | 18 | 18 |
| /ripls.api.MediaService/AddMedia | 324 | 648 | 2 | 2 | 2 | 2 | 2 |
| /ripls.api.TransferService/CompleteTransfer | 17 | 634 | 36 | 37.3 | 38 | 39 | 39 |
| /ripls.api.RequestService/MarkRequestFulfilled | 14 | 623 | 43 | 44.5 | 44 | 46 | 46 |
| /ripls.api.TransferService/StartLoan | 17 | 528 | 31 | 31.1 | 31 | 31 | 32 |
| /ripls.api.ExperienceService/GetExperience | 67 | 524 | 7 | 7.8 | 8 | 8 | 8 |
| /ripls.api.TransferService/ExpressInterest | 20 | 434 | 1 | 21.7 | 24 | 24 | 24 |
| /ripls.api.FeedService/ListStories | 131 | 384 | 2 | 2.9 | 3 | 3 | 3 |
| /ripls.api.ExperienceService/GetExperiencePeople | 67 | 374 | 3 | 5.6 | 6 | 8 | 10 |
| /ripls.api.ExperienceService/CompleteExperience | 11 | 363 | 29 | 33 | 33 | 35 | 35 |
| /ripls.api.RequestService/SubmitRequest | 17 | 357 | 21 | 21 | 21 | 21 | 21 |
| /ripls.api.RequestService/GetRequest | 27 | 250 | 8 | 9.3 | 10 | 10 | 10 |
| /ripls.api.ExperienceService/ShareExperience | 14 | 238 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.ImpactService/GetUserImpactMetrics | 9 | 142 | 10 | 15.8 | 13 | 20 | 30 |
| /ripls.api.TransferService/GetTransfer | 23 | 115 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.UserService/GetUserStats | 22 | 110 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.ChatService/GetConversationForTransfer | 12 | 96 | 8 | 8 | 8 | 8 | 8 |
| /ripls.api.GearService/GetGearPeople | 30 | 94 | 3 | 3.1 | 3 | 4 | 4 |
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
| 1 | MediaService/AddMedia | 11664 | 324 | 36 |
| 2 | ChatService/ListConversations | 7224 | 258 | 28 |
| 3 | FeedService/MarkFeedItemsViewed | 5798 | 446 | 13 |
| 4 | FeedService/GetFeed | 5352 | 446 | 12 |
| 5 | ChatService/GetUnreadCounts | 3354 | 258 | 13 |
| 6 | LoginService/Register | 2000 | 10 | 200 |
| 7 | SearchService/Search | 1800 | 120 | 15 |
| 8 | ExperienceService/RSVPToExperience | 1474 | 67 | 22 |
| 9 | CommunityService/ListCommunityGear | 1308 | 218 | 6 |
| 10 | ChatService/SendMessage | 930 | 186 | 5 |

## Top 10 Query Offenders

| Rank | Endpoint | Total Q | Calls | Avg/Call |
|-----:|----------|--------:|------:|--------:|
| 1 | /ripls.api.FeedService/MarkFeedItemsViewed | 9433 | 446 | 21.2 |
| 2 | /ripls.api.FeedService/GetFeed | 7531 | 446 | 16.9 |
| 3 | /ripls.api.ChatService/ListConversations | 7189 | 258 | 27.9 |
| 4 | /ripls.api.ChatService/GetUnreadCounts | 3083 | 258 | 11.9 |
| 5 | /ripls.api.ExperienceService/RSVPToExperience | 1902 | 67 | 28.4 |
| 6 | /ripls.api.CommunityService/ListCommunityGear | 1718 | 218 | 7.9 |
| 7 | /ripls.api.ChatService/SendMessage | 1026 | 186 | 5.5 |
| 8 | /ripls.api.SearchService/Search | 1010 | 120 | 8.4 |
| 9 | /ripls.api.CommunityService/ShareGear | 998 | 54 | 18.5 |
| 10 | /ripls.api.ChatService/GetConversationHistory | 920 | 101 | 9.1 |
