# Issue #1449: Heap pressure: video thumbnail generation loads entire video into []byte before ffmpeg reads it from disk

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1449
**Filed:** 2026-04-27 | **Last activity:** 2026-04-29
**Labels:** Bug, P1 (set via triage 2026-04-28)
**Assignees:** unassigned
**Context docs:** none

## Problem Statement

`BackfillVideoThumbnails` (`server/storage/media.go:164`) and the inline thumbnail
path inside `StoreMedia` (`server/storage/media.go:387`) both materialize the full
video as `[]byte` on the Go heap, even though `generateVideoThumbnail` immediately
writes those bytes back to disk for ffmpeg to read. The current flow:

1. `bucketStorage.Get(ctx, bucketKey)` returns `[]byte` (the entire video on the
   heap, `media.go:186`).
2. The slice is passed into `generateVideoThumbnail`.
3. Inside `generateVideoThumbnail`: `os.WriteFile(inputPath, data, 0o600)` —
   bytes briefly live in **both** heap and disk (`media.go:97`).
4. ffmpeg reads from disk, writes the thumbnail to disk.
5. The thumbnail is read back into `[]byte` and `Put` to the bucket.

For 50–100 MB MP4s, step 1 alone is a 50–100 MB heap allocation per iteration.
The backfill loop processes videos serially without explicit GC pacing. This is
the same OOM shape as #1262 (fixed by #1415), but from a different angle: there
the bytes round-tripped through Cloud Run on their way back to GCS; here they
round-trip through Cloud Run heap on their way to local ffmpeg.

The function signature `generateVideoThumbnail(ctx, data []byte) ([]byte, error)`
forces every caller to materialize the full video before invoking ffmpeg, even
though ffmpeg never wants the bytes — it wants a path to read from.

**Severity context (from issue body).** No production OOM has been attributed
to this path yet. `BackfillVideoThumbnails` only runs when triggered as a
background job (not on the request path), and the inline `StoreMedia` call only
runs on user-uploaded videos (typically smaller than the stock-video corpus).
Filed as a follow-up so it doesn't get lost; should ship before the next
memory-related Cloud Run incident on the backfill job, not as an emergency.
Triaged P1 / Bug on 2026-04-28.

## Context from Comments

- **Triage update — 2026-04-28 (maintainer):** Issue Type → Bug, Priority →
  P1. Real OOM risk; legacy `bug` label stripped.
- **2026-04-29 (maintainer):** `@claude` request to act on the issue.

The issue body itself is unusually thorough: it lays out the affected callsites,
ranks three solution options, recommends Option A, and flags a known integration-test
mock trap. It even names two open questions and provides recommended answers.
This plan adopts that recommendation and expands it into a phased implementation,
keeping the original issue's reasoning intact.

## Potential Solutions

### Option A: Add streaming `GetToFile` / `PutFromFile` to `BucketStorage` (recommended)

- **Approach:** Extend the `BucketStorage` interface in
  `server/storage/bucket.go` with two streaming methods:

    ```go
    // GetToFile streams an object's bytes directly to a local path,
    // returning the object's content type. No []byte materialises in the
    // caller's heap.
    GetToFile(ctx context.Context, key, destPath string) (contentType string, err error)

    // PutFromFile streams a local file's bytes directly to bucket storage,
    // returning the permanent storage URL. No []byte materialises in the
    // caller's heap.
    PutFromFile(ctx context.Context, key, srcPath, contentType string, metadata map[string]string) (storageURL string, err error)
    ```

  Implement on `GCSBucketStorage` via `obj.NewReader(ctx)` + `io.Copy` to file
  (and `obj.NewWriter(ctx)` + `io.Copy` from file). Implement on
  `LocalBucketStorage` via `os.Link` with `io.Copy` fallback (mirrors the
  existing `Copy` implementation), including the `.metadata` sidecar.

  Refactor `generateVideoThumbnail` to take an input file path:

    ```go
    func generateVideoThumbnail(ctx context.Context, inputPath string) (thumbnailPath string, err error)
    ```

  Update `BackfillVideoThumbnails`: `bucket.GetToFile` → `generateVideoThumbnail(inputPath)`
  → `bucket.PutFromFile`. Update `StoreMedia`: write the upload `data` to a
  temp file once, drop the `data` reference (or accept that it lives until the
  outer caller drops it — see open question), invoke `generateVideoThumbnail`
  on the temp path, then `PutFromFile` the resulting thumbnail.

- **Pros:** Removes the 50–100 MB heap spike on the backfill path entirely.
  Symmetric with the `Copy` interface extension landed in #1415, so
  `BucketStorage` keeps a coherent shape ("server-side or stream-based byte
  ops, no `[]byte` round-trips"). One place to look when adding future
  subprocess integrations (image transcoding, virus scanning, etc.).
- **Cons:** Adds two methods to the interface — every mock must be updated.
  We just learned this lesson with `Copy` in #1415: the integration mock at
  `server/jobs/impact_backfill_integration_test.go:215` was missed and only
  caught when CI ran with `-tags=integration`. Need to update **all** mocks
  including the integration-only one.
- **Effort:** Medium.

### Option B: Use signed URL + ffmpeg HTTP input directly

- **Approach:** Have `BackfillVideoThumbnails` call `bucket.GetSignedURL` and
  pass the URL as ffmpeg's `-i` argument. ffmpeg supports HTTP input natively
  and seeks via HTTP range reads.

    ```go
    url, err := bucketStorage.GetSignedURL(ctx, bucketKey, 5*time.Minute)
    cmd := exec.Command("ffmpeg", "-i", url, ...)
    ```

- **Pros:** Smallest diff — no interface change, no temp file write, no heap
  allocation on the backfill path.
- **Cons:** Adds an outbound HTTP dependency from Cloud Run to GCS for an
  operation that's purely local today. ffmpeg's HTTP behaviour under Cloud Run
  egress + GCS auth is not well-tested in this codebase (signed URLs require
  the credentialed bucket signer to be reachable from Cloud Run; behaviour
  under transient network errors is opaque). Doesn't help `StoreMedia` (the
  inline path already has the bytes in heap from the upload).
- **Effort:** Small for backfill; doesn't apply to the inline upload path.

### Option C: Pass an `io.Reader` through `generateVideoThumbnail`

- **Approach:** Change the signature to
  `generateVideoThumbnail(ctx context.Context, src io.Reader) ([]byte, error)`
  and have the function spool to its own temp file. Backfill passes a bucket
  reader; `StoreMedia` passes `bytes.NewReader(data)`.
- **Pros:** Modest interface change. Helps backfill.
- **Cons:** Doesn't help the inline path at all (`bytes.NewReader(data)` keeps
  the full slice live until the function returns). Half-measure relative to
  Option A. Caller still has to know to use a streaming source on the backfill
  side. Doesn't generalize to other future subprocess integrations.
- **Effort:** Small.

## Recommendation

**Option A.** The recommended approach matches the issue body's recommendation.
Two reasons:

1. **It generalizes.** `GetToFile` / `PutFromFile` will help any future code
   that needs to feed bucket bytes through a subprocess (image transcoding,
   AV scanning, structured-output extraction) without round-tripping through
   the heap. We're already adding subprocess steps over time; getting the
   primitive right once is cheaper than the same workaround three times.

2. **It keeps `BucketStorage` coherent.** With `Copy` (from #1415) plus
   `GetToFile` / `PutFromFile`, the interface tells a clean story:
   server-side or stream-based byte ops are first-class; full `[]byte`
   round-trips through the caller (`Get` / `Put`) exist for small payloads
   only. Option B makes ffmpeg-over-HTTP a side-channel that bypasses the
   abstraction.

The cost is real: every mock in the codebase needs the two new methods. The
issue body specifically calls out
`server/jobs/impact_backfill_integration_test.go:215` (the `testImageBucket`)
as the trap that bit #1415. That file is gated behind `//go:build integration`,
so `npm run build` won't catch a missing implementation — only
`go build -tags=integration ./...` will. The implementation phase below makes
that mock update an explicit step and runs the tagged build before claiming
done.

## Open Questions

- **Does ffmpeg need random access for keyframe seeks at `-ss 00:00:01`?**
  Recommendation: yes, for accurate seeks; that's why the current code writes
  to disk before invoking ffmpeg. Option A's `GetToFile` preserves that
  property by streaming to a regular file on disk before ffmpeg runs. (Option
  B would rely on GCS range-read seeks; not validated here.) No action needed
  beyond preserving the disk-backed input.

- **Should `StoreMedia` also write the upload to disk first instead of
  holding it in heap during ffmpeg?**
  Recommendation: yes, for symmetry and to drop the `[]byte` reference as
  early as possible. The upload path holds `data []byte` while waiting for
  ffmpeg; spooling to disk and dropping the reference shortens the heap
  lifetime. Cheap addition once Option A's primitive is in place. Note the
  caller of `StoreMedia` still owns the `data []byte` it passed in, so the
  win is "stop ffmpeg from extending the lifetime", not "free the upload
  buffer entirely". That's still worth doing.

- **Should `generateVideoThumbnail` return a path or `[]byte`?**
  Recommendation: return a path (`thumbnailPath string`). The caller then
  uses `bucket.PutFromFile(thumbnailPath, ...)` to upload and unlinks the
  temp file. This is the thing that closes the heap-pressure loop: if the
  function returns `[]byte`, we re-read the JPEG into heap before uploading,
  defeating the point. The temp directory cleanup that
  `generateVideoThumbnail` does today (`defer os.RemoveAll(tmpDir)`) needs
  to be moved to the caller, since the caller now needs the file to live
  past the function return. Use a caller-managed `os.MkdirTemp` and
  `defer os.RemoveAll` pattern.

- **`PutFromFile` `contentType` arg or `nil`?**
  Recommendation: take `contentType` as an explicit string argument
  (mirrors `Put`). Don't try to sniff it from the file. Callers always know
  what type they're uploading at this layer — the thumbnail path knows it
  produced JPEG, the backfill knows the original content type from the
  media record.

## Implementation Plan

### Phase 1: Extend `BucketStorage` interface and implementations

**Goal:** Add streaming `GetToFile` / `PutFromFile` without touching callers
of `generateVideoThumbnail` yet. Phase 1 ships green on its own.

**Tasks:**
- [ ] Add `GetToFile(ctx, key, destPath string) (contentType string, err error)`
  and `PutFromFile(ctx, key, srcPath, contentType string, metadata map[string]string) (storageURL string, err error)`
  to the `BucketStorage` interface in `server/storage/bucket.go`.
- [ ] Implement `GCSBucketStorage.GetToFile`: open the file with
  `os.OpenFile(destPath, O_WRONLY|O_CREATE|O_TRUNC, 0o600)`, open
  `obj.NewReader(ctx)`, `io.Copy(file, reader)`, then `obj.Attrs(ctx)` for
  the content type. Be careful to close the reader, the writer, and propagate
  errors from each.
- [ ] Implement `GCSBucketStorage.PutFromFile`: open `obj.NewWriter(ctx)`, set
  `ContentType` and `Metadata`, open `srcPath` for reading, `io.Copy(writer, file)`,
  close writer, return `gs://bucket/key`.
- [ ] Implement `LocalBucketStorage.GetToFile`: prefer `os.Link` from
  `basePath/key` to `destPath`; fall back to `copyLocalFile` on cross-device
  errors. Read content type from `.metadata` sidecar (existing helper logic
  in `Get` — extract into a shared helper).
- [ ] Implement `LocalBucketStorage.PutFromFile`: ensure dest dir, prefer
  `os.Link` from `srcPath` to `basePath/key`; fall back to `copyLocalFile`.
  Write the `.metadata` sidecar using the same format as `Put`.
- [ ] Add `GetToFile` and `PutFromFile` to `MockBucketStorage` in
  `server/services/test_util.go` (write/read a temp file or stub return values
  matching the existing `Get`/`Put` mocks).
- [ ] Add `GetToFile` and `PutFromFile` to `mockBucketStorage` in
  `server/jobs/impact_backfill_test.go` (return `fmt.Errorf("not implemented")`
  matching the existing pattern; the backfill tests don't exercise these).
- [ ] **Add `GetToFile` and `PutFromFile` to `testImageBucket` in
  `server/jobs/impact_backfill_integration_test.go`** — this is the one behind
  `//go:build integration` that bit #1415; do not skip.
- [ ] Search for any other types implementing `BucketStorage` (`grep -rn
  "BucketStorage" --include="*.go" server/`) and update them. Compile-time
  check: `go build ./...` and `go build -tags=integration ./...` must both
  pass.
- [ ] Add unit tests in `server/storage/bucket_test.go` mirroring the existing
  `Copy` tests:
    - `TestGCSBucketStorage` → add `t.Run("GetToFile", ...)`,
      `t.Run("PutFromFile", ...)`, plus error cases (nonexistent source,
      unwritable dest).
    - `TestLocalBucketStorage` → same; verify hard-link path is taken on
      same-FS, byte-copy fallback works, `.metadata` sidecar is preserved.

**Validates:** `npm run build` clean, `go build -tags=integration ./...` clean,
`go test ./server/storage/...` green, no caller behaviour changed yet.

### Phase 2: Refactor `generateVideoThumbnail` and rewire callers

**Goal:** Eliminate the heap allocation on both video paths.

**Tasks:**
- [ ] Change the signature of `generateVideoThumbnail` in
  `server/storage/media.go:85` to
  `func generateVideoThumbnail(ctx context.Context, inputPath string) (thumbnailPath string, err error)`.
- [ ] Move the temp-directory creation responsibility to the caller. The
  function now receives a path to read from and writes the thumbnail into a
  caller-supplied or function-internal temp directory and returns its path.
  Pick one: function-internal temp dir + return path + caller cleanup is the
  simplest contract; document that the caller must `os.RemoveAll(filepath.Dir(thumbnailPath))`
  when done. (Alternative: caller passes both input and output dir. Pick
  whichever is least error-prone — document the choice in the function
  comment.)
- [ ] Drop the `bytes.Buffer` re-encode at the end of `generateVideoThumbnail`
  in favor of writing the resized thumbnail directly to a JPEG file inside
  the temp dir (use `imaging.Save` or `os.Create` + `imaging.Encode` on the
  file). The function never returns `[]byte` again.
- [ ] Rewrite `BackfillVideoThumbnails` (`server/storage/media.go:164`):
  for each candidate video, create a per-iteration temp dir
  (`os.MkdirTemp("", "ripls-video-backfill-*")`), `bucket.GetToFile` the
  video into it, `generateVideoThumbnail(ctx, inputPath)`, then
  `bucket.PutFromFile` the thumbnail. Use `defer os.RemoveAll(tmpDir)` per
  iteration so heap and disk both stay flat. Replace the `Get` + `Put` calls
  on lines 186 and 205.
- [ ] Rewrite the inline video thumbnail branch in `StoreMedia`
  (`server/storage/media.go:387`): write the upload `data` to a temp file
  once, set `data = nil` (or scope it tighter so the slice is collectable),
  call `generateVideoThumbnail(ctx, inputPath)`, then `bucket.PutFromFile`
  the result. Note the outer `StoreMedia` caller may still hold the `data`
  slice — the win here is bounded heap during ffmpeg, not freeing the
  upload entirely.
- [ ] Update `TestGenerateVideoThumbnail_FakeData` in
  `server/storage/media_test.go:537` to write fake bytes to a temp file and
  pass the path. Add a new `TestGenerateVideoThumbnail_SmallValidVideo` if
  there's a fixture available; otherwise stick with the failure-case test.
- [ ] Confirm `TestStoreMedia_ThumbnailGeneration_VideoFile`
  (`server/storage/media_test.go:378`) — the existing video thumbnail test
  in `StoreMedia` — still passes after the inline path is rewritten. Add a
  new `TestBackfillVideoThumbnails_NoHeapMaterialization` that exercises the
  backfill path against the local bucket storage backend with a small fake
  MP4 file and asserts the loop doesn't panic. (No `TestBackfillVideoThumbnails`
  exists today; the issue body's reference to it was aspirational.)

**Validates:** `npm run build` clean, `go build -tags=integration ./...`
clean, all tests green. Spot-check by reading the diff and confirming no
`Get(ctx, ...) ([]byte, ...)` call exists on either video path; both go
through `GetToFile`.

### Phase 3 (optional): Heap-flatness regression test

**Goal:** Make sure a future refactor doesn't reintroduce the spike.

**Tasks:**
- [ ] Reuse the existing `noGetBucketStorage` wrapper at
  `server/storage/media_test.go:786` (introduced for #1415). It already
  embeds `BucketStorage` and panics on `Get`. For the video path, add a
  parallel `noPutBucketStorage` (or extend the existing wrapper to panic on
  `Put` as well) so the regression test asserts neither byte-materializing
  primitive is called. Pass through `GetToFile` / `PutFromFile` to the
  embedded backend.
- [ ] Add `TestBackfillVideoThumbnails_NoHeapMaterialization` and
  `TestStoreMedia_VideoUsesStreamingPath` that wire the wrapper under the
  real local bucket and assert no panic. If either path regresses to `Get`
  or `Put`, the test fails loudly.

**Validates:** Tests fail if the heap-allocating path comes back; pass on
clean implementation.

## Acceptance criteria (from the issue body)

- `BackfillVideoThumbnails` no longer holds the full video in `[]byte` for
  any iteration. Profiling on a 200 MB sample confirms heap stays flat
  across iterations.
- `StoreMedia`'s video path discards the upload `[]byte` before ffmpeg runs
  (reference dropped after writing to temp file, before `exec.Command`).
- Mock implementations of `BucketStorage` updated everywhere, including the
  integration-test mock (`testImageBucket` at
  `server/jobs/impact_backfill_integration_test.go:215`).
- Existing thumbnail-generation tests
  (`TestGenerateVideoThumbnail_FakeData` and any
  `TestStoreMedia_VideoGeneratesThumbnail` / `TestBackfillVideoThumbnails`
  if present) continue to pass.
- Both `npm run build` and `go build -tags=integration ./...` pass.

## Related Issues

- #1262 — the stock-image variant of this same OOM shape; the original Cloud
  Run kill that motivated the streaming pattern.
- #1415 — added `Copy` to `BucketStorage`; this issue extends the interface
  in the same direction (server-side / stream-based byte ops, no `[]byte`
  round-trips). The integration-mock trap was first hit there and is called
  out explicitly in this plan.

## Implementation conventions to honor

These are not new requirements — they're the project conventions
(CLAUDE.md, `docs/server/architecture.md`, `docs/server/observability.md`)
the implementer must follow when executing the plan. Calling them out here
so they don't get lost in the diff.

- **Structured logging.** Every new error or warning must use
  `logging.LoggerWithContext(ctx)` (the package the file already uses). On
  the backfill path, preserve the existing `operation` /  `media_id` fields
  on the logger; add `bucket_key` where useful for cross-correlation. No
  `log.Printf` / `fmt.Printf`. Levels: `Debug` for per-iteration normal
  flow, `Warn` for recoverable per-video failures (matches today's behaviour),
  `Error` only for system failures that abort the whole backfill.

- **No silent error swallowing.** Per CLAUDE.md "Fail Fast": when temp-file
  cleanup (`os.RemoveAll`) fails, log a `Warn` with the path and error
  rather than discarding via blank-identifier assignment. The existing
  `defer os.RemoveAll(tmpDir)` in `generateVideoThumbnail` discards the
  error today; the new caller-managed temp dirs should do the same only
  if accompanied by a justifying comment, or upgrade to a logging deferred
  cleanup function.

- **Build verification.** `BucketStorage` is an exported interface used by
  integration-tagged tests. Every push must pass both `npm run build` *and*
  `go build -tags=integration ./...`. CLAUDE.md spells this out
  ("Also build with the `integration` tag when changing exported APIs"); the
  integration-mock trap mentioned in the recommendation section is exactly
  the case it warns about.

- **Test-first by layer.** `bucket_test.go` (storage layer) gets the
  primitive tests for `GetToFile` / `PutFromFile`; `media_test.go` keeps
  ownership of `generateVideoThumbnail` and the wired-up callers. Don't
  cross-test layers.

- **File size.** `server/storage/media.go` is ~550 lines today; the changes
  here add ~30–50 lines net. No split needed. If you find yourself adding
  more than ~150 lines, reconsider the design before pushing past the
  1000-line guideline.

- **No vendor names in proto comments.** N/A — this change does not touch
  `.proto` files. Mentioned for completeness so the implementer doesn't
  paste log strings from this plan into proto comments.

## Audit notes (post-plan review)

Audit ran 2026-04-29 against
`docs/server/architecture.md` and `docs/server/observability.md`. Findings
folded into the plan above; recording the deltas here for traceability.

- The issue body referenced `TestStoreMedia_VideoGeneratesThumbnail` and
  `TestBackfillVideoThumbnails` as if they exist. They don't. The actual
  existing video-path test is `TestStoreMedia_ThumbnailGeneration_VideoFile`
  at `server/storage/media_test.go:378`. Plan now names the real test.
- The `noGetBucketStorage` wrapper from #1415 already exists at
  `server/storage/media_test.go:786` and embeds `BucketStorage`. Phase 3
  now reuses it instead of describing it as a "similar pattern" to invent.
- Added an "Implementation conventions to honor" section codifying the
  project's structured-logging, error-handling, and build-verification
  rules so the executing agent doesn't have to re-derive them from
  CLAUDE.md.
- Verified `BucketStorage` lives at `server/storage/bucket.go` with eight
  current implementers across server/, services/, and jobs/ (real + test).
  The "find every implementer" step in Phase 1 reflects the actual surface,
  not a guess.
