import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Visual style of a checklist row.
enum _ChecklistRowStyle { suggestion, need, claimed }

/// Small coral pill rendered inline on a [SuggestionRow] / [NeedRow] to
/// flag that tapping the row opens a poll-creation flow rather than
/// just adding a thing. Used by the "Agree Upon Time" entry.
class PollChip extends StatelessWidget {
  final String label;

  const PollChip({super.key, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
      decoration: BoxDecoration(
        color: AppColors.transferCoralSoft,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w700,
          color: AppColors.onContentImage,
          height: 1.2,
          letterSpacing: 0.3,
        ),
      ),
    );
  }
}

/// Public-facing checkbox style for [NeedRow]. Matches the internal
/// `_ChecklistRowStyle` variants but is exposed so callers can pick
/// "Still Needed" (solid checkbox) vs "Suggestions" (dashed checkbox).
enum NeedRowStyle { need, suggestion }

/// Lays out a flat list of items in two columns, row-major: items
/// `[0, mid)` fill the left column, items `[mid, n)` fill the right.
/// `mid = ceil(n / 2)` so the left column is the longer one when `n`
/// is odd. The last row in each column has its divider suppressed via
/// the `showDivider` flag passed to [buildRow].
///
/// Public counterpart of the private helper used by the Plan tab —
/// the batch sheets use this too so their suggestion lists match the
/// Plan tab's two-column treatment.
Widget buildChecklistTwoColumns<T>({
  required List<T> items,
  required Widget Function(T item, bool showDivider) buildRow,
}) {
  if (items.isEmpty) return const SizedBox.shrink();
  if (items.length == 1) return buildRow(items.first, false);
  final mid = (items.length / 2).ceil();
  final leftEnd = mid - 1;
  final rightEnd = items.length - 1;
  final left = <Widget>[];
  final right = <Widget>[];
  for (var i = 0; i < items.length; i++) {
    final showDivider = i != leftEnd && i != rightEnd;
    final row = buildRow(items[i], showDivider);
    if (i < mid) {
      left.add(row);
    } else {
      right.add(row);
    }
  }
  return Row(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: left,
        ),
      ),
      const SizedBox(width: 16),
      Expanded(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: right,
        ),
      ),
    ],
  );
}

/// SuggestionRow renders the zero-state checklist row used in the
/// Plan-tab empty state and in the request/offer modal suggestion list:
/// a dashed checkbox + label + optional inline chip + trailing coral
/// "+ Add" text button. Set [showCheckbox] to false to hide the
/// dashed-square leading icon — used inside the batch sheets where the
/// two-column compact list looks cleaner without per-row checkboxes.
class SuggestionRow extends StatelessWidget {
  final String label;

  /// Tap on the trailing action (e.g. "+ Add", "+ Poll").
  final VoidCallback onAction;

  /// Localized text for the action affordance. Pass `context.l10n.*`.
  final String actionLabel;

  /// Localized semantic label for the action. Pass `context.l10n.*`.
  final String actionSemanticsLabel;

  /// Optional small chip rendered inline between the label and the
  /// trailing action — used by the "Agree Upon Time" entry to flag
  /// that tapping opens a poll-creation flow.
  final Widget? chip;

  /// When false, the dashed leading checkbox is omitted.
  final bool showCheckbox;

  final bool showDivider;

  const SuggestionRow({
    super.key,
    required this.label,
    required this.onAction,
    required this.actionLabel,
    required this.actionSemanticsLabel,
    this.chip,
    this.showCheckbox = true,
    this.showDivider = true,
  });

  @override
  Widget build(BuildContext context) {
    // Tapping anywhere on the row fires the action — the trailing label
    // is a visual affordance, not the sole tap target. This matches the
    // way [NeedRow] behaves and means callers that hide the trailing
    // label (by passing `actionLabel: ''`) still get a working tap.
    final hasTrailingLabel = actionLabel.isNotEmpty;
    return Tappable(
      semanticsLabel: actionSemanticsLabel,
      onTap: onAction,
      child: _ChecklistRowFrame(
        style: _ChecklistRowStyle.suggestion,
        label: label,
        chip: chip,
        showCheckbox: showCheckbox,
        showDivider: showDivider,
        trailing: hasTrailingLabel
            ? Padding(
                padding:
                    const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
                child: Text(
                  actionLabel,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: AppColors.transferCoralSoft,
                  ),
                ),
              )
            : null,
      ),
    );
  }
}

/// NeedRow renders a tappable checklist item. The whole row is the tap
/// target — there is no trailing Claim button. Use [style] to pick the
/// "Still Needed" (solid square) or "Suggestions" (dashed square)
/// appearance.
class NeedRow extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  final String rowSemanticsLabel;
  final NeedRowStyle style;

  /// Optional small inline chip rendered between the label and the
  /// row's right edge — used for the "Agree Upon Time" Poll badge.
  final Widget? chip;

  final bool showDivider;

  const NeedRow({
    super.key,
    required this.label,
    required this.onTap,
    required this.rowSemanticsLabel,
    this.style = NeedRowStyle.need,
    this.chip,
    this.showDivider = true,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: rowSemanticsLabel,
      onTap: onTap,
      child: _ChecklistRowFrame(
        style: style == NeedRowStyle.need
            ? _ChecklistRowStyle.need
            : _ChecklistRowStyle.suggestion,
        label: label,
        chip: chip,
        showDivider: showDivider,
        trailing: null,
      ),
    );
  }
}

/// ClaimedRow renders a fulfilled checklist item: a filled sage-green
/// check + label + trailing roster of claimers. When more than one
/// person is bringing the same item the row aggregates them — first
/// name of the lead claimer plus a "+N" overflow count, with the lead
/// claimer's avatar flush right.
class ClaimedRow extends StatelessWidget {
  final String label;

  /// Primary claimer rendered in the trailing slot.
  final User claimer;

  /// Additional users also bringing the same item. Drives the "+N"
  /// overflow count to the right of the lead name.
  final List<User> additionalClaimers;

  /// Optional tap on the row (e.g. open contribution details / note).
  final VoidCallback? onTap;

  /// Localized semantic label for the row tap target (only used when
  /// [onTap] is non-null). Pass `context.l10n.*`.
  final String? rowSemanticsLabel;

  final bool showDivider;

  const ClaimedRow({
    super.key,
    required this.label,
    required this.claimer,
    this.additionalClaimers = const [],
    this.onTap,
    this.rowSemanticsLabel,
    this.showDivider = true,
  }) : assert(onTap == null || rowSemanticsLabel != null,
            'rowSemanticsLabel required when onTap is non-null');

  @override
  Widget build(BuildContext context) {
    final firstName = claimer.name.split(' ').first;
    final extraCount = additionalClaimers.length;
    final frame = _ChecklistRowFrame(
      style: _ChecklistRowStyle.claimed,
      label: label,
      showDivider: showDivider,
      // Avatar first, then name+"+N" as a single Text.rich run. The
      // frame's Spacer pushes this whole trailing block to the right
      // edge of the row, which makes the *name's* right edge land
      // exactly on the row's right edge on every row — true
      // right-justification across the column, regardless of name
      // length or whether there are co-bringers. The avatars' X
      // positions then float left based on the name width, which is
      // an intentional trade-off for the requested name alignment.
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          UserAvatar(user: claimer, radius: 10),
          const SizedBox(width: 6),
          Text.rich(
            TextSpan(
              style: const TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w500,
                color: AppColors.onContentImage,
              ),
              children: [
                TextSpan(text: firstName),
                if (extraCount > 0)
                  TextSpan(
                    text: ' +$extraCount',
                    style: TextStyle(
                      fontWeight: FontWeight.w600,
                      color:
                          AppColors.onContentImage.withValues(alpha: 0.75),
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );

    if (onTap == null) return frame;
    return Tappable(
      semanticsLabel: rowSemanticsLabel!,
      onTap: onTap,
      child: frame,
    );
  }
}

class _ChecklistRowFrame extends StatelessWidget {
  final _ChecklistRowStyle style;
  final String label;
  final Widget? chip;
  final Widget? trailing;
  final bool showCheckbox;
  final bool showDivider;

  const _ChecklistRowFrame({
    required this.style,
    required this.label,
    this.chip,
    required this.trailing,
    this.showCheckbox = true,
    required this.showDivider,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 10),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: showDivider ? GlassTokens.hairline : Colors.transparent,
            width: 1,
          ),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          if (showCheckbox) ...[
            _CheckboxIcon(style: style),
            const SizedBox(width: 12),
          ],
          // Expanded — not Flexible + Spacer — so the label always
          // absorbs *all* the space between the checkbox and the
          // trailing slot. With the old Flexible/Spacer pair the
          // label only used what it needed (its loose allocation),
          // and the slack at the end pushed the trailing slot left
          // of the row's right edge by a different amount on every
          // row depending on label width. Expanded fixes that — the
          // trailing block is pinned flush right on every row.
          Expanded(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 13,
                color: AppColors.onContentImage,
              ),
            ),
          ),
          if (chip != null) ...[
            const SizedBox(width: 6),
            chip!,
          ],
          if (trailing != null) ...[
            const SizedBox(width: 8),
            trailing!,
          ],
        ],
      ),
    );
  }
}

class _CheckboxIcon extends StatelessWidget {
  final _ChecklistRowStyle style;
  const _CheckboxIcon({required this.style});

  @override
  Widget build(BuildContext context) {
    switch (style) {
      case _ChecklistRowStyle.suggestion:
        return DottedSquare(
          color: Colors.white.withValues(alpha: 0.45),
          size: 18,
        );
      case _ChecklistRowStyle.need:
        return Container(
          width: 18,
          height: 18,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(4),
            border: Border.all(
              color: Colors.white.withValues(alpha: 0.55),
              width: 1.5,
            ),
          ),
        );
      case _ChecklistRowStyle.claimed:
        return Container(
          width: 18,
          height: 18,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(4),
            color: AppColors.experienceSageGreen,
          ),
          alignment: Alignment.center,
          child: const Icon(Icons.check, size: 13, color: GlassTokens.textPrimary),
        );
    }
  }
}

/// DottedSquare draws a square outline using a dashed stroke. Public so
/// the suggestion-row checkbox icon can be reused outside this file.
class DottedSquare extends StatelessWidget {
  final Color color;
  final double size;

  const DottedSquare({super.key, required this.color, this.size = 18});

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      size: Size(size, size),
      painter: _DashedRectPainter(color: color),
    );
  }
}

class _DashedRectPainter extends CustomPainter {
  final Color color;
  _DashedRectPainter({required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;
    const dash = 3.0;
    const gap = 2.0;
    final rect = RRect.fromRectAndRadius(
      Rect.fromLTWH(0, 0, size.width, size.height),
      const Radius.circular(4),
    );
    final path = Path()..addRRect(rect);
    final metrics = path.computeMetrics();
    for (final m in metrics) {
      double distance = 0;
      while (distance < m.length) {
        final next = (distance + dash).clamp(0, m.length);
        canvas.drawPath(m.extractPath(distance, next.toDouble()), paint);
        distance = next + gap;
      }
    }
  }

  @override
  bool shouldRepaint(covariant _DashedRectPainter old) => old.color != color;
}
