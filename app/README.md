# flutter_app
This is a basic Flutter project

# Setting up VSCode

Go to Extensions, search for "Flutter" and install the official Flutter extension. This should also install the Dart plugin if you haven't already.

You can now use the Flutter Sidebar (add to the left-hand same as Explorer sidebar). This should show the platforms available. You can select the target platform from there and any time you run the app it will use that default unless you specify otherwise. 

Since iOS is our initial target platform, you should follow the instructions here to make sure it is properly setup: https://docs.flutter.dev/get-started/install/macos/mobile-ios

For Android development, follow the instructions here: https://docs.flutter.dev/get-started/install/macos/mobile-android

# Flutter Doctor

You should run Flutter Doctor to check for any issues, e.g., Android Studio is not yet setup. This is available in the Flutter Sidebar or run from the command line with:

```
flutter doctor
```

# Environment Configuration

This app uses build-time environment variables passed via `--dart-define-from-file`. Three environment configurations are available:

- **env.dev.json** - Development server (per-deployment; set via `SERVER_URL`)
- **env.local.json** - Local server (`http://localhost:8080`)
- **env.prod.json** - Production server (per-deployment; set via `SERVER_URL`)

Each configuration file contains:
- `ENVIRONMENT` - Server environment (dev/local/prod)
- `LOG_LEVEL` - Logging verbosity (INFO, WARNING, etc.)
- `ENABLE_NOTIFICATIONS` - Enable/disable push notifications (true/false)
- `GOOGLE_CLIENT_ID` - Google OAuth Client ID for OIDC authentication

**Mapbox token is NOT in these files** (#1768/#1770). It lives in
Secret Manager and the build wrappers inject it via `--dart-define` —
see "Running the app" below.

# To run the app from VSCode:

**Two-Step Process:**

**Step 1: Select Your Device**
- Click the device selector in the **bottom status bar** (blue bar at the bottom of VSCode)
- Choose your target device:
  - **Chrome (web)** - For web development
  - **iPhone 16 Pro** (or any iOS simulator)
  - **Medium Phone API 35** (or any Android emulator)
  - **Your physical device** (if connected via USB or wireless debugging)

**Step 2: Select Configuration and Run**
1. Press `F5` or open Run and Debug panel (`⌘+Shift+D` on Mac / `Ctrl+Shift+D` on Windows/Linux)
2. Select one of the pre-configured launch options from the dropdown:
   - **"app: Development (dev)"** - Uses development server
   - **"app: Local Server"** - Uses localhost:8080
   - **"app: Production (prod)"** - Uses production server
   - **"app: Profile Mode (dev)"** - Dev server with performance profiling
   - **"app: Release Mode (prod)"** - Production optimized build
3. Press `F5` or click the green play button

**Examples:**
- **Web with local server:** Select "Chrome (web)" → "app: Local Server" → F5
- **iOS with dev server:** Select "iPhone 16 Pro" → "app: Development (dev)" → F5
- **Android with local:** Select Android emulator → "app: Local Server" → F5

# To run the app from the command line:

Make sure you are in the app directory.

**Step 1: Select environment configuration**

Choose one of the environment files:
- `env.dev.json` - Development server
- `env.local.json` - Local server (localhost:8080)
- `env.prod.json` - Production server

**Step 2: Run with device selection**

The npm wrappers in `package.json` are the standard entry points —
they fetch the Mapbox token from Secret Manager via
`scripts/fetch_secret.sh` and pass it through `--dart-define`. Run
from the **project root**:

```bash
# Local server (env.local.json), pass any extra flags after `--`.
npm run start:app:local -- -d emulator-5554
npm run start:app:local -- -d chrome
npm run start:app:local -- -d <device-id>

# Dev server (env.dev.json).
npm run start:app:dev   -- -d emulator-5554
# Or shorter (alias for start:app:dev):
npm run start:app
```

**Direct `flutter run` (rarely needed):** if you skip the wrapper, you
must also fetch the Mapbox token yourself, otherwise maps render blank.

```bash
cd app
flutter run --dart-define-from-file=env.local.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(../scripts/fetch_secret.sh "$DEV_PROJECT" mapbox-access-token) \
  -d <device-id>
```

Prerequisite for either path: `gcloud auth application-default login`
with an account that has `roles/secretmanager.secretAccessor` on the
`mapbox-access-token` secret in the dev project.

**To run tests:**
```bash
flutter test
```

# Building for App Stores

## Building for iOS App Store

The project provides npm scripts for building production and development versions:

**Production build (for App Store submission):**
```bash
# From project root
npm run build:app:ios:prod
```

This builds a release version with production environment configuration from `env.prod.json`.

**Development build (for testing):**
```bash
# From project root
npm run build:app:ios:dev
```

This builds with development environment configuration from `env.dev.json`.

**Manual build (from app directory):**
```bash
cd app
flutter build ios --release --dart-define-from-file=env.prod.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(../scripts/fetch_secret.sh "$PROD_PROJECT" mapbox-access-token)
```

**After building:**
1. Open Xcode: `open ios/Runner.xcworkspace`
2. Select Product → Archive
3. Once complete, upload to App Store Connect

## Building for Android Play Store

**Production build:**
```bash
# From project root
npm run build:app:android:prod
```

**Development build:**
```bash
npm run build:app:android:dev
```

**Manual build:**
```bash
cd app
flutter build apk --release --dart-define-from-file=env.prod.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(../scripts/fetch_secret.sh "$PROD_PROJECT" mapbox-access-token)
# Or for app bundle (recommended for Play Store):
flutter build appbundle --release --dart-define-from-file=env.prod.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(../scripts/fetch_secret.sh "$PROD_PROJECT" mapbox-access-token)
```

# To inspect the app from VSCode:

1. Open the Flutter command palette with `Ctrl+Shift+P`
2. Select `Flutter: Dev Tools`

This includes Flutter Inspector, Performance Profiler, and Logging. The Flutter Inspector is useful for understanding the layout of the widgets and debugging UX nesting issues.

# Directory structure

See [CLAUDE.md](../CLAUDE.md) in the project root for the Flutter app structure guidelines.

# To specify a custom server

To connect to a different server, edit the appropriate environment configuration file:

- `env.dev.json` - Development environment
- `env.local.json` - Local development
- `env.prod.json` - Production environment

Or create a custom environment file and run (Mapbox still has to come
from Secret Manager):
```bash
flutter run --dart-define-from-file=my-custom-env.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(../scripts/fetch_secret.sh "$DEV_PROJECT" mapbox-access-token)
```

# Troubleshooting

## If there are issues with the dependencies

The 'pubspec.yaml file contains the dependencies for the Flutter clients.

To install the dependencies, run:

```
flutter pub get
```

## If you are having issues with the iOS simulator

If you are having issues with the iOS simulator, check the following:

1. Make sure you have the latest version of Xcode.
2. Make sure you have the latest version of the iOS simulator.
3. Make sure you have the latest version of the Xcode command line tools.
4. Make sure you have selected the correct iOS deployment target

## Setting up an Android physical device

To run the app on your Android phone:

1. **Enable Developer Options:**
   - Open Settings on your Android phone
   - Go to "About phone" (or "About device")
   - Find "Build number" and tap it 7 times
   - You should see "You are now a developer!"

2. **Enable USB Debugging:**
   - Go back to Settings
   - Look for "Developer options" (usually under System)
   - Enable "USB debugging"

3. **Connect your phone:**
   - Connect your Android phone to your computer using a USB cable
   - On your phone, you should see a prompt "Allow USB debugging?" - tap "Allow"
   - Check "Always allow from this computer" if desired

4. **Verify connection:**
   - Run `flutter devices` to see if your phone is detected
   - Your device should appear in the list

5. **Run the app:**
   - Use `flutter run` or specify the device with `flutter run -d <device-id>`

### Wireless Debugging (Android 11+)

For Android 11 and newer, you can connect wirelessly after initial USB setup:

1. **Enable Wireless Debugging (one-time setup):**
   - On your phone: Settings → Developer options → Enable "Wireless debugging"
   - Make sure your phone and computer are on the same WiFi network
   - Tap on "Wireless debugging" to open settings
   - Tap "Pair device with pairing code"
   - Note the IP address, port, and 6-digit pairing code displayed

2. **Pair from your computer:**
   ```bash
   ~/Library/Android/sdk/platform-tools/adb pair <IP>:<PORT>
   # Enter the 6-digit pairing code when prompted
   # Example: ~/Library/Android/sdk/platform-tools/adb pair 192.0.2.10:12345
   ```

3. **Connect wirelessly:**
   - After successful pairing, your device connects automatically
   - You can now disconnect the USB cable
   - Run `flutter devices` to verify wireless connection
   - The device will show with a network address (e.g., 192.0.2.10:5555)

4. **Run the app wirelessly:**
   ```bash
   flutter run -d <wireless-device-id>
   ```

**Note:** After the initial pairing, future connections should happen automatically when wireless debugging is enabled and both devices are on the same network.

**Optional - Add adb to PATH:** To use `adb` without the full path, add this to your `~/.zshrc`:
```bash
export PATH="$PATH:$HOME/Library/Android/sdk/platform-tools"
```
Then run `source ~/.zshrc` to apply the changes.

## If you need to change the package name

Make sure you have the change_app_package_name plugin installed. It is specified in the pubspec.yaml file but it is not installed automatically you can run the following command:

```
flutter pub add change_app_package_name
```

Then run the following command:

```
flutter pub run change_app_package_name:main com.example.new_package_name
```

# Updating the app icon

Update the icon.png in assets/icons and run: 

```
flutter pub run flutter_launcher_icons
```

Optionally you can change the name and location in the the flutter_launcher_icons section of pubspec.yaml 

## Web Platform: RawSocket Constructor Error

**Problem:** When running the Flutter app on web (Chrome), you may encounter: `Unsupported operation: RawSocket constructor`

**Cause:** The ConnectRPC library has different HTTP client implementations for different platforms. The HTTP/2 client (`package:connectrpc/http2.dart`) uses raw sockets which are not supported in web browsers.

**Solution:** Use conditional imports to select the appropriate HTTP client based on the platform:

```dart
// Use web client for browsers, HTTP/2 client for native platforms
import 'package:connectrpc/web.dart' if (dart.library.io) 'package:connectrpc/http2.dart';
```

This pattern checks for the presence of `dart:io` library:
- **Web platforms:** `dart:io` is not available, so it imports `package:connectrpc/web.dart` (fetch-based client)
- **Native platforms** (iOS, Android, etc.): `dart:io` is available, so it imports `package:connectrpc/http2.dart` (full-featured HTTP/2 client)

**References:**
- [ConnectRPC Dart Documentation](https://connectrpc.com/docs/dart/using-clients/)
- [Dart Conditional Imports Guide](https://dart.dev/language/libraries)
- [Practical Tutorial on Conditional Imports](https://codewithandrea.com/tips/dart-conditional-imports/)

## Web Platform: Failed to Fetch / CORS Errors

**Problem:** When running the Flutter web app and trying to connect to your local server, you may see: `TypeError: Failed to fetch`

**Cause:** Browsers enforce Cross-Origin Resource Sharing (CORS) policies. When your web app (running on `localhost:xxxxx`) tries to make requests to your API server (running on `localhost:8080`), the browser blocks these cross-origin requests unless the server explicitly allows them.

**Solution:** Configure CORS middleware on your Go server to allow requests from your web client. See the server/README for implementation details.

