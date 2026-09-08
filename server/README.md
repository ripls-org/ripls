# Server

Go-based server implementation for the ripls platform. Provides gRPC-compatible HTTP/JSON APIs using Connect-Go.

## Directory Structure

```
server/
├── main.go                    # Server entry point
├── main_integration_test.go   # Integration tests
├── e2e_test.go                # End-to-end tests
├── gen/                       # Generated Go code from proto definitions
│   └── ripls/
│       ├── api/               # Generated API types
│       │   ├── *.pb.go        # Protobuf messages
│       │   └── apiconnect/    # Connect service interfaces
│       └── models/            # Generated model types
│           └── *.pb.go        # Protobuf messages
├── ai/                        # AI provider integrations (Gemini, Vertex AI)
├── auth/                      # Authentication library (JWT, OIDC)
├── email/                     # Email service (Mailgun)
├── location/                  # Location services (Mapbox, EXIF)
├── notifications/             # Push notifications (FCM, noop)
├── services/                  # RPC service implementations
│   ├── community/             # Community management
│   ├── device/                # User device registration
│   ├── gear/                  # Gear (items) management
│   ├── loan/                  # Loan tracking
│   ├── location/              # Location/place management
│   ├── login/                 # Authentication & user registration
│   ├── media/                 # Media upload & storage
│   ├── search/                # Search across gear & communities
│   └── user/                  # User profile management
└── storage/                   # Proto-SQL storage abstraction
```

## Building and Running

### Local Development

Start the server with PostgreSQL:

```bash
# From repository root (requires PostgreSQL running on localhost:5432)
go run ./server

# Custom database connection
go run ./server --db "postgres://user:pass@localhost/dbname?sslmode=disable" --dev-mode

# Local dev server with all features enabled (FCM, AI, media storage)
npm run start:server
```

### Command-Line Flags

- `--db`: PostgreSQL connection string (default: `postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable`)
- `--port`: HTTP port (default: `8080`)
- `--dev-mode`: Enable development mode (dev auth, simulation clock, database reset)
- `--dev-client`: Serve the web development client at `/`
- `--notification-provider`: Notification provider for push notifications (default: `noop`)
  - `noop`: No-op provider for testing (logs notifications but doesn't send)
  - `fcm`: Firebase Cloud Messaging for real push notifications. Uses
    Application Default Credentials. Locally, run `gcloud auth
    application-default login` once or set `GOOGLE_APPLICATION_CREDENTIALS`
    to a key file path outside the repo. In Cloud Run the runtime service
    account is picked up automatically.
- `--firebase-project`: Firebase / GCP project ID for FCM and phone auth.
  Required locally (the SDK cannot infer it under
  `gcloud auth application-default login`). In Cloud Run / GKE, leave empty
  — the SDK reads `GOOGLE_CLOUD_PROJECT` from the environment (set by
  Terraform) or the metadata server.
- `--gemini-api-key`: Gemini API key for AI-powered features (optional)
- `--vertex-ai-project`: GCP project ID for Vertex AI (alternative to gemini-api-key)
- `--vertex-ai-location`: GCP location for Vertex AI (default: `us-central1`)
- `--local-media-storage`: Local filesystem path for media storage (dev mode)
- `--gcs-bucket`: GCS bucket name for media storage (production mode)

## API Usage

The API requires authentication. To enable development authentication endpoints, start the server with the `--dev-mode` flag.

### Development Authentication

**1. Register a new user:**

```bash
curl \
    --header "Content-Type: application/json" \
    --data '{"email": "user@example.com", "name": "Your Name"}' \
    http://localhost:8080/ripls.api.LoginService/Register
```

**2. Login (get a fresh token):**

```bash
curl \
    --header "Content-Type: application/json" \
    --data '{"email": "user@example.com"}' \
    http://localhost:8080/ripls.api.LoginService/Login
```

Both registration and login return an access token:

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 86400,
  "user": {
    "id": "user-id",
    "email": "user@example.com",
    "name": "Your Name",
    "role": "ROLE_USER"
  }
}
```

**3. Use the token for API requests:**

Add gear:

```bash
curl \
    --header "Content-Type: application/json" \
    --header "Authorization: Bearer YOUR_ACCESS_TOKEN" \
    --data '{"gear": {"name": "Power Drill", "description": "18V cordless drill"}}' \
    http://localhost:8080/ripls.api.GearService/AddGear
```

Retrieve gear:

```bash
curl \
    --header "Content-Type: application/json" \
    --header "Authorization: Bearer YOUR_ACCESS_TOKEN" \
    --data '{"id": "your-gear-id"}' \
    http://localhost:8080/ripls.api.GearService/GetGear
```

**Note:** In development mode, authentication is simplified - no password is required for login. In production, authentication is handled by external OAuth providers (Google, Apple, etc.).

## CORS Middleware

The server includes Cross-Origin Resource Sharing (CORS) middleware to enable web browser clients to access the API. This is essential for Flutter web apps and other browser-based clients that run on different origins than the API server.

### Required Headers for ConnectRPC

The CORS configuration includes headers required by the Connect protocol:

- **`Connect-Protocol-Version`**: Protocol version negotiation header used by Connect clients
- **`Connect-Timeout-Ms`**: Optional timeout specification for requests
- **`Content-Type`**: Standard HTTP content type (e.g., `application/json`, `application/connect+proto`)
- **`Authorization`**: Bearer token for authenticated requests

### Production Considerations

**Security Warning:** The current configuration uses `AllowedOrigins([]string{"*"})` which allows requests from any origin. This is convenient for development but **should be restricted in production**.

For production deployments, replace with specific allowed origins:

```go
corsMiddleware := handlers.CORS(
    handlers.AllowedOrigins([]string{
        "https://your-app.com",
        "https://app.your-domain.com",
    }),
    // ... other options
)
```

### Testing CORS

To verify CORS is working correctly:

1. **Start the server** with CORS middleware enabled (already configured in `main.go`)

2. **Make a request from a web browser** (e.g., from Flutter web app running on `http://localhost:xxxxx`)

3. **Check browser DevTools Network tab** for CORS headers in the response:
   ```
   Access-Control-Allow-Origin: *
   Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
   Access-Control-Allow-Headers: Content-Type, Authorization, Connect-Protocol-Version, Connect-Timeout-Ms
   Access-Control-Expose-Headers: Connect-Protocol-Version
   ```

### Common Issues

**Problem:** Browser shows "CORS policy: No 'Access-Control-Allow-Origin' header"

**Solution:** Ensure CORS middleware is properly configured and wrapping your handler. Check that the middleware is applied before `http.ListenAndServe`.

**Problem:** "CORS policy: Request header field X is not allowed"

**Solution:** Add the required header to `AllowedHeaders` in the CORS configuration.

**Problem:** Can't read response headers from JavaScript

**Solution:** Add the header name to `ExposedHeaders` - browsers only expose CORS-safelisted headers by default.

## Testing

Server tests are organized into tiers using Go build tags. Each tier
serves a different purpose and has different requirements.

### Quick reference

| Tier | Build tag | Runs in CI? | Command |
|------|-----------|-------------|---------|
| Unit tests | (none) | Yes | `npm run test:server` |
| Integration (live LLM) | `integration` | Yes | `npm run test:server:integration` |
| AI provider benchmark | `benchmark` | No | See below |
| Time parsing golden | `time_parsing_golden` | No | See below |
| End-to-end | `e2e` | No | `npm run test:server:e2e` |

### Unit tests (no build tag)

Standard tests that use mocks for all external dependencies. These run
fast and are the primary feedback loop during development.

```bash
npm run test:server           # parallel (default)
npm run test:server:serial    # serial (for flaky test debugging)
go test ./server/...          # equivalent to the above
```

**Prerequisite**: Docker must be running. Tests use testcontainers to
automatically start PostgreSQL containers with pgvector support. Each
test creates its own database and cleans up after itself.

### Integration tests (`integration` build tag)

Tests that call live AI APIs (Anthropic, OpenAI, Vertex AI). These
verify that prompts produce valid structured output from real models.
They run in CI with API keys provided via GitHub secrets.

```bash
npm run test:server:integration
go test -tags=integration ./server/...
```

**Environment variables** (tests are skipped when the key is missing):
- `ANTHROPIC_API_KEY` — for Claude Haiku 4.5
- `OPENAI_API_KEY` — for GPT-5-mini
- `VERTEX_AI_PROJECT` — for Gemini Flash

### AI prompt evals (`benchmark` build tag)

Runs hand-crafted golden cases against a live AI provider (Anthropic)
across text-, image-, and webpage-mode generation paths; gates on per-
suite aggregate pass rates. Does **not** run in CI. See
`server/ai/README.md` for the full invocation recipes and the list of
suites.

```bash
# All seven suites (text + image + webpage across Experience/Gear/Request)
go test -tags=benchmark -v -timeout 10m ./server/ai/eval/ \
  -anthropic-api-key="$(gcloud secrets versions access latest --secret=anthropic-api-key --project="$(scripts/gcp_project.sh dev)")"
```

### Time parsing golden (`time_parsing_golden` build tag)

Runs 38 time-parsing scenarios against a live LLM and records the
responses to `server/test_data/time_parsing_responses.json`. This
regenerates the golden data used by the unit tests in
`server/services/experience/time_parsing_test.go`. Does **not** run
in CI — run manually when prompts change.

```bash
# Full suite (~2 min):
ANTHROPIC_API_KEY=sk-... go test -v -tags=time_parsing_golden \
  -run TestTimeParsingIntegration -timeout 10m ./server/ai/

# Single scenario:
ANTHROPIC_API_KEY=sk-... go test -v -tags=time_parsing_golden \
  -run "TestTimeParsingIntegration/text_scenarios/text_dst_spring_forward" \
  -timeout 5m ./server/ai/
```

Single-scenario runs log results but do **not** overwrite the golden
response file (prevents partial runs from clobbering the complete data).

### End-to-end tests (`e2e` build tag)

Connect to a deployed server (dev or prod) and exercise complete user
journeys. Does **not** run in CI.

```bash
# Against dev environment (default):
npm run test:server:e2e
go test -tags=e2e -v -timeout 10m ./server

# Against a custom server:
E2E_SERVER_URL=https://server.example.org go test -tags=e2e -v -timeout 10m ./server
```

## Storage Layer

The server uses a generic Proto-SQL storage abstraction that works with any protobuf message type. See `storage/protosql.go` for implementation details.

### Database

The server uses PostgreSQL exclusively. Features:

- pgvector extension support for semantic search
- Full-text search capabilities
- Production-ready concurrent access

The storage layer automatically:

- Creates tables from proto message definitions
- Handles type conversions between proto and SQL types
- Stores both individual fields (for querying) and serialized proto data (for integrity)

## Production Deployment

The server is deployed using Docker and managed by Terraform. See:

- [`../terraform/README.md`](../terraform/README.md) for infrastructure configuration
- [`../Dockerfile`](../Dockerfile) for container build details
- [`.github/workflows/build_docker_image.yaml`](../.github/workflows/build_docker_image.yaml) for CI/CD pipeline

### Environment Variables

When running in containers (Cloud Run, etc.), the server expects:

- `PORT`: HTTP port (set automatically by Cloud Run)
- `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`: PostgreSQL connection details
- `ENABLE_DEV_MODE`: Set to `"true"` to enable development mode
- `ENABLE_DEV_CLIENT`: Set to `"true"` to serve the web client
- `GOOGLE_CLOUD_PROJECT`: GCP project ID for Vertex AI and Firebase
- `GOOGLE_CLOUD_LOCATION`: GCP location for Vertex AI (e.g., `us-central1`)
- `GCS_MEDIA_BUCKET`: GCS bucket name for media storage
- `MAPBOX_ACCESS_TOKEN`: Mapbox access token for location services

The Dockerfile's CMD constructs the appropriate command-line flags from these environment variables.

**Note on Push Notifications:** In production, the server uses FCM with Application Default Credentials. The Cloud Run service account is granted `roles/firebase.admin` via Terraform to enable FCM access. See [`../docs/push_notifications.md`](../docs/push_notifications.md) for details.
