import java.util.Properties
import java.io.FileInputStream
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// Get build number from git commit count for local builds
fun getGitCommitCount(): Int {
    return try {
        val process = ProcessBuilder("git", "rev-list", "--count", "HEAD")
            .directory(rootProject.projectDir)
            .redirectErrorStream(true)
            .start()
        process.inputStream.bufferedReader().readText().trim().toIntOrNull() ?: 1
    } catch (e: Exception) {
        1
    }
}

plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
    id("com.google.gms.google-services")
    id("com.google.firebase.crashlytics")
    id("com.google.firebase.firebase-perf")
}

// Load keystore properties
val keystorePropertiesFile = rootProject.file("key.properties")
val keystoreProperties = Properties()
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(FileInputStream(keystorePropertiesFile))
}

// Google Maps SDK API key (#2188). The Maps SDK reads this from the
// manifest meta-data at app launch; it can't be set later. Resolved once
// here and shared by the manifest-placeholder injection (defaultConfig)
// and the release-build guard (below). Source priority:
//   1. GOOGLE_MAPS_API_KEY environment variable — the canonical source.
//      The same var iOS reads; injected in CI from Secret Manager by
//      release_app.yaml and by the local `npm run start:app:*` and
//      `npm run build:app:android:*` wrappers.
//   2. Gradle property of the same name (CI lanes that prefer -P over env).
//   3. Empty string — tolerated for debug/profile builds so local
//      `flutter run` and CI test builds work without GCP credentials.
//      Release builds with an empty key are rejected by the task-graph
//      guard below; we never ship an app with a non-functional map.
val googleMapsApiKey: String =
    System.getenv("GOOGLE_MAPS_API_KEY")
        ?: (project.findProperty("GOOGLE_MAPS_API_KEY") as String?)
        ?: ""

// Force DataStore 1.0.0 to avoid native library crashes in 1.1.x
// See: docs/ai/bug1.md for detailed analysis
// The 1.1.x versions use native JNI code that fails to load on some devices
configurations.all {
    resolutionStrategy {
        force("androidx.datastore:datastore-core:1.2.1")
        force("androidx.datastore:datastore-preferences:1.2.1")
        force("androidx.datastore:datastore-preferences-core:1.2.1")
    }
}

android {
    namespace = "org.ripls.app"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    // AGP 9 disables the resValues build feature by default. The debug, release,
    // and profile build types below define app_name via resValue, so it must be
    // re-enabled explicitly or configuration fails.
    buildFeatures {
        resValues = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
        isCoreLibraryDesugaringEnabled = true
    }

    // Kotlin 2.x removed the kotlinOptions { jvmTarget = String } DSL.
    // The Kotlin Gradle Plugin requires the compilerOptions block with a
    // typed JvmTarget enum value.
    kotlin {
        compilerOptions {
            jvmTarget.set(JvmTarget.JVM_11)
        }
    }

    defaultConfig {
        applicationId = "org.ripls.app"
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        // Use git commit count if Flutter doesn't provide a build number
        // (Flutter sets versionCode to 1 when no --build-number is passed)
        versionCode = if (flutter.versionCode == 1) getGitCommitCount() else flutter.versionCode
        versionName = flutter.versionName

        // Injected into AndroidManifest.xml's com.google.android.geo.API_KEY
        // meta-data. Resolved at the top of this file; release builds with an
        // empty value are rejected by the task-graph guard below.
        manifestPlaceholders["GOOGLE_MAPS_API_KEY"] = googleMapsApiKey
    }

    signingConfigs {
        // Shared debug keystore checked into source control
        // All developers use the same keystore so only one SHA-1 fingerprint
        // needs to be registered in Firebase for Google Sign-In
        getByName("debug") {
            storeFile = file("debug.keystore")
            storePassword = "android"
            keyAlias = "androiddebugkey"
            keyPassword = "android"
        }
        create("release") {
            keyAlias = keystoreProperties["keyAlias"] as String?
            keyPassword = keystoreProperties["keyPassword"] as String?
            storeFile = keystoreProperties["storeFile"]?.let { file(it) }
            storePassword = keystoreProperties["storePassword"] as String?
        }
    }

    buildTypes {
        debug {
            signingConfig = signingConfigs.getByName("debug")
            applicationIdSuffix = ".dev"
            resValue("string", "app_name", "Ripls Dev")
        }
        release {
            // Fall back to debug signing if release keystore not configured
            // This allows CI to run release builds to verify R8/ProGuard rules
            signingConfig = if (keystoreProperties["storeFile"] != null) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            resValue("string", "app_name", "Ripls")
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
}

// Refuse to assemble a release AAB/APK without a Maps key (#2188). The Maps
// SDK reads the key from the manifest at launch and silently renders a blank
// map if it's missing, so an empty key must fail the build rather than ship.
// Scoped to release assemble/bundle tasks so debug/profile builds (local
// `flutter run`, CI test builds) still work without GCP credentials. Also
// gated on the release keystore being configured: only the signed,
// shipping builds (fastlane writes key.properties) enforce this. The
// R8/ProGuard verification build (release_build_android.yaml) runs
// `flutter build apk --release` with no keystore and no GCP creds, falls
// back to debug signing, and never ships — it must not require the key.
val releaseKeystoreConfigured = keystoreProperties["storeFile"] != null
gradle.taskGraph.whenReady {
    val shippingRelease = allTasks.any { task ->
        val name = task.name
        (name.startsWith("assemble") || name.startsWith("bundle")) && name.contains("Release")
    }
    if (shippingRelease && releaseKeystoreConfigured && googleMapsApiKey.isEmpty()) {
        throw GradleException(
            "GOOGLE_MAPS_API_KEY is not set for a release build (#2188). Refusing " +
                "to ship an app with a non-functional map. In CI, ensure the release " +
                "workflow fetches google-maps-api-key-android from Secret Manager and " +
                "exports GOOGLE_MAPS_API_KEY. Locally, build via " +
                "`npm run start:app:local:android`, which fetches the key."
        )
    }
}

// Configure profile build type after Flutter creates it
project.afterEvaluate {
    android.buildTypes.getByName("profile") {
        signingConfig = android.signingConfigs.getByName("debug")
        applicationIdSuffix = ".dev"
        resValue("string", "app_name", "Ripls Dev")
    }
}

flutter {
    source = "../.."
}

dependencies {
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")
    androidTestImplementation("androidx.test.uiautomator:uiautomator:2.4.0")
    androidTestUtil("androidx.test:orchestrator:1.6.1")
}
