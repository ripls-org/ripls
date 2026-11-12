# Dependency Review - 2026-04-28

## Executive Summary

**Overall Health**: Healthy

**Key Findings:**

- 0 critical updates with directly reachable CVEs in our code
- 1 govulncheck-flagged CVE in a direct dependency that is **imported but not called** (gRPC v1.79.2 → v1.79.3, GO-2026-4762 — auth bypass via missing leading slash in `:path`)
- 4 medium-priority Flutter Firebase ecosystem updates available (1–2 minor versions behind across the bundle)
- 1 Flutter package still requires a major-version migration (`fl_chart` 0.70.2 → 1.2.0)
- Flutter `protobuf` upgrade still blocked by `connectrpc ^1.0.0` requiring `protobuf <5.0.0` (unchanged blocker)
- All previously-flagged Critical findings (Go runtime 1.25.9, golang.org/x/image v0.39.0) and High findings (anthropic-sdk-go v1.38.0, mcp-go v0.49.0, genkit v1.6.1) have been resolved since the last review

**Immediate Actions Required:**

1. Bump indirect `google.golang.org/grpc` from v1.79.2 to v1.79.3+ (govulncheck flags GO-2026-4762 in `go.mod`; we don't appear to call the affected symbols, but a one-line patch closes the supply-chain risk)
2. None other — no reachable CVEs, no deprecated direct dependencies, all AI SDKs are current

---

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Resolved Issues:**

- ✅ Go runtime updated from 1.25.8 to 1.25.9 (closes GO-2026-4870 TLS DoS, GO-2026-4946/4947 x509, GO-2026-4869 archive/tar OOM, GO-2026-4865 html/template XSS — all of which were Critical last review)
- ✅ `golang.org/x/image` updated from v0.33.0 to v0.39.0 (closes GO-2026-4815 TIFF OOM and GO-2026-4961 WebP panic — both Critical last review, reachable via image upload)
- ✅ `github.com/anthropics/anthropic-sdk-go` updated from v1.26.0 to v1.38.0 (was High)
- ✅ `github.com/mark3labs/mcp-go` updated from v0.29.0 to v0.49.0 (was High — 20-version drift closed)
- ✅ `github.com/firebase/genkit/go` updated from v1.4.0 to v1.6.1 (was High)
- ✅ `google.golang.org/genai` updated from v1.49.0 to v1.51.0 (partial — still 3 versions behind v1.54.0)
- ✅ `github.com/weaviate/weaviate-go-client/v5` removed entirely (was a remaining issue — code path no longer in use)

**Remaining Issues:**

- ⏳ Flutter `protobuf` still at 4.2.0 (6.0.0 available — still blocked by `connectrpc ^1.0.0` requiring `<5.0.0`)
- ⏳ Flutter `fl_chart` still at 0.70.2 (1.2.0 available — major-version migration still pending across 3 chart widget files)
- ⏳ Flutter Firebase packages still 1–2 minor versions behind (firebase_core 4.5.0→4.7.0, firebase_auth 6.2.0→6.4.0, etc.)
- ⏳ `mapbox_maps_flutter` still pinned (no `^`) at 2.19.1 (2.22.1 available)

**New Issues:**

- 🆕 `google.golang.org/grpc` v1.79.2 has GO-2026-4762 (auth bypass via missing leading slash in `:path`); fixed in v1.79.3, latest v1.80.0. Govulncheck reports this as "imported but not called" (module-level finding, not symbol-level), so not directly reachable from our code, but the fix is a trivial bump.
- 🆕 `github.com/go-chi/chi/v5` v5.2.3 has GO-2026-4316 (open redirect in `RedirectSlashes` middleware). Indirect via `mailgun-go`; we do not invoke this middleware.

**Dependencies Updated Since Last Review:**

| Package | Previous | Current | Notes |
|---------|----------|---------|-------|
| Go runtime | 1.25.8 | 1.25.9 | Closes 5 stdlib CVEs |
| golang.org/x/image | v0.33.0 | v0.39.0 | Closes TIFF OOM + WebP panic |
| github.com/anthropics/anthropic-sdk-go | v1.26.0 | v1.38.0 | 12-version catch-up |
| github.com/mark3labs/mcp-go | v0.29.0 | v0.49.0 | 20-version catch-up |
| github.com/firebase/genkit/go | v1.4.0 | v1.6.1 | At latest |
| google.golang.org/genai | v1.49.0 | v1.51.0 | Still 3 versions behind |
| github.com/weaviate/weaviate-go-client/v5 | v5.1.0 | (removed) | Module no longer required |

**Trend**: Improving — every Critical and High finding from the previous review is resolved or no longer applicable. The two newly-surfaced govulncheck findings (gRPC, chi) are both module-level (not symbol-level) and are not reachable from our code paths. No reachable CVEs remain.

---

## Go Dependencies

### Critical Updates

_None._ No CVEs are reachable from our code per `govulncheck ./server/...`.

### High Priority Updates

_None._

### Medium Priority Updates

| Package | Current | Latest | Reason | Notes |
|---------|---------|--------|--------|-------|
| google.golang.org/grpc (indirect) | v1.79.2 | v1.80.0 | GO-2026-4762 auth-bypass advisory; fixed in v1.79.3 | Govulncheck flags as imported-not-called; bump anyway for supply-chain hygiene |
| google.golang.org/genai | v1.51.0 | v1.54.0 | 3 minor versions behind; AI SDK with rapid feature changes | Trivial |
| google.golang.org/api | v0.271.0 | v0.276.0 | 5 minor versions behind | Trivial |
| cloud.google.com/go/storage | v1.61.0 | v1.62.1 | Patch behind | Trivial |
| connectrpc.com/connect | v1.19.1 | v1.19.2 | Patch update | Trivial |
| github.com/PuerkitoBio/goquery | v1.11.0 | v1.12.0 | Minor update | Trivial |
| github.com/coreos/go-oidc/v3 | v3.17.0 | v3.18.0 | Minor update | Auth library — review changelog |
| golang.org/x/crypto | v0.48.0 | v0.50.0 | 2 minor versions behind | No confirmed CVEs in current version |
| golang.org/x/net | v0.51.0 | v0.53.0 | 2 minor versions behind | No confirmed CVEs in current version |
| github.com/jackc/pgx/v5 (indirect) | v5.8.0 | (latest) | Driver behind | Pulled in via fake-gcs-server / lib path |
| github.com/testcontainers/testcontainers-go | v0.40.0 | v0.42.0 | Test dep behind | Low risk |
| github.com/lib/pq | v1.11.2 | v1.12.3 | Minor update | Trivial |
| github.com/invopop/jsonschema | v0.13.0 | v0.14.0 | Minor update | Review for breaking changes |
| github.com/bradleyfalzon/ghinstallation/v2 | v2.17.0 | v2.18.0 | Patch update | Trivial |

### Low Priority Updates

| Package | Current | Latest | Notes |
|---------|---------|--------|-------|
| github.com/docker/docker | v28.5.1+incompatible | v28.5.2+incompatible | Test dep; AuthZ vulns (GO-2026-4887, GO-2026-4883) still have no fix available |
| github.com/fsouza/fake-gcs-server | v1.52.3 | v1.54.0 | Test dependency |
| github.com/yalue/onnxruntime_go | v1.14.0 | v1.28.0 | ML runtime binding — review for binary-compat concerns before bumping |

### Up to Date

- `github.com/anthropics/anthropic-sdk-go` v1.38.0
- `github.com/firebase/genkit/go` v1.6.1
- `github.com/mark3labs/mcp-go` v0.49.0
- `golang.org/x/image` v0.39.0
- `golang.org/x/oauth2` v0.36.0
- `google.golang.org/protobuf` v1.36.11
- `firebase.google.com/go/v4` v4.19.0
- `github.com/openai/openai-go` v1.12.0
- `github.com/golang-jwt/jwt/v5` v5.3.1
- `github.com/google/uuid` v1.6.0
- `github.com/stretchr/testify` v1.11.1
- `github.com/coreos/go-oidc/v3` v3.17.0 (v3.18.0 available, listed above)
- `github.com/disintegration/imaging` v1.6.2
- `github.com/mailgun/mailgun-go/v4` v4.23.0

### Dependency Health Concerns

| Package | Issue | Recommendation |
|---------|-------|----------------|
| github.com/adrium/goheif | Pinned to 2023 commit, low activity | Stable read-only HEIF decoder; monitor |
| github.com/rwcarlsen/goexif | Pinned to 2019 commit, effectively unmaintained | Stable read-only EXIF parser; monitor |
| github.com/golang/protobuf | Deprecated (indirect) | Resolves naturally with upstream updates |
| go.mongodb.org/mongo-driver | Deprecated v1 (transitive via fake-gcs-server) | Not actionable until fake-gcs-server upgrades |
| github.com/docker/docker | Two govulncheck vulns (GO-2026-4887, GO-2026-4883) with no fix available | Test-only dep; low production risk |

---

## Flutter Dependencies

### Critical Updates

_None._ No active CVEs affecting Flutter direct dependencies.

### High Priority Updates

| Package | Current | Latest | Reason | Migration Effort |
|---------|---------|--------|--------|------------------|
| protobuf | 4.2.0 | 6.0.0 | 2 major versions behind | **Blocked by connectrpc ^1.0.0** (still requires `>=3.1.0 <5.0.0`); monitor for resolution |
| fl_chart | 0.70.2 | 1.2.0 | 1 major version behind, breaking API changes | Moderate (2-4 hours) — affects 3 chart widget files |

**protobuf 6.0.0 Blocker (Unchanged):** `connectrpc` is still at v1.0.0 (no newer release), and v1.0.0 caps protobuf to `<5.0.0`. Skipping this run.

### Medium Priority Updates

| Package | Current | Latest | Reason | Notes |
|---------|---------|--------|--------|-------|
| firebase_core | 4.5.0 | 4.7.0 | 2 minor versions behind | Bundle with other Firebase updates |
| firebase_analytics | 12.1.3 | 12.3.0 | 2 minor versions behind | Bundle |
| firebase_auth | 6.2.0 | 6.4.0 | 2 minor versions behind | Bundle |
| firebase_crashlytics | 5.0.8 | 5.2.0 | 2 minor versions behind | Bundle |
| firebase_messaging | 16.1.2 | 16.2.0 | 1 minor version behind | Bundle |
| firebase_performance | 0.11.1+5 | 0.11.3 | Patch behind | Bundle |
| mapbox_maps_flutter | 2.19.1 | 2.22.1 | 3 minor versions behind; pinned (no `^`) | Loosen pin and review changelog before update |
| go_router | 17.1.0 | 17.2.2 | 1 minor version behind | Trivial |
| connectivity_plus | 7.0.0 | 7.1.1 | Minor version behind | Trivial |

### Low Priority Updates

| Package | Current | Latest | Notes |
|---------|---------|--------|-------|
| app_badge_plus | 1.2.7 | 1.2.9 | Patch updates |
| shared_preferences | 2.5.4 | 2.5.5 | Patch update |
| cupertino_icons | 1.0.8 | 1.0.9 | Patch update |
| device_info_plus | 12.3.0 | 13.1.0 | Major version available; review when ready |
| share_plus | 12.0.1 | 13.1.0 | Major version available; review when ready |
| package_info_plus | 9.0.0 | 10.1.0 | Major version available; review when ready |
| flutter_typeahead | 5.2.0 | 6.0.0 | Major version available; review when ready |
| flutter_timezone | 4.1.1 | 5.0.2 | Major version available; review when ready |
| build_runner | 2.12.2 | 2.14.1 | Dev dependency |
| freezed | 3.2.3 | 3.2.5 | Dev dependency |
| mockito | 5.6.3 | 5.6.4 | Dev dependency |
| patrol | 4.3.0 | 4.5.0 | Dev dependency |
| patrol_finders | 3.1.0 | 3.2.0 | Dev dependency |
| camera | 0.11.4 | 0.12.0+1 | Major version; review when camera features are next touched |

### Up to Date

Key dependencies at latest compatible versions:
- `connectrpc` 1.0.0 — at latest published version
- `flutter_riverpod` 3.3.1 — at latest
- `flutter_local_notifications` 21.0.0 — at latest
- `flutter_secure_storage` 10.0.0 — at latest
- `dotted_border` 3.1.0 — at latest
- `google_sign_in` ^7.2.0 — at latest
- `geolocator` ^14.0.2 — at latest
- `image_picker` ^1.1.2 — at latest
- `url_launcher` ^6.3.1 — at latest
- `qr_flutter` ^4.1.0 — at latest
- `freezed_annotation` ^3.1.0 — at latest

### Dependency Health Concerns

| Package | Issue | Recommendation |
|---------|-------|----------------|
| protobuf | Blocked by connectrpc constraint | Monitor connectrpc for 6.x support |
| connectrpc | Only v1.0.0 released (12+ months ago) | Monitor for updates, especially protobuf 6.x support |
| mapbox_maps_flutter | Pinned to exact version 2.19.1 (no `^`) | Loosen to `^2.19.1` once changelog for 2.20–2.22 is reviewed |

---

## Security Analysis

### Known Vulnerabilities

| Package | ID | Severity | Affected Versions | Fixed In | Our Version | Status |
|---------|-----|----------|-------------------|----------|-------------|--------|
| google.golang.org/grpc | GO-2026-4762 | High — auth bypass | < v1.79.3 | v1.79.3 | v1.79.2 | **VULNERABLE (imported, not called)** — module-level finding only; no symbol reachable from our code per govulncheck |
| github.com/go-chi/chi/v5 | GO-2026-4316 | Medium — open redirect | v5.2.3 | (transitive only) | v5.2.3 | VULNERABLE (transitive via mailgun); we do not call `RedirectSlashes` |
| github.com/docker/docker | GO-2026-4887 | Medium | — | N/A | v28.5.1 | VULNERABLE (test only) — no fix available |
| github.com/docker/docker | GO-2026-4883 | Medium | — | N/A | v28.5.1 | VULNERABLE (test only) — no fix available |
| crypto/tls (stdlib) | GO-2026-4870 | High — DoS | go1.25.8 | go1.25.9 | go1.25.9 | **SAFE** |
| crypto/x509 (stdlib) | GO-2026-4946/4947 | Medium | go1.25.8 | go1.25.9 | go1.25.9 | **SAFE** |
| archive/tar (stdlib) | GO-2026-4869 | High — OOM | go1.25.8 | go1.25.9 | go1.25.9 | **SAFE** |
| html/template (stdlib) | GO-2026-4865 | High — XSS | go1.25.8 | go1.25.9 | go1.25.9 | **SAFE** |
| golang.org/x/image/tiff | GO-2026-4815 | High — OOM | < v0.38.0 | v0.38.0 | v0.39.0 | **SAFE** |
| golang.org/x/image/webp | GO-2026-4961 | Medium — panic | < v0.39.0 | v0.39.0 | v0.39.0 | **SAFE** |
| golang.org/x/net/http2 | CVE-2026-27141 | High | v0.50.0–v0.50.x | v0.51.0 | v0.51.0 | **SAFE** |
| golang.org/x/crypto/ssh | CVE-2024-45337 | Critical | < v0.31.0 | v0.31.0 | v0.48.0 | **SAFE** |
| github.com/jackc/pgx/v5 | CVE-2024-27304 | High | < v5.5.4 | v5.5.4 | v5.8.0 | **SAFE** |

`govulncheck ./server/...` summary: **2 module-level vulnerabilities found, 0 reachable** (Docker test-only dep with no fix). The gRPC and chi findings are listed under "modules you require, but your code doesn't appear to call these vulnerabilities."

### Packages with Security Concerns

| Package | Concern | Risk Level | Recommendation |
|---------|---------|------------|----------------|
| google.golang.org/grpc v1.79.2 | GO-2026-4762 advisory; not reachable but published CVE in direct-to-indirect dep | Low–Medium | Bump indirect to v1.80.0 in next routine update |
| github.com/go-chi/chi/v5 v5.2.3 | GO-2026-4316 open-redirect (transitive via mailgun); we do not use the affected middleware | Low | Resolves naturally when mailgun-go updates |
| github.com/docker/docker v28.5.1 | Two AuthZ/plugin-validation CVEs with no patch available (test-only dep) | Low (test-only) | Monitor docker/docker for fix |

### Supply Chain Risks

| Package | Risk | Mitigation |
|---------|------|------------|
| AI SDKs (anthropic v1.38.0, genai v1.51.0, genkit v1.6.1, mcp-go v0.49.0) | Rapid development cycle — drift can accumulate quickly between reviews | All current as of this run; consider Dependabot/Renovate for automated PRs |
| github.com/adrium/goheif | Pinned to 2023 commit, single maintainer | Low risk (read-only HEIF decoder); monitor |
| github.com/rwcarlsen/goexif | Pinned to 2019 commit, effectively unmaintained | Low risk (read-only EXIF parser); monitor |

---

## License Compliance

All dependencies use permissive licenses compatible with commercial use:

| License | Primary Packages |
|---------|------------------|
| Apache-2.0 | Google Cloud SDKs, Firebase, gRPC, Connect-RPC |
| MIT | Most utility packages, testify, pgx |
| BSD-3-Clause | Google packages, golang.org/x/* |
| BSD-2-Clause | Some utility packages |

No copyleft (GPL/AGPL) licenses detected in direct dependencies.

---

## Deprecated/Unmaintained Packages

| Package | Status | Last Activity | Alternative | Migration Effort |
|---------|--------|---------------|-------------|------------------|
| github.com/golang/protobuf | Deprecated (indirect) | — | google.golang.org/protobuf | In progress via upstream |
| go.mongodb.org/mongo-driver v1 | Deprecated (transitive) | — | go.mongodb.org/mongo-driver/v2 | Not actionable (transitive via fake-gcs-server) |

---

## Transitive Dependency Issues

### Version Conflicts

No significant version conflicts detected. Go module resolution and Flutter pub both produce clean dependency graphs.

### Heavy Dependencies

| Package | Transitive Deps | Notes |
|---------|-----------------|-------|
| cloud.google.com/go/* | ~100+ modules | GCP SDK ecosystem — expected |
| firebase.google.com/go | ~50+ modules | Firebase ecosystem — expected |

---

## Breaking Change Migration Guide

### Flutter fl_chart 0.70.2 → 1.2.0 (Unchanged from prior review)

**Breaking Changes:**

1. `colors` property renamed to `color` (single solid) with separate `gradient` field
2. Touch callback signature changed
3. `tooltipBgColor` removed, replaced with `getTooltipColor`
4. `y` property renamed to `toY` in `BarChartRodData`

**Our Usage:**

- `app/lib/presentation/widgets/charts/dual_axis_chart_widget.dart`
- `app/lib/presentation/widgets/charts/area_chart_widget.dart`
- `app/lib/presentation/widgets/charts/bar_chart_widget.dart`

**Estimated Effort:** Moderate (2-4 hours)

---

## Recommendations

### Immediate (This Week)

_None required._ All Critical/High findings from the prior review are resolved; no reachable CVEs remain.

### Short Term (This Month)

1. **Bump `google.golang.org/grpc` to v1.80.0** — Forces the indirect dependency past GO-2026-4762. Run `go get google.golang.org/grpc@latest && go mod tidy`. Govulncheck currently reports it as not-called, but trivially closing the advisory is safer than carrying it.
2. **Update `google.golang.org/genai`** — v1.51.0 → v1.54.0; AI SDK with frequent feature additions. Run `go get google.golang.org/genai@latest && go mod tidy && go test -tags=integration ./server/ai/...`.
3. **Bundle Go ecosystem updates** — `cloud.google.com/go/storage`, `connectrpc.com/connect` (patch), `golang.org/x/crypto`, `golang.org/x/net`, `google.golang.org/api`. Run `go get -u <each> && go mod tidy && npm run build`.
4. **Bundle Flutter Firebase updates** — `firebase_core` 4.5.0→4.7.0 and the dependent firebase_* packages. Run `cd app && flutter pub upgrade firebase_core firebase_analytics firebase_auth firebase_crashlytics firebase_messaging firebase_performance`.
5. **Loosen and bump `mapbox_maps_flutter`** — Change pinned `2.19.1` to `^2.19.1` in `app/pubspec.yaml`, run `flutter pub upgrade mapbox_maps_flutter` to pick up 2.22.1.

### Long Term (This Quarter)

1. **Migrate `fl_chart` to 1.x** — Multiple breaking changes across 3 chart widget files; ~2–4 hours.
2. **Monitor `connectrpc` (Dart)** for protobuf 6.x support release; upgrade Flutter `protobuf` package when unblocked.
3. **Evaluate Flutter major-version bumps** — `camera` 0.12.0+1, `device_info_plus` 13.x, `share_plus` 13.x, `package_info_plus` 10.x, `flutter_typeahead` 6.x, `flutter_timezone` 5.x. Cluster these into one focused upgrade pass.
4. **Set up automated dependency monitoring** — The pattern of drift accumulating between manual reviews is consistent. Dependabot or Renovate Bot configured for security-relevant updates would file PRs automatically and reduce the per-review backlog.

---

## Update Commands

### Critical Updates

_None required._

### High Priority / Recommended

```bash
# Bump grpc past GO-2026-4762
go get google.golang.org/grpc@latest
go mod tidy

# AI SDK — modest catch-up
go get google.golang.org/genai@latest
go mod tidy
npm run build
go test -tags=integration ./server/ai/...
```

### Medium Priority Updates

```bash
# Go ecosystem bundle
go get cloud.google.com/go/storage@latest
go get connectrpc.com/connect@latest
go get golang.org/x/crypto@latest
go get golang.org/x/net@latest
go get google.golang.org/api@latest
go mod tidy
npm run build
go test ./server/...

# Flutter Firebase bundle
cd app && flutter pub upgrade firebase_core firebase_analytics firebase_auth firebase_crashlytics firebase_messaging firebase_performance

# Flutter other medium
cd app && flutter pub upgrade go_router connectivity_plus app_badge_plus shared_preferences cupertino_icons
```

### Bulk Update (After Testing)

```bash
# Update all Go dependencies to latest compatible
go get -u ./...
go mod tidy

# Update all Flutter dependencies (within constraints)
cd app && flutter pub upgrade
```

---

## Testing Checklist

After applying updates:

- [ ] `go test ./server/...` passes
- [ ] `go test -tags=integration ./server/...` passes
- [ ] `flutter test --reporter expanded` passes
- [ ] `npm run build` passes (lint + compile)
- [ ] `npm run generate` produces valid code
- [ ] `govulncheck ./server/...` shows 0 reachable vulnerabilities
- [ ] Manual smoke test: image upload (JPEG, PNG, WebP, TIFF, HEIF)
- [ ] Manual smoke test: authentication flow
- [ ] Manual smoke test: Firebase push notification

---

## Next Review

Schedule the next dependency review in 2-4 weeks or when:

- Any new Go security advisories are published affecting reachable code paths
- `connectrpc` releases protobuf 6.x support
- Before any production deployment

## Issues to File

_None this run._

The previous review's four filed issues (Critical: x/image CVE, Critical: Go runtime 1.25.9, High: anthropic-sdk-go drift, High: mcp-go drift) are all resolved in the current `go.mod`. No remaining finding clears the severity bar — the only flagged advisory in our direct/indirect graph (GO-2026-4762 in gRPC v1.79.2) is reported by govulncheck as imported-but-not-called, and the recommended trivial bump is already captured in the "Short Term" section. Filing it as a tracked issue would be triage noise.
