import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/savings_formatter.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_info_row.dart';
import 'package:ripls/presentation/widgets/content/content_metric_sheet.dart';

/// Builder for the single consolidated impact row shown on the first
/// tab of a terminal-state content view (a FULFILLED request, a
/// COMPLETED event, …). The row is an icon-left [ContentInfoRow]
/// whose value column is three subtle pills — Money saved · Quality
/// time · CO₂ avoided — each individually tappable to open
/// [ContentMetricSheet] with the full breakdown.
///
/// Returns `null` when [impact] is missing or empty so callers can
/// spread the result into their row list with the `?` collection
/// element:
///
/// ```dart
/// final rows = <ContentInfoRow>[
///   ?ContentImpactRow.build(context: context, impact: stats?.impact),
///   ...
/// ];
/// ```
///
/// Order of the pills matches the tile order used on completion modals
/// and story cards (Money → Quality Time → CO₂) so a glance at any
/// surface can be cross-checked against the others.
class ContentImpactRow {
  const ContentImpactRow._();

  /// Returns the impact row, or `null` when [impact] is null or has
  /// no usable data on any of the three rendered metrics. Requests
  /// often carry CO₂ / quality-time without monetary impact (and
  /// vice-versa), so the gate fires if *any* metric is present —
  /// per-pill dash fallbacks handle the rest.
  ///
  /// When [onViewImpact] is set, the row body becomes tappable (with a
  /// trailing chevron affordance) and opens the full impact receipt —
  /// the inline entry point that replaces hunting through the ⋯ manage
  /// menu after fulfillment/completion (#2724). The individual pills
  /// stay tappable for the per-metric breakdown sheets.
  static ContentInfoRow? build({
    required BuildContext context,
    required ImpactEstimate? impact,
    VoidCallback? onViewImpact,
  }) {
    if (impact == null) return null;
    final hasAny = impact.hasMoneySaved() ||
        impact.hasEmissionsPrevented() ||
        impact.hasQualityTime();
    if (!hasAny) return null;
    final l10n = context.l10n;
    final savings = SavingsFormatter.formatSavings(impact);

    // Each pill carries a compact label with the value ("$210 saved",
    // "~4 h together", "40kg CO₂") — bare numbers next to icons were
    // uninterpretable on first read (#2724). Empty metrics keep the
    // unlabeled dash.
    final money = _impactDisplayValue(savings?.costSaved, l10n);
    final qt = _impactDisplayValue(savings?.qtSaved, l10n);
    final co2 = _impactDisplayValue(savings?.co2Saved, l10n);

    final pills = <_ImpactPillData>[
      _ImpactPillData(
        icon: Icons.attach_money,
        label: l10n.impactRowMoney,
        value: money == l10n.impactRowEmpty
            ? money
            : l10n.impactChipMoneySaved(money),
        metricId: 'money',
      ),
      _ImpactPillData(
        icon: Icons.schedule,
        label: l10n.impactRowQualityTime,
        value: qt == l10n.impactRowEmpty
            ? qt
            : l10n.impactChipQualityTime(qt),
        metricId: 'qt',
      ),
      _ImpactPillData(
        icon: Icons.eco,
        label: l10n.impactRowCo2,
        value:
            co2 == l10n.impactRowEmpty ? co2 : l10n.impactChipCo2(co2),
        metricId: 'co2',
      ),
    ];

    return ContentInfoRow(
      icon: Icons.insights,
      // [value] is the accessibility-tree fallback; the visual content
      // lives in [valueBuilder].
      value: l10n.a11yImpactRow,
      rowSemanticsLabel: onViewImpact != null
          ? l10n.a11yImpactRowViewDetails
          : l10n.a11yImpactRow,
      valueBuilder: (ctx) => _ImpactPillRow(pills: pills, impact: impact),
      onTap: onViewImpact,
      trailing: onViewImpact != null
          ? const Icon(
              Icons.chevron_right,
              size: 18,
              color: GlassTokens.textMuted,
            )
          : null,
    );
  }

  /// Dash-fallback for null / zero-valued metrics. Matches the rule in
  /// [ContentImpactTiles] so a "0h" QT formatter result never leaks
  /// into the row text.
  static String _impactDisplayValue(String? raw, AppLocalizations l10n) {
    if (raw == null || raw.isEmpty) return l10n.impactRowEmpty;
    if (raw == '\$0' ||
        raw == '0h' ||
        raw == '0kg' ||
        raw == '0 min' ||
        raw == '0 g') {
      return l10n.impactRowEmpty;
    }
    return raw;
  }
}

/// Plain data carrier for one of the three impact pills. Internal to
/// this file — callers go through [ContentImpactRow.build].
class _ImpactPillData {
  const _ImpactPillData({
    required this.icon,
    required this.label,
    required this.value,
    required this.metricId,
  });

  final IconData icon;

  /// Localized full label ("Money saved" etc.) — used only for the
  /// pill's `semanticsLabel`. The visual pill shows the value, not the
  /// label.
  final String label;

  final String value;
  final String metricId;
}

/// Renders three [_ImpactPill]s inline inside the impact row's
/// `valueBuilder` slot. Pills size to their own content and the row scrolls
/// horizontally if they ever exceed the width: equal [Expanded] thirds
/// ellipsized the longest label, which defeats the point of labelling them
/// (#2724), and the host [ContentInfoRow] is height-locked, so wrapping to a
/// second line would clip instead.
class _ImpactPillRow extends StatelessWidget {
  const _ImpactPillRow({required this.pills, required this.impact});

  final List<_ImpactPillData> pills;
  final ImpactEstimate impact;

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (var i = 0; i < pills.length; i++) ...[
            if (i > 0) const SizedBox(width: 6),
            _ImpactPill(
              data: pills[i],
              onTap: () => ContentMetricSheet.show(
                context,
                metricId: pills[i].metricId,
                impact: impact,
                isCompleted: true,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// Subtle, individually-tappable impact pill — small icon + value,
/// sized to its own content inside the impact row. Translucent
/// surface with a hairline border so it reads as secondary chrome
/// while still telegraphing interactivity. Built on [Tappable] so
/// each pill announces independently as "label: value".
class _ImpactPill extends StatelessWidget {
  const _ImpactPill({required this.data, required this.onTap});

  final _ImpactPillData data;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel:
          context.l10n.a11yContentMetricTile(data.label, data.value),
      onTap: onTap,
      child: Container(
        height: 26,
        padding: const EdgeInsets.symmetric(horizontal: 8),
        decoration: BoxDecoration(
          color: GlassTokens.fillFaint,
          border: Border.all(color: GlassTokens.borderSoft),
          borderRadius: BorderRadius.circular(13),
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              data.icon,
              size: 12,
              color: GlassTokens.textMuted,
            ),
            const SizedBox(width: 3),
            Flexible(
              child: Text(
                data.value,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 11.5,
                  fontWeight: FontWeight.w600,
                  color: GlassTokens.textSecondary,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
