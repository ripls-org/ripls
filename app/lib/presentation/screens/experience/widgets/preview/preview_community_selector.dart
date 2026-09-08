import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/services/providers.dart';

/// PreviewCommunitySelector renders a tappable community picker row used in
/// the experience preview modal.
///
/// Reads the available community list from [communitiesProvider] to
/// resolve a human-readable label for the current selection.
/// [selectedCommunityIds] is the list of IDs currently selected; [onTap]
/// opens the community picker sheet.
class PreviewCommunitySelector extends ConsumerWidget {
  const PreviewCommunitySelector({
    super.key,
    required this.selectedCommunityIds,
    required this.onTap,
  });

  final List<String> selectedCommunityIds;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final communities = ref.watch(communitiesProvider).communities;
    final label = selectedCommunityIds.isEmpty
        ? context.l10n.communitySelectPlaceholder
        : selectedCommunityIds.length == 1
            ? communityDisplayName(
                communities
                    .firstWhere((c) => c.id == selectedCommunityIds.first),
                context.l10n,
              )
            : context.l10n.communitySelectCount(selectedCommunityIds.length);

    return Tappable(
      semanticsLabel: context.l10n.a11yExpSelectCommunities,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          children: [
            const Icon(
              Icons.group_outlined,
              color: GlassTokens.textMuted,
              size: 20,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  color: GlassTokens.textPrimary,
                  fontSize: 15,
                ),
              ),
            ),
            const Icon(
              Icons.chevron_right,
              color: GlassTokens.textMuted,
              size: 20,
            ),
          ],
        ),
      ),
    );
  }
}
