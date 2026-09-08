import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Shared presentation widgets for the full-screen poll panels — "Where"
/// (location) and "When" (time). Both panels render the same lettered option
/// rows, badges, voter stacks, unreplied strip, confirm breakdown, and action
/// buttons on the dark media scrim (white-on-dark). These widgets are
/// domain-agnostic: they take pre-resolved strings + `List<User>` voters, never
/// a location/time vote proto.

/// `index` → display letter (0 → A, 1 → B, …) for an option / marker.
String pollOptionLetter(int index) => String.fromCharCode(65 + index);

/// The shared top vote count when 2+ options tie for the lead, else 0. [count]
/// resolves an option id → its effective support.
int pollTiedTopCount(List<String> ids, int Function(String id) count) {
  var top = 0;
  var topShared = false;
  for (final id in ids) {
    final n = count(id);
    if (n > top) {
      top = n;
      topShared = false;
    } else if (n == top && n > 0) {
      topShared = true;
    }
  }
  return topShared ? top : 0;
}

/// The A/B/C square chip that tags each option (and its map pin / calendar day).
class PollLetterChip extends StatelessWidget {
  final String letter;
  final Color accentColor;

  /// Filled (accent background) for the leader / a voted option; otherwise an
  /// outlined accent chip.
  final bool filled;
  final double size;

  const PollLetterChip({
    super.key,
    required this.letter,
    required this.accentColor,
    this.filled = false,
    this.size = 30,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: filled ? accentColor : Colors.transparent,
        border: Border.all(color: accentColor, width: 2),
        borderRadius: BorderRadius.circular(9),
      ),
      child: Text(
        letter,
        style: TextStyle(
          color: filled ? const Color(0xFF10140F) : accentColor,
          fontSize: size * 0.46,
          fontWeight: FontWeight.w800,
        ),
      ),
    );
  }
}

/// Overlapping stack of YES-voter avatars with a trailing count. Renders
/// nothing when no one has voted yet.
class PollVoterStack extends StatelessWidget {
  final List<User> voters;
  const PollVoterStack({super.key, required this.voters});

  @override
  Widget build(BuildContext context) {
    if (voters.isEmpty) return const SizedBox.shrink();
    final shown = voters.take(3).toList();
    const double d = 18;
    const double overlap = 7;
    final width = d + (shown.length - 1) * (d - overlap);
    return Container(
      padding: const EdgeInsets.fromLTRB(4, 3, 8, 3),
      decoration: BoxDecoration(
        color: AppColors.onContentImage.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(100),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: width,
            height: d + 4,
            child: Stack(
              clipBehavior: Clip.none,
              children: [
                for (var i = 0; i < shown.length; i++)
                  Positioned(
                    left: i * (d - overlap),
                    top: 0,
                    child: Container(
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                          color: AppColors.modalBackdrop,
                          width: 2,
                        ),
                      ),
                      child: UserAvatar(user: shown[i], radius: 9),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 4),
          Text(
            '${voters.length}',
            style: const TextStyle(
              color: AppColors.onContentImage,
              fontSize: 12,
              fontWeight: FontWeight.w800,
            ),
          ),
        ],
      ),
    );
  }
}

/// One poll option row: letter chip · name/subtitle/evidence · MOST PICKS /
/// TIED badge · voter stack · vote check. Tapping commits / clears the viewer's
/// YES vote (auto-submit).
class PollOptionRow extends StatelessWidget {
  final String letter;
  final String name;
  final String? subtitle;

  /// Optional decision-relevant evidence line (e.g. "~2 mi away", a calendar
  /// conflict note). [evidenceBad] tints it amber for a conflict / warning.
  final String? evidence;
  final bool evidenceBad;

  /// Optional leading glyph for the evidence line (🚶 for travel, 🗓 for time).
  final String? evidenceGlyph;
  final String? addedByName;
  final List<User> voters;
  final bool voted;
  final bool leading;
  final bool tied;
  final bool enabled;
  final Color accentColor;
  final VoidCallback onTap;

  const PollOptionRow({
    super.key,
    required this.letter,
    required this.name,
    required this.voters,
    required this.voted,
    required this.leading,
    required this.tied,
    required this.enabled,
    required this.accentColor,
    required this.onTap,
    this.subtitle,
    this.evidence,
    this.evidenceBad = false,
    this.evidenceGlyph,
    this.addedByName,
  });

  @override
  Widget build(BuildContext context) {
    final bg = voted
        ? accentColor.withValues(alpha: 0.22)
        : AppColors.onContentImage.withValues(alpha: 0.055);
    final border = voted
        ? accentColor.withValues(alpha: 0.55)
        : (leading
            ? accentColor.withValues(alpha: 0.55)
            : AppColors.darkBorder);

    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Toggle(
        semanticsLabel: name,
        selected: voted,
        onTap: enabled ? onTap : null,
        child: Container(
          decoration: BoxDecoration(
            color: bg,
            border: Border.all(color: border, width: 1.5),
            borderRadius: BorderRadius.circular(18),
          ),
          padding: const EdgeInsets.all(14),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              PollLetterChip(
                letter: letter,
                accentColor: accentColor,
                filled: voted || leading,
              ),
              const SizedBox(width: 12),
              Expanded(child: _main(context)),
              const SizedBox(width: 8),
              // Badge + voter stack column, then the vote check — both
              // top-aligned siblings (not stacked) so a voted/leading row
              // stays as short as an unvoted one.
              _rightMeta(context),
              const SizedBox(width: 10),
              _check(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _main(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          name,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(
            color: AppColors.onContentImage,
            fontSize: 16,
            fontWeight: FontWeight.w700,
            height: 1.25,
          ),
        ),
        if (subtitle != null && subtitle!.isNotEmpty) ...[
          const SizedBox(height: 3),
          Text(
            subtitle!,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: AppColors.darkTextSecondary,
              fontSize: 13,
              height: 1.3,
            ),
          ),
        ],
        if (evidence != null && evidence!.isNotEmpty) ...[
          const SizedBox(height: 5),
          Text(
            evidenceGlyph != null ? '$evidenceGlyph $evidence' : evidence!,
            style: TextStyle(
              color: evidenceBad
                  ? AppColors.statusWarningOnDark
                  : AppColors.darkTextTertiary,
              fontSize: 12,
              fontWeight: evidenceBad ? FontWeight.w700 : FontWeight.w400,
            ),
          ),
        ],
        if (addedByName != null && addedByName!.isNotEmpty) ...[
          const SizedBox(height: 4),
          Text(
            context.l10n.locationPollVoteAddedBy(addedByName!),
            style: const TextStyle(
              color: AppColors.darkTextTertiary,
              fontSize: 11,
              fontWeight: FontWeight.w500,
            ),
          ),
        ],
      ],
    );
  }

  Widget _rightMeta(BuildContext context) {
    final badge = leading
        ? _PollBadge(
            label: context.l10n.locationPollVoteLeadingBadge,
            color: accentColor,
            filled: true,
          )
        : tied
            ? _PollBadge(
                label: context.l10n.locationPanelTiedBadge,
                color: null,
                filled: false,
              )
            : null;
    final hasVoters = voters.isNotEmpty;
    if (badge == null && !hasVoters) return const SizedBox.shrink();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.end,
      mainAxisSize: MainAxisSize.min,
      children: [
        ?badge,
        if (badge != null && hasVoters) const SizedBox(height: 6),
        if (hasVoters) PollVoterStack(voters: voters),
      ],
    );
  }

  Widget _check() {
    return Container(
      width: 26,
      height: 26,
      decoration: BoxDecoration(
        color: voted ? accentColor : Colors.transparent,
        border: voted
            ? null
            : Border.all(
                color: AppColors.onContentImage.withValues(alpha: 0.3),
                width: 2,
              ),
        borderRadius: BorderRadius.circular(8),
      ),
      alignment: Alignment.center,
      child: voted
          ? const Icon(Icons.check, size: 15, color: Color(0xFF10140F))
          : null,
    );
  }
}

class _PollBadge extends StatelessWidget {
  final String label;
  final Color? color;
  final bool filled;
  const _PollBadge({required this.label, required this.color, required this.filled});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: filled ? color : AppColors.onContentImage.withValues(alpha: 0.12),
        border: filled ? null : Border.all(color: AppColors.darkBorder),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(
        label.toUpperCase(),
        style: TextStyle(
          color: filled ? const Color(0xFF10140F) : AppColors.darkTextSecondary,
          fontSize: 10,
          fontWeight: FontWeight.w800,
          letterSpacing: 0.7,
        ),
      ),
    );
  }
}

/// The "add an option" dashed affordance at the foot of the poll list.
class PollAddRow extends StatelessWidget {
  final String label;
  final Color accentColor;
  final VoidCallback? onTap;
  const PollAddRow({
    super.key,
    required this.label,
    required this.accentColor,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Opacity(
        opacity: onTap == null ? 0.5 : 1,
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            border: Border.all(
              color: AppColors.onContentImage.withValues(alpha: 0.22),
              width: 1.5,
            ),
            borderRadius: BorderRadius.circular(16),
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.add, size: 18, color: accentColor),
              const SizedBox(width: 8),
              Text(
                label,
                style: const TextStyle(
                  color: AppColors.onContentImage,
                  fontSize: 14.5,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Full-width action button. [primary] fills with the accent; otherwise an
/// outlined ghost. Disabled when [onTap] is null. The label is flexible +
/// ellipsized so it never overflows a half-width slot.
class PollPanelButton extends StatelessWidget {
  final String label;
  final IconData? icon;
  final Color accentColor;
  final bool primary;
  final String? semanticsLabel;
  final VoidCallback? onTap;

  const PollPanelButton({
    super.key,
    required this.label,
    required this.accentColor,
    required this.onTap,
    this.icon,
    this.primary = false,
    this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    final fg = primary ? const Color(0xFF10140F) : AppColors.onContentImage;
    return Tappable(
      semanticsLabel: semanticsLabel ?? label,
      onTap: onTap,
      child: Opacity(
        opacity: onTap == null ? 0.5 : 1,
        child: Container(
          height: 52,
          padding: const EdgeInsets.symmetric(horizontal: 14),
          decoration: BoxDecoration(
            color: primary
                ? accentColor
                : AppColors.onContentImage.withValues(alpha: 0.10),
            border: primary
                ? null
                : Border.all(
                    color: AppColors.onContentImage.withValues(alpha: 0.30)),
            borderRadius: BorderRadius.circular(16),
          ),
          alignment: Alignment.center,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 18, color: primary ? fg : accentColor),
                const SizedBox(width: 8),
              ],
              Flexible(
                child: Text(
                  label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    color: fg,
                    fontWeight: FontWeight.w700,
                    fontSize: 15,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// The "who hasn't picked yet" strip with a Nudge action. Amber-bordered when
/// [urgent] (deadline near). Hidden by the caller when everyone has replied.
class PollUnrepliedStrip extends StatelessWidget {
  final List<User> unreplied;
  final String message;
  final bool urgent;
  final Color accentColor;
  final VoidCallback? onNudge;

  const PollUnrepliedStrip({
    super.key,
    required this.unreplied,
    required this.message,
    required this.urgent,
    required this.accentColor,
    required this.onNudge,
  });

  @override
  Widget build(BuildContext context) {
    final borderColor = urgent
        ? AppColors.statusWarningOnDark.withValues(alpha: 0.45)
        : AppColors.darkBorder;
    final bg = urgent
        ? AppColors.statusWarningOnDark.withValues(alpha: 0.12)
        : AppColors.onContentImage.withValues(alpha: 0.05);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
      decoration: BoxDecoration(
        color: bg,
        border: Border.all(color: borderColor),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        children: [
          if (unreplied.isNotEmpty) ...[
            _avatars(),
            const SizedBox(width: 10),
          ],
          Expanded(
            child: Text(
              message,
              style: const TextStyle(
                color: AppColors.darkTextSecondary,
                fontSize: 13,
              ),
            ),
          ),
          if (onNudge != null && unreplied.isNotEmpty) ...[
            const SizedBox(width: 8),
            Tappable(
              semanticsLabel: context.l10n.locationPanelNudge,
              onTap: onNudge,
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 13, vertical: 7),
                decoration: BoxDecoration(
                  color: urgent
                      ? AppColors.statusWarningOnDark
                      : Colors.transparent,
                  border: Border.all(
                    color: urgent
                        ? AppColors.statusWarningOnDark
                        : accentColor.withValues(alpha: 0.55),
                  ),
                  borderRadius: BorderRadius.circular(100),
                ),
                child: Text(
                  context.l10n.locationPanelNudge,
                  style: TextStyle(
                    color: urgent ? const Color(0xFF10140F) : accentColor,
                    fontSize: 12.5,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _avatars() {
    final shown = unreplied.take(3).toList();
    const double d = 24;
    const double overlap = 7;
    final width = d + (shown.length - 1) * (d - overlap);
    return SizedBox(
      width: width,
      height: d,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          for (var i = 0; i < shown.length; i++)
            Positioned(
              left: i * (d - overlap),
              child: Container(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(color: AppColors.modalBackdrop, width: 2),
                ),
                child: UserAvatar(user: shown[i], radius: 11),
              ),
            ),
        ],
      ),
    );
  }
}

/// A "how everyone picked" row in the set-final view — letter chip · name ·
/// voter stack. Tapping re-targets the winner. [selected] outlines it.
class PollPickRow extends StatelessWidget {
  final String letter;
  final String name;
  final List<User> voters;
  final bool selected;
  final bool leading;
  final Color accentColor;
  final VoidCallback onTap;

  const PollPickRow({
    super.key,
    required this.letter,
    required this.name,
    required this.voters,
    required this.selected,
    required this.leading,
    required this.accentColor,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: Toggle(
        semanticsLabel: name,
        selected: selected,
        inMutuallyExclusiveGroup: true,
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
          decoration: BoxDecoration(
            color: selected
                ? accentColor.withValues(alpha: 0.10)
                : Colors.transparent,
            border: Border.all(
              color: selected
                  ? accentColor.withValues(alpha: 0.55)
                  : Colors.transparent,
            ),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              PollLetterChip(
                letter: letter,
                accentColor: accentColor,
                filled: leading,
                size: 24,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  name,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: AppColors.onContentImage,
                    fontSize: 14.5,
                    fontWeight: FontWeight.w600,
                    height: 1.3,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              PollVoterStack(voters: voters),
            ],
          ),
        ),
      ),
    );
  }
}

/// The sage "FINAL …" card in the set-final view: an icon, a kicker, the
/// chosen option's name, an optional sub-line, and an optional evidence line.
class PollFinalCard extends StatelessWidget {
  final IconData icon;
  final String kicker;
  final String name;
  final String? subtitle;
  final String? evidence;
  final Color accentColor;

  const PollFinalCard({
    super.key,
    required this.icon,
    required this.kicker,
    required this.name,
    required this.accentColor,
    this.subtitle,
    this.evidence,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: accentColor.withValues(alpha: 0.18),
        border: Border.all(color: accentColor.withValues(alpha: 0.5), width: 1.5),
        borderRadius: BorderRadius.circular(18),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 46,
            height: 46,
            decoration: BoxDecoration(
              color: accentColor.withValues(alpha: 0.25),
              borderRadius: BorderRadius.circular(14),
            ),
            alignment: Alignment.center,
            child: Icon(icon, size: 22, color: accentColor),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  kicker.toUpperCase(),
                  style: TextStyle(
                    color: accentColor,
                    fontSize: 10,
                    fontWeight: FontWeight.w800,
                    letterSpacing: 1,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  name,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: AppColors.onContentImage,
                    fontSize: 18,
                    fontWeight: FontWeight.w800,
                    height: 1.15,
                  ),
                ),
                if (subtitle != null && subtitle!.isNotEmpty) ...[
                  const SizedBox(height: 2),
                  Text(
                    subtitle!,
                    style: const TextStyle(
                      color: AppColors.darkTextSecondary,
                      fontSize: 13,
                    ),
                  ),
                ],
                if (evidence != null && evidence!.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(
                    evidence!,
                    style: const TextStyle(
                      color: AppColors.darkTextTertiary,
                      fontSize: 12,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}
