# Issue #1262: Replace CopyStockImageForUser buffer-copy with GCS server-side CopierFrom

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1262
**Filed:** 2026-04-24 | **Last activity:** 2026-04-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`CopyStockImageForUser` materializes the full image (`io.ReadAll` into a `[]byte`) on
the Go heap before re-uploading it to a destination key in the same GCS bucket. With
8–16 concurrent nudge-imagery goroutines each handling 2–5 MB Unsplash images, the
transient heap cost is 50–100 MB on top of steady state. This caused the 2026-04-24
Cloud Run OOM kill (2076 MiB used against a 2048 MiB limit).

The root cause is that source and destination objects live in the same bucket, so the
server has no reason to see the bytes at all. GCS supports a native server-side copy
RPC (`(*ObjectHandle).CopierFrom(src).Run(ctx)`) that duplicates an object without
routing any payload through the caller.

The same pattern exists in `copyStockVideoThumbnail`, which also downloads and
re-uploads thumbnail bytes unnecessarily.

## Context from Comments

- **Triage (2026-04-25):** Confirmed P2 Task. Engineering lens — performance optimization.
- **@briangbrown (2026-04-26):** Requested fix.
- **Previous attempt (PR #1413, 2026-04-26):** An implementation was drafted but failed CI Go Tests. This plan targets the same fix on a fresh branch, with the CI failure root-caused before implementation.

## Potential Solutions

### Option A: Add `Copy` to `BucketStorage` and route through it (recommended)
- **Approach:** Add `Copy(ctx, srcKey, dstKey string) (storageURL string, size int64, err error)` to the `BucketStorage` interface. `GCSBucketStorage.Copy` uses `CopierFrom`; `LocalBucketStorage.Copy` uses `os.Link` with `io.Copy` fallback. Rewrite `CopyStockImageForUser` (and `copyStockVideoThumbnail`) to call `Copy`.
- **Pros:** Zero bytes transit through Cloud Run for same-bucket copies. Fixes every caller of the function, not just nudges. Clean interface extension.
- **Cons:** Adds a method to the interface — all mock implementations must be updated. Two-step DB write (insert then update after copy).
- **Effort:** Small.

### Option B: Stream via `io.Copy(writer, reader)` inside CopyStockImageForUser
- **Approach:** Keep bytes flowing through the server but never hold the full image in memory at once.
- **Pros:** Handles cross-bucket copies (e.g., if source and destination ever split to separate buckets).
- **Cons:** Bytes still transit through Cloud Run. Requires a chunked upload writer or streaming to avoid accumulating the full object in memory.
- **Effort:** Small–medium.

### Option C: Concurrency cap only (semaphore in fetchNudgeImagery)
- **Approach:** Limit parallel nudge image copies with a semaphore.
- **Pros:** Low-risk change; bounds worst-case memory growth.
- **Cons:** Does not fix the underlying per-copy waste. A separate issue already tracks this.
- **Effort:** Trivial.

## Recommendation

**Option A.** The source and destination are in the same bucket in all environments,
so the bytes never need to cross the server boundary. A server-side copy is strictly
better: it eliminates both the heap allocation and the egress bandwidth cost.
Option C is a complementary guard but not a substitute.

## Open Questions

- **Should `Copy` return `(string, int64, error)` or just `error`?**
  Recommendation: Return `(storageURL string, size int64, err error)`. The size is
  available for free from `CopierFrom` result attrs, and the DB record needs both
  values; returning them avoids a second `Attrs` call.

- **`LocalBucketStorage.Copy`: hard link or byte copy?**
  Recommendation: Try `os.Link` first (instant, zero disk cost on same filesystem).
  Fall back to byte-for-byte `io.Copy` for cross-device mounts or filesystems that
  don't support hard links. Copy the `.metadata` sidecar to preserve content-type.

- **Should the thumbnail copy be broadened from video-only to all media?**
  Recommendation: Yes. The guard `isVideoContentType(...)` was only needed because
  `StoreMedia` handled image thumbnails inline (via `generateThumbnail`). With the
  server-side copy path, the copied media record starts with no thumbnail, so copying
  the source thumbnail (if any) is always the right thing to do regardless of content type.

## Implementation Plan

### Phase 1: Extend BucketStorage interface and implementations
**Goal:** Add server-side copy capability without touching callers.
**Tasks:**
- [ ] Add `Copy(ctx, srcKey, dstKey string) (storageURL string, size int64, err error)` to `BucketStorage` interface in `server/storage/bucket.go`
- [ ] Implement `GCSBucketStorage.Copy` using `dst.CopierFrom(src).Run(ctx)`
- [ ] Implement `LocalBucketStorage.Copy` using `os.Link` with `io.Copy` fallback; copy `.metadata` sidecar
- [ ] Add `Copy` to `MockBucketStorage` in `server/services/test_util.go`
- [ ] Add `Copy` to `mockBucketStorage` in `server/jobs/impact_backfill_test.go`
- [ ] Add unit tests for both `GCSBucketStorage.Copy` and `LocalBucketStorage.Copy` in `server/storage/bucket_test.go`
**Validates:** `npm run build` clean; all existing tests still pass.

### Phase 2: Rewrite CopyStockImageForUser and copyStockVideoThumbnail
**Goal:** Eliminate byte materialization on the hot copy path.
**Tasks:**
- [ ] Rewrite `CopyStockImageForUser` to: insert DB record → `bucketStorage.Copy` → update record with URL + size
- [ ] Rename `copyStockVideoThumbnail` → `copyStockThumbnail` and route through `Copy`; remove `isVideoContentType` guard
- [ ] Add `noGetBucketStorage` test wrapper (panics if `Get` is called) to verify no bytes flow through the server
- [ ] Add `TestCopyStockImageForUser_UsesServerSideCopy` and `TestCopyStockImageForUser_WithThumbnail`
**Validates:** Tests pass; `noGetBucketStorage` wrapper doesn't panic; `npm run build` clean.

## Related Issues

- #1262 — This issue (OOM from per-copy heap cost)
- Concurrency cap on nudge imagery fan-out at `server/services/feed/nudges.go:152-157` — separate issue
- Unbounded `io.ReadAll` in stock-provider downloads — separate issue
