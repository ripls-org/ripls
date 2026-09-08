import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/equivalence_tier_tracking_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:url_launcher/url_launcher.dart';
import 'equivalence_metric.dart';
import 'equivalence_resolver.dart';
import 'equivalence_tier.dart';

/// EquivalenceLadderCard renders an inline impact-equivalence card: an
/// emoji-accented row with the ICU caption ("X of time together — about
/// as good for you as Y.") and a small "Source: Z" link footer.
///
/// Used on the community-metrics stages where the previous walk-distance
/// and meal-comparison cards sat. For the bigger editorial layout used
/// on workshop detail screens, see [LadderEquivalentSection].
class EquivalenceLadderCard extends ConsumerWidget {
  final List<EquivalenceTier> ladder;
  final EquivalenceMetric metric;
  final EquivalenceSurface surface;
  final double value;
  final int communitySize;
  final String emoji;
  final Color accentColor;
  final String fallbackBody;

  const EquivalenceLadderCard({
    super.key,
    required this.ladder,
    required this.metric,
    required this.surface,
    required this.value,
    required this.communitySize,
    required this.emoji,
    required this.accentColor,
    required this.fallbackBody,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final resolved = resolveEquivalence(
      ladder: ladder,
      value: value,
      l10n: l10n,
      fallbackBody: fallbackBody,
    );

    // Fire the analytics event once per (surface, metric, tier) tuple.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref
          .read(equivalenceTierTrackingProvider.notifier)
          .recordCurrentTier(
            surface: surface.persistedName,
            metric: metric.analyticsName,
            tierId: resolved.tierId,
            value: value,
            communitySize: communitySize,
          );
    });

    final captionText = _captionText(l10n, resolved);
    final textStyle = TextStyle(
      fontSize: 15,
      fontWeight: FontWeight.w400,
      color: AppColors.textPrimary(context),
      height: 1.4,
    );

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: accentColor.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: accentColor.withValues(alpha: 0.2)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(emoji, style: const TextStyle(fontSize: 32)),
              const SizedBox(width: 12),
              Expanded(
                child: AnimatedSwitcher(
                  duration: accessibleDuration(
                    context,
                    const Duration(milliseconds: 300),
                  ),
                  child: Semantics(
                    liveRegion: true,
                    container: true,
                    key: ValueKey<String>(resolved.tierId ?? '_fallback'),
                    child: Text(captionText, style: textStyle),
                  ),
                ),
              ),
            ],
          ),
          if (resolved.sourceUrl.isNotEmpty) ...[
            const SizedBox(height: 8),
            _SourceLink(
              name: resolved.sourceName,
              url: resolved.sourceUrl,
              accentColor: accentColor,
            ),
          ],
        ],
      ),
    );
  }

  String _captionText(AppLocalizations l10n, ResolvedEquivalence resolved) {
    if (resolved.tierId == null) {
      return resolved.body;
    }
    switch (metric) {
      case EquivalenceMetric.timeTogether:
        return l10n.equivalenceTimeCaption(
          formatTimeValue(value, l10n),
          resolved.headline,
        );
      case EquivalenceMetric.moneySaved:
        return l10n.equivalenceMoneyCaption(
          formatMoneyValue(value, l10n),
          communitySize,
          resolved.headline,
        );
      case EquivalenceMetric.co2Avoided:
        return l10n.equivalenceCo2Caption(
          formatCo2Value(value, l10n),
          communitySize,
          resolved.headline,
        );
    }
  }
}

/// LadderEquivalentSection renders the bigger editorial layout used on
/// the workshop detail screens: small section header, large serif
/// headline (the tier label), evidence paragraph, source link. The
/// caller supplies the section header and the headline font.
class LadderEquivalentSection extends ConsumerWidget {
  final List<EquivalenceTier> ladder;
  final EquivalenceMetric metric;
  final EquivalenceSurface surface;
  final double value;
  final int communitySize;
  final String sectionHeader;
  final TextStyle sectionHeaderStyle;
  final TextStyle headlineStyle;
  final TextStyle bodyStyle;
  final Color accentColor;
  final String fallbackBody;

  const LadderEquivalentSection({
    super.key,
    required this.ladder,
    required this.metric,
    required this.surface,
    required this.value,
    required this.communitySize,
    required this.sectionHeader,
    required this.sectionHeaderStyle,
    required this.headlineStyle,
    required this.bodyStyle,
    required this.accentColor,
    required this.fallbackBody,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final resolved = resolveEquivalence(
      ladder: ladder,
      value: value,
      l10n: l10n,
      fallbackBody: fallbackBody,
    );

    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref
          .read(equivalenceTierTrackingProvider.notifier)
          .recordCurrentTier(
            surface: surface.persistedName,
            metric: metric.analyticsName,
            tierId: resolved.tierId,
            value: value,
            communitySize: communitySize,
          );
    });

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(sectionHeader, style: sectionHeaderStyle),
        const SizedBox(height: 12),
        AnimatedSwitcher(
          duration: accessibleDuration(
            context,
            const Duration(milliseconds: 300),
          ),
          child: Semantics(
            liveRegion: true,
            container: true,
            key: ValueKey<String>(resolved.tierId ?? '_fallback'),
            child: Text(resolved.headline, style: headlineStyle),
          ),
        ),
        const SizedBox(height: 12),
        Text(resolved.body, style: bodyStyle),
        if (resolved.sourceUrl.isNotEmpty) ...[
          const SizedBox(height: 12),
          _SourceLink(
            name: resolved.sourceName,
            url: resolved.sourceUrl,
            accentColor: accentColor,
          ),
        ],
      ],
    );
  }
}

class _SourceLink extends StatelessWidget {
  final String name;
  final String url;
  final Color accentColor;

  const _SourceLink({
    required this.name,
    required this.url,
    required this.accentColor,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Tappable(
      isLink: true,
      semanticsLabel: l10n.a11yEquivalenceSourceLink(name),
      onTap: () => _openUrl(url),
      child: Text(
        l10n.equivalenceSourcePrefix(name),
        style: TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w500,
          color: accentColor,
          decoration: TextDecoration.underline,
          decorationColor: accentColor.withValues(alpha: 0.6),
        ),
      ),
    );
  }

  Future<void> _openUrl(String url) async {
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}
