import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// TierTrackingRepository persists the highest equivalence tier the user
/// has been shown for a given `(surface, metric)` pair. The
/// `EquivalenceTierTrackingNotifier` consults the repository to decide
/// whether to fire the `tier_unlocked` analytics event — we want it to
/// fire exactly once per real threshold crossing, not on every render.
///
/// Persistence is per-device (SharedPreferences), so a fresh install or
/// data wipe replays the early tiers on first view. That's fine — these
/// are celebrations of milestones, and an analytics double-fire across
/// devices is acceptable noise compared to silent regressions.
class TierTrackingRepository {
  SharedPreferences? _prefsInstance;

  Future<SharedPreferences> get _prefs async {
    _prefsInstance ??= await SharedPreferences.getInstance();
    return _prefsInstance!;
  }

  /// Constructor accepts an optional [SharedPreferences] for tests.
  TierTrackingRepository([SharedPreferences? prefs]) : _prefsInstance = prefs;

  static const String _keyPrefix = 'tier_tracking:';

  String _key(String surface, String metric) =>
      '$_keyPrefix$surface:$metric';

  /// Returns the most-recently-recorded tier id for [surface]/[metric],
  /// or null when the user has never seen any tier on this surface.
  Future<String?> getLastSeenTier(String surface, String metric) async {
    final prefs = await _prefs;
    return prefs.getString(_key(surface, metric));
  }

  /// Persists [tierId] as the latest-seen tier for [surface]/[metric].
  Future<void> recordTier(
      String surface, String metric, String tierId) async {
    final prefs = await _prefs;
    await prefs.setString(_key(surface, metric), tierId);
  }
}

/// Provider for the singleton [TierTrackingRepository].
final tierTrackingRepositoryProvider = Provider<TierTrackingRepository>(
  (_) => TierTrackingRepository(),
);
