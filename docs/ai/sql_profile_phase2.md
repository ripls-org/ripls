# SQL Query Baseline

- **Branch:** sql_refactor
- **Commit:** 5563d33c
- **Date:** 2026-02-26T15:23:33Z
- **Scenario:** college-friends (seed=42)
- **Total requests with DB queries:** 1371
- **Total SQL queries:** 18404

## Per-Endpoint Stats

| Endpoint | Calls | Total Q | Min | Avg | P50 | P95 | Max |
|----------|------:|--------:|----:|----:|----:|----:|----:|
| /ripls.api.ExperienceService/RSVPToExperience | 116 | 3441 | 19 | 29.7 | 32 | 33 | 33 |
| /ripls.api.TransferService/CompleteTransfer | 56 | 2501 | 41 | 44.7 | 45 | 49 | 50 |
| /ripls.api.ChatService/SendMessage | 360 | 1936 | 5 | 5.4 | 5 | 6 | 10 |
| /ripls.api.TransferService/StartLoan | 49 | 1788 | 34 | 36.5 | 36 | 39 | 42 |
| /ripls.api.TransferService/ExpressInterest | 65 | 1687 | 25 | 26 | 26 | 26 | 26 |
| /ripls.api.ExperienceService/CompleteExperience | 36 | 1396 | 33 | 38.8 | 38 | 45 | 50 |
| /ripls.api.RequestService/OfferToFulfill | 38 | 1172 | 30 | 30.8 | 31 | 31 | 31 |
| /ripls.api.CommunityService/ShareGear | 60 | 967 | 4 | 16.1 | 19 | 19 | 19 |
| /ripls.api.RequestService/MarkRequestFulfilled | 18 | 876 | 45 | 48.7 | 48 | 55 | 56 |
| /ripls.api.MediaService/AddMedia | 324 | 648 | 2 | 2 | 2 | 2 | 2 |
| /ripls.api.ExperienceService/ShareExperience | 36 | 612 | 17 | 17 | 17 | 17 | 17 |
| /ripls.api.RequestService/SubmitRequest | 23 | 368 | 16 | 16 | 16 | 16 | 16 |
| /ripls.api.ExperienceService/MarkExperienceInProcess | 36 | 252 | 7 | 7 | 7 | 7 | 7 |
| /ripls.api.ExperienceService/SaveExperience | 36 | 180 | 5 | 5 | 5 | 5 | 5 |
| /ripls.api.TransferService/SelectRecipient | 7 | 112 | 16 | 16 | 16 | 16 | 16 |
| /ripls.api.LocationService/SaveLocation | 36 | 108 | 3 | 3 | 3 | 3 | 3 |
| /ripls.api.AdminService/CleanupSimulation | 1 | 88 | 88 | 88 | 88 | 88 | 88 |
| /ripls.api.TransferService/CancelTransfer | 4 | 72 | 18 | 18 | 18 | 18 | 18 |
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
| 1 | /ripls.api.ExperienceService/RSVPToExperience | 3441 | 116 | 29.7 |
| 2 | /ripls.api.TransferService/CompleteTransfer | 2501 | 56 | 44.7 |
| 3 | /ripls.api.ChatService/SendMessage | 1936 | 360 | 5.4 |
| 4 | /ripls.api.TransferService/StartLoan | 1788 | 49 | 36.5 |
| 5 | /ripls.api.TransferService/ExpressInterest | 1687 | 65 | 26 |
| 6 | /ripls.api.ExperienceService/CompleteExperience | 1396 | 36 | 38.8 |
| 7 | /ripls.api.RequestService/OfferToFulfill | 1172 | 38 | 30.8 |
| 8 | /ripls.api.CommunityService/ShareGear | 967 | 60 | 16.1 |
| 9 | /ripls.api.RequestService/MarkRequestFulfilled | 876 | 18 | 48.7 |
| 10 | /ripls.api.MediaService/AddMedia | 648 | 324 | 2 |
