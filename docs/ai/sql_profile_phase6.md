# SQL Query Baseline

- **Branch:** sql_refactor
- **Commit:** 31e90cd9
- **Date:** 2026-02-26T17:53:07Z
- **Scenario:** college-friends (seed=42)
- **Total requests with DB queries:** 1371
- **Total SQL queries:** 17517

## Per-Endpoint Stats

| Endpoint | Calls | Total Q | Min | Avg | P50 | P95 | Max |
|----------|------:|--------:|----:|----:|----:|----:|----:|
| /ripls.api.ExperienceService/RSVPToExperience | 116 | 3350 | 19 | 28.9 | 31 | 32 | 32 |
| /ripls.api.TransferService/CompleteTransfer | 56 | 2228 | 36 | 39.8 | 40 | 44 | 46 |
| /ripls.api.ChatService/SendMessage | 360 | 1936 | 5 | 5.4 | 5 | 6 | 10 |
| /ripls.api.TransferService/StartLoan | 49 | 1641 | 31 | 33.5 | 33 | 36 | 39 |
| /ripls.api.TransferService/ExpressInterest | 65 | 1557 | 23 | 24 | 24 | 24 | 24 |
| /ripls.api.ExperienceService/CompleteExperience | 36 | 1183 | 29 | 32.9 | 32 | 39 | 39 |
| /ripls.api.RequestService/OfferToFulfill | 38 | 1172 | 30 | 30.8 | 31 | 31 | 31 |
| /ripls.api.CommunityService/ShareGear | 60 | 967 | 4 | 16.1 | 19 | 19 | 19 |
| /ripls.api.RequestService/MarkRequestFulfilled | 18 | 847 | 44 | 47.1 | 47 | 53 | 54 |
| /ripls.api.MediaService/AddMedia | 324 | 648 | 2 | 2 | 2 | 2 | 2 |
| /ripls.api.ExperienceService/ShareExperience | 36 | 612 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.RequestService/SubmitRequest | 23 | 368 | 16 | 16 | 16 | 16 | 16 |
| /ripls.api.ExperienceService/MarkExperienceInProcess | 36 | 252 | 7 | 7 | 7 | 7 | 7 |
| /ripls.api.ExperienceService/SaveExperience | 36 | 180 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.TransferService/SelectRecipient | 7 | 112 | 16 | 16 | 16 | 16 | 16 |
| /ripls.api.LocationService/SaveLocation | 36 | 108 | 3 | 3 | 3 | 3 | 3 |
| /ripls.api.AdminService/CleanupSimulation | 1 | 88 | 88 | 88 | 88 | 88 | 88 |
| /ripls.api.TransferService/CancelTransfer | 4 | 68 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.RequestService/CancelRequest | 3 | 51 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.GearService/SaveGear | 48 | 48 | 1 | 1 | 1 | 1 | 1 |
| /ripls.api.LoginService/Register | 7 | 43 | 1 | 6.1 | 7 | 7 | 7 |
| /ripls.api.RequestService/WithdrawOffer | 1 | 20 | 20 | 20 | 20 | 20 | 20 |
| /ripls.api.UserService/SaveUser | 7 | 14 | 2 | 2 | 2 | 2 | 2 |
| /ripls.api.CommunityService/CreateCommunity | 1 | 11 | 11 | 11 | 11 | 11 | 11 |
| /ripls.api.CommunityService/GetOrCreateInviteLink | 1 | 6 | 6 | 6 | 6 | 6 | 6 |
| /ripls.api.AdminService/ListSimulations | 1 | 4 | 4 | 4 | 4 | 4 | 4 |
| /ripls.api.CommunityService/UpdateCommunity | 1 | 3 | 3 | 3 | 3 | 3 | 3 |

## Top 10 Offenders

| Rank | Endpoint | Total Q | Calls | Avg/Call |
|-----:|----------|--------:|------:|--------:|
| 1 | /ripls.api.ExperienceService/RSVPToExperience | 3350 | 116 | 28.9 |
| 2 | /ripls.api.TransferService/CompleteTransfer | 2228 | 56 | 39.8 |
| 3 | /ripls.api.ChatService/SendMessage | 1936 | 360 | 5.4 |
| 4 | /ripls.api.TransferService/StartLoan | 1641 | 49 | 33.5 |
| 5 | /ripls.api.TransferService/ExpressInterest | 1557 | 65 | 24 |
| 6 | /ripls.api.ExperienceService/CompleteExperience | 1183 | 36 | 32.9 |
| 7 | /ripls.api.RequestService/OfferToFulfill | 1172 | 38 | 30.8 |
| 8 | /ripls.api.CommunityService/ShareGear | 967 | 60 | 16.1 |
| 9 | /ripls.api.RequestService/MarkRequestFulfilled | 847 | 18 | 47.1 |
| 10 | /ripls.api.MediaService/AddMedia | 648 | 324 | 2 |
