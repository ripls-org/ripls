import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/presentation/screens/create/unified_create_input_drawer.dart'
    show UnifiedPillTabs, UnifiedPillTabEntry;
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';

/// Pill-shape 3-segment type selector for the unified-create preview
/// card. Single-select. Selection indicated by background-only
/// (no checkmark). Tapping a non-selected segment calls
/// `viewModel.flipType`; no-op while [state.selectorEnabled] is false.
class UnifiedTypeSelector extends ConsumerWidget {
  const UnifiedTypeSelector({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(unifiedCreateViewModelProvider);
    final vm = ref.read(unifiedCreateViewModelProvider.notifier);
    final enabled = state.selectorEnabled;

    return Opacity(
      opacity: enabled ? 1 : 0.6,
      child: AbsorbPointer(
        absorbing: !enabled,
        child: UnifiedPillTabs<DetectedContentType?>(
          // Null until the classifier's first `type` event: NO segment reads
          // as selected while detection is pending, so the sheet never shows
          // a wrong preselected tab that flips a beat later (#2724).
          value: state.type,
          entries: [
            UnifiedPillTabEntry(
              value: DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST,
              label: context.l10n.unifiedCreateTypeRequest,
            ),
            UnifiedPillTabEntry(
              value: DetectedContentType.DETECTED_CONTENT_TYPE_EVENT,
              label: context.l10n.unifiedCreateTypeEvent,
            ),
            UnifiedPillTabEntry(
              value: DetectedContentType.DETECTED_CONTENT_TYPE_GEAR,
              label: context.l10n.unifiedCreateTypeItem,
            ),
          ],
          onChange: (t) {
            if (t != null && t != state.type) {
              vm.flipType(t);
            }
          },
        ),
      ),
    );
    // Note: `AppColors` import not directly used; left in case downstream
    // styling diverges from the shared pill component.
  }
}

// Suppress unused-import linter complaint for AppColors when the file
// later references a token directly.
// ignore_for_file: unused_import
