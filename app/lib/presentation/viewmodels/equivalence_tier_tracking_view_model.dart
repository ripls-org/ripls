import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/tier_tracking_repository.dart';
import 'package:ripls/services/providers.dart';

/// EquivalenceTierTrackingState carries only what's needed for cheap
/// rebuild equality — the per-surface map of last-seen tier ids. The
/// widget never reads this state; it calls [recordCurrentTier] in a
/// post-frame callback to keep the UI render free of analytics work.
class EquivalenceTierTrackingState {
  /// In-memory cache of the latest tier id we have observed during the
  /// current app session, keyed by `"surface:metric"`. Mirrors the
  /// persisted SharedPreferences value but lets us short-circuit the
  /// hot path (read on every render) without an async round-trip.
  final Map<String, String> sessionLastSeen;

  const EquivalenceTierTrackingState({this.sessionLastSeen = const {}});

  EquivalenceTierTrackingState copyWith({Map<String, String>? sessionLastSeen}) {
    return EquivalenceTierTrackingState(
      sessionLastSeen: sessionLastSeen ?? this.sessionLastSeen,
    );
  }
}

/// EquivalenceTierTrackingNotifier owns the "did this user already see
/// this tier?" decision and the `tier_unlocked` analytics emission.
///
/// Widgets call [recordCurrentTier] each time they render a resolved
/// tier; the notifier consults [TierTrackingRepository] (persisted) and
/// its own in-memory `sessionLastSeen` map (hot), and fires the analytics
/// event only when the supplied tier is genuinely new for that
/// `(surface, metric)` pair. Below-lowest renders pass a null tierId
/// and short-circuit.
class EquivalenceTierTrackingNotifier extends Notifier<EquivalenceTierTrackingState>
    with SafeNotifierMixin<EquivalenceTierTrackingState> {
  @override
  EquivalenceTierTrackingState build() {
    return const EquivalenceTierTrackingState();
  }

  String _key(String surface, String metric) => '$surface:$metric';

  /// Record the tier currently rendered on [surface] for [metric].
  ///
  /// Fires a [TierUnlockedEvent] the first time a given `(surface,
  /// metric, tierId)` tuple is observed (across app sessions, per
  /// device). [tierId] is null when the value is below the lowest
  /// threshold — that case persists nothing and emits nothing.
  Future<void> recordCurrentTier({
    required String surface,
    required String metric,
    required String? tierId,
    required double value,
    required int communitySize,
  }) async {
    if (tierId == null) return;

    final cacheKey = _key(surface, metric);
    if (state.sessionLastSeen[cacheKey] == tierId) {
      // Already fired this session for this exact tier; nothing to do.
      return;
    }

    final repo = ref.read(tierTrackingRepositoryProvider);
    final persisted = await repo.getLastSeenTier(surface, metric);
    if (!ref.mounted) return;

    if (persisted == tierId) {
      // The tier hasn't changed since the last time we ran. Cache it
      // in the session map so we don't keep checking the prefs.
      safeUpdateState((s) => s.copyWith(
            sessionLastSeen: {...s.sessionLastSeen, cacheKey: tierId},
          ));
      return;
    }

    // Real change — fire analytics and persist the new tier. Analytics is
    // best-effort; a missing observability service (e.g. in tests where
    // Firebase isn't initialized) must not crash the UI render.
    try {
      final service = ref.read(observabilityServiceProvider);
      await service.logAnalyticsEvent(TierUnlockedEvent(
        metric: metric,
        tierId: tierId,
        value: value,
        communitySize: communitySize,
      ));
    } catch (_) {
      // Swallow: analytics failures never block the user experience.
    }
    if (!ref.mounted) return;

    await repo.recordTier(surface, metric, tierId);
    if (!ref.mounted) return;

    safeUpdateState((s) => s.copyWith(
          sessionLastSeen: {...s.sessionLastSeen, cacheKey: tierId},
        ));
  }
}

/// Provider for the [EquivalenceTierTrackingNotifier]. Not autoDispose:
/// the notifier's only state is a tiny session-level dedup map and the
/// repository it depends on is a singleton — keeping the notifier alive
/// for the app's lifetime keeps the dedup correct across navigation.
final equivalenceTierTrackingProvider = NotifierProvider<
    EquivalenceTierTrackingNotifier, EquivalenceTierTrackingState>(
  EquivalenceTierTrackingNotifier.new,
);
