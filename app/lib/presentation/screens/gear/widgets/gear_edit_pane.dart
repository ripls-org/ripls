import 'package:flutter/material.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_details_pane.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_sharing_mode_toggle.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';

/// GearEditPane renders the editable title + description fields plus the gear
/// metadata form in edit mode. Read mode lives in `GearReadShell`; this surface
/// is shown beneath the top edit bar (Cancel / Save) when `state.isEditing`.
///
/// The metadata form is delegated to [GearDetailsPane] (which renders the
/// editable brand / model / value / weight / website fields with the parent's
/// buffers) so the gear-specific debounced auto-save behaviour is preserved.
class GearEditPane extends StatelessWidget {
  final GearState state;
  final String gearId;

  final String? editingTitle;
  final String? editingDescription;
  final ValueChanged<String>? onTitleChanged;
  final ValueChanged<String>? onDescriptionChanged;

  // Metadata buffers + callbacks (owned by the parent state).
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

  const GearEditPane({
    super.key,
    required this.state,
    required this.gearId,
    required this.editingTitle,
    required this.editingDescription,
    required this.onTitleChanged,
    required this.onDescriptionChanged,
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
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          ContentViewBuilders.buildEditableTitle(
            value: editingTitle ?? '',
            onChanged: onTitleChanged ?? (_) {},
            enabled: true,
            label: 'Item Title',
            hintText: 'Item title',
          ),
          const SizedBox(height: 8),
          ContentViewBuilders.buildEditableDescription(
            value: editingDescription ?? '',
            onChanged: onDescriptionChanged ?? (_) {},
            enabled: true,
            label: 'Description',
            hintText: 'Item description',
          ),
          const SizedBox(height: 16),
          GearSharingModeToggle(gearId: gearId),
          GearDetailsPane(
            state: state,
            gearId: gearId,
            isEditingMetadataInitialized: isEditingMetadataInitialized,
            editingBrand: editingBrand,
            editingModel: editingModel,
            editingValueUsd: editingValueUsd,
            editingWeightGrams: editingWeightGrams,
            editingWebsite: editingWebsite,
            onBrandChanged: onBrandChanged,
            onModelChanged: onModelChanged,
            onValueChanged: onValueChanged,
            onWeightChanged: onWeightChanged,
            onWebsiteChanged: onWebsiteChanged,
            onLocationTap: onLocationTap,
          ),
        ],
      ),
    );
  }
}
