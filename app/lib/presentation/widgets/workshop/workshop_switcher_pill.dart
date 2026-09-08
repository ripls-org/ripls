import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_menu_avatar.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// The community switcher rendered inside the Workshop overview's shared
/// [FloatingHeader] (community-switcher-dropdown-v1). At rest it shows the
/// selected community name + dropdown chevron, with the profile-menu avatar on
/// the right. When [searching] is true the pill morphs into the dropdown's
/// search field — the name/avatar give way to a magnifier, a text field, and a
/// clear button.
class WorkshopSwitcherPill extends ConsumerWidget {
  /// True while the switcher dropdown is open — the pill becomes the search box.
  final bool searching;

  /// Opens the dropdown (tapping the resting pill).
  final VoidCallback onTap;

  final TextEditingController controller;
  final ValueChanged<String> onQueryChanged;

  const WorkshopSwitcherPill({
    super.key,
    required this.searching,
    required this.onTap,
    required this.controller,
    required this.onQueryChanged,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (searching) return _searchField(context);

    final communitiesState = ref.watch(communitiesProvider);
    final active = ref.watch(workshopActiveCommunityProvider);
    // A nameless (ad-hoc) community has an empty `name`, so render it group-text
    // style ("Ada, Sam +2") rather than letting the pill go blank (#2492).
    final label = active != null
        ? communityDisplayName(active, context.l10n)
        : CommunityHelper.communitiesTitle(communitiesState, context.l10n);

    return Stack(
      alignment: Alignment.center,
      children: [
        Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 56),
            child: Tappable(
              semanticsLabel: context.l10n.a11yWorkshopSwitchCircle,
              onTap: onTap,
              inkBorderRadius: BorderRadius.circular(24),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Flexible(
                    child: Text(
                      label,
                      textAlign: TextAlign.center,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: AppTheme.communityHeaderStyle.copyWith(
                        color: AppColors.textPrimary(context),
                      ),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Icon(
                    Icons.unfold_more,
                    size: 16,
                    color: AppColors.textSecondary(context),
                  ),
                ],
              ),
            ),
          ),
        ),
        const Align(
          alignment: Alignment.centerRight,
          child: ProfileMenuAvatar(),
        ),
      ],
    );
  }

  /// The active search input — matched to the Feed's `SearchNearbyPill` active
  /// row (icon 18, 8px gap, 13px text, collapsed borderless field). The close
  /// (✕) lives in the FloatingHeader's trailing slot, like the Feed.
  Widget _searchField(BuildContext context) {
    return Row(
      children: [
        Icon(Icons.search, size: 18, color: AppColors.textSecondary(context)),
        const SizedBox(width: 8),
        Expanded(
          child: TextField(
            controller: controller,
            autofocus: true,
            onChanged: onQueryChanged,
            textInputAction: TextInputAction.search,
            style: TextStyle(
              fontSize: 13,
              color: AppColors.textPrimary(context),
            ),
            decoration: InputDecoration(
              isCollapsed: true,
              contentPadding: EdgeInsets.zero,
              filled: false,
              fillColor: Colors.transparent,
              border: InputBorder.none,
              enabledBorder: InputBorder.none,
              focusedBorder: InputBorder.none,
              hintText: context.l10n.workshopChooserSearchHint,
              hintStyle: TextStyle(
                fontSize: 13,
                color: AppColors.textSecondary(context),
              ),
            ),
          ),
        ),
      ],
    );
  }
}
