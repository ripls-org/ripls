# Services

The `services` directory contains all RPC service implementations. Each sub-package implements one Connect service interface generated from the `proto/ripls/api/` definitions.

## Sub-packages

| Package | Connect service | Primary responsibility |
|---|---|---|
| `admin` | AdminService | Dev-mode operations (simulation cleanup, database reset) |
| `chat` | ChatService | Messaging, reactions, streaming, conversation summaries |
| `community` | CommunityService | Community lifecycle, membership, invitations, streaming |
| `device` | DeviceService | Push-notification device token registration |
| `experience` | ExperienceService | Community experiences — creation, AI generation, attendance |
| `feed` | FeedService | Activity feed, stories, nudges |
| `feedback` | FeedbackService | In-app feedback filed as issues in the project tracker |
| `gear` | GearService | Gear listing, AI metadata generation, sharing settings |
| `health` | HealthService | Dependency health checks and version reporting |
| `impact_metrics` | ImpactMetricsService | Community and user impact metrics, top contributors |
| `location` | LocationService | Place search, geocoding, EXIF location |
| `login` | LoginService | Authentication, registration, password reset, OIDC |
| `media` | MediaService | User media upload and presigned URL generation |
| `portfolio` | PortfolioService | Personal impact summary, goals, inbox |
| `request` | RequestService | Borrowing/help requests — creation, AI generation, fulfillment |
| `search` | SearchService | Cross-entity keyword search |
| `streaming` | (shared) | Generic serializing sender for server-streaming RPCs |
| `transfer` | TransferService | Loan and giveaway lifecycle (start, complete, cancel) |
| `user` | UserService | User profile reads and updates |
| `waitlist` | WaitlistService | Pre-launch waitlist signup |
| `web` | (HTTP) | Server-side HTML rendering for invitation preview pages |

## Shared helpers

- `roundtrip_helpers.go` — `AssertFieldRoundTrip`: test helper that exercises a save+fetch cycle to verify API field contracts.
- `test_util.go` — shared test infrastructure (storage setup, auth token factories).
- `user_util.go` — common user-fetching helpers used across multiple services.
