# Issue #2210: Transcode images on ingest for web compatibility + efficiency (HEIC/TIFF → WebP/AVIF)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2210
**Filed:** 2026-05-27 | **Last activity:** 2026-05-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/issues/1953-media-upload-sanitization.md

## Problem Statement

The web client can't render **HEIC** (Safari-only; Chrome/Firefox/Edge — ~95% of users — show a broken image) or **TIFF**. After #1953's metadata-retention decision, uploads are stored **as-is by default**, so an iPhone HEIC upload currently renders as a broken image on the web.

The maintainer's refined requirement for the current PR (#2191): **all uploaded media must be renderable on the web client**, so non-web-renderable formats must **always** be transcoded on ingest — and this must be **decoupled from the now-default-off EXIF stripping** (we retain metadata for web-native formats). So:

- **Web-compat transcode = always on** (HEIC/TIFF → a web format).
- **Metadata stripping = opt-in, default off** (the `stripMetadata` flag added in #1953; unchanged).

The original issue also proposed a heavier libvips/WebP/AVIF efficiency pipeline. This plan separates the **must-ship-now web-compat transcode** from that **later efficiency upgrade**.

## Context from Comments

No issue comments yet (filed 2026-05-27 during #1953). Direction comes from the maintainer and the #1953 context doc:

- #1953 made `storage.StoreMedia` take a `stripMetadata bool` (default false at all callers) and added `storage.SanitizeImageForStorage` (stdlib decode → bake orientation → re-encode; transcodes HEIC/TIFF→JPEG, PNG→PNG, else→JPEG) behind that flag.
- Retention decision: EXIF/GPS is kept by default because the trust level in every sharing flow makes leakage acceptable.
- This turn: "I still want HEIC to reencode so that all uploaded media is renderable on the web client." Favor the option that ships web-compat **in this PR with minimal risk**.
- `goheif.ExtractExif(io.ReaderAt) ([]byte, error)` exists (verified), so HEIC EXIF *can* be extracted; `disintegration/imaging` can encode JPEG/PNG/TIFF/GIF/BMP but **not WebP** (no Go WebP encoder in deps).

## Potential Solutions

### Option A: stdlib web-compat transcode now (recommended for this PR)
- **Approach:** Add `storage.TranscodeForWebIfNeeded(data, contentType)`: web-native inputs (jpeg/png/webp/gif) pass through **untouched** (bytes + EXIF retained); HEIC/HEIF/TIFF are decoded, orientation baked into pixels, and re-encoded to JPEG. Wire it into `StoreMedia` to run **always** for images (independent of `stripMetadata`). Keep `SanitizeImageForStorage` as the opt-in strip path (re-encodes everything). No new dependency; reuses the registered decoders + `imaging`.
- **Pros:** Ships web-compat in #2191 with no system dependency or storage-format change. Web-native formats keep their bytes and EXIF (honors retention). Reuses existing orientation/encode helpers. Low blast radius (only HEIC/TIFF bytes change).
- **Cons:** Transcoded formats (HEIC/TIFF) lose their metadata in the stdlib re-encode unless we splice EXIF back (see Open Questions — has an orientation double-rotation pitfall). JPEG output leaves compression on the table vs WebP/AVIF.
- **Effort:** Small–medium.

### Option B: libvips / `govips` → WebP/AVIF pipeline (defer to a later PR)
- **Approach:** Transcode every image on ingest via libvips (`vips.NewImageFromBuffer` → `AutoRotate` → `ExportWebp`/AVIF), preserving or stripping metadata as configured. Store a single web-native format. AVIF/WebP cannot be encoded by the Go stdlib — they require a native lib — so this is the *only* path to those formats.
- **Pros:** One library handles all input formats; **preserves metadata natively** (solves the HEIC-EXIF problem for free); WebP ≈30% smaller than JPEG, universal browser support; AVIF smaller still. Industry-standard (imgproxy/bimg/Cloudflare).
- **Cons:** CGO + a native libvips/libheif dependency; storage/serving format change; needs Docker-build + CI verification. **How libvips is obtained matters a lot** (see below).

#### How to obtain libvips (measured on `debian:trixie-slim`, arm64)

**We already decode HEIC today** via `github.com/adrium/goheif` (registered in `server/storage/media.go`), which vendors libde265 and compiles it via CGO — no system lib, working because `CGO_ENABLED=1`. So Phase 1's stdlib HEIC→JPEG transcode needs nothing new, and **libvips is NOT required to *enable* HEIC decode**. Phase 2's value is WebP/AVIF *output* + metadata-preserving transcode.

That said, **if a libvips pipeline were to *replace* goheif as the decoder**, HEVC-decode availability becomes load-bearing again (it's patent-encumbered and the codec most lean/prebuilt libvips drop). The alternative is to keep goheif for HEIC decode and use libvips only for WebP/AVIF encode. The table below assumes libvips owns the full pipeline:

| Approach | Runtime footprint | ImageMagick/poppler/librsvg | HEIC (HEVC) decode | Verdict |
|---|---:|---|---|---|
| Stock `apt install libvips42t64` | **+97 MB** (~150 pkgs) | **pulled in** (hard `Depends`, can't trim) | **Yes** (libde265) | Works, but worse CVE surface than the decoders it replaces |
| Vendor `@img/sharp-libvips` prebuilt | ~? (lean) | gone | **NO** — sharp omits HEVC for patent reasons (its prebuilt lists jpeg/png/webp/avif/tiff/gif/svg, **no HEIC**) | **Rejected** — can't open the HEIC we need to transcode (AVIF *output* only) |
| **Source-build lean libvips (preferred)** | **~15–20 MB** | **gone** | **Yes** — we explicitly enable libheif + libde265 | meson `-Dmagick/poppler/rsvg/openslide/cfitsio/matio/openexr=disabled`, keep jpeg/png/webp/tiff + heif(+de265 for HEIC, +aom for AVIF). **First-ever source build** in the server image; we own libvips/libheif CVE tracking |
| Alpine `vips` apk | leaner | partial | yes | **Rejected** — Alpine is musl; our glibc base + prebuilt onnxruntime require glibc |

The droppable loaders (ImageMagick + poppler + rsvg + openexr + openslide + cfitsio + matio) measured at **105 MB** and are the CVE-prone part; the loaders we actually need (jpeg/png/webp/tiff/heif+de265) are only **12 MB**. So a lean libvips is **~5× smaller than the stock install and sheds the heavy surface** — comparable to the onnxruntime footprint we already carry.

- **Preferred procurement: source-build a lean libvips** with libheif+libde265 (HEIC decode) and aom (AVIF encode). It's the only lean option that decodes HEIC; prebuilt lean libvips (sharp) drops HEVC, and stock apt is heavy + CVE-prone.
- **HEVC patent note:** decoding HEVC server-side (libde265) is patent-encumbered. Debian ships it and it's widely used for server-side decode; sharp avoids it for *distribution*. For an internal decode-only server it's a generally-accepted risk, but worth a conscious sign-off.
- **Effort:** Large (source-build is the realistic lean path; the easy prebuilt path doesn't cover HEIC).

### Option C: client-side transcode
- **Approach:** Have the Flutter clients convert HEIC before upload.
- **Pros:** No server CPU.
- **Cons:** Unreliable across platforms; doesn't fix media arriving via other paths (webpage og:image, stock, existing data); pushes work to every client. Doesn't satisfy "all uploaded media renderable." **Rejected.**

## Recommendation

**Option A now, Option B later.** Ship the stdlib web-compat transcode in #2191 — it makes every upload renderable on the web with no new infrastructure and preserves EXIF for the web-native majority. Track the WebP/AVIF efficiency pipeline (Option B) as the future of this issue; libvips also retroactively fixes the HEIC-metadata gap.

On metadata for the **transcoded** formats: **accept the loss for HEIC/TIFF in this PR** (see Open Questions) rather than build a fragile EXIF-splice. It only affects formats that *must* be re-encoded to render at all, the retention policy still holds for everything web-native, and Option B restores it properly.

## Open Questions

- **Preserve HEIC EXIF across the transcode, or accept the loss?** — Recommendation: **accept the loss in this PR.** Splicing `goheif.ExtractExif` output into the output JPEG's APP1 segment is possible, but it collides with orientation: we bake ISOBMFF/EXIF rotation into the pixels, so a spliced EXIF carrying an `Orientation` tag would make clients rotate **again** (double-rotation). Doing it safely means editing the EXIF to normalize `Orientation=1` — fragile for a minimal-risk PR. Defer correct metadata-preserving transcode to the libvips path (Option B), which handles it natively.
- **Should `TranscodeForWebIfNeeded` run for stock-provider images too?** — Recommendation: **yes, unconditionally** — it's a no-op for the JPEG/MP4 that providers return, so running it for all images (stock or not) is simplest and guarantees the "everything renders" invariant without a special case.
- **How do transcode-always-on and strip-opt-in compose in `StoreMedia`?** — Recommendation: if `stripMetadata` is set (and non-stock), run `SanitizeImageForStorage` (strip + transcode all); otherwise run `TranscodeForWebIfNeeded` (transcode only the non-web formats, retain the rest). The strip path is a superset, so they don't both run.
- **Output format — JPEG now or jump straight to WebP?** — Recommendation: **JPEG now** (no Go WebP encoder in deps; adding one means libvips = Option B). WebP/AVIF is the Option B payoff.
- **Originals retention for transcoded formats?** — Recommendation: **discard** the original HEIC/TIFF (store only the JPEG) for now; web-native originals are kept as-is. Revisit a cold-storage-originals policy with Option B.
- **Quality:** reuse #1953's uniform `sanitizeJPEGQuality` (q90). Recommendation: yes, keep it consistent.

## Conventions to follow (per server architecture & observability docs)

- **File placement / size.** `TranscodeForWebIfNeeded` goes in `server/storage/media_sanitize.go` alongside `SanitizeImageForStorage` (shared decode/orientation/encode helpers), not a new file and not piling onto `media.go` (already ~490 lines, <1000-line convention).
- **Error codes + fail-closed.** A decode/encode failure on a validated image is rejected (never store the original). Surface it the same way the existing sanitize path does: `StoreMedia` returns the wrapped error, and `AddMedia` maps non-sentinel errors via `connecterr.Internal(ctx, "AddMedia", err, ...)` (generic public message; bare `connect.NewError(CodeInternal, err)` is gated by `scripts/check_codeinternal.js`). A malformed-but-validated image is an edge case; `CodeInternal` is acceptable. Do **not** add a new user-facing sentinel unless we want `CodeInvalidArgument` here.
- **Logging.** Log a transcode at `Debug`, and a transcode rejection at `Warn` (user-input class, never `Error`), with `operation`, `user_id`, `content_type`, `output_content_type`, `size_bytes` — and **never the image bytes or decoded EXIF**. Reuse `logging.LoggerWithContext(ctx)`.
- **Tests.** `storage.SetupTestStorage(t)` + the bucket mock; reuse `server/test_data/books_exif.jpg`. Assert the HEIC→JPEG output decodes and that web-native inputs are byte-identical (EXIF retained), mirroring the #1953 sanitize tests.

## Implementation Plan

### Phase 1: stdlib web-compat transcode (this PR / #2191)
**Goal:** Every stored image is renderable on the web client, while web-native formats keep their original bytes and EXIF.
**Tasks:**
- [ ] Add `storage.TranscodeForWebIfNeeded(data []byte, contentType string) ([]byte, string, error)`: pass through jpeg/png/webp/gif unchanged; decode → bake orientation (EXIF for tiff, ISOBMFF for heic via existing helpers) → encode JPEG for heic/heif/tiff. Fail closed.
- [ ] Wire into `StoreMedia`: for images, run `SanitizeImageForStorage` when `stripMetadata` (non-stock), else run `TranscodeForWebIfNeeded` (always — including stock, where it's a no-op). Update `media.ContentType`/`SizeBytes` from the result.
- [ ] Tests: HEIC fixture → output is decodable JPEG with `image/jpeg` content type; web-native JPEG/PNG stored byte-for-byte with EXIF retained (`books_exif.jpg` round-trips unchanged); TIFF → JPEG; orientation baked correctly (no double-rotation); strip path still strips; fail-closed on undecodable input.
- [ ] Doc comment + plan/PR note that transcoded formats lose metadata (tracked for Option B), and update the #1953 plan/PR to reflect "transcode always on, strip opt-in."
**Validates:** All uploads render on web; retention holds for web-native formats; no new dependency.

### Phase 2: libvips/WebP-AVIF efficiency pipeline (separate, future PR)
**Goal:** Smaller files, universal format, metadata-preserving transcode.
**Tasks:**
- [ ] Source-build a **lean** libvips (meson, loaders disabled per the table; keep jpeg/png/webp/tiff + **libheif with libde265 for HEIC decode and aom for AVIF encode**) in the Docker builder; copy the lean `.so` set into the runtime image. Wire `govips` (CGO) with `Startup`/`Shutdown` lifecycle. Add the build deps to CI so the package compiles there. (Prebuilt lean libvips — e.g. `@img/sharp-libvips` — is **not** viable: it omits HEVC/HEIC decode.)
- [ ] Get a conscious sign-off on the HEVC-decode patent posture (libde265, decode-only, internal server).
- [ ] Replace the stdlib transcode with a libvips pipeline emitting WebP (and/or AVIF with a `<picture>` fallback), auto-rotate, metadata policy aligned with #1953 retention (libvips preserves metadata across transcode — fixes the HEIC-EXIF loss the stdlib path accepts).
- [ ] Decide + implement originals retention (discard vs cold storage).
- [ ] Re-measure size/CPU; migrate or leave existing JPEGs as-is.
**Validates:** Bandwidth/storage win; HEIC metadata preserved end-to-end; lean image (~15–20 MB libvips, no ImageMagick/poppler/rsvg).

## Related Issues
- #1953 — parent PR (#2191); added `StoreMedia(stripMetadata)` + `SanitizeImageForStorage`. This issue's Phase 1 lands in that PR.
- #2209 — `SuggestGearLocation` wire-up-or-remove; depends on EXIF being present (it is, for web-native formats).

## Audit Log

Reviewed against `docs/server/architecture.md` and `docs/server/observability.md`. Client docs (caching/i18n/a11y/testing) are N/A — server-only change; "web client" refers to what renders, not a client code change. Changes applied:

- **Added a Conventions section:** file placement (`media_sanitize.go`, file-size convention), error-code/fail-closed handling (`connecterr.Internal`, codeinternal lint gate), logging levels (transcode at Debug, rejection at Warn, never Error; no bytes/EXIF in logs), and the storage-test pattern.
- Confirmed the design respects service independence (logic stays in the `storage` shared library; no service-to-service calls) and the single-chokepoint pattern (`StoreMedia`).
