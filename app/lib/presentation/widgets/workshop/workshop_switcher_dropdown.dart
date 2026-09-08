import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show authStateProvider;
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// The community-switcher dropdown that hangs off the Workshop top-nav pill
/// (community-switcher-dropdown-v1): a single ranked list of the user's
/// communities filtered by the pill's search field, the active one checked,
/// plus a "Create a community" footer. Replaces the old full-screen chooser.
class WorkshopSwitcherDropdown extends ConsumerWidget {
  /// Search text from the pill (the pill becomes the search field when open).
  final String query;
  final ValueChanged<String> onSelect;
  final VoidCallback onCreate;

  const WorkshopSwitcherDropdown({
    super.key,
    required this.query,
    required this.onSelect,
    required this.onCreate,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final communities =
        ref.watch(communitiesProvider.select((s) => s.communities));
    final activeId = ref.watch(workshopActiveCommunityProvider)?.id;
    // The owner of a nameless (ad-hoc) community can promote it to a real one
    // (name + photo + description): tapping its row opens the create/AI modal
    // in promote mode rather than selecting the community (#2492).
    final viewer = ref.watch(authStateProvider.select((s) => s.user));

    final q = query.trim().toLowerCase();
    final filtered = q.isEmpty
        ? communities
        : communities
            .where((c) =>
                communityDisplayName(c, l10n).toLowerCase().contains(q))
            .toList();

    return Material(
      color: AppColors.cardBackground(context),
      borderRadius: BorderRadius.circular(22),
      clipBehavior: Clip.antiAlias,
      elevation: 10,
      shadowColor: const Color(0x4D141612),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(18, 15, 18, 9),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Text(
                l10n.workshopSwitcherTitle(communities.length),
                style: TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  letterSpacing: -0.1,
                  color: AppColors.textPrimary(context),
                ),
              ),
            ),
          ),
          Flexible(
            child: filtered.isEmpty
                ? Padding(
                    padding: const EdgeInsets.symmetric(vertical: 24),
                    child: Text(
                      l10n.workshopSwitcherNoMatches,
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 13,
                        color: AppColors.textTertiary(context),
                      ),
                    ),
                  )
                : ListView(
                    shrinkWrap: true,
                    padding: const EdgeInsets.symmetric(horizontal: 8),
                    children: [
                      for (final c in filtered)
                        _communityRow(context, c, c.id == activeId, viewer),
                    ],
                  ),
          ),
          _Footer(label: l10n.workshopSwitcherCreate, onTap: onCreate),
        ],
      ),
    );
  }

  /// Builds one community row. Tapping always *selects* the community — a
  /// nameless (ad-hoc) one opens its overview like any other, where the owner
  /// can name it via the header action pill (#2492). [viewer] leads the group
  /// avatar so even a two-person nameless group shows two circles.
  Widget _communityRow(
    BuildContext context,
    CommunityItem c,
    bool selected,
    User? viewer,
  ) {
    return _CommunityRow(
      community: c,
      selected: selected,
      viewer: viewer,
      onTap: () => onSelect(c.id),
    );
  }
}

class _CommunityRow extends StatelessWidget {
  final CommunityItem community;
  final bool selected;
  final VoidCallback onTap;

  /// The current user, placed first in a nameless community's group avatar so
  /// even a two-person group shows two circles. May be null if unresolved.
  final User? viewer;

  const _CommunityRow({
    required this.community,
    required this.selected,
    required this.onTap,
    this.viewer,
  });

  @override
  Widget build(BuildContext context) {
    final green = AppColors.primary(context);
    final l10n = context.l10n;
    final displayName = communityDisplayName(community, l10n);
    // A nameless community reads as a group: a cluster of member faces (the
    // viewer + member-preview names) rather than one community circle, and —
    // instead of forcing a name on tap — a subtitle naming the item that
    // spawned it ("Group from Dinner at Este"). Tapping just selects it; the
    // owner names it from the overview's header action pill (#2492).
    final isNameless = community.name.trim().isEmpty;
    final originName = community.originItemName.trim();
    final groupMembers = groupAvatarMembers(
      viewer: viewer,
      memberPreviewFirstNames: community.memberPreviewFirstNames,
    );
    return Tappable(
      semanticsLabel: displayName,
      semanticsIdentifier: isNameless ? 'workshop-nameless-row' : null,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 1),
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: selected ? green.withValues(alpha: 0.14) : null,
          borderRadius: BorderRadius.circular(14),
        ),
        child: Row(
          children: [
            if (isNameless && groupMembers.isNotEmpty)
              GroupAvatar(
                members: groupMembers,
                diameter: 42,
                ringColor: AppColors.cardBackground(context),
              )
            else
              CommunityAvatar(community: community, radius: 21),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    displayName,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontFamily: AppTheme.headingFont,
                      fontSize: 16,
                      height: 1.2,
                      fontWeight: FontWeight.w500,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                  // Identify a nameless per-item community by what spawned it.
                  if (isNameless && originName.isNotEmpty)
                    Text(
                      l10n.workshopGroupFromItem(originName),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 12.5,
                        height: 1.3,
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                ],
              ),
            ),
            if (selected) Icon(Icons.check, size: 18, color: green),
          ],
        ),
      ),
    );
  }
}

class _Footer extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  const _Footer({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: AppColors.surface(context))),
      ),
      padding: const EdgeInsets.fromLTRB(10, 10, 10, 12),
      child: Tappable(
        semanticsLabel: label,
        onTap: onTap,
        inkBorderRadius: BorderRadius.circular(999),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 12),
          decoration: BoxDecoration(
            color: AppColors.primary(context),
            borderRadius: BorderRadius.circular(999),
          ),
          alignment: Alignment.center,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.add, size: 17, color: Colors.white),
              const SizedBox(width: 6),
              Text(
                label,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

