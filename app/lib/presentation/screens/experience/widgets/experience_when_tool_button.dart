import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// The active-poll actions — Add another time / Set the final time — rendered as
/// the icon-above-label control bar (matching the confirmed-state host
/// controls). Returns an empty list when neither action is available.
List<Widget> whenPollActions({
  required BuildContext context,
  required bool locked,
  required bool isOrganizer,
  required bool busy,
  required bool hasOptions,
  required VoidCallback onAddAnother,
  required VoidCallback onSetFinal,
}) {
  final l10n = context.l10n;
  return whenToolBar([
    if (!locked)
      Expanded(
        child: WhenToolButton(
          icon: Icons.add,
          label: l10n.timePollProposeAddAnother,
          onTap: busy ? null : onAddAnother,
        ),
      ),
    if (isOrganizer)
      Expanded(
        child: WhenToolButton(
          icon: Icons.check,
          label: l10n.timePollConfirmKicker,
          onTap: (!hasOptions || busy) ? null : onSetFinal,
        ),
      ),
  ]);
}

/// Wraps [tools] (each typically an `Expanded(WhenToolButton(...))`) in the
/// "When" screen's control bar: a hairline top border with a small lead gap.
/// Returns an empty list when there are no tools, so callers can spread it
/// directly into a children list.
List<Widget> whenToolBar(List<Widget> tools, {double topGap = 16}) {
  if (tools.isEmpty) return const [];
  return [
    SizedBox(height: topGap),
    Container(
      padding: const EdgeInsets.only(top: 13),
      decoration: const BoxDecoration(
        border: Border(top: BorderSide(color: GlassTokens.hairline)),
      ),
      child: Row(children: tools),
    ),
  ];
}

/// WhenToolButton is the icon-above-label control used across the event "When"
/// screen — the host control bar (Mark done · Change time · Ask the group) and
/// the poll actions (Add another time · Set the final time). A circular icon
/// over a short label; the [active] variant fills the circle with the accent.
/// Disabled (null [onTap]) dims the whole button.
class WhenToolButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool active;
  final VoidCallback? onTap;

  const WhenToolButton({
    super.key,
    required this.icon,
    required this.label,
    this.active = false,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap ?? () {},
      child: Opacity(
        opacity: onTap == null ? 0.5 : 1,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: active
                    ? AppColors.experienceSageGreen
                    : GlassTokens.fillSubtle,
                border: active
                    ? null
                    : Border.all(color: GlassTokens.borderSoft),
              ),
              child: Icon(icon, size: 20, color: AppColors.onContentImage),
            ),
            const SizedBox(height: 7),
            Text(
              label,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: AppColors.darkTextSecondary,
                fontSize: 11,
                fontWeight: FontWeight.w600,
                height: 1.15,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
