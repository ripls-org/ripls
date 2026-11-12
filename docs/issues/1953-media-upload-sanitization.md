# Issue #1953: Image upload path has no content-type, size, or EXIF sanitization

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1953
**Filed:** 2026-05-15 | **Last activity:** 2026-05-25
**Labels:** none (Type: Bug, Priority: P1 via Development Issues project) | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

Every media upload funnels through `storage.StoreMedia` (`server/storage/media.go:364`), which stores the client-supplied bytes and content type **verbatim** and serves them back via signed URLs. There is no server-side validation of:

1. **Declared MIME type** — the client's `content_type` is trusted as-is; `isImageContentType` (`media.go:36`) only checks an `image/` prefix, so `image/svg+xml` (which can carry inline `<script>`) passes the gate.
2. **Actual file contents** — no `http.DetectContentType` sniff, so declared type can lie about the bytes.
3. **File size** — `StoreMedia` accepts `data []byte` of any length. A per-message cap exists only on the *URL-fetch* path (`addMediaFromURLMaxBytes = 60 MB`, `service.go:35`), not the direct `AddMedia` upload.
4. **EXIF metadata** — the full-size object retains GPS coordinates + device identifiers, served to community members via signed URLs and re-fetched by AI providers. This is a **user-privacy** concern, not just a malice one.
5. **Decoded dimensions** — `image.Decode` in the thumbnail path allocates based on the declared header dimensions before parsing pixels; a small file declaring 100000×100000 can OOM the server.

The threat is bounded by auth (no public upload form — `AddMedia` calls `auth.RequireAuth`), and bucket keys are user-scoped, but a compromised account is a real vector and the EXIF leak harms users with no malice required.

**Of the five, four are mechanical and carry no functional risk. EXIF (#4) is the only one that touches a live code path, so the bulk of the analysis below is about scoping it safely. Investigation this session (see Context) showed that risk is far smaller than the issue implies.**

## Context from Comments

The issue has only one comment — the `/ripls-triage` entry (2026-05-19) setting **Type: Bug, Priority: P1** ("closed-beta blocker or P1 carry-forward"). No design discussion happened in-thread.

The substantive direction comes from a maintainer (briangbrown) investigation this session, which establishes the EXIF scoping:

- **`StoreMedia` is the single upload chokepoint.** All 12 call sites — user `AddMedia`, webpage-image (`experience`/`gear`), login avatar, and stock providers (Unsplash/Pexels/Pixabay/fake) — funnel through it. Validation and EXIF stripping placed there cover every path.
- **GPS-from-EXIF is a dead product path.** The only consumer of `location.ExtractGeolocationFromEXIF` is the `SuggestGearLocation` RPC (`server/services/location/exif.go`), which re-fetches the *stored* bytes to read GPS. **The client never calls it** — zero call sites in `app/lib` outside generated stubs. Location is already explicit everywhere via `LocationPicker` (gear, experience, request, unified-create) plus permissioned live device GPS. So removing GPS from stored EXIF has **zero user-facing regression** and directly advances the "make location explicit" goal.
- **Orientation is the only functional EXIF dependency.** `getEXIFOrientation`/`applyEXIFOrientation` (`media.go:344`) rotate the **thumbnail**, which is already re-encoded clean (carries no EXIF forward). The full-size stored image relies on the EXIF Orientation tag for correct client display. HEIC rotation uses ISOBMFF metadata (`getHEICRotations`), not EXIF, so it is unaffected by an EXIF strip.
- **The EXIF library is read-only.** `rwcarlsen/goexif` can decode but not rewrite, so an in-place selective strip needs either a new dependency (e.g. `dsoprea/go-exif`) or manual JPEG APP1-segment surgery; decode→re-encode is the no-new-dep fallback.
- **Client parses no EXIF.** `geolocator`/`image_picker` are live-GPS, not photo metadata.

**Maintainer direction:** err on the side of privacy — strip privacy-sensitive EXIF, keep location entry explicit.

## Potential Solutions

The four mechanical protections (content-type sniff + allow-list, size cap, dimension cap, decoder pinning) have a single sensible design each and are addressed directly in the Implementation Plan. The real design choice is the **EXIF strip strategy**:

### Option A: Selective EXIF strip — drop GPS + device/MakerNote (+ timestamps), keep Orientation
- **Approach:** Rewrite the image's EXIF segment removing the GPS IFD and camera/device identifier tags (Make/Model/MakerNote/serial) and capture timestamps, while preserving the Orientation tag. Scoped to images; video untouched. Requires an EXIF rewriter — add `dsoprea/go-exif/v3`, or do JPEG-only APP1 surgery for the formats we accept.
- **Pros:** Full-size display and thumbnail rotation keep working with no pixel re-encode (no quality loss, low CPU). Surgical — removes exactly the private fields.
- **Cons:** New dependency (or hand-rolled segment parsing). Per-format handling (JPEG EXIF vs HEIC/WebP containers). Orientation tag survives (harmless, not private).
- **Effort:** Medium.

### Option B: Decode → re-encode (normalize), dropping all metadata
- **Approach:** Decode the image, bake EXIF/ISOBMFF orientation into pixels, re-encode (JPEG/PNG/WebP), which drops *all* metadata as a side effect. `disintegration/imaging` is already imported and the thumbnail path already does exactly this rotation bake.
- **Pros:** Most privacy-complete — removes GPS, device, **and** timestamps and everything else. No new dependency. Reuses existing orientation-bake logic.
- **Cons:** Lossy for JPEG; CPU cost on every upload; must handle each accepted format and decide an output encoding (e.g. normalize HEIC→JPEG, which changes content type and size). Higher regression surface (every uploaded image's bytes change).
- **Effort:** Medium-large.

### Option C: Strip the entire EXIF/APP1 segment, no re-encode, no orientation preservation
- **Approach:** Remove the whole APP1 (EXIF) marker from JPEG without re-encoding pixels.
- **Pros:** Simplest, removes all EXIF including GPS; no quality loss; minimal code for JPEG.
- **Cons:** Drops Orientation too — full-size images shot in portrait/landscape would display rotated on clients that honor EXIF orientation (and the thumbnail would lose its rotation source). Would need a pixel-bake to compensate anyway, collapsing into Option B. Format coverage still required.
- **Effort:** Small (JPEG only) but **functionally risky** on orientation.

## Recommendation

**Ship the four mechanical protections first (no functional risk, highest security ROI), then do the EXIF strip as Option A (selective, keep Orientation).**

Option A is the privacy win with no display regression and no quality loss; the only reason to weigh B is the read-only-goexif constraint, and adding a maintained EXIF rewriter is a cleaner cost than re-encoding every upload. If we want to also drop capture timestamps and the dependency footprint is unwelcome, B is the fallback — but its quality/CPU cost and per-format encoding decisions make it the heavier path. Avoid C: it reintroduces the orientation problem it's trying to skip.

Pair the EXIF work with **retiring the dead `SuggestGearLocation` EXIF dependency** — post-strip it would only ever return `Found: false`, so it should either be removed or explicitly documented as deprecated, keeping the codebase honest about location being explicit-only.

Enforcement placement: do content-type sniff + allow-list + size cap at the **`StoreMedia` boundary** (covers all 12 callers) rather than only in `AddMedia`. Add a defense-in-depth `connect.WithReadMaxBytes` on the `MediaService` handler too.

## Decisions (resolved by maintainer)

- **Size caps — DECIDED: 60 MB images / 200 MB video, made consistent codebase-wide.** Enforce the authoritative content-type-aware check in `StoreMedia` (single chokepoint) plus a `connect.WithReadMaxBytes` backstop on the `MediaService` handler (`main.go:1505`). Introduce shared exported constants — `storage.MaxImageUploadBytes = 60 * 1024 * 1024`, `storage.MaxVideoUploadBytes = 200 * 1024 * 1024` — and **replace the three inconsistent 10 MB image caps** (`server/services/experience/webpage_image.go:22`, `server/services/gear/webpage_image.go:21`, `server/services/login/helpers.go:46`) and the 60 MB `addMediaFromURLMaxBytes` (`media/service.go:35`) so every media path references the same constants. Leave unrelated limits alone: `webfetch.DefaultMaxBodySize` (5 MB, HTML body for page fetch) and the client `video_cache_manager` 200 MB (a disk-*cache* cap, not an upload cap).
- **Strip dependency — `dsoprea/go-exif/v3`** behind `StripPrivateEXIF(data, contentType) ([]byte, error)` in a new `server/storage/media_sanitize.go` (not `server/location`, which is geolocation/Mapbox-scoped; keeps `media.go` under the <1000-line convention).
- **Format coverage — DECIDED: cover ALL supported image formats** (jpeg, png, webp, tiff, heic — the decoders registered in `media.go`). No silent pass-through for any image format. Where in-place selective strip can't cover a format, use decode→re-encode for that format (guaranteed metadata-free).
- **Stock providers — DECIDED: do NOT strip stock-provider images.** Unsplash/Pexels/Pixabay images are curated, carry no user GPS, and re-processing them is wasted work. Gate the strip on the existing `sourceStockImageID` parameter already passed to `StoreMedia`: strip when `sourceStockImageID == ""`, skip when set. This means user uploads (`AddMedia`), webpage-image (`experience`/`gear`), and login avatars **are** stripped (they're user-directed/arbitrary-source); only stock-library images are exempt.
- **HEIC — DECIDED: keep `image/heic` in the allow-list AND strip it.** It's the iPhone default, so rejecting it is unacceptable. Since HEIC GPS lives in ISOBMFF container boxes (not a JPEG APP1 segment), if `dsoprea/go-exif` can't rewrite HEIC, use **decode→re-encode for HEIC** to guarantee a metadata-free result rather than passing it through. The content-type sniff must tolerate HEIC (`http.DetectContentType` may report `application/octet-stream`; special-case the ISOBMFF `ftyp` brand, mirroring the existing tolerance in `SuggestGearLocation`).
- **Strip failure — DECIDED: fail closed.** Never `Put` the original unstripped bytes. On an unrecoverable strip failure, reject the upload (`connecterr.Internal(ctx, "StoreMedia", err, ...)` for internal errors; `CodeInvalidArgument` for malformed/undecodable input). The decode→re-encode path above is part of the strip strategy for formats that need it — not a soft fallback that may retain the original.

## Open Questions

- **Does removing `SuggestGearLocation` need a proto/RPC deprecation?** — Recommendation: it's an exposed RPC, so don't delete the proto outright; mark it deprecated and make the handler return `Found: false` (or `CodeUnimplemented`) so no client breaks. Any inline TODO must carry an issue ref per CLAUDE.md (`TODO(#1953):` or a dedicated cleanup issue). Removing the proto is a separate cleanup.

## Conventions to follow (per server architecture & observability docs)

- **Log levels for rejections.** Content-type mismatch, disallowed type (SVG), oversize, and over-dimension are **user input errors** → log at `WARN` (or `INFO`), **never `ERROR`** (`docs/server/observability.md`: ERROR is reserved for system failures and trips the on-call alert). Internal failures (e.g. an unexpected strip-library error) use `connecterr.Internal(ctx, op, err, ...)`, which logs server-side and returns a generic public message — bare `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js`.
- **Structured fields, no payloads.** Use `logging.LoggerWithContext(ctx).With("operation", "StoreMedia", "user_id", userID, "media_id", mediaID, "content_type", contentType, "declared_type", ..., "sniffed_type", ..., "size_bytes", len(data))`. **Never log raw image bytes or decoded EXIF values** (the GPS coords are exactly the private data we're removing). Mask email if logged (`logging.MaskEmail`).
- **`connect` error codes.** Validation rejections → `connect.CodeInvalidArgument` (plain `connect.NewError` is fine for non-internal codes). Strip/internal failures → `connecterr.Internal`. See the fail-closed open question.
- **Storage-layer tests.** `StoreMedia` tests use `storage.SetupTestStorage(t)` (testcontainers) and the existing bucket mock; assert the stored object's bytes for the EXIF cases (decode the read-back bytes and confirm no GPS IFD, Orientation preserved). Unit-test `StripPrivateEXIF` directly, table-driven, with fixture images (reuse the GPS/no-GPS fixtures already in `server/location/exif_test.go`).
- **File size.** New validation + strip code goes in focused new files (e.g. `media_sanitize.go`), not appended to `media.go`.

## Implementation Plan

### Phase 1: Content-type sniff, allow-list, and size cap
**Goal:** Reject MIME spoofing, disallowed types (notably SVG), and oversized uploads — the highest-ROI, zero-functional-risk protections.
**Tasks:**
- [ ] In `StoreMedia` (before `bucketStorage.Put`, `media.go:401`): `http.DetectContentType(data[:512])`, compare to declared `contentType`, and validate against an allow-list (`image/jpeg`, `image/png`, `image/webp`, `image/heic`, `video/mp4`); reject SVG and any non-listed type with `connect.CodeInvalidArgument`. Special-case HEIC `ftyp` sniffing.
- [ ] Replace/extend `isImageContentType` so SVG no longer passes the image gate.
- [ ] Add shared exported constants `storage.MaxImageUploadBytes = 60 * 1024 * 1024` and `storage.MaxVideoUploadBytes = 200 * 1024 * 1024`; enforce the content-type-aware cap in `StoreMedia`.
- [ ] **Unify existing size limits codebase-wide** to reference the new constants: replace the 10 MB caps in `experience/webpage_image.go:22`, `gear/webpage_image.go:21`, `login/helpers.go:46`, and the 60 MB `addMediaFromURLMaxBytes` in `media/service.go:35`. Leave `webfetch.DefaultMaxBodySize` (5 MB HTML) and the client `video_cache_manager` 200 MB cache cap untouched (unrelated).
- [ ] Add `connect.WithReadMaxBytes` to the `MediaService` handler registration only (`main.go:1505`) as a transport backstop — `append(commonOpts, connect.WithReadMaxBytes(...))` for this handler rather than widening the shared `commonOpts` for every service.
- [ ] Unit tests: declared-vs-sniffed mismatch rejected; SVG rejected; oversize image (>60 MB) / video (>200 MB) rejected; valid jpeg/png/webp/heic/mp4 accepted; the three migrated callers still enforce a cap (now 60 MB).
**Validates:** Spoofed and disallowed uploads are rejected at the single chokepoint with no impact on legitimate media.

### Phase 2: Decoded-dimension cap in thumbnail path
**Goal:** Prevent header-driven OOM during thumbnail generation.
**Tasks:**
- [ ] In `generateThumbnail`/`shouldGenerateThumbnail`, call `image.DecodeConfig` (header-only) first; if width or height exceeds the cap (suggest 8000×8000), skip the thumbnail (or reject) before `image.Decode` allocates.
- [ ] Unit test: a PNG declaring 100000×100000 is rejected/skipped before full decode.
**Validates:** A malicious dimension header can no longer exhaust memory.

### Phase 3: Privacy EXIF strip (Option A) + retire dead GPS path
**Goal:** Stored full-size images carry no GPS/device EXIF, while display and thumbnail orientation keep working; location stays explicit-only.
**Tasks:**
- [ ] Add `dsoprea/go-exif/v3` and a `StripPrivateEXIF(data, contentType) ([]byte, error)` helper that removes the GPS IFD + Make/Model/MakerNote/serial + capture timestamps and preserves Orientation. **Cover all supported image formats** (jpeg, png, webp, tiff, heic); where in-place rewrite can't cover a format, decode→re-encode it to guarantee metadata-free output.
- [ ] **HEIC:** since its GPS lives in ISOBMFF boxes, decode→re-encode HEIC if `dsoprea/go-exif` can't rewrite it — do not pass HEIC GPS through.
- [ ] Call `StripPrivateEXIF` in `StoreMedia` between content-type validation and `bucketStorage.Put`, for image types only; leave video bytes untouched. **Skip stock-provider images** — gate on `sourceStockImageID == ""` so Unsplash/Pexels/Pixabay bytes are stored as-is while user/webpage/avatar uploads are stripped. **Fail closed on strip error**: reject (`connecterr.Internal` / `CodeInvalidArgument`) — never `Put` the original.
- [ ] Retire the dead EXIF location path: mark `SuggestGearLocation` deprecated (`TODO(#1953):`) and have it return `Found: false`; or remove the handler body and keep the proto. Remove/quarantine `location.ExtractGeolocationFromEXIF` if it has no other callers.
- [ ] Tests: GPS-bearing fixtures in **each format** read back from the bucket with no GPS; Orientation preserved (or baked) so a portrait photo renders upright and the thumbnail stays correctly rotated; a stock-sourced upload (`sourceStockImageID` set) is stored byte-for-byte unchanged; strip failure rejects rather than storing the original; video bytes unchanged.
**Validates:** GPS/device data no longer reaches GCS or community members/AI providers for any user-uploaded image format; stock images and orientation/location-entry behavior unchanged.

### Phase 4: Decoder version pinning + monitoring
**Goal:** Reduce exposure to known decoder CVEs (lowest priority).
**Tasks:**
- [ ] Confirm `golang.org/x/image` (v0.39.0), `adrium/goheif`, and `disintegration/imaging` are current; bump if behind.
- [ ] Ensure Dependabot (or equivalent) watches these for security advisories.
**Validates:** Third-party decoders are on patched versions and monitored.

## Related Issues
- #1939 — unified-create classifier; its image bytes flow through this same upload path. The gaps here pre-date and are independent of it, but Phase 1–3 changes will also harden the classifier flow.
- #2066 / #2098 — per-user rate limiting on `AddMediaFromURL` / streaming gen; complementary abuse controls on the same media surface (size cap here is the per-request analogue to those per-user limits).

## Audit Log

Reviewed against `docs/server/architecture.md` and `docs/server/observability.md` (client docs N/A — no client UI, string, caching, or a11y change; the only client-side fact is that `SuggestGearLocation` is uncalled). Changes applied:

- **Helper placement / file size.** Moved the `StripPrivateEXIF` helper out of the proposed `server/location` (scoped to geolocation/Mapbox per the architecture doc) into a new `server/storage/media_sanitize.go`, keeping `media.go` under the <1000-line convention.
- **Fail-closed error handling.** Added an explicit open question requiring strip failures to never store the original bytes (Fail Fast + privacy), with re-encode-fallback or `connecterr.Internal` reject.
- **Observability conventions.** Added a conventions section: rejections log at WARN/INFO (never ERROR), no raw bytes / decoded EXIF in logs, `connecterr.Internal` for internal failures (codeinternal lint gate), standard structured fields.
- **Testing conventions.** Specified `storage.SetupTestStorage(t)` + bucket mock for storage tests and reuse of the existing `server/location/exif_test.go` GPS fixtures.
- **Handler option mechanics.** Clarified `WithReadMaxBytes` is appended for the `MediaService` handler only, not the shared `commonOpts`.
- **TODO discipline.** `SuggestGearLocation` deprecation TODO must carry an issue ref (`TODO(#1953):`).

## Phase 3 — implementation note (revised approach)

A new constraint surfaced during implementation: **Ripls has a web client, and HEIC does not render outside Safari** (Chrome/Firefox/Edge — ~95% of users — show a broken image for an `<img>` pointing at HEIC). TIFF is likewise not browser-renderable. So non-web formats must be transcoded on ingest regardless of EXIF. Given that, and the maintainer direction to "strip everything we can," Phase 3 was implemented as a **uniform decode → re-encode** rather than the selective in-place strip originally planned:

- `storage.SanitizeImageForStorage(data, contentType)` decodes the image (via the already-registered jpeg/png/webp/tiff/heic decoders), **bakes orientation into the pixels** (EXIF for jpeg/tiff, ISOBMFF for heic), and re-encodes — dropping GPS, device identifiers, capture timestamps, and all other metadata as a side effect. Single code path, **no new dependency**.
- Output formats (web-renderable): **PNG → PNG** (lossless, keeps alpha); **everything else → JPEG** at a uniform **q90**. So HEIC/TIFF/WebP all become JPEG; JPEG re-encodes to JPEG.
- **GIF passes through untouched** (the format can't carry EXIF, renders everywhere, and re-encoding would flatten animation).
- **HEIC quality:** there is no readable "equivalent JPEG quality" — HEVC QP doesn't map to JPEG quality and isn't exposed by the decoder — so a fixed q90 is used (the standard choice; perceptual-target encoding would require a metric lib and is out of scope).
- **Stock-provider images are exempt** (`sourceStockImageID != ""`): curated, no user GPS, stored byte-for-byte.
- **Fail closed:** any decode/encode failure rejects the upload; the original bytes are never stored.
- Wired into `StoreMedia` after validation, before the bucket write; the media row's `ContentType`/`SizeBytes` reflect the sanitized output.

**Consequences / follow-ups (not in this PR):**
- `SuggestGearLocation` is now naturally dead — sanitized stored bytes carry no GPS, so it always returns `Found: false`. It can be removed in a follow-up (kept untouched here to avoid an RPC change).
- HEIC/TIFF uploads are now served as JPEG; the stored filename extension is left as-uploaded (Content-Type is authoritative for serving).
- **WebP → JPEG** loses alpha for the rare transparent-WebP upload, and animated WebP would fail to decode (→ rejected). Acceptable for now; revisit if WebP uploads matter.
- **Recommended larger initiative (separate issue):** a proper transcode-on-ingest pipeline via libvips/`govips` emitting **WebP/AVIF** (universal browser support, ~30% smaller than JPEG), with a decision on whether to retain originals in cold storage. This PR's stdlib re-encode solves privacy + web-compat now without the CGO system dependency; the libvips pipeline is the efficiency/quality upgrade.

## Phase 3 — metadata-retention decision (supersedes the strip-by-default approach)

**Decision:** we retain EXIF metadata by default. The trust level in every Ripls
sharing flow makes location/device leakage an accepted risk, so stripping is no
longer the default behavior.

- `StoreMedia` gained a `stripMetadata bool` parameter. When **false (the
  default at every caller)** the original bytes are stored verbatim — EXIF,
  GPS, device, and timestamps all retained. When **true**, the upload runs
  through `SanitizeImageForStorage` (strip + transcode) as described above.
- The sanitization code path and its tests are kept and exercise **both**
  directions: `TestStoreMedia_StripsImageMetadata` (strip on) and
  `TestStoreMedia_RetainsMetadataByDefault` (strip off, bytes + EXIF preserved),
  plus the direct `SanitizeImageForStorage` unit tests.
- **Web compatibility is decoupled from stripping** (#2210, folded into this PR).
  `StoreMedia` always runs `TranscodeForWebIfNeeded` for images regardless of the
  `stripMetadata` flag: web-native formats (jpeg/png/webp/gif) pass through
  unchanged (metadata retained), while HEIC/TIFF are transcoded to JPEG so every
  upload renders on the web client. The opt-in strip path (`SanitizeImageForStorage`)
  is a superset that also re-encodes the web-native formats. AVIF/WebP output and a
  metadata-preserving transcode remain the deferred libvips work in **#2210**.
- `SuggestGearLocation` is no longer "dead due to stripping" — with EXIF retained
  it is revivable as a feature; the wire-up-or-remove decision is tracked in
  **#2209**.
