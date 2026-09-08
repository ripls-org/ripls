# Keep DataStore native classes (JNI linkage breaks if obfuscated)
-keep class androidx.datastore.core.NativeSharedCounter { *; }
-keep class androidx.datastore.core.SharedCounter { *; }
-keep class androidx.datastore.core.SharedCounter$* { *; }
-keepclassmembers class androidx.datastore.** {
    native <methods>;
}

# Suppress warning for optional multi-process DataStore dependency
# (referenced by Firebase Sessions but not needed for single-process usage)
-dontwarn androidx.datastore.core.MultiProcessDataStoreFactory

# Keep androidx.window classes that are accessed via reflection
-keep class androidx.window.** { *; }
-dontwarn androidx.window.**

# Keep Window Extensions classes
-keep class androidx.window.extensions.** { *; }
-keep interface androidx.window.extensions.** { *; }
-dontwarn androidx.window.extensions.**

# Keep Window Sidecar classes
-keep class androidx.window.sidecar.** { *; }
-keep interface androidx.window.sidecar.** { *; }
-dontwarn androidx.window.sidecar.**
