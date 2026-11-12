# Invitation System Improvements Plan

This document outlines the implementation plan for improving the Ripls invitation system, including documentation, shorter URLs, server-side rendering for link previews, and Open Graph metadata.

## Current State Summary

### Invitation Flow Overview
1. **Link Generation**: Members call `GetOrCreateInviteLink` RPC, which creates/reuses a `CommunityInvitationLink` with a UUID token
2. **Current URL Format**: `https://ripls.app/invite?token=<UUID>&community=<name>&inviter=<name>&gear_id=<optional>`
3. **Static Landing Page**: `website/content/invite.html` - client-side JavaScript attempts to open the app
4. **App Handling**: Routes to `InvitationLandingScreen` (unauthenticated) or auto-accepts (authenticated)
5. **Validation**: `CheckInvitation` RPC validates tokens without authentication

### Key Files
| Purpose | File |
|---------|------|
| Link management RPCs | `server/services/community/invitations.go` |
| Token validation RPC | `server/services/login/registration.go` |
| Static landing page | `website/content/invite.html` |
| App landing screen | `app/lib/presentation/screens/auth/invitation_landing_screen.dart` |
| Deep link routing | `app/lib/core/router/app_router.dart` |
| Proto definitions | `proto/ripls/api/community_service.proto`, `proto/ripls/models/community.proto` |

### Current Architecture (No Proxying)
- **ripls.app**: Static HTML served by nginx (invite.html, reset-password.html)
- **api.ripls.app**: Go server handles all RPC calls
- No server-side validation happens before rendering the static pages
- Token validation only occurs when app calls `CheckInvitation` RPC

---

## Planned Improvements

### Target URL Format
```
Production:  https://ripls.app/go/Xk9mN2pQ
Development: https://dev.ripls.app/go/Xk9mN2pQ

With optional parameters:
https://ripls.app/go/Xk9mN2pQ?gear_id=abc123
```

The `/go/` prefix is intentionally generic—it's a short-link system that currently
handles invitations but could be extended to other use cases in the future.

---

### 1. Create Documentation (docs/invitations.md)

Create a concise (~1000 word) documentation file explaining:
- How invitation links are generated
- The link validation flow
- Registration and acceptance flows
- Token management (reuse, revocation)
- Deep linking parameters (gear_id, request_id, experience_id)
- Capacity limits

**Effort**: Small

---

### 2. Shorten Invitation URLs

**Goal**: Transform URLs from:
```
https://ripls.app/invite?token=00000000-0000-4000-8000-000000000001&community=Neighborhood%20Share&inviter=John%20Doe
```
To:
```
https://ripls.app/go/Xk9mN2pQ
```

#### Implementation Steps

**a) Add short_code field to CommunityInvitationLink model**

File: `proto/ripls/models/community.proto`

```protobuf
message CommunityInvitationLink {
  string id = 1;
  string community_id = 2;
  string inviter_id = 3;
  string invitation_token = 4;  // Keep for backwards compatibility
  bool is_revoked = 5;
  int64 created_at_unix_sec = 6;
  string short_code = 7;  // 8-char alphanumeric code for /go/ URLs
}
```

**b) Generate short codes on link creation**

File: `server/services/community/invitations.go`

```go
// Charset excludes confusable characters: 0/O, 1/l/I
const shortCodeCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"

func generateShortCode() string {
    code := make([]byte, 8)
    for i := range code {
        code[i] = shortCodeCharset[rand.Intn(len(shortCodeCharset))]
    }
    return string(code)
}
```

**c) Support lookup by short_code**

Update `CheckInvitation` and `AcceptInvitationLink` to:
1. Try lookup by `short_code` first
2. Fall back to `invitation_token` (UUID) for backwards compatibility

**d) Update URL generation**

```go
// Before
inviteURL := fmt.Sprintf("https://ripls.app/invite?token=%s&community=%s&inviter=%s", ...)

// After
baseURL := getBaseURL(ctx)  // ripls.app or dev.ripls.app based on environment
inviteURL := fmt.Sprintf("https://%s/go/%s", baseURL, shortCode)

// With optional deep-link parameters
if gearID != "" {
    inviteURL = fmt.Sprintf("%s?gear_id=%s", inviteURL, gearID)
}
```

**e) Migration for existing tokens**

Generate short codes for existing `CommunityInvitationLink` records where `short_code = ""`.

**f) Update app routing**

File: `app/lib/core/router/app_router.dart`

```dart
// New short-link route
GoRoute(
  path: '/go/:code',
  builder: (context, state) {
    final code = state.pathParameters['code']!;
    final gearId = state.uri.queryParameters['gear_id'];
    // ... handle invitation (same as /invite but with short code)
  },
),
// Keep old routes for backwards compatibility
GoRoute(
  path: '/invite/:code',
  redirect: (context, state) => '/go/${state.pathParameters['code']}',
),
GoRoute(
  path: '/invite',
  redirect: (context, state) {
    final token = state.uri.queryParameters['token'];
    if (token != null) return '/go/$token';
    return '/';
  },
),
```

---

### 3. Server-Side Rendering for Invitation Pages

#### Architecture Options: Proxying vs. Direct Serving

**Option A: Go Server Serves ripls.app Directly**
```
User → ripls.app → Go Server → HTML with OG metadata
```
- Pros:
  - Single process handles everything
  - Simpler DNS/routing configuration
  - All dynamic capabilities available
  - No inter-process communication
- Cons:
  - Go server must serve static assets (CSS, images)
  - Need to embed templates/assets in binary or serve from filesystem

**Option B: Nginx Proxies Dynamic Routes to Go Server**
```
User → ripls.app → Nginx
                    ├── /go/* → proxy to Go Server
                    ├── /reset-password → proxy to Go Server
                    └── static files → serve directly
```
- Pros:
  - nginx excels at static file serving with caching
  - Clear separation between static and dynamic
  - Can use nginx for SSL termination, rate limiting
- Cons:
  - Two processes to manage
  - nginx configuration complexity
  - Latency from proxy hop (minimal)

**Option C: Go Server Proxies to Nginx for Static Assets**
```
User → ripls.app → Go Server
                    ├── /go/* → render HTML
                    └── /css/*, /assets/* → proxy to nginx or serve embedded
```
- Pros:
  - Go handles all routing logic
  - Can embed critical assets, proxy others
- Cons:
  - More complex Go code
  - Potential for inconsistency

**Recommendation**: Option A (Go serves everything) is simplest for ripls.app since it only has a few pages. We can embed the CSS/templates directly in the Go binary or serve from a mounted volume in production.

#### Sharing CSS/Templates

To keep styling consistent between static (ripls.org) and dynamic (ripls.app) pages:

**Approach: Shared Source, Copied at Build**

```
website/
├── content/           # Static pages for ripls.org
│   ├── css/
│   │   ├── shared.css
│   │   └── deeplink.css
│   └── assets/
└── templates/         # Shared templates for Go server
    ├── _base.html     # Common header/footer
    ├── invite.html
    └── reset-password.html

server/
└── web/
    ├── templates/     # Copied or symlinked from website/templates/
    └── static/        # Copied or symlinked from website/content/css/, assets/
```

Build process copies shared assets to both locations, or use a shared volume in Docker.

#### Implementation Steps

**a) Create web service package**

File: `server/web/service.go`

```go
type Service struct {
    storage    storage.Storage
    templates  *template.Template
    baseURL    string  // "ripls.app" or "dev.ripls.app"
}

func (s *Service) HandleGoPage(w http.ResponseWriter, r *http.Request) {
    // Extract short code from path: /go/{code}
    code := strings.TrimPrefix(r.URL.Path, "/go/")

    // Validate invitation
    invitation, community, inviter, err := s.validateInvitation(r.Context(), code)

    // Render template with OG metadata
    s.renderInvitePage(w, TemplateData{
        CommunityName:     community.Name,
        CommunityImageURL: s.getCommunityImageURL(community),
        InviterName:       inviter.Name,
        ShortCode:         code,
        IsValid:           err == nil,
        ErrorMessage:      getErrorMessage(err),
    })
}
```

**b) Create HTML template with Open Graph tags**

File: `website/templates/invite.html`

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Join {{.CommunityName}} on Ripls</title>

    <!-- Open Graph metadata for link previews -->
    <meta property="og:title" content="Join {{.CommunityName}} on Ripls">
    <meta property="og:description" content="{{.InviterName}} invited you to join {{.CommunityName}} — share and borrow with your community!">
    <meta property="og:image" content="{{.CommunityImageURL}}">
    <meta property="og:url" content="https://{{.BaseURL}}/go/{{.ShortCode}}">
    <meta property="og:type" content="website">
    <meta property="og:site_name" content="Ripls">

    <!-- Twitter Card -->
    <meta name="twitter:card" content="summary_large_image">
    <meta name="twitter:title" content="Join {{.CommunityName}} on Ripls">
    <meta name="twitter:description" content="{{.InviterName}} invited you to join {{.CommunityName}}">
    <meta name="twitter:image" content="{{.CommunityImageURL}}">

    <!-- Existing shared CSS -->
    <link rel="stylesheet" href="/css/shared.css">
    <link rel="stylesheet" href="/css/deeplink.css">
</head>
<body>
    {{template "_header.html"}}

    <main class="deeplink-container">
        <div class="deeplink-card">
            {{if .IsValid}}
            <h1>You've Been Invited!</h1>
            <p>
                <strong>{{.InviterName}}</strong> invited you to join
                <strong>{{.CommunityName}}</strong> on Ripls.
            </p>
            {{else}}
            <h1>Invalid Invitation</h1>
            <p>{{.ErrorMessage}}</p>
            {{end}}

            <!-- Rest of page content -->
        </div>
    </main>

    {{template "_footer.html"}}

    <script>
        // Deep link handling (similar to current invite.html)
        const shortCode = "{{.ShortCode}}";
        // ...
    </script>
</body>
</html>
```

**c) Register routes in main.go**

```go
// Web pages for ripls.app
webService := web.NewService(sqlStorage, cfg.baseURL)
mux.HandleFunc("/go/", webService.HandleGoPage)           // Short links (invitations, etc.)
mux.HandleFunc("/invite", webService.HandleLegacyInvite)  // Backwards compat redirect
mux.HandleFunc("/reset-password", webService.HandleResetPasswordPage)

// Static assets
mux.Handle("/css/", http.FileServer(http.FS(webAssets)))
mux.Handle("/assets/", http.FileServer(http.FS(webAssets)))
```

**d) Environment-based base URL**

```go
// Config flag
baseURL := flag.String("base-url", "ripls.app", "Base URL for generated links")

// Usage
if cfg.devMode {
    baseURL = "dev.ripls.app"
}
```

---

### 4. Community Photo in Open Graph Metadata

**Goal**: Display the community's photo in link preview cards.

#### Implementation

**a) Extend CheckInvitationResponse**

File: `proto/ripls/api/login_service.proto`

```protobuf
message CheckInvitationResponse {
  // ... existing fields ...
  string community_image_url = 8;  // Presigned URL for community photo
}
```

**b) Generate presigned URL in validation**

```go
func (s *Service) getCommunityImageURL(ctx context.Context, community *models.Community) string {
    if community.MediaId == "" {
        return s.defaultOGImageURL  // Fallback to Ripls logo
    }

    // Generate presigned URL for community photo
    url, err := s.mediaService.GetPresignedURL(ctx, community.MediaId)
    if err != nil {
        return s.defaultOGImageURL
    }
    return url
}
```

**c) Create default OG image**

File: `website/content/assets/og-default.png`

Create a 1200x630px image (recommended OG dimensions) with Ripls branding.

**d) Image sizing consideration**

For optimal previews, we may want to generate OG-specific image sizes. Options:
1. Use original community photo (may be cropped oddly by platforms)
2. Generate 1200x630 version on upload (adds complexity)
3. Use a template overlay with community photo inset

Start with option 1 and iterate based on how previews look in practice.

---

## Environment Configuration

### Hostnames

| Environment | Base URL | Purpose |
|-------------|----------|---------|
| Production | `ripls.app` | Live invitation links |
| Development | `dev.ripls.app` | Testing invitation flow |
| Local | `localhost:8080` | Local development |

### DNS Configuration

```
ripls.app        → Go server (production)
dev.ripls.app    → Go server (development/staging)
api.ripls.app    → Go server (RPC endpoints) [existing]
ripls.org        → Static website [existing, unchanged]
```

---

## Clean Transition (No UUID Backward Compatibility)

**Decision**: Clean break — no backward compatibility for UUID `invitation_token`.

**Status: COMPLETE.** All `invitation_token` references have been removed from protos,
server code, client code, and tests. The `GetOrCreateInviteLinkResponse.short_code`
intentionally takes over field number 1 (wire-breaking change). The only remaining
`invitation_token` string is in the `reserved` declaration in `CommunityInvitationLink`
model proto (field 4).

---

## Implementation Progress

### DONE

1. **Documentation** - Created `docs/invitations.md` (~1000 words)
2. **Proto changes** - short_code fields added, then full invitation_token removal (see #6)
3. **Server: short code generation**:
   - `generateShortCode()` - crypto/rand, 8-char, no confusable chars
   - `generateUniqueShortCode()` - checks DB uniqueness, retries up to 10x
   - `lookupInvitation()` - looks up by `short_code` only
   - `GetOrCreateInviteLink` - generates short codes, new `/go/` URL format
   - `AcceptInvitationLink` - uses `lookupInvitation` (short_code only)
   - `CheckInvitation` (login service) - uses short_code lookup
   - `validateShortCode` (login service) - renamed from `validateInvitationToken`
   - `inviteLinkHostname` field on Service struct, required flag in main.go
4. **Server tests: UUID → short_code sweep** - All test files updated
5. **Unit tests for short code generation** (`community/short_code_test.go`)
6. **Full invitation_token removal** (clean break, no backward compat):
   - **Protos**: Removed `invitation_token` from all proto files:
     - `models/community.proto`: Removed field 4 from CommunityInvitationLink (reserved 4), renamed `invitation_token` to `invitation_short_code` in CommunityEvent oneof
     - `api/community_service.proto`: Removed field 1 from GetOrCreateInviteLinkResponse (short_code moved to field 1, intentional wire break), renamed `invitation_token` to `short_code` in AcceptInvitationLinkRequest
     - `api/login_service.proto`: Renamed `invitation_token` to `short_code` in RegisterRequest, OIDCRegisterRequest, CheckInvitationRequest
   - **Server code**:
     - `invitations.go`: Removed `generateUUID()`, backfill logic, `uuid` import; updated AcceptInvitationLink to use `ShortCode`/`InvitationShortCode`
     - `registration.go`: Renamed `validateInvitationToken` → `validateShortCode`; updated `recordCommunityEvent` to use `InvitationShortCode`; updated all `InvitationToken` refs to `ShortCode`
     - `patrol-test-server/main.go`: Updated RegisterRequest field
   - **Server tests**: All `InvitationToken` → `ShortCode` in struct literals, error message assertions updated, backfill test removed
   - **Client (Dart)**:
     - `auth_service.dart`: `invitationToken` param → `shortCode` in register, oidcRegister, checkInvitation
     - `community_service.dart`: `invitationToken` param → `shortCode` in acceptInvitationLink
     - `auth_repository.dart`, `community_repository.dart`: param renames
     - `register_view_model.dart`: `checkInvitationToken` → `checkShortCode`
     - `register_screen.dart`: `invitationToken` property → `shortCode`
     - `invitation_landing_screen.dart`: `invitationToken` property → `shortCode`
     - `app_router.dart`: Updated all constructor calls
   - **Client tests**: Updated `community_repository_test.dart`, `register_view_model_test.dart`
7. **App routing** - `/go/:code` route added:
   - `app_router.dart`: Added `/go/:code` route that redirects to `/invite?token=...`
   - `/go/` paths included in `isGoingToAuthScreen` check
8. **Website**:
   - `invite.html`: Extracts code from `/go/CODE` path or legacy `?token=` param
   - `default.conf`: Added nginx `/go/` location serving invite.html
   - `apple-app-site-association`: Added `/go/*` path for iOS deep linking

9. **Web service** - Template rendering with OG metadata:
   - `website/embed.go`: Shared asset embedding via `//go:embed` — single source of truth for CSS/assets used by both nginx (static site) and Go server (dynamic pages)
   - `server/services/web/service.go`: Service struct with `HandleInvitePage` handler, `ServeStaticAssets` for CSS/logo, `getCommunityImageURL` (presigned URL with 1hr expiry, fallback to logo)
   - `server/services/web/templates/invite.html`: Go HTML template with OG meta tags (`og:title`, `og:description`, `og:image`, `og:url`), Twitter Card tags, deep link JavaScript, store buttons
   - `server/services/web/service_test.go`: 7 unit tests (valid invitation, community image, invalid code, revoked, empty code, CSS serving, logo serving)
   - `server/main.go`: Registered `/go/`, `/css/`, `/assets/` handlers (public, no auth)
10. **Community image in CheckInvitation and app landing screen**:
   - `server/services/login/registration.go`: `getCommunityImageURL` populates `community_image_url` field in `CheckInvitationResponse` (presigned URL, 1hr expiry)
   - `app/lib/services/auth_service.dart`: Added `communityImageUrl` to `InvitationCheckResult`, mapped from response
   - `app/lib/presentation/screens/auth/invitation_landing_screen.dart`: Shows community image (circular, 100px) instead of generic `Icons.groups` icon, with fallback
   - `server/services/login/registration_test.go`: Added test for community image URL (with and without media)

### TODO

11. **DNS/Infrastructure** - Route ripls.app to Go server

---

## Open Questions

1. **Image sizing**: Should we generate OG-specific (1200x630) versions of community photos, or use originals?

2. **Rate limiting**: Should invitation page serve have rate limiting to prevent enumeration attacks on short codes?

3. **Analytics**: Should we track invitation link clicks/views server-side before app opens?

4. **Cache headers**: What cache policy for rendered invite pages? (OG crawlers may cache aggressively)
