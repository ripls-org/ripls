allprojects {
    repositories {
        google()
        mavenCentral()
        maven { url = uri("https://maven.leancode.pl/releases") }
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

// Redirect the :app module's build directory up front, before the subprojects
// block below. A plugin subproject's evaluationDependsOn(":app") (added there)
// can force :app to evaluate mid-iteration — before the loop reaches :app to set
// its build directory. Under AGP 9, getDefaultProguardFile() (used by the release
// build type in app/build.gradle.kts) resolves its path eagerly at :app
// evaluation time, so if the redirect isn't applied yet it captures the
// un-redirected location while AGP's proguard-extract task writes to the
// redirected one. R8 then fails with "Supplied proguard configuration does not
// exist". Setting :app's build directory here guarantees the redirect wins.
project(":app").layout.buildDirectory.value(newBuildDir.dir("app"))

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)

    // TODO(#2792): drop once camera_android_camerax puts concurrent-futures on
    // its own compile classpath (or camera-core's POM stops scoping it
    // `runtime`, or javac downgrades the diagnostic again — JDK-8370800).
    //
    // camera-core annotates fields such as
    // SurfaceRequest.mSurfaceRecreationCompleter with JSpecify TYPE_USE
    // annotations whose declared type comes from
    // androidx.concurrent.futures.CallbackToFutureAdapter, but camera-core's POM
    // scopes androidx.concurrent:concurrent-futures to `runtime` — so it is on
    // the plugin's runtime classpath and absent from its compile classpath.
    // javac 24+ turned "cannot attach type annotations: class file for X not
    // found" from a warning into a hard error, so on JDK 25
    // :camera_android_camerax:compileReleaseJavaWithJavac fails and takes the
    // Android release build with it. compileOnly puts the class file where
    // javac can read it without touching the runtime graph or the packaged app.
    //
    // Nested in withPlugin because the `compileOnly` configuration only exists
    // once the Android library plugin has been applied to the subproject.
    if (project.name == "camera_android_camerax") {
        pluginManager.withPlugin("com.android.library") {
            dependencies {
                add("compileOnly", "androidx.concurrent:concurrent-futures:1.3.0")
            }
        }
    }

    // Fix for plugins missing namespace (e.g., flutter_app_badger, add_2_calendar)
    // This must run after plugins are applied, so we use pluginManager.withPlugin
    pluginManager.withPlugin("com.android.library") {
        extensions.findByType<com.android.build.gradle.LibraryExtension>()?.apply {
            if (namespace == null) {
                val manifestFile = file("src/main/AndroidManifest.xml")
                if (manifestFile.exists()) {
                    val manifestText = manifestFile.readText()
                    val packageRegex = Regex("""package\s*=\s*"([^"]+)"""")
                    val packageMatch = packageRegex.find(manifestText)
                    if (packageMatch != null) {
                        namespace = packageMatch.groupValues[1]
                    }
                }
            }
        }
    }

    // Force older plugin AARs (e.g. add_2_calendar 3.0.1, last released
    // against API 33) to recompile at the same compileSdk as the app.
    // Without this, AAR-metadata checks fail because those plugins'
    // transitive androidx.* deps (fragment 1.7+, lifecycle 2.7+, core
    // 1.13+, etc.) require 34+.
    //
    // Has to run *after* the subproject's own `android { compileSdk =
    // 33 }` block, otherwise we set the value first and the plugin
    // overwrites it. Most subprojects haven't evaluated yet at this
    // point, so afterEvaluate works — but `evaluationDependsOn(":app")`
    // below forces :app and any peer it transitively pulls in to
    // already be evaluated by the time we get there, so we apply
    // directly in that case.
    val overrideCompileSdk = {
        extensions.findByType<com.android.build.gradle.LibraryExtension>()?.apply {
            compileSdk = 36
        }
        Unit
    }
    if (state.executed) {
        overrideCompileSdk()
    } else {
        afterEvaluate { overrideCompileSdk() }
    }

    // Evaluate :app dependency for non-app subprojects
    if (project.name != "app") {
        project.evaluationDependsOn(":app")
    }
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
