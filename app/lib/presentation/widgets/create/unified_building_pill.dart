import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';

/// Subtle spinner + "Building..." label shown while the unified-create
/// stream is in flight. Becomes the user-visible cue that the type
/// selector is currently locked out.
class UnifiedBuildingPill extends ConsumerWidget {
  const UnifiedBuildingPill({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final streaming = ref.watch(
      unifiedCreateViewModelProvider.select((s) => s.streaming),
    );
    if (!streaming) return const SizedBox.shrink();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.modalChipBackground,
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: AppColors.modalChipBorder),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: 12,
            height: 12,
            child: CircularProgressIndicator(
              strokeWidth: 1.8,
              valueColor:
                  AlwaysStoppedAnimation<Color>(AppColors.modalTextPrimary),
            ),
          ),
          const SizedBox(width: 7),
          Text(
            context.l10n.unifiedCreateBuilding,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
              letterSpacing: 0.3,
            ),
          ),
        ],
      ),
    );
  }
}
