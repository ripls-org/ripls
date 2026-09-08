# Push Notifications Configuration

This document explains how push notifications are configured in the Ripls app and how to enable/disable them for development and production builds.

## Problem

iOS apps that include the Push Notifications capability require a provisioning profile from an Apple Developer Program account (paid). Personal development teams (free Apple Developer accounts) cannot use push notifications, which blocks development builds to physical devices.

## Solution

The app uses a flexible configuration system that allows notifications to be disabled for development while keeping the infrastructure in place for production use.

### Components

1. **Environment Flag**: `.env` file contains `ENABLE_NOTIFICATIONS` flag
2. **Entitlements File**: Single entitlements file with push notifications capability (now that we have a corporate Apple Developer account)
3. **Conditional Code**: Flutter code checks the flag before initializing notifications

## Configuration Files

### 1. Environment Variable (`.env`)

```bash
# Enable/disable push notifications
# Set to false for development on personal Apple Developer accounts
# Set to true for production builds with proper provisioning profiles
ENABLE_NOTIFICATIONS=false
```

### 2. iOS Entitlements File

**Single Entitlements File**: `ios/Runner/Runner.entitlements`
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>aps-environment</key>
	<string>development</string>
	<key>com.apple.developer.associated-domains</key>
	<array>
		<string>applinks:ripls.app</string>
	</array>
</dict>
</plist>
```

Now that we have a corporate Apple Developer account, all build configurations (Debug, Profile, and Release) use the same entitlements file with push notifications capability enabled. The `.env` flag controls whether the app actually initializes and uses notifications at runtime.

## Usage

### For Development (Testing Without Notifications)

1. Set notifications to disabled in `.env`:
   ```bash
   ENABLE_NOTIFICATIONS=false
   ```

2. Build and run normally:
   ```bash
   flutter run --profile
   ```

   Or for specific device:
   ```bash
   flutter run --profile -d "Thomas Swiss iPhone"
   ```

The app will:
- Skip Firebase initialization
- Skip notification permission requests
- Skip FCM token registration
- Allow testing without notification prompts or dependencies

### For Production (With Notifications Enabled)

1. Set notifications to enabled in `.env`:
   ```bash
   ENABLE_NOTIFICATIONS=true
   ```

2. Build and deploy:
   ```bash
   flutter build ios --release
   ```

The app will:
- Initialize Firebase
- Request notification permissions
- Register FCM tokens with the server
- Handle push notifications normally

## How It Works

### Code Flow

1. **App Startup** ([main.dart:22-27](lib/main.dart#L22-L27)):
   ```dart
   if (Environment.notificationsEnabled) {
     await Firebase.initializeApp();
     FirebaseMessaging.onBackgroundMessage(firebaseMessagingBackgroundHandler);
   }
   ```

2. **FCM Initialization** ([main.dart:58-70](lib/main.dart#L58-L70)):
   ```dart
   void _initializeFCM() {
     if (!Environment.notificationsEnabled) {
       debugPrint('Notifications disabled via ENABLE_NOTIFICATIONS=false');
       return;
     }
     // ... initialize FCM service
   }
   ```

3. **Environment Check** ([environment.dart:38-40](lib/core/config/environment.dart#L38-L40)):
   ```dart
   static bool get notificationsEnabled {
     return dotenv.get('ENABLE_NOTIFICATIONS', fallback: 'false').toLowerCase() == 'true';
   }
   ```

### Build Process

1. Flutter reads `.env` file at startup
2. Xcode uses appropriate entitlements file based on build configuration
3. If `ENABLE_NOTIFICATIONS=false`:
   - No Push Notifications capability in entitlements
   - No Firebase/FCM initialization
   - App builds successfully on personal developer accounts
4. If `ENABLE_NOTIFICATIONS=true`:
   - Push Notifications capability included in entitlements
   - Full Firebase/FCM initialization
   - Requires proper provisioning profile

## Troubleshooting

### Notifications Not Working

**Cause**: Notifications disabled in environment configuration.

**Solution**:
1. Set `ENABLE_NOTIFICATIONS=true` in `.env`
2. Verify you have a valid provisioning profile with Push Notifications capability
3. Rebuild the app

### How to Toggle Notifications

Simply update the `.env` file:
```bash
# Enable notifications
ENABLE_NOTIFICATIONS=true

# Or disable notifications
ENABLE_NOTIFICATIONS=false
```

Then rebuild the app. All build configurations now use the same entitlements file with push notifications capability, so you don't need to modify Xcode project settings.

## Technical Details

### Why a Single Entitlements File Now?

With a corporate Apple Developer account, we can now use the same entitlements file for all build configurations. The entitlements file includes push notifications capability, but the `.env` flag provides runtime control over whether notifications are actually initialized and used. This simplifies the build configuration while maintaining flexibility for development and testing.

### Why Not Use #if Preprocessor Directives?

Flutter doesn't support C-style preprocessor directives for conditional compilation. Instead, we use runtime checks with the environment flag, which is:
- Simpler to manage
- Easier to understand
- More flexible (can be changed without recompiling)
- Works consistently across all platforms

### What About Android?

Android does not have the same restrictions as iOS. The Firebase/FCM initialization still respects the `ENABLE_NOTIFICATIONS` flag for consistency, but Android builds will work regardless of the setting.

## Files Modified

- `.env` - Added `ENABLE_NOTIFICATIONS` flag
- `ios/Runner/Runner.entitlements` - Single entitlements file with push notifications and associated domains
- `ios/Runner.xcodeproj/project.pbxproj` - All build configurations use Runner.entitlements
- `lib/core/config/environment.dart` - Added `notificationsEnabled` getter
- `lib/main.dart` - Added conditional Firebase/FCM initialization

## References

- [Apple Developer: Entitlements](https://developer.apple.com/documentation/bundleresources/entitlements)
- [Firebase Cloud Messaging for Flutter](https://firebase.flutter.dev/docs/messaging/overview)
- [Flutter Environment Variables](https://pub.dev/packages/flutter_dotenv)
