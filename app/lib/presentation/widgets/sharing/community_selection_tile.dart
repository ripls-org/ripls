import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/group_avatar.dart';

/// How the parent picker lets the user select rows.
///
/// The three render as different controls: [multi] shows a `Switch` (a live
/// on/off toggle — used by the manage-access editor), [multiCheck] shows a
/// checkmark indicator (a batch multi-select that's committed on Confirm — used
/// by the additive invite/create pickers), and [single] shows a radio
/// indicator. The choice is purely visual — the row's tap surface and selection
/// state still live in the caller.
enum CommunitySelectionStyle { multi, multiCheck, single }

/// CommunitySelectionTile is the shared visual row used across all
/// community selection modals (post-creation sharing, bulk sharing, and
/// pre-creation picker). It renders an avatar, name, optional subtitle,
/// and either a `Switch` or a radio indicator depending on
/// [CommunitySelectionStyle].
///
/// All logic (what "selected" means, what happens on tap) lives in the
/// caller — this widget is purely presentational.
class CommunitySelectionTile extends StatelessWidget {
  final String name;
  final bool isSelected;
  final bool isLoading;
  final ValueChanged<bool>? onChanged;
  final CommunitySelectionStyle style;

  /// Optional secondary line below the community name.
  final String? subtitle;

  /// Members to depict as a group-avatar cluster for a nameless (ad-hoc)
  /// community — the viewer first, then member-preview faces. When empty the
  /// tile shows the usual single initial circle.
  final List<User> groupMembers;

  const CommunitySelectionTile({
    super.key,
    required this.name,
    required this.isSelected,
    this.isLoading = false,
    this.onChanged,
    this.subtitle,
    this.style = CommunitySelectionStyle.multi,
    this.groupMembers = const [],
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: AppColors.modalInsetCardBg,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.modalInsetCardBorder),
      ),
      child: Row(
        children: [
          if (groupMembers.isNotEmpty)
            GroupAvatar(
              members: groupMembers,
              diameter: 40,
              ringColor: AppColors.modalInsetCardBg,
            )
          else
            CircleAvatar(
              radius: 20,
              backgroundColor: isSelected
                  ? AppColors.experienceSageGreen
                  : AppColors.modalTextMuted.withValues(alpha: 0.4),
              child: Text(
                name.isNotEmpty ? name[0].toUpperCase() : 'C',
                style: TextStyle(
                  color: AppColors.modalTextPrimary,
                  fontWeight: FontWeight.bold,
                  fontSize: 16,
                ),
              ),
            ),
          const SizedBox(width: 12),
          Expanded(
            child: subtitle != null
                ? Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        style: TextStyle(
                          color: AppColors.modalTextPrimary,
                          fontSize: 16,
                          fontWeight: FontWeight.w500,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                      Padding(
                        padding: const EdgeInsets.only(top: 4),
                        child: Text(
                          subtitle!,
                          style: TextStyle(
                            color: AppColors.modalTextSecondary,
                            fontSize: 12,
                          ),
                        ),
                      ),
                    ],
                  )
                : Text(
                    name,
                    style: TextStyle(
                      color: AppColors.modalTextPrimary,
                      fontSize: 16,
                      fontWeight: FontWeight.w500,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
          ),
          const SizedBox(width: 12),
          _buildTrailing(),
        ],
      ),
    );
  }

  Widget _buildTrailing() {
    if (isLoading) {
      return SizedBox(
        width: 24,
        height: 24,
        child: CircularProgressIndicator(
          strokeWidth: 2,
          valueColor: AlwaysStoppedAnimation<Color>(
            AppColors.modalPrimaryButtonBackground,
          ),
        ),
      );
    }
    switch (style) {
      case CommunitySelectionStyle.multi:
        return Switch(
          value: isSelected,
          onChanged: onChanged,
          activeTrackColor: AppColors.modalPrimaryButtonBackground,
        );
      case CommunitySelectionStyle.multiCheck:
      case CommunitySelectionStyle.single:
        return _RadioIndicator(isSelected: isSelected);
    }
  }
}

/// Decorative radio indicator for [CommunitySelectionStyle.single] rows.
///
/// Not a tap target — the row-level `Toggle` wrapper in the parent picker
/// is the only thing that consumes taps. The radio role is announced to
/// assistive tech by passing `inMutuallyExclusiveGroup: true` on that
/// outer `Toggle`, not by this widget.
class _RadioIndicator extends StatelessWidget {
  final bool isSelected;

  const _RadioIndicator({required this.isSelected});

  @override
  Widget build(BuildContext context) {
    // Selected: a solid sage disc with a white check — high contrast on the
    // dark glass. Unselected: an empty muted ring.
    return Container(
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(
          color: isSelected
              ? AppColors.experienceSageGreen
              : AppColors.modalTextMuted.withValues(alpha: 0.6),
          width: 2,
        ),
        color:
            isSelected ? AppColors.experienceSageGreen : Colors.transparent,
      ),
      child: isSelected
          ? const Icon(
              Icons.check,
              size: 16,
              color: GlassTokens.textPrimary,
            )
          : null,
    );
  }
}
