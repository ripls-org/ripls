import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata, GetGearResponse, GetGearPeopleResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_impact_tiles.dart';
import 'package:ripls/presentation/widgets/content/content_metric_sheet.dart';
import 'package:ripls/presentation/widgets/content/content_people_row.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart';
import 'package:ripls/presentation/widgets/gear/gear_borrower_sheet.dart';
import 'package:ripls/presentation/widgets/gear/gear_interest_sheet.dart';

/// GearDetailsPane renders the Details tab (tab 1) content for a gear item.
///
/// Editing state (brand, model, value, weight, website) is owned by the parent
/// [_GearContentViewState] and passed in as mutable values with change
/// callbacks. This keeps all [setState] calls in one place and avoids the
/// complexity of a nested [StatefulWidget] that could lose state on rebuild.
class GearDetailsPane extends StatelessWidget {
  final GearState state;
  final String gearId;

  // Edit-mode metadata buffers (owned by parent state).
  final bool isEditingMetadataInitialized;
  final String editingBrand;
  final String editingModel;
  final String editingValueUsd;
  final String editingWeightGrams;
  final String editingWebsite;
  final ValueChanged<String> onBrandChanged;
  final ValueChanged<String> onModelChanged;
  final ValueChanged<String> onValueChanged;
  final ValueChanged<String> onWeightChanged;
  final ValueChanged<String> onWebsiteChanged;
  final VoidCallback onLocationTap;

  const GearDetailsPane({
    super.key,
    required this.state,
    required this.gearId,
    required this.isEditingMetadataInitialized,
    required this.editingBrand,
    required this.editingModel,
    required this.editingValueUsd,
    required this.editingWeightGrams,
    required this.editingWebsite,
    required this.onBrandChanged,
    required this.onModelChanged,
    required this.onValueChanged,
    required this.onWeightChanged,
    required this.onWebsiteChanged,
    required this.onLocationTap,
  });

  @override
  Widget build(BuildContext context) {
    final gear = state.gearDetails;
    if (gear == null) return const SizedBox.shrink();

    final metadata = gear.hasMetadata() ? gear.metadata : null;
    final valueEstimate = gear.hasValueEstimate() ? gear.valueEstimate : null;
    final isGiveaway = gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;
    final people = state.gearPeople;

    final stats = state.gearStats;
    final hasLoaned = stats != null && stats.timesLoaned > 0;
    final impact = hasLoaned
        ? (stats.hasImpact() ? stats.impact : null)
        : (stats?.hasPotentialImpact() ?? false ? stats!.potentialImpact : null);

    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (impact != null) ...[
            const SizedBox(height: 14),
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
          if (state.isEditing) ...[
            const SizedBox(height: 12),
            _buildInfoCardEdit(context, state, gear),
          ],
          const SizedBox(height: 12),
          if (state.isOwner && state.isEditing)
            _buildDetailRowsEdit(context, state, metadata)
          else
            _buildDetailRows(context, state, gear, metadata, valueEstimate),
          if (people != null && isGiveaway) ...[
            const SizedBox(height: 12),
            _buildPeopleRow(context, state, people, isGiveaway),
          ],
        ],
      ),
    );
  }

  Widget _buildPeopleRow(
    BuildContext context,
    GearState state,
    GetGearPeopleResponse people,
    bool isGiveaway,
  ) {
    if (isGiveaway) {
      final selected = people.interestedParties
          .where(
              (u) => u.id == state.transferContext?.userTransfer.recipient.id)
          .toList();
      final interested = people.interestedParties
          .where((u) => !selected.any((s) => s.id == u.id))
          .toList();
      final parts = <String>[];
      if (selected.isNotEmpty) {
        parts.add(context.l10n.gearSelectedCount(selected.length));
      }
      if (interested.isNotEmpty) {
        parts.add(context.l10n.gearInterestedCount(interested.length));
      }
      final summary =
          parts.isEmpty ? context.l10n.gearNoRequestsYet : parts.join(' · ');
      return ContentPeopleRow(
        users: people.interestedParties,
        summaryText: summary,
        onTap: () => GearInterestSheet.show(context, gearId),
      );
    }

    final currentCount = people.hasCurrentBorrower() ? 1 : 0;
    final pastCount = people.pastBorrowers.length;
    final parts = <String>[];
    if (currentCount > 0) {
      parts.add(context.l10n.gearBorrowingCount(currentCount));
    }
    if (pastCount > 0) {
      parts.add(context.l10n.gearReturnedCount(pastCount));
    }
    final summary =
        parts.isEmpty ? context.l10n.gearNoLoansYet : parts.join(' · ');
    final allUsers = <User>[
      if (people.hasCurrentBorrower()) people.currentBorrower,
      ...people.pastBorrowers,
    ];
    return ContentPeopleRow(
      users: allUsers,
      summaryText: summary,
      onTap: () => GearBorrowerSheet.show(context, gearId),
    );
  }

  Widget _buildDetailRows(
    BuildContext context,
    GearState state,
    GetGearResponse gear,
    GearMetadata? metadata,
    ValueEstimate? valueEstimate,
  ) {
    final brand = metadata?.brand.value ?? '';
    final model = metadata?.model.value ?? '';
    final materialCat = metadata?.materialCategory.value;
    final materialStr = materialCat != null
        ? GearMetadataFormatter.formatMaterialCategory(materialCat)
        : null;
    final weightStr = metadata != null
        ? GearMetadataFormatter.formatWeight(metadata.weightGrams.value)
        : null;
    final valueStr =
        valueEstimate != null && valueEstimate.hasEstimatedValueUsd()
            ? '\$${valueEstimate.estimatedValueUsd.toStringAsFixed(0)}'
            : null;

    final rows = <(String, String)>[
      if (brand.isNotEmpty) (context.l10n.gearDetailBrand, brand),
      if (model.isNotEmpty) (context.l10n.gearDetailModel, model),
      if (valueStr != null) (context.l10n.gearDetailEstimatedValue, valueStr),
      if (materialStr != null) (context.l10n.gearDetailMaterial, materialStr),
      if (weightStr != null) (context.l10n.gearDetailWeight, weightStr),
    ];

    return ContentDetailCard(rows: rows);
  }

  Widget _buildInfoCardEdit(
    BuildContext context,
    GearState state,
    GetGearResponse gear,
  ) {
    final enabled = !state.isSaving;
    return Container(
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.40),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: Colors.white.withValues(alpha: 0.10)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Tappable(
            semanticsLabel: context.l10n.a11yGearOpenLocation,
            onTap: onLocationTap,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
              child: Row(
                children: [
                  Icon(Icons.location_on, size: 20, color: AppColors.transferCoral),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      state.locationName?.isNotEmpty ?? false
                          ? state.locationName!
                          : context.l10n.gearDetailAddLocation,
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                        color: state.locationName?.isNotEmpty ?? false
                            ? Colors.white
                            : Colors.white.withValues(alpha: 0.35),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.08)),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14),
            child: Row(
              children: [
                Icon(Icons.link,
                    size: 20, color: Colors.white.withValues(alpha: 0.65)),
                const SizedBox(width: 12),
                Expanded(
                  child: TextFormField(
                    initialValue: editingWebsite,
                    onChanged: onWebsiteChanged,
                    enabled: enabled,
                    keyboardType: TextInputType.url,
                    maxLines: 1,
                    style: TextStyle(
                      fontSize: 16,
                      color: AppColors.overlayFieldText(),
                    ),
                    decoration: InputDecoration(
                      hintText: 'https://',
                      hintStyle: TextStyle(
                        fontSize: 16,
                        color: AppColors.overlayFieldHint(),
                      ),
                      filled: true,
                      fillColor: Colors.transparent,
                      isDense: true,
                      contentPadding:
                          const EdgeInsets.symmetric(vertical: 12),
                      border: InputBorder.none,
                      enabledBorder: InputBorder.none,
                      focusedBorder: InputBorder.none,
                      disabledBorder: InputBorder.none,
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildDetailRowsEdit(
    BuildContext context,
    GearState state,
    GearMetadata? metadata,
  ) {
    final enabled = !state.isSaving;
    final materialCat = metadata?.materialCategory.value;
    final materialStr = materialCat != null
        ? GearMetadataFormatter.formatMaterialCategory(materialCat)
        : null;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.40),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: Colors.white.withValues(alpha: 0.10)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          _buildEditableRow(
            context,
            context.l10n.gearDetailBrand,
            editingBrand,
            context.l10n.gearDetailBrandHint,
            enabled,
            onBrandChanged,
          ),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.08)),
          _buildEditableRow(
            context,
            context.l10n.gearDetailModel,
            editingModel,
            context.l10n.gearDetailModelHint,
            enabled,
            onModelChanged,
          ),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.08)),
          _buildEditableRow(
            context,
            context.l10n.gearDetailEstimatedValue,
            editingValueUsd,
            context.l10n.gearDetailValueHint,
            enabled,
            onValueChanged,
            keyboardType: TextInputType.number,
          ),
          Divider(height: 1, color: Colors.white.withValues(alpha: 0.08)),
          if (materialStr != null) ...[
            _buildReadOnlyRow(context, context.l10n.gearDetailMaterial, materialStr),
            Divider(height: 1, color: Colors.white.withValues(alpha: 0.08)),
          ],
          _buildEditableRow(
            context,
            context.l10n.gearDetailWeight,
            editingWeightGrams,
            context.l10n.gearDetailWeightHint,
            enabled,
            onWeightChanged,
            keyboardType: TextInputType.number,
          ),
        ],
      ),
    );
  }

  Widget _buildReadOnlyRow(BuildContext context, String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          SizedBox(
            width: 70,
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                color: Colors.white.withValues(alpha: 0.65),
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              value,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: Colors.white.withValues(alpha: 0.65),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildEditableRow(
    BuildContext context,
    String label,
    String value,
    String hintText,
    bool enabled,
    ValueChanged<String> onChanged, {
    TextInputType? keyboardType,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          SizedBox(
            width: 70,
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                color: AppColors.overlayFieldHint(),
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: TextFormField(
              initialValue: value,
              onChanged: onChanged,
              enabled: enabled,
              keyboardType: keyboardType,
              maxLines: 1,
              textAlign: TextAlign.right,
              style: TextStyle(
                fontSize: 16,
                color: AppColors.overlayFieldText(),
              ),
              decoration: InputDecoration(
                hintText: hintText,
                hintStyle: TextStyle(
                  fontSize: 16,
                  color: AppColors.overlayFieldHint(),
                ),
                filled: true,
                fillColor: Colors.transparent,
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(vertical: 10),
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                disabledBorder: InputBorder.none,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
