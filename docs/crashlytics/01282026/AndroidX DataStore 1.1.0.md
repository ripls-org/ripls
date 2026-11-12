# Bug Analysis: DataStore NativeSharedCounter UnsatisfiedLinkError

## Problem Summary

The production Android app (v0.1.4, build 3279) is crashing on startup with:

```
Fatal Exception: java.lang.UnsatisfiedLinkError: No implementation found for int P0.o0.d(int)
   at androidx.datastore.core.NativeSharedCounter.nativeTruncateFile(SharedCounter.android.kt)
```

**Crash Location:** [tmp/bugs/CustomThreadFactory.lambda$newThread$0.txt](../../tmp/bugs/CustomThreadFactory.lambda$newThread$0.txt)

**Issue ID:** `7ae008fcced0f98bb34270d08a831736`

## Root Cause Analysis

### What's Happening

1. **AndroidX DataStore 1.1.0+** introduced multiprocess support that requires native JNI code
2. The `NativeSharedCounter` class uses native methods (`nativeTruncateFile`, etc.) for file locking
3. On certain Android devices/configurations, the native library (`libdatastore_shared_counter.so`) fails to load
4. The obfuscated method `P0.o0.d` is the ProGuard-renamed version of `nativeTruncateFile`

### Dependency Chain

```
shared_preferences: 2.5.3
    └── shared_preferences_android: 2.4.18
            └── AndroidX DataStore Preferences (DataStore 1.1.x)
                    └── DataStore Core (with MultiProcessCoordinator)
                            └── NativeSharedCounter (JNI native library)
```

### Why It Happens

The crash occurs because:

1. `shared_preferences_android` 2.4.x migrated from SharedPreferences to DataStore for storage
2. DataStore 1.1.x includes multiprocess coordination with native code for file locking
3. On some devices/builds, the native library is either:
   - Not properly bundled in the APK
   - Stripped by ProGuard/R8 despite keep rules
   - Incompatible with the device's CPU architecture
   - Failing due to a library extraction issue

### Existing Mitigation (Not Sufficient)

The app already has ProGuard rules in [app/android/app/proguard-rules.pro](../../app/android/app/proguard-rules.pro):

```proguard
-keep class androidx.datastore.core.NativeSharedCounter { *; }
-keep class androidx.datastore.core.SharedCounter { *; }
-keepclassmembers class androidx.datastore.** { native <methods>; }
```

These rules prevent obfuscation of the Java classes, but the native library linking failure persists. This suggests the issue is at the native library level, not Java obfuscation.

## Potential Solutions

### Solution 1: Pin DataStore to Pre-Native Version (Recommended)

Force a dependency override to use DataStore 1.0.0 which doesn't have multiprocess native code:

**In `app/android/app/build.gradle.kts`:**

```kotlin
configurations.all {
    resolutionStrategy {
        force("androidx.datastore:datastore-core:1.0.0")
        force("androidx.datastore:datastore-preferences:1.0.0")
        force("androidx.datastore:datastore-preferences-core:1.0.0")
    }
}
```

**Pros:**
- Immediate fix
- No code changes required
- Well-tested approach (DataStore 1.0.0 is stable)

**Cons:**
- Blocks multiprocess DataStore features (not currently needed)
- Will need revisiting when upgrading shared_preferences

### Solution 2: Downgrade shared_preferences_android

Pin to an older version that uses SharedPreferences instead of DataStore:

**In `pubspec.yaml`:**

```yaml
dependency_overrides:
  shared_preferences_android: 2.2.3  # Last version using SharedPreferences
```

**Pros:**
- Avoids DataStore entirely
- Simpler storage mechanism

**Cons:**
- Loses DataStore benefits (async, type-safety)
- Diverges from Flutter team's direction
- May have breaking API changes

### Solution 3: Add NDK Architecture Filtering

Ensure all required ABI splits are included. Check if the APK includes all architectures:

**In `app/android/app/build.gradle.kts`:**

```kotlin
android {
    defaultConfig {
        ndk {
            abiFilters += listOf("armeabi-v7a", "arm64-v8a", "x86", "x86_64")
        }
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true  // Extract native libs instead of loading from APK
        }
    }
}
```

**Pros:**
- Fixes devices where lib extraction is the issue
- Maintains latest dependencies

**Cons:**
- Increases APK size
- May not fix all devices
- Doesn't address root cause if lib isn't bundled

### Solution 4: Migrate Away from shared_preferences

Replace `shared_preferences` with a different storage solution:

- **Stash with disk persistence** (already using Stash for in-memory caching)
- **Hive** - Pure Dart, no native code
- **Isar** - Modern database, but has native code too

**Pros:**
- Removes DataStore dependency entirely
- Aligns with existing Stash caching architecture

**Cons:**
- Significant migration effort
- Need to migrate existing SharedPreferences data
- More testing required

### Solution 5: Try/Catch with Fallback

Wrap DataStore initialization with error handling and fallback to SharedPreferences:

This requires modifying how shared_preferences is initialized, which is controlled by the plugin itself. Not easily customizable.

**Verdict:** Not practical for this use case.

## Recommendation

**Implement Solution 1 (Force DataStore 1.0.0)** as an immediate fix:

1. It's the least invasive change
2. Directly addresses the root cause (native library issues)
3. Can be done in a single file with minimal risk
4. DataStore 1.0.0 is well-tested and stable
5. The app doesn't need multiprocess DataStore features

## Open Questions

1. **Which devices are affected?** The crash report shows Android 11 (SDK 30) based on thread stack traces. Is this device-specific or ABI-specific?

2. **Is this a regression?** Was this working in a previous build? Check if a recent dependency update introduced this.

3. **What's the crash frequency?** Is this affecting a significant percentage of users or edge cases?

4. **Are there other packages using DataStore?** Firebase packages also use DataStore. Check if Firebase is contributing to the issue.

5. **Is the native library present in the APK?** Inspect the release APK to verify `libdatastore_shared_counter.so` is bundled for all architectures.

## Phased Implementation Plan

### Phase 1: Immediate Hotfix (Priority: Critical) ✅ COMPLETED

**Goal:** Stop crashes in production

**Status:** Implemented on 2026-01-28 (commit e08b3b76)

**Actions:**
1. ✅ Add DataStore version override to `build.gradle.kts`
2. ⏳ Build release APK and verify fix locally
3. ⏳ Test on devices known to crash
4. ⏳ Release hotfix (v0.1.5)

**Files modified:**
- [app/android/app/build.gradle.kts](../../app/android/app/build.gradle.kts) - Added dependency resolution strategy

**Implementation:**

Added the following to `build.gradle.kts` before the `android {}` block:

```kotlin
// Force DataStore 1.0.0 to avoid native library crashes in 1.1.x
// See: docs/crashlytics/AndroidX DataStore 1.1.0.md for detailed analysis
// The 1.1.x versions use native JNI code that fails to load on some devices
configurations.all {
    resolutionStrategy {
        force("androidx.datastore:datastore-core:1.0.0")
        force("androidx.datastore:datastore-preferences:1.0.0")
        force("androidx.datastore:datastore-preferences-core:1.0.0")
    }
}
```

**Verification:**

Confirmed that DataStore dependencies are now forced to version 1.0.0:
```bash
cd app/android
./gradlew :app:dependencies | grep datastore
# Output shows: androidx.datastore:datastore-preferences:1.1.7 -> 1.0.0
# Output shows: androidx.datastore:datastore-core:1.1.7 -> 1.0.0
```

**Next steps:**
- Build and test release APK
- Deploy hotfix to production

### Phase 2: Investigation (Priority: High)

**Goal:** Understand scope and gather data

**Actions:**
1. Query Crashlytics for affected device models and Android versions
2. Inspect APK contents to verify native libraries are bundled
3. Check Firebase console for related errors
4. Review recent dependency changes that might have triggered this

**Commands:**
```bash
# Inspect APK contents
unzip -l app/build/outputs/apk/release/app-release.apk | grep ".so"

# Check for DataStore native libs
unzip -p app/build/outputs/apk/release/app-release.apk lib/*/libdatastore*.so | od -c | head
```

### Phase 3: Long-term Solution (Priority: Medium)

**Goal:** Sustainable dependency management

**Actions:**
1. Add dependency version monitoring to CI/CD
2. Document the DataStore version constraint and why it exists
3. Create a test case that verifies DataStore initialization works
4. Set up alerts for dependency updates that might reintroduce this issue

**Considerations for future:**
- When upgrading shared_preferences major versions, re-evaluate DataStore compatibility
- Consider migrating to a pure-Dart storage solution if DataStore continues to have issues
- Monitor AndroidX DataStore issue tracker for fixes to the native library loading

### Phase 4: Optional Migration (Priority: Low)

**Goal:** Remove DataStore dependency entirely

**If DataStore issues persist, consider:**
1. Migrating to Hive for local storage (pure Dart, no native code)
2. Extending Stash with disk persistence for unified caching/storage
3. Using SQLite via drift/sqflite if structured storage is needed

**Trade-offs:**
- More work than version pinning
- Requires data migration for existing users
- Need to handle upgrade path carefully

## Related Issues

- [AndroidX DataStore Issue #283](https://issuetracker.google.com/issues/283765621) - UnsatisfiedLinkError on certain devices
- [Flutter shared_preferences Issue #744](https://github.com/flutter/packages/issues/744) - DataStore migration issues
- Similar reports in FlutterFire and other Firebase Flutter packages

## References

- [DataStore Multiprocess Documentation](https://developer.android.com/topic/libraries/architecture/datastore#multiprocess)
- [shared_preferences_android Changelog](https://pub.dev/packages/shared_preferences_android/changelog)
- [ProGuard Native Methods](https://www.guardsquare.com/manual/configuration/examples#native)
- [Android NDK ABI Management](https://developer.android.com/ndk/guides/abis)

---

**Author:** Claude Code Analysis
**Date:** 2026-01-28
**Status:** Phase 1 completed (commit e08b3b76) - Awaiting release build and deployment
**Last Updated:** 2026-01-28
