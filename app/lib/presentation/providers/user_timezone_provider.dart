import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/providers.dart';
import 'package:timezone/timezone.dart' as tz;

/// Provider that exposes the current user's preferred timezone.
///
/// Returns the user's preferred timezone in IANA format (e.g., "America/New_York"),
/// or null if not set (falls back to device timezone).
///
/// This provider automatically updates when the user changes their timezone preference.
final userTimezoneProvider = FutureProvider<String?>((ref) async {
  final authState = ref.watch(authStateProvider);

  // Return null if not authenticated
  if (authState.user == null) {
    return null;
  }

  final userRepository = ref.watch(userRepositoryProvider);
  return userRepository.getPreferredTimezone(authState.user!.id);
});

/// Provider that resolves the effective timezone for the current user.
///
/// Returns the user's preferred timezone if set, otherwise falls back to the
/// device's system timezone via [tz.local.name]. This is the canonical source
/// for timezone resolution — all code that needs to store or display a timezone
/// should read from this provider rather than hardcoding 'UTC' or calling
/// [tz.local.name] directly.
///
/// Usage:
/// ```dart
/// final timezone = await ref.read(resolvedTimezoneProvider.future);
/// ```
final resolvedTimezoneProvider = FutureProvider<String>((ref) async {
  final preferred = await ref.watch(userTimezoneProvider.future);
  return preferred ?? tz.local.name;
});
