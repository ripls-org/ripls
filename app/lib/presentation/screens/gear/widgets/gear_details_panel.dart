import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_impact_tiles.dart';
import 'package:ripls/presentation/widgets/content/content_metric_sheet.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart'
    show ContentDetailCard;
import 'package:url_launcher/url_launcher.dart';

/// GearDetailsPanel is the full-screen "Item Details" surface the read shell's
/// DETAILS mini morph-expands into (gear-main-screen.html). It stacks the gear
/// specs (brand / model / value / material / weight), an optional product-page
/// link, and an "Impact so far" metrics row, over the gear photo + scrim.
///
/// Owners edit the **metadata** (brand / model / value / weight / website)
/// inline here — this screen owns the whole edit flow and never touches the
/// title or description (those live in the content view's edit pane).
class GearDetailsPanel extends ConsumerStatefulWidget {
  final String gearId;

  const GearDetailsPanel({super.key, required this.gearId});

  @override
  ConsumerState<GearDetailsPanel> createState() => _GearDetailsPanelState();
}

class _GearDetailsPanelState extends ConsumerState<GearDetailsPanel> {
  bool _editing = false;
  String _brand = '';
  String _model = '';
  String _valueUsd = '';
  String _weightGrams = '';
  String _website = '';
  // Bumps to reset the TextFormFields' initialValue when an edit session starts.
  int _editSession = 0;

  String get gearId => widget.gearId;

  void _startEditing(GetGearResponse gear) {
    final metadata = gear.hasMetadata() ? gear.metadata : null;
    final valueEstimate = gear.hasValueEstimate() ? gear.valueEstimate : null;
    final weightMean = metadata?.weightGrams.value.mean ?? 0;
    setState(() {
      _editing = true;
      _editSession++;
      _brand = metadata?.brand.value ?? '';
      _model = metadata?.model.value ?? '';
      _valueUsd = valueEstimate?.hasEstimatedValueUsd() ?? false
          ? valueEstimate!.estimatedValueUsd.toStringAsFixed(0)
          : '';
      _weightGrams = weightMean > 0 ? weightMean.toStringAsFixed(0) : '';
      _website = gear.sourceUrl;
    });
  }

  Future<void> _save() async {
    await ref.read(gearProvider(gearId).notifier).saveMetadataSilent(
          brand: _brand,
          model: _model,
          valueUsd: _valueUsd,
          weightGrams: _weightGrams,
          website: _website,
        );
    if (!mounted) return;
    setState(() => _editing = false);
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearProvider(gearId));
    final gear = state.gearDetails;
    if (gear == null) {
      return const ContentMorphPanel(child: SizedBox.shrink());
    }
    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context, gear, isOwner: state.isOwner),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(18, 6, 18, 28),
                child: _editing
                    ? _editBody(context, state)
                    : _readBody(context, state, gear),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _readBody(
      BuildContext context, GearState state, GetGearResponse gear) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _specs(context, gear),
        ?_productPage(context, gear),
        ?_impact(context, state),
      ],
    );
  }

  Widget _editBody(BuildContext context, GearState state) {
    final enabled = !state.isSaving;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const SizedBox(height: 4),
        _editCard(
          children: [
            _editRow(context.l10n.gearDetailBrand, _brand,
                context.l10n.gearDetailBrandHint, enabled, (v) => _brand = v),
            _divider(),
            _editRow(context.l10n.gearDetailModel, _model,
                context.l10n.gearDetailModelHint, enabled, (v) => _model = v),
            _divider(),
            _editRow(
                context.l10n.gearDetailEstimatedValue,
                _valueUsd,
                context.l10n.gearDetailValueHint,
                enabled,
                (v) => _valueUsd = v,
                keyboardType: TextInputType.number),
            _divider(),
            _editRow(
                context.l10n.gearDetailWeight,
                _weightGrams,
                context.l10n.gearDetailWeightHint,
                enabled,
                (v) => _weightGrams = v,
                keyboardType: TextInputType.number),
            _divider(),
            _editRow(context.l10n.gearDetailProductPage, _website, 'https://',
                enabled, (v) => _website = v,
                keyboardType: TextInputType.url),
          ],
        ),
      ],
    );
  }

  Widget _header(BuildContext context, GetGearResponse gear,
      {required bool isOwner}) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.fromLTRB(18, 8, 12, 8),
      child: Row(
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                l10n.gearDetailTitle,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 28,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ),
          if (_editing) ...[
            // Editing happens right here — save or cancel back to the read view;
            // the panel itself stays open.
            IconAction(
              icon: Icons.check_rounded,
              semanticsLabel: l10n.a11yGearSaveMetadata,
              color: AppColors.onContentImage,
              onPressed: _save,
            ),
            IconAction(
              icon: Icons.close_rounded,
              semanticsLabel: l10n.commonCancel,
              color: AppColors.onContentImage,
              onPressed: () => setState(() => _editing = false),
            ),
          ] else ...[
            // Owners edit the specs inline (metadata only — never title/desc).
            if (isOwner)
              IconAction(
                icon: Icons.edit_outlined,
                semanticsLabel: l10n.a11yGearEditMetadata,
                color: AppColors.onContentImage,
                onPressed: () => _startEditing(gear),
              ),
            IconAction(
              icon: Icons.close_rounded,
              semanticsLabel: l10n.a11yClose,
              color: AppColors.onContentImage,
              onPressed: () => Navigator.of(context).pop(),
            ),
          ],
        ],
      ),
    );
  }

  // The edit card mirrors the read-only ContentDetailCard exactly — same dark
  // tint, border, dividers, and row typography — so editing reads as the same
  // surface with the values simply turned into fields (no chunky white inputs).
  Widget _editCard({required List<Widget> children}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: OverlayTokens.fieldFill,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: GlassTokens.hairline),
      ),
      child: Column(mainAxisSize: MainAxisSize.min, children: children),
    );
  }

  Widget _divider() => Divider(height: 1, color: GlassTokens.hairline);

  Widget _editRow(
    String label,
    String value,
    String hint,
    bool enabled,
    ValueChanged<String> onChanged, {
    TextInputType? keyboardType,
  }) {
    return Row(
      children: [
        Text(
          label,
          style: const TextStyle(fontSize: 13, color: GlassTokens.textPrimary),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: TextFormField(
            key: ValueKey('$label-$_editSession'),
            initialValue: value,
            onChanged: onChanged,
            enabled: enabled,
            keyboardType: keyboardType,
            maxLines: 1,
            textAlign: TextAlign.right,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: GlassTokens.textPrimary,
            ),
            decoration: InputDecoration(
              hintText: hint,
              hintStyle: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: GlassTokens.textFaint,
              ),
              // Defeat the global filled-input theme that paints white boxes.
              filled: true,
              fillColor: Colors.transparent,
              isDense: true,
              contentPadding: const EdgeInsets.symmetric(vertical: 12),
              border: InputBorder.none,
              enabledBorder: InputBorder.none,
              focusedBorder: InputBorder.none,
              disabledBorder: InputBorder.none,
            ),
          ),
        ),
      ],
    );
  }

  Widget _specs(BuildContext context, GetGearResponse gear) {
    final l10n = context.l10n;
    final metadata = gear.hasMetadata() ? gear.metadata : null;
    final valueEstimate = gear.hasValueEstimate() ? gear.valueEstimate : null;

    final brand = metadata?.brand.value ?? '';
    final model = metadata?.model.value ?? '';
    final materialCat = metadata?.materialCategory.value;
    final materialStr = materialCat != null
        ? GearMetadataFormatter.formatMaterialCategory(materialCat)
        : null;
    final weightStr = metadata != null
        ? GearMetadataFormatter.formatWeight(metadata.weightGrams.value)
        : null;
    final valueStr = valueEstimate != null &&
            valueEstimate.hasEstimatedValueUsd()
        ? '\$${valueEstimate.estimatedValueUsd.toStringAsFixed(0)}'
        : null;

    final rows = <(String, String)>[
      if (brand.isNotEmpty) (l10n.gearDetailBrand, brand),
      if (model.isNotEmpty) (l10n.gearDetailModel, model),
      if (valueStr != null) (l10n.gearDetailEstimatedValue, valueStr),
      if (materialStr != null) (l10n.gearDetailMaterial, materialStr),
      if (weightStr != null) (l10n.gearDetailWeight, weightStr),
    ];
    if (rows.isEmpty) {
      return Padding(
        padding: const EdgeInsets.only(top: 8),
        child: Text(
          l10n.gearDetailNoSpecs,
          style: const TextStyle(
            color: AppColors.darkTextSecondary,
            fontSize: 14,
          ),
        ),
      );
    }
    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: ContentDetailCard(rows: rows),
    );
  }

  Widget? _productPage(BuildContext context, GetGearResponse gear) {
    final url = gear.sourceUrl.trim();
    if (url.isEmpty) return null;
    final domain = Uri.tryParse(url)?.host ?? url;
    return Padding(
      padding: const EdgeInsets.only(top: 14),
      child: Tappable(
        semanticsLabel: context.l10n.gearDetailProductPage,
        onTap: () => _launch(url),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          decoration: BoxDecoration(
            color: AppColors.darkTextPrimary.withValues(alpha: 0.05),
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: AppColors.darkBorder),
          ),
          child: Row(
            children: [
              Container(
                width: 38,
                height: 38,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: AppColors.experienceSageGreen.withValues(alpha: 0.18),
                  borderRadius: BorderRadius.circular(11),
                ),
                child: const Icon(Icons.link,
                    size: 18, color: AppColors.experienceSageGreen),
              ),
              const SizedBox(width: 13),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      context.l10n.gearDetailProductPage.toUpperCase(),
                      style: const TextStyle(
                        color: AppColors.darkTextTertiary,
                        fontSize: 10.5,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 0.6,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      domain,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: AppColors.onContentImage,
                        fontSize: 14.5,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.north_east,
                  size: 16, color: AppColors.darkTextTertiary),
            ],
          ),
        ),
      ),
    );
  }

  Widget? _impact(BuildContext context, GearState state) {
    final stats = state.gearStats;
    if (stats == null) return null;
    final hasLoaned = stats.timesLoaned > 0;
    final impact = hasLoaned
        ? (stats.hasImpact() ? stats.impact : null)
        : (stats.hasPotentialImpact() ? stats.potentialImpact : null);
    if (impact == null) return null;
    return Padding(
      padding: const EdgeInsets.only(top: 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            context.l10n.gearDetailImpactSoFar.toUpperCase(),
            style: const TextStyle(
              color: AppColors.darkTextTertiary,
              fontSize: 11,
              fontWeight: FontWeight.w800,
              letterSpacing: 1,
            ),
          ),
          const SizedBox(height: 11),
          ContentImpactTiles(
            impact: impact,
            onMetricTap: (metricId) => ContentMetricSheet.show(
              context,
              metricId: metricId,
              impact: impact,
              isCompleted: hasLoaned,
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _launch(String url) async {
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}
