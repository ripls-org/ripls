import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/equivalence_resolver.dart';
import 'package:ripls/presentation/widgets/impact/equivalence/time_health_ladder.dart';

/// Tier-based narrative resolution for the user-profile impact rows.
///
/// The user-profile screen uses the same cited equivalence ladders as
/// the community-metrics screens — health framing for time. The adapter
/// resolves the highest tier whose threshold is `<= value` and returns
/// the localized headline as a one-line narrative.

/// timeGivenNarrative returns a tiered one-sentence narrative
/// describing how much time the user has banked. [minutes] is the raw
/// `time_banked_minutes` mean from `UserImpactMetrics`.
String timeGivenNarrative(double minutes, AppLocalizations l10n) {
  final resolved = resolveEquivalence(
    ladder: kTimeHealthLadder,
    value: minutes / 60,
    l10n: l10n,
    fallbackBody: l10n.equivalenceTimeFallbackBody,
  );
  return _ensurePeriod(resolved.headline);
}

String _ensurePeriod(String s) {
  if (s.isEmpty) return s;
  final last = s[s.length - 1];
  if (last == '.' || last == '!' || last == '?') return s;
  return '$s.';
}
