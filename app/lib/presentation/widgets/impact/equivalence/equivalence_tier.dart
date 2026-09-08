/// EquivalenceUnit is the kind of value a ladder threshold compares
/// against. Used so the shared rendering widget can ignore unit details
/// (the per-metric resolution functions handle formatting).
enum EquivalenceUnit { hours, usd, kg }

/// EquivalenceTier is one rung of a metric ladder: a threshold + the
/// localization keys for its headline label, body copy, source-name
/// label, and the source URL itself.
///
/// All `*Key` fields are ARB key names; resolution to localized text
/// happens at the widget layer via `context.l10n`. The source URL is
/// kept as a raw `const String` because URLs are not localized.
class EquivalenceTier {
  /// Stable analytics identifier (e.g. `t_40hr`, `m_1025`). Matches the
  /// `id` column of the spec table verbatim — never mutate post-ship.
  final String id;

  /// Lowest value at which this tier becomes the resolved tier. Values
  /// strictly below the lowest tier's threshold resolve to no tier.
  final double threshold;

  /// Unit the threshold is denominated in.
  final EquivalenceUnit unit;

  /// ARB key for the bold headline phrase (e.g. "A hug, repaid").
  final String labelKey;

  /// ARB key for the body paragraph (the evidence text).
  final String copyKey;

  /// ARB key for the source name shown after "Source:" (e.g.
  /// "Psychology Today"). The display string is localized; the URL is
  /// not.
  final String sourceNameKey;

  /// Permalink to the source. Opens in the system browser.
  final String sourceUrl;

  const EquivalenceTier({
    required this.id,
    required this.threshold,
    required this.unit,
    required this.labelKey,
    required this.copyKey,
    required this.sourceNameKey,
    required this.sourceUrl,
  });
}

/// selectTier returns the highest tier in [ladder] whose threshold is
/// `<= value`, or `null` when [value] is below the lowest tier (the
/// "Just getting started" zone). The ladder is assumed to be sorted by
/// ascending threshold.
EquivalenceTier? selectTier(List<EquivalenceTier> ladder, double value) {
  EquivalenceTier? best;
  for (final tier in ladder) {
    if (tier.threshold <= value) {
      best = tier;
    } else {
      break;
    }
  }
  return best;
}
