import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_section/open_now_list.dart';

/// The person profile's Library-row destination (issue #2568, hybrid
/// v3): everything shareable between the viewer and the target, as a
/// plain pushed list. The v3 mock replaces the profile's inline
/// inventory with a number-only Library row and leaves the destination
/// undesigned — this minimal dark list preserves the reachability the
/// v2 sheet's expanded "everything open" list provided, reusing the
/// same [OpenNowList] rows, until the Library surface is designed.
class ProfileLibraryScreen extends StatelessWidget {
  const ProfileLibraryScreen({super.key, required this.items});

  final List<Item> items;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Scaffold(
      backgroundColor: AppColors.darkBackground,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 4, 12, 4),
              child: Tappable(
                semanticsLabel:
                    MaterialLocalizations.of(context).backButtonTooltip,
                onTap: () => Navigator.of(context).maybePop(),
                child: Container(
                  width: 38,
                  height: 38,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: Colors.white.withValues(alpha: 0.08),
                    border: Border.all(
                        color: Colors.white.withValues(alpha: 0.18)),
                  ),
                  child: const Icon(Icons.arrow_back_ios_new,
                      size: 16, color: Colors.white),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 4),
              child: Text(
                l10n.profileSheetEverythingOpen,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 24,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
            ),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.only(bottom: 24),
                child: OpenNowList(
                  items: items,
                  onOpenItem: (item) =>
                      NavigationHelpers.pushToItem(context: context, item: item),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
