import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/discover_map_helper.dart'
    show mapStyleDark, mapStyleOutdoor, mapStyleSatellite, mapStyleStandard;
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/solid_sheet.dart';

/// MapStyleSheet is the standalone picker for the Discover map's base
/// style (#2634). It sits on the opaque (non-glass) [SolidSheet] surface —
/// the same Me-sheet grammar as the Library location sheet — and is opened
/// from the map-style button floating on the map, its own affordance now
/// that map styling is no longer bundled with result filtering.
///
/// The selection is applied immediately via [searchProvider]'s
/// `setMapStyle`; dismiss by swiping down or tapping the X button.
class MapStyleSheet extends ConsumerWidget {
  const MapStyleSheet({super.key});

  /// Shows the sheet as a modal bottom sheet.
  ///
  /// Uses [useRootNavigator] so the sheet renders above the bottom nav bar.
  static Future<void> show(BuildContext context) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      useRootNavigator: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => const MapStyleSheet(),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final currentStyle = ref.watch(
      searchProvider.select(
        (s) => s.value?.filters.mapStyle ?? mapStyleStandard,
      ),
    );

    final mapStyles = [
      (label: context.l10n.discoverFilterMapDefault, uri: mapStyleStandard),
      (label: context.l10n.discoverFilterMapOutdoor, uri: mapStyleOutdoor),
      (label: context.l10n.discoverFilterMapSatellite, uri: mapStyleSatellite),
      (label: context.l10n.discoverFilterMapDark, uri: mapStyleDark),
    ];

    return SolidSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildHeader(context),
          const SizedBox(height: 14),
          Row(
            children: mapStyles.map((style) {
              return Expanded(
                child: Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: _buildMapStyleTile(
                    context,
                    ref,
                    label: style.label,
                    styleUri: style.uri,
                    isActive: currentStyle == style.uri,
                  ),
                ),
              );
            }).toList(),
          ),
        ],
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Row(
      children: [
        Text(
          context.l10n.discoverFilterMapStyle,
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 20,
            fontWeight: FontWeight.w600,
            color: AppColors.textPrimary(context),
          ),
        ),
        const Spacer(),
        IconAction(
          icon: Icons.close,
          color: AppColors.textSecondary(context),
          semanticsLabel: context.l10n.a11yClose,
          onPressed: () => Navigator.of(context).pop(),
          padding: EdgeInsets.zero,
        ),
      ],
    );
  }

  Widget _buildMapStyleTile(
    BuildContext context,
    WidgetRef ref, {
    required String label,
    required String styleUri,
    required bool isActive,
  }) {
    final accent = AppColors.primary(context);
    final gradient = _mapStyleGradient(styleUri);
    return Toggle(
      semanticsLabel: context.l10n.a11yDiscoverFilterMapStyle(label),
      selected: isActive,
      onTap: () {
        ref.read(searchProvider.notifier).setMapStyle(styleUri);
        Navigator.of(context).pop();
      },
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 200)),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(10),
          border: Border.all(
            color: isActive ? accent : AppColors.border(context),
            width: isActive ? 2 : 1,
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ClipRRect(
              borderRadius: const BorderRadius.vertical(top: Radius.circular(9)),
              child: Container(
                height: 52,
                decoration: BoxDecoration(gradient: gradient),
              ),
            ),
            Container(
              padding: const EdgeInsets.symmetric(vertical: 6),
              decoration: BoxDecoration(
                color: AppColors.surface(context),
                borderRadius: const BorderRadius.vertical(
                  bottom: Radius.circular(9),
                ),
              ),
              child: Text(
                label,
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w500,
                  color: isActive ? accent : AppColors.textSecondary(context),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Returns a gradient approximating each map style's visual appearance.
  LinearGradient _mapStyleGradient(String styleUri) {
    switch (styleUri) {
      case mapStyleOutdoor:
        return const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF8DAA7A), Color(0xFF6B9E69)],
        );
      case mapStyleSatellite:
        return const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF2B4A2E), Color(0xFF1A3020)],
        );
      case mapStyleDark:
        return const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF2C2C3E), Color(0xFF1A1A2E)],
        );
      default: // STANDARD / Streets
        return const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFFE8D9C8), Color(0xFFD4C4A8)],
        );
    }
  }
}
